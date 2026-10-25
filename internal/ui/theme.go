package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// ToUI returns the MyGo theme of an Althea theme, for the toolkit's own widgets. Tokens MyGo
// has no field for (materials, row metrics, motion) are read from the Althea theme directly.
func ToUI(t theme.Theme) *ui.Theme {
	u := ui.DarkTheme()
	if t.Appearance == theme.Light {
		u = ui.LightTheme()
	}
	c := t.Colors
	u.Background = color(c.Background)
	u.Surface = color(c.Surface)
	u.SurfaceHover = color(c.SurfaceHover)
	u.SurfacePressed = color(c.SurfacePressed)
	u.Border = color(c.Border)
	u.Text = color(c.Text)
	u.TextMuted = color(c.TextMuted)
	u.Accent = color(c.Accent)
	u.AccentHover = color(c.AccentHover)
	u.AccentPressed = color(c.AccentPressed)
	u.AccentText = color(c.AccentText)
	u.Danger = color(c.Danger)
	u.Warning = color(c.Warning)
	u.Success = color(c.Success)
	u.Selection = color(c.Selection)
	u.Focus = color(c.Focus)
	u.Radius = t.Shape.ControlRadius
	u.Spacing = t.Space.Gap
	u.FontSize = t.Font.Size
	u.Font = t.Font.Family
	return u
}

func color(c theme.Color) ui.Color { return ui.Color{R: c.R, G: c.G, B: c.B, A: c.A} }
