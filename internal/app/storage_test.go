package app

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/JoaoVitorOli/althea/sdk"
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

type migrating struct {
	id  string
	sql string
}

func (m migrating) Manifest() sdk.Manifest {
	return sdk.Manifest{ID: m.id, Migrations: fstest.MapFS{"0001_create.sql": {Data: []byte(m.sql)}}}
}
func (migrating) Commands(sdk.Context) []sdk.Command { return nil }

func TestExtensionsGetTheirTablesOrAreLeftOut(t *testing.T) {
	ctx := context.Background()
	c, err := bootstrap(ctx, portable(t), "en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	a := &althea{core: c}
	good := migrating{"althea.good", `CREATE TABLE good_items (v TEXT)`}
	bad := migrating{"althea.bad", `CREATE TABLE oops (`}
	kept := a.migrate([]sdk.Extension{good, bad})
	if len(kept) != 1 || kept[0].Manifest().ID != "althea.good" {
		t.Fatalf("kept %v", kept)
	}
	db := a.extensionContext(ctx, "althea.good").Database()
	err = db.Tx(ctx, func(tx sdk.Queryer) error {
		_, err := tx.Exec(ctx, `INSERT INTO good_items VALUES ('x')`)
		return err
	})
	var n int
	if err != nil || db.QueryRow(ctx, `SELECT count(*) FROM good_items`).Scan(&n) != nil || n != 1 {
		t.Errorf("insert %v, rows %d", err, n)
	}
}
