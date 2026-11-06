package notes

import (
	"strings"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// list is the Search Notes view: every note, its text beside it, the one shown last first.
func (e *Extension) list(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title:       ctx.T("search"),
		Placeholder: ctx.T("search_placeholder"),
		ShowDetail:  true,
		Empty:       sdk.Empty{Title: ctx.T("empty"), Description: ctx.T("empty_hint"), Icon: sdk.Icon{Glyph: "note"}},
		Search: func(ctx sdk.Context, query string) ([]sdk.Item, error) {
			notes, err := e.Notes(ctx)
			if err != nil {
				return nil, err
			}
			query = strings.ToLower(strings.TrimSpace(query))
			var items []sdk.Item
			for _, n := range notes {
				if query != "" && !strings.Contains(strings.ToLower(n.Text), query) {
					continue
				}
				items = append(items, e.item(ctx, n))
			}
			return items, nil
		},
	}
}

// item is a note as a row: its title, when it changed, and what can be done with it.
func (e *Extension) item(ctx sdk.Context, n Note) sdk.Item {
	title := n.Title()
	if title == "" {
		title = ctx.T("untitled")
	}
	id := n.ID
	return sdk.Item{
		ID: id, Title: title, Icon: sdk.Icon{Glyph: "note"}, Accessory: e.when(ctx, n.Updated),
		Detail: &sdk.Detail{
			Markdown: n.Text,
			Metadata: []sdk.Metadata{
				{Label: ctx.T("created"), Text: e.when(ctx, n.Created)},
				{Label: ctx.T("updated"), Text: e.when(ctx, n.Updated)},
			},
		},
		Actions: []sdk.Action{
			{Title: ctx.T("open_note"), Icon: sdk.Icon{Glyph: "note"}, Run: func(ctx sdk.Context) error {
				e.show(ctx, id)
				return nil
			}},
			{Title: ctx.T("copy"), Icon: sdk.Icon{Glyph: "copy"}, Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
				if err := ctx.CopyText(n.Text); err != nil {
					return err
				}
				ctx.Toast(ctx.T("copied"))
				return nil
			}},
			{
				Title: ctx.T("delete"), Icon: sdk.Icon{Glyph: "trash"}, Shortcut: "Ctrl+Shift+X", Confirm: ctx.T("delete_confirm"),
				Run: func(ctx sdk.Context) error {
					if err := e.deleted(ctx, id); err != nil {
						return err
					}
					ctx.Toast(ctx.T("deleted"))
					ctx.Refresh()
					return nil
				},
			},
		},
	}
}

// when writes a time as the user's language does, the time alone for today.
func (e *Extension) when(ctx sdk.Context, t time.Time) string {
	now := e.Now()
	pt := ctx.Language() == "pt-BR"
	switch {
	case sameDay(t, now):
		if pt {
			return t.Format("15:04")
		}
		return t.Format("3:04 PM")
	case t.Year() == now.Year():
		if pt {
			return t.Format("02/01")
		}
		return t.Format("Jan 2")
	case pt:
		return t.Format("02/01/2006")
	default:
		return t.Format("Jan 2, 2006")
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
