package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TestRunHandler struct {
	*BaseHandler
	service *services.TestRunService
}

func NewTestRunHandlerWithPool(db database.Database) *TestRunHandler {
	return &TestRunHandler{
		BaseHandler: NewBaseHandler(db),
		service:     services.NewTestRunService(db),
	}
}

func (h *TestRunHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	// Build filters from query params
	filters := services.TestRunListFilters{
		IncludeEnded: true, // By default show all runs
	}

	assigneeFilter := r.URL.Query().Get("assignee_id")
	if assigneeFilter == "unassigned" {
		filters.Unassigned = true
	} else if assigneeFilter != "" {
		assigneeID, _ := strconv.Atoi(assigneeFilter)
		filters.AssigneeID = &assigneeID
	}

	runs, err := h.service.List(workspaceID, filters)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, runs)
}

func (h *TestRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	run, err := h.service.GetByID(id, workspaceID)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, run)
}

func (h *TestRunHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	var input struct {
		Name       string `json:"name"`
		TemplateID int    `json:"template_id"`
		SetID      int    `json:"set_id"`
		AssigneeID *int   `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	input.Name = utils.SanitizeTitle(input.Name)

	run, err := h.service.Create(workspaceID, services.TestRunCreateRequest{
		Name:       input.Name,
		TemplateID: input.TemplateID,
		SetID:      input.SetID,
		AssigneeID: input.AssigneeID,
	})
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	logAudit(h.db, r, user, logger.ActionTestRunCreate, logger.ResourceTestRun, &run.ID, run.Name)

	respondJSONCreated(w, run)
}

func (h *TestRunHandler) End(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Complete(id, workspaceID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Update updates a test run (supports updating assignee)
func (h *TestRunHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		Name       string `json:"name"`
		AssigneeID *int   `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	input.Name = utils.SanitizeTitle(input.Name)

	_, err := h.service.Update(id, workspaceID, services.TestRunUpdateRequest{
		Name:       input.Name,
		AssigneeID: input.AssigneeID,
	})
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondValidationError(w, r, err.Error())
		}
		return
	}

	logAudit(h.db, r, user, logger.ActionTestRunUpdate, logger.ResourceTestRun, &id, "")

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// requireTestRunAccess parses workspaceID and runID from path params,
// verifies the test run belongs to the workspace, and returns the read DB.
func (h *TestRunHandler) requireTestRunAccess(w http.ResponseWriter, r *http.Request) (workspaceID, runID int, db database.Database, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return 0, 0, nil, false
	}

	runID, ok = requireIDParam(w, r, "id")
	if !ok {
		return 0, 0, nil, false
	}

	exists, existsErr := h.service.Exists(runID, workspaceID)
	if existsErr != nil {
		respondInternalError(w, r, existsErr)
		return 0, 0, nil, false
	}
	if !exists {
		respondNotFound(w, r, "test_run")
		return 0, 0, nil, false
	}

	db, ok = h.requireReadDB(w, r)
	if !ok {
		return 0, 0, nil, false
	}

	return workspaceID, runID, db, true
}

