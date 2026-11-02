// Package windows is the Window Management extension: it moves the window the user is in to a
// half, third or quarter of the screen, centers or maximizes it, sends it to the next display and
// back where it was, each from the root search or a hotkey of the user's. Layouts place a set of
// apps together with one command.
package windows

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Display is a screen, without its taskbar.
type Display struct {
	ID   string
	Work Rect
}

// Window is an app's window on the screen.
type Window struct {
	Handle uintptr
	// App opens the app again, as Launch reads it.
	App       string
	Title     string
	Frame     Rect
	Maximized bool
}

// Platform is how the extension reaches the system; the host provides it. A nil Target leaves
// the extension without commands, as on a system that cannot move windows.
type Platform struct {
	// Target returns the window to act on: the one the user was in before the launcher showed,
	// or the one in front when a hotkey runs with the launcher hidden.
	Target func() uintptr
	// Displays returns the screens.
	Displays func() ([]Display, error)
	// Frame returns where a window is, and fails on what is no app's window.
	Frame func(window uintptr) (frame Rect, maximized bool, err error)
	// Place moves a window and raises it, leaving it neither maximized nor minimized.
	Place    func(window uintptr, r Rect) error
	Maximize func(window uintptr) error
	Focus    func(window uintptr) error
	// OpenWindows returns the apps' windows on the screen, topmost first.
	OpenWindows func() ([]Window, error)
	// Launch opens an app.
	Launch func(app string) error
	// Bind runs run, off the main thread, when hotkey is pressed anywhere, until unbind is
	// called; a hotkey that cannot be had is logged. Nil leaves hotkeys out. It must not wait
	// for the main thread, as Althea may be quitting there.
	Bind func(hotkey string, run func()) (unbind func())
	// ValidHotkey reports whether a hotkey typed in a form can be read.
	ValidHotkey func(hotkey string) bool
	// Patience is how long a layout waits for the apps it opened; 0 means 15 seconds.
	Patience time.Duration
}

// Extension is the Window Management extension.
type Extension struct {
	p Platform
	// changed tells Serve that the layouts' hotkeys changed.
	changed chan struct{}

	mu     sync.Mutex // guards before
	before map[uintptr]placed
}

// placed is where a window was before Althea moved it, and where Althea left it.
type placed struct {
	before    Rect
	maximized bool
	left      Rect
}

// New returns the Window Management extension.
func New(p Platform) *Extension {
	return &Extension{p: p, changed: make(chan struct{}, 1), before: map[uintptr]placed{}}
}

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.windows", Title: "Window Management", Icon: sdk.Icon{Glyph: "window.maximize"}, Messages: messages}
}

// action is one of the commands that move a window. Its id also names its hotkey preference.
type action struct {
	id       string
	glyph    string
	keywords []string
	run      func(ctx sdk.Context) error
}

