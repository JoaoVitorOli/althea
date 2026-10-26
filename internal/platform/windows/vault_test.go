//go:build windows

package windows

import (
	"fmt"
	"os"
	"testing"
	"unsafe"
)

func TestCredentialLayout(t *testing.T) {
	// CREDENTIALW on 64-bit Windows: 80 bytes, the blob pointer at 40.
	var c credential
	if unsafe.Sizeof(c) != 80 || unsafe.Offsetof(c.CredentialBlob) != 40 || unsafe.Offsetof(c.UserName) != 72 {
		t.Errorf("size %d, blob at %d, user at %d", unsafe.Sizeof(c), unsafe.Offsetof(c.CredentialBlob), unsafe.Offsetof(c.UserName))
	}
}

func TestVaultKeepsAndForgetsASecret(t *testing.T) {
	name := fmt.Sprintf("Althea-test/%d/api_key", os.Getpid())
	v := Vault{}
	t.Cleanup(func() { _ = v.Delete(name) })

	if _, ok, err := v.Get(name); ok || err != nil {
		t.Fatalf("Get before Set = %v, %v", ok, err)
	}
	for _, secret := range []string{"sk-çé-123", "replaced"} {
		if err := v.Set(name, secret); err != nil {
			t.Fatal(err)
		}
		got, ok, err := v.Get(name)
		if !ok || err != nil || got != secret {
			t.Fatalf("Get = %q, %v, %v; want %q", got, ok, err, secret)
		}
	}
	if err := v.Delete(name); err != nil {
		t.Fatal(err)
	}
	if err := v.Delete(name); err != nil {
		t.Errorf("deleting a missing secret: %v", err)
	}
	if _, ok, _ := v.Get(name); ok {
		t.Error("the secret outlived Delete")
	}
}

func TestGUIThreadInfoLayout(t *testing.T) {
	// GUITHREADINFO on 64-bit Windows: 72 bytes.
	if s := unsafe.Sizeof(guiThreadInfo{}); s != 72 {
		t.Errorf("size %d", s)
	}
}

func TestTypedTextOfALetter(t *testing.T) {
	// A is a letter in every Latin layout; Caps Lock may make it a capital.
	if got := typedText(0x41, 0x1E); got != "a" && got != "A" {
		t.Errorf("A types %q", got)
	}
	if got := typedText(0x25, 0x4B); got != "" { // Left
		t.Errorf("Left types %q", got)
	}
}
