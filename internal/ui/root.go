// Package ui holds Althea's views, drawn with MyGo's native UI toolkit.
package ui

import (
	"embed"
	"io/fs"

	"github.com/egoist/mygo/ui"

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

	theme theme.Theme
	ui    *ui.Theme
	query string
	input ui.Handle
}

// NewRoot returns the root view drawn with a theme.
func NewRoot(t func(string, ...any) string, th theme.Theme) *Root {
	r := &Root{T: t}
	r.SetTheme(th)
	return r
}

// SetTheme changes the theme the view draws with.
func (r *Root) SetTheme(th theme.Theme) {
	r.theme, r.ui = th, ToUI(th)
}

// Theme is the theme the view draws with.
func (r *Root) Theme() theme.Theme { return r.theme }

// Reset starts the view over: an empty search, with the keyboard in it.
func (r *Root) Reset() {
	r.query = ""
	r.input.Focus()
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

	// Escape clears the search first; an Escape left to the window hides it.
	if r.query != "" && c.Shortcut(0, ui.KeyEscape) {
		r.query = ""
	}

	pad := r.theme.Space.Padding
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(pad+2, pad+6).Gap(pad).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(20).TextColor(t.TextMuted)
			ui.TextInputBase(c, &r.query).Bind(&r.input).Grow(1).FontSize(r.theme.Font.Size + 6).
				Placeholder(r.T("launcher.search_placeholder")).Label(r.T("launcher.search_label"))
		})
		ui.Divider(c)
		ui.Column(c).Grow(1).Padding(pad).Center().Children(func() {
			ui.Text(c, r.T("launcher.empty")).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Row(c).Padding(pad-4, pad+4).Gap(pad).AlignItems(ui.Center).Children(func() {
			ui.Text(c, r.T("launcher.title")).FontSize(r.theme.Font.Size - 2).TextColor(t.TextMuted).Grow(1)
			if r.Hotkey != "" {
				ui.Text(c, r.T("launcher.hint_toggle", "hotkey", r.Hotkey)).FontSize(r.theme.Font.Size - 2).TextColor(t.TextMuted)
			}
			ui.Text(c, r.T("launcher.hint_close")).FontSize(r.theme.Font.Size - 2).TextColor(t.TextMuted)
		})
	})
}
