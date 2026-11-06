package app

import (
	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/theme"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// themeEditor is the theme editor's window, made the first time it opens. Its fields are used on
// the main thread.
type themeEditor struct {
	win  *shell.ThemeEditorWindow
	view *ui.ThemeEditor
}

// openThemeEditor shows the theme editor on the theme in use.
func (a *althea) openThemeEditor() {
	e := &a.themes
	if e.win == nil {
		e.view = ui.NewThemeEditor(a.catalog.T, ui.ThemeBackend{
			Names: func() []string { return theme.List(a.paths.Themes) },
			Load: func(name string) (theme.Theme, error) {
				t, _, err := theme.Load(name, a.paths.Themes)
				return t, err
			},
			Save: a.saveTheme,
		}, a.root.Theme())
		e.win = shell.OpenThemeEditor(a.catalog.T("theme_editor.title"), e.view.View)
	}
	e.view.Start(themeName(a.Settings().Theme, a.root.Theme().Appearance == theme.Dark))
	e.win.Show()
}

// saveTheme writes an edited theme and chooses it in the settings: as the theme, or as the light
// or dark one when the settings follow the system. The watchers apply both.
func (a *althea) saveTheme(from string, t theme.Theme) (string, error) {
	name, err := theme.Write(a.paths.Themes, from, t)
	if err != nil {
		return "", err
	}
	key := "theme.name"
	if a.Settings().Theme.Name == "system" {
		key = "theme.light"
		if t.Appearance == theme.Dark {
			key = "theme.dark"
		}
	}
	if err := config.Set(a.paths.Settings, key, name); err != nil {
		return "", err
	}
	a.log.Info("theme saved", "theme", name)
	return name, nil
}

// setThemeChrome draws the editor with the theme in use.
func (e *themeEditor) setChrome(t theme.Theme) {
	if e.win == nil {
		return
	}
	e.view.SetChrome(t)
	e.win.Window().Invalidate()
}
