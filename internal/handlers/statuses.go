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
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
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
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.Status
	for rows.Next() {
		var status models.Status

		err := rows.Scan(&status.ID, &status.Name, &status.Description, &status.CategoryID,
			&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
			&status.CategoryName, &status.CategoryColor)
		if err != nil {
			respondInternalError(w, r, err)
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
		respondNotFound(w, r, "status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, status)
}

func (h *StatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	var status models.Status
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(status.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if status.CategoryID <= 0 {
		respondValidationError(w, r, "Category ID is required")
		return
	}

	// Validate that category exists
	var categoryExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", status.CategoryID).Scan(&categoryExists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !categoryExists {
		respondValidationError(w, r, "Status category not found")
		return
	}

	// Check if name already exists
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ?)", status.Name).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, status.Name, status.Description, status.CategoryID, status.IsDefault, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
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
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionStatusCreate,
			ResourceType: logger.ResourceStatus,
			ResourceID:   &intID,
			ResourceName: status.Name,
			Success:      true,
		})
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
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(status.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if status.CategoryID <= 0 {
		respondValidationError(w, r, "Category ID is required")
		return
	}

	// Validate that category exists
	var categoryExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", status.CategoryID).Scan(&categoryExists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !categoryExists {
		respondValidationError(w, r, "Status category not found")
		return
	}

	// Check if name already exists (excluding current record)
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ? AND id != ?)", status.Name, id).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE statuses
		SET name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, status.Name, status.Description, status.CategoryID, status.IsDefault, now, id)

	if err != nil {
		respondInternalError(w, r, err)
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
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionStatusUpdate,
			ResourceType: logger.ResourceStatus,
			ResourceID:   &id,
			ResourceName: updatedStatus.Name,
			Success:      true,
		})
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
		respondForbidden(w, r)
		return
	}

	// Check if any workflow transitions are using this status
	var transitionCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", id, id).Scan(&transitionCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if transitionCount > 0 {
		respondConflict(w, r, "Cannot delete status that is in use by workflow transitions")
		return
	}

	// Check if any items are using this status
	var itemCount int
	err = h.db.QueryRow("SELECT COUNT(*) FROM items WHERE status_id = ?", id).Scan(&itemCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if itemCount > 0 {
		respondConflict(w, r, "Cannot delete status that is in use by "+strconv.Itoa(itemCount)+" work item(s)")
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM statuses WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionStatusDelete,
			ResourceType: logger.ResourceStatus,
			ResourceID:   &id,
			Success:      true,
		})
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
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var statusIDs []int
	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		if err != nil {
			respondInternalError(w, r, err)
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
