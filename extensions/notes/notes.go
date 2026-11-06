// Package notes is the Floating Notes extension: Markdown notes, as many as the user wants, kept
// encrypted on this computer, written in a small window that stays above the others and found
// from the root search.
package notes

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/JoaoVitorOli/althea/sdk"
)

// ID is the extension's ID.
const ID = "althea.notes"

//go:embed locales/*.toml migrations/*.sql
var files embed.FS

// Platform is what the extension needs from the host. With a nil Key there are no notes, as on a
// system without a vault.
type Platform struct {
	// Key returns the 32-byte key the notes are encrypted with, kept in the system's vault.
	Key func() ([]byte, error)
	// Show shows the floating window with the note of id.
	Show func(id string)
	// Deleted tells the window that a note was deleted from the launcher; it may be nil.
	Deleted func(id string)
}

// Note is a note.
type Note struct {
	ID               string
	Text             string // Markdown
	Created, Updated time.Time
}

// Title is a note's first line with text, without the Markdown that starts it, as "# Groceries"
// gives "Groceries".
func (n Note) Title() string {
	for line := range strings.Lines(n.Text) {
		line = strings.TrimLeftFunc(line, func(r rune) bool {
			return unicode.IsSpace(r) || r == '#' || r == '>' || r == '-' || r == '*' || r == '+'
		})
		line = strings.TrimPrefix(strings.TrimPrefix(line, "[ ] "), "[x] ")
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// Extension is Floating Notes.
type Extension struct {
	p Platform
	// Now is the clock.
	Now func() time.Time

	mu     sync.Mutex
	sealer *sealer
	// notes are the notes read so far, newest opened first; nil until read.
	notes []Note
}

// New returns the Floating Notes extension.
func New(p Platform) *Extension { return &Extension{p: p, Now: time.Now} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(files, "locales")
	migrations, _ := fs.Sub(files, "migrations")
	return sdk.Manifest{
		ID: ID, Title: "Floating Notes", Icon: sdk.Icon{Glyph: "note"},
		Messages: messages, Migrations: migrations,
	}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	if e.p.Key == nil || e.p.Show == nil {
		return nil
	}
	return []sdk.Command{
		{
			ID: "open", Title: ctx.T("open"), Icon: sdk.Icon{Glyph: "note"},
			Keywords: []string{"notes", "floating", "notas", "flutuantes", "anotações"},
			Run: func(ctx sdk.Context) error {
				id, err := e.Last(ctx)
				if err != nil {
					return err
				}
				e.show(ctx, id)
				return nil
			},
		},
		{
			ID: "new", Title: ctx.T("new"), Icon: sdk.Icon{Glyph: "note"},
			Keywords: []string{"note", "create", "nota", "criar", "nova"},
			Run: func(ctx sdk.Context) error {
				n, err := e.Create(ctx, "")
				if err != nil {
					return err
				}
				e.show(ctx, n.ID)
				return nil
			},
		},
		{
			ID: "search", Title: ctx.T("search"), Icon: sdk.Icon{Glyph: "note"},
			Keywords: []string{"notes", "find", "notas", "buscar", "procurar"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.list(ctx))
				return nil
			},
		},
	}
}

// Items implements sdk.ItemSource: each note, by its title, with the words of its text to find
// it by.
func (e *Extension) Items(ctx sdk.Context) ([]sdk.Item, error) {
	if e.p.Key == nil || e.p.Show == nil {
		return nil, nil
	}
	notes, err := e.Notes(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.Item, 0, len(notes))
	for _, n := range notes {
		if n.Title() == "" {
			continue
		}
		it := e.item(ctx, n)
		it.Keywords = words(n.Text, 40)
		it.Detail = nil
		items = append(items, it)
	}
	return items, nil
}

// show shows a note in the window, the launcher giving way to it.
func (e *Extension) show(ctx sdk.Context, id string) {
	ctx.Hide()
	e.p.Show(id)
}

// seal returns the sealer, made from the vault's key the first time.
func (e *Extension) seal() (*sealer, error) {
	if e.sealer != nil {
		return e.sealer, nil
	}
	if e.p.Key == nil {
		return nil, errors.New("this system cannot keep notes")
	}
	key, err := e.p.Key()
	if err != nil {
		return nil, fmt.Errorf("notes key: %w", err)
	}
	s, err := newSealer(key)
	if err != nil {
		return nil, err
	}
	e.sealer = s
	return s, nil
}

// Notes returns the notes, the one shown last first.
func (e *Extension) Notes(ctx sdk.Context) ([]Note, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.load(ctx); err != nil {
		return nil, err
	}
	return slices.Clone(e.notes), nil
}

// load reads the notes, once. It is called with mu held.
func (e *Extension) load(ctx sdk.Context) error {
	if e.notes != nil {
		return nil
	}
	s, err := e.seal()
	if err != nil {
		return err
	}
	rows, err := ctx.Database().Query(ctx,
		`SELECT id, body, created, updated FROM notes_items WHERE deleted = 0 ORDER BY opened DESC, created DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		var (
			n                Note
			body             []byte
			created, updated int64
		)
		if err := rows.Scan(&n.ID, &body, &created, &updated); err != nil {
			return err
		}
		text, err := s.open(body)
		if err != nil {
			ctx.Log().Warn("note not read", "id", n.ID, "err", err)
			continue
		}
		n.Text, n.Created, n.Updated = string(text), time.UnixMilli(created), time.UnixMilli(updated)
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	e.notes = notes
	return nil
}

// Get returns the note of id, and whether there is one.
func (e *Extension) Get(ctx sdk.Context, id string) (Note, bool, error) {
	notes, err := e.Notes(ctx)
	if err != nil {
		return Note{}, false, err
	}
	i := slices.IndexFunc(notes, func(n Note) bool { return n.ID == id })
	if i < 0 {
		return Note{}, false, nil
	}
	return notes[i], true, nil
}

// Last returns the note shown last, making the first note when there is none.
func (e *Extension) Last(ctx sdk.Context) (string, error) {
	notes, err := e.Notes(ctx)
	if err != nil {
		return "", err
	}
	if len(notes) > 0 {
		return notes[0].ID, nil
	}
	n, err := e.Create(ctx, "")
	return n.ID, err
}

// Create makes a note with text, shown first.
func (e *Extension) Create(ctx sdk.Context, text string) (Note, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.load(ctx); err != nil {
		return Note{}, err
	}
	now := e.Now()
	n := Note{ID: sdk.NewID(), Text: text, Created: now, Updated: now}
	if _, err := ctx.Database().Exec(ctx,
		`INSERT INTO notes_items (id, body, created, updated, opened) VALUES (?, ?, ?, ?, ?)`,
		n.ID, e.sealer.seal([]byte(text)), now.UnixMilli(), now.UnixMilli(), now.UnixMilli()); err != nil {
		return Note{}, err
	}
	e.notes = append([]Note{n}, e.notes...)
	return n, nil
}

// Save keeps a note's new text. Saving the text it has changes nothing.
func (e *Extension) Save(ctx sdk.Context, id, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.load(ctx); err != nil {
		return err
	}
	i := slices.IndexFunc(e.notes, func(n Note) bool { return n.ID == id })
	if i < 0 {
		return fmt.Errorf("no note %s", id)
	}
	if e.notes[i].Text == text {
		return nil
	}
	now := e.Now()
	if _, err := ctx.Database().Exec(ctx, `UPDATE notes_items SET body = ?, updated = ? WHERE id = ?`,
		e.sealer.seal([]byte(text)), now.UnixMilli(), id); err != nil {
		return err
	}
	e.notes[i].Text, e.notes[i].Updated = text, now
	return nil
}

// Opened puts a note first, as it shows in the window.
func (e *Extension) Opened(ctx sdk.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.load(ctx); err != nil {
		return err
	}
	i := slices.IndexFunc(e.notes, func(n Note) bool { return n.ID == id })
	if i < 0 {
		return nil
	}
	if _, err := ctx.Database().Exec(ctx, `UPDATE notes_items SET opened = ? WHERE id = ?`,
		e.Now().UnixMilli(), id); err != nil {
		return err
	}
	n := e.notes[i]
	e.notes = append([]Note{n}, slices.Delete(e.notes, i, i+1)...)
	return nil
}

// Delete deletes a note, leaving the tombstone sync needs.
func (e *Extension) Delete(ctx sdk.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.load(ctx); err != nil {
		return err
	}
	now := e.Now().UnixMilli()
	res, err := ctx.Database().Exec(ctx,
		`UPDATE notes_items SET body = NULL, deleted = 1, updated = ? WHERE id = ? AND deleted = 0`, now, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}
	e.notes = slices.DeleteFunc(e.notes, func(n Note) bool { return n.ID == id })
	return nil
}

// record is a note as sync carries it between computers.
type record struct {
	Text    string `json:"text"`
	Created int64  `json:"created"` // Unix milliseconds
}

// Records returns every note as sync carries it, by ID.
func (e *Extension) Records(ctx sdk.Context) (map[string][]byte, error) {
	notes, err := e.Notes(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(notes))
	for _, n := range notes {
		data, err := json.Marshal(record{Text: n.Text, Created: n.Created.UnixMilli()})
		if err != nil {
			return nil, err
		}
		out[n.ID] = data
	}
	return out, nil
}

// PutRecord writes a note that came from another computer, as Records gave it there, or deletes
// it for nil data.
func (e *Extension) PutRecord(ctx sdk.Context, id string, data []byte) error {
	if data == nil {
		if err := e.Delete(ctx, id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("note %s: %w", id, err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.seal()
	if err != nil {
		return err
	}
	now := e.Now().UnixMilli()
	if _, err := ctx.Database().Exec(ctx,
		`INSERT INTO notes_items (id, body, created, updated, opened) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET body = excluded.body, updated = excluded.updated, deleted = 0`,
		id, s.seal([]byte(r.Text)), r.Created, now, r.Created); err != nil {
		return err
	}
	e.notes = nil // read again, in their order
	return nil
}

// deleted deletes a note the user chose in the launcher, and tells the window.
func (e *Extension) deleted(ctx sdk.Context, id string) error {
	if err := e.Delete(ctx, id); err != nil {
		return err
	}
	if e.p.Deleted != nil {
		e.p.Deleted(id)
	}
	return nil
}

// words are the distinct words of text, lowercase, up to max, for the root search to match.
func words(text string, maxWords int) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(w)) < 3 || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) == maxWords {
			break
		}
	}
	return out
}
