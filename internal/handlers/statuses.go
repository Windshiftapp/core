package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"windshift/internal/constants"
	"windshift/internal/database"
	"windshift/internal/models"
)

type StatusHandler struct {
	db database.Database
}

func NewStatusHandler(db database.Database) *StatusHandler {
	return &StatusHandler{db: db}
}

func (h *StatusHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		ORDER BY s.is_default DESC, sc.name ASC, s.name ASC`

	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var statuses []models.Status
	for rows.Next() {
		var status models.Status

		err := rows.Scan(&status.ID, &status.Name, &status.Description, &status.CategoryID,
			&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
			&status.CategoryName, &status.CategoryColor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		statuses = append(statuses, status)
	}

	// Always return an array, even if empty
	if statuses == nil {
		statuses = []models.Status{}
	}

	respondJSONOK(w, statuses)
}

func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var status models.Status
	err := h.db.QueryRow(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, id).Scan(&status.ID, &status.Name, &status.Description, &status.CategoryID,
		&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
		&status.CategoryName, &status.CategoryColor)

	if err == sql.ErrNoRows {
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, status)
}

func (h *StatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	var status models.Status
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(status.Name) == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if status.CategoryID <= 0 {
		http.Error(w, "Category ID is required", http.StatusBadRequest)
		return
	}

	// Validate that category exists
	var categoryExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", status.CategoryID).Scan(&categoryExists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !categoryExists {
		http.Error(w, "Status category not found", http.StatusBadRequest)
		return
	}

	// Check if name already exists
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ?)", status.Name).Scan(&exists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Status with this name already exists", http.StatusConflict)
		return
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, status.Name, status.Description, status.CategoryID, status.IsDefault, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created status with joined data
	var createdStatus models.Status
	err = h.db.QueryRow(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, id).Scan(&createdStatus.ID, &createdStatus.Name, &createdStatus.Description, &createdStatus.CategoryID,
		&createdStatus.IsDefault, &createdStatus.CreatedAt, &createdStatus.UpdatedAt,
		&createdStatus.CategoryName, &createdStatus.CategoryColor)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONCreated(w, createdStatus)
}

func (h *StatusHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var status models.Status
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(status.Name) == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if status.CategoryID <= 0 {
		http.Error(w, "Category ID is required", http.StatusBadRequest)
		return
	}

	// Validate that category exists
	var categoryExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", status.CategoryID).Scan(&categoryExists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !categoryExists {
		http.Error(w, "Status category not found", http.StatusBadRequest)
		return
	}

	// Check if name already exists (excluding current record)
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ? AND id != ?)", status.Name, id).Scan(&exists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Status with this name already exists", http.StatusConflict)
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE statuses 
		SET name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, status.Name, status.Description, status.CategoryID, status.IsDefault, now, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated status with joined data
	var updatedStatus models.Status
	err = h.db.QueryRow(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, id).Scan(&updatedStatus.ID, &updatedStatus.Name, &updatedStatus.Description, &updatedStatus.CategoryID,
		&updatedStatus.IsDefault, &updatedStatus.CreatedAt, &updatedStatus.UpdatedAt,
		&updatedStatus.CategoryName, &updatedStatus.CategoryColor)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, updatedStatus)
}

func (h *StatusHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Protect system-critical statuses from deletion
	if id == constants.StatusIDOpen || id == constants.StatusIDDone {
		http.Error(w, "Cannot delete Open or Done status - these are required by the system", http.StatusForbidden)
		return
	}

	// Check if any workflow transitions are using this status
	var transitionCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", id, id).Scan(&transitionCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if transitionCount > 0 {
		http.Error(w, "Cannot delete status that is in use by workflow transitions", http.StatusConflict)
		return
	}

	// Check if any items are using this status
	var itemCount int
	err = h.db.QueryRow("SELECT COUNT(*) FROM items WHERE status_id = ?", id).Scan(&itemCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if itemCount > 0 {
		http.Error(w, "Cannot delete status that is in use by "+strconv.Itoa(itemCount)+" work item(s)", http.StatusConflict)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM statuses WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetNonDoneStatusIDs returns the IDs of statuses that are not in "Done" category
func (h *StatusHandler) GetNonDoneStatusIDs(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT s.id
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE COALESCE(sc.is_completed, FALSE) = FALSE
		ORDER BY s.id ASC`

	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var statusIDs []int
	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		statusIDs = append(statusIDs, id)
	}

	// Always return an array, even if empty
	if statusIDs == nil {
		statusIDs = []int{}
	}

	respondJSONOK(w, statusIDs)
}
