// Package app is Althea's composition root: it builds every service, wires them together and
// runs the application. It is the only package that knows all the others.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/fswatch"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/paths"
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/platform/host"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/theme"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// Run starts Althea and blocks until it quits. A second launch shows the running instance.
func Run() (err error) {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running Althea shows itself (OnSecondInstance)
	}
	services := host.Services()
	p, err := paths.Resolve(runtime.GOOS, os.Getenv, portableDir())
	if err != nil {
		return err
	}
	c, err := bootstrap(context.Background(), p, services.Locale())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, c.Close()) }()

	a := &althea{core: c, services: services}
	mygo.App.SetName("Althea")
	mygo.App.OnWindowAllClosed(func() {}) // keep running in the tray with the launcher hidden
	mygo.App.OnSecondInstance(func([]string, string) { a.launcher.Show() })
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

	launcher *shell.Launcher
	root     *ui.Root
	tray     *shell.Tray
	hotkey   string
	unbind   func()
}

func (a *althea) ready() {
	s := a.Settings()
	a.root = ui.NewRoot(a.catalog.T, a.loadTheme(s.Theme, mygo.Theme.IsDark()))
	a.launcher = shell.Open(shell.Options{
		View:       a.root.View,
		Material:   a.root.Theme().Material.Kind,
		ResetAfter: time.Duration(s.Window.ResetAfter),
		OnShow: func(reset bool) {
			if reset {
				a.root.Reset()
			}
		},
		Log: a.log,
	})
	a.applyChrome()
	a.bindHotkey(s.Hotkey.Toggle)
	a.syncLogin(s.General.StartAtLogin)
	a.setUpTray()
	a.watchThemes()

	// Settings reload on the watcher's goroutine; apply them on the main thread.
	a.OnSettings(func(s config.Settings) { a.launcher.Window().Update(func() { a.apply(s) }) })
	mygo.Theme.OnUpdated(func() { a.launcher.Window().Update(a.reloadTheme) })

	firstRun := a.needsOnboarding(context.Background())
	if firstRun {
		a.root.Onboarding = ui.NewOnboarding(a.catalog.T, ui.MotionOf(a.root.Theme()), choicesOf(s),
			a.services.KeyHook != nil, a.onboarded)
	}
	if firstRun || !mygo.App.WasOpenedAtLogin() {
		a.launcher.Show() // started by the user: show what it does
	}
	a.log.Info("ready", "hotkey", a.hotkey, "theme", a.root.Theme().Name)
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
	if s.Hotkey.Toggle != a.hotkey {
		a.bindHotkey(s.Hotkey.Toggle)
	}
	a.syncLogin(s.General.StartAtLogin)
	a.reloadTheme()
}

func (a *althea) reloadTheme() {
	t := a.loadTheme(a.Settings().Theme, mygo.Theme.IsDark())
	a.root.SetTheme(t)
	a.launcher.SetMaterial(t.Material.Kind)
	a.applyChrome()
}

// watchThemes reloads the theme when a file in the themes folder changes.
func (a *althea) watchThemes() {
	isTheme := func(name string) bool { return strings.HasSuffix(name, ".toml") }
	w, err := fswatch.WatchDir(a.paths.Themes, isTheme, func() {
		a.launcher.Window().Update(a.reloadTheme)
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
	keys := a.services.KeyHook
	if keys == nil {
		return nil, errors.New("this system cannot watch the Win key")
	}
	var tap hotkey.WinTap // used on the hook's thread alone
	return keys.Start(func(e hotkey.KeyEvent) {
		tapped, mask := tap.Feed(e)
		if mask {
			keys.Mask()
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
