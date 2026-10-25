// Package apps is the Applications extension: every app installed on the computer, Win32 and
// Store alike, in the root search.
package apps

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// App is an installed application.
type App struct {
	Name string
	ID   string // stable across launches, for ranking by use
	Path string // where the system finds it, and its icon
}

// Platform is how the extension reaches the system's apps; the host provides it.
type Platform struct {
	List   func() ([]App, error)
	Launch func(App) error
}

// Extension lists and launches applications.
type Extension struct{ p Platform }

// New returns the Applications extension.
func New(p Platform) *Extension { return &Extension{p: p} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.apps", Title: "Applications", Icon: sdk.Icon{Glyph: "app"}, Messages: messages}
}

// Commands implements sdk.Extension: the apps themselves are items.
func (*Extension) Commands(sdk.Context) []sdk.Command { return nil }

// Items implements sdk.ItemSource.
func (e *Extension) Items(ctx sdk.Context) ([]sdk.Item, error) {
	if e.p.List == nil {
		return nil, nil
	}
	list, err := e.p.List()
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	kind := ctx.T("kind")
	items := make([]sdk.Item, 0, len(list))
	for _, a := range list {
		items = append(items, sdk.Item{
			ID:        a.ID,
			Title:     a.Name,
			Icon:      sdk.Icon{File: a.Path},
			Accessory: kind,
			Actions: []sdk.Action{
				{Title: ctx.T("open"), Run: e.launch(a)},
				{Title: ctx.T("copy_name"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
					return ctx.CopyText(a.Name)
				}},
			},
		})
	}
	return items, nil
}

func (e *Extension) launch(a App) func(sdk.Context) error {
	return func(ctx sdk.Context) error {
		if err := e.p.Launch(a); err != nil {
			return fmt.Errorf("open %s: %w", a.Name, err)
		}
		ctx.Hide()
		return nil
	}
}
