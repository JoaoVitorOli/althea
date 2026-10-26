// Package example is the template for new extensions: a small to-do list that shows a List, a
// Detail and a Form, keeps its items in Storage, reads a preference and speaks English and
// Portuguese. It is built and tested with the rest but not in extensions.Builtin. Copy the
// folder to start an extension; docs/extensions.md walks through it.
package example

import (
	"cmp"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the to-do list.
type Extension struct {
	// Now is the clock, a field so that tests can set it.
	Now func() time.Time
}

// New returns the extension.
func New() *Extension { return &Extension{Now: time.Now} }

// Manifest implements sdk.Extension. The ID never changes once people use the extension: it
// names its storage, its settings and its messages.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.example", Title: "To-do", Icon: sdk.Icon{Text: "✅"}, Messages: messages}
}

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	return []sdk.Preference{{
		Name: "order", Title: ctx.T("prefs.order"), Kind: sdk.ChoicePreference, Default: "newest",
		Options: []sdk.Option{{Value: "newest", Title: ctx.T("prefs.newest")}, {Value: "oldest", Title: ctx.T("prefs.oldest")}},
	}}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{
		{
			ID: "list", Title: ctx.T("list.title"), Icon: sdk.Icon{Text: "✅"},
			Keywords: []string{"todo", "tasks", "tarefas"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.list(ctx))
				return nil
			},
		},
		{
			ID: "add", Title: ctx.T("add.title"), Icon: sdk.Icon{Text: "✅"},
			Keywords: []string{"todo", "new", "nova", "tarefa"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.addForm(ctx))
				return nil
			},
		},
	}
}

// todo is an item of the list, as kept in Storage.
type todo struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Notes string    `json:"notes"`
	Done  bool      `json:"done"`
	Added time.Time `json:"added"`
}

// The keys of Storage: one per to-do, under a prefix to list them by.
const prefix = "todo/"

func load(ctx sdk.Context) ([]todo, error) {
	entries, err := ctx.Storage().List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	todos := make([]todo, 0, len(entries))
	for _, e := range entries {
		var t todo
		if err := json.Unmarshal(e.Value, &t); err != nil {
			ctx.Log().Warn("to-do not read", "key", e.Key, "err", err)
			continue
		}
		todos = append(todos, t)
	}
	newest := ctx.Preferences().String("order") != "oldest"
	slices.SortFunc(todos, func(a, b todo) int {
		if newest {
			return b.Added.Compare(a.Added)
		}
		return a.Added.Compare(b.Added)
	})
	// Open ones first.
	slices.SortStableFunc(todos, func(a, b todo) int { return cmp.Compare(btoi(a.Done), btoi(b.Done)) })
	return todos, nil
}

func save(ctx sdk.Context, t todo) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return ctx.Storage().Set(ctx, prefix+t.ID, data)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// list shows the to-dos, loaded as the view opens: the host shows that it is loading meanwhile
// and filters them as the user types.
func (e *Extension) list(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title:       ctx.T("list.title"),
		Placeholder: ctx.T("list.search"),
		Empty:       sdk.Empty{Title: ctx.T("list.empty"), Description: ctx.T("list.empty_hint"), Icon: sdk.Icon{Text: "✅"}},
		Load: func(ctx sdk.Context) ([]sdk.Item, error) {
			todos, err := load(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]sdk.Item, len(todos))
			for i, t := range todos {
				items[i] = e.item(ctx, t)
			}
			return items, nil
		},
	}
}

// item is the row of a to-do. Its actions: Enter marks it done, Ctrl+Enter shows it, and the
// action panel (Ctrl+K) has the rest, each with its shortcut.
func (e *Extension) item(ctx sdk.Context, t todo) sdk.Item {
	section, icon := ctx.T("list.open"), "⬜"
	if t.Done {
		section, icon = ctx.T("list.done"), "✅"
	}
	toggle := ctx.T("action.done")
	if t.Done {
		toggle = ctx.T("action.undone")
	}
	return sdk.Item{
		ID: t.ID, Title: t.Title, Subtitle: t.Notes, Section: section, Icon: sdk.Icon{Text: icon},
		Actions: []sdk.Action{
			{Title: toggle, Run: func(ctx sdk.Context) error {
				t.Done = !t.Done
				if err := save(ctx, t); err != nil {
					return err
				}
				return e.reopen(ctx)
			}},
			{Title: ctx.T("action.show"), Run: func(ctx sdk.Context) error {
				ctx.Push(e.detail(ctx, t))
				return nil
			}},
			{Title: ctx.T("add.title"), Shortcut: "Ctrl+N", Run: func(ctx sdk.Context) error {
				ctx.Push(e.addForm(ctx))
				return nil
			}},
			{
				Title: ctx.T("action.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("action.delete_confirm"),
				Run: func(ctx sdk.Context) error {
					if err := ctx.Storage().Delete(ctx, prefix+t.ID); err != nil {
						return err
					}
					return e.reopen(ctx)
				},
			},
		},
	}
}

// reopen shows the list anew, after a change to it.
func (e *Extension) reopen(ctx sdk.Context) error {
	ctx.Pop()
	ctx.Push(e.list(ctx))
	return nil
}

// detail shows a to-do with its notes.
func (e *Extension) detail(ctx sdk.Context, t todo) *sdk.Detail {
	md := "# " + t.Title
	if t.Notes != "" {
		md += "\n\n" + t.Notes
	}
	state := ctx.T("detail.open")
	if t.Done {
		state = ctx.T("detail.done")
	}
	return &sdk.Detail{
		Title: t.Title, Markdown: md,
		Metadata: []sdk.Metadata{
			{Label: ctx.T("detail.state"), Text: state},
			{Label: ctx.T("detail.added"), Text: t.Added.Format("2006-01-02 15:04")},
		},
		Actions: []sdk.Action{{Title: ctx.T("action.copy"), Run: func(ctx sdk.Context) error {
			ctx.Toast(ctx.T("copied"))
			return ctx.CopyText(t.Title)
		}}},
	}
}

// addForm asks for a new to-do. Submit runs once the title is filled; returning nil closes the
// form, and an error keeps it open, shown as a toast.
func (e *Extension) addForm(ctx sdk.Context) *sdk.Form {
	return &sdk.Form{
		Title: ctx.T("add.title"), SubmitTitle: ctx.T("add.submit"),
		Fields: []sdk.Field{
			{
				ID: "title", Title: ctx.T("add.name"), Placeholder: ctx.T("add.name_placeholder"), Required: true,
				Validate: func(v string) string {
					if len([]rune(v)) > 120 {
						return ctx.T("add.too_long")
					}
					return ""
				},
			},
			{ID: "notes", Title: ctx.T("add.notes"), Kind: sdk.TextArea},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			now := e.Now()
			t := todo{ID: sdk.NewID(), Title: v["title"], Notes: v["notes"], Added: now}
			if err := save(ctx, t); err != nil {
				return fmt.Errorf("%s: %w", ctx.T("add.not_saved"), err)
			}
			ctx.Toast(ctx.T("add.saved", "title", t.Title))
			return nil
		},
	}
}
