package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

)

type TestRunHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewTestRunHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestRunHandler {
	return &TestRunHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

func (h *TestRunHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	// Support filtering by assignee_id
	assigneeFilter := r.URL.Query().Get("assignee_id")

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	var rows *sql.Rows
	if assigneeFilter != "" {
		if assigneeFilter == "unassigned" {
			rows, err = db.Query(`
				SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
				       tr.started_at, tr.ended_at, tr.created_at,
				       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
				       COALESCE(u.email, '') as assignee_email,
				       COALESCE(u.avatar, '') as assignee_avatar
				FROM test_runs tr
				LEFT JOIN users u ON tr.assignee_id = u.id
				WHERE tr.workspace_id = ? AND tr.assignee_id IS NULL
				ORDER BY tr.id DESC
			`, workspaceID)
		} else {
			assigneeID, _ := strconv.Atoi(assigneeFilter)
			rows, err = db.Query(`
				SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
				       tr.started_at, tr.ended_at, tr.created_at,
				       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
				       COALESCE(u.email, '') as assignee_email,
				       COALESCE(u.avatar, '') as assignee_avatar
				FROM test_runs tr
				LEFT JOIN users u ON tr.assignee_id = u.id
				WHERE tr.workspace_id = ? AND tr.assignee_id = ?
				ORDER BY tr.id DESC
			`, workspaceID, assigneeID)
		}
	} else {
		rows, err = db.Query(`
			SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
			       tr.started_at, tr.ended_at, tr.created_at,
			       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			       COALESCE(u.email, '') as assignee_email,
			       COALESCE(u.avatar, '') as assignee_avatar
			FROM test_runs tr
			LEFT JOIN users u ON tr.assignee_id = u.id
			WHERE tr.workspace_id = ?
			ORDER BY tr.id DESC
		`, workspaceID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		var templateID, assigneeID sql.NullInt64
		var assigneeName, assigneeEmail, assigneeAvatar string
		err := rows.Scan(&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
			&run.StartedAt, &run.EndedAt, &run.CreatedAt,
			&assigneeName, &assigneeEmail, &assigneeAvatar)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		run.TemplateID = utils.NullInt64ToInt(templateID, 0)
		run.AssigneeID = utils.NullInt64ToPtr(assigneeID)
		if run.AssigneeID != nil {
			run.AssigneeName = assigneeName
			run.AssigneeEmail = assigneeEmail
			run.AssigneeAvatar = assigneeAvatar
		}
		runs = append(runs, run)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func (h *TestRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	var run models.TestRun
	var templateID, assigneeID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar string
	err = db.QueryRow(`
		SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
		       tr.started_at, tr.ended_at, tr.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       COALESCE(u.email, '') as assignee_email,
		       COALESCE(u.avatar, '') as assignee_avatar
		FROM test_runs tr
		LEFT JOIN users u ON tr.assignee_id = u.id
		WHERE tr.id = ? AND tr.workspace_id = ?
	`, id, workspaceID).Scan(&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
		&run.StartedAt, &run.EndedAt, &run.CreatedAt,
		&assigneeName, &assigneeEmail, &assigneeAvatar)

	run.TemplateID = utils.NullInt64ToInt(templateID, 0)
	run.AssigneeID = utils.NullInt64ToPtr(assigneeID)
	if run.AssigneeID != nil {
		run.AssigneeName = assigneeName
		run.AssigneeEmail = assigneeEmail
		run.AssigneeAvatar = assigneeAvatar
	}

	if err == sql.ErrNoRows {
		http.Error(w, "Test run not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

func (h *TestRunHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var run models.TestRun
	if err := json.NewDecoder(r.Body).Decode(&run); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	if run.SetID > 0 {
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", run.SetID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, "Test set not found in workspace", http.StatusNotFound)
			return
		}
	}

	// Validate assignee belongs to workspace if provided
	if run.AssigneeID != nil && *run.AssigneeID > 0 {
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM user_workspace_roles WHERE user_id = ? AND workspace_id = ?
		`, *run.AssigneeID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, "Assignee is not a member of this workspace", http.StatusBadRequest)
			return
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	var runID int64

	// Support optional template_id
	var templateIDPtr *int
	if run.TemplateID > 0 {
		templateIDPtr = &run.TemplateID
	}

	err = tx.QueryRow(`
		INSERT INTO test_runs (workspace_id, template_id, set_id, name, assignee_id, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, templateIDPtr, run.SetID, run.Name, run.AssigneeID, now, now).Scan(&runID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := tx.Query(`
		SELECT test_case_id FROM set_test_cases WHERE set_id = ?
	`, run.SetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var testCaseID int
		if err := rows.Scan(&testCaseID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
			VALUES (?, ?, 'not_run', ?, ?)
		`, runID, testCaseID, time.Now(), time.Now())

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	run.ID = int(runID)
	run.WorkspaceID = workspaceID
	run.StartedAt = time.Now()
	run.CreatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(run)
}

func (h *TestRunHandler) End(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	db, ok := h.requireWriteDB(w)
	if !ok {
		return
	}

	now := time.Now()
	_, err = db.Exec(`
		UPDATE test_runs
		SET ended_at = ?
		WHERE id = ? AND workspace_id = ?
	`, now, id, workspaceID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Update updates a test run (supports updating assignee)
func (h *TestRunHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var update struct {
		Name       string `json:"name"`
		AssigneeID *int   `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate assignee belongs to workspace if provided
	if update.AssigneeID != nil && *update.AssigneeID > 0 {
		readDB, ok := h.requireReadDB(w)
		if !ok {
			return
		}
		var count int
		err = readDB.QueryRow(`
			SELECT COUNT(*) FROM user_workspace_roles WHERE user_id = ? AND workspace_id = ?
		`, *update.AssigneeID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, "Assignee is not a member of this workspace", http.StatusBadRequest)
			return
		}
	}

	writeDB, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	_, err = writeDB.Exec(`
		UPDATE test_runs
		SET name = COALESCE(NULLIF(?, ''), name), assignee_id = ?
		WHERE id = ? AND workspace_id = ?
	`, update.Name, update.AssigneeID, id, workspaceID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *TestRunHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify test run belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_runs WHERE id = ? AND workspace_id = ?", runID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test run not found", http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ResultWithTestCase struct {
		models.TestResult
		TestCaseTitle string `json:"test_case_title"`
	}

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	results := make([]ResultWithTestCase, 0)
	for rows.Next() {
		var r ResultWithTestCase
		var actualResult, notes sql.NullString
		var executedAt sql.NullTime

		err := rows.Scan(&r.ID, &r.RunID, &r.TestCaseID, &r.Status, &actualResult, &notes, &executedAt,
			&r.CreatedAt, &r.UpdatedAt, &r.TestCaseTitle)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Handle NULL values
		if actualResult.Valid {
			r.ActualResult = actualResult.String
		}
		if notes.Valid {
			r.Notes = notes.String
		}
		if executedAt.Valid {
			r.ExecutedAt = &executedAt.Time
		}

		results = append(results, r)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (h *TestRunHandler) UpdateResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		http.Error(w, "Invalid result ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	readDB, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify test run belongs to workspace
	var count int
	err = readDB.QueryRow("SELECT COUNT(*) FROM test_runs WHERE id = ? AND workspace_id = ?", runID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test run not found", http.StatusNotFound)
		return
	}

	var update struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeDB, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	now := time.Now()
	_, err = writeDB.Exec(`
		UPDATE test_results
		SET status = ?, actual_result = ?, notes = ?, executed_at = ?, updated_at = ?
		WHERE id = ? AND run_id = ?
	`, update.Status, update.ActualResult, update.Notes, now, now, resultID, runID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *TestRunHandler) GetBySet(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", setID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test set not found", http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
		       tr.started_at, tr.ended_at, tr.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       COALESCE(u.email, '') as assignee_email,
		       COALESCE(u.avatar, '') as assignee_avatar
		FROM test_runs tr
		LEFT JOIN users u ON tr.assignee_id = u.id
		WHERE tr.set_id = ? AND tr.workspace_id = ?
		ORDER BY tr.id DESC
	`, setID, workspaceID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		var templateID, assigneeID sql.NullInt64
		var assigneeName, assigneeEmail, assigneeAvatar string
		err := rows.Scan(&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
			&run.StartedAt, &run.EndedAt, &run.CreatedAt,
			&assigneeName, &assigneeEmail, &assigneeAvatar)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		run.TemplateID = utils.NullInt64ToInt(templateID, 0)
		run.AssigneeID = utils.NullInt64ToPtr(assigneeID)
		if run.AssigneeID != nil {
			run.AssigneeName = assigneeName
			run.AssigneeEmail = assigneeEmail
			run.AssigneeAvatar = assigneeAvatar
		}
		runs = append(runs, run)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// UpdateStepResult updates or creates a step result for a test execution
func (h *TestRunHandler) UpdateStepResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var update struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
		ItemID       *int   `json:"item_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	readDB, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify item belongs to same workspace if provided
	if update.ItemID != nil {
		var count int
		err = readDB.QueryRow("SELECT COUNT(*) FROM items WHERE id = ? AND workspace_id = ?", *update.ItemID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, "Item not found in workspace", http.StatusNotFound)
			return
		}
	}

	// First, get the test result ID for this run and step
	// Make sure we get the correct test case that actually owns this step
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if step result already exists
	var existingID int
	err = readDB.QueryRow(`
		SELECT id FROM test_step_results
		WHERE test_result_id = ? AND test_step_id = ?
	`, testResultID, stepID).Scan(&existingID)

	writeDB, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	now := time.Now()
	if err == sql.ErrNoRows {
		// Create new step result
		_, err = writeDB.Exec(`
			INSERT INTO test_step_results
			(test_result_id, test_step_id, status, actual_result, notes, item_id, executed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, testResultID, stepID, update.Status, update.ActualResult, update.Notes, update.ItemID, now, now, now)
	} else if err == nil {
		// Update existing step result
		_, err = writeDB.Exec(`
			UPDATE test_step_results
			SET status = ?, actual_result = ?, notes = ?, item_id = ?, executed_at = ?, updated_at = ?
			WHERE id = ?
		`, update.Status, update.ActualResult, update.Notes, update.ItemID, now, now, existingID)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update the parent test case status based on step results
	err = h.updateTestCaseStatus(testResultID)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Warning: Failed to update test case status: %v\n", err)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GetStepResults returns all step results for a test run
func (h *TestRunHandler) GetStepResults(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify test run belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_runs WHERE id = ? AND workspace_id = ?", runID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test run not found", http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stepResults := make(map[string]interface{})
	for rows.Next() {
		var stepID, testCaseID int
		var status, actualResult, notes, testCaseTitle string
		var itemID *int
		var executedAt *time.Time

		err := rows.Scan(&stepID, &status, &actualResult, &notes, &itemID, &executedAt, &testCaseID, &testCaseTitle)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Use a composite key to avoid conflicts between test cases
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
	json.NewEncoder(w).Encode(stepResults)
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
	defer rows.Close()

	var stepStatuses []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
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
	if hasFailed {
		finalStatus = "failed"
	} else if hasBlocked {
		finalStatus = "blocked"
	} else if hasSkipped {
		finalStatus = "skipped"
	} else if allPassed {
		finalStatus = "passed"
	} else {
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
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestManage, h.permissionService) {
		return
	}

	db, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	_, err = db.Exec("DELETE FROM test_runs WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// LinkItemToTestResult links a work item to a test result
func (h *TestRunHandler) LinkItemToTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		http.Error(w, "Invalid result ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	var data struct {
		ItemID int `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	readDB, ok := h.requireReadDB(w)
	if !ok {
		return
	}

	// Verify item belongs to same workspace
	var count int
	err = readDB.QueryRow("SELECT COUNT(*) FROM items WHERE id = ? AND workspace_id = ?", data.ItemID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Item not found in workspace", http.StatusNotFound)
		return
	}

	// Verify test result belongs to workspace (via test_runs)
	err = readDB.QueryRow(`
		SELECT COUNT(*) FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.id = ? AND run.workspace_id = ?
	`, resultID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test result not found", http.StatusNotFound)
		return
	}

	writeDB, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	// Insert into test_result_items
	_, err = writeDB.Exec(`
		INSERT INTO test_result_items (test_result_id, item_id, created_at)
		VALUES (?, ?, ?)
	`, resultID, data.ItemID, time.Now())

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// UnlinkItemFromTestResult removes item link from test result
func (h *TestRunHandler) UnlinkItemFromTestResult(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		http.Error(w, "Invalid result ID", http.StatusBadRequest)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestExecute, h.permissionService) {
		return
	}

	readDB, ok := h.requireReadDB(w)
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
		http.Error(w, "Test result not found", http.StatusNotFound)
		return
	}

	writeDB, ok := h.requireWriteDB(w)
	if !ok {
		return
	}
	_, err = writeDB.Exec(`
		DELETE FROM test_result_items
		WHERE test_result_id = ? AND item_id = ?
	`, resultID, itemID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestResultItems gets all linked items for a test result
func (h *TestRunHandler) GetTestResultItems(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	resultID, err := strconv.Atoi(r.PathValue("resultId"))
	if err != nil {
		http.Error(w, "Invalid result ID", http.StatusBadRequest)
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionTestView, h.permissionService) {
		return
	}

	db, ok := h.requireReadDB(w)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		err := rows.Scan(&item.ID, &item.WorkspaceItemNumber, &item.Title, &item.ItemTypeID, &item.StatusID, &item.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
