package search_test

import (
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/search"
)

func TestMatchFindsLettersInOrder(t *testing.T) {
	tests := []struct {
		query, text string
		ok          bool
		positions   []int
	}{
		{"vsc", "Visual Studio Code", true, []int{0, 7, 14}},
		{"term", "Windows Terminal", true, []int{8, 9, 10, 11}},
		{"CHROME", "Google Chrome", true, []int{7, 8, 9, 10, 11, 12}},
		{"configuracoes", "Configurações", true, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{"xyz", "Calculator", false, nil},
		{"ab", "ba", false, nil},
		{"", "Anything", true, nil},
	}
	for _, tc := range tests {
		_, positions, ok := search.Match(tc.query, tc.text)
		if ok != tc.ok || !slices.Equal(positions, tc.positions) {
			t.Errorf("Match(%q, %q) = %v, %v; want %v, %v", tc.query, tc.text, positions, ok, tc.positions, tc.ok)
		}
	}
}

// better asserts that query matches a better than b.
func better(t *testing.T, query, a, b string) {
	t.Helper()
	sa, _, okA := search.Match(query, a)
	sb, _, okB := search.Match(query, b)
	if !okA || !okB || sa <= sb {
		t.Errorf("%q: %q scores %d (%v), %q scores %d (%v); want the first higher", query, a, sa, okA, b, sb, okB)
	}
}

func TestMatchPrefersStartsWordsAndRuns(t *testing.T) {
	better(t, "code", "Code", "Visual Studio Code")            // the whole name starts with it
	better(t, "code", "Visual Studio Code", "Barcode Scanner") // a word starts with it
	better(t, "ter", "Windows Terminal", "Battery Saver")
	better(t, "chr", "Chrome", "Character Map") // a run beats letters spread out
	better(t, "vsc", "Visual Studio Code", "Visual Basic Scripts")
	better(t, "ps", "PowerShell", "Photoshop") // camel case marks a word: Power|Shell
	better(t, "note", "Notepad", "Sticky Notes")
}

func TestRankOrdersByMatchThenUse(t *testing.T) {
	items := []search.Candidate{
		{ID: "calc", Title: "Calculator"},
		{ID: "clip", Title: "Clipboard History", Alias: "cl"},
		{ID: "code", Title: "Visual Studio Code", Keywords: []string{"editor", "vscode"}},
		{ID: "cmd", Title: "Command Prompt", Keywords: []string{"terminal"}},
	}
	none := func(string) float64 { return 0 }

	ids := func(rs []search.Result) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return out
	}
	if got := ids(search.Rank(items, "cl", none)); got[0] != "clip" {
		t.Errorf("Rank(cl) = %v, want the alias first", got)
	}
	if got := ids(search.Rank(items, "editor", none)); !slices.Equal(got, []string{"code"}) {
		t.Errorf("Rank(editor) = %v, want the item with that keyword", got)
	}
	if got := ids(search.Rank(items, "zzz", none)); len(got) != 0 {
		t.Errorf("Rank(zzz) = %v, want nothing", got)
	}

	// Use lifts an item among similar matches, but never lets a non-match in.
	used := func(id string) float64 {
		if id == "cmd" {
			return 10
		}
		return 0
	}
	if got := ids(search.Rank(items, "c", used)); got[0] != "cmd" {
		t.Errorf("Rank(c) with Command Prompt used = %v, want it first", got)
	}
	if got := ids(search.Rank(items, "calc", used)); !slices.Equal(got, []string{"calc"}) {
		t.Errorf("Rank(calc) = %v, want Calculator alone", got)
	}

	// With no query, the most used come first, then by title.
	if got := ids(search.Rank(items, "", used)); !slices.Equal(got, []string{"cmd", "calc", "clip", "code"}) {
		t.Errorf("Rank(\"\") = %v", got)
	}
}

func TestRankHighlightsTheTitle(t *testing.T) {
	rs := search.Rank([]search.Candidate{{ID: "a", Title: "Visual Studio Code"}}, "vsc", func(string) float64 { return 0 })
	if len(rs) != 1 || !slices.Equal(rs[0].Positions, []int{0, 7, 14}) {
		t.Errorf("Rank = %+v", rs)
	}
}
