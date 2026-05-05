package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type TestSetHandler struct {
	repo             *repository.TestSetRepository
	workspaceChecker *repository.WorkspaceResourceRepository
	auditor          *logger.Auditor
}

func NewTestSetHandlerWithPool(
	repo *repository.TestSetRepository,
	workspaceChecker *repository.WorkspaceResourceRepository,
	auditor *logger.Auditor,
) *TestSetHandler {
	return &TestSetHandler{
		repo:             repo,
		workspaceChecker: workspaceChecker,
		auditor:          auditor,
	}
}

func (h *TestSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	sets, err := h.repo.FindAllWithStats(workspaceID)
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

	set, err := h.repo.FindByID(id, workspaceID)
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

// decodeTestSetWrite extracts the workspace ID, current user, decoded+sanitized TestSet.
// Returns false if any step fails (an error response will already have been written).
func (h *TestSetHandler) decodeTestSetWrite(w http.ResponseWriter, r *http.Request) (int, *models.User, models.TestSet, bool) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return 0, nil, models.TestSet{}, false
	}

	user := utils.GetCurrentUser(r)

	set, ok := decodeJSON[models.TestSet](w, r)
	if !ok {
		return 0, nil, models.TestSet{}, false
	}

	set.Name = utils.SanitizeTitle(set.Name)
	set.Description = utils.SanitizeCommentContent(set.Description)

	return workspaceID, user, set, true
}

func (h *TestSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, user, set, ok := h.decodeTestSetWrite(w, r)
	if !ok {
		return
	}

	id, createdAt, err := h.repo.Create(workspaceID, &set)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = id
	set.WorkspaceID = workspaceID
	set.CreatedAt = createdAt
	set.UpdatedAt = createdAt

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTestSetCreate, logger.ResourceTestSet, &id, set.Name)
	}

	respondJSONCreated(w, set)
}

func (h *TestSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, user, set, ok := h.decodeTestSetWrite(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	updatedAt, err := h.repo.Update(id, workspaceID, &set)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = id
	set.WorkspaceID = workspaceID
	set.UpdatedAt = updatedAt

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTestSetUpdate, logger.ResourceTestSet, &id, set.Name)
	}

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

	if !verifyResourceInWorkspace(h.workspaceChecker, w, r, "test_sets", id, workspaceID, "test_set") {
		return
	}

	if err := h.repo.Delete(id, workspaceID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTestSetDelete, logger.ResourceTestSet, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// requireTestSetInWorkspace parses workspaceId+id and verifies the test_set
// belongs to the workspace. Returns workspaceID, setID, ok.
func (h *TestSetHandler) requireTestSetInWorkspace(w http.ResponseWriter, r *http.Request) (workspaceID, setID int, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	setID, ok = requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !verifyResourceInWorkspace(h.workspaceChecker, w, r, "test_sets", setID, workspaceID, "test_set") {
		ok = false
		return
	}
	return workspaceID, setID, true
}

func (h *TestSetHandler) GetTestCases(w http.ResponseWriter, r *http.Request) {
	workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	testCases, err := h.repo.FindTestCases(setID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, testCases)
}

func (h *TestSetHandler) AddTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
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
	if !verifyResourceInWorkspace(h.workspaceChecker, w, r, "test_cases", request.TestCaseID, workspaceID, "test_case") {
		return
	}

	if err := h.repo.AddTestCase(setID, request.TestCaseID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *TestSetHandler) RemoveTestCase(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	testCaseID, ok := requireIDParam(w, r, "testCaseId")
	if !ok {
		return
	}

	if err := h.repo.RemoveTestCase(setID, testCaseID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TestSetHandler) GetRuns(w http.ResponseWriter, r *http.Request) {
	workspaceID, setID, ok := h.requireTestSetInWorkspace(w, r)
	if !ok {
		return
	}

	runs, err := h.repo.FindRuns(setID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, runs)
}
