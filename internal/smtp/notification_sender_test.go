package smtp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/utils"
)

// fakeEncryptor satisfies Encryptor with a deterministic AES-GCM round-trip
// so we can verify decryptOrLegacy + dispatch's password handling without
// pulling in the real *sso.SecretEncryption.
type fakeEncryptor struct {
	key []byte
}

func newFakeEncryptor(t *testing.T) *fakeEncryptor {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return &fakeEncryptor{key: k}
}

func (f *fakeEncryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (f *fakeEncryptor) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func TestDispatch_RejectsEmptyEncryption(t *testing.T) {
	// The default branch in the encryption switch used to silently fall
	// through to plaintext smtp.SendMail, leaking AUTH PLAIN credentials on
	// any deployment with a typo'd or unset SMTPEncryption value.
	s := &NotificationSMTPSender{}
	cfg := &models.ChannelConfig{
		SMTPHost:       "smtp.example.com",
		SMTPPort:       25,
		SMTPFromEmail:  "from@example.com",
		SMTPEncryption: "",
	}
	err := s.dispatch(cfg, "to@example.com", "BODY")
	if err == nil {
		t.Fatal("expected error for empty SMTPEncryption, got nil")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected encryption-not-allowed error, got %v", err)
	}
}

func TestDispatch_RejectsUnknownEncryption(t *testing.T) {
	s := &NotificationSMTPSender{}
	cfg := &models.ChannelConfig{
		SMTPHost:       "smtp.example.com",
		SMTPPort:       25,
		SMTPFromEmail:  "from@example.com",
		SMTPEncryption: "plain", // typo
	}
	err := s.dispatch(cfg, "to@example.com", "BODY")
	if err == nil {
		t.Fatal("expected error for unknown SMTPEncryption, got nil")
	}
	if !strings.Contains(err.Error(), `"plain"`) {
		t.Errorf("expected error to mention bad value, got %v", err)
	}
}

func TestDispatch_RejectsLoopbackHost(t *testing.T) {
	// SMTPHost is admin-configurable through PUT /channels/{id}/config.
	// A channel manager could otherwise point it at 127.0.0.1 to scan the
	// internal network via SMTP error responses. SafeNetDialer must reject
	// the dial before the TCP handshake.
	s := &NotificationSMTPSender{}
	cfg := &models.ChannelConfig{
		SMTPHost:       "127.0.0.1",
		SMTPPort:       1, // any closed port; SafeNetDialer rejects before connect
		SMTPFromEmail:  "from@example.com",
		SMTPEncryption: "ssl",
	}
	err := s.dispatch(cfg, "to@example.com", "BODY")
	if err == nil {
		t.Fatal("expected SSRF guard to reject loopback, got nil error")
	}
	if !errors.Is(err, utils.ErrBlockedSSRFAddr) && !strings.Contains(err.Error(), "blocked IP range") {
		t.Errorf("expected ErrBlockedSSRFAddr, got %v", err)
	}
}

func TestDecryptOrLegacy_PassthroughOnEmpty(t *testing.T) {
	// Empty value is a no-op even with an encryptor wired — keeps the
	// "no SMTP password configured" path silent.
	enc := newFakeEncryptor(t)
	got, err := decryptOrLegacy(enc, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty passthrough, got %q", got)
	}
}

func TestDecryptOrLegacy_PassthroughOnLegacyPlaintext(t *testing.T) {
	// Pre-migration rows hold short plaintext passwords. The 28-byte/base64
	// heuristic must keep returning them verbatim instead of failing decrypt.
	enc := newFakeEncryptor(t)
	got, err := decryptOrLegacy(enc, "shortpass")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "shortpass" {
		t.Errorf("expected legacy plaintext passthrough, got %q", got)
	}
}

func TestDecryptOrLegacy_RoundTrip(t *testing.T) {
	enc := newFakeEncryptor(t)
	encrypted, err := enc.Encrypt("hunter2-with-some-padding-to-hit-min")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plain, err := decryptOrLegacy(enc, encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "hunter2-with-some-padding-to-hit-min" {
		t.Errorf("round-trip mismatch: got %q", plain)
	}
}
