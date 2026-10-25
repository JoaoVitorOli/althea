package theme_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

func userTheme(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// zeroFields lists the tokens a theme leaves unset, which a complete theme has none of.
func zeroFields(v reflect.Value, path string) []string {
	var zero []string
	switch {
	case v.Type() == reflect.TypeFor[theme.Color]():
		if v.IsZero() {
			zero = append(zero, path)
		}
	case v.Kind() == reflect.Struct:
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.Name == "Family" { // "" is the system font
				continue
			}
			zero = append(zero, zeroFields(v.Field(i), path+"."+f.Name)...)
		}
	default:
		if v.IsZero() {
			zero = append(zero, path)
		}
	}
	return zero
}

func TestBuiltinThemesAreComplete(t *testing.T) {
	for _, name := range theme.Builtin() {
		th, problems, err := theme.Load(name, t.TempDir())
		if err != nil || len(problems) != 0 {
			t.Fatalf("%s: problems %v, err %v", name, problems, err)
		}
		if zero := zeroFields(reflect.ValueOf(th), name); len(zero) != 0 {
			t.Errorf("%s leaves tokens unset: %v", name, zero)
		}
	}
	if !slices.Contains(theme.Builtin(), "althea-dark") || !slices.Contains(theme.Builtin(), "althea-light") {
		t.Errorf("Builtin = %v, want althea-dark and althea-light", theme.Builtin())
	}
}

func TestUserThemeExtendsAndOverridesOnlyWhatItSets(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "midnight", `
schema = 1
name = "Midnight"
extends = "althea-dark"

[colors]
accent = "#ff6b9d"

[material]
kind = "glass"

[motion]
scale = 0.5
`)
	base, _, _ := theme.Load("althea-dark", dir)
	got, problems, err := theme.Load("midnight", dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("problems %v, err %v", problems, err)
	}

	want := base
	want.Name = "Midnight"
	want.Colors.Accent = theme.Color{R: 0xff, G: 0x6b, B: 0x9d, A: 0xff}
	want.Material.Kind = theme.Glass
	want.Motion.Scale = 0.5
	if got != want {
		t.Errorf("midnight =\n%+v\nwant\n%+v", got, want)
	}
}

func TestUserThemeShadowsABuiltinOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "althea-dark", `extends = "althea-light"
name = "My Dark"`)
	got, _, err := theme.Load("althea-dark", dir)
	if err != nil {
		t.Fatal(err)
	}
	light, _, _ := theme.Load("althea-light", t.TempDir())
	if got.Name != "My Dark" || got.Colors != light.Colors {
		t.Errorf("user althea-dark = %+v, want althea-light's colors named My Dark", got)
	}
}

func TestBuiltinAppearances(t *testing.T) {
	for name, want := range map[string]theme.Appearance{"althea-dark": theme.Dark, "althea-light": theme.Light} {
		if th, _, _ := theme.Load(name, t.TempDir()); th.Appearance != want {
			t.Errorf("%s appearance = %q, want %q", name, th.Appearance, want)
		}
	}
}

func TestThemeWithoutExtendsStartsFromAltheaDark(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "tiny", `name = "Tiny"
[shape]
window_radius = 4`)
	got, problems, err := theme.Load("tiny", dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("problems %v, err %v", problems, err)
	}
	base, _, _ := theme.Load("althea-dark", t.TempDir())
	if got.Shape.WindowRadius != 4 || got.Colors != base.Colors {
		t.Errorf("tiny = %+v", got)
	}
}

func TestThemeProblemsPointAtTheLineAndKeepTheBase(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "broken", `extends = "althea-dark"
[colors]
accent = "pink"
backgrond = "#000000"

[material]
kind = "frosted"
opacity = 1.5
`)
	base, _, _ := theme.Load("althea-dark", dir)
	got, problems, err := theme.Load("broken", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Colors.Accent != base.Colors.Accent || got.Material != base.Material {
		t.Errorf("invalid values were applied: %+v", got)
	}
	want := []theme.Problem{
		{File: "broken.toml", Line: 3, Key: "colors.accent", Message: `must be a color such as "#7b8cf0" or "#7b8cf080"`},
		{File: "broken.toml", Line: 4, Key: "colors.backgrond", Message: "unknown token"},
		{File: "broken.toml", Line: 7, Key: "material.kind", Message: `must be "acrylic", "mica", "glass" or "solid"`},
		{File: "broken.toml", Line: 8, Key: "material.opacity", Message: "must be between 0 and 1"},
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems =\n%v\nwant\n%v", problems, want)
	}
}

func TestLoadFailsOnMissingThemesCyclesAndSyntax(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "a", `extends = "b"`)
	userTheme(t, dir, "b", `extends = "a"`)
	userTheme(t, dir, "bad", "[colors\n")

	for name, want := range map[string]string{
		"nowhere": "not found",
		"a":       "extends itself",
		"bad":     "bad.toml",
	} {
		if _, _, err := theme.Load(name, dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%q) = %v, want an error containing %q", name, err, want)
		}
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	userTheme(t, dir, "midnight", `name = "Midnight"`)
	userTheme(t, dir, "althea-dark", `name = "Shadowed"`)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := theme.List(dir)
	if want := []string{"althea-dark", "althea-light", "midnight"}; !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}
