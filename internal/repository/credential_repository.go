package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CredentialRepository provides data access for legacy and WebAuthn credentials.
type CredentialRepository struct {
	db database.Database
}

// NewCredentialRepository creates a CredentialRepository.
func NewCredentialRepository(db database.Database) *CredentialRepository {
	return &CredentialRepository{db: db}
}

// LegacyCredentialSummary is the minimal information needed to delete/audit a legacy credential.
type LegacyCredentialSummary struct {
	Type string
	Name string
}

// ListForUser returns all legacy and WebAuthn credentials for a user.
func (r *CredentialRepository) ListForUser(userID int) ([]models.UserCredential, error) {
	credentials := []models.UserCredential{}

	rows, err := r.db.Query(`
		SELECT id, user_id, credential_type, credential_name, is_active, created_at, updated_at, last_used_at
		FROM user_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query legacy credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cred models.UserCredential
		var lastUsedAt sql.NullTime
		var id int
		if err := rows.Scan(&id, &cred.UserID, &cred.CredentialType, &cred.CredentialName, &cred.IsActive, &cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("scan legacy credential: %w", err)
		}
		cred.ID = strconv.Itoa(id)
		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}
		credentials = append(credentials, cred)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy credentials: %w", err)
	}

	webauthnRows, err := r.db.Query(`
		SELECT id, credential_name, created_at, last_used_at
		FROM webauthn_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query webauthn credentials: %w", err)
	}
	defer func() { _ = webauthnRows.Close() }()

	for webauthnRows.Next() {
		var id, credentialName string
		var createdAt time.Time
		var lastUsedAt sql.NullTime
		if err := webauthnRows.Scan(&id, &credentialName, &createdAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("scan webauthn credential: %w", err)
		}
		cred := models.UserCredential{
			ID:             id,
			UserID:         userID,
			CredentialType: "fido",
			CredentialName: credentialName,
			IsActive:       true,
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		}
		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}
		credentials = append(credentials, cred)
	}
	if err := webauthnRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webauthn credentials: %w", err)
	}

	sort.Slice(credentials, func(i, j int) bool { return credentials[i].CreatedAt.After(credentials[j].CreatedAt) })
	return credentials, nil
}

// CreateSSH inserts an SSH credential and returns its id.
func (r *CredentialRepository) CreateSSH(userID int, name, credentialJSON, fingerprint string) (int, error) {
	var id int
	if err := r.db.QueryRow(`
		INSERT INTO user_credentials (user_id, credential_type, credential_name, credential_data, public_key_fingerprint)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, userID, "ssh", name, credentialJSON, fingerprint).Scan(&id); err != nil {
		return 0, fmt.Errorf("create ssh credential: %w", err)
	}
	return id, nil
}

// HasActiveFIDO reports whether a user has an active legacy FIDO/passkey credential.
func (r *CredentialRepository) HasActiveFIDO(userID int) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM user_credentials
		WHERE user_id = ? AND credential_type = 'fido' AND is_active = true
	`, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count active fido credentials for user %d: %w", userID, err)
	}
	return count > 0, nil
}

// GetLegacySummary returns type/name for a legacy credential scoped to user.
func (r *CredentialRepository) GetLegacySummary(id, userID int) (LegacyCredentialSummary, error) {
	var out LegacyCredentialSummary
	err := r.db.QueryRow(`SELECT credential_type, credential_name FROM user_credentials WHERE id = ? AND user_id = ?`, id, userID).Scan(&out.Type, &out.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyCredentialSummary{}, ErrNotFound
	}
	if err != nil {
		return LegacyCredentialSummary{}, fmt.Errorf("get legacy credential %d: %w", id, err)
	}
	return out, nil
}

// DeleteLegacy removes a legacy credential scoped to user.
func (r *CredentialRepository) DeleteLegacy(id, userID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM user_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete legacy credential %d: %w", id, err)
	}
	return nil
}

// GetWebAuthnName returns name for a WebAuthn credential scoped to user.
func (r *CredentialRepository) GetWebAuthnName(id string, userID int) (string, error) {
	var name string
	err := r.db.QueryRow(`SELECT credential_name FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get webauthn credential %q: %w", id, err)
	}
	return name, nil
}

// DeleteWebAuthn removes a WebAuthn credential scoped to user.
func (r *CredentialRepository) DeleteWebAuthn(id string, userID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete webauthn credential %q: %w", id, err)
	}
	return nil
}
