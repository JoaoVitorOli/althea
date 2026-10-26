// Package platform declares the operating system services Althea uses, one narrow interface
// per service. The adapters live in the subpackages windows and linux; host picks the one for
// the running system, and tests use fakes.
package platform

import "github.com/JoaoVitorOli/althea/internal/hotkey"

// Color is an opaque RGB color, as the system draws window frames.
type Color struct{ R, G, B uint8 }

// WindowChrome styles the frame the system draws around a window.
type WindowChrome interface {
	// SetBorderColor colors the window's border. Handle is the native window handle.
	SetBorderColor(handle uintptr, c Color) error
}

// KeyHook watches every key the user presses, system-wide, for the hotkeys a system cannot
// register, as the Win key alone. Only Althea's own detection reads the events; they are never
// stored (docs/adr/0009-hooks.md).
type KeyHook interface {
	// Start calls handle with each key event, on the hook's own thread, until stop is called.
	// Handle must return at once: the system drops hooks that keep it waiting.
	Start(handle func(hotkey.KeyEvent)) (stop func(), err error)
	// Mask presses and releases hotkey.VKMask, which keeps the system from acting on a Win key
	// Althea used. Call it from handle, before the Win key's release goes on.
	Mask()
}

// System acts on the computer, for the System extension.
type System interface {
	Lock() error
	Sleep() error
	Restart() error
	ShutDown() error
	SignOut() error
	EmptyRecycleBin() error
	IsDarkMode() bool
	SetDarkMode(on bool) error
}

// SingleInstance keeps one instance of Althea per user.
type SingleInstance interface {
	// Lock returns true in the first instance, which keeps the lock until release and hears of
	// later launches through onSecond, on a goroutine of its own. A later instance gets false,
	// after telling the first, and should exit.
	Lock(name string, onSecond func()) (first bool, release func(), err error)
}

// Vault keeps secrets, as API keys, in the system's credential store (docs/adr/0008-secrets.md),
// by a name such as "Althea/althea.ai/api_key".
type Vault interface {
	// Get returns the secret of name, and whether there is one.
	Get(name string) (secret string, ok bool, err error)
	Set(name, secret string) error
	// Delete removes the secret of name; removing one that is not there is not an error.
	Delete(name string) error
}

// Services are the platform services of the running system.
type Services struct {
	Instance SingleInstance
	// System is nil where Althea cannot act on the computer yet, as on Linux before v1.1.
	System System
	Chrome WindowChrome
	// KeyHook is nil where the system has no such hook, as on Linux.
	KeyHook KeyHook
	// Locale returns the user's display language, such as "pt-BR", or "" when unknown.
	Locale func() string
	// Vault is nil where Althea cannot keep secrets yet, as on Linux before v1.1: secret
	// preferences cannot be set there.
	Vault Vault
}
