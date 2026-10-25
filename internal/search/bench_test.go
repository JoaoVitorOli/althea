package search_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/search"
)

// candidates returns n realistic names: a few words each, as apps, files and commands have.
func candidates(n int) []search.Candidate {
	words := strings.Fields("visual studio code microsoft edge google chrome windows terminal power shell " +
		"settings control panel calculator notepad paint photos camera spotify discord slack zoom teams " +
		"word excel outlook onenote steam epic games blender figma obsidian docker postman git hub desktop")
	r := rand.New(rand.NewPCG(1, 2))
	out := make([]search.Candidate, n)
	for i := range out {
		parts := make([]string, 2+r.IntN(3))
		for j := range parts {
			w := words[r.IntN(len(words))]
			parts[j] = strings.ToUpper(w[:1]) + w[1:]
		}
		out[i] = search.Candidate{ID: fmt.Sprint(i), Title: strings.Join(parts, " "), Keywords: []string{words[r.IntN(len(words))]}}
	}
	return out
}

func BenchmarkRank10k(b *testing.B) {
	idx := search.NewIndex(candidates(10_000))
	use := func(string) float64 { return 0 }
	for _, q := range []string{"v", "vsc", "chrome", "windows term"} {
		b.Run(q, func(b *testing.B) {
			for b.Loop() {
				idx.Rank(q, use)
			}
		})
	}
}

// TestRankBudget holds the keystroke budget of docs/spec.md: results for 10 000 items within a
// frame. It is generous on slow CI machines (4×), and skipped under the race detector.
func TestRankBudget(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	idx := search.NewIndex(candidates(10_000))
	use := func(string) float64 { return 0 }
	const budget = 16 * time.Millisecond
	for _, q := range []string{"vsc", "windows term"} {
		best := time.Hour
		for range 5 {
			start := time.Now()
			idx.Rank(q, use)
			best = min(best, time.Since(start))
		}
		t.Logf("%q: %v for 10k items", q, best)
		if best > 4*budget {
			t.Errorf("%q took %v, over 4× the %v budget", q, best, budget)
		}
	}
}
