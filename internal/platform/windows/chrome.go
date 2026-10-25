//go:build windows

// Package windows implements Althea's platform services with Win32.
package windows

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JoaoVitorOli/althea/internal/platform"
)

// dwmwaBorderColor is DWMWA_BORDER_COLOR, available from Windows 11.
const dwmwaBorderColor = 34

var procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// Chrome styles window frames through the Desktop Window Manager.
type Chrome struct{}

// SetBorderColor implements platform.WindowChrome.
func (Chrome) SetBorderColor(handle uintptr, c platform.Color) error {
	colorref := uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16
	hr, _, _ := procDwmSetWindowAttribute.Call(
		handle, dwmwaBorderColor, uintptr(unsafe.Pointer(&colorref)), unsafe.Sizeof(colorref))
	if hr != 0 {
		return fmt.Errorf("DwmSetWindowAttribute(border color): HRESULT %#x", hr)
	}
	return nil
}
