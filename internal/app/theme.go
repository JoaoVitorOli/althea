package app

import (
	"strings"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/theme"
)

// themeName is the theme the settings choose, given whether the system is in dark mode.
func themeName(s config.Theme, systemDark bool) string {
	switch {
	case s.Name != "system":
		return s.Name
	case systemDark:
		return s.Dark
	default:
		return s.Light
	}
}

// loadTheme loads the theme the settings choose, falling back to the built-in theme of the same
// appearance when it cannot be loaded, so a broken theme file never leaves Althea unthemed.
func (c *core) loadTheme(s config.Theme, systemDark bool) theme.Theme {
	name := themeName(s, systemDark)
	t, problems, err := theme.Load(name, c.paths.Themes)
	for _, p := range problems {
		c.log.Warn("theme problem", "file", p.File, "line", p.Line, "key", p.Key, "problem", p.Message)
	}
	if err == nil {
		return t
	}
	c.log.Warn("theme not loaded; using a built-in one", "theme", name, "err", err)
	fallback := "althea-light"
	if systemDark || strings.Contains(name, "dark") {
		fallback = "althea-dark"
	}
	t, _, _ = theme.Load(fallback, "")
	return t
}