func (h *TestRunHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	workspaceID, runID, db, ok := h.requireTestRunAccess(w, r)
	if !ok {
		return
	}

	type ResultWithTestCase struct {
		models.TestResult
		TestCaseTitle string `json:"test_case_title"`
	}

	rows, err := repository.NewTestRunRepository(db).FindResultsWithTestCase(runID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	results := make([]ResultWithTestCase, 0, len(rows))
	for _, row := range rows {
		results = append(results, ResultWithTestCase{
			TestResult:    row.TestResult,
			TestCaseTitle: row.TestCaseTitle,
		})
	}

	respondJSONOK(w, results)
}

func (h *TestRunHandler) UpdateResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	runID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	resultID, ok := requireIDParam(w, r, "resultId")
	if !ok {
		return
	}

	// Verify test run belongs to workspace
	exists, err := h.service.Exists(runID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "test_run")
		return
	}

	var input struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	// Sanitize user input to prevent XSS
	input.ActualResult = utils.SanitizeCommentContent(input.ActualResult)
	input.Notes = utils.SanitizeCommentContent(input.Notes)

	if err := h.service.UpdateResult(resultID, services.TestResultUpdateRequest{
		Status:       input.Status,
		ActualResult: input.ActualResult,
		Notes:        input.Notes,
	}); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *TestRunHandler) GetBySet(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	setID, ok := requireIDParam(w, r, "setId")
	if !ok {
		return
	}

	// Use service to filter by set
	runs, err := h.service.List(workspaceID, services.TestRunListFilters{
		SetID:        &setID,
		IncludeEnded: true,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, runs)
}

// UpdateStepResult updates or creates a step result for a test execution
func (h *TestRunHandler) UpdateStepResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	runID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	stepID, ok := requireIDParam(w, r, "stepId")
	if !ok {
		return
	}

	var update struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
		ItemID       *int   `json:"item_id,omitempty"`
	}
	var err error
	if err = json.NewDecoder(r.Body).Decode(&update); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	// Sanitize user input to prevent XSS
	update.ActualResult = utils.SanitizeCommentContent(update.ActualResult)
	update.Notes = utils.SanitizeCommentContent(update.Notes)

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify item belongs to same workspace if provided
	if update.ItemID != nil {
		itemWsID, err := repository.NewItemRepository(readDB).GetWorkspaceID(*update.ItemID)
		if err != nil || itemWsID != workspaceID {
			respondNotFound(w, r, "item")
			return
		}
	}

	readRepo := repository.NewTestRunRepository(readDB)
	testResultID, err := readRepo.FindTestResultIDForStep(runID, stepID, workspaceID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "test_result")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	existingID, findErr := readRepo.FindStepResultID(testResultID, stepID)

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	writeRepo := repository.NewTestRunRepository(writeDB)
	input := repository.StepResultInput{
		TestResultID: testResultID,
		StepID:       stepID,
		Status:       update.Status,
		ActualResult: update.ActualResult,
		Notes:        update.Notes,
		ItemID:       update.ItemID,
	}

	switch {
	case errors.Is(findErr, repository.ErrNotFound):
		err = writeRepo.CreateStepResult(input)
	case findErr == nil:
		err = writeRepo.UpdateStepResult(existingID, input)
	default:
		err = findErr
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update the parent test case status based on step results
	err = h.updateTestCaseStatus(testResultID)
	if err != nil {
		slog.Warn("failed to update test case status", slog.Any("error", err), slog.Int("test_result_id", testResultID))
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GetStepResults returns all step results for a test run
func (h *TestRunHandler) GetStepResults(w http.ResponseWriter, r *http.Request) {
	workspaceID, runID, db, ok := h.requireTestRunAccess(w, r)
	if !ok {
		return
	}

	rows, err := repository.NewTestRunRepository(db).FindStepResultsForRun(runID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	stepResults := make(map[string]interface{}, len(rows))
	for _, row := range rows {
		compositeKey := fmt.Sprintf("%d_%d", row.TestCaseID, row.StepID)
		stepResults[compositeKey] = map[string]interface{}{
			"step_id":       row.StepID,
			"test_case_id":  row.TestCaseID,
			"status":        row.Status,
			"actual_result": row.ActualResult,
			"notes":         row.Notes,
			"item_id":       row.ItemID,
			"executed_at":   row.ExecutedAt,
		}
	}

	respondJSONOK(w, stepResults)
}

// updateTestCaseStatus updates the test case status based on its step results
func (h *TestRunHandler) updateTestCaseStatus(testResultID int) error {
	readDB, err := h.getReadDB()
	if err != nil {
		return err
	}

	stepStatuses, err := repository.NewTestRunRepository(readDB).FindStepResultStatuses(testResultID)
	if err != nil {
		return err
	}

	// If no step results exist, leave test case as not_run
	if len(stepStatuses) == 0 {
		return nil
	}

	// Determine overall test case status based on step results
	var finalStatus string
	hasBlocked := false
	hasFailed := false
	hasSkipped := false
	allPassed := true

	for _, status := range stepStatuses {
		switch status {
		case "failed":
			hasFailed = true
			allPassed = false
		case "blocked":
			hasBlocked = true
			allPassed = false
		case "skipped":
			hasSkipped = true
			allPassed = false
		case "not_run":
			allPassed = false
		}
	}

	// Priority: failed > blocked > skipped > passed
	switch {
	case hasFailed:
		finalStatus = "failed"
	case hasBlocked:
		finalStatus = "blocked"
	case hasSkipped:
		finalStatus = "skipped"
	case allPassed:
		finalStatus = "passed"
	default:
		finalStatus = "not_run"
	}

	writeDB, err := h.getWriteDB()
	if err != nil {
		return err
	}
	return repository.NewTestRunRepository(writeDB).SetTestResultStatus(testResultID, finalStatus)
}

// Delete removes a test run and all associated results
func (h *TestRunHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user := utils.GetCurrentUser(r)

	if err := h.service.Delete(id, workspaceID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	logAudit(h.db, r, user, logger.ActionTestRunDelete, logger.ResourceTestRun, &id, "")

	w.WriteHeader(http.StatusOK)
}

// LinkItemToTestResult links a work item to a test result
func (h *TestRunHandler) LinkItemToTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	resultID, ok := requireIDParam(w, r, "resultId")
	if !ok {
		return
	}

	var data struct {
		ItemID int `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify item belongs to same workspace
	itemWsID, err := repository.NewItemRepository(readDB).GetWorkspaceID(data.ItemID)
	if err != nil || itemWsID != workspaceID {
		respondNotFound(w, r, "item")
		return
	}

	// Verify test result belongs to workspace (via test_runs)
	owned, err := repository.NewTestRunRepository(readDB).TestResultBelongsToWorkspace(resultID, workspaceID)
	if err != nil || !owned {
		respondNotFound(w, r, "test_result")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestRunRepository(writeDB).LinkResultToItem(resultID, data.ItemID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// UnlinkItemFromTestResult removes item link from test result
func (h *TestRunHandler) UnlinkItemFromTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	resultID, ok := requireIDParam(w, r, "resultId")
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	owned, err := repository.NewTestRunRepository(readDB).TestResultBelongsToWorkspace(resultID, workspaceID)
	if err != nil || !owned {
		respondNotFound(w, r, "test_result")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestRunRepository(writeDB).UnlinkResultFromItem(resultID, itemID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestResultItems gets all linked items for a test result
func (h *TestRunHandler) GetTestResultItems(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	resultID, ok := requireIDParam(w, r, "resultId")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	items, err := repository.NewItemRepository(db).ListItemsLinkedToTestResult(resultID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, items)
}
