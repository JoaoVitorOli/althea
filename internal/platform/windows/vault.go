//go:build windows

package windows

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW = advapi32.NewProc("CredWriteW")
	procCredReadW  = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE: this user, this computer
	credMaxBlob             = 5 * 512
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// Vault keeps secrets in Windows Credential Manager, as generic credentials of the user, which
// the Credential Manager control panel lists under Windows Credentials.
type Vault struct{}

// Get implements platform.Vault.
func (Vault) Get(name string) (string, bool, error) {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "", false, err
	}
	var c *credential
	r, _, callErr := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read secret %s: %w", name, callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(c)))
	blob := unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)
	return string(blob), true, nil
}

// Set implements platform.Vault.
func (Vault) Set(name, secret string) error {
	if len(secret) > credMaxBlob {
		return fmt.Errorf("save secret %s: longer than %d bytes", name, credMaxBlob)
	}
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	user, _ := windows.UTF16PtrFromString("Althea")
	blob := []byte(secret)
	c := credential{Type: credTypeGeneric, TargetName: target, Persist: credPersistLocalMachine, UserName: user}
	if len(blob) > 0 {
		c.CredentialBlob, c.CredentialBlobSize = &blob[0], uint32(len(blob)) //nolint:gosec // at most credMaxBlob
	}
	if r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return fmt.Errorf("save secret %s: %w", name, callErr)
	}
	return nil
}

// Delete implements platform.Vault.
func (Vault) Delete(name string) error {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	if r == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("delete secret %s: %w", name, callErr)
	}
	return nil
}
