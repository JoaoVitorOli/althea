package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAccelerator(t *testing.T) {
	tests := []struct {
		mods ui.Modifiers
		key  ui.Key
		want string
		ok   bool
	}{
		{ui.Alt, ui.KeySpace, "Alt+Space", true},
		{ui.Ctrl | ui.Shift, ui.KeyK, "Ctrl+Shift+K", true},
		{ui.Super | ui.Alt, ui.Key1, "Alt+Super+1", true},
		{ui.Ctrl, ui.KeyF5, "Ctrl+F5", true},
		{ui.Ctrl | ui.Alt, ui.KeySlash, "Ctrl+Alt+/", true},
		{0, ui.KeySpace, "", false},         // a key alone
		{ui.Ctrl, ui.KeyUnknown, "", false}, // a modifier alone, as it goes down
	}
	for _, tc := range tests {
		got, ok := Accelerator(tc.mods, tc.key)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Accelerator(%v, %v) = %q, %v; want %q, %v", tc.mods, tc.key, got, ok, tc.want, tc.ok)
		}
	}
}
