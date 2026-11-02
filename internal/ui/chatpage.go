package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/JoaoVitorOli/althea/sdk"
)

// follower keeps a scroll container at its end as what it shows grows, until the user scrolls up
// from it; scrolling back to the end follows again.
type follower struct {
	ui.ScrollState
	away bool
	last float32
}

// followSlack is how near its end, in DIPs, the container counts as being at it.
const followSlack = 24

// frame goes before the container is built.
func (f *follower) frame() {
	if f.Y < f.last-1 {
		f.away = true
	}
	if f.Y >= f.MaxY-followSlack {
		f.away = false
	}
	f.last = f.Y
	if !f.away {
		f.Y = math.MaxFloat32
	}
}

// chatPage shows a Chat: what was said, which scrolls and follows the answer as it comes, over a
// field to say more.
type chatPage struct {
	pageBase
	v *sdk.Chat

	messages []*chatLine
	draft    string
	input    ui.Handle
	scroll   follower

	sending func() // cancels the answer on its way; nil with none
	sends   int    // answers asked, so that the rest of a stopped one is dropped
}

// chatLine is a message, with what draws its Markdown from frame to frame.
type chatLine struct {
	mine     bool
	text     string
	writing  bool
	markdown markdown.Options
}

func newChatPage(r *Root, ext string, v *sdk.Chat) *chatPage {
	p := &chatPage{pageBase: pageBase{r: r, ext: ext, name: v.Title}, v: v}
	for _, m := range v.Messages {
		p.messages = append(p.messages, &chatLine{mine: m.Mine, text: m.Text})
	}
	if v.First != "" {
		p.send(v.First)
	}
	return p
}

// send says text and shows the answer as it comes.
func (p *chatPage) send(text string) {
	if p.v.Send == nil || p.r.Host == nil {
		return
	}
	answer := &chatLine{writing: true}
	p.messages = append(p.messages, &chatLine{mine: true, text: text}, answer)
	p.scroll.away = false
	p.sends++
	n := p.sends
	running := true
	p.sending = func() {} // a host may answer before it returns the real one
	cancel := p.r.Host.Go(p.ext, func(ctx sdk.Context) error {
		err := p.v.Send(ctx, text, func(markdown string) {
			p.update(func() {
				if n == p.sends {
					answer.text = markdown
				}
			})
		})
		p.update(func() {
			if n != p.sends {
				return // stopped: stop tidied up
			}
			running, p.sending = false, nil
			answer.writing = false
			if answer.text == "" {
				p.dropLast()
			}
		})
		return err
	})
	if running {
		p.sending = cancel
	}
}

// dropLast takes out the answer that never came.
func (p *chatPage) dropLast() { p.messages = p.messages[:len(p.messages)-1] }

// stop stops the answer on its way, keeping what came.
func (p *chatPage) stop() {
	if p.sending == nil {
		return
	}
	p.sending()
	p.sending = nil
	p.sends++
	last := p.messages[len(p.messages)-1]
	last.writing = false
	if last.text == "" {
		p.dropLast()
	}
}

func (p *chatPage) close() {
	p.stop()
	p.pageBase.close()
}

func (p *chatPage) header(c *ui.Context) {
	ui.Text(c, p.v.Title).FontSize(p.r.theme.Font.Size + 2).FontWeight(600).TextColor(c.Theme().Text).Grow(1).SingleLine()
	if p.sending != nil {
		ui.Spinner(c).Size(16, 16)
	}
}

func (p *chatPage) body(c *ui.Context) {
	p.scroll.frame()
	ui.Scroll(c).TrackScroll(&p.scroll.ScrollState).Grow(1).Padding(14, 20).Children(func() {
		ui.Column(c).Gap(14).AlignItems(ui.Stretch).Children(func() {
			for i, m := range p.messages {
				ui.Column(c).Key(i).AlignItems(ui.Stretch).Children(func() { p.line(c, m) })
			}
		})
	})
	ui.Divider(c)
	pad := p.r.theme.Space.Padding
	ui.Row(c).Padding(pad-2, pad+6).AlignItems(ui.Center).Children(func() {
		placeholder := p.v.Placeholder
		if placeholder == "" {
			placeholder = p.r.T("view.chat_placeholder")
		}
		field := ui.TextInputBase(c, &p.draft).Bind(&p.input).Grow(1).Placeholder(placeholder).Label(placeholder)
		field.HandleInput(func(ev ui.InputEvent) bool { return p.r.pageShortcut(p, ev) })
		if field.Submitted() {
			if p.r.actions.open {
				p.r.runPage(p, p.r.actions.selected)
			} else {
				p.submit()
			}
		}
	})
}

// line draws a message: the user's as it was typed, in a band of its own, and an answer as
// Markdown, coming in smoothly while it is written.
func (p *chatPage) line(c *ui.Context, m *chatLine) {
	t := c.Theme()
	if m.mine {
		ui.Column(c).Padding(8, 12).Radius(p.r.theme.Shape.RowRadius).Background(t.SurfaceHover).Children(func() {
			ui.Text(c, m.text).TextColor(t.Text)
		})
		return
	}
	if m.markdown.Theme == nil {
		m.markdown.Theme = &markdown.Theme{FontSize: p.r.theme.Font.Size}
	}
	switch {
	case m.writing && m.text == "":
		ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start)
	case m.writing:
		markdown.Stream(c, m.text, &m.markdown)
	default:
		markdown.View(c, m.text, &m.markdown)
	}
}

// submit says what is typed, unless an answer is on its way.
func (p *chatPage) submit() {
	text := strings.TrimSpace(p.draft)
	if text == "" || p.sending != nil {
		return
	}
	p.draft = ""
	p.send(text)
}

func (p *chatPage) keys(*ui.Context) {}

// The first action is saying what is typed, which the field runs on Enter.
func (p *chatPage) actions() []sdk.Action {
	title := p.v.SendTitle
	if title == "" {
		title = p.r.T("view.send")
	}
	return append([]sdk.Action{{Title: title}}, p.v.Actions...)
}

func (*chatPage) secondary() int { return 1 }

func (p *chatPage) chosen() string { return strconv.Itoa(len(p.messages)) }

// escape stops the answer on its way, else clears what is typed, before leaving.
func (p *chatPage) escape() bool {
	switch {
	case p.sending != nil:
		p.stop()
		return true
	case p.draft != "":
		p.draft = ""
		return true
	}
	return false
}

func (p *chatPage) focus() { p.input.Focus() }
