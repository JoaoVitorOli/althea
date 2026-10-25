// Package shell owns the launcher window: where it opens, how it shows and hides, and the
// native chrome around it.
package shell

import (
	"log/slog"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

// Size of the launcher window, in device-independent pixels.
const (
	Width  = 720
	Height = 420
)

// Launcher is the launcher window.
type Launcher struct {
	win *mygo.Window
}

// Options configure a launcher.
type Options struct {
	View   func(*ui.Context)
	Border ui.Color
	Chrome platform.WindowChrome
	Log    *slog.Logger
}

// Open creates the launcher window on the primary display. It must be called once the app is
// ready.
func Open(o Options) *Launcher {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	x, y := Position(area.X, area.Y, area.Width, area.Height)

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:         "Althea",
		Width:         Width,
		Height:        Height,
		MinWidth:      Width,
		MinHeight:     Height,
		X:             x,
		Y:             y,
		DisableResize: true,
		AlwaysOnTop:   true,
		Frameless:     true,
		Content:       ui.View(o.View),
	})

	border := platform.Color{R: o.Border.R, G: o.Border.G, B: o.Border.B}
	mygo.RunOnMain(func() {
		if err := o.Chrome.SetBorderColor(win.NativeHandle(), border); err != nil {
			o.Log.Warn("window border color", "err", err)
		}
	})
	return &Launcher{win: win}
}

// Position returns where the launcher's top-left corner goes in a work area: centered
// horizontally, a quarter of the way down.
func Position(areaX, areaY, areaW, areaH int) (x, y int) {
	return areaX + (areaW-Width)/2, areaY + areaH/4
}
