package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"testing"
)

const testSecret = "test-server-secret-with-sufficient-length-for-derivation"

func TestSecretEncryption_RoundTrip(t *testing.T) {
	enc := NewSecretEncryption(testSecret)
	plaintext := "client-secret-foo"

	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "" {
		t.Fatalf("ciphertext empty")
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestSecretEncryption_EmptyStringPassesThrough(t *testing.T) {
	enc := NewSecretEncryption(testSecret)

	if got, err := enc.Encrypt(""); err != nil || got != "" {
		t.Fatalf("encrypt empty: got %q err %v", got, err)
	}
	if got, err := enc.Decrypt(""); err != nil || got != "" {
		t.Fatalf("decrypt empty: got %q err %v", got, err)
	}
}

// Ciphertexts written under the legacy SHA-256(serverSecret) key must still
// decrypt after the HKDF migration. Synthesises a legacy ciphertext, then
// confirms the production Decrypt path reads it.
func TestSecretEncryption_LegacyCiphertextDecrypts(t *testing.T) {
	plaintext := "legacy-client-secret"
	legacyCT, err := encryptWithLegacyKey(testSecret, plaintext)
	if err != nil {
		t.Fatalf("legacy encrypt setup: %v", err)
	}

	enc := NewSecretEncryption(testSecret)
	got, err := enc.Decrypt(legacyCT)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if got != plaintext {
		t.Fatalf("legacy round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

// encryptWithLegacyKey produces a ciphertext using the pre-HKDF derivation
// (sha256(serverSecret) directly as the AES-GCM key). Test-only.
func encryptWithLegacyKey(serverSecret, plaintext string) (string, error) {
	key := sha256.Sum256([]byte(serverSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func TestSecretEncryption_RejectsTamperedCiphertext(t *testing.T) {
	enc := NewSecretEncryption(testSecret)
	ct, err := enc.Encrypt("orig")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := enc.Decrypt(tampered); err == nil {
		t.Fatalf("tampered ciphertext should not decrypt")
	}
}
