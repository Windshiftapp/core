package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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

var ErrTokenNotFound = errors.New("token not found")

// Scope constants, the scope catalog, and scope validation live in scopes.go.

// tokenCacheEntry is the value stored in the token validation cache.
type tokenCacheEntry struct {
	User          models.User     `json:"user"`
	APIToken      models.APIToken `json:"api_token"`
	OAuthClientID string          `json:"oauth_client_id,omitempty"`
	OAuthResource string          `json:"oauth_resource,omitempty"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
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
	closeOnce    sync.Once
	closeErr     error
}

// Close stops the validation cache's background worker. It does not close the
// shared database or token tracker, and may be called more than once.
func (tm *TokenManager) Close() error {
	tm.closeOnce.Do(func() {
		if tm.cache != nil {
			tm.closeErr = tm.cache.Close()
		}
	})
	return tm.closeErr
}

// NewTokenManager creates a new token manager
func NewTokenManager(db database.Database, tokenTracker TokenUsageRecorder, cacheSizeMB ...int) *TokenManager {
	maxCacheMB := 8
	if len(cacheSizeMB) > 0 && cacheSizeMB[0] > 0 {
		maxCacheMB = cacheSizeMB[0]
	}
	return newTokenManager(db, tokenTracker, "api_tokens", maxCacheMB)
}

// NewTokenManagerWithCacheBudget creates a token manager with an explicitly
// named cache. The SSH server uses this so its share of the aggregate token
// budget remains independently visible in diagnostics.
func NewTokenManagerWithCacheBudget(db database.Database, tokenTracker TokenUsageRecorder, cacheName string, maxCacheMB int) *TokenManager {
	return newTokenManager(db, tokenTracker, cacheName, maxCacheMB)
}

func newTokenManager(db database.Database, tokenTracker TokenUsageRecorder, cacheName string, maxCacheMB int) *TokenManager {
	cache, err := cacheutil.New(cacheName, cacheutil.BigCacheOptions{
		TTL:               30 * time.Second,
		MaxCacheMB:        maxCacheMB,
		MaxEntrySize:      2048,
		Shards:            16,
		InitialCapacityMB: 1,
		CleanWindow:       10 * time.Second,
	})
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
	// Generate a random body and prefix it for indexed lookup.
	tokenBytes := make([]byte, TokenLength-len(TokenPrefix))
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

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
	if err := checkTokenFormat(token, TokenPrefix, 20); err != nil {
		return nil, nil, err
	}

	cacheKey := tokenCacheKey(token)

	// Cached entries avoid the database and bcrypt work; misses use the indexed prefix query below.
	if tm.cache != nil {
		if data, err := tm.cache.Get(cacheKey); err == nil {
			var entry tokenCacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				switch {
				case entry.ExpiresAt != nil && !time.Now().Before(*entry.ExpiresAt):
					tm.cache.Delete(cacheKey) //nolint:errcheck // best-effort cache eviction
				case !entry.User.IsActive:
					tm.cache.Delete(cacheKey) //nolint:errcheck // best-effort cache eviction
				default:
					go tm.updateLastUsed(entry.APIToken.ID)
					user := entry.User
					apiToken := entry.APIToken
					apiToken.OAuthClientID = entry.OAuthClientID
					apiToken.OAuthResource = entry.OAuthResource
					return &user, &apiToken, nil
				}
			}
		}
	}

	tokenPrefix := tm.GetTokenPrefix(token)

	// Prefix filtering limits bcrypt comparisons; CURRENT_TIMESTAMP is portable across both databases.
	rows, err := tm.db.Query(`
		SELECT t.id, t.user_id, t.name, t.token_hash, t.token_prefix, t.permissions,
		       t.is_temporary, t.oauth_client_id, t.oauth_resource,
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
		var oauthClientID, oauthResource sql.NullString

		err := rows.Scan(
			&apiToken.ID, &apiToken.UserID, &apiToken.Name, &apiToken.Token,
			&apiToken.TokenPrefix, &apiToken.Permissions, &apiToken.IsTemporary,
			&oauthClientID, &oauthResource,
			&expiresAt, &lastUsedAt, &apiToken.CreatedAt, &apiToken.UpdatedAt,
			&user.ID, &user.Email, &user.Username, &user.FirstName,
			&user.LastName, &user.IsActive,
		)
		if err != nil {
			// A scan failure usually means a driver/schema issue rather than a
			// malformed row — silently dropping it would surface as "invalid
			// token" and mask the real cause. Log and continue so iteration
			// still has a chance to find the matching token via the prefix
			// scan; rows.Err() below catches iteration-level failures.
			slog.Warn("token row scan failed", slog.String("component", "auth"), slog.String("token_prefix", tokenPrefix), slog.Any("error", err))
			continue
		}

		if verifyTokenHash(apiToken.Token, token) != nil {
			continue // Hash doesn't match, try next token
		}

		if expiresAt.Valid {
			apiToken.ExpiresAt = &expiresAt.Time
		}
		apiToken.OAuthClientID = oauthClientID.String
		apiToken.OAuthResource = oauthResource.String
		if lastUsedAt.Valid {
			apiToken.LastUsedAt = &lastUsedAt.Time
		}

		if !user.IsActive {
			return nil, nil, fmt.Errorf("user account is disabled")
		}

		if tm.cache != nil {
			entry := tokenCacheEntry{
				User:          user,
				APIToken:      apiToken,
				OAuthClientID: apiToken.OAuthClientID,
				OAuthResource: apiToken.OAuthResource,
				ExpiresAt:     apiToken.ExpiresAt,
			}
			if data, err := json.Marshal(entry); err == nil {
				tm.cache.Set(cacheKey, data)                                       //nolint:errcheck // best-effort cache population
				tm.cache.Set(fmt.Sprintf("tid:%d", apiToken.ID), []byte(cacheKey)) //nolint:errcheck // best-effort reverse-lookup cache
			}
		}

		go tm.updateLastUsed(apiToken.ID)

		return &user, &apiToken, nil
	}

	if err := rows.Err(); err != nil {
		// Iteration failed mid-stream (e.g. driver/connection error). Surface
		// it as an error instead of letting the caller see "invalid token",
		// which would obscure real outages.
		return nil, nil, fmt.Errorf("iterate token rows: %w", err)
	}

	return nil, nil, fmt.Errorf("invalid token")
}

