package theme_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

func TestEncodeWritesWhatChangedAndLoadsBack(t *testing.T) {
	dir := t.TempDir()
	base, _, err := theme.Load("althea-dark", dir)
	if err != nil {
		t.Fatal(err)
	}
	edited := base
	edited.Name = "Café Noir"
	edited.Colors.Accent = theme.Color{R: 0xe0, G: 0x90, B: 0x40, A: 0xff}
	edited.Colors.Selection = theme.Color{R: 0xe0, G: 0x90, B: 0x40, A: 0x4d}
	edited.Material.Kind = theme.Mica
	edited.Shape.RowRadius = 12
	edited.Material.Opacity = 0.5

	data := theme.Encode(edited, base, "althea-dark")
	text := string(data)
	for _, want := range []string{
		`extends = "althea-dark"`, `name = "Café Noir"`, `accent = "#e09040"`, `selection = "#e090404d"`,
		"[material]\nkind = \"mica\"\nopacity = 0.5", "[shape]\nrow_radius = 12",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "background") || strings.Contains(text, "[font]") {
		t.Errorf("file sets what did not change:\n%s", text)
	}

	slug := theme.Slug(edited.Name)
	if err := theme.Save(dir, slug, data); err != nil {
		t.Fatal(err)
	}
	got, problems, err := theme.Load(slug, dir)
	if err != nil || len(problems) > 0 {
		t.Fatalf("Load = %v, %v", problems, err)
	}
	if got != edited {
		t.Errorf("loaded\n%+v\nwant\n%+v", got, edited)
	}
}

func TestSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Café Noir": "cafe-noir", "  Solarized -- Dark! ": "solarized-dark", "日本": "custom", "Ocean 2": "ocean-2",
	} {
		if got := theme.Slug(name); got != want {
			t.Errorf("Slug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestWriteNeverExtendsItself(t *testing.T) {
	dir := t.TempDir()
	dark, _, _ := theme.Load("althea-dark", dir)
	dark.Name = "Midnight"
	dark.Colors.Accent = theme.Color{R: 1, G: 2, B: 3, A: 255}
	slug, err := theme.Write(dir, "althea-dark", dark)
	if err != nil || slug != "midnight" {
		t.Fatalf("Write = %q, %v", slug, err)
	}

	// Edited again and saved under its own name: it still extends Althea Dark.
	again, _, _ := theme.Load("midnight", dir)
	again.Shape.RowRadius = 2
	if _, err := theme.Write(dir, "midnight", again); err != nil {
		t.Fatal(err)
	}
	got, problems, err := theme.Load("midnight", dir)
	if err != nil || len(problems) > 0 {
		t.Fatalf("Load = %v, %v", problems, err)
	}
	if got != again {
		t.Errorf("loaded %+v, want %+v", got, again)
	}

	dark.Name = "Althea Light"
	if _, err := theme.Write(dir, "althea-dark", dark); !errors.Is(err, theme.ErrBuiltinName) {
		t.Errorf("saving as a built-in: %v", err)
	}
}
