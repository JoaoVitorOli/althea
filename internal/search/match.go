// Package search finds what the user types: a fuzzy match that wants the query's letters in
// order and scores how naturally they fall (the start of the text, the start of words, runs of
// letters), and a ranking that adds how much each item is used.
package search

import (
	"math"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Scores of a match, tuned so that, for the same letters, a match at the start of the text
// beats one at the start of a word, which beats a camel-case hump, which beats one inside a
// word; and runs of letters beat letters spread out.
const (
	scoreMatch       = 16
	bonusFirst       = 40 // the text's first letter
	bonusBoundary    = 24 // after a space, dash, underscore, dot or slash
	bonusCamel       = 20 // a capital after a lower-case letter: Power|Shell
	bonusConsecutive = 16 // right after the letter matched before
	gapStart         = -6
	gapExtension     = -2
	leadingMax       = 8 // letters skipped before the first match cost one each, up to this
)

const none = math.MinInt32 / 2

// Match scores how well query matches text: the query's letters must appear in the text in
// order, ignoring case and accents. Positions are the indexes, in runes, of the letters
// matched, for highlighting. An empty query matches anything, with nothing to highlight.
func Match(query, text string) (score int, positions []int, ok bool) {
	q := fold(query)
	if len(q) == 0 {
		return 0, nil, true
	}
	var m matcher
	return m.match(q, []rune(text), fold(text), true)
}

// matcher matches with buffers it reuses from one text to the next, so that ranking many
// candidates allocates only for the matches it returns.
type matcher struct {
	score, from []int32
}

// subsequence reports whether q's runes appear in t in order: the cheap test that rejects
// most candidates before the full match.
func subsequence(q, t []rune) bool {
	i := 0
	for _, r := range t {
		if r != q[i] {
			continue
		}
		i++
		if i == len(q) {
			return true
		}
	}
	return false
}

// match aligns folded query q with text t (raw as written, for word boundaries) at the best
// score. Positions are returned only when wanted.
func (mt *matcher) match(q, raw, t []rune, wantPositions bool) (int, []int, bool) {
	m, n := len(q), len(t)
	if m > n || !subsequence(q, t) {
		return 0, nil, false
	}
	if cap(mt.score) < m*n {
		mt.score, mt.from = make([]int32, m*n), make([]int32, m*n)
	}
	// score[i*n+j]: the best score with q[i] matched at t[j]; from[i*n+j]: where q[i-1] was.
	score, from := mt.score[:m*n], mt.from[:m*n]
	for i := range m {
		// gap and gapAt: the best way to reach t[j] after a gap of at least one letter.
		gap, gapAt := int32(none), int32(-1)
		for j := range n {
			if i > 0 && j >= 2 {
				if s := score[(i-1)*n+j-2]; s > none && s+gapStart > gap+gapExtension {
					gap, gapAt = s+gapStart, int32(j-2)
				} else {
					gap += gapExtension
				}
			}
			score[i*n+j], from[i*n+j] = none, -1
			if t[j] != q[i] {
				continue
			}
			base := scoreMatch + bonus(raw, j)
			if i == 0 {
				score[i*n+j] = base - int32(min(j, leadingMax))
				continue
			}
			if j > 0 && score[(i-1)*n+j-1] > none {
				score[i*n+j], from[i*n+j] = score[(i-1)*n+j-1]+base+bonusConsecutive, int32(j-1)
			}
			if gap > none && gap+base > score[i*n+j] {
				score[i*n+j], from[i*n+j] = gap+base, gapAt
			}
		}
	}

	best, at := int32(none), -1
	for j := range n {
		if score[(m-1)*n+j] > best {
			best, at = score[(m-1)*n+j], j
		}
	}
	if at < 0 {
		return 0, nil, false
	}
	if !wantPositions {
		return int(best), nil, true
	}
	positions := make([]int, m)
	for i := m - 1; i >= 0; i-- {
		positions[i] = at
		at = int(from[i*n+at])
	}
	return int(best), positions, true
}

// bonus is what matching the j-th letter of text earns by where it is.
func bonus(text []rune, j int) int32 {
	if j == 0 {
		return bonusFirst
	}
	prev, cur := text[j-1], text[j]
	switch {
	case isSeparator(prev):
		return bonusBoundary
	case unicode.IsLower(prev) && unicode.IsUpper(cur):
		return bonusCamel
	case unicode.IsLetter(prev) && unicode.IsDigit(cur):
		return bonusCamel
	}
	return 0
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '-', '_', '.', '/', '\\', '(', ')', '[', ']', ':', '+', '&':
		return true
	}
	return false
}

// Folded returns text as the search compares it, so that others can tell whether two aliases are
// the same to it.
func Folded(s string) string { return string(fold(s)) }

// fold lowers each rune and strips its accents, one rune for one, so that positions in the
// folded text are positions in the original.
func fold(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, foldRune(r))
	}
	return out
}

func foldRune(r rune) rune {
	r = unicode.ToLower(r)
	if r < 0x80 {
		return r
	}
	if d := []rune(norm.NFD.String(string(r))); len(d) > 0 && d[0] < 0x80 {
		return d[0]
	}
	return r
}
