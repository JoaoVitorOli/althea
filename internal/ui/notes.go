package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// Note is a note as the floating window shows it.
type Note struct {
	ID, Title, Text string
}

// NotesBackend is where the floating window's notes are kept. Its functions run on the main
// thread, so they keep what they write for later rather than wait on the disk.
type NotesBackend struct {
	// List returns the notes, the one shown last first.
	List func() []Note
	// Edit keeps a note's new text.
	Edit func(id, text string)
	// Create makes an empty note.
	Create func() Note
	// Delete deletes a note.
	Delete func(id string)
	// Opened tells that a note shows, which puts it first.
	Opened func(id string)
	// Close hides the window.
	Close func()
}

// NotesWindow is the floating notes window: one note at a time, written in Markdown or shown
// rendered, and a picker to go to another.
type NotesWindow struct {
	T       func(key string, args ...any) string
	Backend NotesBackend

	theme theme.Theme
	ui    *ui.Theme

	note    Note
	text    string
	preview bool
	editor  ui.Handle
	md      markdown.Options
	// deleting is armed by a first press of delete, which a second confirms.
	deleting bool

	picker struct {
		open     bool
		query    string
		input    ui.Handle
		selected int
		notes    []Note
	}
}

// NewNotesWindow returns the window's view, drawn with th.
func NewNotesWindow(t func(string, ...any) string, b NotesBackend, th theme.Theme) *NotesWindow {
	w := &NotesWindow{T: t, Backend: b}
	w.SetTheme(th)
	return w
}

// SetTheme changes the theme the window draws with.
func (w *NotesWindow) SetTheme(th theme.Theme) {
	w.theme, w.ui = th, ToUI(th)
	w.md.Theme = &markdown.Theme{FontSize: th.Font.Size}
}

// Show makes the window show the note of id, or the one shown last for "".
func (w *NotesWindow) Show(id string) {
	notes := w.Backend.List()
	for _, n := range notes {
		if n.ID == id || (id == "" && w.note.ID == "") {
			w.open(n)
			return
		}
	}
	if w.note.ID == "" || id != "" {
		w.open(w.Backend.Create())
	}
}

// Shown is the note that shows, "" before one does.
func (w *NotesWindow) Shown() string { return w.note.ID }

// Changed tells the window that a note changed elsewhere, as from the launcher; a deleted note
// gives way to the next.
func (w *NotesWindow) Changed() {
	if w.note.ID == "" {
		return
	}
	for _, n := range w.Backend.List() {
		if n.ID == w.note.ID {
			return
		}
	}
	w.note = Note{}
	w.Show("")
}

func (w *NotesWindow) open(n Note) {
	w.note, w.text = n, n.Text
	w.preview, w.deleting = false, false
	w.picker.open = false
	w.Backend.Opened(n.ID)
	w.editor.Focus()
}

// title is the shown note's first line, as the text says it now.
func (w *NotesWindow) title() string {
	t := Note{Text: w.text}.firstLine()
	if t == "" {
		return w.T("notes.untitled")
	}
	return t
}

// firstLine is the first line with text, without the Markdown that starts it.
func (n Note) firstLine() string {
	for line := range strings.Lines(n.Text) {
		line = strings.TrimLeft(line, " \t#>-*+")
		line = strings.TrimPrefix(strings.TrimPrefix(line, "[ ] "), "[x] ")
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// View builds the window for one frame.
func (w *NotesWindow) View(c *ui.Context) {
	c.SetTheme(w.ui)
	t := c.Theme()
	bg := t.Background
	if c.Vibrancy() {
		bg = bg.Alpha(w.theme.Material.Opacity * float32(bg.A) / 255)
	} else {
		bg.A = 255
	}
	c.Root().Background(bg)
	w.keys(c)

	ui.Column(c).Fill().Children(func() {
		w.header(c)
		ui.Divider(c)
		switch {
		case w.picker.open:
			w.pickerView(c)
		case w.preview:
			ui.Scroll(c).Grow(1).Padding(12, 16).Children(func() {
				if strings.TrimSpace(w.text) == "" {
					ui.Text(c, w.T("notes.placeholder")).TextColor(t.TextMuted)
					return
				}
				markdown.View(c, w.text, &w.md)
			})
		default:
			area := ui.TextArea(c, &w.text).Bind(&w.editor).Grow(1).Padding(12, 16).
				Placeholder(w.T("notes.placeholder")).Label(w.title())
			if area.Changed() {
				w.deleting = false
				w.Backend.Edit(w.note.ID, w.text)
			}
		}
	})
}

func (w *NotesWindow) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(40).Padding(0, 6, 0, 12).Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Icon(c, glyphs["note"]).FontSize(15).TextColor(t.TextMuted)
		ui.Box(c).Width(6)
		ui.Text(c, w.title()).FontWeight(600).TextColor(t.Text).SingleLine().Grow(1)
		if w.iconButton(c, "search", w.T("notes.notes"), w.picker.open) {
			w.togglePicker()
		}
		if w.iconButton(c, "plus", w.T("notes.new"), false) {
			w.open(w.Backend.Create())
		}
		if w.preview {
			if w.iconButton(c, "pencil", w.T("notes.edit"), false) {
				w.togglePreview()
			}
		} else if w.iconButton(c, "eye", w.T("notes.preview"), false) {
			w.togglePreview()
		}
		label := w.T("notes.delete")
		if w.deleting {
			label = w.T("notes.delete_confirm")
		}
		if w.iconButton(c, "trash", label, w.deleting) {
			w.delete()
		}
		if w.iconButton(c, "close", w.T("notes.close"), false) {
			w.close()
		}
	})
}

