package handlers

import (
	"encoding/json"
	"log/slog"
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
		slog.Error("Failed to list SCIM tokens",
			slog.String("component", "scim"),
			slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, tokens)
}

// CreateToken creates a new SCIM token (POST /api/scim-tokens)
func (h *SCIMTokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	var request models.SCIMTokenCreate
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if request.Name == "" {
		respondValidationError(w, r, "Token name is required")
		return
	}

	response, err := h.tokenManager.CreateToken(currentUser.ID, request)
	if err != nil {
		slog.Error("Failed to create SCIM token",
			slog.String("component", "scim"),
			slog.Int("created_by", currentUser.ID),
			slog.String("token_name", request.Name),
			slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Info("SCIM token created",
		slog.String("component", "scim"),
		slog.Int("created_by", currentUser.ID),
		slog.String("token_name", request.Name),
		slog.String("token_prefix", response.SCIMToken.TokenPrefix))

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
		respondNotFound(w, r, "token")
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

	currentUser := utils.GetCurrentUser(r)
	userID := 0
	if currentUser != nil {
		userID = currentUser.ID
	}

	err := h.tokenManager.RevokeToken(id)
	if err != nil {
		if err.Error() == "token not found" {
			respondNotFound(w, r, "token")
			return
		}
		slog.Error("Failed to revoke SCIM token",
			slog.String("component", "scim"),
			slog.Int("token_id", id),
			slog.Int("revoked_by", userID),
			slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Info("SCIM token revoked",
		slog.String("component", "scim"),
		slog.Int("token_id", id),
		slog.Int("revoked_by", userID))

	w.WriteHeader(http.StatusNoContent)
}

// GetActiveTokenCount returns the count of active SCIM tokens (GET /api/scim-tokens/count)
func (h *SCIMTokenHandler) GetActiveTokenCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.tokenManager.GetActiveTokenCount()
	if err != nil {
		slog.Error("Failed to count SCIM tokens",
			slog.String("component", "scim"),
			slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]int{"count": count})
}
