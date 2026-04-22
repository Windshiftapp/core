package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type TestFolderHandler struct {
	*BaseHandler
}

var (
	errParentFolderNotFound = errors.New("parent folder not found")
	errNestedDepthExceeded  = errors.New("nested folders deeper than two levels are not allowed")
	errParentSelfReference  = errors.New("folder cannot be its own parent")
	errParentHasChildren    = errors.New("folders with subfolders cannot be nested under another folder")
)

func NewTestFolderHandlerWithPool(db database.Database) *TestFolderHandler {
	return &TestFolderHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

func (h *TestFolderHandler) validateParentFolder(db database.Database, workspaceID int, parentID, currentFolderID *int) error {
	if parentID == nil {
		return nil
	}

	if currentFolderID != nil && *parentID == *currentFolderID {
		return errParentSelfReference
	}

	repo := repository.NewTestFolderRepository(db)

	parentParentID, err := repo.GetParentID(*parentID, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		return errParentFolderNotFound
	}
	if err != nil {
		return err
	}

	if parentParentID.Valid {
		return errNestedDepthExceeded
	}

	if currentFolderID != nil {
		childCount, err := repo.CountChildren(*currentFolderID, workspaceID)
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

// GetAllFolders returns all test folders with test case counts
func (h *TestFolderHandler) GetAllFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	folders, err := repository.NewTestFolderRepository(db).FindAllWithCounts(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, folders)
}

// GetFolder returns a single test folder
func (h *TestFolderHandler) GetFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	folder, err := repository.NewTestFolderRepository(db).FindByIDWithCount(id, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_folder")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, folder)
}

// CreateFolder creates a new test folder
func (h *TestFolderHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	folder, ok := decodeJSON[models.TestFolder](w, r)
	if !ok {
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

	if err := h.validateParentFolder(readDB, workspaceID, folder.ParentID, nil); err != nil {
		h.writeParentValidationError(w, r, err)
		return
	}

	maxSortOrder, err := repository.NewTestFolderRepository(readDB).MaxSortOrder(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	folder.SortOrder = maxSortOrder + 1000 // Leave room for reordering
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	id, err := repository.NewTestFolderRepository(writeDB).Create(&folder)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	folder.ID = id
	folder.TestCaseCount = 0

	logAudit(h.db, r, user, logger.ActionTestFolderCreate, logger.ResourceTestFolder, &id, folder.Name)

	respondJSONCreated(w, folder)
}

// UpdateFolder updates an existing test folder
func (h *TestFolderHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

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

	existingParent, existingSortOrder, err := repository.NewTestFolderRepository(readDB).FindParentAndSortOrder(id, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
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

	if err := repository.NewTestFolderRepository(writeDB).Update(id, workspaceID, &folder); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "test_folder")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	folder.ID = id
	folder.WorkspaceID = workspaceID

	logAudit(h.db, r, user, logger.ActionTestFolderUpdate, logger.ResourceTestFolder, &id, folder.Name)

	respondJSONOK(w, folder)
}

// DeleteFolder deletes a test folder (test cases will be moved to no folder)
func (h *TestFolderHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, db, ok := h.requireWorkspaceIDAndIDForWrite(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestFolderRepository(db).DeleteWithCascade(id, workspaceID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "test_folder")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionTestFolderDelete, logger.ResourceTestFolder, &id, "")

	w.WriteHeader(http.StatusNoContent)
}

// ReorderFolders updates the sort order of multiple folders
func (h *TestFolderHandler) ReorderFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	var reorderData struct {
		FolderIDs []int `json:"folder_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reorderData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestFolderRepository(db).Reorder(workspaceID, reorderData.FolderIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]bool{"success": true})
}
