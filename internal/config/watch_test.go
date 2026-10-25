package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/config"
)

type loaded struct {
	s        config.Settings
	problems []config.Problem
	err      error
}

func watch(t *testing.T, path string) <-chan loaded {
	t.Helper()
	got := make(chan loaded, 16)
	w, err := config.Watch(path, func(s config.Settings, problems []config.Problem, err error) {
		got <- loaded{s, problems, err}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	return got
}

func next(t *testing.T, got <-chan loaded) loaded {
	t.Helper()
	select {
	case l := <-got:
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no reload within 5s")
		return loaded{}
	}
}

func TestWatchReloadsWhenTheFileIsSaved(t *testing.T) {
	path := write(t, "[general]\nlanguage = \"en\"\n")
	got := watch(t, path)

	if err := os.WriteFile(path, []byte("[general]\nlanguage = \"pt-BR\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := next(t, got); l.err != nil || l.s.General.Language != "pt-BR" {
		t.Errorf("reload = %+v, want language pt-BR", l)
	}
}

func TestWatchSeesFilesReplacedByRename(t *testing.T) {
	// Editors such as VS Code and Vim save by writing a new file and renaming it over the old.
	path := write(t, "")
	got := watch(t, path)

	tmp := filepath.Join(filepath.Dir(path), "settings.toml.tmp")
	if err := os.WriteFile(tmp, []byte("[hotkey]\ntoggle = \"Win\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if l := next(t, got); l.s.Hotkey.Toggle != "Win" {
		t.Errorf("reload = %+v, want toggle Win", l)
	}
}

func TestWatchReportsBrokenFiles(t *testing.T) {
	path := write(t, "")
	got := watch(t, path)

	if err := os.WriteFile(path, []byte("[general\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := next(t, got); l.err == nil {
		t.Errorf("reload = %+v, want a parse error", l)
	}
}

func TestWatchCoalescesBurstsOfWrites(t *testing.T) {
	path := write(t, "")
	got := watch(t, path)

	for _, lang := range []string{"en", "pt-BR", "en", "pt-BR"} {
		if err := os.WriteFile(path, []byte("[general]\nlanguage = \""+lang+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if l := next(t, got); l.s.General.Language != "pt-BR" {
		t.Errorf("reload = %+v, want the last write", l)
	}
	select {
	case l := <-got:
		t.Errorf("extra reload %+v after one burst", l)
	case <-time.After(400 * time.Millisecond):
	}
}
