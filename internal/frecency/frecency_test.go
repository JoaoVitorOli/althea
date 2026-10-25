package frecency_test

import (
	"context"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/frecency"
	"github.com/JoaoVitorOli/althea/internal/store"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "althea.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestUseDecaysWithAHalfLifeOfAWeek(t *testing.T) {
	ctx := context.Background()
	c := &clock{now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	f, err := frecency.Load(ctx, openDB(t).KV("althea.frecency"), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.Record(ctx, "app.code", ""); err != nil {
			t.Fatal(err)
		}
	}
	if s := f.Score("app.code", ""); !near(s, 2) {
		t.Errorf("score after two uses = %v, want 2", s)
	}
	c.now = c.now.Add(frecency.HalfLife)
	if s := f.Score("app.code", ""); !near(s, 1) {
		t.Errorf("score a half-life later = %v, want 1", s)
	}
	if s := f.Score("app.never", ""); s != 0 {
		t.Errorf("score of an unused item = %v", s)
	}
}

func TestUseIsLearnedPerQuery(t *testing.T) {
	ctx := context.Background()
	c := &clock{now: time.Now()}
	f, _ := frecency.Load(ctx, openDB(t).KV("althea.frecency"), c.Now)
	if err := f.Record(ctx, "app.slack", "S"); err != nil {
		t.Fatal(err)
	}
	if f.Score("app.slack", "s") <= f.Score("app.slack", "x") {
		t.Error(`picking Slack for "s" does not favor it for "s" over other queries`)
	}
	if f.Score("app.slack", "x") <= 0 {
		t.Error("use for one query does not count at all for others")
	}
}

func TestUsePersistsAndLoadsInOneQuery(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	c := &clock{now: time.Now()}
	f, _ := frecency.Load(ctx, db.KV("althea.frecency"), c.Now)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		before := db.Queries()
		if err := f.Record(ctx, id, "q"); err != nil {
			t.Fatal(err)
		}
		if n := db.Queries() - before; n > 2 {
			t.Errorf("Record made %d queries, want at most 2", n)
		}
	}

	before := db.Queries()
	g, err := frecency.Load(ctx, db.KV("althea.frecency"), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	if n := db.Queries() - before; n != 1 {
		t.Errorf("Load made %d queries, want 1", n)
	}
	if !near(g.Score("c", "q"), f.Score("c", "q")) {
		t.Errorf("loaded score %v, recorded %v", g.Score("c", "q"), f.Score("c", "q"))
	}
}

func TestRecordAndScoreFromManyGoroutines(t *testing.T) {
	ctx := context.Background()
	f, _ := frecency.Load(ctx, openDB(t).KV("althea.frecency"), time.Now)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_ = f.Record(ctx, "x", "")
			_ = f.Score("x", "")
		})
	}
	wg.Wait()
	if s := f.Score("x", ""); s < 7.9 {
		t.Errorf("score after 8 concurrent uses = %v", s)
	}
}
