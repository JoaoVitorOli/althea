package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/foldersync"
)

func init() { foldersync.NewFolderKDF = foldersync.KDF{Time: 1, Memory: 1024, Threads: 1} }

// syncedComputer is Althea on a computer of its own, syncing through dir.
func syncedComputer(t *testing.T, dir, device string) (*althea, *foldersync.Syncer) {
	t.Helper()
	ctx := context.Background()
	c, err := bootstrap(ctx, portable(t), "en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	a := &althea{core: c}
	key, err := foldersync.Join(dir, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	return a, &foldersync.Syncer{
		Dir: dir, Key: key, Device: device, Name: device,
		State: a.db.KV(syncStateNamespace), Sources: a.syncSources(),
		Now: time.Now, Log: slog.New(slog.DiscardHandler),
	}
}

func TestSyncCarriesThemesAndSnippets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, sa := syncedComputer(t, dir, "a")
	b, sb := syncedComputer(t, dir, "b")

	theme := []byte("schema = 1\nextends = \"althea-dark\"\nname = \"Mine\"\n")
	if err := os.MkdirAll(a.paths.Themes, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.paths.Themes, "mine.toml"), theme, 0o600); err != nil {
		t.Fatal(err)
	}
	snippets := a.extensionContext(ctx, "althea.snippets").Storage()
	if err := snippets.Set(ctx, "snippet/1", []byte(`{"id":"1","name":"Address"}`)); err != nil {
		t.Fatal(err)
	}
	// The clipboard history is not among what syncs.
	if err := a.extensionContext(ctx, "althea.clipboard").Storage().Set(ctx, "x", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := sa.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := sb.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(b.paths.Themes, "mine.toml"))
	if err != nil || !bytes.Equal(got, theme) {
		t.Errorf("theme on b: %q, %v", got, err)
	}
	snippet, ok, _ := b.extensionContext(ctx, "althea.snippets").Storage().Get(ctx, "snippet/1")
	if !ok || string(snippet) != `{"id":"1","name":"Address"}` {
		t.Errorf("snippet on b: %q", snippet)
	}
	if _, ok, _ := b.extensionContext(ctx, "althea.clipboard").Storage().Get(ctx, "x"); ok {
		t.Error("the clipboard synced")
	}
}

func TestFileSourceStaysInItsFolder(t *testing.T) {
	dir := t.TempDir()
	src := fileSource("theme/", filepath.Join(dir, "themes"), tomlFile)
	for _, name := range []string{
		"../settings.toml", `..\settings.toml`, "sub/x.toml", `sub\x.toml`, "C:x.toml", ".hidden.toml", "x.exe", "..",
	} {
		if err := src.Apply(context.Background(), name, []byte("x")); err == nil {
			t.Errorf("wrote %q", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.toml")); err == nil {
		t.Error("a file was written outside the folder")
	}
}
