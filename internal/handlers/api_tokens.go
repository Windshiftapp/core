package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// APITokenHandler handles API token management
type APITokenHandler struct {
	db                database.Database
	tokenManager      *auth.TokenManager
	permissionService *services.PermissionService
}

// NewAPITokenHandler creates a new API token handler
func NewAPITokenHandler(db database.Database, tokenManager *auth.TokenManager, permissionService *services.PermissionService) *APITokenHandler {
	return &APITokenHandler{
		db:                db,
		tokenManager:      tokenManager,
		permissionService: permissionService,
	}
}

// CreateToken creates a new API token for a user
func (ath *APITokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	request, ok := decodeJSON[models.APITokenCreate](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if request.Name == "" {
		respondValidationError(w, r, "Token name is required")
		return
	}

	// Set default permissions if none provided
	if len(request.Permissions) == 0 {
		request.Permissions = []string{auth.ScopeItemsRead, auth.ScopeWorkspacesRead, auth.ScopeUsersRead}
	}

	// Validate all scopes
	if err := auth.ValidateScopes(request.Permissions); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Reject admin scopes for non-admin users
	for _, scope := range request.Permissions {
		if auth.IsAdminScope(scope) {
			isAdmin, err := ath.permissionService.IsSystemAdmin(user.ID)
			if err != nil || !isAdmin {
				respondForbidden(w, r)
				return
			}
			break
		}
	}

	// Enforce API key creation policy
	if policyErr := ath.checkCreationPolicy(user.ID); policyErr != nil {
		respondForbidden(w, r)
		return
	}

	// Determine which user ID to use for token creation
	targetUserID := user.ID
	if request.UserID != nil && *request.UserID != user.ID {
		// Admin wants to create token for another user - verify admin status
		isSystemAdmin, err := ath.permissionService.IsSystemAdmin(user.ID)
		if err != nil || !isSystemAdmin {
			respondForbidden(w, r)
			return
		}

		// Verify target user exists
		var userExists bool
		err = ath.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", *request.UserID).Scan(&userExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !userExists {
			respondNotFound(w, r, "user")
			return
		}

		targetUserID = *request.UserID
	}

	// Create token
	tokenResponse, err := ath.tokenManager.CreateToken(targetUserID, request)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	details := map[string]interface{}{
		"token_prefix": tokenResponse.APIToken.TokenPrefix,
	}
	if targetUserID != user.ID {
		details["target_user_id"] = targetUserID
	}
	_ = logger.LogAudit(ath.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAPITokenCreate,
		ResourceType: logger.ResourceAPIToken,
		ResourceID:   &tokenResponse.APIToken.ID,
		ResourceName: tokenResponse.APIToken.Name,
		Details:      details,
		Success:      true,
	})

	respondJSONOK(w, tokenResponse)
}

// GetUserTokens retrieves all tokens for the current user
func (ath *APITokenHandler) GetUserTokens(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	tokens, err := ath.tokenManager.GetUserTokens(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, tokens)
}

// GetToken retrieves a specific token by ID (for current user)
func (ath *APITokenHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	tokenIDStr := r.PathValue("id")

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		respondInvalidID(w, r, "token ID")
		return
	}

	// Get user from context (set by auth middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	token, err := ath.tokenManager.GetTokenByID(tokenID)
	if err != nil {
		respondNotFound(w, r, "token")
		return
	}

	// Verify token belongs to current user
	if token.UserID != user.ID {
		respondForbidden(w, r)
		return
	}

	respondJSONOK(w, token)
}

// RevokeToken deletes/revokes a token
func (ath *APITokenHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tokenIDStr := r.PathValue("id")

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		respondInvalidID(w, r, "token ID")
		return
	}

	// Get user from context (set by auth middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	token, err := ath.tokenManager.GetTokenByID(tokenID)
	if err != nil {
		respondNotFound(w, r, "token")
		return
	}
	if token.UserID != user.ID {
		respondNotFound(w, r, "token")
		return
	}

	err = ath.tokenManager.RevokeToken(tokenID, user.ID)
	if err != nil {
		if err.Error() == "token not found or not owned by user" {
			respondNotFound(w, r, "token")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	logAudit(ath.db, r, user, logger.ActionAPITokenRevoke, logger.ResourceAPIToken, &tokenID, token.Name)

	w.WriteHeader(http.StatusNoContent)
}

// ValidateToken endpoint for testing token validity
func (ath *APITokenHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	// This endpoint is useful for API clients to test their tokens
	// If they can call this successfully, their token is valid

	// Get user and token info from context (set by auth middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	apiToken, _ := r.Context().Value("api_token").(*models.APIToken)
	authMethod, _ := r.Context().Value("auth_method").(string)

	response := map[string]interface{}{
		"valid":       true,
		"user_id":     user.ID,
		"username":    user.Username,
		"auth_method": authMethod,
	}

	if apiToken != nil {
		response["token_id"] = apiToken.ID
		response["token_name"] = apiToken.Name
		response["permissions"] = apiToken.Permissions
		if apiToken.ExpiresAt != nil {
			response["expires_at"] = apiToken.ExpiresAt
		}
	}

	respondJSONOK(w, response)
}

// CleanupExpiredTokens removes expired tokens (admin endpoint)
func (ath *APITokenHandler) CleanupExpiredTokens(w http.ResponseWriter, r *http.Request) {
	// This should be protected by admin middleware
	count, err := ath.tokenManager.CleanupExpiredTokens()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"cleaned_count": count,
		"message":       "Successfully cleaned up expired tokens",
	}

	respondJSONOK(w, response)
}

