package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/cacheutil"
	"windshift/internal/database"
	"windshift/internal/models"

	"github.com/allegro/bigcache/v3"
	"golang.org/x/crypto/bcrypt"
)

const (
	TokenPrefix  = "crw_"
	TokenLength  = 32 // Total token length including prefix (to keep under bcrypt 72 byte limit)
	PrefixLength = 4  // Length of visible prefix for identification
)

// Granular resource:action scope constants
const (
	// Items (includes comments, attachments, history)
	ScopeItemsRead   = "items:read"
	ScopeItemsWrite  = "items:write"
	ScopeItemsDelete = "items:delete"

	// Workspaces
	ScopeWorkspacesRead   = "workspaces:read"
	ScopeWorkspacesWrite  = "workspaces:write"
	ScopeWorkspacesDelete = "workspaces:delete"

	// Configuration resources (read-only via API)
	ScopeStatusesRead     = "statuses:read"
	ScopeWorkflowsRead    = "workflows:read"
	ScopeItemTypesRead    = "item-types:read"
	ScopePrioritiesRead   = "priorities:read"
	ScopeCustomFieldsRead = "custom-fields:read"

	// Users
	ScopeUsersRead = "users:read"

	// Milestones
	ScopeMilestonesRead   = "milestones:read"
	ScopeMilestonesWrite  = "milestones:write"
	ScopeMilestonesDelete = "milestones:delete"

	// Iterations
	ScopeIterationsRead   = "iterations:read"
	ScopeIterationsWrite  = "iterations:write"
	ScopeIterationsDelete = "iterations:delete"

	// Projects
	ScopeProjectsRead   = "projects:read"
	ScopeProjectsWrite  = "projects:write"
	ScopeProjectsDelete = "projects:delete"

	// MCP — single binary scope; the MCP server exposes both read and write
	// tools so we don't split this into :read/:write.
	ScopeMCPAccess = "mcp:access"

	// Collections
	ScopeCollectionsRead = "collections:read"

	// Admin scopes (require system admin role AND scope on token)
	ScopeAdminUsersRead      = "admin:users:read"
	ScopeAdminUsersWrite     = "admin:users:write"
	ScopeAdminGroupsRead     = "admin:groups:read"
	ScopeAdminGroupsWrite    = "admin:groups:write"
	ScopeAdminAuditLogsRead  = "admin:audit-logs:read"
	ScopeAdminAPITokensRead  = "admin:api-tokens:read"
	ScopeAdminAPITokensWrite = "admin:api-tokens:write"
)

// AllValidScopes is the complete set of valid scope strings for validation.
var AllValidScopes = []string{
	ScopeItemsRead, ScopeItemsWrite, ScopeItemsDelete,
	ScopeWorkspacesRead, ScopeWorkspacesWrite, ScopeWorkspacesDelete,
	ScopeStatusesRead, ScopeWorkflowsRead, ScopeItemTypesRead,
	ScopePrioritiesRead, ScopeCustomFieldsRead,
	ScopeUsersRead,
	ScopeMilestonesRead, ScopeMilestonesWrite, ScopeMilestonesDelete,
	ScopeIterationsRead, ScopeIterationsWrite, ScopeIterationsDelete,
	ScopeProjectsRead, ScopeProjectsWrite, ScopeProjectsDelete,
	ScopeMCPAccess,
	ScopeCollectionsRead,
	ScopeAdminUsersRead, ScopeAdminUsersWrite,
	ScopeAdminGroupsRead, ScopeAdminGroupsWrite,
	ScopeAdminAuditLogsRead,
	ScopeAdminAPITokensRead, ScopeAdminAPITokensWrite,
}

// allNonAdminReadScopes is the set of non-admin :read scopes (for legacy "read" mapping).
var allNonAdminReadScopes = []string{
	ScopeItemsRead, ScopeWorkspacesRead, ScopeStatusesRead,
	ScopeWorkflowsRead, ScopeItemTypesRead, ScopePrioritiesRead,
	ScopeCustomFieldsRead, ScopeUsersRead, ScopeMilestonesRead,
	ScopeIterationsRead, ScopeProjectsRead, ScopeCollectionsRead,
}

