//go:build windows

package windows

import "golang.org/x/sys/windows"

// Locale returns the user's preferred display language, such as "pt-BR".
func Locale() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}
