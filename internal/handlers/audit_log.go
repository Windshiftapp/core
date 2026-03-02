package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
)

// AuditLogHandler handles audit log query endpoints.
type AuditLogHandler struct {
	db database.Database
}

// NewAuditLogHandler creates a new audit log handler.
func NewAuditLogHandler(db database.Database) *AuditLogHandler {
	return &AuditLogHandler{db: db}
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

	// Build WHERE clauses
	var conditions []string
	var args []interface{}

	if v := q.Get("action_type"); v != "" {
		conditions = append(conditions, "action_type = ?")
		args = append(args, v)
	}
	if v := q.Get("user_id"); v != "" {
		if uid, err := strconv.Atoi(v); err == nil {
			conditions = append(conditions, "user_id = ?")
			args = append(args, uid)
		}
	}
	if v := q.Get("resource_type"); v != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, v)
	}
	if v := q.Get("success"); v != "" {
		if v == "true" {
			conditions = append(conditions, "success = 1")
		} else if v == "false" {
			conditions = append(conditions, "success = 0")
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			conditions = append(conditions, "timestamp >= ?")
			args = append(args, t)
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			conditions = append(conditions, "timestamp <= ?")
			args = append(args, t)
		}
	}
	if v := q.Get("search"); v != "" {
		search := "%" + v + "%"
		conditions = append(conditions, "(username LIKE ? OR resource_name LIKE ? OR action_type LIKE ?)")
		args = append(args, search, search, search)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM audit_logs " + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Query entries
	offset := (page - 1) * perPage
	dataQuery := `SELECT id, timestamp, user_id, username, ip_address, user_agent,
		action_type, resource_type, resource_id, resource_name, details, success, error_message
		FROM audit_logs ` + whereClause + ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`

	dataArgs := append(args, perPage, offset) //nolint:gocritic
	rows, err := h.db.Query(dataQuery, dataArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	entries := make([]AuditLogEntry, 0)
	for rows.Next() {
		var e AuditLogEntry
		var ipAddress, userAgent, resourceName, detailsJSON, errorMessage *string
		var userID, resourceID *int

		if err := rows.Scan(
			&e.ID, &e.Timestamp, &userID, &e.Username,
			&ipAddress, &userAgent,
			&e.ActionType, &e.ResourceType, &resourceID, &resourceName,
			&detailsJSON, &e.Success, &errorMessage,
		); err != nil {
			respondInternalError(w, r, err)
			return
		}

		e.UserID = userID
		e.ResourceID = resourceID
		if ipAddress != nil {
			e.IPAddress = *ipAddress
		}
		if userAgent != nil {
			e.UserAgent = *userAgent
		}
		if resourceName != nil {
			e.ResourceName = *resourceName
		}
		if errorMessage != nil {
			e.ErrorMessage = *errorMessage
		}
		if detailsJSON != nil && *detailsJSON != "" {
			_ = json.Unmarshal([]byte(*detailsJSON), &e.Details)
		}

		entries = append(entries, e)
	}

	totalPages := total / perPage
	if total%perPage > 0 {
		totalPages++
	}

	resp := AuditLogResponse{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAuditLogActionTypes handles GET /api/admin/audit-logs/action-types.
// Returns distinct action types for filter dropdowns.
func (h *AuditLogHandler) GetAuditLogActionTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT DISTINCT action_type FROM audit_logs ORDER BY action_type")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			respondInternalError(w, r, err)
			return
		}
		types = append(types, t)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types)
}

// GetAuditLogResourceTypes handles GET /api/admin/audit-logs/resource-types.
// Returns distinct resource types for filter dropdowns.
func (h *AuditLogHandler) GetAuditLogResourceTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT DISTINCT resource_type FROM audit_logs ORDER BY resource_type")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			respondInternalError(w, r, err)
			return
		}
		types = append(types, t)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types)
}
