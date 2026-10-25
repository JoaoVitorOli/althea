// Package config loads Althea's settings from settings.toml, validates them and reloads them
// when the file changes. See docs/adr/0005-toml-config.md.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Settings are the host's settings. Extensions' preferences join them in M3.
type Settings struct {
	General General `toml:"general"`
	Hotkey  Hotkey  `toml:"hotkey"`
	Window  Window  `toml:"window"`
}

// General holds app-wide settings.
type General struct {
	// Language is "system", "en" or "pt-BR".
	Language string `toml:"language"`
	// Theme is the name of the theme file, without its extension.
	Theme        string `toml:"theme"`
	StartAtLogin bool   `toml:"start_at_login"`
	// CheckUpdates allows the opt-in update check (docs/adr/0012-privacy.md).
	CheckUpdates bool `toml:"check_updates"`
}

// Hotkey holds the global shortcuts.
type Hotkey struct {
	// Toggle shows and hides the launcher: a combination such as "Alt+Space", or "Win".
	Toggle string `toml:"toggle"`
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
		General: General{Language: "system", Theme: "althea-dark", StartAtLogin: true},
		Hotkey:  Hotkey{Toggle: "Alt+Space"},
		Window:  Window{ResetAfter: Duration(90 * time.Second)},
	}
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
	General General `toml:"general"`
	Hotkey  Hotkey  `toml:"hotkey"`
	Window  struct {
		ResetAfter *string `toml:"reset_after"`
	} `toml:"window"`
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
	f := file{General: defaults.General, Hotkey: defaults.Hotkey}
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

	lines := keyLines(data)
	invalid := func(key, message string) {
		problems = append(problems, Problem{Line: lines[key], Key: key, Message: message})
	}

	s := Settings{General: f.General, Hotkey: f.Hotkey, Window: defaults.Window}
	if !slices.Contains(Languages, s.General.Language) {
		invalid("general.language", `must be "system", "en" or "pt-BR"`)
		s.General.Language = defaults.General.Language
	}
	if strings.TrimSpace(s.General.Theme) == "" {
		invalid("general.theme", "must not be empty")
		s.General.Theme = defaults.General.Theme
	}
	if strings.TrimSpace(s.Hotkey.Toggle) == "" {
		invalid("hotkey.toggle", "must not be empty")
		s.Hotkey.Toggle = defaults.Hotkey.Toggle
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
