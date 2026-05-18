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

// Two SecretEncryption instances built from the same server secret but
// different HKDF info labels must not be able to read each other's ciphertext.
// This is the cross-realm isolation guarantee that lets action credentials
// share SSO_SECRET safely.
func TestSecretEncryption_WithInfo_IsolatesRealms(t *testing.T) {
	ssoEnc := NewSecretEncryption(testSecret)
	actionEnc := NewSecretEncryptionWithInfo(testSecret, "windshift-action-credentials-encryption-v1")

	plaintext := "credential-value"
	actionCT, err := actionEnc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("action encrypt: %v", err)
	}

	// The action-realm ciphertext must round-trip in its own realm.
	if got, err := actionEnc.Decrypt(actionCT); err != nil || got != plaintext {
		t.Fatalf("action round-trip: got %q err %v", got, err)
	}

	// And the SSO realm must NOT be able to decrypt it.
	if _, err := ssoEnc.Decrypt(actionCT); err == nil {
		t.Fatalf("SSO realm decrypted an action-credential ciphertext — realms are not isolated")
	}

	// Conversely the action realm must not decrypt SSO-realm ciphertext.
	ssoCT, err := ssoEnc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("sso encrypt: %v", err)
	}
	if _, err := actionEnc.Decrypt(ssoCT); err == nil {
		t.Fatalf("action realm decrypted an SSO ciphertext — realms are not isolated")
	}
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
