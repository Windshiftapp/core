package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"windshift/internal/database"
	"windshift/internal/models"
)

const (
	SCIMTokenPrefix     = "scim_"
	SCIMTokenBodyBytes  = 32 // Random bytes for token body (becomes 64 hex chars)
	// Final token: scim_ (5) + 64 hex chars = 69 bytes (under bcrypt's 72 byte limit)
)

// SCIMTokenManager handles SCIM token operations
type SCIMTokenManager struct {
	db database.Database
}

// NewSCIMTokenManager creates a new SCIM token manager
func NewSCIMTokenManager(db database.Database) *SCIMTokenManager {
	return &SCIMTokenManager{db: db}
}

// GenerateToken creates a cryptographically secure SCIM token
func (tm *SCIMTokenManager) GenerateToken() (string, error) {
	// Generate random bytes for the token body
	tokenBytes := make([]byte, SCIMTokenBodyBytes)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Convert to hex and add prefix
	tokenBody := hex.EncodeToString(tokenBytes)
	fullToken := SCIMTokenPrefix + tokenBody

	return fullToken, nil
}

// HashToken creates a bcrypt hash of the token for secure storage
func (tm *SCIMTokenManager) HashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash token: %w", err)
	}
	return string(hash), nil
}

// GetTokenPrefix returns the visible prefix of a token for identification
func (tm *SCIMTokenManager) GetTokenPrefix(token string) string {
	if len(token) > len(SCIMTokenPrefix)+8 { // Show first 13 chars: scim_12345678...
		return token[:len(SCIMTokenPrefix)+8] + "..."
	}
	return token
}

