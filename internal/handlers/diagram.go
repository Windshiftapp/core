package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type DiagramHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewDiagramHandler(db database.Database, permissionService *services.PermissionService) *DiagramHandler {
	return &DiagramHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

// checkItemEditPermission checks if the current user can edit the given item
func (h *DiagramHandler) checkItemEditPermission(w http.ResponseWriter, r *http.Request, itemID int) bool {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return false
	}
	return CheckItemPermission(w, r, db, h.permissionService, itemID, models.PermissionItemEdit)
}

// decodeDiagramRequest decodes the JSON body, sanitizes the name, and validates
// that both name and diagram_data are non-empty. It writes an error response and
// returns ok=false on failure.
func decodeDiagramRequest(w http.ResponseWriter, r *http.Request) (name, diagramData string, ok bool) {
	var req struct {
		Name        string `json:"name"`
		DiagramData string `json:"diagram_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return "", "", false
	}

	req.Name = utils.SanitizeName(req.Name)

	if req.Name == "" {
		respondValidationError(w, r, "Diagram name is required")
		return "", "", false
	}

	if req.DiagramData == "" {
		respondValidationError(w, r, "Diagram data is required")
		return "", "", false
	}

	return req.Name, req.DiagramData, true
}

// execWriteAndCheck executes a write query, logs errors, checks rows affected,
// and responds 404 if no rows were affected. Returns true on success.
func execWriteAndCheck(w http.ResponseWriter, r *http.Request, wdb database.Database, query string, args ...interface{}) bool {
	result, err := wdb.ExecWrite(query, args...)
	if err != nil {
		slog.Error("failed to execute write", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return false
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		slog.Error("failed to get rows affected", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return false
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "diagram")
		return false
	}

	return true
}

// Create creates a new diagram for an item
func (h *DiagramHandler) Create(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	name, diagramData, ok := decodeDiagramRequest(w, r)
	if !ok {
		return
	}

	// Get current user from context
	var createdBy *int
	if user := utils.GetCurrentUser(r); user != nil {
		createdBy = &user.ID
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	var id int64
	err := db.QueryRow(`
		INSERT INTO item_diagrams (item_id, name, diagram_data, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, itemID, name, diagramData, createdBy, now, now).Scan(&id)
	if err != nil {
		slog.Error("failed to create diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	diagram := &models.ItemDiagram{
		ID:          int(id),
		ItemID:      itemID,
		Name:        name,
		DiagramData: diagramData,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Record history for diagram creation
	if err := h.recordDiagramHistory(itemID, createdBy, "diagram_created", nil, id, name); err != nil {
		slog.Warn("failed to record diagram creation history", slog.String("component", "diagrams"), slog.Any("error", err))
		// Don't fail the whole operation if history recording fails
	}

	respondJSONOK(w, diagram)
}

const diagramSelectWithUsers = `
	SELECT
		d.id, d.item_id, d.name, d.diagram_data, d.created_at, d.updated_at, d.created_by, d.updated_by,
		u1.first_name || ' ' || u1.last_name as creator_name, u1.email as creator_email,
		u2.first_name || ' ' || u2.last_name as updated_by_name, u2.email as updated_by_email
	FROM item_diagrams d
	LEFT JOIN users u1 ON d.created_by = u1.id
	LEFT JOIN users u2 ON d.updated_by = u2.id`

func scanDiagramWithUsers(scanner interface{ Scan(dest ...any) error }) (models.ItemDiagram, error) {
	var d models.ItemDiagram
	var creatorName, creatorEmail, updatedByName, updatedByEmail sql.NullString

	err := scanner.Scan(
		&d.ID, &d.ItemID, &d.Name, &d.DiagramData, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy,
		&creatorName, &creatorEmail,
		&updatedByName, &updatedByEmail,
	)
	if err != nil {
		return d, err
	}

	if creatorName.Valid {
		d.CreatorName = creatorName.String
	}
	if creatorEmail.Valid {
		d.CreatorEmail = creatorEmail.String
	}
	if updatedByName.Valid {
		d.UpdatedByName = updatedByName.String
	}
	if updatedByEmail.Valid {
		d.UpdatedByEmail = updatedByEmail.String
	}

	return d, nil
}

// GetByItem retrieves all diagrams for an item
func (h *DiagramHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	rows, err := db.Query(diagramSelectWithUsers+` WHERE d.item_id = ? ORDER BY d.created_at DESC`, itemID)
	if err != nil {
		slog.Error("failed to query diagrams", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	diagrams := []models.ItemDiagram{}
	for rows.Next() {
		d, err := scanDiagramWithUsers(rows)
		if err != nil {
			slog.Error("failed to scan diagram", slog.String("component", "diagrams"), slog.Any("error", err))
			respondInternalError(w, r, fmt.Errorf("failed to scan diagram: %w", err))
			return
		}
		diagrams = append(diagrams, d)
	}

	respondJSONOK(w, diagrams)
}

// Get retrieves a specific diagram by ID
func (h *DiagramHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	d, err := scanDiagramWithUsers(db.QueryRow(diagramSelectWithUsers+` WHERE d.id = ?`, id))
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to query diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if !CheckItemPermission(w, r, db, h.permissionService, d.ItemID, models.PermissionItemView) {
		return
	}

	respondJSONOK(w, d)
}

// Update updates an existing diagram
func (h *DiagramHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	name, diagramData, ok := decodeDiagramRequest(w, r)
	if !ok {
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := utils.GetCurrentUser(r); user != nil {
		userID = &user.ID
	}

	// Get old diagram name and item_id before updating
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	oldName, itemID, ok := getDiagramDetails(w, r, db, id)
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	wdb, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	if !execWriteAndCheck(w, r, wdb, `
		UPDATE item_diagrams
		SET name = ?, diagram_data = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
	`, name, diagramData, now, userID, id) {
		return
	}

	// Record history for diagram update
	if userID != nil {
		// Track update - show old name if it changed, otherwise show current name
		var historyOldName *string
		if oldName != name {
			historyOldName = &oldName
		}
		if err := h.recordDiagramHistory(itemID, userID, "diagram_updated", historyOldName, int64(id), name); err != nil {
			slog.Warn("failed to record diagram update history", slog.String("component", "diagrams"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	// Retrieve the updated diagram
	d, err := scanDiagramWithUsers(db.QueryRow(diagramSelectWithUsers+` WHERE d.id = ?`, id))
	if err != nil {
		slog.Error("failed to retrieve updated diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, d)
}

// Delete deletes a diagram
func (h *DiagramHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := utils.GetCurrentUser(r); user != nil {
		userID = &user.ID
	}

	// Get diagram details before deletion (for history tracking)
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	diagramName, itemID, ok := getDiagramDetails(w, r, db, id)
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Record history before deletion
	if userID != nil {
		if err := h.recordDiagramHistory(itemID, userID, "diagram_deleted", &diagramName, 0, diagramName); err != nil {
			slog.Warn("failed to record diagram deletion history", slog.String("component", "diagrams"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	wdb, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if !execWriteAndCheck(w, r, wdb, `DELETE FROM item_diagrams WHERE id = ?`, id) {
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Diagram %d deleted successfully", id),
	})
}

// getDiagramDetails fetches the name and item_id for a diagram, writing an error response on failure.
func getDiagramDetails(w http.ResponseWriter, r *http.Request, db database.Database, id int) (name string, itemID int, ok bool) {
	err := db.QueryRow("SELECT name, item_id FROM item_diagrams WHERE id = ?", id).Scan(&name, &itemID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "diagram")
		return "", 0, false
	}
	if err != nil {
		slog.Error("failed to get diagram details", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return "", 0, false
	}
	return name, itemID, true
}

// recordDiagramHistory records diagram-related changes to item history
func (h *DiagramHandler) recordDiagramHistory(itemID int, userID *int, action string, oldValue *string, diagramID int64, diagramName string) error {
	if userID == nil {
		return nil // Skip if no user context
	}

	var value string
	if action == "diagram_deleted" {
		value = diagramName
	} else {
		value = fmt.Sprintf("diagram:%d:%s", diagramID, diagramName)
	}

	query := `INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
	          VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err := h.db.ExecWrite(query, itemID, *userID, action, oldValue, value)
	return err
}
