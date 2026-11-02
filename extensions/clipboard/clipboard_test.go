package clipboard_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/clipboard"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

var key = bytes.Repeat([]byte{7}, 32)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newExt(t *testing.T, lang string) (*clipboard.Extension, *sdktest.Context, *clock) {
	t.Helper()
	e := clipboard.New(clipboard.Platform{Key: func() ([]byte, error) { return key, nil }})
	c := &clock{time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)}
	e.Now = c.now
	return e, sdktest.New(t, e, lang), c
}

func search(t *testing.T, e *clipboard.Extension, ctx *sdktest.Context, q string) []sdk.Item {
	t.Helper()
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.List).Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func titles(items []sdk.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestHistoryIsNewestFirstAndCopyingAgainMovesUp(t *testing.T) {
	e, ctx, c := newExt(t, "en")
	for _, text := range []string{"first", "second", "third"} {
		c.t = c.t.Add(time.Minute)
		if err := e.Record(ctx, clipboard.Copied{Text: text, Source: "notepad.exe"}); err != nil {
			t.Fatal(err)
		}
	}
	c.t = c.t.Add(time.Minute)
	_ = e.Record(ctx, clipboard.Copied{Text: "first"})
	if got := strings.Join(titles(search(t, e, ctx, "")), ","); got != "first,third,second" {
		t.Errorf("history = %s", got)
	}
	if got := titles(search(t, e, ctx, "SEC")); len(got) != 1 || got[0] != "second" {
		t.Errorf("search = %q", got)
	}
}

func TestContentsAreEncryptedAtRest(t *testing.T) {
	e, ctx, _ := newExt(t, "en")
	if err := e.Record(ctx, clipboard.Copied{Text: "my secret note", Source: "notepad.exe"}); err != nil {
		t.Fatal(err)
	}
	var content, digest []byte
	if err := ctx.Database().QueryRow(ctx, `SELECT content, digest FROM clipboard_items`).Scan(&content, &digest); err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{content, digest} {
		if bytes.Contains(b, []byte("secret")) || bytes.Contains(b, []byte("notepad")) {
			t.Errorf("stored in the clear: %q", b)
		}
	}
}

func TestKindsPreviewsAndSections(t *testing.T) {
	e, ctx, c := newExt(t, "pt-BR")
	c.t = c.t.Add(-10 * 24 * time.Hour)
	_ = e.Record(ctx, clipboard.Copied{Text: "https://go.dev"})
	c.t = c.t.Add(10 * 24 * time.Hour)
	_ = e.Record(ctx, clipboard.Copied{Text: "#7c3aed"})
	_ = e.Record(ctx, clipboard.Copied{Image: pngOf(t, 1920, 1080)})
	report := filepath.Join(t.TempDir(), "report.pdf")
	_ = e.Record(ctx, clipboard.Copied{Files: []string{report, filepath.Join(filepath.Dir(report), "b.txt")}})
	items := search(t, e, ctx, "")
	if len(items) != 4 {
		t.Fatalf("items %q", titles(items))
	}
	byTitle := map[string]sdk.Item{}
	for _, it := range items {
		byTitle[it.Title] = it
	}
	img, ok := byTitle["Imagem"]
	if !ok || img.Icon.Data == nil || img.Detail.Image == nil {
		t.Fatalf("image item %+v", img)
	}
	if decoded, err := png.Decode(bytes.NewReader(img.Detail.Image)); err != nil || decoded.Bounds().Dx() != 480 {
		t.Errorf("preview %v, %v; want 480 wide", decoded.Bounds(), err)
	}
	if byTitle["#7c3aed"].Icon.Data == nil {
		t.Error("a color has no swatch")
	}
	if f := byTitle["report.pdf e mais 1"]; f.Icon.File != report {
		t.Errorf("files item %+v", f)
	}
	if link := byTitle["https://go.dev"]; link.Section != "Antes" || link.Icon.Glyph != "link" {
		t.Errorf("link item section %q icon %+v", link.Section, link.Icon)
	}
	if got := search(t, e, ctx, "imagem"); len(got) != 1 {
		t.Errorf("searching the kind gives %q", titles(got))
	}
}

func TestPasteCopyPinAndDelete(t *testing.T) {
	e, ctx, c := newExt(t, "en")
	_ = e.Record(ctx, clipboard.Copied{Text: "bold", HTML: "<b>bold</b>"})
	c.t = c.t.Add(time.Minute)
	img := pngOf(t, 10, 10)
	_ = e.Record(ctx, clipboard.Copied{Image: img})

	items := search(t, e, ctx, "")
	if err := items[0].Actions[0].Run(ctx); err != nil { // paste the image, read whole
		t.Fatal(err)
	}
	if !bytes.Equal(ctx.Pasted[0].Image, img) {
		t.Error("the image pasted is not the one copied")
	}
	text := items[1]
	if len(text.Actions) != 6 || text.Actions[2].Shortcut != "Ctrl+Shift+V" {
		t.Fatalf("text actions %+v", text.Actions)
	}
	_ = text.Actions[2].Run(ctx)
	if p := ctx.Pasted[1]; p.Text != "bold" || p.HTML != "" {
		t.Errorf("plain paste %+v", p)
	}
	_ = text.Actions[1].Run(ctx)
	if p := ctx.Clips[0]; p.HTML != "<b>bold</b>" {
		t.Errorf("copy %+v", p)
	}

	_ = text.Actions[3].Run(ctx) // pin
	items = search(t, e, ctx, "")
	if items[0].Title != "bold" || items[0].Section != "Pinned" {
		t.Fatalf("after pinning %q, section %q", titles(items), items[0].Section)
	}
	_ = items[0].Actions[5].Run(ctx) // delete all unpinned
	if got := titles(search(t, e, ctx, "")); len(got) != 1 || got[0] != "bold" {
		t.Errorf("after deleting all: %q", got)
	}
}

// fakeWatch is a clipboard that the test copies into.
type fakeWatch struct {
	mu     sync.Mutex
	copied func(clipboard.Copied)
}

func (f *fakeWatch) watch(copied func(clipboard.Copied)) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = copied
	return func() {}, nil
}

