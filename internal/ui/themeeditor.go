package ui

import (
	"errors"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// ThemeBackend is where the theme editor's themes come from and go to.
type ThemeBackend struct {
	// Names are the themes there are, by the names the settings choose them by.
	Names func() []string
	// Load resolves a theme by name.
	Load func(name string) (theme.Theme, error)
	// Save writes t as a theme extending from and makes it the one in use, returning its name.
	// It fails with theme.ErrBuiltinName for a built-in theme's name.
	Save func(from string, t theme.Theme) (string, error)
}

// colorTokens are the colors the editor sets, by their key under [colors].
var colorTokens = []struct {
	key string
	get func(*theme.Colors) *theme.Color
}{
	{"background", func(c *theme.Colors) *theme.Color { return &c.Background }},
	{"surface", func(c *theme.Colors) *theme.Color { return &c.Surface }},
	{"surface_hover", func(c *theme.Colors) *theme.Color { return &c.SurfaceHover }},
	{"surface_pressed", func(c *theme.Colors) *theme.Color { return &c.SurfacePressed }},
	{"border", func(c *theme.Colors) *theme.Color { return &c.Border }},
	{"row_selected", func(c *theme.Colors) *theme.Color { return &c.RowSelected }},
	{"text", func(c *theme.Colors) *theme.Color { return &c.Text }},
	{"text_muted", func(c *theme.Colors) *theme.Color { return &c.TextMuted }},
	{"accent", func(c *theme.Colors) *theme.Color { return &c.Accent }},
	{"accent_hover", func(c *theme.Colors) *theme.Color { return &c.AccentHover }},
	{"accent_pressed", func(c *theme.Colors) *theme.Color { return &c.AccentPressed }},
	{"accent_text", func(c *theme.Colors) *theme.Color { return &c.AccentText }},
	{"selection", func(c *theme.Colors) *theme.Color { return &c.Selection }},
	{"focus", func(c *theme.Colors) *theme.Color { return &c.Focus }},
	{"danger", func(c *theme.Colors) *theme.Color { return &c.Danger }},
	{"warning", func(c *theme.Colors) *theme.Color { return &c.Warning }},
	{"success", func(c *theme.Colors) *theme.Color { return &c.Success }},
}

// numberToken is a number the editor sets with a slider.
type numberToken struct {
	key      string // under its section in the messages, as "shape.row_radius"
	lo, hi   float64
	step     float64
	get      func(*theme.Theme) *float32
	percents bool // shown as a percentage
}

var numberSections = []struct {
	key    string
	tokens []numberToken
}{
	{"material", []numberToken{
		{"material.opacity", 0, 1, 0.01, func(t *theme.Theme) *float32 { return &t.Material.Opacity }, true},
	}},
	{"shape", []numberToken{
		{"shape.window_radius", 0, 32, 1, func(t *theme.Theme) *float32 { return &t.Shape.WindowRadius }, false},
		{"shape.row_radius", 0, 24, 1, func(t *theme.Theme) *float32 { return &t.Shape.RowRadius }, false},
		{"shape.control_radius", 0, 24, 1, func(t *theme.Theme) *float32 { return &t.Shape.ControlRadius }, false},
	}},
	{"space", []numberToken{
		{"space.padding", 0, 48, 1, func(t *theme.Theme) *float32 { return &t.Space.Padding }, false},
		{"space.gap", 0, 32, 1, func(t *theme.Theme) *float32 { return &t.Space.Gap }, false},
		{"space.row_height", 24, 80, 1, func(t *theme.Theme) *float32 { return &t.Space.RowHeight }, false},
	}},
	{"font", []numberToken{
		{"font.size", 9, 24, 1, func(t *theme.Theme) *float32 { return &t.Font.Size }, false},
	}},
	{"motion", []numberToken{
		{"motion.scale", 0, 3, 0.1, func(t *theme.Theme) *float32 { return &t.Motion.Scale }, true},
	}},
}

var materials = []string{string(theme.Acrylic), string(theme.Mica), string(theme.Glass), string(theme.Solid)}

// ThemeEditor is the theme editor's window: the theme's tokens beside a launcher drawn with them.
type ThemeEditor struct {
	T       func(key string, args ...any) string
	Backend ThemeBackend

	// chrome is the theme the editor itself is drawn with: the one in use.
	chrome theme.Theme
	ui     *ui.Theme

	names    []string
	from     string // the theme edited, by name
	base     theme.Theme
	edit     theme.Theme
	name     string
	notice   string
	problem  bool
	baseName string // the select's value, followed to change from
}

// NewThemeEditor returns the editor, drawn with the theme in use.
func NewThemeEditor(t func(string, ...any) string, b ThemeBackend, chrome theme.Theme) *ThemeEditor {
	e := &ThemeEditor{T: t, Backend: b}
	e.SetChrome(chrome)
	return e
}

// SetChrome changes the theme the editor itself is drawn with.
func (e *ThemeEditor) SetChrome(th theme.Theme) { e.chrome, e.ui = th, ToUI(th) }

// Start edits the theme called name.
func (e *ThemeEditor) Start(name string) {
	e.names = e.Backend.Names()
	e.choose(name)
}

// choose starts over from the theme called name, keeping the name typed once there is one.
func (e *ThemeEditor) choose(name string) {
	t, err := e.Backend.Load(name)
	if err != nil {
		e.notice, e.problem = err.Error(), true
		return
	}
	e.from, e.baseName, e.base, e.edit = name, name, t, t
	e.notice, e.problem = "", false
	if e.name == "" || e.name == e.base.Name {
		e.name = t.Name
		if isBuiltin(name) {
			e.name = e.T("theme_editor.copy_of", "name", t.Name)
		}
	}
}

func isBuiltin(name string) bool {
	for _, b := range theme.Builtin() {
		if b == name {
			return true
		}
	}
	return false
}

// Edited is the theme as the editor has it now.
func (e *ThemeEditor) Edited() theme.Theme {
	t := e.edit
	t.Name = e.name
	return t
}

// View builds the editor for one frame.
func (e *ThemeEditor) View(c *ui.Context) {
	c.SetTheme(e.ui)
	t := c.Theme()
	bg := t.Background
	bg.A = 255
	c.Root().Background(bg)
	if e.baseName != e.from && e.baseName != "" {
		e.choose(e.baseName)
	}
	if c.Shortcut(ui.Ctrl, ui.KeyS) {
		e.save()
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(380).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(16, 18).Gap(10).AlignItems(ui.Stretch).Children(func() { e.controls(c) })
			})
			ui.Divider(c)
			e.bar(c)
		})
		ui.Box(c).Width(1).Background(t.Border)
		ui.Box(c).Grow(1).Padding(28).Center().Gradient(
			ui.Color{R: 0x3a, G: 0x4f, B: 0x9a, A: 255}, ui.Color{R: 0xb0, G: 0x5c, B: 0x8e, A: 255}, 135,
		).Children(func() { e.preview(c, e.Edited()) })
	})
}