// CreateToken creates a new API token for a user
func (tm *TokenManager) CreateToken(userID int, request models.APITokenCreate) (*models.APITokenResponse, error) {
	return tm.createToken(tm.db, userID, request)
}

// CreateTokenInTx creates an API token inside an existing transaction. It is
// used when issuing the token is only one part of a larger credential update
// that must either commit in full or leave no usable credentials behind.
func (tm *TokenManager) CreateTokenInTx(tx database.Tx, userID int, request models.APITokenCreate) (*models.APITokenResponse, error) {
	return tm.createToken(tx, userID, request)
}

type tokenRowStore interface {
	QueryRow(query string, args ...any) *sql.Row
}

func (tm *TokenManager) createToken(store tokenRowStore, userID int, request models.APITokenCreate) (*models.APITokenResponse, error) {
	var userActive bool
	if err := store.QueryRow(`SELECT COALESCE(is_active, false) FROM users WHERE id = ?`, userID).Scan(&userActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to load token user: %w", err)
	}
	if !userActive {
		return nil, fmt.Errorf("cannot create token for inactive user")
	}

	// Generate and hash the token; only the response below exposes its plaintext.
	token, err := tm.GenerateToken()
	if err != nil {
		return nil, err
	}

	tokenHash, err := tm.HashToken(token)
	if err != nil {
		return nil, err
	}

	tokenPrefix := tm.GetTokenPrefix(token)

	permissionsJSON, err := json.Marshal(request.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	// RETURNING is supported by both SQLite 3.35+ and PostgreSQL.
	var tokenID int64
	err = store.QueryRow(`
		INSERT INTO api_tokens (
			user_id, name, token_hash, token_prefix, permissions, expires_at,
			is_temporary, oauth_client_id, oauth_resource
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, userID, request.Name, tokenHash, tokenPrefix, string(permissionsJSON), request.ExpiresAt,
		request.IsTemporary, nullIfEmpty(request.OAuthClientID), nullIfEmpty(request.OAuthResource)).Scan(&tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	apiToken, err := getTokenByID(store, int(tokenID))
	if err != nil {
		return nil, err
	}
	apiToken.OAuthClientID = request.OAuthClientID
	apiToken.OAuthResource = request.OAuthResource

	return &models.APITokenResponse{
		Token:    token, // Only returned on creation
		APIToken: *apiToken,
	}, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// GetTokenByID retrieves a token by ID (without the actual token value)
func (tm *TokenManager) GetTokenByID(id int) (*models.APIToken, error) {
	return getTokenByID(tm.db, id)
}

func getTokenByID(store tokenRowStore, id int) (*models.APIToken, error) {
	row := store.QueryRow(`
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.id = ?
	`, id)

	token, err := scanAPITokenListRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
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
			slog.Warn("user-token row scan failed", slog.String("component", "auth"), slog.Int("user_id", userID), slog.Any("error", err))
			continue
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user tokens: %w", err)
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
		return fmt.Errorf("%w or not owned by user", ErrTokenNotFound)
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
	countQuery := `SELECT COUNT(*) FROM api_tokens t WHERE NOT t.is_temporary`
	var countArgs []any
	if userIDFilter != nil {
		countQuery += " AND t.user_id = ?"
		countArgs = append(countArgs, *userIDFilter)
	}

	var total int
	if err := tm.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	query := `
		SELECT t.id, t.user_id, t.name, t.token_prefix, t.permissions, t.is_temporary,
		       t.expires_at, t.last_used_at, t.created_at, t.updated_at,
		       u.email, u.username
		FROM api_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE NOT t.is_temporary`
	var args []any
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
			slog.Warn("admin-token-list row scan failed", slog.String("component", "auth"), slog.Any("error", err))
			continue
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate token list: %w", err)
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
		return ErrTokenNotFound
	}

	tm.invalidateTokenCache(tokenID)
	return nil
}

// InvalidateTokens evicts validation cache entries for the given token IDs.
// Use after bulk DB-level revocation (user offboard, deactivation cascade)
// where AdminRevokeToken's per-token path was bypassed; otherwise tokens
// already in the validation cache stay accepted until the 30s TTL expires.
func (tm *TokenManager) InvalidateTokens(tokenIDs []int) {
	if tm.cache == nil {
		return
	}
	for _, id := range tokenIDs {
		tm.invalidateTokenCache(id)
	}
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
// Scopes are granular ("items:read") only. The legacy "read"/"write"/"admin"
// strings were rewritten to their granular equivalents by the
// 20260808_api_token_legacy_scopes migration and are no longer expanded here,
// so a token still carrying one grants nothing.
// last review: ser, 210426
func (tm *TokenManager) CheckTokenPermissions(token *models.APIToken, requiredPermissions []string) bool {
	var scopes []string
	err := json.Unmarshal([]byte(token.Permissions), &scopes)
	if err != nil {
		return false
	}

	return ScopesSatisfy(scopes, requiredPermissions)
}
