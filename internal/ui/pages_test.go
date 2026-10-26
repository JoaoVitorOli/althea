package ui

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// syncHost runs extensions' code at once, on the calling goroutine, and records their errors.
type syncHost struct {
	t      *testing.T
	r      *Root
	errs   []error
	queued []func() // updates, held while hold is set
	hold   bool
}

func (h *syncHost) Go(ext string, fn func(ctx sdk.Context) error) func() {
	ctx, cancel := context.WithCancel(context.Background())
	sc := sdktest.New(h.t, testExt{}, "en")
	sc.Context = ctx
	if err := fn(sc); err != nil {
		h.errs = append(h.errs, err)
	}
	for _, v := range sc.Pushed {
		h.r.Push(ext, v)
	}
	return cancel
}

func (h *syncHost) Update(fn func()) {
	if h.hold {
		h.queued = append(h.queued, fn)
		return
	}
	fn()
}

func (h *syncHost) flush() {
	q := h.queued
	h.queued, h.hold = nil, false
	for _, fn := range q {
		fn()
	}
}

type testExt struct{}

func (testExt) Manifest() sdk.Manifest             { return sdk.Manifest{ID: "test"} }
func (testExt) Commands(sdk.Context) []sdk.Command { return nil }

func pushing(t *testing.T) (*Root, *ui.Tester, *syncHost) {
	t.Helper()
	r := newRoot(t, "en")
	h := &syncHost{t: t, r: r}
	r.Host = h
	tt := ui.NewTester(r.View, 720, 420)
	return r, tt, h
}

