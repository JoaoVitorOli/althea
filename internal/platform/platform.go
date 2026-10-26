// Package platform declares the operating system services Althea uses, one narrow interface
// per service. The adapters live in the subpackages windows and linux; host picks the one for
// the running system, and tests use fakes.
package platform

import (
	"context"
	"os/exec"

	"github.com/JoaoVitorOli/althea/internal/hotkey"
)

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

// Paster pastes into the app the user was in, as snippets and clipboard history do.
type Paster interface {
	// Foreground returns the window that has the keyboard, to paste into later.
	Foreground() uintptr
	// Paste gives window the keyboard back and pastes the clipboard there, then moves the caret
	// back characters left.
	Paste(window uintptr, back int) error
}

// Typist types into the app in front, as snippet expansion does.
type Typist interface {
	// Type presses Backspace erase times, types text and moves the caret back characters left.
	Type(erase int, text string, back int) error
	// InPassword reports whether the keyboard is in a password field, as far as the system
	// tells.
	InPassword() bool
}

// ClipboardWatcher tells when the clipboard changes, for clipboard history.
type ClipboardWatcher interface {
	// Watch calls changed, on a thread of its own, each time the clipboard's content changes,
	// until stop is called.
	Watch(changed func(ClipboardChange)) (stop func(), err error)
}

// ClipboardChange is what is known of new clipboard content before reading it.
type ClipboardChange struct {
	// Private is set when the app that copied asked clipboard managers not to keep it, as
	// password managers do.
	Private bool
	// Source is the executable name of the app that copied, as "chrome.exe", or "".
	Source string
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
	// Command makes the command that runs a program, as exec.CommandContext, without a console
	// window where the system would open one.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
	// EditFile opens a file in an editor, never running it, as opening a script would; nil
	// where Althea cannot tell.
	EditFile func(path string) error
	// Paster is nil where Althea cannot paste into other apps yet, as on Linux.
	Paster Paster
	// Typist is nil where Althea cannot type into other apps, as on Linux; it needs KeyHook.
	Typist Typist
	// Clipboard is nil where Althea cannot watch the clipboard yet, as on Linux.
	Clipboard ClipboardWatcher
}
