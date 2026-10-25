package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestQueriesCountsEveryStatement(t *testing.T) {
	ctx := context.Background()
	db := open(t, filepath.Join(t.TempDir(), "althea.db"))
	kv := db.KV("althea.search")
	for i := range 100 {
		if err := kv.Set(ctx, fmt.Sprintf("k%03d", i), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		op   func() error
		want int64
	}{
		{"List of 100 entries", func() error { _, err := kv.List(ctx, "k"); return err }, 1},
		{"Get", func() error { _, _, err := kv.Get(ctx, "k001"); return err }, 1},
		{"Set", func() error { return kv.Set(ctx, "k001", []byte("w")) }, 1},
		{"Delete", func() error { return kv.Delete(ctx, "k001") }, 1},
		{"Exec", func() error { _, err := db.Exec(ctx, "SELECT 1"); return err }, 1},
		{"QueryRow", func() error { var n int; return db.QueryRow(ctx, "SELECT 1").Scan(&n) }, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := db.Queries()
			if err := tc.op(); err != nil {
				t.Fatal(err)
			}
			if got := db.Queries() - before; got != tc.want {
				t.Errorf("%s made %d queries, want %d", tc.name, got, tc.want)
			}
		})
	}
}
