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
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TestRunHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
	service           *services.TestRunService
}

func NewTestRunHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestRunHandler {
	return &TestRunHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
		service:           services.NewTestRunService(db),
	}
}

func (h *TestRunHandler) GetAll(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

func (h *TestRunHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	run, err := h.service.GetByID(id, workspaceID)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}

func (h *TestRunHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var input struct {
		Name       string `json:"name"`
		TemplateID int    `json:"template_id"`
		SetID      int    `json:"set_id"`
		AssigneeID *int   `json:"assignee_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

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

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionTestRunCreate,
		ResourceType: logger.ResourceTestRun,
		ResourceID:   &run.ID,
		ResourceName: run.Name,
		Success:      true,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(run)
}

func (h *TestRunHandler) End(w http.ResponseWriter, r *http.Request) {
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

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
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

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var input struct {
		Name       string `json:"name"`
		AssigneeID *int   `json:"assignee_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	_, err = h.service.Update(id, workspaceID, services.TestRunUpdateRequest{
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

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionTestRunUpdate,
		ResourceType: logger.ResourceTestRun,
		ResourceID:   &id,
		Success:      true,
	})

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *TestRunHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
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

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT tr.id, tr.run_id, tr.test_case_id, tr.status, tr.actual_result, tr.notes, tr.executed_at, tr.created_at, tr.updated_at,
		       tc.title
		FROM test_results tr
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.run_id = ? AND run.workspace_id = ?
		ORDER BY tc.id
	`, runID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type ResultWithTestCase struct {
		models.TestResult
		TestCaseTitle string `json:"test_case_title"`
	}

	results := make([]ResultWithTestCase, 0)
	for rows.Next() {
		var res ResultWithTestCase
		var actualResult, notes sql.NullString
		var executedAt sql.NullTime

		err := rows.Scan(&res.ID, &res.RunID, &res.TestCaseID, &res.Status, &actualResult, &notes, &executedAt,
			&res.CreatedAt, &res.UpdatedAt, &res.TestCaseTitle)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if actualResult.Valid {
			res.ActualResult = actualResult.String
		}
		if notes.Valid {
			res.Notes = notes.String
		}
		if executedAt.Valid {
			res.ExecutedAt = &executedAt.Time
		}

		results = append(results, res)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *TestRunHandler) UpdateResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		respondInvalidID(w, r, "resultId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
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
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

// UpdateStepResult updates or creates a step result for a test execution
func (h *TestRunHandler) UpdateStepResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		respondInvalidID(w, r, "stepId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var update struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
		ItemID       *int   `json:"item_id,omitempty"`
	}
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
		var count int
		err = readDB.QueryRow("SELECT COUNT(*) FROM items WHERE id = ? AND workspace_id = ?", *update.ItemID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			respondNotFound(w, r, "item")
			return
		}
	}

	// Get the test result ID for this run and step
	var testResultID int
	err = readDB.QueryRow(`
		SELECT tr.id
		FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_steps ts ON ts.test_case_id = tc.id
		WHERE tr.run_id = ? AND ts.id = ? AND run.workspace_id = ?
		LIMIT 1
	`, runID, stepID, workspaceID).Scan(&testResultID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if step result already exists
	var existingID int
	err = readDB.QueryRow(`
		SELECT id FROM test_step_results
		WHERE test_result_id = ? AND test_step_id = ?
	`, testResultID, stepID).Scan(&existingID)

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}
	now := time.Now()
	switch err {
	case sql.ErrNoRows:
		// Create new step result
		_, err = writeDB.Exec(`
			INSERT INTO test_step_results
			(test_result_id, test_step_id, status, actual_result, notes, item_id, executed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, testResultID, stepID, update.Status, update.ActualResult, update.Notes, update.ItemID, now, now, now)
	case nil:
		// Update existing step result
		_, err = writeDB.Exec(`
			UPDATE test_step_results
			SET status = ?, actual_result = ?, notes = ?, item_id = ?, executed_at = ?, updated_at = ?
			WHERE id = ?
		`, update.Status, update.ActualResult, update.Notes, update.ItemID, now, now, existingID)
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
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
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

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT tsr.test_step_id, tsr.status, tsr.actual_result, tsr.notes, tsr.item_id, tsr.executed_at,
		       tc.id as test_case_id, tc.title as test_case_title
		FROM test_step_results tsr
		JOIN test_results tr ON tsr.test_result_id = tr.id
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.run_id = ? AND run.workspace_id = ?
	`, runID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	stepResults := make(map[string]interface{})
	for rows.Next() {
		var stepID, testCaseID int
		var status, actualResult, notes, testCaseTitle string
		var itemID *int
		var executedAt *time.Time

		err := rows.Scan(&stepID, &status, &actualResult, &notes, &itemID, &executedAt, &testCaseID, &testCaseTitle)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		compositeKey := fmt.Sprintf("%d_%d", testCaseID, stepID)
		stepResults[compositeKey] = map[string]interface{}{
			"step_id":       stepID,
			"test_case_id":  testCaseID,
			"status":        status,
			"actual_result": actualResult,
			"notes":         notes,
			"item_id":       itemID,
			"executed_at":   executedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stepResults)
}

// updateTestCaseStatus updates the test case status based on its step results
func (h *TestRunHandler) updateTestCaseStatus(testResultID int) error {
	readDB, err := h.getReadDB()
	if err != nil {
		return err
	}

	// Get all step results for this test case
	rows, err := readDB.Query(`
		SELECT status FROM test_step_results
		WHERE test_result_id = ?
	`, testResultID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var stepStatuses []string
	for rows.Next() {
		var status string
		if err = rows.Scan(&status); err != nil { //nolint:gocritic // Using = to avoid shadowing err from outer scope
			return err
		}
		stepStatuses = append(stepStatuses, status)
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

	// Update the test result status
	writeDB, err := h.getWriteDB()
	if err != nil {
		return err
	}
	_, err = writeDB.Exec(`
		UPDATE test_results
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, finalStatus, time.Now(), testResultID)

	return err
}

