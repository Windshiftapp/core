package handlers

import (
	"database/sql"
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

// scanStatuses scans status rows with their category names and colors.
// The query must select: s.id, s.name, s.description, s.category_id,
// s.is_default, s.created_at, s.updated_at, sc.name, sc.color.
func scanStatuses(rows *sql.Rows) ([]models.Status, error) {
	var statuses []models.Status
	for rows.Next() {
		var status models.Status
		err := rows.Scan(&status.ID, &status.Name, &status.Description, &status.CategoryID,
			&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
			&status.CategoryName, &status.CategoryColor)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	if statuses == nil {
		statuses = []models.Status{}
	}
	return statuses, nil
}

// validateStatusFields checks required fields and verifies the category exists.
// It writes an HTTP error response and returns false on failure.
func (h *StatusHandler) validateStatusFields(w http.ResponseWriter, r *http.Request, status models.Status) bool {
	if strings.TrimSpace(status.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return false
	}
	if status.CategoryID <= 0 {
		respondValidationError(w, r, "Category ID is required")
		return false
	}
	var categoryExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", status.CategoryID).Scan(&categoryExists)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !categoryExists {
		respondValidationError(w, r, "Status category not found")
		return false
	}
	return true
}

// loadStatusByID fetches a single status with its joined category data.
func (h *StatusHandler) loadStatusByID(id int64) (models.Status, error) {
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
	return status, err
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

	statuses, err := scanStatuses(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, statuses)
}

func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	status, err := h.loadStatusByID(int64(id))
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
	status, ok := decodeJSON[models.Status](w, r)
	if !ok {
		return
	}

	if !h.validateStatusFields(w, r, status) {
		return
	}

	// Check if name already exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ?)", status.Name).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	status.Name = utils.SanitizeTitle(status.Name)
	status.Description = utils.SanitizeCommentContent(status.Description)

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
	createdStatus, err := h.loadStatusByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionStatusCreate, logger.ResourceStatus, &intID, status.Name)
	}

	respondJSONCreated(w, createdStatus)
}

func (h *StatusHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	status, ok := decodeJSON[models.Status](w, r)
	if !ok {
		return
	}

	if !h.validateStatusFields(w, r, status) {
		return
	}

	// Check if name already exists (excluding current record)
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ? AND id != ?)", status.Name, id).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	status.Name = utils.SanitizeTitle(status.Name)
	status.Description = utils.SanitizeCommentContent(status.Description)

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
	updatedStatus, err := h.loadStatusByID(int64(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionStatusUpdate, logger.ResourceStatus, &id, updatedStatus.Name)
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
		logAudit(h.db, r, currentUser, logger.ActionStatusDelete, logger.ResourceStatus, &id, "")
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
