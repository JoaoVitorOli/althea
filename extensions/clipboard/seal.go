package clipboard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// sealer encrypts what the history keeps with AES-256-GCM, and makes digests that tell equal
// contents apart without showing them (docs/adr/0008-secrets.md).
type sealer struct {
	aead   cipher.AEAD
	macKey []byte
}

func newSealer(key []byte) (*sealer, error) {
	if len(key) != 32 {
		return nil, errors.New("clipboard key: want 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("althea.clipboard digest"))
	return &sealer{aead: aead, macKey: mac.Sum(nil)}, nil
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
		return nil, errors.New("sealed data too short")
	}
	return s.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// digest identifies a content: equal contents of a kind have equal digests.
func (s *sealer) digest(kind string, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, s.macKey)
	mac.Write([]byte(kind))
	for _, p := range parts {
		mac.Write([]byte{0})
		mac.Write(p)
	}
	return mac.Sum(nil)
}
