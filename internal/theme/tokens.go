package theme

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Color is an RGBA color.
type Color struct{ R, G, B, A uint8 }

// ParseColor reads "#rrggbb" or "#rrggbbaa".
func ParseColor(s string) (Color, bool) {
	digits, ok := strings.CutPrefix(s, "#")
	if !ok || (len(digits) != 6 && len(digits) != 8) {
		return Color{}, false
	}
	if len(digits) == 6 {
		digits += "ff"
	}
	b, err := hex.DecodeString(digits)
	if err != nil {
		return Color{}, false
	}
	return Color{R: b[0], G: b[1], B: b[2], A: b[3]}, true
}

// token is one key a theme file may set, and how it reads into a Theme.
type token struct {
	key string
	set func(t *Theme, v any) (problem string)
}

func color(field func(*Colors) *Color) func(*Theme, any) string {
	return func(t *Theme, v any) string {
		s, _ := v.(string)
		c, ok := ParseColor(s)
		if !ok {
			return `must be a color such as "#7b8cf0" or "#7b8cf080"`
		}
		*field(&t.Colors) = c
		return ""
	}
}

func number(field func(*Theme) *float32, lo, hi float32) func(*Theme, any) string {
	return func(t *Theme, v any) string {
		var f float32
		switch n := v.(type) {
		case int64:
			f = float32(n)
		case float64:
			f = float32(n)
		default:
			return "must be a number"
		}
		if f < lo || f > hi {
			return fmt.Sprintf("must be between %g and %g", lo, hi)
		}
		*field(t) = f
		return ""
	}
}

func oneOf[T ~string](field func(*Theme) *T, values ...T) func(*Theme, any) string {
	return func(t *Theme, v any) string {
		s, _ := v.(string)
		if !slices.Contains(values, T(s)) {
			quoted := make([]string, len(values))
			for i, x := range values {
				quoted[i] = strconv.Quote(string(x))
			}
			return "must be " + strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
		}
		*field(t) = T(s)
		return ""
	}
}

func text(field func(*Theme) *string) func(*Theme, any) string {
	return func(t *Theme, v any) string {
		s, ok := v.(string)
		if !ok {
			return "must be text"
		}
		*field(t) = s
		return ""
	}
}

// tokens are every key a theme file may set.
var tokens = []token{
	{"name", text(func(t *Theme) *string { return &t.Name })},
	{"appearance", oneOf(func(t *Theme) *Appearance { return &t.Appearance }, Dark, Light)},

	{"colors.background", color(func(c *Colors) *Color { return &c.Background })},
	{"colors.surface", color(func(c *Colors) *Color { return &c.Surface })},
	{"colors.surface_hover", color(func(c *Colors) *Color { return &c.SurfaceHover })},
	{"colors.surface_pressed", color(func(c *Colors) *Color { return &c.SurfacePressed })},
	{"colors.border", color(func(c *Colors) *Color { return &c.Border })},
	{"colors.text", color(func(c *Colors) *Color { return &c.Text })},
	{"colors.text_muted", color(func(c *Colors) *Color { return &c.TextMuted })},
	{"colors.accent", color(func(c *Colors) *Color { return &c.Accent })},
	{"colors.accent_hover", color(func(c *Colors) *Color { return &c.AccentHover })},
	{"colors.accent_pressed", color(func(c *Colors) *Color { return &c.AccentPressed })},
	{"colors.accent_text", color(func(c *Colors) *Color { return &c.AccentText })},
	{"colors.danger", color(func(c *Colors) *Color { return &c.Danger })},
	{"colors.warning", color(func(c *Colors) *Color { return &c.Warning })},
	{"colors.success", color(func(c *Colors) *Color { return &c.Success })},
	{"colors.selection", color(func(c *Colors) *Color { return &c.Selection })},
	{"colors.focus", color(func(c *Colors) *Color { return &c.Focus })},
	{"colors.row_selected", color(func(c *Colors) *Color { return &c.RowSelected })},

	{"material.kind", oneOf(func(t *Theme) *MaterialKind { return &t.Material.Kind }, Acrylic, Mica, Glass, Solid)},
	{"material.opacity", number(func(t *Theme) *float32 { return &t.Material.Opacity }, 0, 1)},

	{"shape.window_radius", number(func(t *Theme) *float32 { return &t.Shape.WindowRadius }, 0, 32)},
	{"shape.row_radius", number(func(t *Theme) *float32 { return &t.Shape.RowRadius }, 0, 24)},
	{"shape.control_radius", number(func(t *Theme) *float32 { return &t.Shape.ControlRadius }, 0, 24)},

	{"space.padding", number(func(t *Theme) *float32 { return &t.Space.Padding }, 0, 48)},
	{"space.gap", number(func(t *Theme) *float32 { return &t.Space.Gap }, 0, 32)},
	{"space.row_height", number(func(t *Theme) *float32 { return &t.Space.RowHeight }, 24, 80)},

	{"font.family", text(func(t *Theme) *string { return &t.Font.Family })},
	{"font.size", number(func(t *Theme) *float32 { return &t.Font.Size }, 9, 24)},

	{"motion.scale", number(func(t *Theme) *float32 { return &t.Motion.Scale }, 0, 3)},
}

// meta are keys about the file rather than tokens.
var meta = []string{"schema", "extends"}

// apply sets the tokens a parsed theme file gives over base.
func apply(base Theme, tree map[string]any, lines map[string]int, file string) (Theme, []Problem) {
	t := base
	values := map[string]any{}
	flatten("", tree, values)

	var problems []Problem
	for key, v := range values {
		if slices.Contains(meta, key) {
			continue
		}
		i := slices.IndexFunc(tokens, func(tk token) bool { return tk.key == key })
		msg := "unknown token"
		if i >= 0 {
			msg = tokens[i].set(&t, v)
		}
		if msg != "" {
			problems = append(problems, Problem{File: file, Line: lines[key], Key: key, Message: msg})
		}
	}
	slices.SortFunc(problems, func(a, b Problem) int { return a.Line - b.Line })
	return t, problems
}

func flatten(prefix string, tree, into map[string]any) {
	for k, v := range tree {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flatten(key, sub, into)
			continue
		}
		into[key] = v
	}
}
