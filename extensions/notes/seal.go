package notes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// sealer encrypts the notes with AES-256-GCM (docs/adr/0008-secrets.md).
type sealer struct{ aead cipher.AEAD }

func newSealer(key []byte) (*sealer, error) {
	if len(key) != 32 {
		return nil, errors.New("notes key: want 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal encrypts data, a random nonce before it.
func (s *sealer) seal(data []byte) []byte {
	nonce := make([]byte, s.aead.NonceSize(), s.aead.NonceSize()+len(data)+s.aead.Overhead())
	_, _ = rand.Read(nonce) // crypto/rand.Read never fails
	return s.aead.Seal(nonce, nonce, data, nil)
}

// open decrypts what seal made.
func (s *sealer) open(sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("sealed note too short")
	}
	return s.aead.Open(nil, sealed[:n], sealed[n:], nil)
}
