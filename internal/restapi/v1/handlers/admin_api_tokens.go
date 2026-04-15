package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AdminAPITokenHandler handles admin API token management in REST API v1.
type AdminAPITokenHandler struct {
	BaseHandler
	tokenManager *auth.TokenManager
}

// NewAdminAPITokenHandler creates a new admin API token handler.
func NewAdminAPITokenHandler(db database.Database, tokenManager *auth.TokenManager, permissionService *services.PermissionService) *AdminAPITokenHandler {
	return &AdminAPITokenHandler{
		BaseHandler:  NewBaseHandler(db, permissionService),
		tokenManager: tokenManager,
	}
}

// ListAll handles GET /rest/api/v1/admin/api-tokens
func (h *AdminAPITokenHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	var userIDFilter *int
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		id, err := strconv.Atoi(uid)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid user_id"))
			return
		}
		userIDFilter = &id
	}

	tokens, total, err := h.tokenManager.ListAllTokens(userIDFilter, pagination.Limit, pagination.Offset)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	if tokens == nil {
		tokens = []models.APIToken{}
	}

	h.RespondPaginated(w, tokens, pagination, total)
}

// Revoke handles DELETE /rest/api/v1/admin/api-tokens/{id}
func (h *AdminAPITokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "token ID")
	if !ok {
		return
	}

	if err := h.tokenManager.AdminRevokeToken(id); err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondNoContent(w)
}