// actions are the commands, in the order the settings screen lists their hotkeys.
func (e *Extension) actions() []action {
	grid := func(col, cols, row, rows int) func(ctx sdk.Context) error {
		return e.snap(func(a, _ Rect) Rect { return part(a, col, cols, row, rows) })
	}
	thirdOf := func(i int) func(ctx sdk.Context) error {
		return e.snap(func(a, _ Rect) Rect { return third(a, i) })
	}
	return []action{
		{"left", "window.left", []string{"left half", "metade esquerda"}, grid(0, 2, 0, 1)},
		{"right", "window.right", []string{"right half", "metade direita"}, grid(1, 2, 0, 1)},
		{"top", "window.top", []string{"top half", "metade superior", "metade de cima"}, grid(0, 1, 0, 2)},
		{"bottom", "window.bottom", []string{"bottom half", "metade inferior", "metade de baixo"}, grid(0, 1, 1, 2)},
		{"third_first", "window.third_first", []string{"first third", "primeiro terço"}, thirdOf(0)},
		{"third_center", "window.third_center", []string{"center third", "terço central"}, thirdOf(1)},
		{"third_last", "window.third_last", []string{"last third", "último terço"}, thirdOf(2)},
		{"top_left", "window.top_left", []string{"top left quarter", "quarto superior esquerdo"}, grid(0, 2, 0, 2)},
		{"top_right", "window.top_right", []string{"top right quarter", "quarto superior direito"}, grid(1, 2, 0, 2)},
		{"bottom_left", "window.bottom_left", []string{"bottom left quarter", "quarto inferior esquerdo"}, grid(0, 2, 1, 2)},
		{"bottom_right", "window.bottom_right", []string{"bottom right quarter", "quarto inferior direito"}, grid(1, 2, 1, 2)},
		{"center", "window.center", []string{"center window", "centralizar janela"}, e.snap(func(a, f Rect) Rect { return centered(a, f.W, f.H) })},
		{"maximize", "window.maximize", []string{"maximize window", "maximizar janela", "tela cheia"}, e.maximize},
		{"almost_maximize", "window.almost", []string{"almost maximize", "quase maximizar"}, e.snap(func(a, _ Rect) Rect { return almostMaximized(a) })},
		{"restore", "restart", []string{"restore window", "restaurar janela", "desfazer"}, e.restore},
		{"next_display", "window.display", []string{"next display", "next screen", "move to monitor", "próxima tela", "próximo monitor"}, e.nextDisplay},
	}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	if e.p.Target == nil {
		return nil
	}
	var cmds []sdk.Command
	for _, a := range e.actions() {
		cmds = append(cmds, sdk.Command{
			ID: a.id, Title: ctx.T(a.id), Icon: sdk.Icon{Glyph: a.glyph},
			Keywords: append([]string{"window", "janela"}, a.keywords...), Run: a.run,
		})
	}
	return append(cmds, e.layoutCommands(ctx)...)
}

// Preferences implements sdk.Configurable: a hotkey for each command, none at first, since
// any default could take a combination another app uses.
func (e *Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	if e.p.Target == nil || e.p.Bind == nil {
		return nil
	}
	var out []sdk.Preference
	for _, a := range e.actions() {
		out = append(out, sdk.Preference{
			Name: a.id, Title: ctx.T(a.id), Description: ctx.T("hotkey_info"), Kind: sdk.HotkeyPreference,
		})
	}
	return out
}

// Serve implements sdk.Service: it keeps the hotkeys bound while Althea runs. The host runs it
// again when the preferences change; changes to the layouts arrive through changed.
func (e *Extension) Serve(ctx sdk.Context) error {
	if e.p.Target == nil || e.p.Bind == nil {
		return nil
	}
	for {
		unbind := e.bindAll(ctx)
		select {
		case <-ctx.Done():
			unbind()
			return nil
		case <-e.changed:
			unbind()
		}
	}
}

func (e *Extension) bindAll(ctx sdk.Context) (unbind func()) {
	var unbinds []func()
	bind := func(hotkey string, run func(ctx sdk.Context) error) {
		if hotkey == "" {
			return
		}
		unbinds = append(unbinds, e.p.Bind(hotkey, func() {
			if err := run(ctx); err != nil {
				ctx.Log().Warn("hotkey failed", "hotkey", hotkey, "err", err)
			}
		}))
	}
	for _, a := range e.actions() {
		bind(ctx.Preferences().String(a.id), a.run)
	}
	layouts, err := Load(ctx)
	if err != nil {
		ctx.Log().Warn("layouts not read", "err", err)
	}
	for _, l := range layouts {
		bind(l.Hotkey, func(ctx sdk.Context) error { return e.apply(ctx, l) })
	}
	return func() {
		for _, u := range unbinds {
			u()
		}
	}
}

// subject finds the window to act on and the screens. It runs while the launcher still shows, so
// that what goes wrong is seen.
func (e *Extension) subject(ctx sdk.Context) (w uintptr, frame Rect, maximized bool, displays []Display, err error) {
	w = e.p.Target()
	if frame, maximized, err = e.p.Frame(w); err != nil {
		ctx.Log().Debug("no window to move", "err", err)
		return 0, Rect{}, false, nil, errors.New(ctx.T("no_window"))
	}
	if displays, err = e.p.Displays(); err != nil {
		return 0, Rect{}, false, nil, err
	}
	if len(displays) == 0 {
		return 0, Rect{}, false, nil, errors.New(ctx.T("no_window"))
	}
	return w, frame, maximized, displays, nil
}

