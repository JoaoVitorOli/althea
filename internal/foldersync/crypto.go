package foldersync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// headerName is the folder's header: how its key is derived from the passphrase, and a check
// that tells a wrong passphrase.
const headerName = "althea-sync.json"

// checkText is sealed in the header, for a passphrase to be checked before anything is read.
const checkText = "althea-sync"

// ErrWrongPassphrase is returned by Join when the passphrase is not the one the folder was set
// up with.
var ErrWrongPassphrase = errors.New("the passphrase is not the one this sync folder was set up with")

// KDF are the Argon2id parameters of a new folder.
type KDF struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
}

// NewFolderKDF is what a folder set up now derives its key with: the RFC 9106 second
// recommendation, which takes about a tenth of a second on a laptop.
var NewFolderKDF = KDF{Time: 3, Memory: 64 * 1024, Threads: 4}

type header struct {
	Version int    `json:"version"`
	KDF     string `json:"kdf"`
	Params  KDF    `json:"params"`
	Salt    []byte `json:"salt"`
	Check   []byte `json:"check"`
}

// Join returns the key of the sync folder dir for passphrase. A folder without a header, as a
// new one, is set up with it; a folder set up before must be given the same passphrase.
func Join(dir, passphrase string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, headerName))
	if errors.Is(err, fs.ErrNotExist) {
		return create(dir, passphrase)
	}
	if err != nil {
		return nil, err
	}
	var h header
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("%s is damaged: %w", headerName, err)
	}
	if h.Version != 1 || h.KDF != "argon2id" {
		return nil, fmt.Errorf("%s was written by a newer Althea", headerName)
	}
	// A header asking for much more than Althea's own parameters is not one it wrote: deriving
	// with them could take minutes and gigabytes.
	if h.Params.Time == 0 || h.Params.Time > 10 || h.Params.Memory == 0 || h.Params.Memory > 256<<10 ||
		h.Params.Threads == 0 || len(h.Salt) < 16 {
		return nil, fmt.Errorf("%s has parameters out of range", headerName)
	}
	key := derive(passphrase, h.Salt, h.Params)
	if got, err := open(key, h.Check, "header"); err != nil || string(got) != checkText {
		return nil, ErrWrongPassphrase
	}
	return key, nil
}

func create(dir, passphrase string) ([]byte, error) {
	h := header{Version: 1, KDF: "argon2id", Params: NewFolderKDF, Salt: make([]byte, 16)}
	_, _ = rand.Read(h.Salt) // crypto/rand.Read never fails
	key := derive(passphrase, h.Salt, h.Params)
	h.Check = seal(key, []byte(checkText), "header")
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(dir, headerName), data); err != nil {
		return nil, err
	}
	return key, nil
}

func derive(passphrase string, salt []byte, p KDF) []byte {
	return argon2.IDKey([]byte(passphrase), salt, p.Time, p.Memory, p.Threads, 32)
}

// seal encrypts data with AES-256-GCM, bound to what it is, so that one sealed thing cannot pass
// for another: the nonce, then the ciphertext.
func seal(key, data []byte, what string) []byte {
	gcm := aead(key)
	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(data)+gcm.Overhead())
	_, _ = rand.Read(nonce)
	return gcm.Seal(nonce, nonce, data, []byte("althea-sync/"+what))
}

func open(key, sealed []byte, what string) ([]byte, error) {
	gcm := aead(key)
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("too short")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte("althea-sync/"+what))
}

func aead(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err) // the key is always 32 bytes
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return gcm
}

// writeFile writes a file whole or not at all: a sync tool never sees half of it.
func writeFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}
