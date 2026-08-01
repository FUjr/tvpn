package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("proxy master key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}
func (c *Cipher) Encrypt(value string, aad []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, []byte(value), aad), nil
}
func (c *Cipher) Decrypt(value, aad []byte) (string, error) {
	size := c.aead.NonceSize()
	if len(value) < size {
		return "", errors.New("invalid encrypted value")
	}
	plain, err := c.aead.Open(nil, value[:size], value[size:], aad)
	return string(plain), err
}
