package app

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/extensions/notes"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/ui"
	"github.com/JoaoVitorOli/althea/sdk"
)

// notesKeyName names the notes' key in the vault.
const notesKeyName = "Althea/althea.notes/key"

// saveAfter is how long typing pauses before a note is written.
const saveAfter = 400 * time.Millisecond

// floatingNotes is the notes' window and what keeps what is typed in it. Its fields are used on
// the main thread, but for pending, which mu guards.
type floatingNotes struct {
	a    *althea
	ext  *notes.Extension
	win  *shell.NotesWindow
	view *ui.NotesWindow

	mu      sync.Mutex
	pending map[string]string // text typed, by note, not written yet
	timer   *time.Timer
}

// notesPlatform keeps the notes' key in the vault and shows them in their window; nothing where
// the system has no vault.
func (a *althea) notesPlatform() notes.Platform {
	vault := a.services.Vault
	if vault == nil {
		return notes.Platform{}
	}
	return notes.Platform{
		Key: func() ([]byte, error) { return vaultKey(vault, notesKeyName) },
		Show: func(id string) {
			mygo.RunOnMain(func() { a.notes.show(id) })
		},
		Deleted: func(string) {
			mygo.RunOnMain(func() {
				if a.notes.view != nil {
					a.notes.view.Changed()
					a.notes.win.Window().Invalidate()
				}
			})
		},
	}
}

// setUpNotes finds the notes extension among the built-in ones.
func (a *althea) setUpNotes(exts []sdk.Extension) {
	a.notes = &floatingNotes{a: a, pending: map[string]string{}}
	for _, e := range exts {
		if n, ok := e.(*notes.Extension); ok {
			a.notes.ext = n
		}
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.notes.flush() })
}

func (f *floatingNotes) ctx() sdk.Context {
	return f.a.extensionContext(context.Background(), notes.ID)
}

// show shows the note of id in the window, creating the window the first time.
func (f *floatingNotes) show(id string) {
	if f.ext == nil {
		return
	}
	if f.win == nil {
		th := f.a.root.Theme()
		f.view = ui.NewNotesWindow(f.a.catalog.T, ui.NotesBackend{
			List:   f.list,
			Edit:   f.edit,
			Create: f.create,
			Delete: f.delete,
			Opened: func(id string) {
				if err := f.ext.Opened(f.ctx(), id); err != nil {
					f.a.log.Warn("note not put first", "err", err)
				}
			},
			Close: func() {
				f.flush()
				f.win.Hide()
			},
		}, th)
		f.win = shell.OpenNotes(f.view.View, th.Material.Kind)
	}
	f.view.Show(id)
	f.win.Show()
}

// setTheme draws the window with a new theme.
func (f *floatingNotes) setTheme() {
	if f.win == nil {
		return
	}
	th := f.a.root.Theme()
	f.view.SetTheme(th)
	f.win.SetMaterial(th.Material.Kind)
	f.win.Window().Invalidate()
}

func (f *floatingNotes) list() []ui.Note {
	f.flush()
	all, err := f.ext.Notes(f.ctx())
	if err != nil {
		f.a.log.Error("notes not read", "err", err)
		return nil
	}
	out := make([]ui.Note, len(all))
	for i, n := range all {
		out[i] = ui.Note{ID: n.ID, Title: n.Title(), Text: n.Text}
	}
	return out
}

func (f *floatingNotes) create() ui.Note {
	f.flush()
	n, err := f.ext.Create(f.ctx(), "")
	if err != nil {
		f.a.log.Error("note not created", "err", err)
		return ui.Note{}
	}
	return ui.Note{ID: n.ID}
}

func (f *floatingNotes) delete(id string) {
	f.flush()
	if err := f.ext.Delete(f.ctx(), id); err != nil {
		f.a.log.Error("note not deleted", "err", err)
	}
	f.refreshSearch()
}

// edit keeps the text typed for a note, to write once typing pauses.
func (f *floatingNotes) edit(id, text string) {
	if id == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending[id] = text
	if f.timer == nil {
		f.timer = time.AfterFunc(saveAfter, f.flush)
	} else {
		f.timer.Reset(saveAfter)
	}
}

// flush writes the text typed and not written yet.
func (f *floatingNotes) flush() {
	f.mu.Lock()
	pending := f.pending
	f.pending = map[string]string{}
	if f.timer != nil {
		f.timer.Stop()
	}
	f.mu.Unlock()
	if len(pending) == 0 || f.ext == nil {
		return
	}
	for id, text := range pending {
		if err := f.ext.Save(f.ctx(), id, text); err != nil {
			f.a.log.Error("note not saved", "err", err)
		}
	}
	f.refreshSearch()
}

// refreshSearch lets the root search find notes by what they say now.
func (f *floatingNotes) refreshSearch() {
	if s := f.a.search; s != nil {
		go s.refresh()
	}
}
