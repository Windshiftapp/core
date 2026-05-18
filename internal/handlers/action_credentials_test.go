package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"windshift/internal/models"
)

// TestActionCredential_JSONNeverLeaksCiphertext is the most security-critical
// invariant for this feature: any JSON encoding of an ActionCredential must
// not include the encrypted_secret column. The model uses `json:"-"` on that
// field; this test pins that contract so a future refactor (e.g. switching
// to a different serializer, exposing the type in a generated DTO, or
// adding an `Inspect` helper) can't silently regress.
func TestActionCredential_JSONNeverLeaksCiphertext(t *testing.T) {
	plaintext := "supersecret-plaintext-token"
	cipher := "BASE64-CIPHERTEXT-DO-NOT-LEAK-AbCd1234"
	cred := &models.ActionCredential{
		ID:              5,
		Name:            "GitHub PAT",
		CredentialType:  models.CredentialBearerToken,
		EncryptedSecret: cipher,
		SecretPrefix:    "ghp_…",
		IsEnabled:       true,
		CreatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Direct encoding of the full model.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(cred); err != nil {
		t.Fatalf("encode credential: %v", err)
	}
	body := buf.String()
	if strings.Contains(body, cipher) {
		t.Fatalf("encrypted_secret leaked into JSON: %s", body)
	}
	if strings.Contains(body, plaintext) {
		t.Fatalf("plaintext leaked into JSON: %s", body)
	}
	if !strings.Contains(body, `"has_secret"`) && strings.Contains(body, `"encrypted_secret"`) {
		t.Fatalf("JSON included encrypted_secret column: %s", body)
	}

	// Encoding via Sanitize() — the path every handler uses.
	buf.Reset()
	if err := json.NewEncoder(&buf).Encode(cred.Sanitize()); err != nil {
		t.Fatalf("encode sanitized: %v", err)
	}
	s := buf.String()
	if strings.Contains(s, cipher) || strings.Contains(s, plaintext) {
		t.Fatalf("sanitized JSON leaked secret material: %s", s)
	}
	if !strings.Contains(s, `"has_secret":true`) {
		t.Fatalf("sanitized JSON missing has_secret flag: %s", s)
	}
	if !strings.Contains(s, `"secret_prefix":"ghp_…"`) {
		t.Fatalf("sanitized JSON missing prefix: %s", s)
	}
}

// TestSanitizeList preserves order and never returns nil (clients prefer
// `[]` over `null`).
func TestSanitizeList(t *testing.T) {
	got := sanitizeList(nil)
	if got == nil {
		t.Fatalf("sanitizeList(nil) should return empty slice, not nil")
	}
	if len(got) != 0 {
		t.Fatalf("sanitizeList(nil) should be empty, got %d", len(got))
	}

	creds := []*models.ActionCredential{
		{ID: 1, Name: "a", EncryptedSecret: "ct1"},
		{ID: 2, Name: "b", EncryptedSecret: ""},
	}
	out := sanitizeList(creds)
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
	if out[0].ID != 1 || out[1].ID != 2 {
		t.Errorf("order not preserved: %+v", out)
	}
	if !out[0].HasSecret || out[1].HasSecret {
		t.Errorf("HasSecret mismatch: %+v", out)
	}
}
