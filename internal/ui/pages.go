package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
)

// Host runs extensions' code for the views they push, off the main thread.
type Host interface {
	// Go runs fn with the Context of extension ext on a goroutine of its own, showing its error
	// as a toast; cancel cancels fn's Context.
	Go(ext string, fn func(ctx sdk.Context) error) (cancel func())
	// Update runs fn on the main thread, then draws a frame.
	Update(fn func())
}

// page is a view an extension pushed over the root search. Its methods run on the main
// thread.
type page interface {
	// header draws the page's part of the header, after the back button: its search field or
	// its title.
	header(c *ui.Context)
	body(c *ui.Context)
	// keys handles the keys that move through the page.
	keys(c *ui.Context)
	// actions are those of what is chosen, the first run by Enter.
	actions() []sdk.Action
	// secondary is the action Ctrl+Enter runs, -1 for none.
	secondary() int
	// chosen identifies what is chosen, so that a confirmation asked follows it.
	chosen() string
	// escape undoes something of the page, as a query typed, reporting whether it did.
	escape() bool
	title() string
	// focus gives the page the keyboard.
	focus()
	// close stops the page's work in flight.
	close()
	extension() string
}

// Push shows view v of extension ext over what shows.
func (r *Root) Push(ext string, v sdk.View) {
	var p page
	switch v := v.(type) {
	case *sdk.List:
		p = newListPage(r, ext, listSource{
			title: v.Title, placeholder: v.Placeholder, items: v.Items, load: v.Load, search: v.Search,
			empty: v.Empty, showDetail: v.ShowDetail,
		})
	case *sdk.Grid:
		p = newListPage(r, ext, listSource{
			title: v.Title, placeholder: v.Placeholder, items: v.Items, load: v.Load, search: v.Search,
			empty: v.Empty, grid: true, columns: v.Columns,
		})
	case *sdk.Detail:
		p = newDetailPage(r, ext, v)
	case *sdk.Form:
		p = newFormPage(r, ext, v)
	default:
		return
	}
	r.actions.open = false
	r.armed, r.notice = armed{}, ""
	r.pushes++
	r.stack = append(r.stack, p)
	p.focus()
}

// Pop closes the view on top, back to the one below or to the root search.
func (r *Root) Pop() {
	if len(r.stack) > 0 {
		r.remove(r.stack[len(r.stack)-1])
	}
}

// remove closes page p wherever it is in the stack.
func (r *Root) remove(p page) {
	for i, q := range r.stack {
		if q != p {
			continue
		}
		p.close()
		top := i == len(r.stack)-1
		r.stack = append(r.stack[:i], r.stack[i+1:]...)
		if top {
			r.actions.open = false
			r.armed, r.notice = armed{}, ""
			if len(r.stack) > 0 {
				r.stack[len(r.stack)-1].focus()
			} else {
				r.input.Focus()
			}
		}
		return
	}
}

// closeAll closes every page, back to the root search.
func (r *Root) closeAll() {
	for _, p := range r.stack {
		p.close()
	}
	r.stack = nil
}

// pageView draws the page on top: its header, its body and a footer with its actions.
func (r *Root) pageView(c *ui.Context, p page) {
	t := c.Theme()
	pad := r.theme.Space.Padding
	ui.Box(c).Fill().Children(func() {
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Height(r.theme.Font.Size+6+2*(pad+2)).Padding(0, pad+2).Gap(pad - 2).AlignItems(ui.Center).Children(func() {
				back := ui.Row(c).Size(28, 28).Center().Radius(8).Label(r.T("view.back"))
				if back.Hovered() {
					back.Background(t.SurfaceHover)
				}
				if back.Clicked() {
					r.Pop()
				}
				back.Children(func() { ui.Icon(c, glyphs["back"]).FontSize(18).TextColor(t.TextMuted) })
				p.header(c)
			})
			ui.Divider(c)
			body := ui.Column(c).Key(r.pushes).Grow(1).AlignItems(ui.Stretch)
			r.arrive(body)
			body.Children(func() { p.body(c) })
			ui.Divider(c)
			r.pageFooter(c, p)
		})
		if r.actions.open {
			r.actionPanel(c, p.actions(), func(i int) { r.runPage(p, i) })
		}
	})
}

// arrive fades a pushed page in, sliding it a little from the right, where it comes from.
func (r *Root) arrive(el ui.Element) {
	m := MotionOf(r.theme)
	d := m.scaled(180 * time.Millisecond)
	if m.Off() || d <= 0 {
		return
	}
	fade := el.Keyframes("fade", []ui.Keyframe{{Value: 0}, {At: d, Value: 1, Ease: strongEaseOut}}, ui.KeyframeOptions{})
	slide := el.Keyframes("slide", []ui.Keyframe{{Value: 12}, {At: d, Value: 0, Ease: strongEaseOut}}, ui.KeyframeOptions{})
	el.Opacity(fade).Translate(slide, 0)
}

