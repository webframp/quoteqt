package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor("test-passphrase")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "super-secret-session-token-12345"

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if ciphertext == plaintext {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDifferentNonces(t *testing.T) {
	enc, _ := NewEncryptor("test-passphrase")
	plaintext := "same-plaintext"

	c1, _ := enc.Encrypt(plaintext)
	c2, _ := enc.Encrypt(plaintext)

	if c1 == c2 {
		t.Error("encrypting same plaintext should produce different ciphertexts")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	enc1, _ := NewEncryptor("passphrase-1")
	enc2, _ := NewEncryptor("passphrase-2")

	ciphertext, _ := enc1.Encrypt("secret")

	_, err := enc2.Decrypt(ciphertext)
	if err == nil {
		t.Error("decrypting with wrong key should fail")
	}
}

func TestNewEncryptorEmptyKey(t *testing.T) {
	_, err := NewEncryptor("")
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	enc, _ := NewEncryptor("test")
	_, err := enc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecryptTooShort(t *testing.T) {
	enc, _ := NewEncryptor("test")
	_, err := enc.Decrypt("YWJj") // "abc" in base64, too short for nonce
	if err != ErrInvalidCiphertext {
		t.Errorf("expected ErrInvalidCiphertext, got %v", err)
	}
}
