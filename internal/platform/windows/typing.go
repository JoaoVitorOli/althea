//go:build windows

package windows

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetKeyState              = user32.NewProc("GetKeyState")
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procToUnicodeEx              = user32.NewProc("ToUnicodeEx")
	procGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
)

const (
	vkBack    = 0x08
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkCapital = 0x14

	keyeventfUnicode = 0x0004
	// toUnicodeKeepState keeps ToUnicodeEx from changing the keyboard's state, so that reading
	// a dead key does not eat it from the app the user types in (Windows 10 1607 and later).
	toUnicodeKeepState = 0x4

	gwlStyle   = ^uintptr(15) // GWL_STYLE, -16
	esPassword = 0x0020
)

// typedText returns what a key going down types in the layout of the app in front, "" for
// none. It runs in the keyboard hook, so it reads the modifiers' state as it is now.
func typedText(vk, scan uint32) string {
	var state [256]byte
	down := func(k int) bool { s, _, _ := procGetAsyncKeyState.Call(uintptr(k)); return s&0x8000 != 0 }
	ctrl, alt := down(vkControl), down(vkMenu)
	if ctrl != alt {
		return "" // a shortcut, as Ctrl+C; Ctrl and Alt together are AltGr, which types
	}
	for _, k := range []int{vkShift, vkControl, vkMenu} {
		if down(k) {
			state[k] = 0x80
		}
	}
	if caps, _, _ := procGetKeyState.Call(vkCapital); caps&1 != 0 {
		state[vkCapital] = 0x01
	}
	fg, _, _ := procGetForegroundWindow.Call()
	thread, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
	layout, _, _ := procGetKeyboardLayout.Call(thread)
	var buf [8]uint16
	n, _, _ := procToUnicodeEx.Call(uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(&state[0])),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), toUnicodeKeepState, layout)
	if int32(n) <= 0 { //nolint:gosec // ToUnicodeEx returns an int: -1 for a dead key, 0 for none
		return ""
	}
	text := string(utf16.Decode(buf[:n]))
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			return "" // Backspace, Enter, Escape and the like type control characters
		}
	}
	return text
}

// Typist types text into the app in front, as snippet expansion does.
type Typist struct{}

// Type implements platform.Typist: it presses Backspace erase times, types text, and presses
// Left back times. Text goes in as Unicode characters, not through the clipboard.
func (Typist) Type(erase int, text string, back int) error {
	var in []keyboardInput
	press := func(vk uint16, flags uint32) {
		in = append(in, keyboardInput{typ: inputKeyboard, vk: vk, flags: flags},
			keyboardInput{typ: inputKeyboard, vk: vk, flags: flags | keyeventfKeyUp})
	}
	// Keys the user still holds, as Shift of a keyword's last letter, would change what types.
	for _, vk := range []uint16{vkShift, vkControl, vkMenu} {
		if state, _, _ := procGetAsyncKeyState.Call(uintptr(vk)); state&0x8000 != 0 {
			in = append(in, keyboardInput{typ: inputKeyboard, vk: vk, flags: keyeventfKeyUp})
		}
	}
	for range erase {
		press(vkBack, 0)
	}
	for _, r := range strings.ReplaceAll(text, "\r\n", "\n") {
		switch r {
		case '\n':
			press(vkReturn, 0)
		case '\t':
			press(vkTab, 0)
		default:
			for _, u := range utf16.Encode([]rune{r}) {
				in = append(in, keyboardInput{typ: inputKeyboard, scan: u, flags: keyeventfUnicode},
					keyboardInput{typ: inputKeyboard, scan: u, flags: keyeventfUnicode | keyeventfKeyUp})
			}
		}
	}
	for range min(back, maxCaretBack) {
		press(vkLeft, keyeventfExtendedKey)
	}
	if len(in) == 0 {
		return nil
	}
	sent, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(sent) != len(in) {
		return fmt.Errorf("type: %w", err)
	}
	return nil
}

// guiThreadInfo is GUITHREADINFO.
type guiThreadInfo struct {
	size                                               uint32
	flags                                              uint32
	active, focus, capture, menuOwner, moveSize, caret windows.HWND
	caretLeft, caretTop, caretRight, caretBottom       int32
}

// InPassword implements platform.Typist: whether the keyboard is in a password field. Windows
// tells only of its own edit controls; browsers and other toolkits keep it to themselves.
func (Typist) InPassword() bool {
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == 0 {
		return false
	}
	thread, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
	info := guiThreadInfo{size: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	if ok, _, _ := procGetGUIThreadInfo.Call(thread, uintptr(unsafe.Pointer(&info))); ok == 0 || info.focus == 0 {
		return false
	}
	var class [64]uint16
	n, _, _ := procGetClassNameW.Call(uintptr(info.focus), uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	name := strings.ToLower(windows.UTF16ToString(class[:n]))
	if name != "edit" && !strings.HasPrefix(name, "richedit") {
		return false
	}
	style, _, _ := procGetWindowLongPtrW.Call(uintptr(info.focus), gwlStyle)
	return style&esPassword != 0
}
