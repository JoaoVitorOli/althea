// Package registry gathers what every extension offers (commands, items and answers) and
// answers the root search with it: the items ranked by the fuzzy search and the user's habits,
// under the answers of the moment. A misbehaving extension costs only its own results: its
// panics become log entries and its slow answers are dropped.
package registry

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/internal/search"
	"github.com/JoaoVitorOli/althea/sdk"
)

// Deps are what the registry needs from the host.
type Deps struct {
	// Use returns how much the item with a key ("<extension>/<item>") is used after query.
	Use func(key, query string) float64
	// Context makes the context an extension's call gets.
	Context func(ctx context.Context, extension string) sdk.Context
	Log     *slog.Logger
	// AnswerTimeout bounds how long a query waits for answers; 50 ms when zero.
	AnswerTimeout time.Duration
}

// Result is one row of the root search.
type Result struct {
	Key       string // "<extension>/<item>", unique across extensions
	Extension string
	Item      sdk.Item
	Positions []int // matched letters of the title
	Answer    bool  // an answer to the query, shown first
	Command   bool  // one of an extension's commands, rather than an item
}

// Registry holds the extensions and what they offer. It is safe for concurrent use: Refresh
// swaps a new index in while queries read the old one.
type Registry struct {
	exts []sdk.Extension
	deps Deps

	mu    sync.RWMutex
	index *search.Index
	items map[string]Result // by key
}

// New returns a registry of extensions; call Refresh before the first query.
func New(exts []sdk.Extension, d Deps) *Registry {
	if d.AnswerTimeout <= 0 {
		d.AnswerTimeout = 50 * time.Millisecond
	}
	return &Registry{exts: exts, deps: d, index: search.NewIndex(nil), items: map[string]Result{}}
}

// Refresh collects the commands and items of every extension and rebuilds the index. An
// extension that fails keeps none of its items until the next refresh; the others go on.
func (r *Registry) Refresh(ctx context.Context) error {
	items := map[string]Result{}
	var candidates []search.Candidate
	add := func(ext string, it sdk.Item, command bool) {
		key := ext + "/" + it.ID
		items[key] = Result{Key: key, Extension: ext, Item: it, Command: command}
		candidates = append(candidates, search.Candidate{ID: key, Title: it.Title, Keywords: it.Keywords})
	}
	for _, e := range r.exts {
		id := e.Manifest().ID
		var cmds []sdk.Command
		if err := r.guard(id, "commands", func() error { cmds = e.Commands(); return nil }); err != nil {
			continue
		}
		for _, c := range cmds {
			add(id, commandItem(c), true)
		}
		src, ok := e.(sdk.ItemSource)
		if !ok {
			continue
		}
		var its []sdk.Item
		if err := r.guard(id, "items", func() (err error) {
			its, err = src.Items(r.deps.Context(ctx, id))
			return err
		}); err != nil {
			continue
		}
		for _, it := range its {
			add(id, it, false)
		}
	}
	index := search.NewIndex(candidates)
	r.mu.Lock()
	r.index, r.items = index, items
	r.mu.Unlock()
	return nil
}

// commandItem turns a command into a row whose first action runs it.
func commandItem(c sdk.Command) sdk.Item {
	return sdk.Item{
		ID: c.ID, Title: c.Title, Subtitle: c.Subtitle, Icon: c.Icon, Keywords: c.Keywords,
		Actions: []sdk.Action{{Title: c.Title, Run: c.Run}},
	}
}

// Query returns the rows for query: answers first, then the items that match, best first.
func (r *Registry) Query(ctx context.Context, query string) []Result {
	answers := r.answers(ctx, query)

	r.mu.RLock()
	index, items := r.index, r.items
	r.mu.RUnlock()
	ranked := index.Rank(query, func(key string) float64 { return r.deps.Use(key, query) })

	results := make([]Result, 0, len(answers)+len(ranked))
	results = append(results, answers...)
	for _, rk := range ranked {
		res := items[rk.ID]
		res.Positions = rk.Positions
		results = append(results, res)
	}
	return results
}

// answers asks every Answerer at once and keeps the answers that arrive in time, in the order
// of the extensions.
func (r *Registry) answers(ctx context.Context, query string) []Result {
	if query == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, r.deps.AnswerTimeout)
	defer cancel()

	type answer struct {
		order int
		res   Result
	}
	var asked int
	got := make(chan answer, len(r.exts)) // buffered: a late answer never blocks its goroutine
	for i, e := range r.exts {
		a, ok := e.(sdk.Answerer)
		if !ok {
			continue
		}
		asked++
		id := e.Manifest().ID
		go func() {
			var it *sdk.Item
			_ = r.guard(id, "answer", func() (err error) {
				it, err = a.Answer(r.deps.Context(ctx, id), query)
				return err
			})
			if it == nil {
				got <- answer{order: -1}
				return
			}
			got <- answer{order: i, res: Result{Key: id + "/" + it.ID, Extension: id, Item: *it, Answer: true}}
		}()
	}

	byOrder := map[int]Result{}
	for late := false; asked > 0 && !late; asked-- {
		select {
		case a := <-got:
			if a.order >= 0 {
				byOrder[a.order] = a.res
			}
		case <-ctx.Done():
			late = true // the rest answer too late to show
		}
	}
	var out []Result
	for i := range r.exts {
		if res, ok := byOrder[i]; ok {
			out = append(out, res)
		}
	}
	return out
}

// guard runs an extension's code, turning its panics and errors into log entries.
func (r *Registry) guard(ext, what string, fn func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
			r.deps.Log.Error("extension panicked", "extension", ext, "call", what, "panic", p)
		}
	}()
	if err = fn(); err != nil {
		r.deps.Log.Warn("extension failed", "extension", ext, "call", what, "err", err)
	}
	return err
}
