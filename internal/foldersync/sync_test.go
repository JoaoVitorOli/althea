package foldersync

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/store"
)

func init() { NewFolderKDF = KDF{Time: 1, Memory: 1024, Threads: 1} }

// computer is a computer with records of one kind, in memory, sharing a sync folder.
type computer struct {
	*Syncer
	records map[string]string
	clock   *time.Time
	fail    map[string]bool
}

func newComputer(t *testing.T, dir, device string, clock *time.Time, records map[string]string) *computer {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "althea.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	key, err := Join(dir, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	c := &computer{records: records, clock: clock, fail: map[string]bool{}}
	c.Syncer = &Syncer{
		Dir: dir, Key: key, Device: device, Name: device, State: db.KV("sync"),
		Now: func() time.Time { return *c.clock }, Log: slog.New(slog.DiscardHandler),
		Sources: []Source{{
			Prefix: "note/",
			Snapshot: func(context.Context) (map[string][]byte, error) {
				out := map[string][]byte{}
				for k, v := range c.records {
					out[k] = []byte(v)
				}
				return out, nil
			},
			Apply: func(_ context.Context, key string, data []byte) error {
				if c.fail[key] {
					return errors.New("disk full")
				}
				if data == nil {
					delete(c.records, key)
				} else {
					c.records[key] = string(data)
				}
				return nil
			},
		}},
	}
	return c
}

func (c *computer) sync(t *testing.T) Result {
	t.Helper()
	*c.clock = c.clock.Add(time.Second)
	res, err := c.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestJoinChecksThePassphrase(t *testing.T) {
	dir := t.TempDir()
	key, err := Join(dir, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Join(dir, "correct horse")
	if err != nil || !bytes.Equal(key, again) {
		t.Fatalf("again: %v, same key %v", err, bytes.Equal(key, again))
	}
	if _, err := Join(dir, "wrong horse"); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("wrong passphrase: %v", err)
	}
}

func TestTwoComputersMerge(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	a := newComputer(t, dir, "a", &now, map[string]string{"groceries": "milk"})
	b := newComputer(t, dir, "b", &now, map[string]string{"ideas": "an app"})

	if res := a.sync(t); res.Pushed != 1 || res.Devices != 1 {
		t.Errorf("a first: %+v", res)
	}
	if res := b.sync(t); res.Pulled != 1 || res.Pushed != 1 || res.Devices != 2 {
		t.Errorf("b first: %+v", res)
	}
	a.sync(t)
	want := map[string]string{"groceries": "milk", "ideas": "an app"}
	if !maps.Equal(a.records, want) || !maps.Equal(b.records, want) {
		t.Fatalf("a %v, b %v", a.records, b.records)
	}

	// Both change the same record: the later write wins on both.
	a.records["groceries"] = "milk, eggs"
	a.sync(t)
	b.records["groceries"] = "milk, coffee"
	b.sync(t)
	a.sync(t)
	if a.records["groceries"] != "milk, coffee" || b.records["groceries"] != "milk, coffee" {
		t.Errorf("a %q, b %q", a.records["groceries"], b.records["groceries"])
	}

	// A delete goes over too, and a quiet sync writes nothing.
	delete(b.records, "ideas")
	b.sync(t)
	if res := a.sync(t); res.Pulled != 1 {
		t.Errorf("a pulled %+v", res)
	}
	if _, ok := a.records["ideas"]; ok {
		t.Error("the delete did not reach a")
	}
	path := filepath.Join(dir, devicesDir, "a"+fileExt)
	before, _ := os.Stat(path)
	if res := a.sync(t); res != (Result{Devices: 2}) {
		t.Errorf("quiet sync: %+v", res)
	}
	if after, _ := os.Stat(path); !after.ModTime().Equal(before.ModTime()) {
		t.Error("a quiet sync wrote the file again")
	}
}

func TestAComputerThatJoinsTakesWhatIsThere(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	a := newComputer(t, dir, "a", &now, map[string]string{"settings": "theme = dark"})
	a.sync(t)
	// b starts later with its defaults: they do not replace what a had.
	now = now.Add(time.Hour)
	b := newComputer(t, dir, "b", &now, map[string]string{"settings": "theme = light", "own": "kept"})
	b.sync(t)
	a.sync(t)
	if b.records["settings"] != "theme = dark" || a.records["own"] != "kept" {
		t.Errorf("a %v, b %v", a.records, b.records)
	}
	// After joining, b's changes count as any other.
	b.records["settings"] = "theme = light"
	b.sync(t)
	a.sync(t)
	if a.records["settings"] != "theme = light" {
		t.Errorf("a %v", a.records)
	}
}

func TestARecordNotWrittenComesAgain(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	a := newComputer(t, dir, "a", &now, map[string]string{"x": "1"})
	b := newComputer(t, dir, "b", &now, map[string]string{})
	a.sync(t)
	b.fail["x"] = true
	*b.clock = b.clock.Add(time.Second)
	if _, err := b.Sync(context.Background()); err == nil {
		t.Fatal("no error for a record not written")
	}
	b.fail["x"] = false
	b.sync(t)
	if b.records["x"] != "1" {
		t.Errorf("b %v", b.records)
	}
}

func TestFilesAreEncrypted(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	a := newComputer(t, dir, "a", &now, map[string]string{"secret": "the boat plan"})
	a.sync(t)
	data, err := os.ReadFile(filepath.Join(dir, devicesDir, "a"+fileExt))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("boat")) || bytes.Contains(data, []byte("secret")) {
		t.Error("the file is readable without the passphrase")
	}
	// A file renamed to pass for another computer's is not read.
	if err := os.WriteFile(filepath.Join(dir, devicesDir, "z"+fileExt), data, 0o600); err != nil {
		t.Fatal(err)
	}
	b := newComputer(t, dir, "b", &now, map[string]string{})
	if res := b.sync(t); res.Devices != 2 || len(b.records) != 1 {
		t.Errorf("res %+v, records %v", res, b.records)
	}
}
