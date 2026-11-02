package windows

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// Layout is a set of apps placed together, as kept in Storage.
type Layout struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Hotkey  string `json:"hotkey,omitempty"`
	Windows []Slot `json:"windows"`
}

// Slot is where a layout puts a window of an app. Shares of the screen rather than pixels keep a
// layout right after the resolution or the taskbar changes.
type Slot struct {
	App string `json:"app"`
	// Title tells apart two windows of an app, while they keep their titles.
	Title     string   `json:"title,omitempty"`
	Display   string   `json:"display"`
	Share     fraction `json:"share"`
	Maximized bool     `json:"maximized,omitempty"`
}

const (
	layoutPrefix = "layout/"
	// defaultPatience is how long a layout waits for the apps it opened.
	defaultPatience = 15 * time.Second
	pollEvery       = 150 * time.Millisecond
)

func (e *Extension) layoutCommands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{
		{
			ID: "layout_create", Title: ctx.T("layout.create"), Icon: sdk.Icon{Glyph: "window.layout"},
			Keywords: []string{"layout", "save windows", "salvar janelas", "arranjo", "novo"},
			Run: func(ctx sdk.Context) error {
				slots, err := e.capture()
				if err != nil {
					return err
				}
				if len(slots) == 0 {
					return errors.New(ctx.T("layout.no_windows"))
				}
				ctx.Push(e.form(ctx, Layout{Windows: slots}))
				return nil
			},
		},
		{
			ID: "layout_manage", Title: ctx.T("layout.manage"), Icon: sdk.Icon{Glyph: "window.layout"},
			Keywords: []string{"layouts", "window layouts", "layouts de janelas", "arranjos"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.manage(ctx))
				return nil
			},
		},
	}
}

// Items implements sdk.ItemSource: every layout, in the root search.
func (e *Extension) Items(ctx sdk.Context) ([]sdk.Item, error) {
	if e.p.Target == nil {
		return nil, nil
	}
	layouts, err := Load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.Item, len(layouts))
	for i, l := range layouts {
		items[i] = e.item(ctx, l)
		items[i].Accessory = ctx.T("layout.kind")
	}
	return items, nil
}

// Load returns the layouts, by name.
func Load(ctx sdk.Context) ([]Layout, error) {
	entries, err := ctx.Storage().List(ctx, layoutPrefix)
	if err != nil {
		return nil, err
	}
	layouts := make([]Layout, 0, len(entries))
	for _, en := range entries {
		var l Layout
		if err := json.Unmarshal(en.Value, &l); err != nil {
			ctx.Log().Warn("layout not read", "key", en.Key, "err", err)
			continue
		}
		layouts = append(layouts, l)
	}
	slices.SortFunc(layouts, func(a, b Layout) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return layouts, nil
}

// save keeps a layout and has its hotkey bound.
func (e *Extension) save(ctx sdk.Context, l Layout) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := ctx.Storage().Set(ctx, layoutPrefix+l.ID, data); err != nil {
		return fmt.Errorf("%s: %w", ctx.T("layout.not_saved"), err)
	}
	e.layoutsChanged(ctx)
	return nil
}

func (e *Extension) layoutsChanged(ctx sdk.Context) {
	ctx.Refresh()
	select {
	case e.changed <- struct{}{}:
	default: // Serve has yet to read the last change, and will read this one with it
	}
}

// capture turns the windows on the screen into slots, topmost first.
func (e *Extension) capture() ([]Slot, error) {
	open, err := e.p.OpenWindows()
	if err != nil {
		return nil, err
	}
	displays, err := e.p.Displays()
	if err != nil {
		return nil, err
	}
	if len(displays) == 0 {
		return nil, nil
	}
	slots := make([]Slot, len(open))
	for i, w := range open {
		d := displayOf(displays, w.Frame)
		slots[i] = Slot{
			App: w.App, Title: w.Title, Display: d.ID, Share: fractionOf(w.Frame, d.Work), Maximized: w.Maximized,
		}
	}
	return slots, nil
}

// item is the row of a layout: Enter places its windows.
func (e *Extension) item(ctx sdk.Context, l Layout) sdk.Item {
	return sdk.Item{
		ID: l.ID, Title: l.Name, Subtitle: ctx.T("layout.windows", "count", strconv.Itoa(len(l.Windows))),
		Icon: sdk.Icon{Glyph: "window.layout"}, Keywords: []string{"layout"},
		Actions: []sdk.Action{
			{Title: ctx.T("layout.apply"), Run: func(ctx sdk.Context) error { return e.apply(ctx, l) }},
			{Title: ctx.T("layout.edit"), Shortcut: "Ctrl+E", Run: func(ctx sdk.Context) error {
				ctx.Push(e.form(ctx, l))
				return nil
			}},
			{Title: ctx.T("layout.recapture"), Shortcut: "Ctrl+R", Run: func(ctx sdk.Context) error {
				slots, err := e.capture()
				if err != nil {
					return err
				}
				if len(slots) == 0 {
					return errors.New(ctx.T("layout.no_windows"))
				}
				l.Windows = slots
				if err := e.save(ctx, l); err != nil {
					return err
				}
				ctx.Toast(ctx.T("layout.saved", "name", l.Name))
				return nil
			}},
			{
				Title: ctx.T("layout.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("layout.delete_confirm"),
				Run: func(ctx sdk.Context) error {
					if err := ctx.Storage().Delete(ctx, layoutPrefix+l.ID); err != nil {
						return err
					}
					e.layoutsChanged(ctx)
					ctx.Toast(ctx.T("layout.deleted", "name", l.Name))
					return nil
				},
			},
		},
	}
}

// manage lists the layouts, to place, edit and delete them.
func (e *Extension) manage(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title: ctx.T("layout.manage"), Placeholder: ctx.T("layout.search"),
		Empty: sdk.Empty{Title: ctx.T("layout.empty"), Description: ctx.T("layout.empty_hint"), Icon: sdk.Icon{Glyph: "window.layout"}},
		Load: func(ctx sdk.Context) ([]sdk.Item, error) {
			layouts, err := Load(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]sdk.Item, len(layouts))
			for i, l := range layouts {
				items[i] = e.item(ctx, l)
				items[i].Accessory = l.Hotkey
			}
			return items, nil
		},
	}
}

