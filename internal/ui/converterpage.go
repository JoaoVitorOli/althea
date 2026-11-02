package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
)

// searchableOptions is how many options a form's dropdown holds before it can be searched:
// arrows are fine for a few, and no way to find one among forty.
const searchableOptions = 8

// converterPage shows a Converter: the two choices with a swap between them over the header, and
// below it a card with the text and another with what it became. Nothing runs as the user types,
// since it may send the text somewhere: Enter runs it.
type converterPage struct {
	pageBase
	v *sdk.Converter

	text     string
	input    ui.Handle
	from, to string
	detected string // what Convert said From turned out to be, when From is left to it

	result  string
	note    string
	failure string
	scroll  ui.ScrollState

	running func() // cancels the run on its way; nil with none
	runs    int    // runs asked, so that the rest of a stopped one is dropped
}

func newConverterPage(r *Root, ext string, v *sdk.Converter) *converterPage {
	p := &converterPage{
		pageBase: pageBase{r: r, ext: ext, name: v.Title}, v: v,
		text: v.Text, from: v.From.Value, to: v.To.Value,
	}
	if v.RunAtOnce {
		p.run()
	}
	return p
}

// run converts what is typed, in place of a run on its way.
func (p *converterPage) run() {
	if p.v.Convert == nil || p.r.Host == nil || strings.TrimSpace(p.text) == "" {
		return
	}
	p.stop()
	p.result, p.note, p.failure = "", "", ""
	p.scroll.Y = 0
	p.runs++
	n := p.runs
	asked := sdk.Conversion{Text: p.text, From: p.from, To: p.to}
	running := true
	p.running = func() {} // a host may answer before it returns the real one
	cancel := p.r.Host.Go(p.ext, func(ctx sdk.Context) error {
		err := p.v.Convert(ctx, asked, func(c sdk.Converted) {
			p.update(func() {
				if n != p.runs {
					return
				}
				p.result, p.note = c.Text, c.Note
				if c.From != "" {
					p.detected = c.From
				}
				if c.To != "" {
					p.to = c.To
				}
			})
		})
		stopped := ctx.Err() != nil
		p.update(func() {
			if n != p.runs {
				return
			}
			running, p.running = false, nil
			if err != nil && !stopped {
				// Beside what it is about, for as long as it applies, rather than a passing toast.
				p.failure, p.note = err.Error(), ""
			}
		})
		return nil
	})
	if running {
		p.running = cancel
	}
}

// stop stops the run on its way, keeping what came.
func (p *converterPage) stop() {
	if p.running == nil {
		return
	}
	p.running()
	p.running = nil
	p.runs++
}

func (p *converterPage) close() {
	p.stop()
	p.pageBase.close()
}

// again runs anew what has been run, as after the choices changed.
func (p *converterPage) again() {
	if p.result != "" || p.running != nil {
		p.run()
	}
}

// swap turns the choices around, and the result into the text.
func (p *converterPage) swap() {
	from := p.from
	if p.detected != "" {
		from = p.detected
	}
	if !hasOption(p.v.To.Options, from) || !hasOption(p.v.From.Options, p.to) {
		return // From is left to Convert, which has not said what it is yet
	}
	p.from, p.to, p.detected = p.to, from, ""
	if p.result != "" {
		p.text = p.result
		p.run()
	}
}

func hasOption(options []sdk.Option, value string) bool {
	return slices.ContainsFunc(options, func(o sdk.Option) bool { return o.Value == value })
}

// pick shows a picker's options to search through, the current one chosen.
func (p *converterPage) pick(picker sdk.Picker, current string, set func(string)) {
	items := make([]sdk.Item, len(picker.Options))
	at := 0
	for i, o := range picker.Options {
		item := sdk.Item{
			ID: o.Value, Title: o.Title, Keywords: []string{o.Value, o.Badge}, Accessory: o.Badge,
			Icon: sdk.Icon{Glyph: "translate"},
		}
		if o.Value == current {
			at = i
			item.Icon = sdk.Icon{Glyph: "check"}
		}
		item.Actions = []sdk.Action{{Title: p.r.T("view.choose"), Run: func(sdk.Context) error {
			p.update(func() {
				set(o.Value)
				if top, ok := p.r.top().(*listPage); ok {
					p.r.remove(top)
				}
				p.again()
			})
			return nil
		}}}
		items[i] = item
	}
	p.r.Push(p.ext, &sdk.List{Title: picker.Title, Placeholder: picker.Placeholder, Items: items})
	if top, ok := p.r.top().(*listPage); ok {
		top.choose(at)
	}
}

