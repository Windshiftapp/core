package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"

	"golang.org/x/crypto/bcrypt"
)

const (
	TokenPrefix  = "crw_"
	TokenLength  = 32 // Total token length including prefix (to keep under bcrypt 72 byte limit)
	PrefixLength = 4  // Length of visible prefix for identification
)

// TokenManager handles API token operations
type TokenManager struct {
	db           database.Database
	tokenTracker *services.TokenTracker
}

// NewTokenManager creates a new token manager
func NewTokenManager(db database.Database, tokenTracker *services.TokenTracker) *TokenManager {
	return &TokenManager{
		db:           db,
		tokenTracker: tokenTracker,
	}
}

// GenerateToken creates a cryptographically secure API token
func (tm *TokenManager) GenerateToken() (string, error) {
	// Generate random bytes for the token body
	tokenBytes := make([]byte, TokenLength-len(TokenPrefix))
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Convert to hex and add prefix
	tokenBody := hex.EncodeToString(tokenBytes)
	fullToken := TokenPrefix + tokenBody

	return fullToken, nil
}

// HashToken creates a bcrypt hash of the token for secure storage
func (tm *TokenManager) HashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash token: %w", err)
	}
	return string(hash), nil
}

// GetTokenPrefix returns the visible prefix of a token for identification
func (tm *TokenManager) GetTokenPrefix(token string) string {
	if len(token) > PrefixLength+8 { // Show first 12 chars: crw_12345678
		return token[:PrefixLength+8] + "..."
	}
	return token
}

// ValidateToken checks if a token is valid and returns the associated user
func (tm *TokenManager) ValidateToken(token string) (*models.User, *models.APIToken, error) {
	// Check token format
	if !strings.HasPrefix(token, TokenPrefix) || len(token) < 20 {
		return nil, nil, fmt.Errorf("invalid token format")
	}

	// Extract token prefix for efficient database lookup (matches stored format with "...")
	tokenPrefix := tm.GetTokenPrefix(token)

	// Query tokens matching prefix to avoid full table scan and excessive bcrypt comparisons
	// Use CURRENT_TIMESTAMP which works in both SQLite and PostgreSQL
	rows, err := tm.db.Query(`
		SELECT t.id, t.user_id, t.name, t.token_hash, t.token_prefix, t.permissions,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.id, u.email, u.username, u.first_name, u.last_name, u.is_active
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.token_prefix = ?
		  AND (t.expires_at IS NULL OR t.expires_at > CURRENT_TIMESTAMP)
	`, tokenPrefix)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var apiToken models.APIToken
		var user models.User
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&apiToken.ID, &apiToken.UserID, &apiToken.Name, &apiToken.Token,
			&apiToken.TokenPrefix, &apiToken.Permissions,
			&expiresAt, &lastUsedAt, &apiToken.CreatedAt, &apiToken.UpdatedAt,
			&user.ID, &user.Email, &user.Username, &user.FirstName,
			&user.LastName, &user.IsActive,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Check if token hash matches
		err = bcrypt.CompareHashAndPassword([]byte(apiToken.Token), []byte(token))
		if err != nil {
			continue // Hash doesn't match, try next token
		}

		// Convert nullable times
		if expiresAt.Valid {
			apiToken.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			apiToken.LastUsedAt = &lastUsedAt.Time
		}

		// Check if user is active
		if !user.IsActive {
			return nil, nil, fmt.Errorf("user account is disabled")
		}

		// Update last used timestamp
		go tm.updateLastUsed(apiToken.ID)

		return &user, &apiToken, nil
	}

	return nil, nil, fmt.Errorf("invalid token")
}

// CreateToken creates a new API token for a user
func (tm *TokenManager) CreateToken(userID int, request models.APITokenCreate) (*models.APITokenResponse, error) {
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

	// Convert permissions to JSON
	permissionsJSON, err := json.Marshal(request.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	// Insert token into database using RETURNING clause (supported by both SQLite 3.35+ and PostgreSQL)
	var tokenID int64
	err = tm.db.QueryRow(`
		INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, permissions, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`, userID, request.Name, tokenHash, tokenPrefix, string(permissionsJSON), request.ExpiresAt).Scan(&tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// Get the created token details
	apiToken, err := tm.GetTokenByID(int(tokenID))
	if err != nil {
		return nil, err
	}

	return &models.APITokenResponse{
		Token:    token, // Only returned on creation
		APIToken: *apiToken,
	}, nil
}

// GetTokenByID retrieves a token by ID (without the actual token value)
func (tm *TokenManager) GetTokenByID(id int) (*models.APIToken, error) {
	var token models.APIToken
	var expiresAt, lastUsedAt sql.NullTime

	err := tm.db.QueryRow(`
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.id = ?
	`, id).Scan(
		&token.ID, &token.UserID, &token.Name, &token.TokenPrefix, &token.Permissions, &token.IsTemporary,
		&expiresAt, &lastUsedAt, &token.CreatedAt, &token.UpdatedAt,
		&token.UserEmail, &token.UserName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Convert nullable times
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}

	return &token, nil
}

// GetUserTokens retrieves all tokens for a user (without the actual token values)
// Excludes expired tokens and temporary SSH session tokens
func (tm *TokenManager) GetUserTokens(userID int) ([]models.APIToken, error) {
	rows, err := tm.db.Query(`
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.user_id = ?
		  AND (t.is_temporary = 0 OR t.is_temporary = false)
		  AND (t.expires_at IS NULL OR t.expires_at > CURRENT_TIMESTAMP)
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.APIToken
	for rows.Next() {
		var token models.APIToken
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&token.ID, &token.UserID, &token.Name, &token.TokenPrefix, &token.Permissions, &token.IsTemporary,
			&expiresAt, &lastUsedAt, &token.CreatedAt, &token.UpdatedAt,
			&token.UserEmail, &token.UserName,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Convert nullable times
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

// RevokeToken deletes a token (revokes access)
func (tm *TokenManager) RevokeToken(tokenID, userID int) error {
	result, err := tm.db.ExecWrite("DELETE FROM api_tokens WHERE id = ? AND user_id = ?", tokenID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found or not owned by user")
	}

	return nil
}

// updateLastUsed updates the last_used_at timestamp for a token
// This now uses the TokenTracker for batched writes instead of immediate database updates
func (tm *TokenManager) updateLastUsed(tokenID int) {
	if tm.tokenTracker != nil {
		tm.tokenTracker.RecordTokenUse(tokenID)
	}
}

// CleanupExpiredTokens removes expired tokens from the database
func (tm *TokenManager) CleanupExpiredTokens() (int, error) {
	result, err := tm.db.ExecWrite("DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP")
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// CheckTokenPermissions checks if a token has specific permissions
func (tm *TokenManager) CheckTokenPermissions(token *models.APIToken, requiredPermissions []string) bool {
	var permissions []string
	err := json.Unmarshal([]byte(token.Permissions), &permissions)
	if err != nil {
		return false
	}

	// Check if token has admin permission (grants all access)
	for _, perm := range permissions {
		if perm == "admin" {
			return true
		}
	}

	// Check if token has all required permissions
	for _, required := range requiredPermissions {
		found := false
		for _, perm := range permissions {
			if perm == required || perm == "write" && required == "read" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
