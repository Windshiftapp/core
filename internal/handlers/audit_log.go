package handlers

import (
	"net/http"
	"strconv"
	"time"

	"windshift/internal/repository"
)

// AuditLogHandler handles audit log query endpoints.
type AuditLogHandler struct {
	repo *repository.AuditLogRepository
}

// NewAuditLogHandler creates a new audit log handler.
func NewAuditLogHandler(repo *repository.AuditLogRepository) *AuditLogHandler {
	return &AuditLogHandler{repo: repo}
}

// AuditLogEntry represents a single audit log entry in API responses.
type AuditLogEntry struct {
	ID           int                    `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	UserID       *int                   `json:"user_id"`
	Username     string                 `json:"username"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	ActionType   string                 `json:"action_type"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   *int                   `json:"resource_id,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Success      bool                   `json:"success"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// AuditLogResponse is the paginated response for audit log queries.
type AuditLogResponse struct {
	Entries    []AuditLogEntry `json:"entries"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PerPage    int             `json:"per_page"`
	TotalPages int             `json:"total_pages"`
}

// ListAuditLogs handles GET /api/admin/audit-logs with filtering and pagination.
func (h *AuditLogHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Pagination
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	filters := repository.AuditLogFilters{
		ActionType:   q.Get("action_type"),
		ResourceType: q.Get("resource_type"),
		Search:       q.Get("search"),
	}
	if v := q.Get("user_id"); v != "" {
		if uid, err := strconv.Atoi(v); err == nil {
			filters.UserID = &uid
		}
	}
	if v := q.Get("success"); v == "true" || v == "false" {
		b := v == "true"
		filters.Success = &b
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.From = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.To = &t
		}
	}

	rows, total, err := h.repo.List(filters, page, perPage)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	entries := make([]AuditLogEntry, 0, len(rows))
	for _, e := range rows {
		entries = append(entries, AuditLogEntry{
			ID:           e.ID,
			Timestamp:    e.Timestamp,
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
		})
	}

	totalPages := total / perPage
	if total%perPage > 0 {
		totalPages++
	}

	respondJSONOK(w, AuditLogResponse{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

// GetAuditLogActionTypes handles GET /api/admin/audit-logs/action-types.
// Returns distinct action types for filter dropdowns.
func (h *AuditLogHandler) GetAuditLogActionTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.repo.ListDistinctActionTypes()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, types)
}

// GetAuditLogResourceTypes handles GET /api/admin/audit-logs/resource-types.
// Returns distinct resource types for filter dropdowns.
func (h *AuditLogHandler) GetAuditLogResourceTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.repo.ListDistinctResourceTypes()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, types)
}