func (p *converterPage) pickFrom() {
	current := p.from
	if p.detected != "" && current == "" {
		current = p.detected
	}
	p.pick(p.v.From, current, func(v string) { p.from, p.detected = v, "" })
}

func (p *converterPage) pickTo() {
	p.pick(p.v.To, p.to, func(v string) { p.to = v })
}

// option finds the option of a value, for showing it.
func option(options []sdk.Option, value string) sdk.Option {
	if i := slices.IndexFunc(options, func(o sdk.Option) bool { return o.Value == value }); i >= 0 {
		return options[i]
	}
	return sdk.Option{Value: value, Title: value}
}

func (p *converterPage) header(c *ui.Context) {
	ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Children(func() {
			o := option(p.v.From.Options, p.from)
			if p.detected != "" {
				found := option(p.v.To.Options, p.detected)
				o = sdk.Option{Badge: found.Badge, Title: p.r.T("view.detected", "name", found.Title)}
			}
			if p.chip(c, o, p.v.From.Title) {
				p.pickFrom()
			}
		})
		if p.iconButton(c, "swap", p.r.T("view.swap"), 16) {
			p.swap()
		}
		ui.Row(c).Grow(1).Basis(0).Children(func() {
			if p.chip(c, option(p.v.To.Options, p.to), p.v.To.Title) {
				p.pickTo()
			}
		})
		// As wide as the back button, so that the swap sits in the middle of the window.
		ui.Box(c).Size(28, 28)
	})
}

// chip is a choice in the header: its badge and title, opening the options on a click.
func (p *converterPage) chip(c *ui.Context, o sdk.Option, label string) bool {
	t := c.Theme()
	m := MotionOf(p.r.theme)
	chip := ui.Row(c).Padding(5, 10).Gap(8).AlignItems(ui.Center).Radius(8).Label(label)
	bg := t.SurfaceHover.Alpha(0)
	if chip.Hovered() {
		bg = t.SurfaceHover
	}
	chip.Background(m.color(chip, "bg", bg, m.Hover()))
	m.pose(chip, pressed(chip.Pressed()), m.Press())
	chip.Children(func() {
		if o.Badge != "" {
			ui.Row(c).Padding(1, 5).Radius(4).Background(t.SurfaceHover).Children(func() {
				ui.Text(c, o.Badge).FontSize(p.r.theme.Font.Size - 4).FontWeight(600).TextColor(t.TextMuted)
			})
		}
		ui.Text(c, o.Title).FontWeight(600).TextColor(t.Text).SingleLine()
	})
	return chip.Clicked()
}

// iconButton is a small button with a glyph, for the pointer: every one has a shortcut too.
func (p *converterPage) iconButton(c *ui.Context, glyph, label string, size float32) bool {
	t := c.Theme()
	m := MotionOf(p.r.theme)
	b := ui.Row(c).Size(28, 28).AlignSelf(ui.Start).Center().Radius(8).Label(label).Tooltip(label)
	bg := t.SurfaceHover.Alpha(0)
	if b.Hovered() {
		bg = t.SurfaceHover
	}
	b.Background(m.color(b, "bg", bg, m.Hover()))
	m.pose(b, pressed(b.Pressed()), m.Press())
	b.Children(func() { ui.Icon(c, glyphs[glyph]).FontSize(size).TextColor(t.TextMuted) })
	return b.Clicked()
}

func (p *converterPage) body(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(10, 12).Gap(8).AlignItems(ui.Stretch).Children(func() {
		p.card(c, p.text, func() {
			placeholder := p.v.Placeholder
			if placeholder == "" {
				placeholder = p.r.T("view.chat_placeholder")
			}
			field := ui.TextAreaBase(c, &p.text).Bind(&p.input).Grow(1).Placeholder(placeholder).Label(placeholder)
			field.HandleInput(p.inputKeys)
		})
		p.card(c, p.result, func() {
			ui.Scroll(c).TrackScroll(&p.scroll).Grow(1).Children(func() { p.output(c) })
		})
	})
}

