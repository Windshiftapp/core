package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

)

type TestCaseHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewTestCaseHandler(db database.Database) *TestCaseHandler {
	// Legacy constructor for backward compatibility
	panic("Use NewTestCaseHandlerWithPool instead")
}

func NewTestCaseHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestCaseHandler {
	return &TestCaseHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

// Valid priority and status values for test cases
var validTestCasePriorities = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
var validTestCaseStatuses = map[string]bool{"active": true, "inactive": true, "draft": true}

// GetAllTestCases returns all test cases with optional folder filtering
func (h *TestCaseHandler) GetAllTestCases(w http.ResponseWriter, r *http.Request) {
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

	allParam := r.URL.Query().Get("all")
	folderIDParam := r.URL.Query().Get("folder_id")

	var query string
	var args []interface{}

	if allParam == "true" {
		// Get ALL test cases in workspace (for test plan picker, etc.)
		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ?
			ORDER BY tf.sort_order, tc.sort_order, tc.title
		`
		args = append(args, workspaceID)
	} else if folderIDParam == "null" || folderIDParam == "" {
		// Get test cases with no folder (root level)
		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ? AND tc.folder_id IS NULL
			ORDER BY tc.sort_order, tc.title
		`
		args = append(args, workspaceID)
	} else {
		// Get test cases for specific folder
		folderID, err := strconv.Atoi(folderIDParam)
		if err != nil {
			http.Error(w, "Invalid folder ID", http.StatusBadRequest)
			return
		}

		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ? AND tc.folder_id = ?
			ORDER BY tc.sort_order, tc.title
		`
		args = append(args, workspaceID, folderID)
	}

	rows, err := h.getReadDB().Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var testCases []models.TestCase
	for rows.Next() {
		var testCase models.TestCase
		var folderName sql.NullString

		err := rows.Scan(
			&testCase.ID, &testCase.WorkspaceID, &testCase.FolderID, &testCase.Title, &testCase.Preconditions,
			&testCase.Priority, &testCase.Status, &testCase.EstimatedDuration,
			&testCase.SortOrder, &testCase.CreatedAt, &testCase.UpdatedAt, &folderName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if folderName.Valid {
			testCase.FolderName = folderName.String
		}

		// Load labels for this test case
		labels, err := h.getTestCaseLabels(testCase.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		testCase.Labels = labels

		testCases = append(testCases, testCase)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testCases)
}

// getTestCaseLabels is a helper function to get labels for a test case
func (h *TestCaseHandler) getTestCaseLabels(testCaseID int) ([]models.TestLabel, error) {
	query := `
		SELECT tl.id, tl.workspace_id, tl.name, tl.color, tl.description, tl.created_at, tl.updated_at
		FROM test_labels tl
		INNER JOIN test_case_labels tcl ON tl.id = tcl.label_id
		WHERE tcl.test_case_id = ?
		ORDER BY tl.name
	`

	rows, err := h.getReadDB().Query(query, testCaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []models.TestLabel
	for rows.Next() {
		var label models.TestLabel
		err := rows.Scan(
			&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
			&label.CreatedAt, &label.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}

	return labels, nil
}

// GetTestCase returns a single test case
func (h *TestCaseHandler) GetTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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
		SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
		       COALESCE(tc.preconditions, '') as preconditions,
		       COALESCE(tc.priority, 'medium') as priority,
		       COALESCE(tc.status, 'active') as status,
		       COALESCE(tc.estimated_duration, 0) as estimated_duration,
		       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
		FROM test_cases tc
		LEFT JOIN test_folders tf ON tc.folder_id = tf.id
		WHERE tc.id = ? AND tc.workspace_id = ?
	`

	var testCase models.TestCase
	var folderName sql.NullString

	err = h.getReadDB().QueryRow(query, id, workspaceID).Scan(
		&testCase.ID, &testCase.WorkspaceID, &testCase.FolderID, &testCase.Title, &testCase.Preconditions,
		&testCase.Priority, &testCase.Status, &testCase.EstimatedDuration,
		&testCase.SortOrder, &testCase.CreatedAt, &testCase.UpdatedAt, &folderName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Test case not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if folderName.Valid {
		testCase.FolderName = folderName.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testCase)
}

// CreateTestCase creates a new test case
func (h *TestCaseHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
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

	var testCase models.TestCase
	if err := json.NewDecoder(r.Body).Decode(&testCase); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if testCase.Title == "" {
		http.Error(w, "Test case title is required", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	testCase.Title = utils.StripHTMLTags(testCase.Title)
	testCase.Preconditions = utils.StripHTMLTags(testCase.Preconditions)

	// Set defaults and validate priority
	if testCase.Priority == "" {
		testCase.Priority = "medium"
	} else if !validTestCasePriorities[testCase.Priority] {
		http.Error(w, "Invalid priority value. Must be: low, medium, high, or critical", http.StatusBadRequest)
		return
	}

	// Set defaults and validate status
	if testCase.Status == "" {
		testCase.Status = "active"
	} else if !validTestCaseStatuses[testCase.Status] {
		http.Error(w, "Invalid status value. Must be: active, inactive, or draft", http.StatusBadRequest)
		return
	}

	// Validate estimated duration is non-negative
	if testCase.EstimatedDuration < 0 {
		http.Error(w, "Estimated duration cannot be negative", http.StatusBadRequest)
		return
	}

	testCase.WorkspaceID = workspaceID

	// Get the highest sort_order within the folder for new test case ordering
	var maxSortOrder sql.NullInt64
	if testCase.FolderID != nil {
		err := h.getReadDB().QueryRow("SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id = ?", workspaceID, *testCase.FolderID).Scan(&maxSortOrder)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		err := h.getReadDB().QueryRow("SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id IS NULL", workspaceID).Scan(&maxSortOrder)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	testCase.SortOrder = int(maxSortOrder.Int64) + 1000 // Leave room for reordering
	testCase.CreatedAt = time.Now()
	testCase.UpdatedAt = time.Now()

	query := `
		INSERT INTO test_cases (workspace_id, folder_id, title, preconditions, priority, status, estimated_duration, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err = h.getWriteDB().QueryRow(query, testCase.WorkspaceID, testCase.FolderID, testCase.Title, testCase.Preconditions,
		testCase.Priority, testCase.Status, testCase.EstimatedDuration,
		testCase.SortOrder, testCase.CreatedAt, testCase.UpdatedAt).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testCase.ID = int(id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(testCase)
}

// UpdateTestCase updates an existing test case
func (h *TestCaseHandler) UpdateTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	var testCase models.TestCase
	if err := json.NewDecoder(r.Body).Decode(&testCase); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if testCase.Title == "" {
		http.Error(w, "Test case title is required", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	testCase.Title = utils.StripHTMLTags(testCase.Title)
	testCase.Preconditions = utils.StripHTMLTags(testCase.Preconditions)

	// Validate priority if provided
	if testCase.Priority != "" && !validTestCasePriorities[testCase.Priority] {
		http.Error(w, "Invalid priority value. Must be: low, medium, high, or critical", http.StatusBadRequest)
		return
	}

	// Validate status if provided
	if testCase.Status != "" && !validTestCaseStatuses[testCase.Status] {
		http.Error(w, "Invalid status value. Must be: active, inactive, or draft", http.StatusBadRequest)
		return
	}

	// Validate estimated duration is non-negative
	if testCase.EstimatedDuration < 0 {
		http.Error(w, "Estimated duration cannot be negative", http.StatusBadRequest)
		return
	}

	testCase.UpdatedAt = time.Now()

	query := `
		UPDATE test_cases
		SET folder_id = ?, title = ?, preconditions = ?,
		    priority = ?, status = ?, estimated_duration = ?,
		    sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := h.getWriteDB().Exec(query, testCase.FolderID, testCase.Title, testCase.Preconditions,
		testCase.Priority, testCase.Status, testCase.EstimatedDuration,
		testCase.SortOrder, testCase.UpdatedAt, id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	testCase.ID = id
	testCase.WorkspaceID = workspaceID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testCase)
}

// DeleteTestCase deletes a test case
func (h *TestCaseHandler) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	result, err := h.getWriteDB().Exec("DELETE FROM test_cases WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MoveTestCase moves a test case to a different folder
func (h *TestCaseHandler) MoveTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	var moveData struct {
		FolderID  *int `json:"folder_id"`
		SortOrder int  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&moveData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	query := `
		UPDATE test_cases
		SET folder_id = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := h.getWriteDB().Exec(query, moveData.FolderID, moveData.SortOrder, time.Now(), id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ReorderTestCases updates the sort order of multiple test cases within a folder
func (h *TestCaseHandler) ReorderTestCases(w http.ResponseWriter, r *http.Request) {
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
		FolderID    *int  `json:"folder_id"`
		TestCaseIDs []int `json:"test_case_ids"`
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
	for i, testCaseID := range reorderData.TestCaseIDs {
		sortOrder := (i + 1) * 1000 // Leave gaps for future insertions
		_, err = tx.Exec("UPDATE test_cases SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?",
			sortOrder, time.Now(), testCaseID, workspaceID)
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

// Test Step Handlers

// GetTestSteps returns all test steps for a test case
func (h *TestCaseHandler) GetTestSteps(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseId, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseId, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	query := `
		SELECT id, test_case_id, step_number, action, data, expected, created_at, updated_at
		FROM test_steps
		WHERE test_case_id = ?
		ORDER BY step_number
	`

	rows, err := h.getReadDB().Query(query, testCaseId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var testSteps []models.TestStep
	for rows.Next() {
		var testStep models.TestStep
		err := rows.Scan(
			&testStep.ID, &testStep.TestCaseID, &testStep.StepNumber,
			&testStep.Action, &testStep.Data, &testStep.Expected,
			&testStep.CreatedAt, &testStep.UpdatedAt,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		testSteps = append(testSteps, testStep)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testSteps)
}

// CreateTestStep creates a new test step
func (h *TestCaseHandler) CreateTestStep(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseId, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseId, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	var testStep models.TestStep
	if err := json.NewDecoder(r.Body).Decode(&testStep); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if testStep.Action == "" {
		http.Error(w, "Test step action is required", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	testStep.Action = utils.StripHTMLTags(testStep.Action)
	testStep.Data = utils.StripHTMLTags(testStep.Data)
	testStep.Expected = utils.StripHTMLTags(testStep.Expected)

	testStep.TestCaseID = testCaseId

	// Get the highest step_number for the test case
	var maxStepNumber sql.NullInt64
	err = h.getReadDB().QueryRow("SELECT MAX(step_number) FROM test_steps WHERE test_case_id = ?", testCaseId).Scan(&maxStepNumber)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testStep.StepNumber = int(maxStepNumber.Int64) + 1
	testStep.CreatedAt = time.Now()
	testStep.UpdatedAt = time.Now()

	query := `
		INSERT INTO test_steps (test_case_id, step_number, action, data, expected, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err = h.getWriteDB().QueryRow(query, testStep.TestCaseID, testStep.StepNumber,
		testStep.Action, testStep.Data, testStep.Expected, testStep.CreatedAt, testStep.UpdatedAt).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testStep.ID = int(id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(testStep)
}

// UpdateTestStep updates an existing test step
func (h *TestCaseHandler) UpdateTestStep(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseId, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
		return
	}

	stepId, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseId, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	var testStep models.TestStep
	if err := json.NewDecoder(r.Body).Decode(&testStep); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if testStep.Action == "" {
		http.Error(w, "Test step action is required", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS
	testStep.Action = utils.StripHTMLTags(testStep.Action)
	testStep.Data = utils.StripHTMLTags(testStep.Data)
	testStep.Expected = utils.StripHTMLTags(testStep.Expected)

	testStep.UpdatedAt = time.Now()

	query := `
		UPDATE test_steps
		SET step_number = ?, action = ?, data = ?, expected = ?, updated_at = ?
		WHERE id = ? AND test_case_id = ?
	`

	result, err := h.getWriteDB().Exec(query, testStep.StepNumber, testStep.Action, testStep.Data,
		testStep.Expected, testStep.UpdatedAt, stepId, testCaseId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Test step not found", http.StatusNotFound)
		return
	}

	testStep.ID = stepId
	testStep.TestCaseID = testCaseId
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testStep)
}

// DeleteTestStep deletes a test step
func (h *TestCaseHandler) DeleteTestStep(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseId, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
		return
	}

	stepId, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseId, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	result, err := h.getWriteDB().Exec("DELETE FROM test_steps WHERE id = ? AND test_case_id = ?", stepId, testCaseId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Test step not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderTestSteps updates the step order of multiple test steps
func (h *TestCaseHandler) ReorderTestSteps(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseId, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseId, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	var reorderData struct {
		StepIDs []int `json:"step_ids"`
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

	// Update step order based on array position
	for i, stepID := range reorderData.StepIDs {
		stepNumber := i + 1
		_, err = tx.Exec("UPDATE test_steps SET step_number = ?, updated_at = ? WHERE id = ? AND test_case_id = ?",
			stepNumber, time.Now(), stepID, testCaseId)
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

// GetAllTestLabels returns all available test labels for a workspace
func (h *TestCaseHandler) GetAllTestLabels(w http.ResponseWriter, r *http.Request) {
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
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels
		WHERE workspace_id = ?
		ORDER BY name
	`

	rows, err := h.getReadDB().Query(query, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var labels []models.TestLabel
	for rows.Next() {
		var label models.TestLabel
		err := rows.Scan(&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
			&label.CreatedAt, &label.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		labels = append(labels, label)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(labels)
}

// CreateTestLabel creates a new test label
func (h *TestCaseHandler) CreateTestLabel(w http.ResponseWriter, r *http.Request) {
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

	var label models.TestLabel
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if label.Name == "" {
		http.Error(w, "Label name is required", http.StatusBadRequest)
		return
	}

	if label.Color == "" {
		label.Color = "#3B82F6" // Default blue color
	}

	label.WorkspaceID = workspaceID
	now := time.Now()
	err = h.getReadDB().QueryRow(`
		INSERT INTO test_labels (workspace_id, name, color, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, updated_at
	`, label.WorkspaceID, label.Name, label.Color, label.Description, now, now).Scan(
		&label.ID, &label.CreatedAt, &label.UpdatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(label)
}

// UpdateTestLabel updates an existing test label
func (h *TestCaseHandler) UpdateTestLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	labelID, err := strconv.Atoi(r.PathValue("labelId"))
	if err != nil {
		http.Error(w, "Invalid label ID", http.StatusBadRequest)
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

	var label models.TestLabel
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if label.Name == "" {
		http.Error(w, "Label name is required", http.StatusBadRequest)
		return
	}

	now := time.Now()
	_, err = h.getWriteDB().Exec(`
		UPDATE test_labels
		SET name = ?, color = ?, description = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, label.Name, label.Color, label.Description, now, labelID, workspaceID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch updated label
	err = h.getReadDB().QueryRow(`
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels WHERE id = ? AND workspace_id = ?
	`, labelID, workspaceID).Scan(&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
		&label.CreatedAt, &label.UpdatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(label)
}

// DeleteTestLabel deletes a test label
func (h *TestCaseHandler) DeleteTestLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	labelID, err := strconv.Atoi(r.PathValue("labelId"))
	if err != nil {
		http.Error(w, "Invalid label ID", http.StatusBadRequest)
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

	_, err = h.getReadDB().Exec("DELETE FROM test_labels WHERE id = ? AND workspace_id = ?", labelID, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestCaseLabels returns all labels for a specific test case
func (h *TestCaseHandler) GetTestCaseLabels(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseID, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	query := `
		SELECT tl.id, tl.workspace_id, tl.name, tl.color, tl.description, tl.created_at, tl.updated_at
		FROM test_labels tl
		INNER JOIN test_case_labels tcl ON tl.id = tcl.label_id
		WHERE tcl.test_case_id = ?
		ORDER BY tl.name
	`

	rows, err := h.getReadDB().Query(query, testCaseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var labels []models.TestLabel
	for rows.Next() {
		var label models.TestLabel
		err := rows.Scan(&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
			&label.CreatedAt, &label.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		labels = append(labels, label)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(labels)
}

// AddTestCaseLabel adds a label to a test case
func (h *TestCaseHandler) AddTestCaseLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseID, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	var data struct {
		LabelID int `json:"label_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Verify label belongs to workspace
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_labels WHERE id = ? AND workspace_id = ?", data.LabelID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Label not found in workspace", http.StatusNotFound)
		return
	}

	now := time.Now()
	_, err = h.getWriteDB().Exec(`
		INSERT INTO test_case_labels (test_case_id, label_id, created_at)
		VALUES (?, ?, ?)
	`, testCaseID, data.LabelID, now)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// RemoveTestCaseLabel removes a label from a test case
func (h *TestCaseHandler) RemoveTestCaseLabel(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	testCaseID, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
		return
	}

	labelID, err := strconv.Atoi(r.PathValue("labelId"))
	if err != nil {
		http.Error(w, "Invalid label ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", testCaseID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	_, err = h.getWriteDB().Exec(`
		DELETE FROM test_case_labels
		WHERE test_case_id = ? AND label_id = ?
	`, testCaseID, labelID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTestCaseConnections returns related sets, templates, and executions for a test case
func (h *TestCaseHandler) GetTestCaseConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid test case ID", http.StatusBadRequest)
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

	// Verify test case belongs to workspace
	var count int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", id, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		http.Error(w, "Test case not found", http.StatusNotFound)
		return
	}

	response := struct {
		TestSets []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"test_sets"`
		RunTemplates []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			SetID       int    `json:"set_id"`
			SetName     string `json:"set_name"`
		} `json:"run_templates"`
		Executions []struct {
			RunID        int        `json:"run_id"`
			RunName      string     `json:"run_name"`
			Status       string     `json:"status"`
			StartedAt    time.Time  `json:"started_at"`
			EndedAt      *time.Time `json:"ended_at"`
			TemplateID   *int       `json:"template_id,omitempty"`
			TemplateName string     `json:"template_name,omitempty"`
			SetID        int        `json:"set_id"`
			SetName      string     `json:"set_name"`
		} `json:"executions"`
	}{}

	setRows, err := h.getReadDB().Query(`
		SELECT ts.id, ts.name, COALESCE(ts.description, '')
		FROM test_sets ts
		JOIN set_test_cases stc ON stc.set_id = ts.id
		WHERE stc.test_case_id = ? AND ts.workspace_id = ?
		ORDER BY ts.name
	`, id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for setRows.Next() {
		var summary struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := setRows.Scan(&summary.ID, &summary.Name, &summary.Description); err != nil {
			setRows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response.TestSets = append(response.TestSets, summary)
	}
	setRows.Close()

	tmplRows, err := h.getReadDB().Query(`
		SELECT trt.id, trt.name, COALESCE(trt.description, ''), trt.set_id, COALESCE(ts.name, '')
		FROM test_run_templates trt
		JOIN set_test_cases stc ON stc.set_id = trt.set_id
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE stc.test_case_id = ? AND trt.workspace_id = ?
		ORDER BY trt.updated_at DESC
	`, id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for tmplRows.Next() {
		var summary struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			SetID       int    `json:"set_id"`
			SetName     string `json:"set_name"`
		}
		if err := tmplRows.Scan(&summary.ID, &summary.Name, &summary.Description, &summary.SetID, &summary.SetName); err != nil {
			tmplRows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response.RunTemplates = append(response.RunTemplates, summary)
	}
	tmplRows.Close()

	runRows, err := h.getReadDB().Query(`
		SELECT tr.id, tr.name, tr.set_id, COALESCE(ts.name, ''), tr.template_id, COALESCE(trt.name, ''),
		       tr.started_at, tr.ended_at, trr.status
		FROM test_runs tr
		JOIN test_results trr ON trr.run_id = tr.id AND trr.test_case_id = ?
		LEFT JOIN test_sets ts ON tr.set_id = ts.id
		LEFT JOIN test_run_templates trt ON tr.template_id = trt.id
		WHERE tr.workspace_id = ?
		ORDER BY tr.started_at DESC
		LIMIT 20
	`, id, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for runRows.Next() {
		var record struct {
			RunID        int
			RunName      string
			SetID        int
			SetName      string
			TemplateID   sql.NullInt64
			TemplateName string
			StartedAt    time.Time
			EndedAt      sql.NullTime
			Status       string
		}
		if err := runRows.Scan(&record.RunID, &record.RunName, &record.SetID, &record.SetName, &record.TemplateID, &record.TemplateName, &record.StartedAt, &record.EndedAt, &record.Status); err != nil {
			runRows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		execution := struct {
			RunID        int        `json:"run_id"`
			RunName      string     `json:"run_name"`
			Status       string     `json:"status"`
			StartedAt    time.Time  `json:"started_at"`
			EndedAt      *time.Time `json:"ended_at"`
			TemplateID   *int       `json:"template_id,omitempty"`
			TemplateName string     `json:"template_name,omitempty"`
			SetID        int        `json:"set_id"`
			SetName      string     `json:"set_name"`
		}{
			RunID:     record.RunID,
			RunName:   record.RunName,
			Status:    record.Status,
			StartedAt: record.StartedAt,
			SetID:     record.SetID,
			SetName:   record.SetName,
		}
		if record.EndedAt.Valid {
			end := record.EndedAt.Time
			execution.EndedAt = &end
		}
		if record.TemplateID.Valid {
			tid := int(record.TemplateID.Int64)
			execution.TemplateID = &tid
			execution.TemplateName = record.TemplateName
		}
		response.Executions = append(response.Executions, execution)
	}
	runRows.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
