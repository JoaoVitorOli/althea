//go:build windows

package windows

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/hotkey"
)

const (
	whKeyboardLL  = 13
	hcAction      = 0
	wmQuit        = 0x0012
	wmKeyDown     = 0x0100
	wmSysKeyDown  = 0x0104
	llkhfInjected = 0x10

	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procSendInput           = user32.NewProc("SendInput")
)

// kbdllHookStruct is KBDLLHOOKSTRUCT.
type kbdllHookStruct struct {
	vkCode, scanCode, flags, time uint32
	extraInfo                     uintptr
}

// keyboardInput is an INPUT holding a KEYBDINPUT, padded to the size of the union's largest
// member, MOUSEINPUT.
type keyboardInput struct {
	typ         uint32
	_           uint32 // the union is 8-byte aligned
	vk, scan    uint16
	flags, time uint32
	extraInfo   uintptr
	_           [8]byte
}

// The hook's callback is made once for the process: callbacks are never freed. It hands each
// event to the handler of the running hook, if any.
var (
	hookProcOnce sync.Once
	hookProc     uintptr
	handler      atomic.Pointer[func(hotkey.KeyEvent)]
)

func lowLevelKeyboardProc(code int, wParam uintptr, kb *kbdllHookStruct) uintptr {
	if code == hcAction {
		if h := handler.Load(); h != nil {
			(*h)(hotkey.KeyEvent{
				VK:       kb.vkCode,
				Down:     wParam == wmKeyDown || wParam == wmSysKeyDown,
				Injected: kb.flags&llkhfInjected != 0,
			})
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, uintptr(unsafe.Pointer(kb)))
	return r
}

// KeyHook is a low-level keyboard hook (WH_KEYBOARD_LL). One runs at a time.
type KeyHook struct{}

// Start implements platform.KeyHook. The hook runs on a thread of its own, which pumps the
// messages the system delivers the events with.
func (KeyHook) Start(handle func(hotkey.KeyEvent)) (stop func(), err error) {
	if !handler.CompareAndSwap(nil, &handle) {
		return nil, errors.New("keyboard hook: already running")
	}
	hookProcOnce.Do(func() { hookProc = windows.NewCallback(lowLevelKeyboardProc) })

	started := make(chan error, 1)
	done := make(chan struct{})
	var thread uint32
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		thread = windows.GetCurrentThreadId()
		var module windows.Handle
		if err := windows.GetModuleHandleEx(0, nil, &module); err != nil {
			started <- fmt.Errorf("keyboard hook: %w", err)
			return
		}
		hook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, hookProc, uintptr(module), 0)
		if hook == 0 {
			started <- fmt.Errorf("keyboard hook: SetWindowsHookExW: %w", callErr)
			return
		}
		started <- nil
		var msg [48]byte // MSG
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 { //nolint:gosec // GetMessage returns a BOOL, -1 on error
				break
			}
		}
		procUnhookWindowsHookEx.Call(hook)
	}()

	if err := <-started; err != nil {
		<-done
		handler.Store(nil)
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0)
			<-done
			handler.Store(nil)
		})
	}, nil
}

// Mask implements platform.KeyHook.
func (KeyHook) Mask() {
	in := [2]keyboardInput{
		{typ: inputKeyboard, vk: hotkey.VKMask},
		{typ: inputKeyboard, vk: hotkey.VKMask, flags: keyeventfKeyUp},
	}
	procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}
