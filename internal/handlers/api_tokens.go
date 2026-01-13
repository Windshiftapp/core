package handlers

import (
	"windshift/internal/database"
	"windshift/internal/services"
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/auth"
	"windshift/internal/models"
)

// ApiTokenHandler handles API token management
type ApiTokenHandler struct {
	db                database.Database
	tokenManager      *auth.TokenManager
	permissionService *services.PermissionService
}

// NewApiTokenHandler creates a new API token handler
func NewApiTokenHandler(db database.Database, tokenManager *auth.TokenManager, permissionService *services.PermissionService) *ApiTokenHandler {
	return &ApiTokenHandler{
		db:                db,
		tokenManager:      tokenManager,
		permissionService: permissionService,
	}
}

// CreateToken creates a new API token for a user
func (ath *ApiTokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value("user").(*models.User)
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusUnauthorized)
		return
	}

	var request models.ApiTokenCreate
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.Name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
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
			http.Error(w, "Only system administrators can create tokens for other users", http.StatusForbidden)
			return
		}

		// Verify target user exists
		var userExists bool
		err = ath.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", *request.UserID).Scan(&userExists)
		if err != nil {
			http.Error(w, "Failed to verify target user: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !userExists {
			http.Error(w, "Target user not found", http.StatusNotFound)
			return
		}

		targetUserID = *request.UserID
	}

	// Create token
	tokenResponse, err := ath.tokenManager.CreateToken(targetUserID, request)
	if err != nil {
		http.Error(w, "Failed to create token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse)
}

// GetUserTokens retrieves all tokens for the current user
func (ath *ApiTokenHandler) GetUserTokens(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value("user").(*models.User)
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusUnauthorized)
		return
	}

	tokens, err := ath.tokenManager.GetUserTokens(user.ID)
	if err != nil {
		http.Error(w, "Failed to get tokens: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

// GetToken retrieves a specific token by ID (for current user)
func (ath *ApiTokenHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	tokenIDStr := r.PathValue("id")
	
	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		http.Error(w, "Invalid token ID", http.StatusBadRequest)
		return
	}

	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value("user").(*models.User)
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusUnauthorized)
		return
	}

	token, err := ath.tokenManager.GetTokenByID(tokenID)
	if err != nil {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	// Verify token belongs to current user
	if token.UserID != user.ID {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(token)
}

// RevokeToken deletes/revokes a token
func (ath *ApiTokenHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tokenIDStr := r.PathValue("id")
	
	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		http.Error(w, "Invalid token ID", http.StatusBadRequest)
		return
	}

	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value("user").(*models.User)
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusUnauthorized)
		return
	}

	err = ath.tokenManager.RevokeToken(tokenID, user.ID)
	if err != nil {
		if err.Error() == "token not found or not owned by user" {
			http.Error(w, "Token not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to revoke token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ValidateToken endpoint for testing token validity
func (ath *ApiTokenHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	// This endpoint is useful for API clients to test their tokens
	// If they can call this successfully, their token is valid
	
	// Get user and token info from context (set by auth middleware)
	user, ok := r.Context().Value("user").(*models.User)
	if !ok || user == nil {
		http.Error(w, "Token validation failed", http.StatusUnauthorized)
		return
	}

	apiToken, ok := r.Context().Value("api_token").(*models.ApiToken)
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
	json.NewEncoder(w).Encode(response)
}

// CleanupExpiredTokens removes expired tokens (admin endpoint)
func (ath *ApiTokenHandler) CleanupExpiredTokens(w http.ResponseWriter, r *http.Request) {
	// This should be protected by admin middleware
	count, err := ath.tokenManager.CleanupExpiredTokens()
	if err != nil {
		http.Error(w, "Failed to cleanup expired tokens: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"cleaned_count": count,
		"message":       "Successfully cleaned up expired tokens",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}