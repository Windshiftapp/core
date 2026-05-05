package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TestCaseHandler struct {
	service *services.TestCaseService
	auditor *logger.Auditor
}

func NewTestCaseHandlerWithPool(service *services.TestCaseService, auditor *logger.Auditor) *TestCaseHandler {
	return &TestCaseHandler{
		service: service,
		auditor: auditor,
	}
}

// GetAllTestCases returns all test cases with optional folder filtering
func (h *TestCaseHandler) GetAllTestCases(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	allParam := r.URL.Query().Get("all")
	folderIDParam := r.URL.Query().Get("folder_id")

	// Build list params
	params := services.TestCaseListParams{
		WorkspaceID: workspaceID,
		All:         allParam == "true",
	}

	if folderIDParam != "" && folderIDParam != "null" {
		var folderID int
		var err error
		folderID, err = strconv.Atoi(folderIDParam)
		if err != nil {
			respondInvalidID(w, r, "folder_id")
			return
		}
		params.FolderID = &folderID
	}

	testCases, err := h.service.List(params)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, testCases)
}

// GetTestCase returns a single test case
func (h *TestCaseHandler) GetTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	testCase, err := h.service.GetByID(id, workspaceID)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_case")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, testCase)
}

// CreateTestCase creates a new test case
func (h *TestCaseHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	var input struct {
		Title             string `json:"title"`
		Preconditions     string `json:"preconditions"`
		Priority          string `json:"priority"`
		Status            string `json:"status"`
		EstimatedDuration int    `json:"estimated_duration"`
		FolderID          *int   `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	input.Title = utils.SanitizeTitle(input.Title)
	input.Preconditions = utils.SanitizeCommentContent(input.Preconditions)

	if input.Title == "" {
		respondValidationError(w, r, "Test case title is required")
		return
	}

	testCase, err := h.service.Create(workspaceID, services.TestCaseCreateRequest{
		Title:             input.Title,
		Preconditions:     input.Preconditions,
		Priority:          input.Priority,
		Status:            input.Status,
		EstimatedDuration: input.EstimatedDuration,
		FolderID:          input.FolderID,
	})
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	h.auditor.Log(r, user, logger.ActionTestCaseCreate, logger.ResourceTestCase, &testCase.ID, testCase.Title)

	respondJSONCreated(w, testCase)
}

// UpdateTestCase updates an existing test case
func (h *TestCaseHandler) UpdateTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	var input struct {
		Title             string `json:"title"`
		Preconditions     string `json:"preconditions"`
		Priority          string `json:"priority"`
		Status            string `json:"status"`
		EstimatedDuration int    `json:"estimated_duration"`
		FolderID          *int   `json:"folder_id"`
		SortOrder         int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	input.Title = utils.SanitizeTitle(input.Title)
	input.Preconditions = utils.SanitizeCommentContent(input.Preconditions)

	if input.Title == "" {
		respondValidationError(w, r, "Test case title is required")
		return
	}

	testCase, err := h.service.Update(id, workspaceID, services.TestCaseUpdateRequest{
		Title:             input.Title,
		Preconditions:     input.Preconditions,
		Priority:          input.Priority,
		Status:            input.Status,
		EstimatedDuration: input.EstimatedDuration,
		FolderID:          input.FolderID,
		SortOrder:         input.SortOrder,
	})
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_case")
		} else {
			respondValidationError(w, r, err.Error())
		}
		return
	}

	h.auditor.Log(r, user, logger.ActionTestCaseUpdate, logger.ResourceTestCase, &testCase.ID, testCase.Title)

	respondJSONOK(w, testCase)
}

// DeleteTestCase deletes a test case
func (h *TestCaseHandler) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := requireWorkspaceIDAndID(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(id, workspaceID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_case")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	h.auditor.Log(r, user, logger.ActionTestCaseDelete, logger.ResourceTestCase, &id, "")

	w.WriteHeader(http.StatusNoContent)
}

// MoveTestCase moves a test case to a different folder
func (h *TestCaseHandler) MoveTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var moveData struct {
		FolderID  *int `json:"folder_id"`
		SortOrder int  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&moveData); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if err := h.service.Move(id, workspaceID, moveData.FolderID, moveData.SortOrder); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_case")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, map[string]bool{"success": true})
}

// ReorderTestCases updates the sort order of multiple test cases within a folder
func (h *TestCaseHandler) ReorderTestCases(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	var reorderData struct {
		FolderID    *int  `json:"folder_id"`
		TestCaseIDs []int `json:"test_case_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reorderData); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if err := h.service.Reorder(workspaceID, reorderData.TestCaseIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]bool{"success": true})
}

// requireTestCaseInWorkspace extracts and validates workspaceId and testCaseId from route params,
// and verifies that the test case belongs to the workspace. Returns false if any step fails
// (the appropriate error response will already have been written).
func (h *TestCaseHandler) requireTestCaseInWorkspace(w http.ResponseWriter, r *http.Request) (workspaceID, testCaseID int, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	testCaseID, ok = requireIDParam(w, r, "testCaseId")
	if !ok {
		return
	}
	exists, err := h.service.Exists(testCaseID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, 0, false
	}
	if !exists {
		respondNotFound(w, r, "test_case")
		return 0, 0, false
	}
	return workspaceID, testCaseID, true
}

// Test Step Handlers

// GetTestSteps returns all test steps for a test case
func (h *TestCaseHandler) GetTestSteps(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	steps, err := h.service.GetSteps(testCaseID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, steps)
}

// CreateTestStep creates a new test step
func (h *TestCaseHandler) CreateTestStep(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	var input struct {
		Action   string `json:"action"`
		Data     string `json:"data"`
		Expected string `json:"expected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if input.Action == "" {
		respondValidationError(w, r, "Test step action is required")
		return
	}

	step, err := h.service.CreateStep(testCaseID, services.TestStepCreateRequest{
		Action:   input.Action,
		Data:     input.Data,
		Expected: input.Expected,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, step)
}

// UpdateTestStep updates an existing test step
func (h *TestCaseHandler) UpdateTestStep(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	stepID, ok := requireIDParam(w, r, "stepId")
	if !ok {
		return
	}

	var input struct {
		StepNumber int    `json:"step_number"`
		Action     string `json:"action"`
		Data       string `json:"data"`
		Expected   string `json:"expected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if input.Action == "" {
		respondValidationError(w, r, "Test step action is required")
		return
	}

	step, err := h.service.UpdateStep(stepID, testCaseID, services.TestStepUpdateRequest{
		StepNumber: input.StepNumber,
		Action:     input.Action,
		Data:       input.Data,
		Expected:   input.Expected,
	})
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_step")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, step)
}

// DeleteTestStep deletes a test step
func (h *TestCaseHandler) DeleteTestStep(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	stepID, ok := requireIDParam(w, r, "stepId")
	if !ok {
		return
	}

	if err := h.service.DeleteStep(stepID, testCaseID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_step")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderTestSteps updates the step order of multiple test steps
func (h *TestCaseHandler) ReorderTestSteps(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	var reorderData struct {
		StepIDs []int `json:"step_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reorderData); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if err := h.service.ReorderSteps(testCaseID, reorderData.StepIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]bool{"success": true})
}

// GetAllTestLabels returns all available test labels for a workspace
func (h *TestCaseHandler) GetAllTestLabels(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	labels, err := h.service.GetAllLabels(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// testLabelInput represents the decoded and sanitized input for creating/updating a test label.
type testLabelInput struct {
	Name        string
	Color       string
	Description string
}

// decodeTestLabelInput decodes, sanitizes, and validates the label input from the request body.
// Returns the input and true on success; writes an error response and returns false on failure.
func decodeTestLabelInput(w http.ResponseWriter, r *http.Request) (testLabelInput, bool) {
	var raw struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return testLabelInput{}, false
	}

	raw.Name = utils.SanitizeName(raw.Name)
	raw.Description = utils.SanitizeDescription(raw.Description)

	if raw.Name == "" {
		respondValidationError(w, r, "Label name is required")
		return testLabelInput{}, false
	}

	return testLabelInput{Name: raw.Name, Color: raw.Color, Description: raw.Description}, true
}

// CreateTestLabel creates a new test label
func (h *TestCaseHandler) CreateTestLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	input, ok := decodeTestLabelInput(w, r)
	if !ok {
		return
	}

	label, err := h.service.CreateLabel(workspaceID, services.TestLabelCreateRequest{
		Name:        input.Name,
		Color:       input.Color,
		Description: input.Description,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, label)
}

// UpdateTestLabel updates an existing test label
func (h *TestCaseHandler) UpdateTestLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	labelID, ok := requireIDParam(w, r, "labelId")
	if !ok {
		return
	}

	input, ok := decodeTestLabelInput(w, r)
	if !ok {
		return
	}

	label, err := h.service.UpdateLabel(labelID, workspaceID, services.TestLabelUpdateRequest{
		Name:        input.Name,
		Color:       input.Color,
		Description: input.Description,
	})
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "label")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, label)
}

// DeleteTestLabel deletes a test label
func (h *TestCaseHandler) DeleteTestLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	labelID, ok := requireIDParam(w, r, "labelId")
	if !ok {
		return
	}

	if err := h.service.DeleteLabel(labelID, workspaceID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestCaseLabels returns all labels for a specific test case
func (h *TestCaseHandler) GetTestCaseLabels(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	labels, err := h.service.GetLabelsForTestCase(testCaseID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// AddTestCaseLabel adds a label to a test case
func (h *TestCaseHandler) AddTestCaseLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	var data struct {
		LabelID int `json:"label_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if err := h.service.AddLabelToTestCase(testCaseID, data.LabelID, workspaceID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "label")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// RemoveTestCaseLabel removes a label from a test case
func (h *TestCaseHandler) RemoveTestCaseLabel(w http.ResponseWriter, r *http.Request) {
	_, testCaseID, ok := h.requireTestCaseInWorkspace(w, r)
	if !ok {
		return
	}

	labelID, ok := requireIDParam(w, r, "labelId")
	if !ok {
		return
	}

	if err := h.service.RemoveLabelFromTestCase(testCaseID, labelID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestCaseConnections returns related sets, templates, and executions for a test case
func (h *TestCaseHandler) GetTestCaseConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify test case belongs to workspace
	exists, err := h.service.Exists(id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "test_case")
		return
	}

	connections, err := h.service.GetConnections(id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, connections)
}
