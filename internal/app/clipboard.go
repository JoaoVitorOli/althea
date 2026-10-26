package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"

	"github.com/JoaoVitorOli/althea/extensions/clipboard"
	"github.com/JoaoVitorOli/althea/internal/platform"
)

// clipboardKeyName names the clipboard history's key in the vault.
const clipboardKeyName = "Althea/althea.clipboard/key"

// clipboardPlatform lets Clipboard History watch the clipboard and keep its key in the vault;
// nothing where the system can do neither.
func (a *althea) clipboardPlatform() clipboard.Platform {
	w, vault := a.services.Clipboard, a.services.Vault
	if w == nil || vault == nil {
		return clipboard.Platform{}
	}
	return clipboard.Platform{
		Watch: func(copied func(clipboard.Copied)) (func(), error) { return watchClipboard(w, copied) },
		Key:   func() ([]byte, error) { return vaultKey(vault, clipboardKeyName) },
	}
}

// watchClipboard reads each new clipboard content that its app did not mark as private and
// hands it to copied, one at a time, on a goroutine of its own.
func watchClipboard(w platform.ClipboardWatcher, copied func(clipboard.Copied)) (func(), error) {
	changes := make(chan platform.ClipboardChange, 16)
	done := make(chan struct{})
	stop, err := w.Watch(func(c platform.ClipboardChange) {
		if c.Private {
			return
		}
		select {
		case changes <- c:
		default: // copies faster than they are read: the newest come later
		}
	})
	if err != nil {
		return nil, err
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case c := <-changes:
				// The app that copied may still be writing its other formats.
				time.Sleep(50 * time.Millisecond)
				if content, ok := readClipboard(); ok {
					content.Source = c.Source
					copied(content)
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		stop()
		close(done)
		<-finished
	}, nil
}

// readClipboard reads what the clipboard holds: files, an image or text.
func readClipboard() (clipboard.Copied, bool) {
	data, err := mygo.Clipboard.Read(transfer.FileList, transfer.PNG, transfer.HTML, transfer.Text)
	if err != nil {
		return clipboard.Copied{}, false
	}
	var c clipboard.Copied
	if files, err := data.Files(); err == nil && len(files) > 0 {
		c.Files = files
		return c, true
	}
	if img, err := data.Read(transfer.PNG); err == nil && len(img) > 0 {
		c.Image = img
		return c, true
	}
	if text, err := data.Read(transfer.Text); err == nil && len(text) > 0 {
		c.Text = string(text)
		if html, err := data.Read(transfer.HTML); err == nil {
			c.HTML = string(html)
		}
		return c, true
	}
	return c, false
}

// vaultKey returns a random 32-byte key kept in the vault under name, made the first time.
func vaultKey(vault platform.Vault, name string) ([]byte, error) {
	secret, ok, err := vault.Get(name)
	if err != nil {
		return nil, err
	}
	if ok {
		key, err := base64.StdEncoding.DecodeString(secret)
		if err != nil || len(key) != 32 {
			return nil, errors.New("the key in the vault is damaged; delete " + name + " in Credential Manager to start a new history")
		}
		return key, nil
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key) // crypto/rand.Read never fails
	if err := vault.Set(name, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("keep the key: %w", err)
	}
	return key, nil
}
