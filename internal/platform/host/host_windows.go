// Package host returns the platform services of the system Althea runs on.
package host

import (
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/platform/windows"
)

// Services returns the Windows services.
func Services() platform.Services {
	return platform.Services{
		Chrome: windows.Chrome{}, KeyHook: windows.KeyHook{}, System: windows.System{},
		Instance: windows.SingleInstance{}, Locale: windows.Locale, Vault: windows.Vault{},
		Command: windows.Command, EditFile: windows.EditFile, Paster: windows.Paster{},
	}
}