// pageKeys handles the keys every page shares: Escape, the action panel, Ctrl+Enter and the
// actions' own shortcuts.
func (r *Root) pageKeys(c *ui.Context, p page) {
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case r.actions.open:
			r.actions.open = false
		case !p.escape():
			r.Pop()
		}
		return
	}
	actions := p.actions()
	if r.actions.open {
		r.actions.keys(c, len(actions))
		return
	}
	if i := p.secondary(); i >= 0 && i < len(actions) && c.Shortcut(ui.Ctrl, ui.KeyEnter) {
		r.runPage(p, i)
		return
	}
	if len(actions) > 0 && c.Shortcut(ui.Ctrl, ui.KeyK) {
		r.actions = actionPanel{open: true}
		return
	}
	for i, a := range actions {
		if mods, key, ok := ParseAccelerator(a.Shortcut); ok && c.Shortcut(mods, key) {
			r.runPage(p, i)
			return
		}
	}
	p.keys(c)
}

// pageShortcut runs the action of a page whose shortcut was pressed in its search field, which
// would take some, as Ctrl+Shift+C, for itself.
func (r *Root) pageShortcut(p page, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || r.actions.open {
		return false
	}
	for i, a := range p.actions() {
		if mods, key, ok := ParseAccelerator(a.Shortcut); ok && mods == ev.Mods && key == ev.Key {
			r.runPage(p, i)
			return true
		}
	}
	return false
}

// runPage runs the i-th action of what is chosen in page p, after its confirmation if it asks
// for one.
func (r *Root) runPage(p page, i int) {
	r.actions.open = false
	actions := p.actions()
	if i >= len(actions) {
		return
	}
	a := actions[i]
	arm := armed{key: p.chosen(), action: i}
	if a.Confirm != "" && r.armed != arm {
		r.armed, r.notice = arm, a.Confirm
		return
	}
	r.armed, r.notice = armed{}, ""
	if f, ok := p.(*formPage); ok {
		f.submit()
		return
	}
	if a.Run == nil || r.Host == nil {
		return
	}
	r.Host.Go(p.extension(), a.Run)
}

func (r *Root) pageFooter(c *ui.Context, p page) {
	t := c.Theme()
	small := r.theme.Font.Size - 2
	pad := r.theme.Space.Padding
	ui.Row(c).Padding(pad-4, pad+4).Gap(pad).AlignItems(ui.Center).Children(func() {
		title := p.title()
		if title == "" {
			title = r.T("launcher.title")
		}
		ui.Text(c, title).FontSize(small).TextColor(t.TextMuted).Grow(1).SingleLine()
		if r.notice != "" {
			ui.Text(c, r.notice).FontSize(small).FontWeight(600).TextColor(t.Warning)
			return
		}
		actions := p.actions()
		if len(actions) > 0 {
			keys := "↵"
			if _, form := p.(*formPage); form {
				keys = "Ctrl ↵"
			}
			hint(c, actions[0].Title, keys, small)
		}
		if len(actions) > 1 {
			hint(c, r.T("launcher.actions"), "Ctrl K", small)
		}
	})
}

// pageBase is what every page keeps.
type pageBase struct {
	r     *Root
	ext   string
	name  string
	works []func() // cancels of the work in flight
}

func (b *pageBase) extension() string { return b.ext }
func (b *pageBase) title() string     { return b.name }

func (b *pageBase) close() {
	for _, cancel := range b.works {
		cancel()
	}
	b.works = nil
}

// work runs fn with the extension's Context off the main thread, unless there is no host;
// closing the page cancels it.
func (b *pageBase) work(fn func(ctx sdk.Context) error) {
	if b.r.Host != nil {
		b.works = append(b.works, b.r.Host.Go(b.ext, fn))
	}
}

// update runs fn on the main thread, after work.
func (b *pageBase) update(fn func()) { b.r.Host.Update(fn) }

// emptyState is what a page shows with nothing in it: a title, a line below and an icon.
func (r *Root) emptyState(c *ui.Context, e sdk.Empty) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(6).Children(func() {
		if e.Icon.Glyph != "" || e.Icon.Text != "" || e.Icon.File != "" || e.Icon.Image != "" || e.Icon.Data != nil {
			r.icon(c, e.Icon, 36, t.TextMuted)
		}
		ui.Text(c, e.Title).FontWeight(600).TextColor(t.Text)
		if e.Description != "" {
			ui.Text(c, e.Description).FontSize(r.theme.Font.Size - 1).TextColor(t.TextMuted)
		}
	})
}
