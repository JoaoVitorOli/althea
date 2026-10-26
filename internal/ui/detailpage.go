package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/JoaoVitorOli/althea/sdk"
)

// detailPage shows a Detail: its Markdown, which scrolls, and its metadata.
type detailPage struct {
	pageBase
	d  *sdk.Detail
	md markdown.Options
}

func newDetailPage(r *Root, ext string, d *sdk.Detail) *detailPage {
	return &detailPage{pageBase: pageBase{r: r, ext: ext, name: d.Title}, d: d}
}

func (p *detailPage) header(c *ui.Context) {
	ui.Text(c, p.d.Title).FontSize(p.r.theme.Font.Size + 2).FontWeight(600).TextColor(c.Theme().Text).Grow(1).SingleLine()
}

func (p *detailPage) body(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(16, 20).Children(func() { p.r.detail(c, p.d, &p.md) })
}

func (p *detailPage) keys(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEnter) {
		p.r.runPage(p, 0)
	}
}

func (p *detailPage) actions() []sdk.Action { return p.d.Actions }
func (*detailPage) secondary() int          { return 1 }
func (*detailPage) chosen() string          { return "" }
func (*detailPage) escape() bool            { return false }
func (*detailPage) focus()                  {}

// detail draws a Detail's Markdown, then its metadata as labeled lines.
func (r *Root) detail(c *ui.Context, d *sdk.Detail, md *markdown.Options) {
	t := c.Theme()
	if md.Theme == nil {
		md.Theme = &markdown.Theme{FontSize: r.theme.Font.Size}
	}
	ui.Column(c).Gap(14).Children(func() {
		if b := r.Icons.Data(d.Image); b != nil {
			ui.Image(c, b).Height(220).Fit(ui.Contain).Radius(r.theme.Shape.RowRadius).AlignSelf(ui.Start)
		}
		if d.Markdown != "" {
			markdown.View(c, d.Markdown, md)
		}
		if len(d.Metadata) == 0 {
			return
		}
		if d.Markdown != "" {
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
