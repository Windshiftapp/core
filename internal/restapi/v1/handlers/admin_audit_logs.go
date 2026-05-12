package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/services"
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
		uid, err := strconv.Atoi(v)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid user_id"))
			return
		}
		where += " AND user_id = ?"
		args = append(args, uid)
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid from (expected RFC3339)"))
			return
		}
		where += " AND timestamp >= ?"
		args = append(args, t)
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid to (expected RFC3339)"))
			return
		}
		where += " AND timestamp <= ?"
		args = append(args, t)
	}

	// Count
	var total int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	// Fetch
	fetchArgs := make([]interface{}, len(args), len(args)+2)
	copy(fetchArgs, args)
	fetchArgs = append(fetchArgs, pagination.Limit, pagination.Offset)
	rows, err := h.DB.Query(`
		SELECT id, timestamp, user_id, username, ip_address, action_type,
		       resource_type, resource_id, resource_name, details, success
		FROM audit_logs `+where+`
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, fetchArgs...)
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondPaginated(w, entries, pagination, total)
}
