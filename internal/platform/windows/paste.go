//go:build windows

package windows

import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

var (
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsWindow            = user32.NewProc("IsWindow")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
)

const (
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkLWin    = 0x5B
	vkRWin    = 0x5C
	vkV       = 0x56
	vkLeft    = 0x25

	keyeventfExtendedKey = 0x0001
	// maxCaretBack bounds the presses of Left after a paste.
	maxCaretBack = 2000
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
	procSetForegroundWindow.Call(window)
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if h, _, _ := procGetForegroundWindow.Call(); h == window {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("the window to paste into did not take the keyboard")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Keys the user still holds, as Shift of Shift+Enter, would change Ctrl+V: release them first.
	var in []keyboardInput
	for _, vk := range []uint16{vkShift, vkMenu, vkLWin, vkRWin} {
		if state, _, _ := procGetAsyncKeyState.Call(uintptr(vk)); state&0x8000 != 0 {
			in = append(in, keyboardInput{typ: inputKeyboard, vk: vk, flags: keyeventfKeyUp})
		}
	}
	in = append(in,
		keyboardInput{typ: inputKeyboard, vk: vkControl},
		keyboardInput{typ: inputKeyboard, vk: vkV},
		keyboardInput{typ: inputKeyboard, vk: vkV, flags: keyeventfKeyUp},
		keyboardInput{typ: inputKeyboard, vk: vkControl, flags: keyeventfKeyUp},
	)
	for range min(back, maxCaretBack) {
		in = append(in,
			keyboardInput{typ: inputKeyboard, vk: vkLeft, flags: keyeventfExtendedKey},
			keyboardInput{typ: inputKeyboard, vk: vkLeft, flags: keyeventfExtendedKey | keyeventfKeyUp},
		)
	}
	sent, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(sent) != len(in) {
		return fmt.Errorf("press Ctrl+V: %w", err)
	}
	return nil
}
