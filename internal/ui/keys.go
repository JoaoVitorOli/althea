package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// keyNames are the names of keys in accelerators, for those whose name is not their character.
var keyNames = map[ui.Key]string{
	ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeyBackspace: "Backspace",
	ui.KeyDelete: "Delete", ui.KeyInsert: "Insert", ui.KeyHome: "Home", ui.KeyEnd: "End",
	ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
	ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/",
	ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
	ui.KeyBackslash: `\`, ui.KeyBackquote: "`",
}

func init() {
	for i := range 26 {
		keyNames[ui.KeyA+ui.Key(i)] = string(rune('A' + i))
	}
	for i := range 10 {
		keyNames[ui.Key0+ui.Key(i)] = string(rune('0' + i))
	}
	for i, k := range []ui.Key{
		ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6,
		ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12,
	} {
		keyNames[k] = "F" + strconv.Itoa(i+1)
	}
}

// Accelerator spells a key pressed with modifiers as settings and MyGo write it, such as
// "Ctrl+Shift+K". A key without a modifier, or a modifier alone, is not a hotkey.
func Accelerator(mods ui.Modifiers, key ui.Key) (string, bool) {
	name, ok := keyNames[key]
	if !ok {
		return "", false
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Super"}} {
		if mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(append(parts, name), "+"), true
}
