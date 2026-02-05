package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
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
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
		return
	}

	var request models.APITokenCreate
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate required fields
	if request.Name == "" {
		respondValidationError(w, r, "Token name is required")
		return
	}

	// Set default permissions if none provided
	if len(request.Permissions) == 0 {
		request.Permissions = []string{"read"}
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokenResponse)
}

// GetUserTokens retrieves all tokens for the current user
func (ath *APITokenHandler) GetUserTokens(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
		return
	}

	tokens, err := ath.tokenManager.GetUserTokens(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokens)
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
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(token)
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
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	w.WriteHeader(http.StatusNoContent)
}

// ValidateToken endpoint for testing token validity
func (ath *APITokenHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	// This endpoint is useful for API clients to test their tokens
	// If they can call this successfully, their token is valid

	// Get user and token info from context (set by auth middleware)
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
