package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"windshift/internal/models"
)

// TestSanitizeCapabilitiesForWorkspace pins the workspace-listing contract:
// any sensitive header value or credential ID must be removed before the
// config string is sent to a workspace-admin client.
func TestSanitizeCapabilitiesForWorkspace(t *testing.T) {
	cfg := models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://api.example.com/**"},
		DefaultHeaders: map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer SUPERSECRET",   // legacy/inline → must be stripped
			"X-Api-Key":     "leaked-key-1234",      // must be stripped
		},
		TimeoutSecs: 30,
		Auth: &models.HTTPAuthRef{
			CredentialID: 42,
			HeaderName:   "Authorization",
			Scheme:       "Bearer",
		},
		SecretHeaderRefs: map[string]int{
			"X-Api-Key":   99,
			"X-Signature": 14,
		},
	}
	raw, _ := json.Marshal(cfg)
	in := []*models.ActionCapability{{
		ID:             1,
		Name:           "GitHub",
		CapabilityType: models.CapabilityHTTPClient,
		Config:         string(raw),
	}, {
		ID:             2,
		Name:           "OpenAI",
		CapabilityType: models.CapabilityLLMConnection,
		Config:         `{"connection_id":7}`,
	}}

	out := sanitizeCapabilitiesForWorkspace(in)

	// Non-HTTP capabilities pass through unchanged.
	if out[1].Config != in[1].Config {
		t.Errorf("non-HTTP capability was modified: %q", out[1].Config)
	}

	// HTTP capability config must not contain plaintext secrets or
	// credential IDs.
	if strings.Contains(out[0].Config, "SUPERSECRET") {
		t.Fatalf("plaintext Authorization leaked: %s", out[0].Config)
	}
	if strings.Contains(out[0].Config, "leaked-key-1234") {
		t.Fatalf("plaintext X-Api-Key leaked: %s", out[0].Config)
	}
	// Credential IDs must be hidden — auth.credential_id zeroed, header refs
	// replaced with the presence sentinel.
	if strings.Contains(out[0].Config, `"credential_id":42`) {
		t.Fatalf("auth credential_id leaked: %s", out[0].Config)
	}
	if strings.Contains(out[0].Config, `"X-Api-Key":99`) || strings.Contains(out[0].Config, `"X-Signature":14`) {
		t.Fatalf("secret_header_refs credential ids leaked: %s", out[0].Config)
	}

	// Non-sensitive defaults survive.
	if !strings.Contains(out[0].Config, `"Accept":"application/json"`) {
		t.Errorf("non-sensitive header was stripped: %s", out[0].Config)
	}

	// Sanity: sanitization must not mutate the input slice's underlying
	// capability rows — that would corrupt admin views in the same process.
	var inputCfg models.HTTPClientConfig
	if err := json.Unmarshal([]byte(in[0].Config), &inputCfg); err != nil {
		t.Fatalf("re-read input config: %v", err)
	}
	if inputCfg.Auth == nil || inputCfg.Auth.CredentialID != 42 {
		t.Errorf("input capability was mutated by sanitization (auth.credential_id is now %v)", inputCfg.Auth)
	}
	if _, ok := inputCfg.DefaultHeaders["Authorization"]; !ok {
		t.Errorf("input capability default_headers was mutated by sanitization")
	}
}

// TestIsSensitiveHeaderName guards the allowlist that classifies HTTP header
// names. Adding a header here means new rejection rules in
// validateCapabilityConfig + validateHTTPRequestNodeConfig.
func TestIsSensitiveHeaderName(t *testing.T) {
	for _, name := range []string{
		"Authorization", "authorization", "AUTHORIZATION",
		"X-API-Key", "x-api-key", "Api-Key",
		"Cookie", "Set-Cookie", "Proxy-Authorization",
		"x-github-token", "Custom-Token", "Some-Secret", "Foo-Password",
		"X-Access-Token", "X-Auth-Token", "X-Secret-Key",
	} {
		if !models.IsSensitiveHeaderName(name) {
			t.Errorf("expected %q to be sensitive", name)
		}
	}
	for _, name := range []string{
		"Accept", "Content-Type", "User-Agent", "X-Request-Id",
		"Accept-Language", "If-None-Match", "",
	} {
		if models.IsSensitiveHeaderName(name) {
			t.Errorf("expected %q to NOT be sensitive", name)
		}
	}
}