func (f *fakeWatch) copy(t *testing.T, c clipboard.Copied) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); ; {
		f.mu.Lock()
		fn := f.copied
		f.mu.Unlock()
		if fn != nil {
			fn(c)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("not watching")
		}
		time.Sleep(time.Millisecond)
	}
}

// canceling is a Context that ends with another, leaving the test's own usable after it. Changing
// the test's Context instead would race with the database, which watches the one it was given.
type canceling struct {
	*sdktest.Context
	c context.Context
}

func (c canceling) Done() <-chan struct{} { return c.c.Done() }
func (c canceling) Err() error            { return c.c.Err() }

func TestServeKeepsCopiesButIgnoredAppsAndOldOnes(t *testing.T) {
	w := &fakeWatch{}
	e := clipboard.New(clipboard.Platform{Key: func() ([]byte, error) { return key, nil }, Watch: w.watch})
	c := &clock{time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)}
	e.Now = c.now
	ctx := sdktest.New(t, e, "en")
	ctx.Prefs["ignore"] = "KeePass, bitwarden.exe"
	ctx.Prefs["keep"] = "day"
	cctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- e.Serve(canceling{ctx, cctx}) }()

	w.copy(t, clipboard.Copied{Text: "old", Source: "notepad.exe"})
	c.t = c.t.Add(25 * time.Hour)
	w.copy(t, clipboard.Copied{Text: "hunter2", Source: "KeePass.exe"})
	w.copy(t, clipboard.Copied{Text: "token", Source: "Bitwarden.exe"})
	w.copy(t, clipboard.Copied{Text: "new", Source: "notepad.exe"})
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := titles(search(t, e, ctx, "")); len(got) != 1 || got[0] != "new" {
		t.Errorf("history = %q; want only what is recent and not ignored", got)
	}
}
