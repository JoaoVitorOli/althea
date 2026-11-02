// Package hotkey decides what Althea's global shortcut is: a combination the system registers,
// or the Windows key on its own, which Althea detects itself from the keyboard's events
// (docs/adr/0009-hooks.md).
package hotkey

import (
	"errors"
	"slices"
	"strings"
)

// Virtual-key codes the Win key detection needs.
const (
	VKLWin = 0x5B
	VKRWin = 0x5C
	// VKMask is the unassigned key Althea presses to keep Windows from opening the Start menu
	// on a Win key it used.
	VKMask = 0xE8
)

// KeyEvent is a key going down (or repeating) or up.
type KeyEvent struct {
	VK   uint32
	Down bool
	// Injected events were sent by software, as Althea's own masking key.
	Injected bool
	// Text is what the key types as it goes down, in the keyboard layout of the app the user
	// is in: "" for keys that type nothing, as arrows, and with Ctrl or Alt alone.
	Text string
}

// WinTap detects the Win key pressed and released with no other key held or pressed meanwhile.
// The zero WinTap is ready.
type WinTap struct {
	held  map[uint32]bool // keys other than Win that are down
	win   bool            // Win is down
	spoil bool            // another key took part since Win went down
}

// Feed takes the next keyboard event. Tap reports that Win was tapped alone; mask, that the
// Start menu must be suppressed before the release goes on, which is the case for every tap.
func (w *WinTap) Feed(e KeyEvent) (tap, mask bool) {
	if e.Injected {
		return false, false
	}
	if w.held == nil {
		w.held = map[uint32]bool{}
	}
	isWin := e.VK == VKLWin || e.VK == VKRWin
	switch {
	case isWin && e.Down:
		if !w.win {
			w.win, w.spoil = true, len(w.held) > 0
		}
	case isWin:
		tap = w.win && !w.spoil
		w.win, w.spoil = false, false
		return tap, tap
	case e.Down:
		w.held[e.VK] = true
		if w.win {
			w.spoil = true
		}
	default:
		delete(w.held, e.VK)
	}
	return false, false
}

// Binding is a parsed hotkey.
type Binding struct {
	WinKey      bool   // the Win key on its own
	Accelerator string // otherwise, a combination in MyGo's accelerator syntax
}

var modifiers = []string{"Ctrl", "Alt", "Shift", "Super", "Cmd", "CmdOrCtrl", "Meta"}

// Parse reads a hotkey as settings write it: "Win", or modifiers and a key joined by "+", such
// as "Alt+Space". A key needs at least one modifier, so the hotkey cannot swallow typing.
func Parse(s string) (Binding, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "win") {
		return Binding{WinKey: true}, nil
	}
	parts := strings.Split(s, "+")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return Binding{}, errors.New(`hotkey: want "Win" or a combination such as "Alt+Space"`)
		}
		if j := slices.IndexFunc(modifiers, func(m string) bool { return strings.EqualFold(m, parts[i]) }); j >= 0 {
			parts[i] = modifiers[j]
		} else if len(parts[i]) > 1 {
			parts[i] = strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		} else {
			parts[i] = strings.ToUpper(parts[i])
		}
	}
	key := parts[len(parts)-1]
	if len(parts) < 2 || slices.Contains(modifiers, key) {
		return Binding{}, errors.New("hotkey: a combination needs one or more modifiers and a key, such as Alt+Space")
	}
	// One spelling for each combination, so that two can be compared.
	slices.SortStableFunc(parts[:len(parts)-1], func(a, b string) int {
		return slices.Index(modifiers, a) - slices.Index(modifiers, b)
	})
	return Binding{Accelerator: strings.Join(parts, "+")}, nil
}
