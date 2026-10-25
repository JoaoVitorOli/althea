// Package store keeps Althea's data in one SQLite database: a key-value store per namespace,
// tables that packages create through migrations, and a count of the queries made, which
// tests use to catch N+1 access. See docs/adr/0007-sqlite.md.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"sync/atomic"

	_ "modernc.org/sqlite" // the pure Go driver, registered as "sqlite"
)

// DB is Althea's database.
type DB struct {
	sql     *sql.DB
	queries atomic.Int64
}

// Open opens the database file at path, creating it if needed, in WAL mode.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db := &DB{sql: sqldb}
	if err := db.init(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return db, nil
}

//go:embed migrations/*.sql
var migrations embed.FS

// owner is the store's own name among the owners of migrations.
const owner = "althea.store"

func (db *DB) init(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS migrations (
		owner      TEXT    NOT NULL,
		version    INTEGER NOT NULL,
		name       TEXT    NOT NULL,
		applied_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		PRIMARY KEY (owner, version)
	) WITHOUT ROWID`); err != nil {
		return err
	}
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	return db.Migrate(ctx, owner, sub)
}

// Close closes the database.
func (db *DB) Close() error { return db.sql.Close() }

// Exec runs a statement that returns no rows, for tables a package created by migration.
func (db *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.queries.Add(1)
	return db.sql.ExecContext(ctx, query, args...)
}

// Query runs a query that returns rows.
func (db *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db.queries.Add(1)
	return db.sql.QueryContext(ctx, query, args...)
}

// QueryRow runs a query that returns at most one row.
func (db *DB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	db.queries.Add(1)
	return db.sql.QueryRowContext(ctx, query, args...)
}

// Queries returns how many statements the database has run through Exec, Query, QueryRow and
// the key-value stores. Tests compare it before and after an operation to catch N+1 access.
func (db *DB) Queries() int64 { return db.queries.Load() }

// KV returns the key-value store of a namespace, such as an extension's ID.
func (db *DB) KV(namespace string) *KV { return &KV{db: db, ns: namespace} }

// KV is the key-value store of one namespace.
type KV struct {
	db *DB
	ns string
}

// Get returns the value of key, and whether it is set.
func (kv *KV) Get(ctx context.Context, key string) (value []byte, ok bool, err error) {
	err = kv.db.QueryRow(ctx,
		`SELECT value FROM kv WHERE namespace = ? AND key = ?`, kv.ns, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get %s/%s: %w", kv.ns, key, err)
	}
	return value, true, nil
}

// Set sets the value of key. A nil value is stored as an empty one.
func (kv *KV) Set(ctx context.Context, key string, value []byte) error {
	if value == nil {
		value = []byte{}
	}
	_, err := kv.db.Exec(ctx,
		`INSERT INTO kv (namespace, key, value) VALUES (?, ?, ?)
		 ON CONFLICT (namespace, key) DO UPDATE SET value = excluded.value`, kv.ns, key, value)
	if err != nil {
		return fmt.Errorf("set %s/%s: %w", kv.ns, key, err)
	}
	return nil
}

// Delete removes key; removing a key that is not set is not an error.
func (kv *KV) Delete(ctx context.Context, key string) error {
	_, err := kv.db.Exec(ctx, `DELETE FROM kv WHERE namespace = ? AND key = ?`, kv.ns, key)
	if err != nil {
		return fmt.Errorf("delete %s/%s: %w", kv.ns, key, err)
	}
	return nil
}

// Entry is a key and its value.
type Entry struct {
	Key   string
	Value []byte
}

// List returns the entries whose keys start with prefix, in key order, in one query.
func (kv *KV) List(ctx context.Context, prefix string) ([]Entry, error) {
	// A range over the primary key rather than LIKE: it uses the index, and % and _ in the
	// prefix are plain characters.
	rows, err := kv.db.Query(ctx,
		`SELECT key, value FROM kv WHERE namespace = ? AND key >= ? AND key < ? ORDER BY key`,
		kv.ns, prefix, prefix+"\U0010FFFF")
	if err != nil {
		return nil, fmt.Errorf("list %s/%s: %w", kv.ns, prefix, err)
	}
	defer func() { _ = rows.Close() }()
	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Key, &e.Value); err != nil {
			return nil, fmt.Errorf("list %s/%s: %w", kv.ns, prefix, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %s/%s: %w", kv.ns, prefix, err)
	}
	return entries, nil
}
