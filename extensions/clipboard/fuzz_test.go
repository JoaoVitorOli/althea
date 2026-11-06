package clipboard

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// FuzzThumbnail previews anything copied as an image: a preview or an error, never a panic.
func FuzzThumbnail(f *testing.F) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 300, 20)))
	f.Add(buf.Bytes())
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("\xff\xd8\xff\xe0"))
	f.Fuzz(func(_ *testing.T, data []byte) { _, _ = thumbnail(data) })
}
