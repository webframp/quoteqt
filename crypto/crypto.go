// Package crypto provides encryption utilities for sensitive data storage.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey       = errors.New("encryption key must be non-empty")
	ErrInvalidCiphertext = errors.New("ciphertext too short")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

// Encryptor handles AES-GCM encryption/decryption with a derived key.
type Encryptor struct {
	key []byte
}

// NewEncryptor creates an Encryptor from a passphrase.
// The passphrase is hashed with SHA-256 to derive a 32-byte key.
func NewEncryptor(passphrase string) (*Encryptor, error) {
	if passphrase == "" {
		return nil, ErrInvalidKey
	}
	hash := sha256.Sum256([]byte(passphrase))
	return &Encryptor{key: hash[:]}, nil
}

// Encrypt encrypts plaintext using AES-GCM.
// Returns base64-encoded ciphertext (nonce prepended).
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext (with prepended nonce).
func (e *Encryptor) Decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
