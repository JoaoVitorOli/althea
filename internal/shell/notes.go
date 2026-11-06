package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// NotesWindow is the floating notes' window: small, above the other windows, where the user left
// it, and hidden rather than closed.
type NotesWindow struct {
	win *mygo.Window
}

// OpenNotes creates the notes window, hidden, drawing view. It must be called on the main thread
// once the app is ready.
func OpenNotes(view func(*ui.Context), material theme.MaterialKind) *NotesWindow {
	n := &NotesWindow{}
	n.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Althea Notes",
		Hidden:            true,
		AlwaysOnTop:       true,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		Transparent:       true,
		Vibrancy:          VibrancyOf(material),
		Width:             380,
		Height:            460,
		MinWidth:          260,
		MinHeight:         180,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		StateKey:          "notes",
		Content:           ui.View(view),
	})
	n.win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		n.win.Hide()
	})
	return n
}

// Window is the notes' window, for Update and Invalidate.
func (n *NotesWindow) Window() *mygo.Window { return n.win }

// Show shows the window and gives it the keyboard.
func (n *NotesWindow) Show() {
	n.win.Show()
	n.win.Focus()
}

// Hide hides the window.
func (n *NotesWindow) Hide() { n.win.Hide() }

// SetMaterial changes the backdrop, as a theme changes.
func (n *NotesWindow) SetMaterial(kind theme.MaterialKind) { n.win.SetVibrancy(VibrancyOf(kind)) }
