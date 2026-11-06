//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
	procGetProcessHandleCnt  = kernel32.NewProc("GetProcessHandleCount")
	procGetGuiResources      = user32.NewProc("GetGuiResources")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procKeybdEvent           = user32.NewProc("keybd_event")
)

// processMemoryCountersEx2 is PROCESS_MEMORY_COUNTERS_EX2, of Windows 10 1809 and later.
type processMemoryCountersEx2 struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
	PrivateWorkingSetSize      uintptr
	SharedCommitUsage          uint64
}

// resources are what a process holds at a moment.
type resources struct {
	at                      time.Duration
	privateWS, privateBytes uint64
	handles, gdi, user      uint32
	threads                 int
}

func sample(pid uint32) (resources, error) {
	var r resources
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return r, err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var c processMemoryCountersEx2
	c.cb = uint32(unsafe.Sizeof(c))
	if ok, _, err := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb)); ok == 0 {
		return r, err
	}
	r.privateWS, r.privateBytes = uint64(c.PrivateWorkingSetSize), uint64(c.PrivateUsage)
	var n uint32
	if ok, _, _ := procGetProcessHandleCnt.Call(uintptr(h), uintptr(unsafe.Pointer(&n))); ok != 0 {
		r.handles = n
	}
	gdi, _, _ := procGetGuiResources.Call(uintptr(h), 0)  // GR_GDIOBJECTS
	user, _, _ := procGetGuiResources.Call(uintptr(h), 1) // GR_USEROBJECTS
	r.gdi, r.user = uint32(gdi), uint32(user)             //nolint:gosec // counts of a few thousand at most
	r.threads = threads(pid)
	return r, nil
}

func threads(pid uint32) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ThreadEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err := windows.Thread32First(snap, &e); err == nil; err = windows.Thread32Next(snap, &e) {
		if e.OwnerProcessID == pid {
			n++
		}
	}
	return n
}

// cpuTime is the processor time pid used so far, in user and kernel mode.
func cpuTime(pid uint32) (time.Duration, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	ticks := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return time.Duration((ticks(kernel) + ticks(user)) * 100), nil
}

// launcherVisible reports whether pid has a visible window titled Althea.
func launcherVisible(pid uint32) bool {
	visible := false
	cb := syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		_, _ = windows.GetWindowThreadProcessId(hwnd, &owner)
		if owner != pid {
			return 1
		}
		buf := make([]uint16, 64)
		n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if windows.UTF16ToString(buf[:n]) != "Althea" {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(uintptr(hwnd)); v != 0 {
			visible = true
			return 0
		}
		return 1
	})
	_, _, _ = procEnumWindows.Call(cb, 0)
	return visible
}

const (
	vkMenu       = 0x12
	vkSpace      = 0x20
	vkEscape     = 0x1B
	keyeventfKey = 0x0002 // KEYEVENTF_KEYUP
)

func key(vk byte, up bool) {
	var flags uintptr
	if up {
		flags = keyeventfKey
	}
	_, _, _ = procKeybdEvent.Call(uintptr(vk), 0, flags, 0)
}

func pressHotkey() {
	key(vkMenu, false)
	key(vkSpace, false)
	key(vkSpace, true)
	key(vkMenu, true)
}

func pressEscape() {
	key(vkEscape, false)
	key(vkEscape, true)
}
