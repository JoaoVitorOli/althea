package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var notes = fstest.MapFS{
	"0001_notes.sql":  {Data: []byte("CREATE TABLE notes_note (id INTEGER PRIMARY KEY, body TEXT NOT NULL);")},
	"0002_pinned.sql": {Data: []byte("ALTER TABLE notes_note ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;")},
	"README.md":       {Data: []byte("not a migration")},
	"0003_index.sql":  {Data: []byte("CREATE INDEX notes_pinned ON notes_note (pinned);")},
}

func TestMigrateAppliesInOrderOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "althea.db")

	db := open(t, path)
	if err := db.Migrate(ctx, "althea.notes", notes); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO notes_note (body, pinned) VALUES ('hi', 1)"); err != nil {
		t.Fatalf("migrated table is not usable: %v", err)
	}
	_ = db.Close()

	// Reopening and migrating again must not re-run anything (ALTER TABLE would fail).
	db = open(t, path)
	if err := db.Migrate(ctx, "althea.notes", notes); err != nil {
		t.Fatalf("second Migrate = %v, want nil", err)
	}
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM notes_note").Scan(&n); err != nil || n != 1 {
		t.Errorf("rows after reopening = %d, %v; want 1", n, err)
	}
}

func TestMigrateRollsBackAFailingMigration(t *testing.T) {
	ctx := context.Background()
	db := open(t, filepath.Join(t.TempDir(), "althea.db"))
	broken := fstest.MapFS{
		"0001_ok.sql":     {Data: []byte("CREATE TABLE x_a (id INTEGER);")},
		"0002_broken.sql": {Data: []byte("CREATE TABLE x_b (id INTEGER); INSERT INTO nowhere VALUES (1);")},
	}
	err := db.Migrate(ctx, "x", broken)
	if err == nil || !strings.Contains(err.Error(), "0002_broken.sql") {
		t.Fatalf("Migrate = %v, want an error naming 0002_broken.sql", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO x_a VALUES (1)"); err != nil {
		t.Errorf("the migration before the broken one was lost: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO x_b VALUES (1)"); err == nil {
		t.Error("the broken migration was partly applied: x_b exists")
	}

	// Fixed, it applies from where it stopped.
	broken["0002_broken.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE x_b (id INTEGER);")}
	if err := db.Migrate(ctx, "x", broken); err != nil {
		t.Errorf("Migrate after the fix = %v", err)
	}
}

func TestMigrateRejectsDuplicateVersions(t *testing.T) {
	db := open(t, filepath.Join(t.TempDir(), "althea.db"))
	dup := fstest.MapFS{
		"0001_a.sql": {Data: []byte("SELECT 1;")},
		"0001_b.sql": {Data: []byte("SELECT 1;")},
	}
	if err := db.Migrate(context.Background(), "dup", dup); err == nil {
		t.Error("Migrate with two version 1 files = nil, want an error")
	}
}

func TestMigrationsOfOwnersAreIndependent(t *testing.T) {
	ctx := context.Background()
	db := open(t, filepath.Join(t.TempDir(), "althea.db"))
	a := fstest.MapFS{"0001_init.sql": {Data: []byte("CREATE TABLE a_t (id INTEGER);")}}
	b := fstest.MapFS{"0001_init.sql": {Data: []byte("CREATE TABLE b_t (id INTEGER);")}}
	if err := db.Migrate(ctx, "a", a); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, "b", b); err != nil {
		t.Fatalf("version 1 of another owner was skipped or failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO b_t VALUES (1)"); err != nil {
		t.Error(err)
	}
}
