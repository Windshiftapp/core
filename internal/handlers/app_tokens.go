package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type AppTokenHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewAppTokenHandler(db database.Database, permissionService *services.PermissionService) *AppTokenHandler {
	return &AppTokenHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// CreateAppTokenRequest represents the request to create a new app token
type CreateAppTokenRequest struct {
	TokenName string   `json:"token_name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at"` // Optional ISO 8601 date string
}

// CreateAppTokenResponse represents the response after creating a token
type CreateAppTokenResponse struct {
	Token    string              `json:"token"` // Full token (only shown once)
	TokenID  int                 `json:"token_id"`
	AppToken models.UserAppToken `json:"app_token"`
}

// GetUserAppTokens returns all app tokens for a user
func (h *AppTokenHandler) GetUserAppTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	query := `
		SELECT id, user_id, token_name, token_prefix, scopes, expires_at, is_active, last_used_at, created_at, updated_at
		FROM user_app_tokens
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := h.db.Query(query, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var tokens []models.UserAppToken
	for rows.Next() {
		var token models.UserAppToken
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(&token.ID, &token.UserID, &token.TokenName, &token.TokenPrefix,
			&token.Scopes, &expiresAt, &token.IsActive, &lastUsedAt, &token.CreatedAt, &token.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			token.LastUsedAt = &lastUsedAt.Time
		}

		tokens = append(tokens, token)
	}

	if tokens == nil {
		tokens = []models.UserAppToken{}
	}

	respondJSONOK(w, tokens)
}

// CreateAppToken creates a new app token for a user
func (h *AppTokenHandler) CreateAppToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	currentUser := AuthorizeUserRequest(w, r, userID, h.permissionService)
	if currentUser == nil {
		return
	}

	req, expiresAt, scopesJSON, ok := decodeAndValidateTokenRequest(w, r)
	if !ok {
		return
	}

	// Verify user exists
	var userExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&userExists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !userExists {
		respondNotFound(w, r, "user")
		return
	}

	// Generate secure token
	token, tokenHash, tokenPrefix, err := generateAppToken()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Store token in database
	now := time.Now()
	var tokenID int64
	err = h.db.QueryRow(`
		INSERT INTO user_app_tokens (user_id, token_name, token_hash, token_prefix, scopes, expires_at, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?) RETURNING id
	`, userID, req.TokenName, tokenHash, tokenPrefix, scopesJSON, expiresAt, now, now).Scan(&tokenID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create response with the app token info
	appToken := models.UserAppToken{
		ID:          int(tokenID),
		UserID:      userID,
		TokenName:   req.TokenName,
		TokenPrefix: tokenPrefix,
		Scopes:      scopesJSON,
		ExpiresAt:   expiresAt,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	response := CreateAppTokenResponse{
		Token:    token,
		TokenID:  int(tokenID),
		AppToken: appToken,
	}

	// Log audit event
	// Get username for audit log
	var username string
	if err := h.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		slog.Warn("failed to look up username for audit log", slog.Any("error", err))
	}

	tokenIDInt := int(tokenID)
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAPITokenCreate,
		ResourceType: logger.ResourceAPIToken,
		ResourceID:   &tokenIDInt,
		ResourceName: req.TokenName,
		Details: map[string]interface{}{
			"token_name":      req.TokenName,
			"token_prefix":    tokenPrefix,
			"target_user_id":  userID,
			"target_username": username,
			"scopes":          req.Scopes,
			"expires_at":      expiresAt,
		},
		Success: true,
	})

	respondJSONCreated(w, response)
}

// RevokeAppToken revokes (deletes) an app token
func (h *AppTokenHandler) RevokeAppToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	currentUser := AuthorizeUserRequest(w, r, userID, h.permissionService)
	if currentUser == nil {
		return
	}

	tokenID, ok := requireIDParam(w, r, "tokenId")
	if !ok {
		return
	}

	var err error

	// Get token details for audit logging before deletion
	var tokenName, tokenPrefix, username string
	var expiresAt sql.NullTime
	err = h.db.QueryRow(`
		SELECT t.token_name, t.token_prefix, t.expires_at, u.username
		FROM user_app_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.id = ? AND t.user_id = ?
	`, tokenID, userID).Scan(&tokenName, &tokenPrefix, &expiresAt, &username)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "token")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the token
	_, err = h.db.ExecWrite(`DELETE FROM user_app_tokens WHERE id = ? AND user_id = ?`, tokenID, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAPITokenRevoke,
		ResourceType: logger.ResourceAPIToken,
		ResourceID:   &tokenID,
		ResourceName: tokenName,
		Details: map[string]interface{}{
			"token_name":      tokenName,
			"token_prefix":    tokenPrefix,
			"target_user_id":  userID,
			"target_username": username,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// UpdateAppToken updates token properties (name, scopes, expiration)
func (h *AppTokenHandler) UpdateAppToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	tokenID, ok := requireIDParam(w, r, "tokenId")
	if !ok {
		return
	}

	req, expiresAt, scopesJSON, ok := decodeAndValidateTokenRequest(w, r)
	if !ok {
		return
	}

	// Update the token
	result, err := h.db.ExecWrite(`
		UPDATE user_app_tokens
		SET token_name = ?, scopes = ?, expires_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, req.TokenName, scopesJSON, expiresAt, time.Now(), tokenID, userID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// decodeAndValidateTokenRequest decodes a CreateAppTokenRequest from the request body,
// validates the token name, parses the expiration, and marshals scopes.
// Returns the decoded request, parsed expiration, scopes JSON, and true on success.
// On failure it writes an appropriate error response and returns false.
func decodeAndValidateTokenRequest(w http.ResponseWriter, r *http.Request) (CreateAppTokenRequest, *time.Time, string, bool) {
	req, ok := decodeJSON[CreateAppTokenRequest](w, r)
	if !ok {
		return req, nil, "", false
	}

	if strings.TrimSpace(req.TokenName) == "" {
		respondValidationError(w, r, "Token name is required")
		return req, nil, "", false
	}

	expiresAt, err := parseTokenExpiration(req.ExpiresAt)
	if err != nil {
		respondValidationError(w, r, "Invalid expiration date format")
		return req, nil, "", false
	}

	scopesJSON, err := marshalScopes(req.Scopes)
	if err != nil {
		respondValidationError(w, r, "Invalid scopes format")
		return req, nil, "", false
	}

	return req, expiresAt, scopesJSON, true
}

// parseTokenExpiration parses an optional RFC3339 expiration string into a *time.Time.
func parseTokenExpiration(expiresAt *string) (*time.Time, error) {
	if expiresAt == nil || *expiresAt == "" {
		return nil, nil
	}
	parsedTime, err := time.Parse(time.RFC3339, *expiresAt)
	if err != nil {
		return nil, err
	}
	return &parsedTime, nil
}

// marshalScopes converts a scopes slice to a JSON string, defaulting to "[]".
func marshalScopes(scopes []string) (string, error) {
	if len(scopes) == 0 {
		return "[]", nil
	}
	scopesBytes, err := json.Marshal(scopes)
	if err != nil {
		return "", err
	}
	return string(scopesBytes), nil
}

// generateAppToken creates a secure random token with hash and prefix
func generateAppToken() (token, hash, prefix string, err error) {
	// Generate 32 random bytes for the token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", "", err
	}

	// Create token with prefix (vx_ for legacy compatibility)
	token = "vx_" + base64.URLEncoding.EncodeToString(tokenBytes)

	// Create SHA256 hash of the token for storage
	hashBytes := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(hashBytes[:])

	// Create prefix for display (first 8 characters after vx_)
	if len(token) > 10 {
		prefix = token[:10] + "..."
	} else {
		prefix = token
	}

	return token, hash, prefix, nil
}
