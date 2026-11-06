//go:build linux

package linux

import (
	"os"
	"strings"
)

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

// Region returns the country of the monetary locale, as "BR" of "pt_BR.UTF-8", or "" when
// unknown.
func Region() string {
	for _, name := range []string{"LC_ALL", "LC_MONETARY", "LANG"} {
		v := os.Getenv(name)
		if v == "" {
			continue
		}
		v, _, _ = strings.Cut(v, ".")
		v, _, _ = strings.Cut(v, "@")
		if _, region, ok := strings.Cut(v, "_"); ok && len(region) == 2 {
			return strings.ToUpper(region)
		}
		return ""
	}
	return ""
}
