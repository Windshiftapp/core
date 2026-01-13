package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

)

type TestFolderHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

var (
	errParentFolderNotFound = errors.New("parent folder not found")
	errNestedDepthExceeded  = errors.New("nested folders deeper than two levels are not allowed")
	errParentSelfReference  = errors.New("folder cannot be its own parent")
	errParentHasChildren    = errors.New("folders with subfolders cannot be nested under another folder")
)

func NewTestFolderHandler(db database.Database) *TestFolderHandler {
	// Legacy constructor for backward compatibility
	panic("Use NewTestFolderHandlerWithPool instead")
}

func NewTestFolderHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestFolderHandler {
	return &TestFolderHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

func (h *TestFolderHandler) validateParentFolder(workspaceID int, parentID *int, currentFolderID *int) error {
	if parentID == nil {
		return nil
	}

	if currentFolderID != nil && *parentID == *currentFolderID {
		return errParentSelfReference
	}

	var parentParentID sql.NullInt64
	err := h.getReadDB().QueryRow("SELECT parent_id FROM test_folders WHERE id = ? AND workspace_id = ?", *parentID, workspaceID).Scan(&parentParentID)
	if err == sql.ErrNoRows {
		return errParentFolderNotFound
	}
	if err != nil {
		return err
	}

	if parentParentID.Valid {
		return errNestedDepthExceeded
	}

	if currentFolderID != nil {
		var childCount int
		err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_folders WHERE parent_id = ? AND workspace_id = ?", *currentFolderID, workspaceID).Scan(&childCount)
		if err != nil {
			return err
		}
		if childCount > 0 {
			return errParentHasChildren
		}
	}

	return nil
}

func (h *TestFolderHandler) writeParentValidationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errParentFolderNotFound):
		http.Error(w, errParentFolderNotFound.Error(), http.StatusBadRequest)
	case errors.Is(err, errNestedDepthExceeded):
		http.Error(w, errNestedDepthExceeded.Error(), http.StatusBadRequest)
	case errors.Is(err, errParentSelfReference):
		http.Error(w, errParentSelfReference.Error(), http.StatusBadRequest)
	case errors.Is(err, errParentHasChildren):
		http.Error(w, errParentHasChildren.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func nullableParentID(parentID *int) sql.NullInt64 {
	if parentID == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: int64(*parentID),
		Valid: true,
	}
}

// GetAllFolders returns all test folders with test case counts
func (h *TestFolderHandler) GetAllFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	query := `
		SELECT tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at,
		       COUNT(tc.id) as test_case_count
		FROM test_folders tf
		LEFT JOIN test_cases tc ON tf.id = tc.folder_id
		WHERE tf.workspace_id = ?
		GROUP BY tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at
		ORDER BY tf.sort_order, tf.name
	`

	rows, err := h.getReadDB().Query(query, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var folders []models.TestFolder
	for rows.Next() {
		var folder models.TestFolder
		err := rows.Scan(
			&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
			&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		folders = append(folders, folder)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
}

// GetFolder returns a single test folder
func (h *TestFolderHandler) GetFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid folder ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	query := `
		SELECT tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at,
		       COUNT(tc.id) as test_case_count
		FROM test_folders tf
		LEFT JOIN test_cases tc ON tf.id = tc.folder_id
		WHERE tf.id = ? AND tf.workspace_id = ?
		GROUP BY tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at
	`

	var folder models.TestFolder
	err = h.getReadDB().QueryRow(query, id, workspaceID).Scan(
		&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
		&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Folder not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folder)
}

// CreateFolder creates a new test folder
func (h *TestFolderHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var folder models.TestFolder
	if err := json.NewDecoder(r.Body).Decode(&folder); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if folder.Name == "" {
		http.Error(w, "Folder name is required", http.StatusBadRequest)
		return
	}

	folder.WorkspaceID = workspaceID

	if err := h.validateParentFolder(workspaceID, folder.ParentID, nil); err != nil {
		h.writeParentValidationError(w, err)
		return
	}

	// Get the highest sort_order for new folder ordering
	var maxSortOrder sql.NullInt64
	err = h.getReadDB().QueryRow("SELECT MAX(sort_order) FROM test_folders WHERE workspace_id = ?", workspaceID).Scan(&maxSortOrder)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	folder.SortOrder = int(maxSortOrder.Int64) + 1000 // Leave room for reordering
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()

	query := `
		INSERT INTO test_folders (workspace_id, name, parent_id, description, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err = h.getWriteDB().QueryRow(
		query,
		folder.WorkspaceID,
		folder.Name,
		nullableParentID(folder.ParentID),
		folder.Description,
		folder.SortOrder,
		folder.CreatedAt,
		folder.UpdatedAt,
	).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	folder.ID = int(id)
	folder.TestCaseCount = 0

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(folder)
}

// UpdateFolder updates an existing test folder
func (h *TestFolderHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid folder ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var folder models.TestFolder
	if err := json.Unmarshal(body, &folder); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if folder.Name == "" {
		http.Error(w, "Folder name is required", http.StatusBadRequest)
		return
	}

	var existingParent sql.NullInt64
	var existingSortOrder int
	err = h.getReadDB().QueryRow("SELECT parent_id, sort_order FROM test_folders WHERE id = ? AND workspace_id = ?", id, workspaceID).Scan(&existingParent, &existingSortOrder)
	if err == sql.ErrNoRows {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, parentProvided := rawPayload["parent_id"]
	_, sortOrderProvided := rawPayload["sort_order"]

	if !parentProvided && existingParent.Valid {
		parentID := int(existingParent.Int64)
		folder.ParentID = &parentID
	}
	if !sortOrderProvided {
		folder.SortOrder = existingSortOrder
	}

	if parentProvided && folder.ParentID != nil {
		if err := h.validateParentFolder(workspaceID, folder.ParentID, &id); err != nil {
			h.writeParentValidationError(w, err)
			return
		}
	}

	folder.UpdatedAt = time.Now()

	query := `
		UPDATE test_folders
		SET name = ?, description = ?, parent_id = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := h.getWriteDB().Exec(
		query,
		folder.Name,
		folder.Description,
		nullableParentID(folder.ParentID),
		folder.SortOrder,
		folder.UpdatedAt,
		id,
		workspaceID,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	folder.ID = id
	folder.WorkspaceID = workspaceID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folder)
}

// DeleteFolder deletes a test folder (test cases will be moved to no folder)
func (h *TestFolderHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid folder ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Start transaction to move test cases and delete folder
	tx, err := h.getWriteDB().Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Move test cases to no folder (set folder_id to NULL)
	_, err = tx.Exec("UPDATE test_cases SET folder_id = NULL WHERE folder_id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Promote subfolders to the root level
	_, err = tx.Exec("UPDATE test_folders SET parent_id = NULL WHERE parent_id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the folder
	result, err := tx.Exec("DELETE FROM test_folders WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderFolders updates the sort order of multiple folders
func (h *TestFolderHandler) ReorderFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var reorderData struct {
		FolderIDs []int `json:"folder_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reorderData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Start transaction for atomic reordering
	tx, err := h.getWriteDB().Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Update sort order based on array position
	for i, folderID := range reorderData.FolderIDs {
		sortOrder := (i + 1) * 1000 // Leave gaps for future insertions
		_, err = tx.Exec("UPDATE test_folders SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?",
			sortOrder, time.Now(), folderID, workspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