// card is a surface with its content and a button that copies its text.
func (p *converterPage) card(c *ui.Context, text string, content func()) {
	t := c.Theme()
	ui.Row(c).Grow(1).Basis(0).Padding(10, 8, 10, 14).Gap(6).AlignItems(ui.Stretch).Radius(p.r.theme.Shape.RowRadius + 2).Background(t.Surface).Clip().
		Children(func() {
			content()
			if text == "" {
				return
			}
			if p.iconButton(c, "copy", p.r.T("view.copy"), 15) {
				p.copyText(text)
			}
		})
}

// output draws what the run came to, or what is in its way.
func (p *converterPage) output(c *ui.Context) {
	t := c.Theme()
	switch {
	case p.failure != "":
		ui.Text(c, p.failure).TextColor(t.Danger)
	case p.note != "":
		ui.Text(c, p.note).TextColor(t.TextMuted)
	case p.result != "":
		ui.Column(c).Selectable().Children(func() { ui.Text(c, p.result).TextColor(t.Accent) })
	case p.running != nil:
		ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start)
	default:
		ui.Text(c, p.v.ResultPlaceholder).TextColor(t.TextMuted)
	}
}

func (p *converterPage) copyText(text string) {
	p.work(func(ctx sdk.Context) error {
		ctx.Toast(p.r.T("view.copied"))
		return ctx.CopyText(text)
	})
}

// inputKeys handles the keys the text area would take for itself. Enter runs and Shift+Enter breaks
// the line, as in a chat; with the action panel open, the keys are the panel's.
func (p *converterPage) inputKeys(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	if p.r.pageShortcut(p, ev) {
		return true
	}
	if p.r.actions.open {
		switch ev.Key {
		case ui.KeyEnter:
			p.r.runPage(p, p.r.actions.selected)
			return true
		case ui.KeyUp:
			p.r.actions.selected = max(p.r.actions.selected-1, 0)
			return true
		case ui.KeyDown:
			p.r.actions.selected = min(p.r.actions.selected+1, len(p.actions())-1)
			return true
		}
		return false
	}
	if ev.Key == ui.KeyEnter {
		switch ev.Mods {
		case 0:
			p.run()
			return true
		case ui.Ctrl:
			p.r.runPage(p, p.secondary())
			return true
		}
	}
	return false
}

func (p *converterPage) keys(*ui.Context) {}

// The first action is running, which Enter does. The results' are for what the run came to as these
// are made, every frame.
func (p *converterPage) actions() []sdk.Action {
	title := p.v.ConvertTitle
	if title == "" {
		title = p.r.T("view.submit")
	}
	result := p.result
	return []sdk.Action{
		{Title: title},
		{Title: p.r.T("view.copy_result"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
			if result == "" {
				return nil // nothing yet: the clipboard stays as it is
			}
			ctx.Toast(p.r.T("view.copied"))
			return ctx.CopyText(result)
		}},
		{Title: p.r.T("view.paste_result"), Shortcut: "Ctrl+Shift+V", Run: func(ctx sdk.Context) error {
			if result == "" {
				return nil
			}
			return ctx.Paste(sdk.Clip{Text: result})
		}},
		{Title: p.r.T("view.swap"), Shortcut: "Ctrl+S", Run: func(sdk.Context) error {
			p.update(p.swap)
			return nil
		}},
		{Title: p.v.To.Title, Shortcut: "Ctrl+L", Run: func(sdk.Context) error {
			p.update(p.pickTo)
			return nil
		}},
		{Title: p.v.From.Title, Shortcut: "Ctrl+Shift+L", Run: func(sdk.Context) error {
			p.update(p.pickFrom)
			return nil
		}},
	}
}

func (*converterPage) secondary() int { return 1 }
func (*converterPage) chosen() string { return "" }

// escape stops the run on its way before leaving.
func (p *converterPage) escape() bool {
	if p.running == nil {
		return false
	}
	p.stop()
	return true
}

func (p *converterPage) focus() { p.input.Focus() }
