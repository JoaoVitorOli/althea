package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/JoaoVitorOli/althea/sdk"
)

// detailPage shows a Detail: its Markdown, which scrolls, and its metadata. A Detail that streams
// follows its text as it comes, until the user scrolls away from the end.
type detailPage struct {
	pageBase
	d  *sdk.Detail
	md markdown.Options

	text      string
	streaming bool
	scroll    follower
}

func newDetailPage(r *Root, ext string, d *sdk.Detail) *detailPage {
	p := &detailPage{pageBase: pageBase{r: r, ext: ext, name: d.Title}, d: d, text: d.Markdown}
	if d.Stream == nil || r.Host == nil {
		return p
	}
	p.streaming = true
	p.work(func(ctx sdk.Context) error {
		err := d.Stream(ctx, func(markdown string) {
			p.update(func() {
				if p.streaming {
					p.text = markdown
				}
			})
		})
		p.update(func() { p.streaming = false })
		return err
	})
	return p
}

func (p *detailPage) header(c *ui.Context) {
	ui.Text(c, p.d.Title).FontSize(p.r.theme.Font.Size + 2).FontWeight(600).TextColor(c.Theme().Text).Grow(1).SingleLine()
	if p.streaming {
		ui.Spinner(c).Size(16, 16)
	}
}

func (p *detailPage) body(c *ui.Context) {
	if p.streaming {
		p.scroll.frame()
	}
	ui.Scroll(c).TrackScroll(&p.scroll.ScrollState).Grow(1).Padding(16, 20).Children(func() {
		p.r.detailText(c, p.d, p.text, p.streaming, &p.md)
	})
}

func (p *detailPage) keys(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEnter) {
		p.r.runPage(p, 0)
	}
}

func (p *detailPage) actions() []sdk.Action { return p.d.Actions }
func (*detailPage) secondary() int          { return 1 }
func (*detailPage) chosen() string          { return "" }
func (*detailPage) focus()                  {}

// escape stops what is being written, keeping what came, before leaving.
func (p *detailPage) escape() bool {
	if !p.streaming {
		return false
	}
	p.streaming = false
	p.close()
	return true
}

// detail draws a Detail's Markdown, then its metadata as labeled lines.
func (r *Root) detail(c *ui.Context, d *sdk.Detail, md *markdown.Options) {
	r.detailText(c, d, d.Markdown, false, md)
}

// detailText draws a Detail with the Markdown text, still being written when streaming is set.
func (r *Root) detailText(c *ui.Context, d *sdk.Detail, text string, streaming bool, md *markdown.Options) {
	t := c.Theme()
	if md.Theme == nil {
		md.Theme = &markdown.Theme{FontSize: r.theme.Font.Size}
	}
	ui.Column(c).Gap(14).Children(func() {
		if b := r.Icons.Data(d.Image); b != nil {
			ui.Image(c, b).Height(220).Fit(ui.Contain).Radius(r.theme.Shape.RowRadius).AlignSelf(ui.Start)
		}
		switch {
		case streaming && text != "":
			markdown.Stream(c, text, md)
		case text != "":
			markdown.View(c, text, md)
		}
		if len(d.Metadata) == 0 {
			return
		}
		if text != "" {
			ui.Divider(c)
		}
		ui.Column(c).Gap(8).Children(func() {
			for _, m := range d.Metadata {
				ui.Row(c).Gap(12).Children(func() {
					ui.Text(c, m.Label).FontSize(r.theme.Font.Size - 1).TextColor(t.TextMuted).Width(140)
					ui.Text(c, m.Text).FontSize(r.theme.Font.Size - 1).TextColor(t.Text).Grow(1)
				})
			}
		})
	})
}
