package snippets

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoaoVitorOli/althea/sdk"
)

// Platform is what keyword expansion needs from the host. With a nil function, expansion is
// not offered, as on a system without a keyboard hook.
type Platform struct {
	// Keys calls on with each key the user types anywhere, one at a time, until stop.
	Keys func(on func(Key)) (stop func(), err error)
	// Type presses Backspace erase times, types text, then moves the caret back characters left.
	Type func(erase int, text string, back int) error
	// InPassword reports whether the keyboard is in a password field.
	InPassword func() bool
}

// Key is a key the user typed: its text, Backspace, or, with neither, a key that ends a word,
// as Enter, Tab or an arrow.
type Key struct {
	Text      string
	Backspace bool
}

// canExpand reports whether the host gives what expansion needs.
func (p Platform) canExpand() bool { return p.Keys != nil && p.Type != nil }

// Preferences implements sdk.Configurable: expansion, which is off until the user turns it on.
func (e *Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	if !e.p.canExpand() {
		return nil
	}
	return []sdk.Preference{{
		Name: "expand", Title: ctx.T("prefs.expand"), Description: ctx.T("prefs.expand_info"),
		Kind: sdk.BoolPreference, Default: "false",
	}}
}

// Serve implements sdk.Service: while expansion is on, it watches what the user types and
// replaces a snippet's keyword, as it is typed, with the snippet. It keeps no more of the
// typing than the longest keyword, and nothing while the keyboard is in a password field.
func (e *Extension) Serve(ctx sdk.Context) error {
	if !e.p.canExpand() || !ctx.Preferences().Bool("expand") {
		return nil
	}
	x := &expander{e: e, ctx: ctx}
	stop, err := e.p.Keys(x.key)
	if err != nil {
		return err
	}
	defer stop()
	<-ctx.Done()
	return nil
}

// keywordsFor is how long the keywords read from Storage are used before reading them again,
// so that a snippet saved meanwhile expands soon after.
const keywordsFor = 2 * time.Second

// expander matches what is typed against the snippets' keywords. Its methods run one at a time.
type expander struct {
	e   *Extension
	ctx sdk.Context

	typed    []rune // the end of what was typed, at most as long as the longest keyword
	keywords map[string]Snippet
	longest  int
	readAt   time.Time
}

func (x *expander) key(k Key) {
	switch {
	case k.Backspace:
		if len(x.typed) > 0 {
			x.typed = x.typed[:len(x.typed)-1]
		}
		return
	case k.Text == "":
		x.typed = x.typed[:0]
		return
	}
	if x.e.p.InPassword != nil && x.e.p.InPassword() {
		x.typed = x.typed[:0]
		return
	}
	x.load()
	if x.longest == 0 {
		return
	}
	x.typed = append(x.typed, []rune(k.Text)...)
	if over := len(x.typed) - x.longest; over > 0 {
		x.typed = append(x.typed[:0], x.typed[over:]...)
	}
	typed := string(x.typed)
	for kw, s := range x.keywords {
		if !strings.HasSuffix(typed, kw) {
			continue
		}
		x.typed = x.typed[:0]
		text, back := x.e.Expand(x.ctx, s.Text)
		if err := x.e.p.Type(utf8.RuneCountInString(kw), text, back); err != nil {
			x.ctx.Log().Warn("snippet not expanded", "snippet", s.Name, "err", err)
		}
		return
	}
}

// load reads the keywords again when they are old.
func (x *expander) load() {
	now := x.e.Now()
	if x.keywords != nil && now.Sub(x.readAt) < keywordsFor {
		return
	}
	x.readAt = now
	snippets, err := Load(x.ctx)
	if err != nil {
		x.ctx.Log().Warn("snippets not read", "err", err)
		return
	}
	x.keywords, x.longest = map[string]Snippet{}, 0
	for _, s := range snippets {
		if s.Keyword != "" {
			x.keywords[s.Keyword] = s
			x.longest = max(x.longest, utf8.RuneCountInString(s.Keyword))
		}
	}
}
