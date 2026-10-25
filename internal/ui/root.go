// Package ui holds Althea's views, drawn with MyGo's native UI toolkit.
package ui

import "github.com/egoist/mygo/ui"

// Root is the launcher's root view: the search bar, the results and the footer.
type Root struct{}

// Theme is the theme the root view draws with.
func (*Root) Theme() *ui.Theme { return darkTheme }

// View builds the root view for one frame.
func (r *Root) View(c *ui.Context) {
	c.SetTheme(r.Theme())
	t := c.Theme()
	c.Root().BorderColor(t.Border).Background(t.Background)

	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).Padding(12).Children(func() {
			ui.Text(c, "Althea").TextColor(t.Text)
		})
		ui.Row(c).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Padding(6).Children(func() {
			ui.Text(c, "Esc to close").TextColor(t.TextMuted)
		})
	})
}
