package app

import (
	"context"
	"testing"
)

func TestExtensionsKeepTheirOwnStorage(t *testing.T) {
	ctx := context.Background()
	c, err := bootstrap(ctx, portable(t), "en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	a := &althea{core: c}

	notes := a.extensionContext(ctx, "althea.notes").Storage()
	other := a.extensionContext(ctx, "althea.other").Storage()
	if err := notes.Set(ctx, "note/1", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := notes.Set(ctx, "note/2", []byte("world")); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := other.Get(ctx, "note/1"); ok {
		t.Error("another extension reads the notes")
	}
	entries, err := notes.List(ctx, "note/")
	if err != nil || len(entries) != 2 || string(entries[1].Value) != "world" {
		t.Fatalf("List = %v, %v", entries, err)
	}
	if err := notes.Delete(ctx, "note/1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := notes.Get(ctx, "note/1"); ok {
		t.Error("a deleted key is still set")
	}
}
