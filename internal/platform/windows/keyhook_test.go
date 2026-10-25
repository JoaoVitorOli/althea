//go:build windows

package windows

import (
	"testing"
	"unsafe"
)

func TestInputMatchesTheSystemLayout(t *testing.T) {
	// sizeof(INPUT) on 64-bit Windows; SendInput rejects any other size.
	if got := unsafe.Sizeof(keyboardInput{}); got != 40 {
		t.Errorf("sizeof(keyboardInput) = %d, want 40", got)
	}
	if got := unsafe.Offsetof(keyboardInput{}.vk); got != 8 {
		t.Errorf("offset of wVk = %d, want 8", got)
	}
}
