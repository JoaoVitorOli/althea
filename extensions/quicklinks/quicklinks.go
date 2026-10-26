// Package quicklinks is the Quicklinks extension: named links to web pages, files and folders,
// found from the root search by name or alias. A link may hold {query}, asked when it opens, and
// {clipboard}, filled with the clipboard's text: "https://github.com/search?q={query}".
package quicklinks

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the Quicklinks extension.
type Extension struct{}

// New returns the Quicklinks extension.
func New() *Extension { return &Extension{} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.quicklinks", Title: "Quicklinks", Icon: sdk.Icon{Glyph: "link"}, Messages: messages}
}

// Link is a quicklink, as kept in Storage.
type Link struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Target string `json:"target"` // a URL, or a path to a file or folder
	Alias  string `json:"alias,omitempty"`
	Icon   string `json:"icon,omitempty"` // an emoji
}

const prefix = "link/"

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{
		{
			ID: "create", Title: ctx.T("create.title"), Icon: sdk.Icon{Glyph: "link"},
			Keywords: []string{"quicklink", "link", "bookmark", "atalho", "favorito", "new", "novo"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, Link{}))
				return nil
			},
		},
		{
			ID: "manage", Title: ctx.T("manage.title"), Icon: sdk.Icon{Glyph: "link"},
			Keywords: []string{"quicklinks", "links", "bookmarks", "atalhos", "favoritos"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.manage(ctx))
				return nil
			},
		},
	}
}

// Items implements sdk.ItemSource: every link, in the root search.
func (e *Extension) Items(ctx sdk.Context) ([]sdk.Item, error) {
	links, err := Load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.Item, len(links))
	for i, l := range links {
		items[i] = e.item(ctx, l)
		items[i].Accessory = ctx.T("kind")
	}
	return items, nil
}

// Load returns the links, by name.
func Load(ctx sdk.Context) ([]Link, error) {
	entries, err := ctx.Storage().List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	links := make([]Link, 0, len(entries))
	for _, en := range entries {
		var l Link
		if err := json.Unmarshal(en.Value, &l); err != nil {
			ctx.Log().Warn("quicklink not read", "key", en.Key, "err", err)
			continue
		}
		links = append(links, l)
	}
	slices.SortFunc(links, func(a, b Link) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return links, nil
}

func save(ctx sdk.Context, l Link) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return ctx.Storage().Set(ctx, prefix+l.ID, data)
}

// item is the row of a link: Enter opens it, asking for {query} first.
func (e *Extension) item(ctx sdk.Context, l Link) sdk.Item {
	icon := sdk.Icon{Glyph: "link"}
	if l.Icon != "" {
		icon.Text = l.Icon
	} else if !isURL(l.Target) && !strings.Contains(l.Target, "{") {
		icon = sdk.Icon{File: l.Target, Glyph: "link"}
	}
	var keywords []string
	if l.Alias != "" {
		keywords = append(keywords, l.Alias)
	}
	return sdk.Item{
		ID: l.ID, Title: l.Name, Subtitle: l.Target, Icon: icon, Keywords: keywords,
		Actions: []sdk.Action{
			{Title: ctx.T("action.open"), Run: func(ctx sdk.Context) error { return e.open(ctx, l) }},
			{Title: ctx.T("action.copy"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
				ctx.Toast(ctx.T("copied"))
				return ctx.CopyText(l.Target)
			}},
			{Title: ctx.T("action.edit"), Shortcut: "Ctrl+E", Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, l))
				return nil
			}},
			{
				Title: ctx.T("action.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("action.delete_confirm"),
				Run: func(ctx sdk.Context) error {
					if err := ctx.Storage().Delete(ctx, prefix+l.ID); err != nil {
						return err
					}
					ctx.Refresh()
					ctx.Toast(ctx.T("deleted", "name", l.Name))
					return nil
				},
			},
		},
	}
}

// open opens a link, asking for the query in a form when it has {query}.
func (e *Extension) open(ctx sdk.Context, l Link) error {
	if !strings.Contains(l.Target, "{query}") {
		return launch(ctx, Expand(l.Target, "", ctx.ClipboardText()))
	}
	ctx.Push(&sdk.Form{
		Title: l.Name, SubmitTitle: ctx.T("action.open"),
		Fields: []sdk.Field{{ID: "query", Title: ctx.T("query"), Required: true}},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			return launch(ctx, Expand(l.Target, v["query"], ctx.ClipboardText()))
		},
	})
	return nil
}