// iconButton is a small glyph button of the header; on draws it as pressed in, or as armed for
// the delete button.
func (w *NotesWindow) iconButton(c *ui.Context, glyph, label string, on bool) bool {
	t := c.Theme()
	m := MotionOf(w.theme)
	b := ui.Row(c).Size(28, 28).Center().Radius(8).Label(label).Tooltip(label)
	bg := t.SurfaceHover.Alpha(0)
	switch {
	case on:
		bg = t.SurfacePressed
	case b.Hovered():
		bg = t.SurfaceHover
	}
	color := t.TextMuted
	if on && glyph == "trash" {
		color = w.ui.Danger
	}
	b.Background(m.color(b, "bg", bg, m.Hover()))
	m.pose(b, pressed(b.Pressed()), m.Press())
	b.Children(func() { ui.Icon(c, glyphs[glyph]).FontSize(15).TextColor(color) })
	return b.Clicked()
}

// keys are the window's shortcuts; each button has one.
func (w *NotesWindow) keys(c *ui.Context) {
	switch {
	case c.Shortcut(0, ui.KeyEscape):
		switch {
		case w.picker.open:
			w.togglePicker()
		case w.deleting:
			w.deleting = false
		default:
			w.close()
		}
	case c.Shortcut(ui.Ctrl, ui.KeyN):
		w.open(w.Backend.Create())
	case c.Shortcut(ui.Ctrl, ui.KeyE):
		w.togglePreview()
	case c.Shortcut(ui.Ctrl, ui.KeyP):
		w.togglePicker()
	case c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyD):
		w.delete()
	}
	if !w.picker.open {
		return
	}
	found := w.found()
	switch {
	case c.Shortcut(0, ui.KeyDown):
		w.picker.selected = min(w.picker.selected+1, len(found)-1)
	case c.Shortcut(0, ui.KeyUp):
		w.picker.selected = max(w.picker.selected-1, 0)
	}
}

func (w *NotesWindow) togglePreview() {
	w.preview = !w.preview
	w.picker.open = false
	if !w.preview {
		w.editor.Focus()
	}
}

func (w *NotesWindow) togglePicker() {
	w.picker.open = !w.picker.open
	if !w.picker.open {
		w.editor.Focus()
		return
	}
	w.picker.query, w.picker.selected = "", 0
	w.picker.notes = w.Backend.List()
	w.picker.input.Focus()
}

func (w *NotesWindow) delete() {
	if !w.deleting {
		w.deleting = true
		return
	}
	w.Backend.Delete(w.note.ID)
	w.note = Note{}
	w.Show("")
}

func (w *NotesWindow) close() {
	w.picker.open, w.deleting = false, false
	w.Backend.Close()
}

// found are the picker's notes that hold what is typed, in their order.
func (w *NotesWindow) found() []Note {
	q := strings.ToLower(strings.TrimSpace(w.picker.query))
	if q == "" {
		return w.picker.notes
	}
	var out []Note
	for _, n := range w.picker.notes {
		if strings.Contains(strings.ToLower(n.Text), q) {
			out = append(out, n)
		}
	}
	return out
}

func (w *NotesWindow) pickerView(c *ui.Context) {
	t := c.Theme()
	found := w.found()
	w.picker.selected = max(0, min(w.picker.selected, len(found)-1))
	ui.Row(c).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, searchIcon).FontSize(15).TextColor(t.TextMuted)
		field := ui.TextInputBase(c, &w.picker.query).Bind(&w.picker.input).Grow(1).
			Placeholder(w.T("notes.find")).Label(w.T("notes.find"))
		if field.Changed() {
			w.picker.selected = 0
		}
		if field.Submitted() && len(found) > 0 {
			w.open(found[w.picker.selected])
		}
	})
	ui.Divider(c)
	ui.Scroll(c).Grow(1).Padding(6).Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			if len(found) == 0 {
				ui.Text(c, w.T("notes.none")).TextColor(t.TextMuted).Padding(10)
			}
			for i, n := range found {
				title := n.firstLine()
				if title == "" {
					title = w.T("notes.untitled")
				}
				row := ui.Row(c).Key(n.ID).Height(w.theme.Space.RowHeight-8).Padding(0, 10).Gap(8).
					AlignItems(ui.Center).Radius(w.theme.Shape.RowRadius).Label(title)
				switch {
				case i == w.picker.selected:
					row.Background(color(w.theme.Colors.RowSelected))
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Icon(c, glyphs["note"]).FontSize(14).TextColor(t.TextMuted)
					ui.Text(c, title).TextColor(t.Text).SingleLine().Grow(1)
					if n.ID == w.note.ID {
						ui.Icon(c, glyphs["check"]).FontSize(14).TextColor(t.TextMuted)
					}
				})
				if row.Clicked() {
					w.open(n)
				}
			}
		})
	})
}
