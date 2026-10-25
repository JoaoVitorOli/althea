// Package host returns the platform services of the system Althea runs on.
package host

import (
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/platform/windows"
)

// Services returns the Windows services.
func Services() platform.Services {
	return platform.Services{Chrome: windows.Chrome{}, Locale: windows.Locale}
}