// snap moves the window to where place says, from the work area of its display and its frame.
func (e *Extension) snap(place func(work, frame Rect) Rect) func(ctx sdk.Context) error {
	return func(ctx sdk.Context) error {
		w, frame, maximized, displays, err := e.subject(ctx)
		if err != nil {
			return err
		}
		ctx.Hide()
		to := place(displayOf(displays, frame).Work, frame)
		return e.move(ctx, w, frame, maximized, func() error { return e.p.Place(w, to) })
	}
}

func (e *Extension) maximize(ctx sdk.Context) error {
	w, frame, maximized, _, err := e.subject(ctx)
	if err != nil {
		return err
	}
	ctx.Hide()
	return e.move(ctx, w, frame, maximized, func() error { return e.p.Maximize(w) })
}

// nextDisplay sends the window to the next screen, left to right, in the same place on it; a
// maximized window stays so.
func (e *Extension) nextDisplay(ctx sdk.Context) error {
	w, frame, maximized, displays, err := e.subject(ctx)
	if err != nil {
		return err
	}
	if len(displays) < 2 {
		return errors.New(ctx.T("one_display"))
	}
	ordered := slices.Clone(displays)
	slices.SortStableFunc(ordered, func(a, b Display) int {
		if a.Work.X != b.Work.X {
			return a.Work.X - b.Work.X
		}
		return a.Work.Y - b.Work.Y
	})
	from := displayOf(ordered, frame)
	i := slices.IndexFunc(ordered, func(d Display) bool { return d.ID == from.ID })
	to := ordered[(i+1)%len(ordered)]
	ctx.Hide()
	return e.move(ctx, w, frame, maximized, func() error {
		if err := e.p.Place(w, fractionOf(frame, from.Work).in(to.Work)); err != nil {
			return err
		}
		if maximized {
			return e.p.Maximize(w)
		}
		return nil
	})
}

// move does a move, keeps where the window was for Restore, and gives the window the keyboard
// the launcher had.
func (e *Extension) move(ctx sdk.Context, w uintptr, frame Rect, maximized bool, do func() error) error {
	if err := do(); err != nil {
		ctx.Log().Debug("window not moved", "err", err)
		return fmt.Errorf("%s: %w", ctx.T("not_moved"), err)
	}
	left, _, err := e.p.Frame(w)
	if err == nil {
		e.remember(w, frame, maximized, left)
	}
	e.focus(ctx, w)
	return nil
}

// remember keeps where a window was. After several moves in a row that is still where it was
// before the first; a window the user moved since starts anew from where they put it.
func (e *Extension) remember(w uintptr, frame Rect, maximized bool, left Rect) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.before[w]
	if !ok || p.left != frame {
		p = placed{before: frame, maximized: maximized}
	}
	p.left = left
	e.before[w] = p
	if len(e.before) > maxRemembered {
		for old := range e.before {
			if _, _, err := e.p.Frame(old); err != nil {
				delete(e.before, old)
			}
		}
	}
}

// maxRemembered is how many windows Restore knows before forgetting those that closed.
const maxRemembered = 64

// restore puts the window back where it was before Althea moved it.
func (e *Extension) restore(ctx sdk.Context) error {
	w, _, _, _, err := e.subject(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	p, ok := e.before[w]
	delete(e.before, w)
	e.mu.Unlock()
	if !ok {
		return errors.New(ctx.T("nothing_to_restore"))
	}
	ctx.Hide()
	if err := e.p.Place(w, p.before); err != nil {
		return fmt.Errorf("%s: %w", ctx.T("not_moved"), err)
	}
	if p.maximized {
		if err := e.p.Maximize(w); err != nil {
			return fmt.Errorf("%s: %w", ctx.T("not_moved"), err)
		}
	}
	e.focus(ctx, w)
	return nil
}

// focus gives a window the keyboard. The system may refuse, which leaves the window moved all
// the same.
func (e *Extension) focus(ctx sdk.Context, w uintptr) {
	if err := e.p.Focus(w); err != nil {
		ctx.Log().Debug("window not focused", "err", err)
	}
}
