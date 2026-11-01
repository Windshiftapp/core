package handlers

import (
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// Default and maximum batch sizes for the cursor-based audit log stream.
const (
	auditLogStreamDefaultLimit = 500
	auditLogStreamMaxLimit     = 1000
)

// AdminAuditLogHandler handles audit log access in REST API v1.
type AdminAuditLogHandler struct {
	BaseHandler
}

// NewAdminAuditLogHandler creates a new admin audit log handler.
func NewAdminAuditLogHandler(db database.Database, permissionService *services.PermissionService) *AdminAuditLogHandler {
	return &AdminAuditLogHandler{BaseHandler: NewBaseHandler(db, permissionService)}
}

// AuditLogEntryResponse is the REST API v1 representation of an audit log entry.
type AuditLogEntryResponse struct {
	ID           int            `json:"id"`
	Timestamp    string         `json:"timestamp"`
	UserID       *int           `json:"user_id"`
	Username     string         `json:"username"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	ActionType   string         `json:"action_type"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *int           `json:"resource_id,omitempty"`
	ResourceName string         `json:"resource_name,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	Success      bool           `json:"success"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Source       string         `json:"source,omitempty"`
}

// List handles GET /rest/api/v1/admin/audit-logs
//
// @Summary      List audit log entries (admin)
// @Description  System-admin only. Filter by action type, resource type, user, and timestamp range.
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        action_type    query     string  false  "Exact-match filter on action type"
// @Param        resource_type  query     string  false  "Exact-match filter on resource type"
// @Param        user_id        query     int     false  "Filter to actions performed by this user"
// @Param        from           query     string  false  "Start of timestamp range (RFC3339)"
// @Param        to             query     string  false  "End of timestamp range (RFC3339)"
// @Param        page           query     int     false  "Page number (1-based)"
// @Param        limit          query     int     false  "Items per page (max 100)"
// @Success      200            {object}  handlers.PaginatedResponse{data=[]handlers.AuditLogEntryResponse}
// @Failure      400            {object}  handlers.ErrorResponse  "Invalid filter parameter"
// @Failure      401            {object}  handlers.ErrorResponse
// @Failure      403            {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:audit-logs:read scope"
// @Failure      500            {object}  handlers.ErrorResponse
// @Router       /admin/audit-logs [get]
func (h *AdminAuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
	q := r.URL.Query()

	filters, err := repository.ParseAuditLogFilters(q)
	if err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error()))
		return
	}

	repo := repository.NewAuditLogRepository(h.DB)
	rows, total, err := repo.List(filters, pagination.Page, pagination.Limit)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	entries := mapAuditLogRowsToResponses(rows)

	if user := middleware.GetUser(r.Context()); user != nil {
		details := filters.Details()
		details["count"] = len(entries)
		details["total"] = total
		details["page"] = pagination.Page
		h.Auditor.LogWithDetails(r, user, logger.ActionAuditLogRead, logger.ResourceAuditLog, nil, "audit_log", details)
	}

	h.RespondPaginated(w, entries, pagination, total)
}

// AuditLogStreamResponse is the cursor-based response shape returned by
// /admin/audit-logs/since. Entries are ordered by id ascending; callers
// persist next_after_id and pass it back as after_id on the next call.
type AuditLogStreamResponse struct {
	Entries     []AuditLogEntryResponse `json:"entries"`
	NextAfterID int                     `json:"next_after_id"`
	HasMore     bool                    `json:"has_more"`
}

// ListSince handles GET /rest/api/v1/admin/audit-logs/since
//
// @Summary      Tail audit log entries via cursor (admin)
// @Description  System-admin only. Returns audit log rows with id > after_id in ascending id order, capped at limit. Designed for external streaming consumers (e.g. SIEM exporters): persist next_after_id between calls and pass it back as after_id; rows are never re-delivered. Use after_id=0 to start from the beginning.
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        after_id  query     int  false  "Strict-greater-than cursor (default 0 = from beginning)"
// @Param        limit     query     int  false  "Max rows to return (default 500, capped at 1000)"
// @Success      200       {object}  handlers.AuditLogStreamResponse
// @Failure      400       {object}  handlers.ErrorResponse  "Invalid after_id or limit"
// @Failure      401       {object}  handlers.ErrorResponse
// @Failure      403       {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:audit-logs:read scope"
// @Failure      500       {object}  handlers.ErrorResponse
// @Router       /admin/audit-logs/since [get]
func (h *AdminAuditLogHandler) ListSince(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	afterID := 0
	if v := q.Get("after_id"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid after_id (expected non-negative integer)"))
			return
		}
		afterID = n
	}

	limit := auditLogStreamDefaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid limit (expected positive integer)"))
			return
		}
		if n > auditLogStreamMaxLimit {
			n = auditLogStreamMaxLimit
		}
		limit = n
	}

	repo := repository.NewAuditLogRepository(h.DB)
	rows, err := repo.ListSince(afterID, limit)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	entries := mapAuditLogRowsToResponses(rows)

	nextAfterID := afterID
	if len(entries) > 0 {
		nextAfterID = entries[len(entries)-1].ID
	}

	if user := middleware.GetUser(r.Context()); user != nil {
		h.Auditor.LogWithDetails(r, user, logger.ActionAuditLogStream, logger.ResourceAuditLog, nil, "audit_log", map[string]any{
			"after_id": afterID,
			"limit":    limit,
			"count":    len(entries),
		})
	}

	h.RespondOK(w, AuditLogStreamResponse{
		Entries:     entries,
		NextAfterID: nextAfterID,
		HasMore:     len(entries) == limit,
	})
}

func mapAuditLogRowsToResponses(rows []repository.AuditLogRow) []AuditLogEntryResponse {
	entries := make([]AuditLogEntryResponse, 0, len(rows))
	for _, e := range rows {
		entries = append(entries, AuditLogEntryResponse{
			ID:           e.ID,
			Timestamp:    e.Timestamp.Format(time.RFC3339),
			UserID:       e.UserID,
			Username:     e.Username,
			IPAddress:    e.IPAddress,
			UserAgent:    e.UserAgent,
			ActionType:   e.ActionType,
			ResourceType: e.ResourceType,
			ResourceID:   e.ResourceID,
			ResourceName: e.ResourceName,
			Details:      e.Details,
			Success:      e.Success,
			ErrorMessage: e.ErrorMessage,
			Source:       e.Source,
		})
	}
	return entries
}
