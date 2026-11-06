package ui

import "testing"

// FuzzParseAccelerator reads anything as an action's shortcut.
func FuzzParseAccelerator(f *testing.F) {
	for _, s := range []string{"Ctrl+Shift+C", "Ctrl+Enter", "Alt+F4", "+", "Ctrl+", ""} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) { _, _, _ = ParseAccelerator(s) })
}