func (e *ThemeEditor) heading(c *ui.Context, key string) {
	ui.Text(c, e.T("theme_editor.section."+key)).FontWeight(600).FontSize(e.chrome.Font.Size-1).
		TextColor(c.Theme().TextMuted).Margin(10, 0, 0, 0)
}

// row is a labeled control.
func (e *ThemeEditor) row(c *ui.Context, label string, control func()) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).MinHeight(30).Children(func() {
		ui.Text(c, label).TextColor(c.Theme().Text).Grow(1).SingleLine()
		control()
	})
}

func (e *ThemeEditor) controls(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, e.T("theme_editor.title")).FontSize(e.chrome.Font.Size + 4).FontWeight(700).TextColor(t.Text)
	e.row(c, e.T("theme_editor.name"), func() {
		ui.TextInput(c, &e.name).Width(200).Label(e.T("theme_editor.name"))
	})
	e.row(c, e.T("theme_editor.based_on"), func() {
		ui.Select(c, &e.baseName, e.names).Width(200).Label(e.T("theme_editor.based_on"))
	})
	appearance := string(e.edit.Appearance)
	e.row(c, e.T("theme_editor.appearance"), func() {
		if ui.Select(c, &appearance, []string{string(theme.Dark), string(theme.Light)}).Width(200).
			Label(e.T("theme_editor.appearance")).Changed() {
			e.edit.Appearance = theme.Appearance(appearance)
		}
	})

	e.heading(c, "colors")
	for _, tk := range colorTokens {
		p := tk.get(&e.edit.Colors)
		well := color(*p)
		label := e.T("theme_editor.color." + tk.key)
		e.row(c, label, func() {
			ui.Text(c, p.Hex()).FontSize(e.chrome.Font.Size - 2).TextColor(t.TextMuted)
			if ui.ColorWell(c, &well).Label(label).Changed() {
				*p = theme.Color{R: well.R, G: well.G, B: well.B, A: well.A}
			}
		})
	}

	for _, sec := range numberSections {
		e.heading(c, sec.key)
		if sec.key == "material" {
			kind := string(e.edit.Material.Kind)
			e.row(c, e.T("theme_editor.material_kind"), func() {
				if ui.Select(c, &kind, materials).Width(200).Label(e.T("theme_editor.material_kind")).Changed() {
					e.edit.Material.Kind = theme.MaterialKind(kind)
				}
			})
		}
		if sec.key == "font" {
			e.row(c, e.T("theme_editor.font.family"), func() {
				ui.TextInput(c, &e.edit.Font.Family).Width(200).Placeholder(e.T("theme_editor.system_font")).
					Label(e.T("theme_editor.font.family"))
			})
		}
		for _, tk := range sec.tokens {
			e.number(c, tk)
		}
	}
}

func (e *ThemeEditor) number(c *ui.Context, tk numberToken) {
	p := tk.get(&e.edit)
	v := float64(*p)
	label := e.T("theme_editor." + tk.key)
	e.row(c, label, func() {
		shown := strconv.FormatFloat(v, 'f', -1, 32)
		if tk.percents {
			shown = strconv.Itoa(int(v*100+0.5)) + "%"
		}
		ui.Text(c, shown).FontSize(e.chrome.Font.Size - 2).TextColor(c.Theme().TextMuted).MinWidth(34)
		if ui.StepSlider(c, &v, tk.lo, tk.hi, tk.step).Width(140).Label(label).Changed() {
			*p = float32(v)
		}
	})
}

