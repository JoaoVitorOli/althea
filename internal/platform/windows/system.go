//go:build windows

package windows

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	procLockWorkStation     = user32.NewProc("LockWorkStation")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procSetSuspendState     = windows.NewLazySystemDLL("powrprof.dll").NewProc("SetSuspendState")
	procSHEmptyRecycleBinW  = windows.NewLazySystemDLL("shell32.dll").NewProc("SHEmptyRecycleBinW")
)

const (
	hwndBroadcast       = 0xFFFF
	wmSettingChange     = 0x001A
	smtoAbortIfHung     = 0x0002
	sherbNoConfirmation = 0x1 // Althea asked already
	eUnexpected         = 0x8000FFFF
	personalizeKey      = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
)

// System acts on the computer: the System extension's commands.
type System struct{}

// Lock locks the screen.
func (System) Lock() error {
	if r, _, err := procLockWorkStation.Call(); r == 0 {
		return fmt.Errorf("lock: %w", err)
	}
	return nil
}

// Sleep suspends the computer, without hibernating.
func (System) Sleep() error {
	if r, _, err := procSetSuspendState.Call(0, 0, 0); r == 0 {
		return fmt.Errorf("sleep: %w", err)
	}
	return nil
}

// Restart restarts the computer at once.
func (System) Restart() error { return shutdown("/r", "/t", "0") }

// ShutDown shuts the computer down at once.
func (System) ShutDown() error { return shutdown("/s", "/t", "0") }

// SignOut signs the user out.
func (System) SignOut() error { return shutdown("/l") }

// shutdown runs the system's shutdown.exe, by its full path so that no other program of the
// same name on the PATH runs instead.
func shutdown(args ...string) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	//nolint:gosec,noctx // a fixed system program with fixed arguments; it returns at once
	cmd := exec.Command(filepath.Join(dir, "shutdown.exe"), args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("shutdown %v: %w: %s", args, err, out)
	}
	return nil
}

// EmptyRecycleBin empties the recycle bins of every drive; an empty bin is not an error.
func (System) EmptyRecycleBin() error {
	hr, _, _ := procSHEmptyRecycleBinW.Call(0, 0, sherbNoConfirmation)
	if hr != 0 && hr != eUnexpected {
		return fmt.Errorf("empty recycle bin: HRESULT %#x", hr)
	}
	return nil
}

// IsDarkMode reports whether apps use the dark theme.
func (System) IsDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	light, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && light == 0
}

// SetDarkMode turns dark mode on or off for apps and for Windows itself, and tells running
// apps, as the Settings app does.
func (System) SetDarkMode(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("dark mode: %w", err)
	}
	defer k.Close()
	light := uint32(1)
	if on {
		light = 0
	}
	for _, name := range []string{"AppsUseLightTheme", "SystemUsesLightTheme"} {
		if err := k.SetDWordValue(name, light); err != nil {
			return fmt.Errorf("dark mode: %w", err)
		}
	}
	area, _ := windows.UTF16PtrFromString("ImmersiveColorSet")
	procSendMessageTimeoutW.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(area)),
		smtoAbortIfHung, 200, 0)
	return nil
}
