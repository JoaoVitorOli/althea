package translator

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

// FuzzExchange reads anything as the engine's answer: a translation or an error, never a panic,
// nor more memory than an answer may take.
func FuzzExchange(f *testing.F) {
	for _, s := range []string{"ok 5\nolá!", "error 4\nfail", "ok -1\n", "ok 99999999999\n", "ok\n", "ok 3\nab"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = exchange(io.Discard, bufio.NewReader(bytes.NewReader(data)), "en-pt", "hello")
	})
}
