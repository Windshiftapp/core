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

func NewTestFolderHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestFolderHandler {
	return &TestFolderHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

func (h *TestFolderHandler) validateParentFolder(db *sql.DB, workspaceID int, parentID, currentFolderID *int) error {
	if parentID == nil {
		return nil
	}

	if currentFolderID != nil && *parentID == *currentFolderID {
		return errParentSelfReference
	}

	var parentParentID sql.NullInt64
	err := db.QueryRow("SELECT parent_id FROM test_folders WHERE id = ? AND workspace_id = ?", *parentID, workspaceID).Scan(&parentParentID)
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
		err = db.QueryRow("SELECT COUNT(*) FROM test_folders WHERE parent_id = ? AND workspace_id = ?", *currentFolderID, workspaceID).Scan(&childCount)
		if err != nil {
			return err
		}
		if childCount > 0 {
			return errParentHasChildren
		}
	}

	return nil
}

func (h *TestFolderHandler) writeParentValidationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errParentFolderNotFound):
		respondValidationError(w, r, errParentFolderNotFound.Error())
	case errors.Is(err, errNestedDepthExceeded):
		respondValidationError(w, r, errNestedDepthExceeded.Error())
	case errors.Is(err, errParentSelfReference):
		respondValidationError(w, r, errParentSelfReference.Error())
	case errors.Is(err, errParentHasChildren):
		respondValidationError(w, r, errParentHasChildren.Error())
	default:
		respondInternalError(w, r, err)
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
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
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

	rows, err := db.Query(query, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var folders []models.TestFolder
	for rows.Next() {
		var folder models.TestFolder
		err := rows.Scan(
			&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
			&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		folders = append(folders, folder)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(folders)
}

// GetFolder returns a single test folder
func (h *TestFolderHandler) GetFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
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
	err = db.QueryRow(query, id, workspaceID).Scan(
		&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
		&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "test_folder")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(folder)
}

// CreateFolder creates a new test folder
func (h *TestFolderHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestManage, h.permissionService) {
		return
	}

	var folder models.TestFolder
	if err = json.NewDecoder(r.Body).Decode(&folder); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if folder.Name == "" {
		respondValidationError(w, r, "Folder name is required")
		return
	}

	folder.WorkspaceID = workspaceID

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	if err = h.validateParentFolder(readDB, workspaceID, folder.ParentID, nil); err != nil {
		h.writeParentValidationError(w, r, err)
		return
	}

	// Get the highest sort_order for new folder ordering
	var maxSortOrder sql.NullInt64
	err = readDB.QueryRow("SELECT MAX(sort_order) FROM test_folders WHERE workspace_id = ?", workspaceID).Scan(&maxSortOrder)
	if err != nil && err != sql.ErrNoRows {
		respondInternalError(w, r, err)
		return
	}

	folder.SortOrder = int(maxSortOrder.Int64) + 1000 // Leave room for reordering
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	query := `
		INSERT INTO test_folders (workspace_id, name, parent_id, description, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err = writeDB.QueryRow(
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
		respondInternalError(w, r, err)
		return
	}

	folder.ID = int(id)
	folder.TestCaseCount = 0

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(folder)
}

// UpdateFolder updates an existing test folder
func (h *TestFolderHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestManage, h.permissionService) {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	var folder models.TestFolder
	if err = json.Unmarshal(body, &folder); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	var rawPayload map[string]json.RawMessage
	if err = json.Unmarshal(body, &rawPayload); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if folder.Name == "" {
		respondValidationError(w, r, "Folder name is required")
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var existingParent sql.NullInt64
	var existingSortOrder int
	err = readDB.QueryRow("SELECT parent_id, sort_order FROM test_folders WHERE id = ? AND workspace_id = ?", id, workspaceID).Scan(&existingParent, &existingSortOrder)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "test_folder")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
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
		if err = h.validateParentFolder(readDB, workspaceID, folder.ParentID, &id); err != nil {
			h.writeParentValidationError(w, r, err)
			return
		}
	}

	folder.UpdatedAt = time.Now()

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	query := `
		UPDATE test_folders
		SET name = ?, description = ?, parent_id = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := writeDB.Exec(
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
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "test_folder")
		return
	}

	folder.ID = id
	folder.WorkspaceID = workspaceID
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(folder)
}

// DeleteFolder deletes a test folder (test cases will be moved to no folder)
func (h *TestFolderHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestManage, h.permissionService) {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Start transaction to move test cases and delete folder
	tx, err := db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Move test cases to no folder (set folder_id to NULL)
	_, err = tx.Exec("UPDATE test_cases SET folder_id = NULL WHERE folder_id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Promote subfolders to the root level
	_, err = tx.Exec("UPDATE test_folders SET parent_id = NULL WHERE parent_id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the folder
	result, err := tx.Exec("DELETE FROM test_folders WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "test_folder")
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderFolders updates the sort order of multiple folders
func (h *TestFolderHandler) ReorderFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestManage, h.permissionService) {
		return
	}

	var reorderData struct {
		FolderIDs []int `json:"folder_ids"`
	}

	if err = json.NewDecoder(r.Body).Decode(&reorderData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Start transaction for atomic reordering
	tx, err := db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Update sort order based on array position
	for i, folderID := range reorderData.FolderIDs {
		sortOrder := (i + 1) * 1000 // Leave gaps for future insertions
		_, err = tx.Exec("UPDATE test_folders SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?",
			sortOrder, time.Now(), folderID, workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
