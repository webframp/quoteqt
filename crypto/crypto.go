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

	"golang.org/x/crypto/pbkdf2"
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

// Fixed salt for key derivation. Since we use a strong passphrase (32 bytes
// from openssl rand), a fixed salt is acceptable. The salt prevents rainbow
// table attacks and ensures the same passphrase produces different keys in
// different applications.
var derivationSalt = []byte("quoteqt-nightbot-session-v1")

// NewEncryptor creates an Encryptor from a passphrase.
// The passphrase is stretched using PBKDF2-SHA256 to derive a 32-byte key.
func NewEncryptor(passphrase string) (*Encryptor, error) {
	if passphrase == "" {
		return nil, ErrInvalidKey
	}
	// PBKDF2 with 100,000 iterations as recommended by OWASP
	key := pbkdf2.Key([]byte(passphrase), derivationSalt, 100000, 32, sha256.New)
	return &Encryptor{key: key}, nil
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
