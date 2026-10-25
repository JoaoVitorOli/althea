package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/config"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// write puts a settings file with the given contents in a new directory.
func write(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	s, problems, err := config.Load(filepath.Join(t.TempDir(), "settings.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("problems = %v, want none", problems)
	}
	want := config.Settings{
		General: config.General{Language: "system", StartAtLogin: true},
		Theme:   config.Theme{Name: "system", Dark: "althea-dark", Light: "althea-light"},
		Hotkey:  config.Hotkey{Toggle: "Alt+Space"},
		Window:  config.Window{ResetAfter: config.Duration(90 * time.Second)},
	}
	if s != want {
		t.Errorf("Load = %+v, want %+v", s, want)
	}
}

func TestLoadReportsUnknownKeysAndKeepsTheRest(t *testing.T) {
	path := write(t, `[general]
language = "en"
langauge = "pt-BR"

[colours]
accent = "red"
`)
	s, problems, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.General.Language != "en" {
		t.Errorf("language = %q, want %q", s.General.Language, "en")
	}
	want := []config.Problem{
		{Line: 3, Key: "general.langauge", Message: "unknown setting"},
		{Line: 5, Key: "colours", Message: "unknown setting"}, // a whole unknown table, once
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems = %v, want %v", problems, want)
	}
}

func TestLoadReportsInvalidValuesAndKeepsTheirDefaults(t *testing.T) {
	path := write(t, `[general]
language = "fr"

[theme]
name = "midnight"
dark = ""

[hotkey]
toggle = ""

[window]
reset_after = "-5s"
`)
	s, problems, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Defaults()
	want.Theme.Name = "midnight"
	if s != want {
		t.Errorf("Load = %+v, want %+v", s, want)
	}
	wantProblems := []config.Problem{
		{Line: 2, Key: "general.language", Message: `must be "system", "en" or "pt-BR"`},
		{Line: 6, Key: "theme.dark", Message: "must not be empty"},
		{Line: 9, Key: "hotkey.toggle", Message: "must not be empty"},
		{Line: 12, Key: "window.reset_after", Message: "must not be negative"},
	}
	if !slices.Equal(problems, wantProblems) {
		t.Errorf("problems = %v, want %v", problems, wantProblems)
	}
}

func TestLoadReportsUnparsableDurationAsProblem(t *testing.T) {
	path := write(t, "[window]\nreset_after = \"soon\"\n")
	s, problems, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s != config.Defaults() {
		t.Errorf("Load = %+v, want the defaults", s)
	}
	if len(problems) != 1 || problems[0].Line != 2 || problems[0].Key != "window.reset_after" {
		t.Errorf("problems = %v, want one on line 2 for window.reset_after", problems)
	}
}

func TestLoadFailsOnSyntaxErrorWithLine(t *testing.T) {
	path := write(t, "[general]\nlanguage = \"en\n")
	_, _, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v, want a syntax error on line 2", err)
	}
}

func TestWriteDefaultCreatesACommentedFileOfTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Althea", "settings.toml")
	if err := config.WriteDefault(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# ") {
		t.Error("default settings file has no comments")
	}
	s, problems, err := config.Load(path)
	if err != nil || len(problems) != 0 || s != config.Defaults() {
		t.Errorf("Load(default file) = %+v, %v, %v; want the defaults", s, problems, err)
	}
}

func TestWriteDefaultKeepsAnExistingFile(t *testing.T) {
	path := write(t, "[general]\nlanguage = \"en\"\n")
	if err := config.WriteDefault(path); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := config.Load(path); s.General.Language != "en" {
		t.Error("WriteDefault replaced the user's file")
	}
}

func TestLoadOverridesOnlyWhatTheFileSets(t *testing.T) {
	path := write(t, `
# my settings
[general]
language = "pt-BR"

[window]
reset_after = "2m"
`)
	s, problems, err := config.Load(path)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Load: problems %v, err %v", problems, err)
	}
	want := config.Defaults()
	want.General.Language = "pt-BR"
	want.Window.ResetAfter = config.Duration(2 * time.Minute)
	if s != want {
		t.Errorf("Load = %+v, want %+v", s, want)
	}
}
