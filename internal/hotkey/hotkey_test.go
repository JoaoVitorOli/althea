package hotkey_test

import (
	"testing"

	"github.com/JoaoVitorOli/althea/internal/hotkey"
)

const (
	lwin = hotkey.VKLWin
	rwin = hotkey.VKRWin
	keyE = 0x45
	keyS = 0x53
	shft = 0xA0
)

func down(vk uint32) hotkey.KeyEvent { return hotkey.KeyEvent{VK: vk, Down: true} }
func up(vk uint32) hotkey.KeyEvent   { return hotkey.KeyEvent{VK: vk} }

// feed runs events through a WinTap and counts the taps and the Start menus it suppressed.
func feed(events ...hotkey.KeyEvent) (taps, masks int) {
	var w hotkey.WinTap
	for _, e := range events {
		tap, mask := w.Feed(e)
		if tap {
			taps++
		}
		if mask {
			masks++
		}
	}
	return taps, masks
}

func TestWinTap(t *testing.T) {
	tests := []struct {
		name   string
		events []hotkey.KeyEvent
		taps   int
	}{
		{"Win alone", []hotkey.KeyEvent{down(lwin), up(lwin)}, 1},
		{"right Win alone", []hotkey.KeyEvent{down(rwin), up(rwin)}, 1},
		{"Win held, key repeats", []hotkey.KeyEvent{down(lwin), down(lwin), down(lwin), up(lwin)}, 1},
		{"Win+E opens Explorer, not Althea", []hotkey.KeyEvent{down(lwin), down(keyE), up(keyE), up(lwin)}, 0},
		{"Win+Shift+S", []hotkey.KeyEvent{down(lwin), down(shft), down(keyS), up(keyS), up(shft), up(lwin)}, 0},
		{"Win released after the other key", []hotkey.KeyEvent{down(lwin), down(keyE), up(lwin), up(keyE)}, 0},
		{"Shift held before Win", []hotkey.KeyEvent{down(shft), down(lwin), up(lwin), up(shft)}, 0},
		{"two taps", []hotkey.KeyEvent{down(lwin), up(lwin), down(lwin), up(lwin)}, 2},
		{"a combo, then a tap", []hotkey.KeyEvent{down(lwin), down(keyE), up(keyE), up(lwin), down(lwin), up(lwin)}, 1},
		{"release without a press", []hotkey.KeyEvent{up(lwin)}, 0},
		{"injected keys are ignored", []hotkey.KeyEvent{
			down(lwin), {VK: hotkey.VKMask, Down: true, Injected: true}, {VK: hotkey.VKMask, Injected: true}, up(lwin),
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			taps, masks := feed(tc.events...)
			if taps != tc.taps {
				t.Errorf("taps = %d, want %d", taps, tc.taps)
			}
			if masks != taps {
				t.Errorf("Start menu suppressed %d times for %d taps", masks, taps)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want hotkey.Binding
		ok   bool
	}{
		{"Win", hotkey.Binding{WinKey: true}, true},
		{" win ", hotkey.Binding{WinKey: true}, true},
		{"Alt+Space", hotkey.Binding{Accelerator: "Alt+Space"}, true},
		{"ctrl + shift + space", hotkey.Binding{Accelerator: "Ctrl+Shift+Space"}, true},
		{"Super+Space", hotkey.Binding{Accelerator: "Super+Space"}, true},
		{"shift+ctrl+k", hotkey.Binding{Accelerator: "Ctrl+Shift+K"}, true},
		{"Control+K", hotkey.Binding{Accelerator: "Ctrl+K"}, true},
		{"ctrl+alt+pageup", hotkey.Binding{Accelerator: "Ctrl+Alt+Pageup"}, true},
		{"Ctrl+F12", hotkey.Binding{Accelerator: "Ctrl+F12"}, true},
		{"Ctrl+Num5", hotkey.Binding{Accelerator: "Ctrl+Num5"}, true},
		{"Ctrl+Q+P", hotkey.Binding{}, false}, // two keys: the system registers one
		{"Q+W+E+O+P", hotkey.Binding{}, false},
		{"Ctrl+Banana", hotkey.Binding{}, false},
		{"Ctrl+F25", hotkey.Binding{}, false},
		{"", hotkey.Binding{}, false},
		{"Alt+", hotkey.Binding{}, false},
		{"Ctrl+Shift", hotkey.Binding{}, false}, // modifiers alone
		{"Space", hotkey.Binding{}, false},      // a key alone would swallow it everywhere
	}
	for _, tc := range tests {
		got, err := hotkey.Parse(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v, ok %v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}
