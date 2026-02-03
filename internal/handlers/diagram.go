package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
)

type DiagramHandler struct {
	db database.Database
}

func NewDiagramHandler(db database.Database) *DiagramHandler {
	return &DiagramHandler{
		db: db,
	}
}

// Create creates a new diagram for an item
func (h *DiagramHandler) Create(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

	var req struct {
		Name        string `json:"name"`
		DiagramData string `json:"diagram_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Diagram name is required")
		return
	}

	if req.DiagramData == "" {
		respondValidationError(w, r, "Diagram data is required")
		return
	}

	// Get current user from context
	var createdBy *int
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			createdBy = &u.ID
		}
	}

	query := `
		INSERT INTO item_diagrams (item_id, name, diagram_data, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	result, err := h.db.ExecWrite(query, itemID, req.Name, req.DiagramData, createdBy, now, now)
	if err != nil {
		slog.Error("failed to create diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		slog.Error("failed to get last insert ID", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	diagram := &models.ItemDiagram{
		ID:          int(id),
		ItemID:      itemID,
		Name:        req.Name,
		DiagramData: req.DiagramData,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Record history for diagram creation
	if err := h.recordDiagramHistory(itemID, createdBy, "diagram_created", nil, id, req.Name); err != nil {
		slog.Warn("failed to record diagram creation history", slog.String("component", "diagrams"), slog.Any("error", err))
		// Don't fail the whole operation if history recording fails
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diagram)
}

// GetByItem retrieves all diagrams for an item
func (h *DiagramHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

	query := `
		SELECT
			d.id, d.item_id, d.name, d.diagram_data, d.created_at, d.updated_at, d.created_by, d.updated_by,
			u1.first_name || ' ' || u1.last_name as creator_name, u1.email as creator_email,
			u2.first_name || ' ' || u2.last_name as updated_by_name, u2.email as updated_by_email
		FROM item_diagrams d
		LEFT JOIN users u1 ON d.created_by = u1.id
		LEFT JOIN users u2 ON d.updated_by = u2.id
		WHERE d.item_id = ?
		ORDER BY d.created_at DESC
	`

	rows, err := h.db.Query(query, itemID)
	if err != nil {
		slog.Error("failed to query diagrams", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	diagrams := []models.ItemDiagram{}
	for rows.Next() {
		var d models.ItemDiagram
		var creatorName, creatorEmail, updatedByName, updatedByEmail sql.NullString

		err := rows.Scan(
			&d.ID, &d.ItemID, &d.Name, &d.DiagramData, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy,
			&creatorName, &creatorEmail,
			&updatedByName, &updatedByEmail,
		)
		if err != nil {
			slog.Warn("failed to scan diagram", slog.String("component", "diagrams"), slog.Any("error", err))
			continue
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

		diagrams = append(diagrams, d)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diagrams)
}

// Get retrieves a specific diagram by ID
func (h *DiagramHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	query := `
		SELECT
			d.id, d.item_id, d.name, d.diagram_data, d.created_at, d.updated_at, d.created_by, d.updated_by,
			u1.first_name || ' ' || u1.last_name as creator_name, u1.email as creator_email,
			u2.first_name || ' ' || u2.last_name as updated_by_name, u2.email as updated_by_email
		FROM item_diagrams d
		LEFT JOIN users u1 ON d.created_by = u1.id
		LEFT JOIN users u2 ON d.updated_by = u2.id
		WHERE d.id = ?
	`

	var d models.ItemDiagram
	var creatorName, creatorEmail, updatedByName, updatedByEmail sql.NullString

	err = h.db.QueryRow(query, id).Scan(
		&d.ID, &d.ItemID, &d.Name, &d.DiagramData, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy,
		&creatorName, &creatorEmail,
		&updatedByName, &updatedByEmail,
	)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to query diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// Update updates an existing diagram
func (h *DiagramHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var req struct {
		Name        string `json:"name"`
		DiagramData string `json:"diagram_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Diagram name is required")
		return
	}

	if req.DiagramData == "" {
		respondValidationError(w, r, "Diagram data is required")
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			userID = &u.ID
		}
	}

	// Get old diagram name and item_id before updating
	var oldName string
	var itemID int
	err = h.db.QueryRow("SELECT name, item_id FROM item_diagrams WHERE id = ?", id).Scan(&oldName, &itemID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to get diagram details", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	query := `
		UPDATE item_diagrams
		SET name = ?, diagram_data = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
	`

	now := time.Now()
	result, err := h.db.ExecWrite(query, req.Name, req.DiagramData, now, userID, id)
	if err != nil {
		slog.Error("failed to update diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		slog.Error("failed to get rows affected", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "diagram")
		return
	}

	// Record history for diagram update
	if userID != nil {
		// Track update - show old name if it changed, otherwise show current name
		var historyOldName *string
		if oldName != req.Name {
			historyOldName = &oldName
		}
		if err := h.recordDiagramHistory(itemID, userID, "diagram_updated", historyOldName, int64(id), req.Name); err != nil {
			slog.Warn("failed to record diagram update history", slog.String("component", "diagrams"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	// Retrieve the updated diagram
	getQuery := `
		SELECT
			d.id, d.item_id, d.name, d.diagram_data, d.created_at, d.updated_at, d.created_by, d.updated_by,
			u1.first_name || ' ' || u1.last_name as creator_name, u1.email as creator_email,
			u2.first_name || ' ' || u2.last_name as updated_by_name, u2.email as updated_by_email
		FROM item_diagrams d
		LEFT JOIN users u1 ON d.created_by = u1.id
		LEFT JOIN users u2 ON d.updated_by = u2.id
		WHERE d.id = ?
	`

	var d models.ItemDiagram
	var creatorName, creatorEmail, updatedByName, updatedByEmail sql.NullString

	err = h.db.QueryRow(getQuery, id).Scan(
		&d.ID, &d.ItemID, &d.Name, &d.DiagramData, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy,
		&creatorName, &creatorEmail,
		&updatedByName, &updatedByEmail,
	)

	if err != nil {
		slog.Error("failed to retrieve updated diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// Delete deletes a diagram
func (h *DiagramHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			userID = &u.ID
		}
	}

	// Get diagram details before deletion (for history tracking)
	var diagramName string
	var itemID int
	err = h.db.QueryRow("SELECT name, item_id FROM item_diagrams WHERE id = ?", id).Scan(&diagramName, &itemID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to get diagram details", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Record history before deletion
	if userID != nil {
		if err := h.recordDiagramHistory(itemID, userID, "diagram_deleted", &diagramName, 0, diagramName); err != nil {
			slog.Warn("failed to record diagram deletion history", slog.String("component", "diagrams"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	query := `DELETE FROM item_diagrams WHERE id = ?`
	result, err := h.db.ExecWrite(query, id)
	if err != nil {
		slog.Error("failed to delete diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		slog.Error("failed to get rows affected", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "diagram")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Diagram %d deleted successfully", id),
	})
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
