package foldersync

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzJoin reads anything as a sync folder's header, as another program could have written:
// a key or an error, never a panic, nor a derivation that takes the machine.
func FuzzJoin(f *testing.F) {
	f.Add([]byte(`{"version":1,"kdf":"argon2id","params":{"time":1,"memory":1024,"threads":1},"salt":"AAAAAAAAAAAAAAAAAAAAAA==","check":"AA=="}`))
	f.Add([]byte(`{"version":1,"kdf":"argon2id","params":{"time":4294967295,"memory":4294967295,"threads":255}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, headerName), data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = Join(dir, "passphrase")
	})
}
