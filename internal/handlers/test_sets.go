package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type TestSetHandler struct {
	*BaseHandler
}

func NewTestSetHandlerWithPool(db database.Database) *TestSetHandler {
	return &TestSetHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

func (h *TestSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	sets, err := repository.NewTestSetRepository(db).FindAllWithStats(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, sets)
}

func (h *TestSetHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	set, err := repository.NewTestSetRepository(db).FindByID(id, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, set)
}

// decodeTestSetWrite extracts the workspace ID, current user, decoded+sanitized TestSet, and
// write DB. Returns false if any step fails (an error response will already have been written).
func (h *TestSetHandler) decodeTestSetWrite(w http.ResponseWriter, r *http.Request) (int, *models.User, models.TestSet, database.Database, bool) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return 0, nil, models.TestSet{}, nil, false
	}

	user := utils.GetCurrentUser(r)

	set, ok := decodeJSON[models.TestSet](w, r)
	if !ok {
		return 0, nil, models.TestSet{}, nil, false
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return 0, nil, models.TestSet{}, nil, false
	}

	set.Name = utils.SanitizeTitle(set.Name)
	set.Description = utils.SanitizeCommentContent(set.Description)

	return workspaceID, user, set, db, true
}

func (h *TestSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, user, set, db, ok := h.decodeTestSetWrite(w, r)
	if !ok {
		return
	}

	id, createdAt, err := repository.NewTestSetRepository(db).Create(workspaceID, &set)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = id
	set.WorkspaceID = workspaceID
	set.CreatedAt = createdAt
	set.UpdatedAt = createdAt

	logAudit(h.db, r, user, logger.ActionTestSetCreate, logger.ResourceTestSet, &id, set.Name)

	respondJSONCreated(w, set)
}

func (h *TestSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, user, set, db, ok := h.decodeTestSetWrite(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	updatedAt, err := repository.NewTestSetRepository(db).Update(id, workspaceID, &set)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = id
	set.WorkspaceID = workspaceID
	set.UpdatedAt = updatedAt

	logAudit(h.db, r, user, logger.ActionTestSetUpdate, logger.ResourceTestSet, &id, set.Name)

	respondJSONOK(w, set)
}

func (h *TestSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestSetRepository(db).Delete(id, workspaceID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionTestSetDelete, logger.ResourceTestSet, &id, "")

	w.WriteHeader(http.StatusNoContent)
}

func (h *TestSetHandler) requireTestSetInWorkspace(w http.ResponseWriter, r *http.Request) (db database.Database, workspaceID, setID int, ok bool) {
	return h.requireResourceInWorkspace(w, r, "test_sets", "id", "test_set")
}

func (h *TestSetHandler) GetTestCases(w http.ResponseWriter, r *http.Request) {
	db, workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	testCases, err := repository.NewTestSetRepository(db).FindTestCases(setID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, testCases)
}

func (h *TestSetHandler) AddTestCase(w http.ResponseWriter, r *http.Request) {
	readDB, workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	var request struct {
		TestCaseID int `json:"test_case_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Verify test case belongs to same workspace
	if !verifyResourceInWorkspace(readDB, w, r, "test_cases", request.TestCaseID, workspaceID, "test_case") {
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestSetRepository(writeDB).AddTestCase(setID, request.TestCaseID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *TestSetHandler) RemoveTestCase(w http.ResponseWriter, r *http.Request) {
	_, _, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	testCaseID, ok := requireIDParam(w, r, "testCaseId")
	if !ok {
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestSetRepository(writeDB).RemoveTestCase(setID, testCaseID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TestSetHandler) GetRuns(w http.ResponseWriter, r *http.Request) {
	db, workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	runs, err := repository.NewTestSetRepository(db).FindRuns(setID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, runs)
}
