package theme

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoad reads anything as a user's theme: a theme with problems, or an error, never a panic.
func FuzzLoad(f *testing.F) {
	base, _ := builtinFS.ReadFile("builtin/althea-dark.toml")
	f.Add(base)
	for _, s := range []string{
		"schema = 1\nextends = \"althea-dark\"\n[colors]\naccent = \"#zzz\"",
		"schema = 1\nextends = \"fuzz\"", "schema = 1\n[shape]\nradius = -1e309",
		"schema = 1\nextends = \"../../etc\"", "[colors]\nbackground = [1,2]",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "fuzz.toml"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, _ = Load("fuzz", dir)
	})
}

// FuzzParseColor reads anything as a color.
func FuzzParseColor(f *testing.F) {
	for _, s := range []string{"#fff", "#ffffff", "#ffffff80", "rgba(1,2,3,0.5)", "#", "#ggg", "transparent"} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		if c, ok := ParseColor(s); ok {
			if again, ok := ParseColor(c.Hex()); !ok || again != c {
				panic("a color does not survive its hex: " + s)
			}
		}
	})
}
