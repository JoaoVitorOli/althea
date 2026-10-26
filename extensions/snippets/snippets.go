// Package snippets is the Snippets extension: texts the user pastes often, as an address or a
// signature, searched by name and pasted into the app they were in. Placeholders fill in as
// the snippet pastes: {clipboard}, {date}, {time}, {datetime}, and {cursor}, where the caret
// ends up.
package snippets

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the Snippets extension.
type Extension struct {
	// Now is the clock, for {date} and {time}.
	Now func() time.Time
}

// New returns the Snippets extension.
func New() *Extension { return &Extension{Now: time.Now} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.snippets", Title: "Snippets", Icon: sdk.Icon{Glyph: "document"}, Messages: messages}
}

// Snippet is a text kept to paste, as kept in Storage.
type Snippet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
	// Keyword, typed anywhere, expands to the snippet when expansion is on.
	Keyword string `json:"keyword,omitempty"`
}

const prefix = "snippet/"

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{
		{
			ID: "search", Title: ctx.T("search.title"), Icon: sdk.Icon{Glyph: "document"},
			Keywords: []string{"snippets", "text", "paste", "trechos", "textos", "colar"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.list(ctx))
				return nil
			},
		},
		{
			ID: "create", Title: ctx.T("create.title"), Icon: sdk.Icon{Glyph: "document"},
			Keywords: []string{"snippet", "new", "novo", "trecho"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, Snippet{}))
				return nil
			},
		},
	}
}

// Load returns the snippets, by name.
func Load(ctx sdk.Context) ([]Snippet, error) {
	entries, err := ctx.Storage().List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]Snippet, 0, len(entries))
	for _, en := range entries {
		var s Snippet
		if err := json.Unmarshal(en.Value, &s); err != nil {
			ctx.Log().Warn("snippet not read", "key", en.Key, "err", err)
			continue
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Snippet) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

func save(ctx sdk.Context, s Snippet) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return ctx.Storage().Set(ctx, prefix+s.ID, data)
}

// list shows the snippets with a preview of the chosen one.
func (e *Extension) list(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title: ctx.T("search.title"), Placeholder: ctx.T("search.placeholder"), ShowDetail: true,
		Empty: sdk.Empty{Title: ctx.T("search.empty"), Description: ctx.T("search.empty_hint"), Icon: sdk.Icon{Glyph: "document"}},
		Load: func(ctx sdk.Context) ([]sdk.Item, error) {
			snippets, err := Load(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]sdk.Item, len(snippets))
			for i, s := range snippets {
				items[i] = e.item(ctx, s)
			}
			return items, nil
		},
	}
}

func (e *Extension) item(ctx sdk.Context, s Snippet) sdk.Item {
	meta := []sdk.Metadata{{Label: ctx.T("detail.characters"), Text: fmt.Sprint(utf8.RuneCountInString(s.Text))}}
	if s.Keyword != "" {
		meta = append([]sdk.Metadata{{Label: ctx.T("detail.keyword"), Text: s.Keyword}}, meta...)
	}
	return sdk.Item{
		ID: s.ID, Title: s.Name, Icon: sdk.Icon{Glyph: "document"}, Accessory: s.Keyword,
		Keywords: slices.DeleteFunc([]string{s.Keyword, firstLine(s.Text)}, func(k string) bool { return k == "" }),
		Detail:   &sdk.Detail{Markdown: fence(s.Text), Metadata: meta},
		Actions: []sdk.Action{
			{Title: ctx.T("action.paste"), Run: func(ctx sdk.Context) error {
				text, back := e.Expand(ctx, s.Text)
				return ctx.Paste(sdk.Clip{Text: text, CaretBack: back})
			}},
			{Title: ctx.T("action.copy"), Run: func(ctx sdk.Context) error {
				text, _ := e.Expand(ctx, s.Text)
				ctx.Toast(ctx.T("copied"))
				return ctx.CopyText(text)
			}},
			{Title: ctx.T("action.edit"), Shortcut: "Ctrl+E", Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, s))
				return nil
			}},
			{Title: ctx.T("create.title"), Shortcut: "Ctrl+N", Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, Snippet{}))
				return nil
			}},
			{
				Title: ctx.T("action.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("action.delete_confirm"),
				Run: func(ctx sdk.Context) error {
					if err := ctx.Storage().Delete(ctx, prefix+s.ID); err != nil {
						return err
					}
					ctx.Pop()
					ctx.Push(e.list(ctx))
					return nil
				},
			},
		},
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// fence shows text as it is, in a block of code.
func fence(text string) string {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}
	return f + "\n" + text + "\n" + f
}

