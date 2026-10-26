//go:build windows

package windows

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

func TestClipboardWatcherStartsAndStops(t *testing.T) {
	for range 2 { // and again: the window class is registered once
		stop, err := ClipboardWatcher{}.Watch(func(platform.ClipboardChange) {})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (ClipboardWatcher{}).Watch(func(platform.ClipboardChange) {}); err == nil {
			t.Error("a second watcher started")
		}
		stop()
	}
}

// TestClipboardWatcherHearsChanges writes to the clipboard, so it runs only when asked:
// ALTHEA_CLIPBOARD_TESTS=1, as on CI.
func TestClipboardWatcherHearsChanges(t *testing.T) {
	if os.Getenv("ALTHEA_CLIPBOARD_TESTS") == "" {
		t.Skip("writes to the clipboard; set ALTHEA_CLIPBOARD_TESTS=1")
	}
	heard := make(chan platform.ClipboardChange, 4)
	stop, err := ClipboardWatcher{}.Watch(func(c platform.ClipboardChange) { heard <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	setClipboardText(t, "althea test")
	select {
	case c := <-heard:
		if c.Private || c.Source == "" {
			t.Errorf("change %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no change heard")
	}
}

func setClipboardText(t *testing.T, s string) {
	t.Helper()
	const cfUnicodeText, gmemMoveable = 13, 0x0002
	if r, _, err := procOpenClipboard.Call(0); r == 0 {
		t.Fatal(err)
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard := user32.NewProc("EmptyClipboard")
	procSetClipboardData := user32.NewProc("SetClipboardData")
	procGlobalAlloc := kernel32.NewProc("GlobalAlloc")
	procEmptyClipboard.Call()
	u, _ := windows.UTF16FromString(s)
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(u)*2))
	p, _, _ := procGlobalLock.Call(h)
	procRtlMoveMemory.Call(p, uintptr(unsafeAddr(u)), uintptr(len(u)*2))
	procGlobalUnlock.Call(h)
	procSetClipboardData.Call(cfUnicodeText, h)
}

func unsafeAddr(u []uint16) unsafe.Pointer { return unsafe.Pointer(&u[0]) }
