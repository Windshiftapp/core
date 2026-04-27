package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

var (
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// HKDF info label for secret encryption. Domain-separated from the cookie-key
// derivation in handlers/sso.go so a future use of the same SSO_SECRET (e.g.
// a different at-rest cipher) gets a different key.
const secretEncryptionInfo = "windshift-sso-secret-encryption-v1" //nolint:gosec // G101: HKDF domain-separation label, not a credential

// SecretEncryption handles encryption/decryption of sensitive data like client secrets.
//
// Keys: encrypt always uses the HKDF-derived key. Decrypt tries HKDF first,
// then falls back to a legacy SHA-256(serverSecret) key so ciphertexts written
// before the HKDF migration keep decrypting. Any successful legacy decrypt
// is a candidate for re-encryption by the caller.
type SecretEncryption struct {
	key       []byte // primary (HKDF)
	legacyKey []byte // SHA-256(serverSecret) — for back-compat decrypt only
}

// NewSecretEncryption creates a new encryption instance
// The serverSecret should be a long, random string stored securely (e.g., in environment variable)
func NewSecretEncryption(serverSecret string) *SecretEncryption {
	hkdfKey := make([]byte, 32)
	reader := hkdf.New(sha256.New, []byte(serverSecret), nil, []byte(secretEncryptionInfo))
	if _, err := io.ReadFull(reader, hkdfKey); err != nil {
		// HKDF over SHA-256 with constant inputs cannot fail in practice;
		// fall through to the legacy key as a last resort.
		legacy := sha256.Sum256([]byte(serverSecret))
		return &SecretEncryption{key: legacy[:], legacyKey: legacy[:]}
	}

	legacy := sha256.Sum256([]byte(serverSecret))
	return &SecretEncryption{key: hkdfKey, legacyKey: legacy[:]}
}

// Encrypt encrypts plaintext and returns base64-encoded ciphertext
func (e *SecretEncryption) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", ErrEncryptionFailed
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrEncryptionFailed
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrEncryptionFailed
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext and returns plaintext.
// Tries the HKDF key first, then the legacy SHA-256 key for back-compat.
func (e *SecretEncryption) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	if plaintext, err := decryptWith(e.key, data); err == nil {
		return plaintext, nil
	}
	if e.legacyKey != nil {
		if plaintext, err := decryptWith(e.legacyKey, data); err == nil {
			return plaintext, nil
		}
	}
	return "", ErrDecryptionFailed
}

func decryptWith(key, data []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrDecryptionFailed
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
