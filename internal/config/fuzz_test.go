package config

import "testing"

// FuzzParse reads anything as settings.toml: settings with problems, or an error, never a panic.
func FuzzParse(f *testing.F) {
	f.Add(defaultFile)
	for _, s := range []string{
		"[general]\nlanguage = 1", "theme = { name = \"x\" }", "[hotkey]\ntoggle = \"Ctrl+\"",
		"[extensions.x]\na = [1, {b = 2}]", "= =", "[[general]]",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _, _ = parse(data)
	})
}
