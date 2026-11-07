// Package app is Althea's composition root: it builds every service, wires them together and
// runs the application. It is the only package that knows all the others.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/fswatch"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/netpolicy"
	"github.com/JoaoVitorOli/althea/internal/paths"
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/platform/host"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/theme"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// Run starts Althea and blocks until it quits. A second launch shows the running instance.
func Run() (err error) {
	started := time.Now()
	services := host.Services()
	p, err := paths.Resolve(runtime.GOOS, os.Getenv, portableDir())
	if err != nil {
		return err
	}

	// One Althea per set of data: a portable copy runs beside an installed one.
	a := &althea{services: services, started: started, keys: &keyboard{hook: services.KeyHook}}
	first, release, err := services.Instance.Lock(instanceName(p.Data), func() {
		mygo.RunOnMain(func() {
			if a.launcher != nil {
				a.launcher.Show()
			}
		})
	})
	if err != nil {
		return err
	}
	if !first {
		return nil // the running Althea shows itself
	}
	defer release()

	c, err := bootstrap(context.Background(), p, services.Locale())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, c.Close()) }()
	a.core = c
	a.net = netpolicy.New(c.log, nil)

	mygo.App.SetName("Althea")
	mygo.App.OnWindowAllClosed(func() {}) // keep running in the tray with the launcher hidden
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		c.log.Error("app stopped", "err", err)
		return fmt.Errorf("run app: %w", err)
	}
	c.log.Info("quit")
	return nil
}

// althea is the running app: the launcher, the tray and the global hotkey, kept in step with
// the settings and the theme. Its fields are used on the main thread.
type althea struct {
	*core
	services platform.Services
	started  time.Time

	launcher *shell.Launcher
	root     *ui.Root
	tray     *shell.Tray
	search   *searcher
	// pasteTarget is the window that had the keyboard before the launcher showed, which
	// Paste pastes into.
	pasteTarget atomic.Uintptr
	prefs       *preferences
	// prefsSettings are the settings commands, which also ask for missing preferences.
	prefsSettings *settingsExtension
	// background runs the extensions' services.
	background *services
	// net is the only way to the network.
	net *netpolicy.Policy
	// selection reads the text selected in other apps.
	selection selectionReader
	// keys shares the keyboard hook.
	keys *keyboard
	// shortcuts binds the extensions' hotkeys, in order.
	shortcuts serial
	// commands are the hotkeys the user gave commands.
	commands commandHotkeys
	hotkey   string
	unbind   func()
	// notes is the floating notes' window.
	notes *floatingNotes
	// themes is the theme editor.
	themes themeEditor
	// syncing keeps the computers in step through the sync folder.
	syncing *folderSync
	// updates checks for and installs new versions.
	updates *updater
}

func (a *althea) ready() {
	// Made before anything that may ask it to sync; off until setUpSync turns it on.
	a.syncing = &folderSync{a: a}
	a.updates = newUpdater(a)
	s := a.Settings()
	a.root = ui.NewRoot(a.catalog.T, a.loadTheme(s.Theme, mygo.Theme.IsDark()))
	a.launcher = shell.Open(shell.Options{
		View:       a.root.View,
		Material:   a.root.Theme().Material.Kind,
		ResetAfter: time.Duration(s.Window.ResetAfter),
		// What was done in the launcher may have changed what syncs.
		OnHide: func() { a.syncing.soon() },
		OnShow: func(reset bool) {
			if p := a.services.Paster; p != nil {
				a.pasteTarget.Store(p.Foreground())
			}
			if reset {
				a.root.Reset()
			}
			if a.search != nil {
				go a.search.refresh()
			}
			a.updates.soon()
		},
		Log: a.log,
	})
	a.applyChrome()
	if err := a.setUpSearch(); err != nil {
		a.log.Error("search not set up", "err", err)
	}
	a.bindHotkey(s.Hotkey.Toggle)
	a.commands = commandHotkeys{bind: a.bindCommandHotkey, run: a.runCommand}
	a.commands.sync(wantedHotkeys(s.Hotkey, a.log))
	a.syncLogin(s.General.StartAtLogin)
	a.setUpTray()
	a.watchThemes()

	// Settings reload on the watcher's goroutine; apply them on the main thread.
	a.OnSettings(func(s config.Settings) {
		a.launcher.Window().Update(func() { a.apply(s) })
		a.syncing.soon()
	})
	mygo.Theme.OnUpdated(func() { a.launcher.Window().Update(a.reloadTheme) })

	firstRun := a.needsOnboarding(context.Background())
	if firstRun {
		a.root.Onboarding = ui.NewOnboarding(a.catalog.T, ui.MotionOf(a.root.Theme()), choicesOf(s),
			a.services.KeyHook != nil, a.onboarded)
	}
	if firstRun || !mygo.App.WasOpenedAtLogin() {
		a.launcher.Show() // started by the user: show what it does
	}
	a.updates.soon()
	a.log.Info("ready", "hotkey", a.hotkey, "theme", a.root.Theme().Name, "since_start", time.Since(a.started))
}

// onboarded saves the onboarding's choices, off the main thread, and turns to the search. The
// settings watcher applies the choices as the file changes.
func (a *althea) onboarded(ch ui.Choices) {
	a.root.Onboarding = nil
	a.root.Reset()
	go func() {
		if err := a.finishOnboarding(context.Background(), ch); err != nil {
			a.log.Error("onboarding not saved", "err", err)
			return
		}
		a.log.Info("onboarding finished", "hotkey", ch.Hotkey, "theme", ch.Theme)
	}()
}

