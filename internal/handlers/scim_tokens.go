package handlers

import (
	"encoding/json"
	"net/http"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// SCIMTokenHandler handles SCIM token management endpoints
type SCIMTokenHandler struct {
	tokenManager *auth.SCIMTokenManager
}

// NewSCIMTokenHandler creates a new SCIM token handler
func NewSCIMTokenHandler(tokenManager *auth.SCIMTokenManager) *SCIMTokenHandler {
	return &SCIMTokenHandler{
		tokenManager: tokenManager,
	}
}

// ListTokens returns all SCIM tokens (GET /api/scim-tokens)
func (h *SCIMTokenHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.tokenManager.ListTokens()
	if err != nil {
		http.Error(w, "Failed to list SCIM tokens: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, tokens)
}

// CreateToken creates a new SCIM token (POST /api/scim-tokens)
func (h *SCIMTokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var request models.SCIMTokenCreate
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	response, err := h.tokenManager.CreateToken(currentUser.ID, request)
	if err != nil {
		http.Error(w, "Failed to create SCIM token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONCreated(w, response)
}

// GetToken returns a single SCIM token by ID (GET /api/scim-tokens/{id})
func (h *SCIMTokenHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	token, err := h.tokenManager.GetTokenByID(id)
	if err != nil {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	respondJSONOK(w, token)
}

// RevokeToken revokes a SCIM token (DELETE /api/scim-tokens/{id})
func (h *SCIMTokenHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.tokenManager.RevokeToken(id)
	if err != nil {
		if err.Error() == "token not found" {
			http.Error(w, "Token not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to revoke token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetActiveTokenCount returns the count of active SCIM tokens (GET /api/scim-tokens/count)
func (h *SCIMTokenHandler) GetActiveTokenCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.tokenManager.GetActiveTokenCount()
	if err != nil {
		http.Error(w, "Failed to count tokens: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, map[string]int{"count": count})
}