func launch(ctx sdk.Context, target string) error {
	ctx.Hide()
	return ctx.Open(target)
}

// Expand fills the placeholders of a link: {query} and {clipboard}, encoded for a URL when
// the link is one.
func Expand(target, query, clipboard string) string {
	enc := func(s string) string { return s }
	if isURL(target) {
		enc = url.QueryEscape
	}
	return strings.NewReplacer("{query}", enc(query), "{clipboard}", enc(clipboard)).Replace(target)
}

func isURL(s string) bool {
	scheme, _, ok := strings.Cut(s, "://")
	return ok && scheme != "" && !strings.ContainsAny(scheme, `\/ `)
}

// manage lists the links, to open, edit and delete them.
func (e *Extension) manage(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title: ctx.T("manage.title"), Placeholder: ctx.T("manage.search"),
		Empty: sdk.Empty{Title: ctx.T("manage.empty"), Description: ctx.T("manage.empty_hint"), Icon: sdk.Icon{Glyph: "link"}},
		Load: func(ctx sdk.Context) ([]sdk.Item, error) {
			links, err := Load(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]sdk.Item, len(links))
			for i, l := range links {
				items[i] = e.item(ctx, l)
				items[i].Accessory = l.Alias
			}
			return items, nil
		},
	}
}

// form creates a link, or edits l when it has an ID.
func (e *Extension) form(ctx sdk.Context, l Link) *sdk.Form {
	title, submit := ctx.T("create.title"), ctx.T("create.submit")
	if l.ID != "" {
		title, submit = ctx.T("edit.title"), ctx.T("edit.submit")
	}
	return &sdk.Form{
		Title: title, SubmitTitle: submit,
		Fields: []sdk.Field{
			{ID: "name", Title: ctx.T("field.name"), Placeholder: "GitHub", Default: l.Name, Required: true},
			{
				ID: "target", Title: ctx.T("field.target"), Placeholder: "https://github.com/search?q={query}",
				Default: l.Target, Required: true, Info: ctx.T("field.target_info"),
				Validate: func(v string) string {
					if strings.ContainsAny(v, "\r\n") {
						return ctx.T("field.one_line")
					}
					return ""
				},
			},
			{
				ID: "alias", Title: ctx.T("field.alias"), Placeholder: "gh", Default: l.Alias, Info: ctx.T("field.alias_info"),
				Validate: func(v string) string {
					if strings.ContainsAny(v, " \t") {
						return ctx.T("field.no_spaces")
					}
					return ""
				},
			},
			{
				ID: "icon", Title: ctx.T("field.icon"), Placeholder: "🐙", Default: l.Icon, Info: ctx.T("field.icon_info"),
				Validate: func(v string) string {
					if utf8.RuneCountInString(v) > 4 {
						return ctx.T("field.icon_short")
					}
					return ""
				},
			},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			n := Link{
				ID: l.ID, Name: strings.TrimSpace(v["name"]), Target: strings.TrimSpace(v["target"]),
				Alias: strings.TrimSpace(v["alias"]), Icon: strings.TrimSpace(v["icon"]),
			}
			if n.ID == "" {
				n.ID = sdk.NewID()
			}
			if err := e.checkAlias(ctx, n); err != nil {
				return err
			}
			if err := save(ctx, n); err != nil {
				return fmt.Errorf("%s: %w", ctx.T("not_saved"), err)
			}
			ctx.Refresh()
			ctx.Toast(ctx.T("saved", "name", n.Name))
			return nil
		},
	}
}

// checkAlias refuses an alias another link already has.
func (e *Extension) checkAlias(ctx sdk.Context, n Link) error {
	if n.Alias == "" {
		return nil
	}
	links, err := Load(ctx)
	if err != nil {
		return err
	}
	for _, o := range links {
		if o.ID != n.ID && strings.EqualFold(o.Alias, n.Alias) {
			return errors.New(ctx.T("alias_taken", "alias", n.Alias, "name", o.Name))
		}
	}
	return nil
}
