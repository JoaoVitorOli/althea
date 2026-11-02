//go:build windows

package windows

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

var (
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procIsZoomed            = user32.NewProc("IsZoomed")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procEnumChildWindows    = user32.NewProc("EnumChildWindows")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindow           = user32.NewProc("GetWindow")

	procDwmGetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")

	procGetApplicationUserModelID = kernel32.NewProc("GetApplicationUserModelId")
)

const (
	swMaximize = 3
	swRestore  = 9

	swpNoActivate = 0x0010

	gwOwner         = 4
	gwlExStyle      = ^uintptr(19) // GWL_EXSTYLE, -20
	wsExToolWindow  = 0x00000080
	monitorPrimary  = 1
	appsFolderShell = `shell:AppsFolder\`

	dwmwaExtendedFrameBounds = 9
	dwmwaCloaked             = 14
)

// WindowManager moves other apps' windows.
type WindowManager struct{}

// monitorInfoEx is MONITORINFOEXW.
type monitorInfoEx struct {
	size    uint32
	monitor windows.Rect
	work    windows.Rect
	flags   uint32
	device  [32]uint16
}

// The system's enumerations call back through a function it keeps forever, and Go can make only
// so many of them, so there is one of each for all calls, filling what enumerating guards.
var (
	enumerating sync.Mutex
	enumHandles []uintptr
	enumWindow  = windows.NewCallback(func(h, _ uintptr) uintptr {
		enumHandles = append(enumHandles, h)
		return 1
	})
	enumMonitor = windows.NewCallback(func(h, _, _, _ uintptr) uintptr {
		enumHandles = append(enumHandles, h)
		return 1
	})
)

// Foreground implements platform.WindowManager.
func (WindowManager) Foreground() (window uintptr, own bool) {
	h, _, _ := procGetForegroundWindow.Call()
	return h, h != 0 && processID(h) == windows.GetCurrentProcessId()
}

// Displays implements platform.WindowManager: the primary display first.
func (WindowManager) Displays() ([]platform.Display, error) {
	enumerating.Lock()
	enumHandles = nil
	r, _, err := procEnumDisplayMonitors.Call(0, 0, enumMonitor, 0)
	monitors := enumHandles
	enumerating.Unlock()
	if r == 0 {
		return nil, fmt.Errorf("list displays: %w", err)
	}
	type found struct {
		platform.Display
		primary bool
	}
	var all []found
	for _, m := range monitors {
		info := monitorInfoEx{size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		if ok, _, _ := procGetMonitorInfoW.Call(m, uintptr(unsafe.Pointer(&info))); ok == 0 {
			continue
		}
		all = append(all, found{
			Display: platform.Display{ID: windows.UTF16ToString(info.device[:]), Work: rectOf(info.work)},
			primary: info.flags&monitorPrimary != 0,
		})
	}
	slices.SortStableFunc(all, func(a, b found) int {
		switch {
		case a.primary == b.primary:
			return cmp.Compare(a.ID, b.ID)
		case a.primary:
			return -1
		}
		return 1
	})
	out := make([]platform.Display, len(all))
	for i, f := range all {
		out[i] = f.Display
	}
	return out, nil
}

// Frame implements platform.WindowManager.
func (WindowManager) Frame(window uintptr) (frame platform.Rect, maximized bool, err error) {
	if err := checkMovable(window); err != nil {
		return platform.Rect{}, false, err
	}
	r, err := visibleFrame(window)
	if err != nil {
		return platform.Rect{}, false, err
	}
	z, _, _ := procIsZoomed.Call(window)
	return rectOf(r), z != 0, nil
}

// Place implements platform.WindowManager. A window's frame holds an invisible border to grab it
// by, so the rectangle asked of the system is larger than the one seen. Crossing to a display of
// another scale changes that border as the window arrives, so a second pass settles it.
func (WindowManager) Place(window uintptr, r platform.Rect) error {
	if err := checkMovable(window); err != nil {
		return err
	}
	z, _, _ := procIsZoomed.Call(window)
	i, _, _ := procIsIconic.Call(window)
	if z != 0 || i != 0 {
		procShowWindow.Call(window, swRestore)
	}
	for range 2 {
		var outer windows.Rect
		if ok, _, err := procGetWindowRect.Call(window, uintptr(unsafe.Pointer(&outer))); ok == 0 {
			return fmt.Errorf("read window: %w", err)
		}
		seen, err := visibleFrame(window)
		if err != nil {
			return err
		}
		left, top := int(seen.Left-outer.Left), int(seen.Top-outer.Top)
		right, bottom := int(outer.Right-seen.Right), int(outer.Bottom-seen.Bottom)
		x, y := r.X-left, r.Y-top
		w, h := r.W+left+right, r.H+top+bottom
		// Without SWP_NOZORDER, this raises the window, which is what a layout's order relies on.
		ok, _, err := procSetWindowPos.Call(window, 0,
			uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate)
		if ok == 0 {
			return fmt.Errorf("move window: %w", err)
		}
		if seen, err = visibleFrame(window); err != nil || rectOf(seen) == r {
			return err
		}
	}
	return nil // a window may refuse the size, as one with a minimum
}

// Maximize implements platform.WindowManager.
func (WindowManager) Maximize(window uintptr) error {
	if err := checkMovable(window); err != nil {
		return err
	}
	procShowWindow.Call(window, swMaximize)
	return nil
}

// Focus implements platform.WindowManager.
func (WindowManager) Focus(window uintptr) error {
	if ok, _, err := procSetForegroundWindow.Call(window); ok == 0 {
		return fmt.Errorf("focus window: %w", err)
	}
	return nil
}

// OpenWindows implements platform.WindowManager: the windows the taskbar would show and that are
// on the screen, topmost first, as EnumWindows gives them.
func (WindowManager) OpenWindows() ([]platform.OpenWindow, error) {
	all, err := topLevelWindows()
	if err != nil {
		return nil, err
	}
	var out []platform.OpenWindow
	for _, h := range all {
		if !isAppWindow(h) {
			continue
		}
		app := appOf(h)
		if app == "" {
			continue // elevated, or gone: Althea could neither move it nor open it again
		}
		r, err := visibleFrame(h)
		if err != nil {
			continue
		}
		z, _, _ := procIsZoomed.Call(h)
		out = append(out, platform.OpenWindow{
			Handle: h, App: app, Title: windowText(h), Frame: rectOf(r), Maximized: z != 0,
		})
	}
	return out, nil
}

// topLevelWindows lists every top-level window, topmost first.
func topLevelWindows() ([]uintptr, error) {
	enumerating.Lock()
	defer enumerating.Unlock()
	enumHandles = nil
	if r, _, err := procEnumWindows.Call(enumWindow, 0); r == 0 {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	return enumHandles, nil
}

// childWindows lists the windows inside h.
func childWindows(h uintptr) []uintptr {
	enumerating.Lock()
	defer enumerating.Unlock()
	enumHandles = nil
	procEnumChildWindows.Call(h, enumWindow, 0) // its result means nothing
	return enumHandles
}

// checkMovable refuses what is no app's window: the desktop and the taskbars are windows too.
func checkMovable(h uintptr) error {
	if h == 0 {
		return errors.New("no window")
	}
	if ok, _, _ := procIsWindow.Call(h); ok == 0 {
		return errors.New("the window has closed")
	}
	if ok, _, _ := procIsWindowVisible.Call(h); ok == 0 {
		return errors.New("the window is not visible")
	}
	if isShell(h) {
		return errors.New("the desktop and the taskbar do not move")
	}
	return nil
}

func isShell(h uintptr) bool {
	switch className(h) {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return true
	}
	return false
}

// isAppWindow reports whether h is a window of an app the user sees now. Cloaked windows are on
// another virtual desktop, or suspended Store apps.
func isAppWindow(h uintptr) bool {
	if ok, _, _ := procIsWindowVisible.Call(h); ok == 0 {
		return false
	}
	if i, _, _ := procIsIconic.Call(h); i != 0 {
		return false
	}
	if owner, _, _ := procGetWindow.Call(h, gwOwner); owner != 0 {
		return false
	}
	if style, _, _ := procGetWindowLongPtrW.Call(h, gwlExStyle); style&wsExToolWindow != 0 {
		return false
	}
	var cloaked int32
	hr, _, _ := procDwmGetWindowAttribute.Call(h, dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked))
	if hr == 0 && cloaked != 0 {
		return false
	}
	return !isShell(h) && windowText(h) != "" && processID(h) != windows.GetCurrentProcessId()
}

// appOf names the app of a window as it opens again: a Store app by its ID, any other by its
// program. A Store app's window belongs to ApplicationFrameHost, with the app in a child window.
func appOf(h uintptr) string {
	pid := processID(h)
	app := processApp(pid)
	if !strings.EqualFold(filepath.Base(app), "ApplicationFrameHost.exe") {
		return app
	}
	for _, c := range childWindows(h) {
		if p := processID(c); p != pid {
			return processApp(p)
		}
	}
	return ""
}

func processID(h uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessID.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func processApp(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var id [256]uint16
	n := uint32(len(id))
	if r, _, _ := procGetApplicationUserModelID.Call(uintptr(h), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&id[0]))); r == 0 {
		return appsFolderShell + windows.UTF16ToString(id[:])
	}
	var path [windows.MAX_LONG_PATH]uint16
	n = uint32(len(path))
	if err := windows.QueryFullProcessImageName(h, 0, &path[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(path[:n])
}

// visibleFrame is the rectangle the user sees, without the invisible border.
func visibleFrame(h uintptr) (windows.Rect, error) {
	var r windows.Rect
	hr, _, _ := procDwmGetWindowAttribute.Call(h, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r))
	if hr == 0 {
		return r, nil
	}
	if ok, _, err := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return r, fmt.Errorf("read window: %w", err)
	}
	return r, nil
}

func className(h uintptr) string {
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func windowText(h uintptr) string {
	var buf [512]uint16
	n, _, _ := procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func rectOf(r windows.Rect) platform.Rect {
	return platform.Rect{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
}
