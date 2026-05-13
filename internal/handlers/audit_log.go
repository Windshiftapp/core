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
		uid, err := strconv.Atoi(v)
		if err != nil {
			respondBadRequest(w, r, "Invalid user_id")
			return
		}
		filters.UserID = &uid
	}
	if v := q.Get("success"); v != "" {
		if v != "true" && v != "false" {
			respondBadRequest(w, r, "Invalid success (expected true or false)")
			return
		}
		b := v == "true"
		filters.Success = &b
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			respondBadRequest(w, r, "Invalid from (expected RFC3339)")
			return
		}
		filters.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			respondBadRequest(w, r, "Invalid to (expected RFC3339)")
			return
		}
		filters.To = &t
	}

	rows, total, err := h.repo.List(filters, page, perPage)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	entries := make([]AuditLogEntry, 0, len(rows))
	for _, e := range rows {
		entries = append(entries, auditRowToEntry(e))
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

// auditRowToEntry converts a repository row to the wire-shape used by both
// the paginated list endpoint and the cursor-based streaming endpoint.
func auditRowToEntry(e repository.AuditLogRow) AuditLogEntry {
	return AuditLogEntry{
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
	}
}

// AuditLogStreamResponse is the cursor-based response shape returned by
// StreamAuditLogsSince. Entries are ordered by id ascending; callers persist
// next_after_id and pass it back as after_id on the next call.
type AuditLogStreamResponse struct {
	Entries     []AuditLogEntry `json:"entries"`
	NextAfterID int             `json:"next_after_id"`
	HasMore     bool            `json:"has_more"`
}

// Default and maximum batch sizes for the streaming endpoint. The cap exists
// to bound response size and DB pressure when a consumer is far behind.
const (
	auditLogStreamDefaultLimit = 500
	auditLogStreamMaxLimit     = 1000
)

// StreamAuditLogsSince handles GET /api/admin/audit-logs/since.
//
// Cursor-based tail for external streaming consumers (e.g. a SIEM exporter
// pro plugin). Returns rows with id > after_id, ordered ASC, capped at limit.
// The strict-greater-than cursor lets callers safely persist next_after_id
// and pass it back without ever re-receiving the same row.
//
// next_after_id is the id of the last returned entry, or the input after_id
// if no entries are returned — callers can blindly persist it either way.
func (h *AuditLogHandler) StreamAuditLogsSince(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	afterID := 0
	if v := q.Get("after_id"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			respondBadRequest(w, r, "Invalid after_id (expected non-negative integer)")
			return
		}
		afterID = n
	}

	limit := auditLogStreamDefaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			respondBadRequest(w, r, "Invalid limit (expected positive integer)")
			return
		}
		if n > auditLogStreamMaxLimit {
			n = auditLogStreamMaxLimit
		}
		limit = n
	}

	rows, err := h.repo.ListSince(afterID, limit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	entries := make([]AuditLogEntry, 0, len(rows))
	for _, e := range rows {
		entries = append(entries, auditRowToEntry(e))
	}

	nextAfterID := afterID
	if len(entries) > 0 {
		nextAfterID = entries[len(entries)-1].ID
	}

	respondJSONOK(w, AuditLogStreamResponse{
		Entries:     entries,
		NextAfterID: nextAfterID,
		HasMore:     len(entries) == limit,
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
