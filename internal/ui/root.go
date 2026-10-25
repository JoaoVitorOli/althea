// Package ui holds Althea's views, drawn with MyGo's native UI toolkit.
package ui

import (
	"embed"
	"io/fs"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/internal/theme"
)

//go:embed locales/*.toml
var locales embed.FS

// Messages are the catalogs of the host's own text, for i18n.Catalog.Add.
var Messages, _ = fs.Sub(locales, "locales")

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>`))

// Root is the launcher's root view: the search bar, the results and the footer. Its state is
// read and changed on the main thread.
type Root struct {
	// T translates a message key, as i18n.Catalog.T does.
	T func(key string, args ...any) string
	// Hotkey is the shortcut that shows the launcher, for the footer.
	Hotkey string
	// Onboarding, while set, shows in place of the search.
	Onboarding *Onboarding
	// OnQuery is called as the search changes; the host answers with SetResults.
	OnQuery func(query string)
	// OnRun runs the action-th action of a result.
	OnRun func(res registry.Result, action int)
	// Icons loads the system's icons of files and apps; nil shows glyphs alone.
	Icons *Icons

	theme theme.Theme
	ui    *ui.Theme
	query string
	input ui.Handle

	results  []registry.Result
	selected int
	armed    armed  // the action waiting to be confirmed
	notice   string // shown in the footer, as a confirmation asked
	list     ui.ListState
	actions  actionPanel
}

// NewRoot returns the root view drawn with a theme.
func NewRoot(t func(string, ...any) string, th theme.Theme) *Root {
	r := &Root{T: t}
	r.SetTheme(th)
	r.list.Key = func(i int) any { return r.results[i].Key }
	r.list.Label = func(i int) string { return r.results[i].Item.Title }
	return r
}

// SetTheme changes the theme the view draws with.
func (r *Root) SetTheme(th theme.Theme) {
	r.theme, r.ui = th, ToUI(th)
}

// Theme is the theme the view draws with.
func (r *Root) Theme() theme.Theme { return r.theme }

// Query is the text in the search field.
func (r *Root) Query() string { return r.query }

// Reset starts the view over: an empty search, with the keyboard in it.
func (r *Root) Reset() {
	r.query = ""
	r.actions.open = false
	r.input.Focus()
	if r.OnQuery != nil {
		r.OnQuery("")
	}
}

// SetResults shows the results of query, unless the search has moved on since.
func (r *Root) SetResults(query string, results []registry.Result) {
	if query != r.query {
		return
	}
	r.results, r.selected = results, 0
	r.armed, r.notice = armed{}, ""
	r.actions.open = false
	if len(results) > 0 {
		r.list.ScrollIntoView(0)
	}
}

// View builds the root view for one frame.
func (r *Root) View(c *ui.Context) {
	c.SetTheme(r.ui)
	t := c.Theme()
	bg := t.Background
	if c.Vibrancy() {
		// The material shows through the background, as much as the theme lets it.
		bg = bg.Alpha(r.theme.Material.Opacity * float32(bg.A) / 255)
	} else {
		bg.A = 255
	}
	c.Root().Background(bg)

	if r.Onboarding != nil {
		r.Onboarding.Motion = MotionOf(r.theme)
		r.Onboarding.View(c)
		return
	}

	r.keys(c)

	pad := r.theme.Space.Padding
	ui.Box(c).Fill().Children(func() {
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Padding(pad+2, pad+6).Gap(pad).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, searchIcon).FontSize(20).TextColor(t.TextMuted)
				field := ui.TextInputBase(c, &r.query).Bind(&r.input).Grow(1).FontSize(r.theme.Font.Size + 6).
					Placeholder(r.T("launcher.search_placeholder")).Label(r.T("launcher.search_label"))
				if field.Changed() {
					r.actions.open = false
					if r.OnQuery != nil {
						r.OnQuery(r.query)
					}
				}
				// Actions' own shortcuts go through the field first: it would take some, as
				// Ctrl+Shift+C, for itself.
				field.HandleInput(r.actionShortcut)
				// The field takes Enter: it runs the chosen action, of the panel or of the result.
				if field.Submitted() && len(r.results) > 0 {
					action := 0
					if r.actions.open {
						action = r.actions.selected
					}
					r.run(r.results[r.selected], action)
				}
			})
			ui.Divider(c)
			r.resultList(c)
			ui.Divider(c)
			r.footer(c)
		})
		if r.actions.open {
			r.actionPanel(c)
		}
	})
}

// keys handles the keyboard: moving through results and actions, running them, Escape.
// Nothing here animates: these keys are pressed hundreds of times a day.
func (r *Root) keys(c *ui.Context) {
	// One Escape closes one thing: the action panel, else the search; with neither, it is left
	// to the window, which hides.
	if (r.actions.open || r.query != "") && c.Shortcut(0, ui.KeyEscape) {
		if r.actions.open {
			r.actions.open = false
		} else {
			r.query = ""
			if r.OnQuery != nil {
				r.OnQuery("")
			}
		}
		return
	}
	if r.actions.open {
		r.actions.keys(c, r)
		return
	}
	if len(r.results) == 0 {
		return
	}
	if c.Shortcut(0, ui.KeyDown) {
		r.choose(r.selected + 1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		r.choose(r.selected - 1)
	}
	res := r.results[r.selected]
	if len(res.Item.Actions) > 1 && c.Shortcut(ui.Ctrl, ui.KeyEnter) {
		r.run(res, 1)
	}
	if c.Shortcut(ui.Ctrl, ui.KeyK) {
		r.actions = actionPanel{open: true}
	}
}

func (r *Root) choose(i int) {
	r.armed, r.notice = armed{}, ""
	r.selected = max(0, min(i, len(r.results)-1))
	r.list.ScrollIntoView(r.selected)
}

func (r *Root) run(res registry.Result, action int) {
	r.actions.open = false
	if r.OnRun == nil || action >= len(res.Item.Actions) {
		return
	}
	arm := armed{key: res.Key, action: action}
	if msg := res.Item.Actions[action].Confirm; msg != "" && r.armed != arm {
		r.armed, r.notice = arm, msg
		return
	}
	r.armed, r.notice = armed{}, ""
	r.OnRun(res, action)
}

// armed is an action asking for confirmation, waiting for its second press.
type armed struct {
	key    string
	action int
}

func (r *Root) resultList(c *ui.Context) {
	t := c.Theme()
	list := ui.List(c, &r.list, len(r.results), func(i int) { r.row(c, i) }).Grow(1).Padding(6)
	list.Children(func() {
		if len(r.results) > 0 {
			return
		}
		msg := r.T("launcher.empty")
		if r.query != "" {
			msg = r.T("launcher.no_results", "query", r.query)
		}
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, msg).TextColor(t.TextMuted) })
	})
}

// row draws one result: its icon, its title with the matched letters in the accent color, its
// subtitle and what kind it is.
func (r *Root) row(c *ui.Context, i int) {
	res := r.results[i]
	t := c.Theme()
	selected := i == r.selected
	height := r.theme.Space.RowHeight
	if res.Answer {
		height *= 1.6
	}
	row := ui.Row(c).Height(height).Padding(0, 10).Gap(12).AlignItems(ui.Center).Radius(r.theme.Shape.RowRadius)
	switch {
	case selected:
		row.Background(color(r.theme.Colors.RowSelected))
	case row.Hovered():
		row.Background(color(r.theme.Colors.RowSelected).Alpha(0.5))
	}
	if row.Clicked() {
		r.selected = i
		r.run(res, 0)
	}
	row.Children(func() {
		size := float32(24)
		if res.Answer {
			size = 30
		}
		r.icon(c, res.Item.Icon, size, t.TextMuted)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			titleSize := r.theme.Font.Size
			if res.Answer {
				titleSize += 8
			}
			ui.RichText(c, highlight(res.Item.Title, res.Positions, t.Text, t.Accent)...).FontSize(titleSize).SingleLine()
			if res.Item.Subtitle != "" {
				ui.Text(c, res.Item.Subtitle).FontSize(r.theme.Font.Size - 2).TextColor(t.TextMuted).SingleLine()
			}
		})
		accessory := res.Item.Accessory
		if accessory == "" && res.Command {
			accessory = r.T("launcher.kind_command")
		}
		if accessory != "" {
			ui.Text(c, accessory).FontSize(r.theme.Font.Size - 2).TextColor(t.TextMuted)
		}
	})
}

// highlight splits a title into spans, the matched letters in accent.
func highlight(title string, positions []int, text, accent ui.Color) []ui.Span {
	runes := []rune(title)
	matched := make([]bool, len(runes))
	for _, p := range positions {
		if p >= 0 && p < len(runes) {
			matched[p] = true
		}
	}
	var spans []ui.Span
	for start := 0; start < len(runes); {
		end := start
		for end < len(runes) && matched[end] == matched[start] {
			end++
		}
		s := ui.Span{Text: string(runes[start:end]), Color: text}
		if matched[start] {
			s.Color, s.Weight = accent, 600
		}
		spans = append(spans, s)
		start = end
	}
	return spans
}

func (r *Root) footer(c *ui.Context) {
	t := c.Theme()
	small := r.theme.Font.Size - 2
	pad := r.theme.Space.Padding
	ui.Row(c).Padding(pad-4, pad+4).Gap(pad).AlignItems(ui.Center).Children(func() {
		ui.Text(c, r.T("launcher.title")).FontSize(small).TextColor(t.TextMuted).Grow(1)
		if len(r.results) == 0 {
			if r.Hotkey != "" {
				ui.Text(c, r.T("launcher.hint_toggle", "hotkey", r.Hotkey)).FontSize(small).TextColor(t.TextMuted)
			}
			ui.Text(c, r.T("launcher.hint_close")).FontSize(small).TextColor(t.TextMuted)
			return
		}
		if r.notice != "" {
			ui.Text(c, r.notice).FontSize(small).FontWeight(600).TextColor(t.Warning)
			return
		}
		res := r.results[r.selected]
		if len(res.Item.Actions) > 0 {
			hint(c, res.Item.Actions[0].Title, "↵", small)
		}
		if len(res.Item.Actions) > 1 {
			hint(c, r.T("launcher.actions"), "Ctrl K", small)
		}
	})
}

// hint is a footer label with its keys.
func hint(c *ui.Context, label, keys string, size float32) {
	t := c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(size).TextColor(t.Text)
		ui.Row(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
			ui.Text(c, keys).FontSize(size - 1).TextColor(t.TextMuted)
		})
	})
}

// actionShortcut runs the action of the chosen result whose shortcut was pressed.
func (r *Root) actionShortcut(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || len(r.results) == 0 || r.actions.open {
		return false
	}
	res := r.results[r.selected]
	for i, a := range res.Item.Actions {
		if mods, key, ok := ParseAccelerator(a.Shortcut); ok && mods == ev.Mods && key == ev.Key {
			r.run(res, i)
			return true
		}
	}
	return false
}
