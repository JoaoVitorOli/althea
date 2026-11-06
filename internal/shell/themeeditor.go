package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// ThemeEditorWindow is the theme editor's window: an ordinary one, where the user left it, hidden
// rather than closed.
type ThemeEditorWindow struct {
	win *mygo.Window
}

// OpenThemeEditor creates the theme editor's window, hidden, drawing view. It must be called on
// the main thread once the app is ready.
func OpenThemeEditor(title string, view func(*ui.Context)) *ThemeEditorWindow {
	e := &ThemeEditorWindow{}
	e.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     title,
		Hidden:    true,
		Width:     980,
		Height:    640,
		MinWidth:  760,
		MinHeight: 480,
		StateKey:  "theme-editor",
		Content:   ui.View(view),
	})
	e.win.OnClose(func(ev *mygo.CloseEvent) {
		ev.PreventDefault()
		e.win.Hide()
	})
	return e
}

// Window is the editor's window, for Update and Invalidate.
func (e *ThemeEditorWindow) Window() *mygo.Window { return e.win }

// Show shows the window and gives it the keyboard.
func (e *ThemeEditorWindow) Show() {
	e.win.Show()
	e.win.Focus()
}
