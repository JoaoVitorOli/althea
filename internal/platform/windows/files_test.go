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

// A change made the moment WatchTree returns is heard, whatever the goroutine is doing. Run many
// times, since what breaks this is the goroutine not having run yet.
func TestWatchTreeHearsWhatChangesRightAfterItReturns(t *testing.T) {
	for i := range 50 {
		dir := t.TempDir()
		heard := make(chan string, 64)
		stop, err := WatchTree(dir, func(p string) { heard <- p })
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		deadline := time.After(3 * time.Second)
	wait:
		for {
			select {
			case p := <-heard:
				if p == want {
					break wait
				}
			case <-deadline:
				t.Fatalf("run %d did not hear of %s", i, want)
			}
		}
		stop()
	}
}

// Stopping at any moment ends the watch, as Althea waits for it as it quits.
func TestWatchTreeStopsWhenStoppedAtOnce(t *testing.T) {
	for range 200 {
		stop, err := WatchTree(t.TempDir(), func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		stopped := make(chan struct{})
		go func() {
			stop()
			stop() // again changes nothing
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("stop did not return")
		}
	}
}

func TestWatchTreeHearsAfterManyChangesAndWhenAFolderIsMissing(t *testing.T) {
	if _, err := WatchTree(filepath.Join(t.TempDir(), "missing"), func(string) {}); err == nil {
		t.Error("watching a folder that is not there succeeded")
	}
	dir := t.TempDir()
	heard := make(chan string, 4096)
	stop, err := WatchTree(dir, func(p string) {
		select {
		case heard <- p:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		want := filepath.Join(dir, name)
		if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		deadline := time.After(3 * time.Second)
	wait:
		for {
			select {
			case p := <-heard:
				if p == want {
					break wait
				}
			case <-deadline:
				t.Fatalf("did not hear of %s", want)
			}
		}
	}
}
