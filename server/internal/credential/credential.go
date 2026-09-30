// Package credential seals secrets at rest with AES-256-GCM.
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

const version byte = 1

var (
	ErrInvalidKey  = errors.New("credential key must be 32 bytes")
	ErrMalformed   = errors.New("credential ciphertext malformed")
	ErrUnsupported = errors.New("credential version unsupported")
)

type Cipher struct{ aead cipher.AEAD }

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return &Cipher{aead: aead}, nil
}

// Seal outputs version || nonce || ciphertext, binding purpose so blobs cannot be swapped between columns.
func (c *Cipher) Seal(plaintext []byte, purpose string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{version}, nonce...)
	return c.aead.Seal(out, nonce, plaintext, []byte(purpose)), nil
}

func (c *Cipher) Open(sealed []byte, purpose string) ([]byte, error) {
	size := c.aead.NonceSize()
	if len(sealed) < 1+size+c.aead.Overhead() {
		return nil, ErrMalformed
	}
	if sealed[0] != version {
		return nil, ErrUnsupported
	}
	plain, err := c.aead.Open(nil, sealed[1:1+size], sealed[1+size:], []byte(purpose))
	if err != nil {
		return nil, ErrMalformed
	}
	return plain, nil
}
