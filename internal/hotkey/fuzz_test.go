package hotkey

import "testing"

// FuzzParse reads anything as a hotkey.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"Alt+Space", "Ctrl+Shift+V", "Win", "ctrl+alt+del", "+", "Ctrl++", "F24", ""} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) { _, _ = Parse(s) })
}