// allNonAdminScopes is the set of all non-admin scopes (for legacy "write" mapping).
var allNonAdminScopes = []string{
	ScopeItemsRead, ScopeItemsWrite, ScopeItemsDelete,
	ScopeWorkspacesRead, ScopeWorkspacesWrite, ScopeWorkspacesDelete,
	ScopeStatusesRead, ScopeWorkflowsRead, ScopeItemTypesRead,
	ScopePrioritiesRead, ScopeCustomFieldsRead,
	ScopeUsersRead,
	ScopeMilestonesRead, ScopeMilestonesWrite, ScopeMilestonesDelete,
	ScopeIterationsRead, ScopeIterationsWrite, ScopeIterationsDelete,
	ScopeProjectsRead, ScopeProjectsWrite, ScopeProjectsDelete,
	ScopeMCPAccess,
	ScopeCollectionsRead,
}

// AdminScopes returns the set of scopes that require system admin role.
func AdminScopes() []string {
	return []string{
		ScopeAdminUsersRead, ScopeAdminUsersWrite,
		ScopeAdminGroupsRead, ScopeAdminGroupsWrite,
		ScopeAdminAuditLogsRead,
		ScopeAdminAPITokensRead, ScopeAdminAPITokensWrite,
	}
}

// IsAdminScope returns true if the scope is an admin scope.
func IsAdminScope(scope string) bool {
	return strings.HasPrefix(scope, "admin:")
}

