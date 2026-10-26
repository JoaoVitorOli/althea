package files_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/files"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func touch(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// watcher is a fake that the test tells of changes.
type watcher struct {
	mu      sync.Mutex
	changed map[string]func(string)
}

func (w *watcher) watch(dir string, changed func(string)) (func(), error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.changed[dir] = changed
	return func() {}, nil
}

func (w *watcher) tell(t *testing.T, dir, path string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); ; {
		w.mu.Lock()
		fn := w.changed[dir]
		w.mu.Unlock()
		if fn != nil {
			fn(path)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("not watching")
		}
		time.Sleep(time.Millisecond)
	}
}

// indexed starts the service over dir and waits for the first scan.
func indexed(t *testing.T, dir string) (*sdktest.Context, *watcher) {
	t.Helper()
	w := &watcher{changed: map[string]func(string){}}
	e := files.New(files.Platform{Folders: func() []string { return []string{dir} }, Watch: w.watch})
	ctx := sdktest.New(t, e, "en")
	c, cancel := context.WithCancel(context.Background())
	ctx.Context = c
	done := make(chan error)
	go func() { done <- e.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	w.tell(t, dir, "x-never") // the watcher starts after the scan
	return ctx, w
}

func names(fs []files.File) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return strings.Join(out, ",")
}

func find(t *testing.T, ctx *sdktest.Context, q string) string {
	t.Helper()
	found, err := files.Find(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	return names(found)
}

func TestSearchRanksNamesAndIgnoresAccents(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "report.pdf"), time.Hour)
	touch(t, filepath.Join(dir, "old", "annual report 2025.docx"), 2*time.Hour)
	touch(t, filepath.Join(dir, "myreport.txt"), 3*time.Hour)
	touch(t, filepath.Join(dir, "Relatório Anual.xlsx"), 4*time.Hour)
	touch(t, filepath.Join(dir, "node_modules", "report.js"), 0)
	touch(t, filepath.Join(dir, ".git", "report"), 0)
	ctx, _ := indexed(t, dir)

	if got := find(t, ctx, "report"); got != "report.pdf,annual report 2025.docx,myreport.txt" {
		t.Errorf("report: %s", got)
	}
	if got := find(t, ctx, "relatorio anual"); got != "Relatório Anual.xlsx" {
		t.Errorf("accents: %s", got)
	}
	if got := find(t, ctx, "my"); got != "myreport.txt" {
		t.Errorf("a short word: %s", got)
	}
	if got := find(t, ctx, ""); !strings.HasPrefix(got, "report.pdf,annual") {
		t.Errorf("recent: %s", got)
	}
}

func TestTheIndexFollowsChanges(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "keep.txt"), 0)
	touch(t, filepath.Join(dir, "gone", "inside.txt"), 0)
	ctx, w := indexed(t, dir)

	// A folder moved in is read whole; a folder removed goes with what it held.
	touch(t, filepath.Join(dir, "moved", "deep", "plan.md"), 0)
	if err := os.RemoveAll(filepath.Join(dir, "gone")); err != nil {
		t.Fatal(err)
	}
	w.tell(t, dir, filepath.Join(dir, "moved"))
	w.tell(t, dir, filepath.Join(dir, "gone"))
	waitFor(t, func() bool { return find(t, ctx, "plan") == "plan.md" && find(t, ctx, "inside") == "" })

	// Lost changes read the folder again.
	touch(t, filepath.Join(dir, "quiet.txt"), 0)
	w.tell(t, dir, "")
	waitFor(t, func() bool { return find(t, ctx, "quiet") == "quiet.txt" })
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestItemsOpenRevealAndCopy(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.txt"), 0)
	ctx, _ := indexed(t, dir)
	var revealed string
	e := files.New(files.Platform{Reveal: func(p string) error { revealed = p; return nil }})
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := ctx.Pushed[0].(*sdk.List).Search(ctx, "a.txt")
	if err != nil || len(items) != 1 {
		t.Fatalf("items %+v, %v", items, err)
	}
	it := items[0]
	want := filepath.Join(dir, "a.txt")
	if it.Subtitle != dir || it.Icon.File != want {
		t.Errorf("item %+v", it)
	}
	for _, a := range it.Actions {
		if err := a.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if ctx.Opened[0] != want || revealed != want || ctx.Copied[0] != want || ctx.Clips[0].Files[0] != want {
		t.Errorf("opened %q, revealed %q, copied %q, clips %+v", ctx.Opened, revealed, ctx.Copied, ctx.Clips)
	}
}
