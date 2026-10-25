package ui

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
)

type ran struct {
	key    string
	action int
}

// searching returns a root view wired to fake search results, and what it ran.
func searching(t *testing.T) (*Root, *ui.Tester, *[]string, *[]ran) {
	t.Helper()
	r := newRoot(t, "en")
	var queries []string
	var runs []ran
	r.OnQuery = func(q string) { queries = append(queries, q) }
	r.OnRun = func(res registry.Result, action int) { runs = append(runs, ran{res.Key, action}) }
	tt := ui.NewTester(r.View, 720, 420)
	return r, tt, &queries, &runs
}

func results(titles ...string) []registry.Result {
	var rs []registry.Result
	for _, title := range titles {
		rs = append(rs, registry.Result{Key: "x/" + title, Item: sdk.Item{
			ID: title, Title: title,
			Actions: []sdk.Action{{Title: "Open"}, {Title: "Show in Folder"}, {Title: "Copy Path", Shortcut: "Ctrl+Shift+C"}},
		}})
	}
	return rs
}

func TestTypingAsksForResultsAndShowsThem(t *testing.T) {
	r, tt, queries, _ := searching(t)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("co")
	if len(*queries) == 0 || (*queries)[len(*queries)-1] != "co" {
		t.Fatalf("queries = %q, want the last to be %q", *queries, "co")
	}
	r.SetResults("co", results("Visual Studio Code", "Command Prompt"))
	tt.Frame()
	for _, want := range []string{"Visual Studio Code", "Command Prompt"} {
		if !tt.HasText(want) {
			t.Errorf("does not show %q; texts %q", want, tt.Texts())
		}
	}
}

func TestStaleResultsAreIgnored(t *testing.T) {
	r, tt, _, _ := searching(t)
	_ = tt.Click("Search")
	tt.Type("cod")
	r.SetResults("co", results("Old Result"))
	tt.Frame()
	if tt.HasText("Old Result") {
		t.Error("results for an earlier query replaced the current ones")
	}
}

func TestKeyboardChoosesAndRuns(t *testing.T) {
	r, tt, _, runs := searching(t)
	_ = tt.Click("Search")
	tt.Type("c")
	r.SetResults("c", results("One", "Two", "Three"))
	tt.Frame()

	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Ctrl, ui.KeyEnter)
	want := []ran{{"x/Two", 0}, {"x/Two", 1}}
	if !slices.Equal(*runs, want) {
		t.Errorf("runs = %v, want %v", *runs, want)
	}
}

func TestSelectionStaysInTheList(t *testing.T) {
	r, tt, _, runs := searching(t)
	_ = tt.Click("Search")
	tt.Type("c")
	r.SetResults("c", results("One", "Two"))
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	for range 5 {
		tt.Key(0, ui.KeyDown)
	}
	tt.Key(0, ui.KeyEnter)
	if len(*runs) != 1 || (*runs)[0].key != "x/Two" {
		t.Errorf("runs = %v, want the last row", *runs)
	}
}

func TestActionPanel(t *testing.T) {
	r, tt, _, runs := searching(t)
	_ = tt.Click("Search")
	tt.Type("c")
	r.SetResults("c", results("One"))
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyK)
	for _, want := range []string{"Show in Folder", "Copy Path", "Shift"} {
		if !tt.HasText(want) {
			t.Errorf("action panel does not show %q; texts %q", want, tt.Texts())
		}
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if want := []ran{{"x/One", 2}}; !slices.Equal(*runs, want) {
		t.Errorf("runs = %v, want %v", *runs, want)
	}
	if tt.HasText("Show in Folder") {
		t.Error("action panel still open after running an action")
	}

	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Show in Folder") || r.query != "c" {
		t.Errorf("Escape should close the panel alone; query %q", r.query)
	}
}

func TestActionShortcutsRunFromTheList(t *testing.T) {
	r, tt, _, runs := searching(t)
	_ = tt.Click("Search")
	tt.Type("c")
	r.SetResults("c", results("One"))
	tt.Frame()
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyC)
	if want := []ran{{"x/One", 2}}; !slices.Equal(*runs, want) {
		t.Errorf("runs = %v, want %v", *runs, want)
	}
}

func TestNoResults(t *testing.T) {
	r, tt, _, _ := searching(t)
	_ = tt.Click("Search")
	tt.Type("zzz")
	r.SetResults("zzz", nil)
	tt.Frame()
	if !tt.HasText(`No results for "zzz"`) {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestHighlight(t *testing.T) {
	spans := highlight("Visual Studio Code", []int{0, 7, 14}, ui.Color{A: 1}, ui.Color{A: 2})
	var texts []string
	for _, s := range spans {
		texts = append(texts, s.Text)
	}
	if want := []string{"V", "isual ", "S", "tudio ", "C", "ode"}; !slices.Equal(texts, want) {
		t.Errorf("spans = %q, want %q", texts, want)
	}
	if spans[0].Color.A != 2 || spans[1].Color.A != 1 {
		t.Error("matched letters are not in the accent color")
	}
}
