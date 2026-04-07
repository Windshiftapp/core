package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/restapi"
)

// AdminAuditLogHandler handles audit log access in REST API v1.
type AdminAuditLogHandler struct {
	db database.Database
}

// NewAdminAuditLogHandler creates a new admin audit log handler.
func NewAdminAuditLogHandler(db database.Database) *AdminAuditLogHandler {
	return &AdminAuditLogHandler{db: db}
}

// AuditLogEntryResponse is the REST API v1 representation of an audit log entry.
type AuditLogEntryResponse struct {
	ID           int                    `json:"id"`
	Timestamp    string                 `json:"timestamp"`
	UserID       *int                   `json:"user_id"`
	Username     string                 `json:"username"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	ActionType   string                 `json:"action_type"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   *int                   `json:"resource_id,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Success      bool                   `json:"success"`
}

// List handles GET /rest/api/v1/admin/audit-logs
func (h *AdminAuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	pagination := restapi.ParsePaginationParams(r)
	q := r.URL.Query()

	// Build filter
	where := "WHERE 1=1"
	var args []interface{}

	if v := q.Get("action_type"); v != "" {
		where += " AND action_type = ?"
		args = append(args, v)
	}
	if v := q.Get("resource_type"); v != "" {
		where += " AND resource_type = ?"
		args = append(args, v)
	}
	if v := q.Get("user_id"); v != "" {
		if uid, err := strconv.Atoi(v); err == nil {
			where += " AND user_id = ?"
			args = append(args, uid)
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			where += " AND timestamp >= ?"
			args = append(args, t)
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			where += " AND timestamp <= ?"
			args = append(args, t)
		}
	}

	// Count
	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Fetch
	fetchArgs := append(args, pagination.Limit, pagination.Offset)
	rows, err := h.db.Query(`
		SELECT id, timestamp, user_id, username, ip_address, action_type,
		       resource_type, resource_id, resource_name, details, success
		FROM audit_logs `+where+`
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, fetchArgs...)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var entries []AuditLogEntryResponse
	for rows.Next() {
		var e AuditLogEntryResponse
		var ts time.Time
		var userID sql.NullInt64
		var resourceID sql.NullInt64
		var ipAddr, resourceName sql.NullString
		var detailsJSON sql.NullString

		if err := rows.Scan(&e.ID, &ts, &userID, &e.Username, &ipAddr,
			&e.ActionType, &e.ResourceType, &resourceID, &resourceName,
			&detailsJSON, &e.Success); err != nil {
			continue
		}

		e.Timestamp = ts.Format(time.RFC3339)
		if userID.Valid {
			uid := int(userID.Int64)
			e.UserID = &uid
		}
		if ipAddr.Valid {
			e.IPAddress = ipAddr.String
		}
		if resourceID.Valid {
			rid := int(resourceID.Int64)
			e.ResourceID = &rid
		}
		if resourceName.Valid {
			e.ResourceName = resourceName.String
		}
		if detailsJSON.Valid && detailsJSON.String != "" {
			_ = json.Unmarshal([]byte(detailsJSON.String), &e.Details)
		}

		entries = append(entries, e)
	}

	if entries == nil {
		entries = []AuditLogEntryResponse{}
	}

	restapi.RespondPaginated(w, entries, restapi.NewPaginationMeta(pagination, total))
}
