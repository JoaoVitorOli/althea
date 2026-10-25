package fswatch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/fswatch"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestWatchDirReportsMatchingFilesOnly(t *testing.T) {
	dir := t.TempDir()
	changed := make(chan struct{}, 8)
	w, err := fswatch.WatchDir(dir, func(name string) bool { return strings.HasSuffix(name, ".toml") },
		func() { changed <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Fatal("a file that does not match was reported")
	case <-time.After(3 * fswatch.Debounce):
	}

	if err := os.WriteFile(filepath.Join(dir, "midnight.toml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("a new theme file was not reported")
	}
}

func TestWatchDirCreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	w, err := fswatch.WatchDir(dir, func(string) bool { return true }, func() {})
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("themes directory not created: %v", err)
	}
}