func TestListFiltersItsItemsAndRunsTheChosenOne(t *testing.T) {
	r, tt, _ := pushing(t)
	var ran []string
	item := func(title, section string) sdk.Item {
		return sdk.Item{ID: title, Title: title, Section: section, Actions: []sdk.Action{
			{Title: "Paste", Run: func(sdk.Context) error { ran = append(ran, title); return nil }},
		}}
	}
	r.Push("test", &sdk.List{Title: "Fruit", Items: []sdk.Item{
		item("Apple", "Red"), item("Banana", "Yellow"), item("Cherry", "Red"),
	}})
	tt.Frame()
	for _, want := range []string{"Red", "Yellow", "Apple", "Banana", "Cherry", "Fruit"} {
		if !tt.HasText(want) {
			t.Errorf("does not show %q; texts %q", want, tt.Texts())
		}
	}
	// Sections keep their items together: Cherry comes before Banana.
	texts := tt.Texts()
	if slices.Index(texts, "Cherry") > slices.Index(texts, "Banana") {
		t.Errorf("sections not grouped; texts %q", texts)
	}

	tt.Type("ban")
	if tt.HasText("Apple") || !tt.HasText("Banana") || tt.HasText("Yellow") {
		t.Errorf("filtering by %q shows %q", "ban", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if !slices.Equal(ran, []string{"Banana"}) {
		t.Errorf("ran %q, want Banana", ran)
	}
}

func TestEscapeClearsAPagesQueryThenGoesBack(t *testing.T) {
	r, tt, _ := pushing(t)
	_ = tt.Click("Search")
	tt.Type("fr")
	r.Push("test", &sdk.List{Items: []sdk.Item{{Title: "Apple"}}})
	tt.Frame()
	tt.Type("x")
	tt.Key(0, ui.KeyEscape)
	if len(r.stack) != 1 || !tt.HasText("Apple") {
		t.Fatalf("first Escape should clear the query; stack %d, texts %q", len(r.stack), tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if len(r.stack) != 0 {
		t.Fatalf("second Escape should go back; stack %d", len(r.stack))
	}
	if r.query != "fr" {
		t.Errorf("root query = %q, want it kept as %q", r.query, "fr")
	}
}

func TestBackButtonGoesBack(t *testing.T) {
	r, tt, _ := pushing(t)
	r.Push("test", &sdk.Detail{Title: "Readme", Markdown: "# Hello"})
	tt.Frame()
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if len(r.stack) != 0 {
		t.Error("Back did not close the page")
	}
}

func TestListLoadsAndSearches(t *testing.T) {
	r, tt, h := pushing(t)
	h.hold = true
	r.Push("test", &sdk.List{Load: func(sdk.Context) ([]sdk.Item, error) {
		return []sdk.Item{{Title: "Loaded"}}, nil
	}})
	tt.Frame()
	if tt.HasText("Loaded") {
		t.Fatal("shows the items before they arrive")
	}
	h.flush()
	tt.Frame()
	if !tt.HasText("Loaded") {
		t.Errorf("does not show loaded items; texts %q", tt.Texts())
	}

	var asked []string
	r.Push("test", &sdk.List{Search: func(_ sdk.Context, q string) ([]sdk.Item, error) {
		asked = append(asked, q)
		return []sdk.Item{{Title: "Result for " + q}}, nil
	}})
	tt.Frame()
	tt.Type("g")
	tt.Type("o")
	if !slices.Equal(asked, []string{"", "g", "go"}) {
		t.Errorf("searched %q", asked)
	}
	if !tt.HasText("Result for go") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestLateSearchesAreDropped(t *testing.T) {
	r, tt, h := pushing(t)
	r.Push("test", &sdk.List{Search: func(_ sdk.Context, q string) ([]sdk.Item, error) {
		return []sdk.Item{{Title: "Result for " + q + "."}}, nil
	}})
	tt.Frame()
	h.hold = true
	tt.Type("a")
	tt.Type("b")
	// The answer for "a" arrives after "ab" was asked.
	h.flush()
	tt.Frame()
	if tt.HasText("Result for a.") || !tt.HasText("Result for ab.") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestListShowsEmptyAndErrors(t *testing.T) {
	r, tt, h := pushing(t)
	r.Push("test", &sdk.List{Empty: sdk.Empty{Title: "No snippets", Description: "Create one first."}})
	tt.Frame()
	if !tt.HasText("No snippets") || !tt.HasText("Create one first.") {
		t.Errorf("texts %q", tt.Texts())
	}
	r.Push("test", &sdk.List{Load: func(sdk.Context) ([]sdk.Item, error) { return nil, errors.New("offline") }})
	tt.Frame()
	if len(h.errs) != 1 {
		t.Errorf("errors %v, want the load's", h.errs)
	}
}

func TestGridMovesInTwoDimensions(t *testing.T) {
	r, tt, _ := pushing(t)
	var ran string
	var items []sdk.Item
	for _, e := range []string{"😀", "😁", "😂", "🤣", "😃", "😄", "😅", "😆"} {
		items = append(items, sdk.Item{ID: e, Title: "face " + e, Icon: sdk.Icon{Text: e}, Actions: []sdk.Action{
			{Title: "Paste", Run: func(sdk.Context) error { ran = e; return nil }},
		}})
	}
	r.Push("test", &sdk.Grid{Items: items, Columns: 4})
	tt.Frame()
	tt.Key(0, ui.KeyDown)  // 😃
	tt.Key(0, ui.KeyRight) // 😄
	tt.Key(0, ui.KeyEnter)
	if ran != "😄" {
		t.Errorf("ran %q, want 😄", ran)
	}
}

func TestDetailShowsMarkdownAndMetadataAndRunsActions(t *testing.T) {
	r, tt, _ := pushing(t)
	copied := 0
	r.Push("test", &sdk.Detail{
		Title: "Color", Markdown: "Some **bold** text",
		Metadata: []sdk.Metadata{{Label: "Hex", Text: "#7c3aed"}},
		Actions: []sdk.Action{
			{Title: "Copy", Run: func(sdk.Context) error { copied++; return nil }},
			{Title: "Open"},
		},
	})
	tt.Frame()
	for _, want := range []string{"Hex", "#7c3aed", "Copy", "Color"} {
		if !tt.HasText(want) {
			t.Errorf("does not show %q; texts %q", want, tt.Texts())
		}
	}
	if !strings.Contains(strings.Join(tt.Texts(), " "), "bold") {
		t.Errorf("markdown not shown; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if copied != 1 {
		t.Errorf("Enter ran the first action %d times", copied)
	}
}

func TestFormChecksThenSubmits(t *testing.T) {
	r, tt, _ := pushing(t)
	var got sdk.Values
	r.Push("test", &sdk.Form{
		Title: "New Snippet",
		Fields: []sdk.Field{
			{ID: "name", Title: "Name", Required: true},
			{
				ID: "kind", Title: "Kind", Kind: sdk.Dropdown, Default: "b",
				Options: []sdk.Option{{Value: "a", Title: "Alpha"}, {Value: "b", Title: "Beta"}},
			},
			{ID: "pin", Title: "Pinned", Kind: sdk.Checkbox, Default: "true"},
			{ID: "code", Title: "Code", Validate: func(v string) string {
				if v != "" && len(v) < 3 {
					return "At least 3 letters"
				}
				return ""
			}},
		},
		Submit: func(_ sdk.Context, v sdk.Values) error { got = v; return nil },
	})
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyEnter)
	if got != nil || !tt.HasText("Required") {
		t.Fatalf("submitted without the required field; texts %q", tt.Texts())
	}
	tt.Type("hello") // the first field has the keyboard
	if err := tt.Click("Code"); err != nil {
		t.Fatal(err)
	}
	tt.Type("ab")
	tt.Key(ui.Ctrl, ui.KeyEnter)
	if got != nil || !tt.HasText("At least 3 letters") {
		t.Fatalf("submitted an invalid value; texts %q", tt.Texts())
	}
	tt.Type("c")
	tt.Key(ui.Ctrl, ui.KeyEnter)
	want := sdk.Values{"name": "hello", "kind": "b", "pin": "true", "code": "abc"}
	if !maps.Equal(got, want) {
		t.Errorf("values %v, want %v", got, want)
	}
	if len(r.stack) != 0 {
		t.Error("the form stays open after a successful submit")
	}
}

func TestFormKeepsOpenOnErrorAndGivesItsPlaceToAPushedView(t *testing.T) {
	r, tt, h := pushing(t)
	fail := true
	r.Push("test", &sdk.Form{
		Fields: []sdk.Field{{ID: "q", Title: "Query"}},
		Submit: func(ctx sdk.Context, _ sdk.Values) error {
			if fail {
				return errors.New("server down")
			}
			ctx.Push(&sdk.Detail{Title: "Answer"})
			return nil
		},
	})
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyEnter)
	if len(r.stack) != 1 || len(h.errs) != 1 {
		t.Fatalf("stack %d, errors %v: a failed submit should keep the form", len(r.stack), h.errs)
	}
	fail = false
	tt.Key(ui.Ctrl, ui.KeyEnter)
	if len(r.stack) != 1 {
		t.Fatalf("stack %d, want the pushed view alone", len(r.stack))
	}
	if _, ok := r.stack[0].(*detailPage); !ok {
		t.Errorf("top is %T, want the detail Submit pushed", r.stack[0])
	}
}

func TestPageActionsConfirmAndShowInThePanel(t *testing.T) {
	r, tt, _ := pushing(t)
	deleted := 0
	r.Push("test", &sdk.List{Items: []sdk.Item{{ID: "a", Title: "Note", Actions: []sdk.Action{
		{Title: "Open"},
		{Title: "Delete", Shortcut: "Ctrl+D", Confirm: "Press again to delete", Run: func(sdk.Context) error { deleted++; return nil }},
	}}}})
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyD)
	if deleted != 0 || !tt.HasText("Press again to delete") {
		t.Fatalf("deleted %d on the first press; texts %q", deleted, tt.Texts())
	}
	tt.Key(ui.Ctrl, ui.KeyD)
	if deleted != 1 {
		t.Errorf("deleted %d after confirming", deleted)
	}
	tt.Key(ui.Ctrl, ui.KeyK)
	if !tt.HasText("Delete") || !tt.HasText("Ctrl") {
		t.Errorf("action panel; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if len(r.stack) != 1 {
		t.Error("Escape closed the page with the panel open")
	}
}

func TestResetClosesPages(t *testing.T) {
	r, tt, _ := pushing(t)
	r.Push("test", &sdk.Detail{Title: "One"})
	r.Push("test", &sdk.Detail{Title: "Two"})
	r.Reset()
	tt.Frame()
	if len(r.stack) != 0 || !tt.HasText("Type to search") {
		t.Errorf("stack %d; texts %q", len(r.stack), tt.Texts())
	}
}
