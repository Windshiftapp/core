package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/restapi"
)

// AdminGroupHandler handles admin group management in REST API v1.
type AdminGroupHandler struct {
	db database.Database
}

// NewAdminGroupHandler creates a new admin group handler.
func NewAdminGroupHandler(db database.Database) *AdminGroupHandler {
	return &AdminGroupHandler{db: db}
}

// AdminGroupResponse is the admin representation of a group.
type AdminGroupResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
}

// AdminGroupCreateRequest is the request body for creating a group.
type AdminGroupCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AdminGroupUpdateRequest is the request body for updating a group.
type AdminGroupUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// List handles GET /rest/api/v1/admin/groups
func (h *AdminGroupHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM team_groups").Scan(&total); err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	rows, err := h.db.Query(`
		SELECT g.id, g.name, g.description,
		       (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id) AS member_count,
		       g.created_at
		FROM team_groups g
		ORDER BY g.name ASC
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var groups []AdminGroupResponse
	for rows.Next() {
		var g AdminGroupResponse
		var desc sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.MemberCount, &createdAt); err != nil {
			continue
		}
		if desc.Valid {
			g.Description = desc.String
		}
		g.CreatedAt = createdAt.Format("2006-01-02T15:04:05Z07:00")
		groups = append(groups, g)
	}

	if groups == nil {
		groups = []AdminGroupResponse{}
	}

	restapi.RespondPaginated(w, groups, restapi.NewPaginationMeta(pagination, total))
}

// Create handles POST /rest/api/v1/admin/groups
func (h *AdminGroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req AdminGroupCreateRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}

	if req.Name == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "Group name is required"))
		return
	}

	var id int
	err := h.db.QueryRow(`
		INSERT INTO team_groups (name, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, req.Name, req.Description, user.ID).Scan(&id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondCreated(w, AdminGroupResponse{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		MemberCount: 0,
		CreatedAt:   time.Now().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// Update handles PUT /rest/api/v1/admin/groups/{id}
func (h *AdminGroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, ok := parsePathID(w, r, "id", "group ID")
	if !ok {
		return
	}

	var req AdminGroupUpdateRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}

	var sets []string
	var args []interface{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}

	if len(sets) == 0 {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "No fields to update"))
		return
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE team_groups SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = ?"

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondNoContent(w)
}

// Delete handles DELETE /rest/api/v1/admin/groups/{id}
func (h *AdminGroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, ok := parsePathID(w, r, "id", "group ID")
	if !ok {
		return
	}

	// Delete members first, then group
	_, _ = h.db.ExecWrite("DELETE FROM group_members WHERE group_id = ?", id)

	result, err := h.db.ExecWrite("DELETE FROM team_groups WHERE id = ?", id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondNoContent(w)
}
