package search

import (
	"cmp"
	"slices"
	"strings"
)

// Candidate is something the search can find.
type Candidate struct {
	ID       string
	Title    string
	Keywords []string // also matched, without highlighting
	Alias    string   // typed exactly, it puts the candidate first
}

// Result is a candidate that matched.
type Result struct {
	ID        string
	Score     int
	Positions []int // in the title, for highlighting
}

// Weights of the ranking beyond the match itself.
const (
	aliasScore = 100_000
	useWeight  = 20  // per unit of use, as frecency counts it
	useMax     = 400 // so much use still cannot outrank a far better match
)

// Index holds candidates prepared for searching: their text folded once, rather than on
// every keystroke. Build one when the candidates change; it is read-only afterwards, so
// several searches may run on it at once.
type Index struct {
	entries []entry
}

type entry struct {
	c        Candidate
	raw, low []rune   // the title as written, and folded
	keywords [][]rune // folded
	alias    string   // folded
	sortKey  string
}

// NewIndex prepares candidates for searching.
func NewIndex(candidates []Candidate) *Index {
	idx := &Index{entries: make([]entry, len(candidates))}
	for i, c := range candidates {
		e := entry{c: c, raw: []rune(c.Title), low: fold(c.Title), alias: string(fold(c.Alias)), sortKey: strings.ToLower(c.Title)}
		for _, k := range c.Keywords {
			e.keywords = append(e.keywords, fold(k))
		}
		idx.entries[i] = e
	}
	return idx
}

// Len returns how many candidates the index holds.
func (idx *Index) Len() int { return len(idx.entries) }

// Rank returns the candidates matching query, best first. Use reports how much a candidate
// is used (its frecency), which lifts it among matches but never makes a non-match match.
// With an empty query, every candidate comes back, the most used first.
func (idx *Index) Rank(query string, use func(id string) float64) []Result {
	q := fold(strings.TrimSpace(query))
	var buf matcher
	type ranked struct {
		Result
		key string
	}
	found := make([]ranked, 0, 64)
	for i := range idx.entries {
		e := &idx.entries[i]
		r, ok := buf.rank(e, q)
		if !ok {
			continue
		}
		r.Score += min(int(use(e.c.ID)*useWeight), useMax)
		found = append(found, ranked{r, e.sortKey})
	}
	slices.SortStableFunc(found, func(a, b ranked) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.key, b.key)
	})
	results := make([]Result, len(found))
	for i, f := range found {
		results[i] = f.Result
	}
	return results
}

// Rank is NewIndex(candidates).Rank(query, use), for candidates searched once.
func Rank(candidates []Candidate, query string, use func(id string) float64) []Result {
	return NewIndex(candidates).Rank(query, use)
}

func (m *matcher) rank(e *entry, q []rune) (Result, bool) {
	r := Result{ID: e.c.ID}
	if len(q) == 0 {
		return r, true
	}
	if e.alias != "" && e.alias == string(q) {
		r.Score = aliasScore
		return r, true
	}
	s, pos, ok := m.match(q, e.raw, e.low, true)
	if ok {
		r.Score, r.Positions = s, pos
	}
	for _, k := range e.keywords {
		// A keyword match counts for less than a title match, and highlights nothing.
		if ks, _, kok := m.match(q, k, k, false); kok && (!ok || ks*3/4 > r.Score) {
			r.Score, r.Positions, ok = ks*3/4, nil, true
		}
	}
	return r, ok
}
