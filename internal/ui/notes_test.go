package ui

import (
	"slices"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/theme"
)

// fakeNotes keeps notes in memory, the one shown last first, as the extension does.
type fakeNotes struct {
	notes  []Note
	edits  map[string]string
	closed int
	made   int
}

func (f *fakeNotes) backend() NotesBackend {
	return NotesBackend{
		List: func() []Note {
			out := slices.Clone(f.notes)
			for i, n := range out {
				if text, ok := f.edits[n.ID]; ok {
					out[i].Text = text
				}
			}
			return out
		},
		Edit: func(id, text string) { f.edits[id] = text },
		Create: func() Note {
			f.made++
			n := Note{ID: "new" + strconv.Itoa(f.made)}
			f.notes = append([]Note{n}, f.notes...)
			return n
		},
		Delete: func(id string) {
			f.notes = slices.DeleteFunc(f.notes, func(n Note) bool { return n.ID == id })
		},
		Opened: func(id string) {
			i := slices.IndexFunc(f.notes, func(n Note) bool { return n.ID == id })
			if i > 0 {
				n := f.notes[i]
				f.notes = append([]Note{n}, slices.Delete(f.notes, i, i+1)...)
			}
		},
		Close: func() { f.closed++ },
	}
}

func notesWindow(t *testing.T, f *fakeNotes) (*NotesWindow, *ui.Tester) {
	t.Helper()
	catalog := i18n.New("en")
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	th, _, err := theme.Load("althea-dark", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.edits = map[string]string{}
	w := NewNotesWindow(catalog.T, f.backend(), th)
	return w, ui.NewTester(w.View, 380, 460)
}

func TestNotesWindowWritesTheNoteShownLast(t *testing.T) {
	f := &fakeNotes{notes: []Note{{ID: "a", Text: "# Groceries\nmilk"}, {ID: "b", Text: "# Ideas"}}}
	w, tt := notesWindow(t, f)
	w.Show("")
	tt.Frame()
	if w.Shown() != "a" || !tt.HasText("Groceries") {
		t.Fatalf("shown %q; texts %q", w.Shown(), tt.Texts())
	}
	tt.Key(ui.Ctrl, ui.KeyEnd)
	tt.Type(", eggs")
	tt.Frame()
	if got := f.edits["a"]; got != "# Groceries\nmilk, eggs" {
		t.Errorf("edit = %q", got)
	}
}

func TestNotesWindowShowsANoteAskedFor(t *testing.T) {
	f := &fakeNotes{notes: []Note{{ID: "a", Text: "first"}, {ID: "b", Text: "# Second"}}}
	w, tt := notesWindow(t, f)
	w.Show("b")
	tt.Frame()
	if w.Shown() != "b" || f.notes[0].ID != "b" || !tt.HasText("Second") {
		t.Fatalf("shown %q, order %v; texts %q", w.Shown(), f.notes, tt.Texts())
	}
}

func TestNotesWindowKeys(t *testing.T) {
	f := &fakeNotes{notes: []Note{{ID: "a", Text: "# Groceries\n**milk**"}, {ID: "b", Text: "# Ideas\ncoffee app"}}}
	w, tt := notesWindow(t, f)
	w.Show("a")
	tt.Frame()

	// Ctrl+E shows it rendered: the asterisks go.
	tt.Key(ui.Ctrl, ui.KeyE)
	tt.Frame()
	if !tt.HasText("milk") || tt.HasText("**milk**") {
		t.Errorf("preview texts %q", tt.Texts())
	}
	tt.Key(ui.Ctrl, ui.KeyE)
	tt.Frame()

	// Ctrl+P finds another note by what it says; Enter goes to it.
	tt.Key(ui.Ctrl, ui.KeyP)
	tt.Frame()
	tt.Type("coffee")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if w.Shown() != "b" {
		t.Fatalf("shown %q after the picker", w.Shown())
	}

	// Ctrl+N starts a note.
	tt.Key(ui.Ctrl, ui.KeyN)
	tt.Frame()
	if w.Shown() != "new1" || !tt.HasText("Untitled note") {
		t.Errorf("shown %q; texts %q", w.Shown(), tt.Texts())
	}

	// Deleting asks twice, then shows the next note.
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyD)
	tt.Frame()
	if len(f.notes) != 3 {
		t.Fatal("deleted on the first press")
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyD)
	tt.Frame()
	if len(f.notes) != 2 || w.Shown() != "b" {
		t.Errorf("notes %v, shown %q", f.notes, w.Shown())
	}

	// Escape closes the picker first, then the window.
	tt.Key(ui.Ctrl, ui.KeyP)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if f.closed != 0 {
		t.Error("Escape closed the window with the picker open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if f.closed != 1 {
		t.Error("Escape did not close the window")
	}
}

func TestNotesWindowFollowsADeleteElsewhere(t *testing.T) {
	f := &fakeNotes{notes: []Note{{ID: "a", Text: "one"}, {ID: "b", Text: "two"}}}
	w, tt := notesWindow(t, f)
	w.Show("a")
	tt.Frame()
	f.notes = f.notes[1:]
	w.Changed()
	tt.Frame()
	if w.Shown() != "b" {
		t.Errorf("shown %q, want b", w.Shown())
	}
}
