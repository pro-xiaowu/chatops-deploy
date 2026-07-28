package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var ErrInvalidCiphertext = errors.New("invalid ciphertext")

type SecretBox struct{ aead cipher.AEAD }

func NewSecretBox(encodedKey string) (*SecretBox, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("master key must decode to 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Seal(plaintext, aad []byte) ([]byte, error) {
	if len(aad) == 0 {
		return nil, ErrInvalidCiphertext
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := append([]byte{1}, nonce...)
	return b.aead.Seal(out, nonce, plaintext, aad), nil
}

func (b *SecretBox) Open(payload, aad []byte) ([]byte, error) {
	if len(aad) == 0 || len(payload) < 1+b.aead.NonceSize() || payload[0] != 1 {
		return nil, ErrInvalidCiphertext
	}
	nonce := payload[1 : 1+b.aead.NonceSize()]
	plain, err := b.aead.Open(nil, nonce, payload[1+b.aead.NonceSize():], aad)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plain, nil
}