// ValidateToken checks if a SCIM token is valid and returns the token record
func (tm *SCIMTokenManager) ValidateToken(token string) (*models.SCIMToken, error) {
	// Check token format
	if !strings.HasPrefix(token, SCIMTokenPrefix) || len(token) < 20 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Extract token prefix for efficient database lookup
	tokenPrefix := tm.GetTokenPrefix(token)

	// Query tokens matching prefix
	rows, err := tm.db.Query(`
		SELECT t.id, t.name, t.token_hash, t.token_prefix, t.is_active,
		       t.created_by, t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as created_by_name
		FROM scim_tokens t
		LEFT JOIN users u ON t.created_by = u.id
		WHERE t.token_prefix = ?
		  AND t.is_active = true
		  AND (t.expires_at IS NULL OR t.expires_at > CURRENT_TIMESTAMP)
	`, tokenPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scimToken models.SCIMToken
		var tokenHash string
		var createdBy sql.NullInt64
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&scimToken.ID, &scimToken.Name, &tokenHash, &scimToken.TokenPrefix,
			&scimToken.IsActive, &createdBy, &expiresAt, &lastUsedAt,
			&scimToken.CreatedAt, &scimToken.UpdatedAt, &scimToken.CreatedByName,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Check if token hash matches
		err = bcrypt.CompareHashAndPassword([]byte(tokenHash), []byte(token))
		if err != nil {
			continue // Hash doesn't match, try next token
		}

		// Convert nullable fields
		if createdBy.Valid {
			id := int(createdBy.Int64)
			scimToken.CreatedBy = &id
		}
		if expiresAt.Valid {
			scimToken.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			scimToken.LastUsedAt = &lastUsedAt.Time
		}

		// Update last used timestamp asynchronously
		go tm.updateLastUsed(scimToken.ID)

		return &scimToken, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// updateLastUsed updates the last_used_at timestamp for a token
func (tm *SCIMTokenManager) updateLastUsed(tokenID int) {
	_, _ = tm.db.Exec(`UPDATE scim_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, tokenID)
}

// CreateToken creates a new SCIM token
func (tm *SCIMTokenManager) CreateToken(createdByUserID int, request models.SCIMTokenCreate) (*models.SCIMTokenResponse, error) {
	// Generate token
	token, err := tm.GenerateToken()
	if err != nil {
		return nil, err
	}

	// Hash token
	tokenHash, err := tm.HashToken(token)
	if err != nil {
		return nil, err
	}

	// Get token prefix for identification
	tokenPrefix := tm.GetTokenPrefix(token)

	// Insert token into database
	var tokenID int64
	err = tm.db.QueryRow(`
		INSERT INTO scim_tokens (name, token_hash, token_prefix, is_active, created_by, expires_at)
		VALUES (?, ?, ?, true, ?, ?)
		RETURNING id
	`, request.Name, tokenHash, tokenPrefix, createdByUserID, request.ExpiresAt).Scan(&tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// Get the created token details
	scimToken, err := tm.GetTokenByID(int(tokenID))
	if err != nil {
		return nil, err
	}

	return &models.SCIMTokenResponse{
		Token:     token, // Only returned on creation
		SCIMToken: *scimToken,
	}, nil
}

// GetTokenByID retrieves a token by ID (without the actual token value)
func (tm *SCIMTokenManager) GetTokenByID(id int) (*models.SCIMToken, error) {
	var token models.SCIMToken
	var createdBy sql.NullInt64
	var expiresAt, lastUsedAt sql.NullTime

	err := tm.db.QueryRow(`
		SELECT t.id, t.name, t.token_prefix, t.is_active,
		       t.created_by, t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as created_by_name
		FROM scim_tokens t
		LEFT JOIN users u ON t.created_by = u.id
		WHERE t.id = ?
	`, id).Scan(
		&token.ID, &token.Name, &token.TokenPrefix, &token.IsActive,
		&createdBy, &expiresAt, &lastUsedAt, &token.CreatedAt, &token.UpdatedAt,
		&token.CreatedByName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Convert nullable fields
	if createdBy.Valid {
		id := int(createdBy.Int64)
		token.CreatedBy = &id
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}

	return &token, nil
}

// ListTokens returns all SCIM tokens (active and inactive)
func (tm *SCIMTokenManager) ListTokens() ([]models.SCIMToken, error) {
	rows, err := tm.db.Query(`
		SELECT t.id, t.name, t.token_prefix, t.is_active,
		       t.created_by, t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as created_by_name
		FROM scim_tokens t
		LEFT JOIN users u ON t.created_by = u.id
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.SCIMToken
	for rows.Next() {
		var token models.SCIMToken
		var createdBy sql.NullInt64
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&token.ID, &token.Name, &token.TokenPrefix, &token.IsActive,
			&createdBy, &expiresAt, &lastUsedAt, &token.CreatedAt, &token.UpdatedAt,
			&token.CreatedByName,
		)
		if err != nil {
			continue
		}

		if createdBy.Valid {
			id := int(createdBy.Int64)
			token.CreatedBy = &id
		}
		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			token.LastUsedAt = &lastUsedAt.Time
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

// RevokeToken revokes a SCIM token by setting is_active to false
func (tm *SCIMTokenManager) RevokeToken(tokenID int) error {
	result, err := tm.db.Exec(`
		UPDATE scim_tokens SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, tokenID)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// DeleteToken permanently deletes a SCIM token
func (tm *SCIMTokenManager) DeleteToken(tokenID int) error {
	result, err := tm.db.Exec(`DELETE FROM scim_tokens WHERE id = ?`, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// CleanupExpiredTokens removes expired tokens from the database
func (tm *SCIMTokenManager) CleanupExpiredTokens() (int64, error) {
	result, err := tm.db.Exec(`
		DELETE FROM scim_tokens
		WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	return result.RowsAffected()
}

// GetActiveTokenCount returns the count of active, non-expired tokens
func (tm *SCIMTokenManager) GetActiveTokenCount() (int, error) {
	var count int
	err := tm.db.QueryRow(`
		SELECT COUNT(*) FROM scim_tokens
		WHERE is_active = true
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active tokens: %w", err)
	}
	return count, nil
}
