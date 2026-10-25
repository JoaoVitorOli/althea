// Package host returns the platform services of the system Althea runs on.
package host

import (
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/platform/linux"
)

// Services returns the Linux services.
func Services() platform.Services {
	return platform.Services{Chrome: linux.Chrome{}}
}
