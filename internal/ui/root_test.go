package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/theme"
)

func newRoot(t *testing.T, lang string) *Root {
	t.Helper()
	catalog := i18n.New(lang)
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	th, _, err := theme.Load("althea-dark", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewRoot(catalog.T, th)
}

func TestRootShowsSearchAndHintsInTheLanguage(t *testing.T) {
	for lang, want := range map[string][]string{
		"en":    {"Type to search", "Esc to close", "Alt+Space to show or hide"},
		"pt-BR": {"Digite para buscar", "Esc para fechar", "Alt+Space para mostrar ou esconder"},
	} {
		r := newRoot(t, lang)
		r.Hotkey = "Alt+Space"
		tt := ui.NewTester(r.View, 720, 420)
		for _, text := range want {
			if !tt.HasText(text) {
				t.Errorf("%s: root view does not show %q; texts %q", lang, text, tt.Texts())
			}
		}
	}
}

func TestEscapeClearsTheSearchFirst(t *testing.T) {
	r := newRoot(t, "en")
	tt := ui.NewTester(r.View, 720, 420)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("code")
	if r.query != "code" {
		t.Fatalf("query = %q after typing", r.query)
	}
	tt.Key(0, ui.KeyEscape)
	if r.query != "" {
		t.Errorf("query = %q after Escape, want it cleared", r.query)
	}
}

func TestResetEmptiesTheSearch(t *testing.T) {
	r := newRoot(t, "en")
	tt := ui.NewTester(r.View, 720, 420)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("notes")
	r.Reset()
	tt.Frame()
	if r.query != "" || !tt.Focused("Search") {
		t.Errorf("after Reset: query %q, search focused %v", r.query, tt.Focused("Search"))
	}
}
