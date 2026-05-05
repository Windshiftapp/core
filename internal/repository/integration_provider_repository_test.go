package repository

import (
	"errors"
	"testing"

	"windshift/internal/database"
)

// TestIntegrationProviderRepository covers the partial-update path that
// replaced six separate UPDATE statements in the handler. Each update only
// writes the fields the caller passes; missing rows return ErrNotFound;
// slug collisions return ErrDuplicateEntry.
func TestIntegrationProviderRepository(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE integration_providers (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			oauth_client_id TEXT,
			oauth_client_secret_encrypted TEXT,
			provider_config TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	repo := NewIntegrationProviderRepository(db)

	t.Run("Create + GetByID round-trip", func(t *testing.T) {
		err := repo.Create(IntegrationProviderInsert{
			ID:                         "p-1",
			Slug:                       "notion",
			Name:                       "Notion",
			ProviderType:               "notion",
			Enabled:                    true,
			OAuthClientID:              "client-abc",
			OAuthClientSecretEncrypted: "ciphertext",
			ProviderConfig:             `{"foo":"bar"}`,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID("p-1")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Slug != "notion" || got.Name != "Notion" || !got.Enabled {
			t.Errorf("got = %+v", got)
		}
		if !got.HasOAuthClientSecret {
			t.Errorf("HasOAuthClientSecret should be true when ciphertext is stored")
		}
		if got.OAuthClientID != "client-abc" {
			t.Errorf("OAuthClientID = %q, want client-abc", got.OAuthClientID)
		}
	})

	t.Run("Create with duplicate slug returns ErrDuplicateEntry", func(t *testing.T) {
		err := repo.Create(IntegrationProviderInsert{
			ID:           "p-2",
			Slug:         "notion", // collides with p-1
			Name:         "Other",
			ProviderType: "notion",
			Enabled:      true,
		})
		if !errors.Is(err, ErrDuplicateEntry) {
			t.Errorf("err = %v, want ErrDuplicateEntry", err)
		}
	})

	t.Run("GetByID missing returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID("missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Update with name-only leaves other fields untouched", func(t *testing.T) {
		newName := "Notion Renamed"
		if err := repo.Update("p-1", IntegrationProviderUpdate{Name: &newName}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := repo.GetByID("p-1")
		if got.Name != newName {
			t.Errorf("Name = %q, want %q", got.Name, newName)
		}
		if got.Slug != "notion" {
			t.Errorf("Slug = %q, expected unchanged", got.Slug)
		}
		if got.OAuthClientID != "client-abc" {
			t.Errorf("OAuthClientID = %q, expected unchanged", got.OAuthClientID)
		}
		if !got.HasOAuthClientSecret {
			t.Errorf("HasOAuthClientSecret should still be true")
		}
	})

	t.Run("Update with empty fields is a no-op but verifies existence", func(t *testing.T) {
		if err := repo.Update("p-1", IntegrationProviderUpdate{}); err != nil {
			t.Errorf("Update empty on existing: %v", err)
		}
		if err := repo.Update("missing", IntegrationProviderUpdate{}); !errors.Is(err, ErrNotFound) {
			t.Errorf("Update empty on missing: %v, want ErrNotFound", err)
		}
	})

	t.Run("Update missing returns ErrNotFound", func(t *testing.T) {
		newName := "x"
		err := repo.Update("missing", IntegrationProviderUpdate{Name: &newName})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Delete + Delete-again returns ErrNotFound second time", func(t *testing.T) {
		if err := repo.Delete("p-1"); err != nil {
			t.Fatalf("first Delete: %v", err)
		}
		err := repo.Delete("p-1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("second Delete = %v, want ErrNotFound", err)
		}
	})
}
