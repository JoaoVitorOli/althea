//go:build windows

package windows

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

var (
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procRegisterClipboardFormatW      = user32.NewProc("RegisterClipboardFormatW")
	procIsClipboardFormatAvailable    = user32.NewProc("IsClipboardFormatAvailable")
	procOpenClipboard                 = user32.NewProc("OpenClipboard")
	procCloseClipboard                = user32.NewProc("CloseClipboard")
	procGetClipboardData              = user32.NewProc("GetClipboardData")
	procGetClipboardOwner             = user32.NewProc("GetClipboardOwner")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")

	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalLock    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock  = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory = kernel32.NewProc("RtlMoveMemory")
)

const (
	wmClipboardUpdate = 0x031D
	hwndMessage       = ^uintptr(2) // HWND_MESSAGE, -3
)

// wndclassex is WNDCLASSEXW.
type wndclassex struct {
	size                   uint32
	style                  uint32
	wndProc                uintptr
	clsExtra, wndExtra     int32
	instance, icon, cursor windows.Handle
	background             windows.Handle
	menuName, className    *uint16
	iconSm                 windows.Handle
}

// The window procedure is made once for the process, as callbacks are never freed; it hands
// clipboard updates to the running watcher.
var (
	clipClassOnce sync.Once
	clipClassErr  error
	clipClass     *uint16
	clipHandler   func()
	clipMu        sync.Mutex
)

func clipWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmClipboardUpdate {
		clipMu.Lock()
		h := clipHandler
		clipMu.Unlock()
		if h != nil {
			h()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// The clipboard formats by which apps ask clipboard managers to leave their content alone.
var (
	formatsOnce                             sync.Once
	cfExclude, cfViewerIgnore, cfCanInclude uintptr
)

func registerFormats() {
	formatsOnce.Do(func() {
		reg := func(name string) uintptr {
			p, _ := windows.UTF16PtrFromString(name)
			f, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(p)))
			return f
		}
		cfExclude = reg("ExcludeClipboardContentFromMonitorProcessing")
		cfViewerIgnore = reg("Clipboard Viewer Ignore")
		cfCanInclude = reg("CanIncludeInClipboardHistory")
	})
}

// ClipboardWatcher tells of clipboard changes through a message-only window that listens for
// them (AddClipboardFormatListener). One runs at a time.
type ClipboardWatcher struct{}

// Watch implements platform.ClipboardWatcher.
func (ClipboardWatcher) Watch(changed func(platform.ClipboardChange)) (stop func(), err error) {
	clipMu.Lock()
	if clipHandler != nil {
		clipMu.Unlock()
		return nil, errors.New("clipboard watcher: already running")
	}
	clipMu.Unlock()
	clipClassOnce.Do(func() {
		clipClass, _ = windows.UTF16PtrFromString("AltheaClipboardWatcher")
		wc := wndclassex{wndProc: windows.NewCallback(clipWndProc), className: clipClass}
		wc.size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			clipClassErr = fmt.Errorf("clipboard watcher: RegisterClassExW: %w", err)
		}
	})
	if clipClassErr != nil {
		return nil, clipClassErr
	}
	registerFormats()

	started := make(chan error, 1)
	done := make(chan struct{})
	var thread uint32
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		thread = windows.GetCurrentThreadId()

		hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(clipClass)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, 0, 0)
		if hwnd == 0 {
			started <- fmt.Errorf("clipboard watcher: CreateWindowExW: %w", err)
			return
		}
		defer procDestroyWindow.Call(hwnd)
		clipMu.Lock()
		clipHandler = func() { changed(readChange(hwnd)) }
		clipMu.Unlock()
		defer func() {
			clipMu.Lock()
			clipHandler = nil
			clipMu.Unlock()
		}()
		if r, _, err := procAddClipboardFormatListener.Call(hwnd); r == 0 {
			started <- fmt.Errorf("clipboard watcher: AddClipboardFormatListener: %w", err)
			return
		}
		defer procRemoveClipboardFormatListener.Call(hwnd)
		started <- nil
		var msg [48]byte // MSG
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 { //nolint:gosec // GetMessage returns a BOOL, -1 on error
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	if err := <-started; err != nil {
		<-done
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0)
			<-done
		})
	}, nil
}

// readChange tells what the watcher knows of the clipboard's new content without reading it:
// whether its app asked to keep it private, and which app copied it.
func readChange(hwnd uintptr) platform.ClipboardChange {
	available := func(f uintptr) bool { r, _, _ := procIsClipboardFormatAvailable.Call(f); return r != 0 }
	c := platform.ClipboardChange{Private: available(cfExclude) || available(cfViewerIgnore)}
	if !c.Private && available(cfCanInclude) {
		c.Private = !canInclude(hwnd)
	}
	owner, _, _ := procGetClipboardOwner.Call()
	if owner == 0 {
		owner, _, _ = procGetForegroundWindow.Call()
	}
	c.Source = processName(owner)
	return c
}

// canInclude reads the CanIncludeInClipboardHistory flag: 0 asks to keep the content out of
// clipboard history. The clipboard may be busy a moment while its owner writes.
func canInclude(hwnd uintptr) bool {
	for range 5 {
		if r, _, _ := procOpenClipboard.Call(hwnd); r != 0 {
			ok := readCanInclude()
			procCloseClipboard.Call()
			return ok
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false // unreadable: keep it out, to be safe
}

// readCanInclude reads the flag from the open clipboard.
func readCanInclude() bool {
	h, _, _ := procGetClipboardData.Call(cfCanInclude)
	if h == 0 {
		return true
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return true
	}
	defer procGlobalUnlock.Call(h)
	var flag uint32 // copied out, as Go may not point into memory it does not own
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&flag)), p, unsafe.Sizeof(flag))
	return flag != 0
}

// processName returns the executable name of the process of a window, as "chrome.exe".
func processName(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var buf [windows.MAX_PATH]uint16
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
