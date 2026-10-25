// Package ui holds Althea's views, drawn with MyGo's native UI toolkit.
package ui

import (
	"embed"
	"io/fs"

	"github.com/egoist/mygo/ui"
)

//go:embed locales/*.toml
var locales embed.FS

// Messages are the catalogs of the host's own text, for i18n.Catalog.Add.
var Messages, _ = fs.Sub(locales, "locales")

// Root is the launcher's root view: the search bar, the results and the footer.
type Root struct {
	// T translates a message key, as i18n.Catalog.T does.
	T func(key string, args ...any) string
}

// Theme is the theme the root view draws with.
func (*Root) Theme() *ui.Theme { return darkTheme }

// View builds the root view for one frame.
func (r *Root) View(c *ui.Context) {
	c.SetTheme(r.Theme())
	t := c.Theme()
	c.Root().BorderColor(t.Border).Background(t.Background)

	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).Padding(12).Children(func() {
			ui.Text(c, r.T("launcher.title")).TextColor(t.Text)
		})
		ui.Row(c).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Padding(6).Children(func() {
			ui.Text(c, r.T("launcher.hint_close")).TextColor(t.TextMuted)
		})
	})
}
