package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetReplacesAValueAndKeepsComments(t *testing.T) {
	path := write(t, `# my settings
[hotkey]
# what I press
toggle = "Alt+Space"   # muscle memory

[general]
start_at_login = true
`)
	if err := config.Set(path, "hotkey.toggle", "Win"); err != nil {
		t.Fatal(err)
	}
	if err := config.Set(path, "general.start_at_login", false); err != nil {
		t.Fatal(err)
	}
	want := `# my settings
[hotkey]
# what I press
toggle = "Win"   # muscle memory

[general]
start_at_login = false
`
	if got := read(t, path); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestSetAddsAMissingKeyToItsTable(t *testing.T) {
	path := write(t, "[general]\nlanguage = \"en\"\n\n[window]\nreset_after = \"0s\"\n")
	if err := config.Set(path, "general.check_updates", true); err != nil {
		t.Fatal(err)
	}
	want := "[general]\nlanguage = \"en\"\ncheck_updates = true\n\n[window]\nreset_after = \"0s\"\n"
	if got := read(t, path); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestSetAddsAMissingTable(t *testing.T) {
	path := write(t, "[general]\nlanguage = \"en\"\n")
	if err := config.Set(path, "theme.name", "midnight"); err != nil {
		t.Fatal(err)
	}
	s, problems, err := config.Load(path)
	if err != nil || len(problems) != 0 || s.Theme.Name != "midnight" || s.General.Language != "en" {
		t.Errorf("after Set: %+v, %v, %v", s, problems, err)
	}
	if !strings.HasSuffix(read(t, path), "\n[theme]\nname = \"midnight\"\n") {
		t.Errorf("file = %q", read(t, path))
	}
}

func TestSetCreatesTheDefaultFileFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := config.Set(path, "hotkey.toggle", `Ctrl+"Space"`); err != nil {
		t.Fatal(err)
	}
	s, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Hotkey.Toggle != `Ctrl+"Space"` {
		t.Errorf("toggle = %q, want the quotes escaped and read back", s.Hotkey.Toggle)
	}
	if !strings.Contains(read(t, path), "# Althea settings") {
		t.Error("the default file's comments are missing")
	}
}
