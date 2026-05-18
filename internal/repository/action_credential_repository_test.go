package repository

import (
	"errors"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestActionCredentialRepository covers the credential CRUD path:
// global vs workspace scoping, rotation, metadata-only update, and the
// "list for workspace" view that includes globals.
func TestActionCredentialRepository(t *testing.T) {
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
		t.Fatalf("seed schema: %v", err)
	}

	repo := NewActionCredentialRepository(db)
	creator := 10

	t.Run("Create global", func(t *testing.T) {
		c := &models.ActionCredential{
			Name:            "GitHub global",
			CredentialType:  models.CredentialBearerToken,
			WorkspaceID:     nil,
			CreatedBy:       &creator,
			EncryptedSecret: "ciphertext-global",
			SecretPrefix:    "ghp_…",
			IsEnabled:       true,
		}
		id, err := repo.CreateActionCredential(c)
		if err != nil {
			t.Fatalf("create global: %v", err)
		}
		got, err := repo.GetActionCredentialByID(id)
		if err != nil {
			t.Fatalf("get global: %v", err)
		}
		if got.WorkspaceID != nil {
			t.Errorf("WorkspaceID should be nil for global, got %v", *got.WorkspaceID)
		}
		if got.EncryptedSecret != "ciphertext-global" {
			t.Errorf("EncryptedSecret mismatch")
		}
		if got.SecretPrefix != "ghp_…" {
			t.Errorf("SecretPrefix = %q", got.SecretPrefix)
		}
	})

	t.Run("Create workspace-scoped", func(t *testing.T) {
		ws := 1
		c := &models.ActionCredential{
			Name:            "alpha token",
			CredentialType:  models.CredentialAPIKey,
			WorkspaceID:     &ws,
			CreatedBy:       &creator,
			EncryptedSecret: "ciphertext-alpha",
			SecretMetadata:  `{"provider":"linear"}`,
			IsEnabled:       true,
		}
		_, err := repo.CreateActionCredential(c)
		if err != nil {
			t.Fatalf("create workspace: %v", err)
		}
	})

	t.Run("Rejects empty ciphertext", func(t *testing.T) {
		_, err := repo.CreateActionCredential(&models.ActionCredential{
			Name:           "bad",
			CredentialType: models.CredentialBearerToken,
		})
		if err == nil {
			t.Fatalf("expected error on empty ciphertext")
		}
	})

	t.Run("ListForWorkspace includes globals", func(t *testing.T) {
		list, err := repo.ListActionCredentialsForWorkspace(1, true)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("want 2 (1 global + 1 workspace), got %d", len(list))
		}
	})

	t.Run("ListForWorkspace excludes other workspaces", func(t *testing.T) {
		list, err := repo.ListActionCredentialsForWorkspace(2, false)
		if err != nil {
			t.Fatalf("list ws2: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("want 0 for empty workspace, got %d", len(list))
		}
	})

	t.Run("Rotate replaces ciphertext but keeps metadata", func(t *testing.T) {
		list, _ := repo.ListActionCredentialsForWorkspace(1, false)
		if len(list) == 0 {
			t.Fatalf("no workspace creds to rotate")
		}
		id := list[0].ID
		if err := repo.RotateActionCredential(id, "ciphertext-alpha-v2", "lin_…"); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		got, err := repo.GetActionCredentialByID(id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.EncryptedSecret != "ciphertext-alpha-v2" {
			t.Errorf("ciphertext not rotated: %q", got.EncryptedSecret)
		}
		if got.SecretPrefix != "lin_…" {
			t.Errorf("prefix not updated: %q", got.SecretPrefix)
		}
		if got.SecretMetadata != `{"provider":"linear"}` {
			t.Errorf("metadata clobbered: %q", got.SecretMetadata)
		}
	})

	t.Run("UpdateMetadata never touches ciphertext", func(t *testing.T) {
		list, _ := repo.ListActionCredentialsForWorkspace(1, false)
		id := list[0].ID
		updated := *list[0]
		updated.Name = "alpha token (renamed)"
		updated.IsEnabled = false
		updated.SecretMetadata = `{"provider":"linear","scope":"read"}`
		if err := repo.UpdateActionCredentialMetadata(&updated); err != nil {
			t.Fatalf("update metadata: %v", err)
		}
		got, _ := repo.GetActionCredentialByID(id)
		if got.Name != "alpha token (renamed)" || got.IsEnabled {
			t.Errorf("metadata not applied: %+v", got)
		}
		if got.EncryptedSecret != "ciphertext-alpha-v2" {
			t.Errorf("ciphertext changed unexpectedly: %q", got.EncryptedSecret)
		}
	})

	t.Run("GetByID returns ErrNotFound for missing row", func(t *testing.T) {
		_, err := repo.GetActionCredentialByID(99999)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete removes row", func(t *testing.T) {
		list, _ := repo.ListActionCredentialsForWorkspace(1, false)
		if len(list) == 0 {
			t.Skip("nothing to delete")
		}
		if err := repo.DeleteActionCredential(list[0].ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := repo.GetActionCredentialByID(list[0].ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})
}

func TestSanitizeStripsCiphertext(t *testing.T) {
	c := &models.ActionCredential{
		ID:              7,
		Name:            "x",
		CredentialType:  models.CredentialBearerToken,
		EncryptedSecret: "should-never-leak",
		SecretPrefix:    "tok_…",
		IsEnabled:       true,
	}
	s := c.Sanitize()
	if !s.HasSecret {
		t.Errorf("HasSecret should be true")
	}
	// Ensure the sanitized struct has no field exposing ciphertext.
	// (Compile-time guarantee: ActionCredentialSanitized has no EncryptedSecret field.)
	if s.SecretPrefix != "tok_…" {
		t.Errorf("prefix lost in sanitize")
	}
}

func TestSecretPrefixFor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"short", ""}, // 5 chars, below 2*prefixLen threshold → fully masked
		{"ghp_AbCdEfGhIj", "ghp_…"},
	}
	for _, tc := range cases {
		got := models.SecretPrefixFor(tc.in)
		if got != tc.want {
			t.Errorf("SecretPrefixFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
