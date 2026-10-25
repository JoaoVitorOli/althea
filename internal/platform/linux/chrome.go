//go:build linux

// Package linux implements Althea's platform services on Linux desktops. Most services arrive
// in v1.1; until then they do nothing, so the app builds and its tests run on Linux.
package linux

import "github.com/JoaoVitorOli/althea/internal/platform"

// Chrome leaves window frames to the desktop, which draws them on Linux.
type Chrome struct{}

// SetBorderColor implements platform.WindowChrome.
func (Chrome) SetBorderColor(uintptr, platform.Color) error { return nil }
