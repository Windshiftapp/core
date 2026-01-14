//go:build test

package sso

import (
	"testing"
)

func TestSecretEncryption_RoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		serverSecret string
		plaintext    string
	}{
		{
			name:         "short secret",
			serverSecret: "short",
			plaintext:    "my-client-secret",
		},
		{
			name:         "32-byte secret",
			serverSecret: "12345678901234567890123456789012",
			plaintext:    "another-secret-value",
		},
		{
			name:         "long secret",
			serverSecret: "this-is-a-very-long-secret-that-is-much-longer-than-32-bytes",
			plaintext:    "test-data-to-encrypt",
		},
		{
			name:         "empty plaintext",
			serverSecret: "test-secret",
			plaintext:    "",
		},
		{
			name:         "unicode plaintext",
			serverSecret: "test-secret",
			plaintext:    "секрет-данные-🔐",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewSecretEncryption(tt.serverSecret)

			// Encrypt
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}

			// Empty plaintext should return empty ciphertext
			if tt.plaintext == "" {
				if ciphertext != "" {
					t.Error("Empty plaintext should produce empty ciphertext")
				}
				return
			}

			// Ciphertext should be different from plaintext
			if ciphertext == tt.plaintext {
				t.Error("Ciphertext should not equal plaintext")
			}

			// Decrypt
			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}

			// Should match original
			if decrypted != tt.plaintext {
				t.Errorf("Decrypted text doesn't match. Expected: %s, Got: %s", tt.plaintext, decrypted)
			}
		})
	}
}

func TestSecretEncryption_DifferentSecrets_DifferentCiphertext(t *testing.T) {
	enc1 := NewSecretEncryption("secret1")
	enc2 := NewSecretEncryption("secret2")

	plaintext := "test-data"

	cipher1, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encryption 1 failed: %v", err)
	}

	cipher2, err := enc2.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encryption 2 failed: %v", err)
	}

	// Different secrets should produce different ciphertext
	// (due to random nonces, even same secret produces different ciphertext,
	// but different secrets definitely should)
	if cipher1 == cipher2 {
		t.Error("Different secrets should produce different ciphertext")
	}

	// Cross-decryption should fail
	_, err = enc2.Decrypt(cipher1)
	if err == nil {
		t.Error("Decryption with wrong secret should fail")
	}

	_, err = enc1.Decrypt(cipher2)
	if err == nil {
		t.Error("Decryption with wrong secret should fail")
	}
}

func TestSecretEncryption_SameSecret_DifferentNonces(t *testing.T) {
	enc := NewSecretEncryption("test-secret")
	plaintext := "same-data"

	// Encrypt the same data twice
	cipher1, _ := enc.Encrypt(plaintext)
	cipher2, _ := enc.Encrypt(plaintext)

	// Should produce different ciphertext due to random nonces
	if cipher1 == cipher2 {
		t.Error("Same plaintext should produce different ciphertext due to random nonces")
	}

	// Both should decrypt to same value
	decrypted1, _ := enc.Decrypt(cipher1)
	decrypted2, _ := enc.Decrypt(cipher2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both ciphertexts should decrypt to original plaintext")
	}
}

func TestSecretEncryption_InvalidCiphertext(t *testing.T) {
	enc := NewSecretEncryption("test-secret")

	// Invalid base64
	_, err := enc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("Should fail on invalid base64")
	}

	// Valid base64 but too short (less than nonce size)
	_, err = enc.Decrypt("YWJj") // "abc" in base64
	if err == nil {
		t.Error("Should fail on ciphertext shorter than nonce")
	}

	// Valid base64, correct length, but tampered data
	validCipher, _ := enc.Encrypt("test")
	if len(validCipher) > 10 {
		// Tamper with the ciphertext
		tampered := validCipher[:len(validCipher)-5] + "XXXXX"
		_, err = enc.Decrypt(tampered)
		if err == nil {
			t.Error("Should fail on tampered ciphertext")
		}
	}
}