// apply brings the running app in step with reloaded settings.
func (a *althea) apply(s config.Settings) {
	a.launcher.SetResetAfter(time.Duration(s.Window.ResetAfter))
	a.applyHotkeys(s.Hotkey)
	a.syncLogin(s.General.StartAtLogin)
	a.reloadTheme()
}

func (a *althea) reloadTheme() {
	t := a.loadTheme(a.Settings().Theme, mygo.Theme.IsDark())
	a.root.SetTheme(t)
	a.launcher.SetMaterial(t.Material.Kind)
	a.applyChrome()
	if a.notes != nil {
		a.notes.setTheme()
	}
	a.themes.setChrome(t)
}

// watchThemes reloads the theme when a file in the themes folder changes.
func (a *althea) watchThemes() {
	isTheme := func(name string) bool { return strings.HasSuffix(name, ".toml") }
	w, err := fswatch.WatchDir(a.paths.Themes, isTheme, func() {
		a.launcher.Window().Update(a.reloadTheme)
		a.syncing.soon()
	})
	if err != nil {
		a.log.Warn("themes not watched", "err", err)
		return
	}
	a.closers = append(a.closers, w.Close)
}

// applyChrome colors the border Windows draws around the launcher with the theme's, laid over
// its background, since the system draws borders opaque.
func (a *althea) applyChrome() {
	c := a.root.Theme().Colors
	border := over(c.Border, c.Background)
	win := a.launcher.Window()
	mygo.RunOnMain(func() {
		if err := a.services.Chrome.SetBorderColor(win.NativeHandle(), border); err != nil {
			a.log.Debug("window border color", "err", err)
		}
	})
}

// over lays color fg over bg, opaque.
func over(fg, bg theme.Color) platform.Color {
	mix := func(f, b uint8) uint8 {
		v := (uint16(f)*uint16(fg.A) + uint16(b)*(255-uint16(fg.A))) / 255
		return uint8(v) //nolint:gosec // a weighted mean of two bytes is a byte
	}
	return platform.Color{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B)}
}

// applyHotkeys binds the hotkeys of reloaded settings. The launcher's and a command's may swap
// keys, and the system gives a key to one at a time: the commands let go of theirs first, and
// the rest follows on the main thread once they did, which nothing here waits for.
func (a *althea) applyHotkeys(s config.Hotkey) {
	a.commands.release()
	a.shortcuts.do(func() {
		a.launcher.Window().Update(func() {
			if s.Toggle != a.hotkey {
				a.bindHotkey(s.Toggle)
			}
			a.commands.sync(wantedHotkeys(s, a.log))
		})
	})
}

// bindHotkey makes shortcut show and hide the launcher, in place of the one bound before.
func (a *althea) bindHotkey(shortcut string) {
	if a.unbind != nil {
		a.unbind()
		a.unbind = nil
	}
	a.hotkey, a.root.Hotkey = "", ""
	b, err := hotkey.Parse(shortcut)
	if err != nil {
		a.log.Warn("hotkey not understood", "hotkey", shortcut, "err", err)
		return
	}
	if b.WinKey {
		a.unbind, err = a.bindWinKey()
	} else {
		err = mygo.GlobalShortcut.Register(b.Accelerator, a.launcher.Toggle)
		a.unbind = func() { mygo.GlobalShortcut.Unregister(b.Accelerator) }
	}
	if err != nil {
		a.unbind = nil
		a.log.Warn("hotkey not registered", "hotkey", shortcut, "err", err)
		return
	}
	a.hotkey, a.root.Hotkey = shortcut, shortcut
	a.log.Info("hotkey bound", "hotkey", shortcut)
}

// bindWinKey opens the launcher when the Win key is tapped alone, through the keyboard hook,
// keeping the Start menu closed for those taps. Win with another key still reaches Windows.
func (a *althea) bindWinKey() (unbind func(), err error) {
	if a.services.KeyHook == nil {
		return nil, errors.New("this system cannot watch the Win key")
	}
	var tap hotkey.WinTap // used on the hook's thread alone
	return a.keys.listen(func(e hotkey.KeyEvent) {
		tapped, mask := tap.Feed(e)
		if mask {
			a.services.KeyHook.Mask()
		}
		if tapped {
			// The hook must return at once: toggle on the main thread, later.
			go a.launcher.Window().Update(a.launcher.Toggle)
		}
	})
}

func (a *althea) syncLogin(start bool) {
	if mygo.App.OpenAtLogin() == start {
		return
	}
	if err := mygo.App.SetOpenAtLogin(start); err != nil {
		a.log.Warn("start at login not changed", "err", err)
	}
}

func (a *althea) setUpTray() {
	open := func(path string) func() {
		return func() {
			_ = os.MkdirAll(path, 0o700)
			if err := mygo.Shell.OpenPath(path); err != nil {
				a.log.Warn("open", "path", path, "err", err)
			}
		}
	}
	tray, err := shell.NewTray(shell.TrayActions{
		T:            a.catalog.T,
		Hotkey:       a.hotkey,
		Open:         a.launcher.Show,
		Settings:     open(a.paths.Settings),
		Themes:       open(a.paths.Themes),
		Logs:         open(a.paths.Logs),
		StartAtLogin: mygo.App.OpenAtLogin,
		SetLogin:     a.syncLogin,
	})
	if err != nil {
		a.log.Warn("no tray icon", "err", err)
		return
	}
	a.tray = tray
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
}

// portableDir returns the executable's directory when an althea.portable file sits beside it,
// as in the portable zip, and "" otherwise.
func portableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "althea.portable")); err != nil {
		return ""
	}
	return dir
}

// instanceName names the single-instance lock of the data in dir.
func instanceName(dir string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
	return "com.joaovitoroli.althea." + hex.EncodeToString(sum[:6])
}
