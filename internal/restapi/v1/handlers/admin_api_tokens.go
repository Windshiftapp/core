package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/restapi"
)

// AdminAPITokenHandler handles admin API token management in REST API v1.
type AdminAPITokenHandler struct {
	tokenManager *auth.TokenManager
}

// NewAdminAPITokenHandler creates a new admin API token handler.
func NewAdminAPITokenHandler(tokenManager *auth.TokenManager) *AdminAPITokenHandler {
	return &AdminAPITokenHandler{tokenManager: tokenManager}
}

// ListAll handles GET /rest/api/v1/admin/api-tokens
func (h *AdminAPITokenHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	var userIDFilter *int
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		id, err := strconv.Atoi(uid)
		if err != nil {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid user_id"))
			return
		}
		userIDFilter = &id
	}

	tokens, total, err := h.tokenManager.ListAllTokens(userIDFilter, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	if tokens == nil {
		tokens = []models.APIToken{}
	}

	restapi.RespondPaginated(w, tokens, restapi.NewPaginationMeta(pagination, total))
}

// Revoke handles DELETE /rest/api/v1/admin/api-tokens/{id}
func (h *AdminAPITokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, ok := parsePathID(w, r, "id", "token ID")
	if !ok {
		return
	}

	if err := h.tokenManager.AdminRevokeToken(id); err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondNoContent(w)
}