// Expand fills a snippet's placeholders, and says how many characters follow {cursor}, for
// the caret to go back over; only the first {cursor} counts.
func (e *Extension) Expand(ctx sdk.Context, text string) (string, int) {
	now := e.Now()
	date, clock := "01/02/2006", "3:04 PM"
	if ctx.Language() == "pt-BR" {
		date, clock = "02/01/2006", "15:04"
	}
	var clipboard string
	if strings.Contains(text, "{clipboard}") {
		clipboard = ctx.ClipboardText()
	}
	text = strings.NewReplacer(
		"{clipboard}", clipboard,
		"{datetime}", now.Format(date+" "+clock),
		"{date}", now.Format(date),
		"{time}", now.Format(clock),
	).Replace(text)
	before, after, ok := strings.Cut(text, "{cursor}")
	if !ok {
		return text, 0
	}
	after = strings.ReplaceAll(after, "{cursor}", "")
	// Windows text fields count a line break as one caret step.
	return before + after, utf8.RuneCountInString(strings.ReplaceAll(after, "\r\n", "\n"))
}

// form creates a snippet, or edits s when it has an ID.
func (e *Extension) form(ctx sdk.Context, s Snippet) *sdk.Form {
	title, submit := ctx.T("create.title"), ctx.T("create.submit")
	if s.ID != "" {
		title, submit = ctx.T("edit.title"), ctx.T("edit.submit")
	}
	return &sdk.Form{
		Title: title, SubmitTitle: submit,
		Fields: []sdk.Field{
			{ID: "name", Title: ctx.T("field.name"), Placeholder: ctx.T("field.name_placeholder"), Default: s.Name, Required: true},
			{ID: "text", Title: ctx.T("field.text"), Kind: sdk.TextArea, Default: s.Text, Required: true, Info: ctx.T("field.text_info")},
			{
				ID: "keyword", Title: ctx.T("field.keyword"), Placeholder: ";addr", Default: s.Keyword, Info: ctx.T("field.keyword_info"),
				Validate: func(v string) string {
					if strings.ContainsAny(v, " \t\r\n") {
						return ctx.T("field.no_spaces")
					}
					return ""
				},
			},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			n := Snippet{ID: s.ID, Name: strings.TrimSpace(v["name"]), Text: v["text"], Keyword: strings.TrimSpace(v["keyword"])}
			if n.ID == "" {
				n.ID = sdk.NewID()
			}
			if err := checkKeyword(ctx, n); err != nil {
				return err
			}
			if err := save(ctx, n); err != nil {
				return fmt.Errorf("%s: %w", ctx.T("not_saved"), err)
			}
			ctx.Toast(ctx.T("saved", "name", n.Name))
			return nil
		},
	}
}

// checkKeyword refuses a keyword another snippet has, or one that starts another's, which
// would expand before the user finished typing the longer one.
func checkKeyword(ctx sdk.Context, n Snippet) error {
	if n.Keyword == "" {
		return nil
	}
	snippets, err := Load(ctx)
	if err != nil {
		return err
	}
	for _, o := range snippets {
		if o.ID == n.ID || o.Keyword == "" {
			continue
		}
		if strings.HasPrefix(o.Keyword, n.Keyword) || strings.HasPrefix(n.Keyword, o.Keyword) {
			return errors.New(ctx.T("keyword_taken", "keyword", o.Keyword, "name", o.Name))
		}
	}
	return nil
}
