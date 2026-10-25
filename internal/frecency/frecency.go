// Package frecency counts how often and how recently each item is picked, so that what the
// user reaches for ranks higher. Each use counts 1 and halves every HalfLife, so a habit fades
// in a few weeks once it stops. Use is also learned per query: picking Slack after typing "s"
// favors Slack for "s" above all.
package frecency

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/internal/store"
)

// HalfLife is how long a use takes to count half.
const HalfLife = 7 * 24 * time.Hour

// queryWeight is how much more use for the same query counts than use in general.
const queryWeight = 2

// Frecency holds the use of every item, in memory, written through to the store. It is safe
// for concurrent use.
type Frecency struct {
	kv  *store.KV
	now func() time.Time

	mu   sync.Mutex
	uses map[string]use // by key: "g/<id>" or "q/<query>\x00<id>"
}

// use is a decayed count as of a time; as returns what it counts at another.
type use struct {
	count float64
	at    time.Time
}

func (u use) as(t time.Time) float64 {
	if u.count == 0 {
		return 0
	}
	return u.count * math.Exp2(-float64(t.Sub(u.at))/float64(HalfLife))
}

// Load reads every item's use from kv in one query. Now tells the time, as time.Now.
func Load(ctx context.Context, kv *store.KV, now func() time.Time) (*Frecency, error) {
	entries, err := kv.List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("frecency: %w", err)
	}
	f := &Frecency{kv: kv, now: now, uses: make(map[string]use, len(entries))}
	for _, e := range entries {
		if u, ok := decode(e.Value); ok {
			f.uses[e.Key] = u
		}
	}
	return f, nil
}

// Record counts one use of item id, picked after typing query.
func (f *Frecency) Record(ctx context.Context, id, query string) error {
	now := f.now()
	keys := []string{"g/" + id}
	if q := normalize(query); q != "" {
		keys = append(keys, "q/"+q+"\x00"+id)
	}
	// Writing under the lock keeps the store in the order of the counts: a use is rare enough.
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		u := use{count: f.uses[k].as(now) + 1, at: now}
		f.uses[k] = u
		if err := f.kv.Set(ctx, k, encode(u)); err != nil {
			return fmt.Errorf("frecency: %w", err)
		}
	}
	return nil
}

// Score returns how much item id is used: its use in general, and more so after query.
func (f *Frecency) Score(id, query string) float64 {
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.uses["g/"+id].as(now)
	if q := normalize(query); q != "" {
		s += queryWeight * f.uses["q/"+q+"\x00"+id].as(now)
	}
	return s
}

func normalize(q string) string { return strings.ToLower(strings.TrimSpace(q)) }

func encode(u use) []byte {
	return []byte(strconv.FormatFloat(u.count, 'g', -1, 64) + "," + strconv.FormatInt(u.at.UnixMilli(), 10))
}

func decode(b []byte) (use, bool) {
	count, at, ok := strings.Cut(string(b), ",")
	c, err1 := strconv.ParseFloat(count, 64)
	ms, err2 := strconv.ParseInt(at, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		return use{}, false
	}
	return use{count: c, at: time.UnixMilli(ms)}, true
}
