package theme

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/unicode/norm"
)

// Hex writes a color as a theme file does: "#rrggbb", with the alpha only when it is not opaque.
func (c Color) Hex() string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// values are a theme's tokens by key, with the values a theme file writes.
func values(t Theme) map[string]any {
	c := t.Colors
	return map[string]any{
		"name": t.Name, "appearance": string(t.Appearance),
		"colors.background": c.Background, "colors.surface": c.Surface,
		"colors.surface_hover": c.SurfaceHover, "colors.surface_pressed": c.SurfacePressed,
		"colors.border": c.Border, "colors.text": c.Text, "colors.text_muted": c.TextMuted,
		"colors.accent": c.Accent, "colors.accent_hover": c.AccentHover,
		"colors.accent_pressed": c.AccentPressed, "colors.accent_text": c.AccentText,
		"colors.danger": c.Danger, "colors.warning": c.Warning, "colors.success": c.Success,
		"colors.selection": c.Selection, "colors.focus": c.Focus, "colors.row_selected": c.RowSelected,
		"material.kind": string(t.Material.Kind), "material.opacity": t.Material.Opacity,
		"shape.window_radius": t.Shape.WindowRadius, "shape.row_radius": t.Shape.RowRadius,
		"shape.control_radius": t.Shape.ControlRadius,
		"space.padding":        t.Space.Padding, "space.gap": t.Space.Gap, "space.row_height": t.Space.RowHeight,
		"font.family": t.Font.Family, "font.size": t.Font.Size,
		"motion.scale": t.Motion.Scale,
	}
}

// Encode writes t as a theme file that extends the theme called extends, whose resolved tokens
// are base: it sets the name and appearance, and the tokens where t differs from base, by
// section in the order of the reference.
func Encode(t, base Theme, extends string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s — made with Althea's theme editor. Tokens left out are %s's.\n", t.Name, extends)
	fmt.Fprintf(&b, "schema = 1\nextends = %s\nname = %s\nappearance = %s\n",
		strconv.Quote(extends), strconv.Quote(t.Name), strconv.Quote(string(t.Appearance)))
	mine, theirs := values(t), values(base)
	section := ""
	for _, tk := range tokens {
		sec, key, ok := strings.Cut(tk.key, ".")
		if !ok || mine[tk.key] == theirs[tk.key] {
			continue
		}
		if sec != section {
			fmt.Fprintf(&b, "\n[%s]\n", sec)
			section = sec
		}
		fmt.Fprintf(&b, "%s = %s\n", key, literal(mine[tk.key]))
	}
	return b.Bytes()
}

func literal(v any) string {
	switch v := v.(type) {
	case Color:
		return strconv.Quote(v.Hex())
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case string:
		return strconv.Quote(v)
	}
	return fmt.Sprint(v)
}

// Slug turns a theme's name into its file's, as "Café Noir" into "cafe-noir"; "custom" when
// nothing of it is left.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
	}
	if b.Len() == 0 {
		return "custom"
	}
	return b.String()
}

// Save writes a theme file into dir as slug.toml, whole or not at all.
func Save(dir, slug string, data []byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, slug+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, slug+".toml"))
}

// ErrBuiltinName is the error of saving a theme under a built-in theme's name.
var ErrBuiltinName = errors.New("a built-in theme has that name")

// Write saves t as the user's theme called by its name, in dir, extending the theme called from,
// and returns its file's name without ".toml", which the settings choose it by. Saving over the
// theme it comes from extends that one's parent in its place, so that it never extends itself.
func Write(dir, from string, t Theme) (string, error) {
	slug := Slug(t.Name)
	if slices.Contains(Builtin(), slug) {
		return "", ErrBuiltinName
	}
	extends := from
	if from == slug {
		var err error
		if extends, err = parent(from, dir); err != nil {
			return "", err
		}
	}
	base, _, err := Load(extends, dir)
	if err != nil {
		return "", err
	}
	return slug, Save(dir, slug, Encode(t, base, extends))
}

// parent is the theme the user's theme called name extends.
func parent(name, dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".toml"))
	if err != nil {
		return "", err
	}
	var tree struct {
		Extends string `toml:"extends"`
	}
	if err := toml.Unmarshal(data, &tree); err != nil {
		return "", err
	}
	if tree.Extends == "" || tree.Extends == name {
		return root, nil
	}
	return tree.Extends, nil
}
