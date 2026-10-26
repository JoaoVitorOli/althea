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