// form names a layout whose windows were captured, or edits one that has an ID.
func (e *Extension) form(ctx sdk.Context, l Layout) *sdk.Form {
	title, submit := ctx.T("layout.create"), ctx.T("layout.create_submit")
	if l.ID != "" {
		title, submit = ctx.T("layout.edit"), ctx.T("layout.edit_submit")
	}
	return &sdk.Form{
		Title: title, SubmitTitle: submit,
		Fields: []sdk.Field{
			{
				ID: "name", Title: ctx.T("layout.name"), Placeholder: ctx.T("layout.name_placeholder"), Default: l.Name, Required: true,
				Info: ctx.T("layout.windows", "count", strconv.Itoa(len(l.Windows))),
			},
			{
				ID: "hotkey", Title: ctx.T("layout.hotkey"), Placeholder: "Ctrl+Alt+1", Default: l.Hotkey, Info: ctx.T("hotkey_info"),
				Validate: func(v string) string {
					if v = strings.TrimSpace(v); v != "" && e.p.ValidHotkey != nil && !e.p.ValidHotkey(v) {
						return ctx.T("layout.bad_hotkey")
					}
					return ""
				},
			},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			l.Name, l.Hotkey = strings.TrimSpace(v["name"]), strings.TrimSpace(v["hotkey"])
			if l.ID == "" {
				l.ID = sdk.NewID()
			}
			if err := e.save(ctx, l); err != nil {
				return err
			}
			ctx.Toast(ctx.T("layout.saved", "name", l.Name))
			return nil
		},
	}
}

// apply places a layout's windows: those of apps already open first, then those of apps it opens
// as their windows appear. Windows go bottom to top, each raised as it moves, so that the order
// comes out as it was captured.
func (e *Extension) apply(ctx sdk.Context, l Layout) error {
	ctx.Hide()
	displays, err := e.p.Displays()
	if err != nil {
		return err
	}
	if len(displays) == 0 {
		return errors.New(ctx.T("no_window"))
	}
	open, err := e.p.OpenWindows()
	if err != nil {
		return err
	}
	used := map[uintptr]bool{}
	waiting := map[int]bool{}
	var top uintptr
	var failed error
	place := func(i int, w Window) {
		s := l.Windows[i]
		used[w.Handle] = true
		delete(waiting, i)
		if err := e.fill(displays, s, w); err != nil {
			failed = errors.Join(failed, fmt.Errorf("%s: %w", w.Title, err))
			return
		}
		if i == 0 {
			top = w.Handle
		}
	}
	for i := len(l.Windows) - 1; i >= 0; i-- {
		if w, ok := match(open, used, l.Windows[i]); ok {
			place(i, w)
		} else {
			waiting[i] = true
		}
	}

	// An app with several slots opens once for each, as apps that hold one window per launch need.
	for i := range waiting {
		if err := e.p.Launch(l.Windows[i].App); err != nil {
			ctx.Log().Warn("app not opened", "app", l.Windows[i].App, "err", err)
			delete(waiting, i)
			failed = errors.Join(failed, err)
		}
	}
	patience := e.p.Patience
	if patience == 0 {
		patience = defaultPatience
	}
	deadline := time.Now().Add(patience)
	for len(waiting) > 0 && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
		open, err := e.p.OpenWindows()
		if err != nil {
			continue
		}
		for i := len(l.Windows) - 1; i >= 0; i-- {
			if !waiting[i] {
				continue
			}
			if w, ok := match(open, used, l.Windows[i]); ok {
				place(i, w)
			}
		}
	}
	if top != 0 {
		e.focus(ctx, top)
	}
	if len(waiting) > 0 {
		failed = errors.Join(failed, errors.New(ctx.T("layout.not_all", "count", strconv.Itoa(len(waiting)))))
	}
	if failed != nil {
		ctx.Log().Warn("layout not placed whole", "layout", l.Name, "err", failed)
		return failed
	}
	return nil
}

// fill puts a window in its slot, on its display, or the primary one when it is unplugged.
func (e *Extension) fill(displays []Display, s Slot, w Window) error {
	d := displays[0]
	if i := slices.IndexFunc(displays, func(d Display) bool { return d.ID == s.Display }); i >= 0 {
		d = displays[i]
	}
	if err := e.p.Place(w.Handle, s.Share.in(d.Work)); err != nil {
		return err
	}
	if s.Maximized {
		return e.p.Maximize(w.Handle)
	}
	return nil
}

// match finds a free window for a slot: of its app, with its title if there is one.
func match(open []Window, used map[uintptr]bool, s Slot) (Window, bool) {
	var found Window
	ok := false
	for _, w := range open {
		if used[w.Handle] || !strings.EqualFold(w.App, s.App) {
			continue
		}
		if w.Title == s.Title {
			return w, true
		}
		if !ok {
			found, ok = w, true
		}
	}
	return found, ok
}
