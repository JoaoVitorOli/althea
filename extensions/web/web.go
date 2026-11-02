// Package web is the Web Search extension: when the root search finds nothing, it offers to look
// for the query on the web, in the browser, with the search engine the user chose.
package web

import (
	"embed"
	"io/fs"
	"net/url"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the Web Search extension.
type Extension struct{}

// New returns the Web Search extension.
func New() *Extension { return &Extension{} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.web", Title: "Web Search", Icon: sdk.Icon{Glyph: "search"}, Messages: messages}
}

// Commands implements sdk.Extension: the extension has none, only its fallback.
func (*Extension) Commands(sdk.Context) []sdk.Command { return nil }

// engine is a search engine; its address ends where the query goes.
type engine struct {
	id, title, address string
}

// engines are the engines to choose from, the default first.
var engines = []engine{
	{"duckduckgo", "DuckDuckGo", "https://duckduckgo.com/?q="},
	{"google", "Google", "https://www.google.com/search?q="},
	{"bing", "Bing", "https://www.bing.com/search?q="},
}

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	options := make([]sdk.Option, len(engines))
	for i, e := range engines {
		options[i] = sdk.Option{Value: e.id, Title: e.title}
	}
	return []sdk.Preference{{
		Name: "engine", Title: ctx.T("engine"), Kind: sdk.ChoicePreference, Default: engines[0].id, Options: options,
	}}
}

// chosen returns the engine the user chose.
func chosen(ctx sdk.Context) engine {
	id := ctx.Preferences().String("engine")
	for _, e := range engines {
		if e.id == id {
			return e
		}
	}
	return engines[0]
}

// Fallbacks implements sdk.FallbackSource.
func (*Extension) Fallbacks(ctx sdk.Context, query string) []sdk.Item {
	e := chosen(ctx)
	return []sdk.Item{{
		ID: "search", Title: ctx.T("search_for", "query", query), Subtitle: e.title, Icon: sdk.Icon{Glyph: "search"},
		Actions: []sdk.Action{{Title: ctx.T("open"), Run: func(ctx sdk.Context) error {
			ctx.Hide()
			return ctx.Open(e.address + url.QueryEscape(query))
		}}},
	}}
}
