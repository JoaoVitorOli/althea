// Package theme loads Althea's themes: TOML files of tokens (colors, material, shape, space,
// type and motion) that may extend another theme and override only what they change. Users'
// themes live in the themes directory and shadow the built-in ones of the same name.
// See docs/adr/0006-themes.md.
package theme

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/JoaoVitorOli/althea/internal/tomlpos"
)

// Theme is a resolved theme: every token has a value.
type Theme struct {
	Name       string
	Appearance Appearance
	Colors     Colors
	Material   Material
	Shape      Shape
	Space      Space
	Font       Font
	Motion     Motion
}

// Appearance says whether a theme is light or dark, for the system to match.
type Appearance string

// Appearances.
const (
	Dark  Appearance = "dark"
	Light Appearance = "light"
)

// Colors are the theme's palette.
type Colors struct {
	Background, Surface, SurfaceHover, SurfacePressed, Border Color
	Text, TextMuted                                           Color
	Accent, AccentHover, AccentPressed, AccentText            Color
	Danger, Warning, Success                                  Color
	Selection, Focus, RowSelected                             Color
}

// MaterialKind is what shows behind the launcher.
type MaterialKind string

// Materials. Acrylic and Mica are the system's (Windows 11); Glass is drawn by Althea;
// Solid is the background color alone.
const (
	Acrylic MaterialKind = "acrylic"
	Mica    MaterialKind = "mica"
	Glass   MaterialKind = "glass"
	Solid   MaterialKind = "solid"
)

// Material is the window's backdrop.
type Material struct {
	Kind MaterialKind
	// Opacity is how much the background color covers the material, from 0 to 1.
	Opacity float32
}

// Shape holds corner radii, in DIPs.
type Shape struct {
	WindowRadius, RowRadius, ControlRadius float32
}

// Space holds spacing, in DIPs.
type Space struct {
	Padding, Gap, RowHeight float32
}

// Font is the type of the interface. An empty Family is the system's font.
type Font struct {
	Family string
	Size   float32
}

// Motion tunes animation.
type Motion struct {
	// Scale multiplies durations; 0 turns motion off, as the system's reduce motion does.
	Scale float32
}

// Problem is a mistake in a theme file.
type Problem struct {
	File    string // the file's name, such as "midnight.toml"
	Line    int
	Key     string
	Message string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", p.File, p.Line, p.Key, p.Message)
}

//go:embed builtin/*.toml
var builtinFS embed.FS

// root is the theme every other one starts from unless it extends another.
const root = "althea-dark"

// Builtin returns the names of the themes that ship with Althea.
func Builtin() []string {
	entries, _ := fs.ReadDir(builtinFS, "builtin")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
	}
	return names
}

// List returns the names of every theme, built-in or in dir, sorted, each once.
func List(dir string) []string {
	names := Builtin()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".toml"); ok && !e.IsDir() && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Load resolves the theme called name, looking in dir before the built-in themes. Problems are
// mistakes that leave a token at its inherited value; err is set when the theme, or one it
// extends, cannot be found or parsed, or when themes extend each other in a cycle.
func Load(name, dir string) (Theme, []Problem, error) {
	return resolve(name, dir, false, nil)
}

// resolve loads name and the chain it extends. Built-in themes extend built-in themes only, so
// a user's theme shadowing one cannot pull itself into the chain.
func resolve(name, dir string, builtinOnly bool, chain []link) (Theme, []Problem, error) {
	data, isBuiltin, err := read(name, dir, builtinOnly)
	if err != nil {
		return Theme{}, nil, err
	}
	// A user's theme may share its name with the built-in one it extends: tell them apart.
	here := link{name, isBuiltin}
	if slices.Contains(chain, here) {
		names := make([]string, 0, len(chain)+1)
		for _, l := range append(chain, here) {
			names = append(names, l.name)
		}
		return Theme{}, nil, fmt.Errorf("theme %s extends itself (%s)", chain[0].name, strings.Join(names, " → "))
	}
	chain = append(chain, here)
	file := name + ".toml"
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return Theme{}, nil, fmt.Errorf("theme %s: %w", file, err)
	}

	var base Theme
	var problems []Problem
	switch extends, _ := tree["extends"].(string); {
	case extends != "":
		base, problems, err = resolve(extends, dir, isBuiltin, chain)
	case !isBuiltin || name != root:
		base, problems, err = resolve(root, dir, true, chain)
	}
	if err != nil {
		return Theme{}, nil, err
	}

	t, own := apply(base, tree, tomlpos.Lines(data), file)
	return t, append(problems, own...), nil
}

// link is a theme in a chain of extends.
type link struct {
	name    string
	builtin bool
}

func read(name, dir string, builtinOnly bool) (data []byte, isBuiltin bool, err error) {
	if !builtinOnly && dir != "" {
		data, err = os.ReadFile(filepath.Join(dir, name+".toml"))
		if err == nil {
			return data, false, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, false, fmt.Errorf("theme %s: %w", name, err)
		}
	}
	data, err = builtinFS.ReadFile("builtin/" + name + ".toml")
	if err != nil {
		return nil, false, fmt.Errorf("theme %s not found", name)
	}
	return data, true, nil
}
