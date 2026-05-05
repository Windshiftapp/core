package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// LabelHandler handles label CRUD and item-label management endpoints
type LabelHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

// NewLabelHandler creates a new LabelHandler
func NewLabelHandler(db database.Database, permissionService *services.PermissionService) *LabelHandler {
	return &LabelHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

// GetAll lists labels for a workspace
func (h *LabelHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	if workspaceIDStr == "" {
		respondValidationError(w, r, "workspace_id is required")
		return
	}

	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		respondValidationError(w, r, "Invalid workspace_id")
		return
	}

	// Check workspace view permission
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if h.permissionService != nil {
		hasPermission, permErr := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
		if permErr != nil || !hasPermission {
			respondNotFound(w, r, "Labels")
			return
		}
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT id, name, color, workspace_id, created_at, updated_at
		FROM labels
		WHERE workspace_id = ?
		ORDER BY name
	`, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	labels, err := scanLabels(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// Get returns a single label by ID
func (h *LabelHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var label models.Label
	err := db.QueryRow(`
		SELECT id, name, color, workspace_id, created_at, updated_at
		FROM labels WHERE id = ?
	`, id).Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID,
		&label.CreatedAt, &label.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "Label")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check workspace view permission
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if h.permissionService != nil {
		hasPermission, permErr := h.permissionService.HasWorkspacePermission(user.ID, label.WorkspaceID, models.PermissionItemView)
		if permErr != nil || !hasPermission {
			respondNotFound(w, r, "Label")
			return
		}
	}

	respondJSONOK(w, label)
}

// Create creates a new label
func (h *LabelHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		WorkspaceID int    `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	input.Name = utils.SanitizeName(input.Name)
	if input.Name == "" {
		respondValidationError(w, r, "Label name is required")
		return
	}
	if input.WorkspaceID == 0 {
		respondValidationError(w, r, "workspace_id is required")
		return
	}
	if input.Color == "" {
		input.Color = "#3B82F6"
	}

	// Check workspace permission
	if !h.requireWorkspaceEditPermission(w, r, input.WorkspaceID) {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Check uniqueness
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM labels WHERE name = ? AND workspace_id = ?",
		input.Name, input.WorkspaceID).Scan(&count)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if count > 0 {
		respondConflict(w, r, "A label with this name already exists in this workspace")
		return
	}

	now := time.Now()
	var id int64
	err = db.QueryRow(`
		INSERT INTO labels (name, color, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, input.Name, input.Color, input.WorkspaceID, now, now).Scan(&id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	label, err := loadLabelByID(db, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLabelCreate, logger.ResourceLabel, &label.ID, label.Name)
	}

	respondJSONCreated(w, label)
}

// Update updates a label's name and/or color
func (h *LabelHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	var input struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	input.Name = utils.SanitizeName(input.Name)
	if input.Name == "" {
		respondValidationError(w, r, "Label name is required")
		return
	}

	// Get current label to find workspace_id and check permission
	workspaceID, ok := h.resolveLabelWorkspace(w, r, id)
	if !ok {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Check uniqueness (excluding current)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM labels WHERE name = ? AND workspace_id = ? AND id != ?",
		input.Name, workspaceID, id).Scan(&count)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if count > 0 {
		respondConflict(w, r, "A label with this name already exists in this workspace")
		return
	}

	if input.Color == "" {
		input.Color = "#3B82F6"
	}

	now := time.Now()
	_, err = db.ExecWrite(`
		UPDATE labels SET name = ?, color = ?, updated_at = ? WHERE id = ?
	`, input.Name, input.Color, now, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	label, err := loadLabelByID(db, int64(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLabelUpdate, logger.ResourceLabel, &id, label.Name)
	}

	respondJSONOK(w, label)
}

// Delete deletes a label (cascade removes from items)
func (h *LabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify label exists and user has workspace edit permission
	if _, ok := h.resolveLabelWorkspace(w, r, id); !ok {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err := db.ExecWrite("DELETE FROM labels WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLabelDelete, logger.ResourceLabel, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetItemLabels returns labels for a specific item
func (h *LabelHandler) GetItemLabels(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	h.respondItemLabels(w, r, itemID)
}

// checkItemEditPermission checks if the current user can edit the given item
func (h *LabelHandler) checkItemEditPermission(w http.ResponseWriter, r *http.Request, itemID int) bool {
	return CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemEdit)
}

// requireWorkspaceEditPermission verifies the current user has edit permission
// on the given workspace. Returns false (and writes an HTTP error) on failure.
func (h *LabelHandler) requireWorkspaceEditPermission(w http.ResponseWriter, r *http.Request, workspaceID int) bool {
	if h.permissionService != nil {
		user := utils.GetCurrentUser(r)
		if user == nil {
			respondUnauthorized(w, r)
			return false
		}
		hasPermission, permErr := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemEdit)
		if permErr != nil || !hasPermission {
			respondNotFound(w, r, "Label")
			return false
		}
	}
	return true
}

// resolveLabelWorkspace fetches the workspace_id for the given label and
// verifies the current user has edit permission. Returns the workspace ID and
// true on success, or writes an HTTP error and returns false on failure.
func (h *LabelHandler) resolveLabelWorkspace(w http.ResponseWriter, r *http.Request, labelID int) (int, bool) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return 0, false
	}

	var workspaceID int
	err := db.QueryRow("SELECT workspace_id FROM labels WHERE id = ?", labelID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "Label")
		return 0, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}
	if !h.requireWorkspaceEditPermission(w, r, workspaceID) {
		return 0, false
	}
	return workspaceID, true
}

// SetItemLabels replaces all labels on an item
func (h *LabelHandler) SetItemLabels(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	var input struct {
		LabelIDs []int `json:"label_ids"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing
	_, err = tx.Exec("DELETE FROM item_labels WHERE item_id = ?", itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert new
	now := time.Now()
	for _, labelID := range input.LabelIDs {
		_, err = tx.Exec("INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
			itemID, labelID, now)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to add label %d: %w", labelID, err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated labels
	h.respondItemLabels(w, r, itemID)
}

// AddItemLabel adds a single label to an item
func (h *LabelHandler) AddItemLabel(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	var input struct {
		LabelID int `json:"label_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}
	if input.LabelID == 0 {
		respondValidationError(w, r, "label_id is required")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	_, err = db.ExecWrite("INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
		itemID, input.LabelID, now)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Label is already assigned to this item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	h.respondItemLabels(w, r, itemID)
}

// RemoveItemLabel removes a label from an item
func (h *LabelHandler) RemoveItemLabel(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	labelID, ok := requireIDParam(w, r, "labelId")
	if !ok {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err := db.ExecWrite("DELETE FROM item_labels WHERE item_id = ? AND label_id = ?",
		itemID, labelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// respondItemLabels is a helper to return labels for an item
func (h *LabelHandler) respondItemLabels(w http.ResponseWriter, r *http.Request, itemID int) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id = ?
		ORDER BY l.name
	`, itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	labels, err := scanLabels(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// loadLabelByID loads a single label by its primary key.
func loadLabelByID(db database.Database, id int64) (models.Label, error) {
	var label models.Label
	err := db.QueryRow(`
		SELECT id, name, color, workspace_id, created_at, updated_at
		FROM labels WHERE id = ?
	`, id).Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID,
		&label.CreatedAt, &label.UpdatedAt)
	return label, err
}

// scanLabels iterates over rows scanning each into a models.Label and returns
// the collected slice.
func scanLabels(rows *sql.Rows) ([]models.Label, error) {
	labels := []models.Label{}
	for rows.Next() {
		var label models.Label
		if err := rows.Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// LoadLabelsForItems loads labels for a slice of items in bulk and attaches them
func LoadLabelsForItems(db database.Database, items []models.Item) error {
	if len(items) == 0 {
		return nil
	}

	// Collect item IDs
	itemIDs := make([]interface{}, len(items))
	placeholders := make([]string, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT il.item_id, l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id IN (%s)
		ORDER BY l.name
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, itemIDs...)
	if err != nil {
		return fmt.Errorf("failed to load labels for items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Group labels by item ID
	labelMap := make(map[int][]models.Label)
	for rows.Next() {
		var itemID int
		var label models.Label
		if err := rows.Scan(&itemID, &label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan label: %w", err)
		}
		labelMap[itemID] = append(labelMap[itemID], label)
	}

	// Attach labels to items
	for i := range items {
		if labels, ok := labelMap[items[i].ID]; ok {
			items[i].Labels = labels
		}
	}

	return nil
}
