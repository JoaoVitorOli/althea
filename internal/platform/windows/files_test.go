//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKnownFoldersExist(t *testing.T) {
	folders := KnownFolders()
	if len(folders) < 3 {
		t.Fatalf("folders %q", folders)
	}
	for _, f := range folders {
		if !filepath.IsAbs(f) {
			t.Errorf("%q is not absolute", f)
		}
	}
}

func TestWatchTreeHearsDeepChanges(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	heard := make(chan string, 64)
	stop, err := WatchTree(dir, func(p string) { heard <- p })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	want := filepath.Join(deep, "notes.txt")
	if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.After(3 * time.Second); ; {
		select {
		case p := <-heard:
			if p == want {
				return
			}
		case <-deadline:
			t.Fatalf("did not hear of %s", want)
		}
	}
}
