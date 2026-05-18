package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const testServerSecret = "test-server-secret-with-sufficient-length-for-derivation"

func newTestCredentialService(t *testing.T) (*ActionCredentialService, database.Database) {
	t.Helper()
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
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
		INSERT INTO workspaces (id, name) VALUES (1, 'alpha'), (2, 'beta');
		INSERT INTO users (id) VALUES (10);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repository.NewActionCredentialRepository(db)
	return NewActionCredentialService(repo, testServerSecret), db
}

func TestActionCredentialService_CreateEncryptsAndStripsPlaintext(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	created, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "GitHub PAT",
		CredentialType: models.CredentialBearerToken,
		Secret:         "ghp_AbCdEfGhIjKlMnOpQrStUv",
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.EncryptedSecret == "" {
		t.Fatalf("encrypted_secret empty after Create")
	}
	if strings.Contains(created.EncryptedSecret, "ghp_") {
		t.Fatalf("ciphertext contains plaintext fragment: %q", created.EncryptedSecret)
	}
	if created.SecretPrefix != "ghp_…" {
		t.Errorf("SecretPrefix = %q, want %q", created.SecretPrefix, "ghp_…")
	}

	// Sanitized form must not have any way to recover plaintext or ciphertext.
	sanitized := created.Sanitize()
	if !sanitized.HasSecret {
		t.Errorf("HasSecret should be true")
	}
}

func TestActionCredentialService_ResolveDecrypts(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	const plaintext = "ghp_AbCdEfGhIjKlMnOpQrStUv"
	created, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "GitHub PAT",
		CredentialType: models.CredentialBearerToken,
		Secret:         plaintext,
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, _, err := svc.Resolve(context.Background(), created.ID, 1)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != plaintext {
		t.Errorf("resolve plaintext mismatch: got %q want %q", got, plaintext)
	}
}

func TestActionCredentialService_Resolve_ScopeMismatch(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	ws := 1
	created, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "alpha-only",
		CredentialType: models.CredentialBearerToken,
		Secret:         "alpha-secret-1234567890",
		WorkspaceID:    &ws,
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Same workspace works.
	if _, _, err := svc.Resolve(context.Background(), created.ID, 1); err != nil {
		t.Errorf("same-workspace resolve failed: %v", err)
	}
	// Other workspace must be blocked.
	if _, _, err := svc.Resolve(context.Background(), created.ID, 2); !errors.Is(err, ErrCredentialScopeMismatch) {
		t.Errorf("other-workspace resolve: want ErrCredentialScopeMismatch, got %v", err)
	}
}

func TestActionCredentialService_Resolve_Disabled(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	disabled := false
	created, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "disabled",
		CredentialType: models.CredentialBearerToken,
		Secret:         "secret-value-1234567890",
		IsEnabled:      &disabled,
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Resolve(context.Background(), created.ID, 1); !errors.Is(err, ErrCredentialDisabled) {
		t.Errorf("want ErrCredentialDisabled, got %v", err)
	}
}

func TestActionCredentialService_Rotate(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	created, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "rotate-me",
		CredentialType: models.CredentialBearerToken,
		Secret:         "old-secret-1234567890",
	}, ptrInt(10))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Rotate(created.ID, models.RotateActionCredentialRequest{
		Secret: "new-secret-9876543210",
	}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	plaintext, _, err := svc.Resolve(context.Background(), created.ID, 1)
	if err != nil {
		t.Fatalf("resolve after rotate: %v", err)
	}
	if plaintext != "new-secret-9876543210" {
		t.Errorf("rotated plaintext mismatch: got %q", plaintext)
	}
}

func TestActionCredentialService_RejectsInvalidType(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	_, err := svc.Create(models.CreateActionCredentialRequest{
		Name:           "x",
		CredentialType: models.ActionCredentialType("malformed_type"),
		Secret:         "anything",
	}, nil)
	if err == nil {
		t.Fatalf("expected error for invalid type")
	}
}

func TestActionCredentialService_RejectsSensitiveMetadataKeys(t *testing.T) {
	svc, _ := newTestCredentialService(t)
	for _, key := range []string{"secret", "token", "password", "client_secret", "api_token"} {
		_, err := svc.Create(models.CreateActionCredentialRequest{
			Name:           "bad-" + key,
			CredentialType: models.CredentialBearerToken,
			Secret:         "real-secret-1234567890",
			SecretMetadata: `{"` + key + `":"leak"}`,
		}, nil)
		if err == nil {
			t.Errorf("metadata key %q must be rejected", key)
		}
	}
}

func TestCanCapabilityReference(t *testing.T) {
	global := &models.ActionCredential{}
	scoped := &models.ActionCredential{WorkspaceID: ptrInt(1)}

	if !CanCapabilityReference(global, nil) {
		t.Error("global capability should be allowed to reference global credential")
	}
	if CanCapabilityReference(scoped, nil) {
		t.Error("global capability must NOT reference workspace credential")
	}
	if !CanCapabilityReference(global, []int{2, 3}) {
		t.Error("scoped capability should always be able to reference global credential")
	}
	if !CanCapabilityReference(scoped, []int{1, 2}) {
		t.Error("scoped capability covering workspace 1 should accept credential scoped to workspace 1")
	}
	if CanCapabilityReference(scoped, []int{2, 3}) {
		t.Error("scoped capability not covering workspace 1 must reject credential scoped to workspace 1")
	}
}

func ptrInt(v int) *int { return &v }
