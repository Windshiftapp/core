package services

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// TestBuildHTTPHeadersWithCredentials_InjectsBearer verifies the end-to-end
// credential-resolution path: a capability with an Auth ref is decrypted at
// runtime and the resulting Authorization header is set on the merged map.
// The plaintext must NEVER appear in the capability config or any returned
// error string.
func TestBuildHTTPHeadersWithCredentials_InjectsBearer(t *testing.T) {
	as, credSvc := newTestActionRuntime(t)

	cred, err := credSvc.Create(models.CreateActionCredentialRequest{
		Name:           "GitHub PAT",
		CredentialType: models.CredentialBearerToken,
		Secret:         "ghp_RuntimeSecret_1234567890",
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create cred: %v", err)
	}

	httpCfg := &models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://api.github.com/**"},
		DefaultHeaders:     map[string]string{"Accept": "application/vnd.github+json"},
		TimeoutSecs:        30,
		Auth: &models.HTTPAuthRef{
			CredentialID: cred.ID,
			HeaderName:   "Authorization",
			Scheme:       "Bearer",
		},
	}

	headers, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, map[string]string{"X-Trace": "abc"}, 0, 7)
	if err != nil {
		t.Fatalf("build headers: %v", err)
	}
	authVal := headers["Authorization"]
	if authVal != "Bearer ghp_RuntimeSecret_1234567890" {
		t.Errorf("Authorization header = %q", authVal)
	}
	if headers["Accept"] != "application/vnd.github+json" {
		t.Errorf("default headers lost: %+v", headers)
	}
	if headers["X-Trace"] != "abc" {
		t.Errorf("caller headers lost: %+v", headers)
	}
}

func TestBuildHTTPHeadersWithCredentials_BasicAuthEncodes(t *testing.T) {
	as, credSvc := newTestActionRuntime(t)
	cred, _ := credSvc.Create(models.CreateActionCredentialRequest{
		Name:           "basic",
		CredentialType: models.CredentialBasicAuth,
		Secret:         "alice:hunter2-correct-horse-battery",
	}, ptrInt(10))

	httpCfg := &models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://example/**"},
		Auth: &models.HTTPAuthRef{
			CredentialID: cred.ID,
			HeaderName:   "Authorization",
		},
	}
	headers, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, nil, 0, 1)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:hunter2-correct-horse-battery"))
	if headers["Authorization"] != want {
		t.Errorf("Basic auth header = %q, want %q", headers["Authorization"], want)
	}
}

func TestBuildHTTPHeadersWithCredentials_SecretHeaderRefs(t *testing.T) {
	as, credSvc := newTestActionRuntime(t)
	apiKey, _ := credSvc.Create(models.CreateActionCredentialRequest{
		Name: "api-key", CredentialType: models.CredentialAPIKey, Secret: "k-1234567890abcdef",
	}, ptrInt(10))
	sig, _ := credSvc.Create(models.CreateActionCredentialRequest{
		Name: "sig", CredentialType: models.CredentialCustomHeader, Secret: "sig-1234567890abcdef",
	}, ptrInt(10))

	httpCfg := &models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://example/**"},
		SecretHeaderRefs: map[string]int{
			"X-Api-Key":   apiKey.ID,
			"X-Signature": sig.ID,
		},
	}
	headers, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, nil, 0, 1)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if headers["X-Api-Key"] != "k-1234567890abcdef" {
		t.Errorf("X-Api-Key = %q", headers["X-Api-Key"])
	}
	if headers["X-Signature"] != "sig-1234567890abcdef" {
		t.Errorf("X-Signature = %q", headers["X-Signature"])
	}
}

func TestBuildHTTPHeadersWithCredentials_RejectsSensitiveCallerHeader(t *testing.T) {
	as, _ := newTestActionRuntime(t)
	httpCfg := &models.HTTPClientConfig{AllowedURLPatterns: []string{"https://example/**"}}
	_, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, map[string]string{
		"Authorization": "Bearer raw-token-from-caller",
	}, 0, 1)
	if err == nil {
		t.Fatalf("expected rejection of caller-supplied Authorization header")
	}
	// Don't leak the offending plaintext via the error.
	if strings.Contains(err.Error(), "raw-token-from-caller") {
		t.Errorf("error message contains plaintext: %v", err)
	}
}

func TestBuildHTTPHeadersWithCredentials_DropsLegacyInlineSensitiveDefault(t *testing.T) {
	as, _ := newTestActionRuntime(t)
	httpCfg := &models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://example/**"},
		DefaultHeaders: map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer SHOULD-NOT-LEAK-LEGACY",
		},
	}
	headers, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, nil, 0, 1)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, ok := headers["Authorization"]; ok {
		t.Fatalf("legacy inline Authorization header leaked into runtime headers: %+v", headers)
	}
	if headers["Accept"] != "application/json" {
		t.Errorf("non-sensitive header dropped: %+v", headers)
	}
}

func TestBuildHTTPHeadersWithCredentials_ScopeMismatchFails(t *testing.T) {
	as, credSvc := newTestActionRuntime(t)
	ws := 5
	cred, _ := credSvc.Create(models.CreateActionCredentialRequest{
		Name: "alpha-only", CredentialType: models.CredentialBearerToken,
		Secret: "alpha-only-1234567890", WorkspaceID: &ws,
	}, ptrInt(10))

	httpCfg := &models.HTTPClientConfig{
		AllowedURLPatterns: []string{"https://example/**"},
		Auth: &models.HTTPAuthRef{
			CredentialID: cred.ID,
			HeaderName:   "Authorization",
			Scheme:       "Bearer",
		},
	}
	if _, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, nil, 5, 1); err != nil {
		t.Errorf("same-workspace resolve should succeed: %v", err)
	}
	if _, err := as.buildHTTPHeadersWithCredentials(context.Background(), httpCfg, nil, 6, 1); err == nil {
		t.Errorf("cross-workspace resolve should fail")
	}
}

// newTestActionRuntime builds the minimum ActionService + credential service
// wiring for header-resolution tests. No event loop, no caches — just enough
// to call buildHTTPHeadersWithCredentials.
func newTestActionRuntime(t *testing.T) (*ActionService, *ActionCredentialService) {
	t.Helper()
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE workspaces (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT);
		CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT);
		CREATE TABLE action_credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			credential_type TEXT NOT NULL,
			workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
			created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			encrypted_secret TEXT NOT NULL,
			secret_prefix TEXT,
			secret_metadata TEXT,
			is_enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO workspaces (id, name) VALUES (5, 'alpha'), (6, 'beta');
		INSERT INTO users (id) VALUES (10);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	credSvc := NewActionCredentialService(repository.NewActionCredentialRepository(db), testServerSecret)
	as := &ActionService{
		db:                db,
		credentialService: credSvc,
	}
	return as, credSvc
}
