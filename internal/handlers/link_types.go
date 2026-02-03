package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
)

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
		SELECT id, name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at
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
	defer rows.Close()

	var linkTypes []models.LinkType
	for rows.Next() {
		var lt models.LinkType
		err := rows.Scan(&lt.ID, &lt.Name, &lt.Description, &lt.ForwardLabel, &lt.ReverseLabel,
			&lt.Color, &lt.IsSystem, &lt.Active, &lt.CreatedAt, &lt.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
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
	err := h.db.QueryRow(`
		SELECT id, name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at
		FROM link_types 
		WHERE id = ?
	`, id).Scan(&lt.ID, &lt.Name, &lt.Description, &lt.ForwardLabel, &lt.ReverseLabel,
		&lt.Color, &lt.IsSystem, &lt.Active, &lt.CreatedAt, &lt.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "link_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, lt)
}

func (h *LinkTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var lt models.LinkType
	if err := json.NewDecoder(r.Body).Decode(&lt); err != nil {
		respondBadRequest(w, r, err.Error())
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
		INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color, false, true, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = int(id)
	lt.IsSystem = false
	lt.Active = true
	lt.CreatedAt = now
	lt.UpdatedAt = now

	respondJSONCreated(w, lt)
}

func (h *LinkTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var lt models.LinkType
	if err := json.NewDecoder(r.Body).Decode(&lt); err != nil {
		respondBadRequest(w, r, err.Error())
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
		SET name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, updated_at = ?
		WHERE id = ?
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color, lt.Active, now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = id
	lt.UpdatedAt = now

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

	w.WriteHeader(http.StatusNoContent)
}