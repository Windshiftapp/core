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
//
// @Summary      List all API tokens (admin)
// @Description  System-admin only. Optionally filter to a single user via `user_id`.
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        user_id  query     int  false  "Filter to tokens owned by this user"
// @Param        page     query     int  false  "Page number (1-based)"
// @Param        limit    query     int  false  "Items per page (max 100)"
// @Success      200      {object}  handlers.PaginatedResponse{data=[]models.APIToken}
// @Failure      400      {object}  handlers.ErrorResponse  "Invalid user_id"
// @Failure      401      {object}  handlers.ErrorResponse
// @Failure      403      {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:api-tokens:read scope"
// @Failure      500      {object}  handlers.ErrorResponse
// @Router       /admin/api-tokens [get]
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
//
// @Summary      Revoke an API token (admin)
// @Description  System-admin only.
// @Tags         admin
// @Security     BearerAuth
// @Param        id   path  int  true  "Token ID"
// @Success      204  "Token revoked"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid token ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:api-tokens:write scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Token not found"
// @Router       /admin/api-tokens/{id} [delete]
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
