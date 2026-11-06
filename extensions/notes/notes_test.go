package notes_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/notes"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

var key = bytes.Repeat([]byte{9}, 32)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) tick() { c.t = c.t.Add(time.Minute) }

type shown struct{ ids, deleted []string }

func newExt(t *testing.T) (*notes.Extension, *sdktest.Context, *clock, *shown) {
	t.Helper()
	s := &shown{}
	e := notes.New(notes.Platform{
		Key:     func() ([]byte, error) { return key, nil },
		Show:    func(id string) { s.ids = append(s.ids, id) },
		Deleted: func(id string) { s.deleted = append(s.deleted, id) },
	})
	c := &clock{time.Date(2026, 11, 6, 12, 0, 0, 0, time.Local)}
	e.Now = c.now
	return e, sdktest.New(t, e, "en"), c, s
}

func command(t *testing.T, e *notes.Extension, ctx sdk.Context, id string) sdk.Command {
	t.Helper()
	for _, c := range e.Commands(ctx) {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no command %q", id)
	return sdk.Command{}
}

func TestTitle(t *testing.T) {
	for text, want := range map[string]string{
		"":                        "",
		"\n\n  \n":                "",
		"# Groceries\n- milk":     "Groceries",
		"\n\n- [ ] call the bank": "call the bank",
		"> a quote":               "a quote",
		"plain text\nmore":        "plain text",
	} {
		if got := (notes.Note{Text: text}).Title(); got != want {
			t.Errorf("Title(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestNotesAreKeptEncrypted(t *testing.T) {
	e, ctx, clk, _ := newExt(t)
	n, err := e.Create(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	clk.tick()
	if err := e.Save(ctx, n.ID, "# Secret plan\nbuy a boat"); err != nil {
		t.Fatal(err)
	}
	var body []byte
	if err := ctx.Database().QueryRow(context.Background(), `SELECT body FROM notes_items WHERE id = ?`, n.ID).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("boat")) {
		t.Error("the note is written in the clear")
	}

	// A new run reads what the last one wrote.
	again := notes.New(notes.Platform{Key: func() ([]byte, error) { return key, nil }, Show: func(string) {}})
	got, ok, err := again.Get(ctx, n.ID)
	if err != nil || !ok {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	if got.Text != "# Secret plan\nbuy a boat" || !got.Updated.Equal(clk.t.Truncate(time.Millisecond)) {
		t.Errorf("got %+v", got)
	}
}

func TestOrderFollowsWhatShowedLast(t *testing.T) {
	e, ctx, clk, _ := newExt(t)
	a, _ := e.Create(ctx, "a")
	clk.tick()
	b, _ := e.Create(ctx, "b")
	clk.tick()
	if err := e.Opened(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	all, _ := e.Notes(ctx)
	if ids := []string{all[0].ID, all[1].ID}; !slices.Equal(ids, []string{a.ID, b.ID}) {
		t.Errorf("order = %v, want a then b", ids)
	}
	last, err := e.Last(ctx)
	if err != nil || last != a.ID {
		t.Errorf("Last = %q, %v; want a", last, err)
	}
}

func TestLastMakesTheFirstNote(t *testing.T) {
	e, ctx, _, s := newExt(t)
	if err := command(t, e, ctx, "open").Run(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := e.Notes(ctx)
	if len(all) != 1 || len(s.ids) != 1 || s.ids[0] != all[0].ID {
		t.Fatalf("notes %v, shown %v", all, s.ids)
	}
	if ctx.Hidden == 0 {
		t.Error("the launcher should give way to the window")
	}
}

func TestDeleteLeavesATombstone(t *testing.T) {
	e, ctx, _, s := newExt(t)
	n, _ := e.Create(ctx, "# Gone soon")
	if err := command(t, e, ctx, "search").Run(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := ctx.Pushed[0].(*sdk.List).Search(ctx, "")
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v, %v", items, err)
	}
	del := items[0].Actions[2]
	if del.Confirm == "" {
		t.Error("deleting should be confirmed")
	}
	if err := del.Run(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := e.Notes(ctx)
	if len(all) != 0 || !slices.Equal(s.deleted, []string{n.ID}) {
		t.Errorf("notes %v, deleted %v", all, s.deleted)
	}
	var deleted int
	var body []byte
	if err := ctx.Database().QueryRow(context.Background(), `SELECT deleted, body FROM notes_items WHERE id = ?`, n.ID).Scan(&deleted, &body); err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || body != nil {
		t.Errorf("tombstone = %d, %q", deleted, body)
	}
}

func TestSearchAndItems(t *testing.T) {
	e, ctx, _, _ := newExt(t)
	_, _ = e.Create(ctx, "# Groceries\nmilk, eggs and coffee")
	_, _ = e.Create(ctx, "# Ideas\nan app for coffee lovers")
	_, _ = e.Create(ctx, "") // untitled: in the list, not in the root search

	if err := command(t, e, ctx, "search").Run(ctx); err != nil {
		t.Fatal(err)
	}
	list := ctx.Pushed[0].(*sdk.List)
	for q, want := range map[string]int{"": 3, "coffee": 2, "EGGS": 1, "tea": 0} {
		items, err := list.Search(ctx, q)
		if err != nil || len(items) != want {
			t.Errorf("Search(%q) = %d items, %v; want %d", q, len(items), err, want)
		}
	}

	items, err := e.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("Items = %d, want 2", len(items))
	}
	if !slices.Contains(items[1].Keywords, "eggs") || items[1].Title != "Groceries" {
		t.Errorf("item = %+v", items[1])
	}
}

func TestNoVaultNoNotes(t *testing.T) {
	e := notes.New(notes.Platform{})
	ctx := sdktest.New(t, e, "en")
	if cmds := e.Commands(ctx); cmds != nil {
		t.Errorf("Commands = %v, want none", cmds)
	}
	if items, err := e.Items(ctx); items != nil || err != nil {
		t.Errorf("Items = %v, %v", items, err)
	}
}

func TestRecordsGoToAnotherComputer(t *testing.T) {
	e, ctx, _, _ := newExt(t)
	n, _ := e.Create(ctx, "# Trip\npack the tent")
	records, err := e.Records(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("Records = %v, %v", records, err)
	}

	other, octx, _, _ := newExt(t)
	if err := other.PutRecord(octx, n.ID, records[n.ID]); err != nil {
		t.Fatal(err)
	}
	got, ok, err := other.Get(octx, n.ID)
	if err != nil || !ok || got.Text != "# Trip\npack the tent" || !got.Created.Equal(n.Created.Truncate(time.Millisecond)) {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}

	// A delete there, then the note again: it comes back.
	if err := other.PutRecord(octx, n.ID, nil); err != nil {
		t.Fatal(err)
	}
	if all, _ := other.Notes(octx); len(all) != 0 {
		t.Errorf("after the delete: %v", all)
	}
	if err := other.PutRecord(octx, n.ID, nil); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
	if err := other.PutRecord(octx, n.ID, records[n.ID]); err != nil {
		t.Fatal(err)
	}
	if all, _ := other.Notes(octx); len(all) != 1 {
		t.Errorf("after coming back: %v", all)
	}
}
