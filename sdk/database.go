package sdk

import (
	"context"
	"database/sql"
)

// Database is Althea's SQLite database, for the tables an extension creates with its
// Manifest.Migrations. It has FTS5 for full-text search. Name tables after the extension, as
// clipboard_items for althea.clipboard, and touch no others.
type Database interface {
	Queryer
	// Tx runs fn in a transaction, committed when fn returns nil and rolled back otherwise.
	Tx(ctx context.Context, fn func(tx Queryer) error) error
}

// Queryer runs SQL, in the database or in a transaction.
type Queryer interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
}