// ValidateScopes checks that all provided scopes are valid.
// Returns an error listing any invalid scopes.
// last review: ser, 210426
func ValidateScopes(scopes []string) error {
	valid := make(map[string]bool, len(AllValidScopes))
	// Also accept legacy scopes for backward compatibility
	valid["read"] = true
	valid["write"] = true
	valid["admin"] = true
	for _, s := range AllValidScopes {
		valid[s] = true
	}
	var invalid []string
	for _, s := range scopes {
		if !valid[s] {
			invalid = append(invalid, s)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid scopes: %s", strings.Join(invalid, ", "))
	}
	return nil
}

// expandLegacyScopes maps old-style permission strings to granular scopes.
// Returns the original scopes unchanged if they are already in resource:action format.
func expandLegacyScopes(scopes []string) []string {
	for _, s := range scopes {
		switch s {
		case "admin":
			// "admin" grants everything
			return append(append([]string{}, allNonAdminScopes...), AdminScopes()...)
		case "write":
			return allNonAdminScopes
		case "read":
			return allNonAdminReadScopes
		}
	}
	// No legacy scopes found — return as-is
	return scopes
}

// tokenCacheEntry is the value stored in the token validation cache.
type tokenCacheEntry struct {
	User      models.User     `json:"user"`
	APIToken  models.APIToken `json:"api_token"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

// tokenCacheKey returns a SHA-256 hex digest of the raw token for use as a cache key.
func tokenCacheKey(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}

// TokenUsageRecorder records that a token was used so the last-used-at
// timestamp can be batched-flushed by a higher layer. The services package
// implements this via *services.TokenTracker; auth holds only the consumer
// interface so it doesn't take a layering dependency on services.
type TokenUsageRecorder interface {
	RecordTokenUse(tokenID int)
}

// TokenManager handles API token operations
type TokenManager struct {
	db           database.Database
	tokenTracker TokenUsageRecorder
	cache        *bigcache.BigCache
}

// NewTokenManager creates a new token manager
func NewTokenManager(db database.Database, tokenTracker TokenUsageRecorder) *TokenManager {
	cacheConfig := cacheutil.NewBigCacheConfig(cacheutil.BigCacheOptions{
		TTL:          30 * time.Second,
		MaxCacheMB:   32,
		MaxEntrySize: 2048,
		Shards:       256,
		CleanWindow:  10 * time.Second,
	})

	cache, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		slog.Warn("failed to create token validation cache, continuing without cache", "error", err)
	}

	return &TokenManager{
		db:           db,
		tokenTracker: tokenTracker,
		cache:        cache,
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
	if err := checkTokenFormat(token, TokenPrefix, 20); err != nil {
		return nil, nil, err
	}

	cacheKey := tokenCacheKey(token)

	// Try cache first
	if tm.cache != nil {
		if data, err := tm.cache.Get(cacheKey); err == nil {
			var entry tokenCacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				// Check expiry
				switch {
				case entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()):
					tm.cache.Delete(cacheKey) //nolint:errcheck // best-effort cache eviction
				case !entry.User.IsActive:
					tm.cache.Delete(cacheKey) //nolint:errcheck // best-effort cache eviction
				default:
					go tm.updateLastUsed(entry.APIToken.ID)
					user := entry.User
					apiToken := entry.APIToken
					return &user, &apiToken, nil
				}
			}
		}
	}

	// Cache miss — full DB + bcrypt path
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
		if verifyTokenHash(apiToken.Token, token) != nil {
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

		// Populate cache
		if tm.cache != nil {
			entry := tokenCacheEntry{
				User:      user,
				APIToken:  apiToken,
				ExpiresAt: apiToken.ExpiresAt,
			}
			if data, err := json.Marshal(entry); err == nil {
				tm.cache.Set(cacheKey, data)                                       //nolint:errcheck // best-effort cache population
				tm.cache.Set(fmt.Sprintf("tid:%d", apiToken.ID), []byte(cacheKey)) //nolint:errcheck // best-effort reverse-lookup cache
			}
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
	row := tm.db.QueryRow(`
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.id = ?
	`, id)

	token, err := scanAPITokenListRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
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
		  AND NOT t.is_temporary
		  AND (t.expires_at IS NULL OR t.expires_at > CURRENT_TIMESTAMP)
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.APIToken
	for rows.Next() {
		token, err := scanAPITokenListRow(rows)
		if err != nil {
			continue // Skip invalid rows
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

	tm.invalidateTokenCache(tokenID)
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

	// Bulk cleanup — reset entire cache since we don't know which keys were affected
	if rowsAffected > 0 && tm.cache != nil {
		tm.cache.Reset() //nolint:errcheck // best-effort bulk cache invalidation
	}

	return int(rowsAffected), nil
}

// ListAllTokens retrieves all non-temporary tokens across all users (admin endpoint).
// Supports optional filtering by user_id and pagination.
func (tm *TokenManager) ListAllTokens(userIDFilter *int, limit, offset int) ([]models.APIToken, int, error) {
	// Count total
	countQuery := `SELECT COUNT(*) FROM api_tokens t WHERE NOT t.is_temporary`
	var countArgs []interface{}
	if userIDFilter != nil {
		countQuery += " AND t.user_id = ?"
		countArgs = append(countArgs, *userIDFilter)
	}

	var total int
	if err := tm.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	// Fetch page
	query := `
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE NOT t.is_temporary`
	var args []interface{}
	if userIDFilter != nil {
		query += " AND t.user_id = ?"
		args = append(args, *userIDFilter)
	}
	query += " ORDER BY t.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query all tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.APIToken
	for rows.Next() {
		token, err := scanAPITokenListRow(rows)
		if err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	return tokens, total, nil
}

// AdminRevokeToken deletes any token by ID (admin use, no user_id check).
func (tm *TokenManager) AdminRevokeToken(tokenID int) error {
	result, err := tm.db.ExecWrite("DELETE FROM api_tokens WHERE id = ?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	tm.invalidateTokenCache(tokenID)
	return nil
}

// invalidateTokenCache removes a token from the validation cache using the reverse-lookup key.
func (tm *TokenManager) invalidateTokenCache(tokenID int) {
	if tm.cache == nil {
		return
	}
	reverseKey := fmt.Sprintf("tid:%d", tokenID)
	if cacheKeyBytes, err := tm.cache.Get(reverseKey); err == nil {
		tm.cache.Delete(string(cacheKeyBytes)) //nolint:errcheck // best-effort cache eviction
		tm.cache.Delete(reverseKey)            //nolint:errcheck // best-effort reverse-lookup eviction
	}
}

// CheckTokenPermissions checks if a token has specific permissions.
// Supports both legacy ("read", "write", "admin") and granular ("items:read") scopes.
// last review: ser, 210426
func (tm *TokenManager) CheckTokenPermissions(token *models.APIToken, requiredPermissions []string) bool {
	var rawScopes []string
	err := json.Unmarshal([]byte(token.Permissions), &rawScopes)
	if err != nil {
		return false
	}

	// Expand legacy scopes into granular ones
	expanded := expandLegacyScopes(rawScopes)

	// Build lookup set
	have := make(map[string]bool, len(expanded))
	for _, s := range expanded {
		have[s] = true
	}

	// Check each required permission
	for _, required := range requiredPermissions {
		if have[required] {
			continue
		}
		// Hierarchy: write implies read for the same resource
		// e.g. "items:write" satisfies "items:read"
		if strings.HasSuffix(required, ":read") {
			resource := strings.TrimSuffix(required, ":read")
			if have[resource+":write"] {
				continue
			}
		}
		// admin:*:write implies admin:*:read
		if strings.HasPrefix(required, "admin:") && strings.HasSuffix(required, ":read") {
			resource := strings.TrimSuffix(required, ":read")
			if have[resource+":write"] {
				continue
			}
		}
		return false
	}

	return true
}
