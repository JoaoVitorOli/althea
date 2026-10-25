package ui

import "github.com/egoist/mygo/ui"

// darkTheme is Althea's built-in dark palette. M1 replaces it with themes loaded from TOML.
var darkTheme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#15171f")
	t.Surface = ui.Hex("#1e2130")
	t.SurfaceHover = ui.Hex("#262a3c")
	t.SurfacePressed = ui.Hex("#2e3247")
	t.Border = ui.Hex("#2e3247")
	t.Text = ui.Hex("#eef0f8")
	t.TextMuted = ui.Hex("#8f95ad")
	t.Accent = ui.Hex("#7b8cf0")
	t.AccentHover = ui.Hex("#95a3f4")
	t.AccentPressed = ui.Hex("#6375e0")
	t.AccentText = ui.Hex("#15171f")
	t.Danger = ui.Hex("#e05a8e")
	t.Warning = ui.Hex("#e8c35a")
	t.Selection = ui.RGBA(123, 140, 240, 0.30)
	t.Focus = ui.RGBA(123, 140, 240, 0.55)
	return t
}()
