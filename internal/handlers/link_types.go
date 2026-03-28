package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// scanAllowedEntityTypes reads the allowed_entity_types JSON column into a []string slice.
func scanAllowedEntityTypes(raw sql.NullString) []string {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var types []string
	if err := json.Unmarshal([]byte(raw.String), &types); err != nil {
		return nil
	}
	return types
}

// marshalAllowedEntityTypes converts a []string slice to a JSON string suitable for SQL, or nil.
func marshalAllowedEntityTypes(types []string) interface{} {
	if len(types) == 0 {
		return nil
	}
	b, err := json.Marshal(types)
	if err != nil {
		return nil
	}
	return string(b)
}

type LinkTypeHandler struct {
	db database.Database
}

func NewLinkTypeHandler(db database.Database) *LinkTypeHandler {
	return &LinkTypeHandler{db: db}
}

func (h *LinkTypeHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Check if we should include inactive link types (admin only)
	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	query := `
		SELECT id, name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at
		FROM link_types
	`
	if !includeInactive {
		query += " WHERE active = true"
	}
	query += " ORDER BY is_system DESC, name ASC"

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var linkTypes []models.LinkType
	for rows.Next() {
		var lt models.LinkType
		var aetRaw sql.NullString
		err := rows.Scan(&lt.ID, &lt.Name, &lt.Description, &lt.ForwardLabel, &lt.ReverseLabel,
			&lt.Color, &lt.IsSystem, &lt.Active, &aetRaw, &lt.CreatedAt, &lt.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		lt.AllowedEntityTypes = scanAllowedEntityTypes(aetRaw)
		linkTypes = append(linkTypes, lt)
	}

	respondJSONOK(w, linkTypes)
}

func (h *LinkTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var lt models.LinkType
	var aetRaw sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at
		FROM link_types
		WHERE id = ?
	`, id).Scan(&lt.ID, &lt.Name, &lt.Description, &lt.ForwardLabel, &lt.ReverseLabel,
		&lt.Color, &lt.IsSystem, &lt.Active, &aetRaw, &lt.CreatedAt, &lt.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "link_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	lt.AllowedEntityTypes = scanAllowedEntityTypes(aetRaw)

	respondJSONOK(w, lt)
}

func (h *LinkTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	lt, ok := decodeJSON[models.LinkType](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if lt.Name == "" || lt.ForwardLabel == "" || lt.ReverseLabel == "" {
		respondValidationError(w, r, "Name, forward_label, and reverse_label are required")
		return
	}

	// Set defaults
	if lt.Color == "" {
		lt.Color = "#6b7280"
	}
	now := time.Now()

	var id int64
	err := h.db.QueryRow(`
		INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color, false, true, marshalAllowedEntityTypes(lt.AllowedEntityTypes), now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = int(id)
	lt.IsSystem = false
	lt.Active = true
	lt.CreatedAt = now
	lt.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeCreate, logger.ResourceLinkType, &intID, lt.Name)
	}

	respondJSONCreated(w, lt)
}

func (h *LinkTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	lt, ok := decodeJSON[models.LinkType](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if lt.Name == "" || lt.ForwardLabel == "" || lt.ReverseLabel == "" {
		respondValidationError(w, r, "Name, forward_label, and reverse_label are required")
		return
	}

	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE link_types
		SET name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, allowed_entity_types = ?, updated_at = ?
		WHERE id = ?
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color, lt.Active, marshalAllowedEntityTypes(lt.AllowedEntityTypes), now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = id
	lt.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeUpdate, logger.ResourceLinkType, &id, lt.Name)
	}

	respondJSONOK(w, lt)
}

func (h *LinkTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check if it's a system link type (can't be deleted)
	var isSystem bool
	err := h.db.QueryRow("SELECT is_system FROM link_types WHERE id = ?", id).Scan(&isSystem)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "link_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if isSystem {
		respondForbidden(w, r)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM link_types WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeDelete, logger.ResourceLinkType, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}
