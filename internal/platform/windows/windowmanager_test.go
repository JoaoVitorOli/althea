//go:build windows

package windows

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

// testWindow opens a real window of this thread, shown without taking the keyboard.
func testWindow(t *testing.T) uintptr {
	t.Helper()
	const wsOverlappedWindow, swShowNoActivate = 0x00CF0000, 4
	class, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("Althea test")
	h, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, 100, 100, 500, 400, 0, 0, 0, 0)
	if h == 0 {
		t.Skipf("no window here: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(h) })
	procShowWindow.Call(h, swShowNoActivate)
	return h
}

func TestMonitorInfoLayout(t *testing.T) {
	if got := unsafe.Sizeof(monitorInfoEx{}); got != 104 {
		t.Errorf("MONITORINFOEXW is %d bytes, want 104", got)
	}
}

func TestDisplaysHaveWorkAreas(t *testing.T) {
	displays, err := WindowManager{}.Displays()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range displays {
		if d.ID == "" || d.Work.W <= 0 || d.Work.H <= 0 {
			t.Errorf("display %+v", d)
		}
	}
}

func TestPlaceMovesTheFrameTheUserSees(t *testing.T) {
	runtime.LockOSThread() // a window answers on the thread that made it
	defer runtime.UnlockOSThread()
	h := testWindow(t)
	m := WindowManager{}
	want := platform.Rect{X: 120, Y: 140, W: 640, H: 480}
	if err := m.Place(h, want); err != nil {
		t.Fatal(err)
	}
	got, maximized, err := m.Frame(h)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || maximized {
		t.Errorf("frame = %+v, maximized %v, want %+v", got, maximized, want)
	}
}

func TestPlaceLeavesMaximized(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	h := testWindow(t)
	m := WindowManager{}
	if err := m.Maximize(h); err != nil {
		t.Fatal(err)
	}
	if _, maximized, _ := m.Frame(h); !maximized {
		t.Error("not maximized")
	}
	want := platform.Rect{X: 50, Y: 60, W: 600, H: 400}
	if err := m.Place(h, want); err != nil {
		t.Fatal(err)
	}
	if got, maximized, _ := m.Frame(h); got != want || maximized {
		t.Errorf("frame = %+v, maximized %v, want %+v", got, maximized, want)
	}
}

func TestFrameRefusesTheDesktopAndClosedWindows(t *testing.T) {
	m := WindowManager{}
	if _, _, err := m.Frame(0); err == nil {
		t.Error("no window was accepted")
	}
	progman, _ := windows.UTF16PtrFromString("Progman")
	desktop, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(progman)), 0)
	if desktop != 0 {
		if _, _, err := m.Frame(desktop); err == nil {
			t.Error("the desktop was accepted")
		}
	}
}

func TestOpenWindowsLeaveOutAlthea(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	h := testWindow(t)
	open, err := WindowManager{}.OpenWindows()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range open {
		if w.Handle == h {
			t.Error("this process's own window was listed")
		}
		if w.App == "" || w.Title == "" {
			t.Errorf("window %+v", w)
		}
	}
}