// bar holds what is done with the theme: starting over and saving.
func (e *ThemeEditor) bar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(10, 18).Gap(8).AlignItems(ui.Center).Children(func() {
		note := ui.Text(c, e.notice).FontSize(e.chrome.Font.Size - 2).Grow(1)
		if e.problem {
			note.TextColor(t.Danger)
		} else {
			note.TextColor(t.TextMuted)
		}
		changed := e.edit != e.base
		if ui.Button(c, e.T("theme_editor.reset")).Disabled(!changed).Clicked() {
			e.edit = e.base
			e.notice = ""
		}
		if ui.PrimaryButton(c, e.T("theme_editor.save")).Tooltip("Ctrl+S").Clicked() {
			e.save()
		}
	})
}

func (e *ThemeEditor) save() {
	if e.name == "" {
		e.notice, e.problem = e.T("theme_editor.needs_name"), true
		return
	}
	t := e.Edited()
	name, err := e.Backend.Save(e.from, t)
	switch {
	case errors.Is(err, theme.ErrBuiltinName):
		e.notice, e.problem = e.T("theme_editor.builtin_name"), true
	case err != nil:
		e.notice, e.problem = err.Error(), true
	default:
		e.names = e.Backend.Names()
		e.from, e.baseName, e.base, e.edit = name, name, t, t
		e.notice, e.problem = e.T("theme_editor.saved", "file", name+".toml"), false
	}
}

// preview draws a launcher with th, as it shows over the desktop.
func (e *ThemeEditor) preview(c *ui.Context, th theme.Theme) {
	col := th.Colors
	bg := color(col.Background)
	if th.Material.Kind == theme.Solid {
		bg.A = 255
	} else {
		bg = bg.Alpha(th.Material.Opacity * float32(bg.A) / 255)
	}
	pad, size := th.Space.Padding, th.Font.Size
	text := func(s string) ui.Element {
		return ui.Text(c, s).TextColor(color(col.Text)).FontSize(size).Font(th.Font.Family).SingleLine()
	}
	muted := func(s string, d float32) ui.Element {
		return ui.Text(c, s).TextColor(color(col.TextMuted)).FontSize(size + d).Font(th.Font.Family).SingleLine()
	}
	line := func() { ui.Box(c).Height(1).Background(color(col.Border)) }

	ui.Column(c).Width(480).MaxWidth(480).AlignItems(ui.Stretch).Radius(th.Shape.WindowRadius).Clip().
		Background(bg).Border(1, color(col.Border)).Shadow(0, 18, 48, 0, ui.Color{A: 90}).Children(func() {
		ui.Row(c).Padding(pad+2, pad+6).Gap(pad).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(20).TextColor(color(col.TextMuted))
			muted(e.T("launcher.search_placeholder"), 6).Grow(1)
		})
		line()
		ui.Column(c).Padding(6).Gap(th.Space.Gap / 2).AlignItems(ui.Stretch).Children(func() {
			rows := []struct{ glyph, title, accessory string }{
				{"calculator", "2 + 2 = 4", e.T("theme_editor.sample.answer")},
				{"note", e.T("theme_editor.sample.notes"), e.T("theme_editor.sample.command")},
				{"folder", e.T("theme_editor.sample.files"), e.T("theme_editor.sample.command")},
				{"settings", e.T("theme_editor.sample.settings"), e.T("theme_editor.sample.command")},
			}
			for i, r := range rows {
				row := ui.Row(c).Height(th.Space.RowHeight).Padding(0, pad).Gap(pad).AlignItems(ui.Center).
					Radius(th.Shape.RowRadius)
				if i == 0 {
					row.Background(color(col.RowSelected))
				}
				row.Children(func() {
					ui.Row(c).Size(26, 26).Center().Radius(th.Shape.ControlRadius).Background(color(col.Surface)).
						Children(func() { ui.Icon(c, glyphs[r.glyph]).FontSize(15).TextColor(color(col.Text)) })
					text(r.title).Grow(1)
					muted(r.accessory, -2)
				})
			}
		})
		line()
		ui.Row(c).Padding(pad-4, pad+2).Gap(8).AlignItems(ui.Center).Children(func() {
			muted("Althea", -2).Grow(1)
			for _, s := range []struct{ c theme.Color }{{col.Danger}, {col.Warning}, {col.Success}} {
				ui.Box(c).Size(8, 8).Radius(4).Background(color(s.c))
			}
			ui.Row(c).Padding(4, 10).Radius(th.Shape.ControlRadius).Background(color(col.Accent)).Children(func() {
				ui.Text(c, e.T("theme_editor.sample.open")).FontSize(size - 1).FontWeight(600).
					TextColor(color(col.AccentText)).Font(th.Font.Family)
			})
		})
	})
}
