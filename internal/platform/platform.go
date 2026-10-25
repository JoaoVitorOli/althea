// Package platform declares the operating system services Althea uses, one narrow interface
// per service. The adapters live in the subpackages windows and linux; host picks the one for
// the running system, and tests use fakes.
package platform

// Color is an opaque RGB color, as the system draws window frames.
type Color struct{ R, G, B uint8 }

// WindowChrome styles the frame the system draws around a window.
type WindowChrome interface {
	// SetBorderColor colors the window's border. Handle is the native window handle.
	SetBorderColor(handle uintptr, c Color) error
}

// Services are the platform services of the running system.
type Services struct {
	Chrome WindowChrome
}