// Delete removes a test run and all associated results
func (h *TestRunHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.Delete(id, workspaceID); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "test_run")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionTestRunDelete,
		ResourceType: logger.ResourceTestRun,
		ResourceID:   &id,
		Success:      true,
	})

	w.WriteHeader(http.StatusOK)
}

// LinkItemToTestResult links a work item to a test result
func (h *TestRunHandler) LinkItemToTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		respondInvalidID(w, r, "resultId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var data struct {
		ItemID int `json:"item_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify item belongs to same workspace
	var count int
	err = readDB.QueryRow("SELECT COUNT(*) FROM items WHERE id = ? AND workspace_id = ?", data.ItemID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "item")
		return
	}

	// Verify test result belongs to workspace (via test_runs)
	err = readDB.QueryRow(`
		SELECT COUNT(*) FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.id = ? AND run.workspace_id = ?
	`, resultID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_result")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = writeDB.Exec(`
		INSERT INTO test_result_items (test_result_id, item_id, created_at)
		VALUES (?, ?, ?)
	`, resultID, data.ItemID, time.Now())

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// UnlinkItemFromTestResult removes item link from test result
func (h *TestRunHandler) UnlinkItemFromTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		respondInvalidID(w, r, "resultId")
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify workspace ownership
	var count int
	err = readDB.QueryRow(`
		SELECT COUNT(*) FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.id = ? AND run.workspace_id = ?
	`, resultID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_result")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = writeDB.Exec(`
		DELETE FROM test_result_items
		WHERE test_result_id = ? AND item_id = ?
	`, resultID, itemID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestResultItems gets all linked items for a test result
func (h *TestRunHandler) GetTestResultItems(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		respondInvalidID(w, r, "resultId")
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
		SELECT i.id, i.workspace_item_number, i.title, i.item_type_id, i.status_id, i.created_at
		FROM items i
		JOIN test_result_items tri ON i.id = tri.item_id
		JOIN test_results tr ON tri.test_result_id = tr.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tri.test_result_id = ? AND run.workspace_id = ?
		ORDER BY tri.created_at DESC
	`

	rows, err := db.Query(query, resultID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	items := make([]models.Item, 0)
	for rows.Next() {
		var item models.Item
		err := rows.Scan(&item.ID, &item.WorkspaceItemNumber, &item.Title, &item.ItemTypeID, &item.StatusID, &item.CreatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}
