package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/theme"
)

func themeEditor(t *testing.T) (*ThemeEditor, *ui.Tester, string) {
	t.Helper()
	dir := t.TempDir()
	catalog := i18n.New("en")
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	chrome, _, err := theme.Load("althea-dark", dir)
	if err != nil {
		t.Fatal(err)
	}
	e := NewThemeEditor(catalog.T, ThemeBackend{
		Names: func() []string { return theme.List(dir) },
		Load: func(name string) (theme.Theme, error) {
			th, _, err := theme.Load(name, dir)
			return th, err
		},
		Save: func(from string, th theme.Theme) (string, error) { return theme.Write(dir, from, th) },
	}, chrome)
	return e, ui.NewTester(e.View, 980, 640), dir
}

func TestThemeEditorSavesACopyOfABuiltInTheme(t *testing.T) {
	e, tt, dir := themeEditor(t)
	e.Start("althea-dark")
	tt.Frame()
	for _, want := range []string{"Theme Editor", "Accent", "Row corners", "Search for apps and commands…", "Floating Notes"} {
		if !tt.HasText(want) {
			t.Errorf("does not show %q; texts %q", want, tt.Texts())
		}
	}
	if e.name != "Althea Dark Custom" {
		t.Errorf("name %q, want a copy's", e.name)
	}
	if !tt.HasText("#7b8cf0") {
		t.Errorf("does not show the accent's value; texts %q", tt.Texts())
	}

	e.edit.Colors.Accent = theme.Color{R: 0xe0, G: 0x90, B: 0x40, A: 255}
	tt.Frame()
	if !tt.HasText("#e09040") {
		t.Errorf("does not show the new accent; texts %q", tt.Texts())
	}
	if err := tt.Click("Save and Use"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Saved as althea-dark-custom.toml and in use") {
		t.Fatalf("texts %q", tt.Texts())
	}
	got, problems, err := theme.Load("althea-dark-custom", dir)
	if err != nil || len(problems) > 0 || got.Colors.Accent != e.edit.Colors.Accent || got.Name != "Althea Dark Custom" {
		t.Errorf("saved %+v, %v, %v", got, problems, err)
	}
	if e.from != "althea-dark-custom" {
		t.Errorf("edits %q after saving, want the saved theme", e.from)
	}
}

func TestThemeEditorRefusesABuiltInName(t *testing.T) {
	e, tt, _ := themeEditor(t)
	e.Start("althea-dark")
	e.name = "Althea Light"
	tt.Key(ui.Ctrl, ui.KeyS)
	tt.Frame()
	if !tt.HasText("A built-in theme has that name: choose another") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestThemeEditorResets(t *testing.T) {
	e, tt, _ := themeEditor(t)
	e.Start("althea-light")
	e.edit.Shape.RowRadius = 20
	tt.Frame()
	if err := tt.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if e.edit != e.base {
		t.Error("Reset kept the edit")
	}
}
