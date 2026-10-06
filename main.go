package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app is the state the window shows. Its view builds the interface from
// it, on the main thread, whenever the window needs a frame: after input,
// after Window.Update, and while something animates.
type app struct {
	name  string
	count int
}

func (a *app) view(c *ui.Context) {
	c.Root().Background(ui.Hex("#1c1c1f"))

	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, "teste").TextColor(ui.Hex("#fff"))
		})
		ui.Row(c).BorderWidth(1, 0, 0, 0).BorderColor(ui.Hex("#fff")).Padding(6).Children(func() {
			ui.Text(c, "Dismiss")
		})
	})
}

func main() {
	a := &app{}

	mygo.App.WhenReady(func() {
		workArea := mygo.Screen.PrimaryDisplay().WorkArea

		const width, height = 720, 420
		x := workArea.X + (workArea.Width-width)/2
		y := workArea.Y + workArea.Height/4

		mygo.NewWindow(mygo.WindowOptions{
			Title:         "althea",
			Width:         width,
			Height:        height,
			MinWidth:      width,
			MinHeight:     height,
			X:             x,
			Y:             y,
			DisableResize: true,
			AlwaysOnTop:   true,
			Frameless:     true,
			// VibrancyNone        Vibrancy = ""
			// VibrancyWindow      Vibrancy = "window"
			// VibrancyContent     Vibrancy = "content"
			// VibrancySidebar     Vibrancy = "sidebar"
			// VibrancyHeader      Vibrancy = "header"
			// VibrancyTitlebar    Vibrancy = "titlebar"
			// VibrancyUnderWindow Vibrancy = "under-window"
			// VibrancyUnderPage   Vibrancy = "under-page"
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
