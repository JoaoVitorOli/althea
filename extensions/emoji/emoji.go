// Package emoji is the Emoji & Symbols extension: every emoji and many text symbols in a grid,
// searched by name and keyword in English and Portuguese, pasted into the app the user was in.
// Its data comes from Unicode (emoji-test 16.0) and CLDR 46: see data/README.md.
package emoji

import (
	"bufio"
	"embed"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed data/emoji.tsv
var data string

//go:embed locales/*.toml
var locales embed.FS

// Emoji is an emoji or a symbol.
type Emoji struct {
	Chars string
	Group string // as Unicode names it, as "Smileys & Emotion"
	// Names and keywords by language, "en" and "pt-BR".
	Name     map[string]string
	Keywords map[string][]string
	// Tones are the five skin tone variants, light to dark, for the emoji that take them.
	Tones []string
}

// groups are the groups, in order, with the keys of their names in the catalogs.
var groups = []struct{ name, key string }{
	{"Smileys & Emotion", "smileys"},
	{"People & Body", "people"},
	{"Animals & Nature", "nature"},
	{"Food & Drink", "food"},
	{"Travel & Places", "travel"},
	{"Activities", "activities"},
	{"Objects", "objects"},
	{"Symbols", "symbols"},
	{"Flags", "flags"},
	{"Text Symbols", "text"},
}

var (
	loadOnce sync.Once
	all      []Emoji
)

// All returns every emoji and symbol, in Unicode's order, read once.
func All() []Emoji {
	loadOnce.Do(func() {
		sc := bufio.NewScanner(strings.NewReader(data))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) != 7 {
				continue
			}
			e := Emoji{
				Chars: f[0], Group: f[1],
				Name:     map[string]string{"en": f[2], "pt-BR": f[4]},
				Keywords: map[string][]string{"en": split(f[3], "|"), "pt-BR": split(f[5], "|")},
				Tones:    split(f[6], ","),
			}
			all = append(all, e)
		}
	})
	return all
}

func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

// Extension is the Emoji & Symbols extension.
type Extension struct{}

// New returns the Emoji & Symbols extension.
func New() *Extension { return &Extension{} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.emoji", Title: "Emoji & Symbols", Icon: sdk.Icon{Text: "😀"}, Messages: messages}
}

// tonePrefs are the values of the skin tone preference: none, then light to dark.
var tonePrefs = []string{"none", "light", "medium_light", "medium", "medium_dark", "dark"}

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	opts := make([]sdk.Option, len(tonePrefs))
	for i, v := range tonePrefs {
		opts[i] = sdk.Option{Value: v, Title: ctx.T("tone." + v)}
	}
	return []sdk.Preference{{Name: "tone", Title: ctx.T("prefs.tone"), Kind: sdk.ChoicePreference, Default: "none", Options: opts}}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{{
		ID: "search", Title: ctx.T("title"), Icon: sdk.Icon{Text: "😀"},
		Keywords: []string{"emoji", "emojis", "symbols", "símbolos", "smiley", "carinha", "figurinha"},
		Run: func(ctx sdk.Context) error {
			ctx.Push(e.grid(ctx))
			return nil
		},
	}}
}

// toned returns an emoji in the skin tone the user prefers, when it takes one.
func toned(ctx sdk.Context, em Emoji) string {
	i := slices.Index(tonePrefs, ctx.Preferences().String("tone"))
	if i <= 0 || len(em.Tones) != 5 {
		return em.Chars
	}
	return em.Tones[i-1]
}

// grid shows everything, the recently used first, in groups.
func (e *Extension) grid(ctx sdk.Context) *sdk.Grid {
	return &sdk.Grid{
		Title: ctx.T("title"), Placeholder: ctx.T("search"), Columns: 8,
		Load: func(ctx sdk.Context) ([]sdk.Item, error) {
			byChars := map[string]Emoji{}
			for _, em := range All() {
				byChars[em.Chars] = em
			}
			var items []sdk.Item
			for _, chars := range recent(ctx) {
				if em, ok := byChars[chars]; ok {
					it := e.item(ctx, em)
					it.ID, it.Section = "recent:"+em.Chars, ctx.T("recent")
					items = append(items, it)
				}
			}
			for _, em := range All() {
				items = append(items, e.item(ctx, em))
			}
			return items, nil
		},
	}
}

func groupName(ctx sdk.Context, group string) string {
	for _, g := range groups {
		if g.name == group {
			return ctx.T("group." + g.key)
		}
	}
	return group
}

func (e *Extension) item(ctx sdk.Context, em Emoji) sdk.Item {
	lang := ctx.Language()
	if lang != "pt-BR" {
		lang = "en"
	}
	chars := toned(ctx, em)
	// Both languages' words find it, whatever the interface's.
	keywords := append([]string{em.Name["en"], em.Name["pt-BR"]}, em.Keywords["en"]...)
	keywords = append(keywords, em.Keywords["pt-BR"]...)
	acts := []sdk.Action{
		{Title: ctx.T("action.paste"), Run: func(ctx sdk.Context) error {
			remember(ctx, em.Chars)
			return ctx.Paste(sdk.Clip{Text: chars})
		}},
		{Title: ctx.T("action.copy"), Run: func(ctx sdk.Context) error {
			remember(ctx, em.Chars)
			ctx.Toast(ctx.T("copied", "emoji", chars))
			return ctx.CopyText(chars)
		}},
	}
	if len(em.Tones) == 5 {
		acts = append(acts, sdk.Action{Title: ctx.T("action.tones"), Shortcut: "Ctrl+T", Run: func(ctx sdk.Context) error {
			ctx.Push(e.tones(ctx, em))
			return nil
		}})
	}
	return sdk.Item{
		ID: em.Chars, Title: em.Name[lang], Icon: sdk.Icon{Text: chars}, Keywords: keywords,
		Section: groupName(ctx, em.Group), Actions: acts,
	}
}

// tones offers an emoji in each skin tone.
func (e *Extension) tones(ctx sdk.Context, em Emoji) *sdk.Grid {
	var items []sdk.Item
	for i, chars := range append([]string{em.Chars}, em.Tones...) {
		items = append(items, sdk.Item{
			ID: chars, Title: ctx.T("tone." + tonePrefs[i]), Icon: sdk.Icon{Text: chars},
			Actions: []sdk.Action{
				{Title: ctx.T("action.paste"), Run: func(ctx sdk.Context) error {
					remember(ctx, em.Chars)
					return ctx.Paste(sdk.Clip{Text: chars})
				}},
				{Title: ctx.T("action.copy"), Run: func(ctx sdk.Context) error {
					ctx.Toast(ctx.T("copied", "emoji", chars))
					return ctx.CopyText(chars)
				}},
			},
		})
	}
	return &sdk.Grid{Title: em.Name["en"], Items: items, Columns: 6}
}

// maxRecent is how many emoji the recent section holds.
const maxRecent = 16

func recent(ctx sdk.Context) []string {
	data, ok, err := ctx.Storage().Get(ctx, "recent")
	if err != nil || !ok {
		return nil
	}
	var r []string
	_ = json.Unmarshal(data, &r)
	return r
}

// remember puts an emoji first among the recently used.
func remember(ctx sdk.Context, chars string) {
	r := slices.DeleteFunc(recent(ctx), func(c string) bool { return c == chars })
	r = append([]string{chars}, r...)
	data, _ := json.Marshal(r[:min(len(r), maxRecent)])
	if err := ctx.Storage().Set(ctx, "recent", data); err != nil {
		ctx.Log().Warn("recent emoji not kept", "err", err)
	}
}
