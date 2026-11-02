// Package config loads Althea's settings from settings.toml, validates them and reloads them
// when the file changes. See docs/adr/0005-toml-config.md.
package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/tomlpos"
)

// Settings are the host's settings, and the extensions' preferences as written.
type Settings struct {
	General General `toml:"general"`
	Theme   Theme   `toml:"theme"`
	Hotkey  Hotkey  `toml:"hotkey"`
	Window  Window  `toml:"window"`
	// Extensions are the [extensions."<id>"] tables, by extension ID, unchecked: only the
	// extension knows its preferences (see internal/prefs).
	Extensions map[string]map[string]any `toml:"extensions"`
}

// General holds app-wide settings.
type General struct {
	// Language is "system", "en" or "pt-BR".
	Language     string `toml:"language"`
	StartAtLogin bool   `toml:"start_at_login"`
	// CheckUpdates allows the opt-in update check (docs/adr/0012-privacy.md).
	CheckUpdates bool `toml:"check_updates"`
}

// Theme chooses the theme, by the name of its file without .toml.
type Theme struct {
	// Name is a theme, or "system" to follow the system between Light and Dark.
	Name  string `toml:"name"`
	Dark  string `toml:"dark"`
	Light string `toml:"light"`
}

// Hotkey holds the global shortcuts.
type Hotkey struct {
	// Toggle shows and hides the launcher: a combination such as "Alt+Space", or "Win".
	Toggle string `toml:"toggle"`
	// Commands run commands of the extensions from anywhere: a combination by the command's key,
	// "<extension>/<command>".
	Commands map[string]string `toml:"commands"`
}

// Window holds the launcher window's behavior.
type Window struct {
	// ResetAfter is how long the launcher keeps its state while hidden before it opens at the
	// root again; 0 always opens at the root.
	ResetAfter Duration `toml:"reset_after"`
}

// Duration is a time.Duration written as text in TOML, such as "90s" or "2m".
type Duration time.Duration

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Languages are the values General.Language accepts.
var Languages = []string{"system", "en", "pt-BR"}

// Defaults returns the settings used where the file sets nothing.
func Defaults() Settings {
	return Settings{
		General: General{Language: "system", StartAtLogin: true},
		Theme:   Theme{Name: "system", Dark: "althea-dark", Light: "althea-light"},
		Hotkey:  Hotkey{Toggle: "Alt+Space"},
		Window:  Window{ResetAfter: Duration(90 * time.Second)},
	}
}

//go:embed default.toml
var defaultFile []byte

// WriteDefault writes the commented settings file of the defaults at path, creating its
// directory, unless a file is already there.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write default settings: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("write default settings: %w", err)
	}
	if _, err := f.Write(defaultFile); err != nil {
		_ = f.Close()
		return fmt.Errorf("write default settings: %w", err)
	}
	return f.Close()
}

// Problem is a mistake in the settings file.
type Problem struct {
	Line    int    // 1-based; 0 when unknown
	Key     string // dotted, such as "general.language"
	Message string
}

func (p Problem) String() string {
	return fmt.Sprintf("line %d: %s: %s", p.Line, p.Key, p.Message)
}

// file is the settings file as written: values that need parsing or checking are read as
// text first, so a mistake in one costs only that setting.
type file struct {
	// Schema is the version of the file's format, 1 so far.
	Schema  int     `toml:"schema"`
	General General `toml:"general"`
	Theme   Theme   `toml:"theme"`
	Hotkey  struct {
		Toggle   string         `toml:"toggle"`
		Commands map[string]any `toml:"commands"`
	} `toml:"hotkey"`
	Window struct {
		ResetAfter *string `toml:"reset_after"`
	} `toml:"window"`
	Extensions map[string]map[string]any `toml:"extensions"`
}

// Load reads the settings file at path. A missing file gives the defaults. Problems are
// mistakes in an otherwise readable file, each leaving its setting at the default; err is set
// when the file cannot be read or parsed at all.
func Load(path string) (s Settings, problems []Problem, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil, nil
	}
	if err != nil {
		return Settings{}, nil, fmt.Errorf("read settings: %w", err)
	}
	return parse(data)
}

func parse(data []byte) (Settings, []Problem, error) {
	defaults := Defaults()
	f := file{General: defaults.General, Theme: defaults.Theme}
	f.Hotkey.Toggle = defaults.Hotkey.Toggle
	var problems []Problem

	err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&f)
	var unknown *toml.StrictMissingError
	var syntax *toml.DecodeError
	switch {
	case errors.As(err, &unknown):
		for _, e := range unknown.Errors {
			line, _ := e.Position()
			problems = append(problems, Problem{
				Line: line, Key: strings.Join(e.Key(), "."), Message: "unknown setting",
			})
		}
	case errors.As(err, &syntax):
		line, col := syntax.Position()
		return Settings{}, nil, fmt.Errorf("settings: line %d, column %d: %w", line, col, err)
	case err != nil:
		return Settings{}, nil, fmt.Errorf("settings: %w", err)
	}

	lines := tomlpos.Lines(data)
	invalid := func(key, message string) {
		problems = append(problems, Problem{Line: lines[key], Key: key, Message: message})
	}

	s := Settings{
		General: f.General, Theme: f.Theme, Hotkey: Hotkey{Toggle: f.Hotkey.Toggle},
		Window: defaults.Window, Extensions: f.Extensions,
	}
	if !slices.Contains(Languages, s.General.Language) {
		invalid("general.language", `must be "system", "en" or "pt-BR"`)
		s.General.Language = defaults.General.Language
	}
	nonEmpty := func(key string, v *string, fallback string) {
		if strings.TrimSpace(*v) == "" {
			invalid(key, "must not be empty")
			*v = fallback
		}
	}
	nonEmpty("theme.name", &s.Theme.Name, defaults.Theme.Name)
	nonEmpty("theme.dark", &s.Theme.Dark, defaults.Theme.Dark)
	nonEmpty("theme.light", &s.Theme.Light, defaults.Theme.Light)
	if _, err := hotkey.Parse(s.Hotkey.Toggle); err != nil {
		invalid("hotkey.toggle", `must be "Win" or a combination such as "Alt+Space"`)
		s.Hotkey.Toggle = defaults.Hotkey.Toggle
	}
	for _, command := range slices.Sorted(maps.Keys(f.Hotkey.Commands)) {
		text, ok := f.Hotkey.Commands[command].(string)
		b, err := hotkey.Parse(text)
		if !ok || err != nil || b.WinKey {
			invalid("hotkey.commands."+command, `must be a combination such as "Ctrl+Alt+Left"`)
			continue
		}
		if s.Hotkey.Commands == nil {
			s.Hotkey.Commands = map[string]string{}
		}
		s.Hotkey.Commands[command] = text
	}
	if text := f.Window.ResetAfter; text != nil {
		d, err := time.ParseDuration(*text)
		switch {
		case err != nil:
			invalid("window.reset_after", `must be a duration such as "90s" or "2m"`)
		case d < 0:
			invalid("window.reset_after", "must not be negative")
		default:
			s.Window.ResetAfter = Duration(d)
		}
	}
	return s, problems, nil
}
