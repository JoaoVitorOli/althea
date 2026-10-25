package store_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/store"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func open(t *testing.T, path string) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestKVPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "althea.db")

	db := open(t, path)
	if err := db.KV("althea.search").Set(ctx, "frecency", []byte("42")); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	got, ok, err := open(t, path).KV("althea.search").Get(ctx, "frecency")
	if err != nil || !ok || string(got) != "42" {
		t.Errorf("Get = %q, %v, %v; want \"42\", true, nil", got, ok, err)
	}
}

func TestOpenAcceptsPathsWithSpacesAndAccents(t *testing.T) {
	// Windows user folders are often named after people: C:\Users\João Silva.
	path := filepath.Join(t.TempDir(), "João Silva", "Althea", "althea.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	open(t, path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database not created at %s: %v", path, err)
	}
}

func TestKVNamespacesAreIsolated(t *testing.T) {
	ctx := context.Background()
	db := open(t, filepath.Join(t.TempDir(), "althea.db"))
	if err := db.KV("althea.clipboard").Set(ctx, "retention", []byte("30d")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.KV("althea.notes").Get(ctx, "retention"); ok || err != nil {
		t.Errorf("another namespace sees the key: ok %v, err %v", ok, err)
	}
}

func TestKVDeleteAndListByPrefix(t *testing.T) {
	ctx := context.Background()
	kv := open(t, filepath.Join(t.TempDir(), "althea.db")).KV("althea.search")
	for _, k := range []string{"rank/b", "rank/a", "rank/c", "other"} {
		if err := kv.Set(ctx, k, []byte(k)); err != nil {
			t.Fatal(err)
		}
	}
	if err := kv.Delete(ctx, "rank/c"); err != nil {
		t.Fatal(err)
	}
	if err := kv.Delete(ctx, "never-set"); err != nil {
		t.Errorf("Delete of a missing key = %v, want nil", err)
	}

	entries, err := kv.List(ctx, "rank/")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range entries {
		keys = append(keys, e.Key)
		if string(e.Value) != e.Key {
			t.Errorf("value of %s = %q", e.Key, e.Value)
		}
	}
	if want := []string{"rank/a", "rank/b"}; !slices.Equal(keys, want) {
		t.Errorf("List = %v, want %v in key order", keys, want)
	}
}

func TestKVListTreatsPrefixLiterally(t *testing.T) {
	ctx := context.Background()
	kv := open(t, filepath.Join(t.TempDir(), "althea.db")).KV("ns")
	for _, k := range []string{"100%_done", "100abc", "1000"} {
		if err := kv.Set(ctx, k, nil); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := kv.List(ctx, "100%_")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key != "100%_done" {
		t.Errorf("List(\"100%%_\") = %v, want only 100%%_done", entries)
	}
}
