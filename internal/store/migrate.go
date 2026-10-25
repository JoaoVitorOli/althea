package store

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Migrate applies, in order, the migrations in fsys that owner has not applied yet. A migration
// is a file named NNNN_description.sql; other files are ignored. Each runs in its own
// transaction, so a failing one leaves nothing behind and the next call resumes from it.
// Owners, such as extension IDs, have independent version sequences.
func (db *DB) Migrate(ctx context.Context, owner string, fsys fs.FS) error {
	migrations, err := readMigrations(fsys)
	if err != nil {
		return fmt.Errorf("migrations of %s: %w", owner, err)
	}
	var applied int
	err = db.sql.QueryRowContext(ctx,
		`SELECT coalesce(max(version), 0) FROM migrations WHERE owner = ?`, owner).Scan(&applied)
	if err != nil {
		return fmt.Errorf("migrations of %s: %w", owner, err)
	}
	for _, m := range migrations {
		if m.version <= applied {
			continue
		}
		if err := db.apply(ctx, owner, m); err != nil {
			return fmt.Errorf("migration %s of %s: %w", m.name, owner, err)
		}
	}
	return nil
}

type migration struct {
	version int
	name    string
	sql     string
}

func readMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, name := range names {
		prefix, _, ok := strings.Cut(path.Base(name), "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version <= 0 {
			return nil, fmt.Errorf("%s: want a name such as 0001_create_table.sql", name)
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: version, name: name, sql: string(body)})
	}
	slices.SortFunc(ms, func(a, b migration) int { return a.version - b.version })
	for i := 1; i < len(ms); i++ {
		if ms[i].version == ms[i-1].version {
			return nil, fmt.Errorf("%s and %s have the same version", ms[i-1].name, ms[i].name)
		}
	}
	return ms, nil
}

func (db *DB) apply(ctx context.Context, owner string, m migration) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO migrations (owner, version, name) VALUES (?, ?, ?)`, owner, m.version, m.name); err != nil {
		return err
	}
	return tx.Commit()
}
