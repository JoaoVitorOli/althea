// Package shell owns the launcher window: a panel that floats over every app, shows on the
// display with the pointer, hides as the user goes elsewhere, and remembers where the user was
// for a while.
package shell

import (
	"log/slog"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// Size of the launcher window, in device-independent pixels.
const (
	Width  = 720
	Height = 420
)

// Options configure a launcher.
type Options struct {
	View     func(*ui.Context)
	Material theme.MaterialKind
	// ResetAfter is how long the launcher may stay hidden and still show where the user was.
	ResetAfter time.Duration
	// OnShow runs as the launcher shows, with reset set when the view should start over.
	OnShow func(reset bool)
	Log    *slog.Logger
}

// Launcher is the launcher window.
type Launcher struct {
	win      *mygo.Window
	opts     Options
	hiddenAt time.Time
}

// Open creates the launcher, hidden. It must be called once the app is ready.
func Open(o Options) *Launcher {
	l := &Launcher{opts: o}
	l.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         "Althea",
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		Vibrancy:      VibrancyOf(o.Material),
		Width:         Width,
		Height:        Height,
		DisableResize: true,
		Content:       ui.View(o.View),
	})
	// Hide rather than close, on Escape and Alt+F4, and when the user goes elsewhere: the
	// launcher shows again at once, as it was. Quitting destroys it.
	l.win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		l.Hide()
	})
	l.win.OnBlur(l.Hide)
	return l
}

// Window is the launcher's window, for Update and Invalidate.
func (l *Launcher) Window() *mygo.Window { return l.win }

// Show shows the launcher on the display with the pointer.
func (l *Launcher) Show() {
	start := time.Now()
	defer func() { l.opts.Log.Info("shown", "took", time.Since(start)) }()
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, _ := l.win.Size()
	l.win.SetPosition(Position(d.WorkArea, w))
	if l.opts.OnShow != nil {
		l.opts.OnShow(ShouldReset(l.hiddenAt, time.Now(), l.opts.ResetAfter))
	}
	l.win.Show()
	l.win.Focus()
}

// Hide hides the launcher, remembering when.
func (l *Launcher) Hide() {
	if !l.win.IsVisible() {
		return
	}
	l.hiddenAt = time.Now()
	l.win.Hide()
}

// Toggle shows the launcher, or hides it if it shows.
func (l *Launcher) Toggle() {
	if l.win.IsVisible() {
		l.Hide()
		return
	}
	l.Show()
}

// SetMaterial changes the backdrop, as a theme changes.
func (l *Launcher) SetMaterial(kind theme.MaterialKind) {
	l.opts.Material = kind
	l.win.SetVibrancy(VibrancyOf(kind))
}

// SetResetAfter changes how long the launcher remembers where the user was.
func (l *Launcher) SetResetAfter(d time.Duration) { l.opts.ResetAfter = d }

// Position returns where the launcher's top-left corner goes in a display's work area:
// centered horizontally, a quarter of the way down.
func Position(area mygo.Rectangle, w int) (x, y int) {
	return area.X + (area.Width-w)/2, area.Y + area.Height/4
}

// ShouldReset reports whether the launcher, hidden at hiddenAt, should start over when it
// shows at now: on its first show, and once it was hidden for at least after.
func ShouldReset(hiddenAt, now time.Time, after time.Duration) bool {
	return after <= 0 || hiddenAt.IsZero() || now.Sub(hiddenAt) >= after
}

// VibrancyOf returns the system material behind a theme's material. Glass is drawn by Althea
// over the system's blur.
func VibrancyOf(kind theme.MaterialKind) mygo.Vibrancy {
	switch kind {
	case theme.Acrylic, theme.Glass:
		return mygo.VibrancyAcrylic
	case theme.Mica:
		return mygo.VibrancyMica
	default:
		return mygo.VibrancyNone
	}
}
