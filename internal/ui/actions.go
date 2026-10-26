package ui

import (
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
	"github.com/JoaoVitorOli/althea/sdk"
)

// actionPanel lists every action of the chosen item, from Ctrl+K: Up and Down choose,
// Enter runs, Escape closes. It opens at once, without motion, as it comes from the keyboard.
type actionPanel struct {
	open     bool
	selected int
}

// keys moves through n actions.
func (p *actionPanel) keys(c *ui.Context, n int) {
	if n == 0 || c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Ctrl, ui.KeyK) {
		p.open = false
		return
	}
	if c.Shortcut(0, ui.KeyDown) {
		p.selected = min(p.selected+1, n-1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		p.selected = max(p.selected-1, 0)
	}
}

// actionPanel floats over the results, above the footer, on glass when the theme asks for it.
// A click runs the i-th action with run.
func (r *Root) actionPanel(c *ui.Context, actions []sdk.Action, run func(i int)) {
	t := c.Theme()
	panel := ui.Column(c).Absolute().Right(12).Bottom(44).Width(320).Padding(6).Gap(2).
		Radius(r.theme.Shape.WindowRadius - 2)
	if r.theme.Material.Kind == theme.Glass {
		panel.Material(glass.Glass{})
	} else {
		panel.Background(t.Surface).BorderWidth(1).BorderColor(t.Border).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.35))
	}
	panel.Children(func() {
		for i, a := range actions {
			row := ui.Row(c).Key(i).Height(36).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(r.theme.Shape.RowRadius)
			switch {
			case i == r.actions.selected:
				row.Background(color(r.theme.Colors.RowSelected))
			case row.Hovered():
				row.Background(color(r.theme.Colors.RowSelected).Alpha(0.5))
			}
			if row.Clicked() {
				run(i)
			}
			row.Children(func() {
				ui.Text(c, a.Title).TextColor(t.Text).Grow(1).SingleLine()
				shortcut := a.Shortcut
				switch i {
				case 0:
					shortcut = "↵"
				case 1:
					shortcut = "Ctrl+↵"
				}
				if shortcut != "" {
					ui.Row(c).Gap(3).Children(func() {
						for _, k := range strings.Split(shortcut, "+") {
							ui.Row(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
								ui.Text(c, k).FontSize(11).TextColor(t.TextMuted)
							})
						}
					})
				}
			})
		}
	})
}
