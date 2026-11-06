//go:build windows

package windows

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Locale returns the user's preferred display language, such as "pt-BR".
func Locale() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}

var procGetUserDefaultGeoName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultGeoName")

// Region returns the country the user set in Windows' region settings, as "BR", or "" when
// unknown. Without GetUserDefaultGeoName (before Windows 10 1709) it reads the locale's.
func Region() string {
	if procGetUserDefaultGeoName.Find() == nil {
		buf := make([]uint16, 16)
		n, _, _ := procGetUserDefaultGeoName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 1 {
			if r := windows.UTF16ToString(buf); len(r) == 2 {
				return r
			}
		}
	}
	_, region, _ := strings.Cut(Locale(), "-")
	if len(region) == 2 {
		return strings.ToUpper(region)
	}
	return ""
}
