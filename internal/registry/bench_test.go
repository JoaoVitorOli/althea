package registry_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// many is an item source of n items with names like those of apps and files.
func many(n int) sdk.Extension {
	names := []string{
		"Visual Studio Code", "Google Chrome", "Windows Terminal", "Spotify", "Discord",
		"Notepad", "Calculator", "Settings", "Steam", "Obsidian", "Figma", "Docker Desktop",
	}
	return source{&fake{id: "many", items: func() ([]sdk.Item, error) {
		items := make([]sdk.Item, n)
		for i := range items {
			items[i] = sdk.Item{ID: fmt.Sprint(i), Title: fmt.Sprintf("%s %d", names[i%len(names)], i)}
		}
		return items, nil
	}}}
}

var calc = answerer{&fake{id: "calc", answer: func(context.Context, string) (*sdk.Item, error) { return nil, nil }}}

func BenchmarkQuery10k(b *testing.B) {
	r := newRegistry(&testing.T{}, many(10_000), calc, system)
	for _, q := range []string{"c", "vsc", "windows term"} {
		b.Run(q, func(b *testing.B) {
			for b.Loop() {
				r.Query(context.Background(), q)
			}
		})
	}
}

// TestQueryBudget holds docs/spec.md's keystroke budget end to end in the registry: answers
// asked, 10 000 items ranked, results built, within a frame (4× on slow CI machines).
func TestQueryBudget(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	r := newRegistry(t, many(10_000), calc, system)
	const budget = 16 * time.Millisecond
	for _, q := range []string{"c", "vsc", "windows term"} {
		best := time.Hour
		for range 5 {
			start := time.Now()
			r.Query(context.Background(), q)
			best = min(best, time.Since(start))
		}
		t.Logf("%q: %v", q, best)
		if best > 4*budget {
			t.Errorf("%q took %v, over 4× the %v budget", q, best, budget)
		}
	}
}
