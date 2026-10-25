//go:build linux

package linux

import "os"

// Locale returns the language of messages from the POSIX locale variables, such as
// "pt_BR.UTF-8".
func Locale() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return ""
}
