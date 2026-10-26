package ui

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestHeldImagesAreScaledDown(t *testing.T) {
	b, err := decodeScaled(pngOf(t, 3840, 2160), maxHeldSide)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := b.Size(); w != 768 || h != 432 {
		t.Errorf("size %v×%v, want 768×432", w, h)
	}
	if _, err := decodeScaled([]byte("not an image"), maxHeldSide); err == nil {
		t.Error("garbage decoded")
	}
}

func TestFewImagesAreHeld(t *testing.T) {
	ic := &Icons{}
	for i := range maxHeld + 5 {
		ic.Data(pngOf(t, 10+i, 10))
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if len(ic.held) != maxHeld {
		t.Errorf("held %d images, want %d", len(ic.held), maxHeld)
	}
}
