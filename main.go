package main

import (
	"althea/theme"
	"log"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	dwmwaBorderColor = 34
	dwmwaColorNone   = 0xFFFFFFFE
)

var dwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

func setBorderColor(w *mygo.Window, c ui.Color) {
	colorref := uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16
	mygo.RunOnMain(func() {
		dwmSetWindowAttribute.Call(w.NativeHandle(), dwmwaBorderColor,
			uintptr(unsafe.Pointer(&colorref)), unsafe.Sizeof(colorref))
	})
}

// app is the state the window shows. Its view builds the interface from
// it, on the main thread, whenever the window needs a frame: after input,
// after Window.Update, and while something animates.
type app struct {
	name  string
	count int
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(theme.Althea)
	t := c.Theme()
	c.Root().BorderColor(t.Border)
	c.Root().Background(t.Background)

	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).Padding(12).Children(func() {
			ui.Text(c, "teste").TextColor(ui.Hex("#fff"))
		})
		ui.Row(c).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Padding(6).Children(func() {
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

		win := mygo.NewWindow(mygo.WindowOptions{
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
			Content:       ui.View(a.view),
		})

		setBorderColor(win, theme.Althea.Border)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