// ListAllTokens lists all non-temporary API tokens across users (admin endpoint).
// Supports ?user_id= filter and ?page=&per_page= pagination.
func (ath *APITokenHandler) ListAllTokens(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	var userIDFilter *int
	if uid := query.Get("user_id"); uid != "" {
		id, err := strconv.Atoi(uid)
		if err != nil {
			respondBadRequest(w, r, "Invalid user_id")
			return
		}
		userIDFilter = &id
	}

	page := 1
	perPage := 50
	if p := query.Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := query.Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}

	offset := (page - 1) * perPage
	tokens, total, err := ath.tokenManager.ListAllTokens(userIDFilter, perPage, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	totalPages := total / perPage
	if total%perPage > 0 {
		totalPages++
	}

	respondJSONOK(w, map[string]interface{}{
		"tokens":      tokens,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// AdminRevokeToken revokes any token by ID (admin endpoint).
func (ath *APITokenHandler) AdminRevokeToken(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "token ID")
		return
	}

	// Get the token first for audit logging
	token, err := ath.tokenManager.GetTokenByID(tokenID)
	if err != nil {
		respondNotFound(w, r, "token")
		return
	}

	if err := ath.tokenManager.AdminRevokeToken(tokenID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "token")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Audit log
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		tid := tokenID
		_ = logger.LogAudit(ath.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "api_token.admin_revoke",
			ResourceType: "api_token",
			ResourceID:   &tid,
			ResourceName: token.Name,
			Details: map[string]interface{}{
				"revoked_user_id": token.UserID,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAPIKeyPolicy returns whether the current user can create API keys.
func (ath *APITokenHandler) GetAPIKeyPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	policy := ath.getCreationPolicy()
	canCreate := true

	switch policy {
	case "disabled":
		// System admins can always create
		isAdmin, err := ath.permissionService.IsSystemAdmin(user.ID)
		if err != nil || !isAdmin {
			canCreate = false
		}
	case "groups_only":
		isAdmin, _ := ath.permissionService.IsSystemAdmin(user.ID)
		if !isAdmin {
			canCreate = ath.userInAllowedGroups(user.ID)
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"can_create": canCreate,
		"policy":     policy,
	})
}

// checkCreationPolicy enforces the API key creation policy for a user.
// Returns nil if the user is allowed to create tokens, or an error if not.
func (ath *APITokenHandler) checkCreationPolicy(userID int) error {
	// System admins always bypass
	isAdmin, _ := ath.permissionService.IsSystemAdmin(userID)
	if isAdmin {
		return nil
	}

	policy := ath.getCreationPolicy()

	switch policy {
	case "disabled":
		return fmt.Errorf("API key creation is disabled")
	case "groups_only":
		if !ath.userInAllowedGroups(userID) {
			return fmt.Errorf("API key creation is restricted to specific groups")
		}
	}

	return nil
}

// getCreationPolicy reads the api_key_creation_policy setting.
func (ath *APITokenHandler) getCreationPolicy() string {
	var value string
	err := ath.db.QueryRow("SELECT value FROM system_settings WHERE key = 'api_key_creation_policy'").Scan(&value)
	if err != nil {
		return "all_users" // default
	}
	return value
}

// userInAllowedGroups checks if the user is a member of any group in the allowed list.
func (ath *APITokenHandler) userInAllowedGroups(userID int) bool {
	var groupIDsJSON string
	err := ath.db.QueryRow("SELECT value FROM system_settings WHERE key = 'api_key_allowed_group_ids'").Scan(&groupIDsJSON)
	if err != nil {
		return false
	}

	var groupIDs []int
	if err := json.Unmarshal([]byte(groupIDsJSON), &groupIDs); err != nil || len(groupIDs) == 0 {
		return false
	}

	// Build placeholders
	placeholders := make([]string, len(groupIDs))
	args := make([]interface{}, 0, len(groupIDs)+1)
	args = append(args, userID)
	for i, gid := range groupIDs {
		placeholders[i] = "?"
		args = append(args, gid)
	}

	var count int
	query := "SELECT COUNT(*) FROM group_members WHERE user_id = ? AND group_id IN (" + strings.Join(placeholders, ",") + ")"
	if err := ath.db.QueryRow(query, args...).Scan(&count); err != nil {
		return false
	}

	return count > 0
}
