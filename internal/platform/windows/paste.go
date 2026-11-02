//go:build windows

package windows

import (
	"errors"
	"fmt"
	"slices"
	"syscall"
	"time"
	"unsafe"
)

var (
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsWindow            = user32.NewProc("IsWindow")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")

	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
)

const (
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkLWin    = 0x5B
	vkRWin    = 0x5C
	vkV       = 0x56
	vkC       = 0x43
	vkLeft    = 0x25

	keyeventfExtendedKey = 0x0001
	// maxCaretBack bounds the presses of Left after a paste.
	maxCaretBack = 2000
	// copyTimeout is how long an app has to answer Ctrl+C; with nothing selected none does.
	copyTimeout = 300 * time.Millisecond
)

// Paster pastes into the window the user was in before Althea showed.
type Paster struct{}

// Foreground implements platform.Paster.
func (Paster) Foreground() uintptr {
	h, _, _ := procGetForegroundWindow.Call()
	return h
}

// Paste implements platform.Paster: it gives window the keyboard back, waits until it has it,
// and presses Ctrl+V there, then Left back times. The app reads the keys in order, so the caret
// moves once the paste is done.
func (Paster) Paste(window uintptr, back int) error {
	if window == 0 {
		return errors.New("no window to paste into")
	}
	if ok, _, _ := procIsWindow.Call(window); ok == 0 {
		return errors.New("the window to paste into has closed")
	}
	if !giveKeyboard(window) {
		return errors.New("the window to paste into did not take the keyboard")
	}
	in := chord(vkV)
	for range min(back, maxCaretBack) {
		in = append(in,
			keyboardInput{typ: inputKeyboard, vk: vkLeft, flags: keyeventfExtendedKey},
			keyboardInput{typ: inputKeyboard, vk: vkLeft, flags: keyeventfExtendedKey | keyeventfKeyUp},
		)
	}
	return press(in, "Ctrl+V")
}

// CopySelection implements platform.Paster.
func (Paster) CopySelection(window uintptr) (bool, error) {
	if window == 0 {
		return false, nil
	}
	if ok, _, _ := procIsWindow.Call(window); ok == 0 || isTerminal(window) {
		return false, nil
	}
	if !giveKeyboard(window) {
		return false, errors.New("the window with the selection did not take the keyboard")
	}
	before, _, _ := procGetClipboardSequenceNumber.Call()
	if err := press(chord(vkC), "Ctrl+C"); err != nil {
		return false, err
	}
	deadline := time.Now().Add(copyTimeout)
	for time.Now().Before(deadline) {
		if now, _, _ := procGetClipboardSequenceNumber.Call(); now != before {
			// The app may still be writing its other formats.
			time.Sleep(30 * time.Millisecond)
			return true, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false, nil
}

// giveKeyboard gives window the keyboard, reporting whether it took it in time.
func giveKeyboard(window uintptr) bool {
	procSetForegroundWindow.Call(window)
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if h, _, _ := procGetForegroundWindow.Call(); h == window {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// chord is the keys that press Ctrl with the key vk. Keys the user still holds, as Shift of
// Shift+Enter, would change it, so they are released first.
func chord(vk uint16) []keyboardInput {
	var in []keyboardInput
	for _, held := range []uint16{vkShift, vkMenu, vkLWin, vkRWin} {
		if state, _, _ := procGetAsyncKeyState.Call(uintptr(held)); state&0x8000 != 0 {
			in = append(in, keyboardInput{typ: inputKeyboard, vk: held, flags: keyeventfKeyUp})
		}
	}
	return append(in,
		keyboardInput{typ: inputKeyboard, vk: vkControl},
		keyboardInput{typ: inputKeyboard, vk: vk},
		keyboardInput{typ: inputKeyboard, vk: vk, flags: keyeventfKeyUp},
		keyboardInput{typ: inputKeyboard, vk: vkControl, flags: keyeventfKeyUp},
	)
}

func press(in []keyboardInput, what string) error {
	sent, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(sent) != len(in) {
		return fmt.Errorf("press %s: %w", what, err)
	}
	return nil
}

// terminalClasses are the window classes of terminals, where Ctrl+C stops what runs.
var terminalClasses = []string{"ConsoleWindowClass", "CASCADIA_HOSTING_WINDOW_CLASS", "mintty", "PuTTY", "VirtualConsoleClass"}

// isTerminal reports whether window is a terminal's.
func isTerminal(window uintptr) bool {
	var name [256]uint16
	n, _, _ := procGetClassNameW.Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	return slices.Contains(terminalClasses, syscall.UTF16ToString(name[:n]))
}
