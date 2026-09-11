package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestRunRepository provides data access methods for test runs
type TestRunRepository struct {
	db database.Database
}

// NewTestRunRepository creates a new test run repository
func NewTestRunRepository(db database.Database) *TestRunRepository {
	return &TestRunRepository{db: db}
}

// GetWorkspaceIDForResult resolves the workspace owning a test result via its
// run. Returns ErrNotFound when the result is missing.
func (r *TestRunRepository) GetWorkspaceIDForResult(resultID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow(`
		SELECT run.workspace_id
		FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.id = ?
	`, resultID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve test result workspace: %w", err)
	}
	return workspaceID, nil
}

// TestRunFilters contains filter parameters for listing test runs
type TestRunFilters struct {
	AssigneeID   *int // Filter by specific assignee
	Unassigned   bool // Filter for unassigned runs
	TemplateID   *int // Filter by template
	SetID        *int // Filter by test set
	IncludeEnded bool // Include ended runs
	Limit        int
	Offset       int
}

// FindAll returns test runs for a workspace with optional filters
func (r *TestRunRepository) FindAll(workspaceID int, filters TestRunFilters) ([]models.TestRun, error) {
	query := `
		SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
		       tr.started_at, tr.ended_at, tr.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       COALESCE(u.email, '') as assignee_email,
		       COALESCE(u.avatar_url, '') as assignee_avatar
		FROM test_runs tr
		LEFT JOIN users u ON tr.assignee_id = u.id
	` + testRunWhere(filters) + " ORDER BY tr.id DESC"
	args := testRunArgs(workspaceID, filters)
	if filters.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filters.Limit, max(filters.Offset, 0))
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query test runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]models.TestRun, 0)
	for rows.Next() {
		run, err := r.scanTestRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}

	return runs, nil
}

// FindPage returns a stable newest-first page and the total matching rows.
func (r *TestRunRepository) FindPage(workspaceID int, filters TestRunFilters) ([]models.TestRun, int, error) {
	if filters.Limit <= 0 {
		return nil, 0, errors.New("test run page limit must be positive")
	}
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM test_runs tr "+testRunWhere(filters), testRunArgs(workspaceID, filters)...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count test runs: %w", err)
	}
	items, err := r.FindAll(workspaceID, filters)
	return items, total, err
}

func testRunWhere(filters TestRunFilters) string {
	query := "WHERE tr.workspace_id = ?"
	if filters.Unassigned {
		query += " AND tr.assignee_id IS NULL"
	} else if filters.AssigneeID != nil {
		query += " AND tr.assignee_id = ?"
	}
	if filters.TemplateID != nil {
		query += " AND tr.template_id = ?"
	}
	if filters.SetID != nil {
		query += " AND tr.set_id = ?"
	}
	if !filters.IncludeEnded {
		query += " AND tr.ended_at IS NULL"
	}
	return query
}

func testRunArgs(workspaceID int, filters TestRunFilters) []any {
	args := []any{workspaceID}
	if !filters.Unassigned && filters.AssigneeID != nil {
		args = append(args, *filters.AssigneeID)
	}
	if filters.TemplateID != nil {
		args = append(args, *filters.TemplateID)
	}
	if filters.SetID != nil {
		args = append(args, *filters.SetID)
	}
	return args
}

// FindByID retrieves a single test run by ID
func (r *TestRunRepository) FindByID(id, workspaceID int) (*models.TestRun, error) {
	query := `
		SELECT tr.id, tr.workspace_id, tr.template_id, tr.set_id, tr.name, tr.assignee_id,
		       tr.started_at, tr.ended_at, tr.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       COALESCE(u.email, '') as assignee_email,
		       COALESCE(u.avatar_url, '') as assignee_avatar
		FROM test_runs tr
		LEFT JOIN users u ON tr.assignee_id = u.id
		WHERE tr.id = ? AND tr.workspace_id = ?
	`

	row := r.db.QueryRow(query, id, workspaceID)
	return r.scanTestRunRow(row)
}

// Create inserts a new test run and returns its ID
func (r *TestRunRepository) Create(tx database.Tx, run *models.TestRun) (int, error) {
	var templateIDPtr *int
	if run.TemplateID > 0 {
		templateIDPtr = &run.TemplateID
	}

	now := time.Now()
	var runID int64
	err := tx.QueryRow(`
		INSERT INTO test_runs (workspace_id, template_id, set_id, name, assignee_id, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, run.WorkspaceID, templateIDPtr, run.SetID, run.Name, run.AssigneeID, now, now).Scan(&runID)

	if err != nil {
		return 0, fmt.Errorf("failed to create test run: %w", err)
	}

	return int(runID), nil
}

// Update updates an existing test run
func (r *TestRunRepository) Update(tx database.Tx, run *models.TestRun) error {
	var templateIDPtr *int
	if run.TemplateID > 0 {
		templateIDPtr = &run.TemplateID
	}

	_, err := tx.Exec(`
		UPDATE test_runs
		SET template_id = ?, set_id = ?, name = ?, assignee_id = ?, ended_at = ?
		WHERE id = ? AND workspace_id = ?
	`, templateIDPtr, run.SetID, run.Name, run.AssigneeID, run.EndedAt, run.ID, run.WorkspaceID)

	if err != nil {
		return fmt.Errorf("failed to update test run: %w", err)
	}

	return nil
}

// Delete removes a test run by ID
func (r *TestRunRepository) Delete(tx database.Tx, id, workspaceID int) error {
	// Delete results first
	_, err := tx.Exec("DELETE FROM test_results WHERE run_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete test results: %w", err)
	}

	result, err := tx.Exec("DELETE FROM test_runs WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete test run: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Complete marks a test run as ended
func (r *TestRunRepository) Complete(tx database.Tx, id, workspaceID int) error {
	now := time.Now()
	result, err := tx.Exec(`
		UPDATE test_runs SET ended_at = ? WHERE id = ? AND workspace_id = ? AND ended_at IS NULL
	`, now, id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to complete test run: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Exists checks if a test run exists in a workspace
func (r *TestRunRepository) Exists(id, workspaceID int) (bool, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM test_runs WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check test run existence: %w", err)
	}
	return count > 0, nil
}

// Test Result methods

// UpdateResult updates an existing test result. runID scopes the update so
// callers can't update a result that belongs to a different run by
// substituting only the result ID. Returns ErrNotFound when no row matches
// both id AND run_id.
func (r *TestRunRepository) UpdateResult(tx database.Tx, runID int, result *models.TestResult) error {
	now := time.Now()
	res, err := tx.Exec(`
		UPDATE test_results
		SET status = ?, actual_result = ?, notes = ?, executed_at = ?, updated_at = ?
		WHERE id = ? AND run_id = ?
	`, result.Status, result.ActualResult, result.Notes, result.ExecutedAt, now, result.ID, runID)

	if err != nil {
		return fmt.Errorf("failed to update test result: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// CreateResultsFromSet creates test results for all test cases in a set.
//
// We drain the set_test_cases rows into a slice BEFORE running the INSERTs:
// lib/pq pins the transaction to a single connection, and issuing more
// statements while a Rows is still streaming yields "unexpected Parse
// response 'C'" because the connection's buffer still holds CommandComplete
// frames from the prior query. SQLite's driver doesn't have this constraint,
// which is why the original two-loop pattern only failed on Postgres.
func (r *TestRunRepository) CreateResultsFromSet(tx database.Tx, runID, setID int) error {
	rows, err := tx.Query(`
		SELECT test_case_id FROM set_test_cases WHERE set_id = ?
	`, setID)
	if err != nil {
		return fmt.Errorf("failed to query set test cases: %w", err)
	}
	var testCaseIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan test case ID: %w", err)
		}
		testCaseIDs = append(testCaseIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate set test cases: %w", err)
	}
	_ = rows.Close()

	now := time.Now()
	for _, testCaseID := range testCaseIDs {
		if _, err := tx.Exec(`
			INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
			VALUES (?, ?, 'not_run', ?, ?)
		`, runID, testCaseID, now, now); err != nil {
			return fmt.Errorf("failed to create test result: %w", err)
		}
	}

	return nil
}

// TestResultWithTestCase is a test result paired with the parent test case title.
type TestResultWithTestCase struct {
	models.TestResult
	TestCaseTitle string
}

// FindResultsWithTestCase returns test results for a run (scoped to workspace)
// with the parent test case title included.
func (r *TestRunRepository) FindResultsWithTestCase(runID, workspaceID int) ([]TestResultWithTestCase, error) {
	rows, err := r.db.Query(`
		SELECT tr.id, tr.run_id, tr.test_case_id, tr.status, tr.actual_result, tr.notes,
		       tr.executed_at, tr.created_at, tr.updated_at, tc.title
		FROM test_results tr
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.run_id = ? AND run.workspace_id = ?
		ORDER BY tc.id
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test results with test case: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]TestResultWithTestCase, 0)
	for rows.Next() {
		var res TestResultWithTestCase
		var actualResult, notes sql.NullString
		var executedAt sql.NullTime

		if err := rows.Scan(&res.ID, &res.RunID, &res.TestCaseID, &res.Status,
			&actualResult, &notes, &executedAt,
			&res.CreatedAt, &res.UpdatedAt, &res.TestCaseTitle); err != nil {
			return nil, fmt.Errorf("failed to scan test result row: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test results with test case: %w", err)
	}
	return results, nil
}

// FindCasesWithStepsForRun returns the run's test cases with all steps using
// two bounded queries, independent of the number of cases. test_results is the
// authoritative case-membership snapshot for a run, so later test-set edits do
// not add or remove cases from an in-progress or completed execution.
func (r *TestRunRepository) FindCasesWithStepsForRun(runID, workspaceID int) ([]models.TestCase, error) {
	rows, err := r.db.Query(`
		SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title, tc.name,
		       tc.priority, tc.status, tc.estimated_duration, tc.preconditions,
		       tc.sort_order, tc.created_at, tc.updated_at
		FROM test_results tr
		JOIN test_runs run ON run.id = tr.run_id
		JOIN test_cases tc ON tc.id = tr.test_case_id
		WHERE tr.run_id = ? AND run.workspace_id = ?
		ORDER BY tc.id
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query test cases for run: %w", err)
	}

	testCases := make([]models.TestCase, 0)
	caseIndex := make(map[int]int)
	for rows.Next() {
		var testCase models.TestCase
		if scanErr := rows.Scan(
			&testCase.ID, &testCase.WorkspaceID, &testCase.FolderID,
			&testCase.Title, &testCase.Name, &testCase.Priority, &testCase.Status,
			&testCase.EstimatedDuration, &testCase.Preconditions, &testCase.SortOrder,
			&testCase.CreatedAt, &testCase.UpdatedAt,
		); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan test case for run: %w", scanErr)
		}
		testCase.TestSteps = []models.TestStep{}
		caseIndex[testCase.ID] = len(testCases)
		testCases = append(testCases, testCase)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate test cases for run: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close test cases for run: %w", err)
	}

	stepRows, err := r.db.Query(`
		SELECT ts.id, ts.test_case_id, ts.step_number, ts.action, ts.data,
		       ts.expected, ts.created_at, ts.updated_at
		FROM test_steps ts
		JOIN test_results tr ON tr.test_case_id = ts.test_case_id
		JOIN test_runs run ON run.id = tr.run_id
		WHERE tr.run_id = ? AND run.workspace_id = ?
		ORDER BY ts.test_case_id, ts.step_number
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query test steps for run: %w", err)
	}
	defer func() { _ = stepRows.Close() }()

	for stepRows.Next() {
		var step models.TestStep
		if scanErr := stepRows.Scan(
			&step.ID, &step.TestCaseID, &step.StepNumber, &step.Action,
			&step.Data, &step.Expected, &step.CreatedAt, &step.UpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan test step for run: %w", scanErr)
		}
		if index, ok := caseIndex[step.TestCaseID]; ok {
			testCases[index].TestSteps = append(testCases[index].TestSteps, step)
		}
	}
	if err := stepRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test steps for run: %w", err)
	}
	return testCases, nil
}

// TestResultBelongsToWorkspace reports whether a test result is owned by the given workspace
// (via its parent run).
func (r *TestRunRepository) TestResultBelongsToWorkspace(resultID, workspaceID int) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.id = ? AND run.workspace_id = ?
	`, resultID, workspaceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check test result workspace: %w", err)
	}
	return count > 0, nil
}

// FindTestResultIDForStep resolves the test_result row that corresponds to the given
// run + step combination within a workspace.
func (r *TestRunRepository) FindTestResultIDForStep(runID, stepID, workspaceID int) (int, error) {
	var id int
	err := r.db.QueryRow(`
		SELECT tr.id
		FROM test_results tr
		JOIN test_runs run ON tr.run_id = run.id
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_steps ts ON ts.test_case_id = tc.id
		WHERE tr.run_id = ? AND ts.id = ? AND run.workspace_id = ?
		LIMIT 1
	`, runID, stepID, workspaceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find test result for step: %w", err)
	}
	return id, nil
}

// FindStepResultID returns the id of an existing test_step_results row for the given
// test_result/step combination, or ErrNotFound if there is none.
func (r *TestRunRepository) FindStepResultID(testResultID, stepID int) (int, error) {
	var id int
	err := r.db.QueryRow(`
		SELECT id FROM test_step_results
		WHERE test_result_id = ? AND test_step_id = ?
	`, testResultID, stepID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find step result: %w", err)
	}
	return id, nil
}

// StepResultInput carries the fields needed to create or update a step result.
type StepResultInput struct {
	TestResultID int
	StepID       int
	Status       string
	ActualResult string
	Notes        string
	ItemID       *int
}

// CreateStepResult inserts a new test_step_results row.
func (r *TestRunRepository) CreateStepResult(in StepResultInput) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO test_step_results
		(test_result_id, test_step_id, status, actual_result, notes, item_id, executed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.TestResultID, in.StepID, in.Status, in.ActualResult, in.Notes, in.ItemID, now, now, now)
	if err != nil {
		return fmt.Errorf("failed to create step result: %w", err)
	}
	return nil
}

// UpdateStepResult updates an existing test_step_results row by its id.
func (r *TestRunRepository) UpdateStepResult(id int, in StepResultInput) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE test_step_results
		SET status = ?, actual_result = ?, notes = ?, item_id = ?, executed_at = ?, updated_at = ?
		WHERE id = ?
	`, in.Status, in.ActualResult, in.Notes, in.ItemID, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to update step result: %w", err)
	}
	return nil
}

// StepResultRow captures all data needed to render a single step-result entry in the UI.
type StepResultRow struct {
	StepID        int
	TestCaseID    int
	TestCaseTitle string
	Status        string
	ActualResult  string
	Notes         string
	ItemID        *int
	ExecutedAt    *time.Time
}

// FindStepResultsForRun returns every step result attached to a test run, scoped by workspace.
func (r *TestRunRepository) FindStepResultsForRun(runID, workspaceID int) ([]StepResultRow, error) {
	rows, err := r.db.Query(`
		SELECT tsr.test_step_id, tsr.status, tsr.actual_result, tsr.notes, tsr.item_id, tsr.executed_at,
		       tc.id as test_case_id, tc.title as test_case_title
		FROM test_step_results tsr
		JOIN test_results tr ON tsr.test_result_id = tr.id
		JOIN test_cases tc ON tr.test_case_id = tc.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tr.run_id = ? AND run.workspace_id = ?
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query step results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]StepResultRow, 0)
	for rows.Next() {
		var row StepResultRow
		if err := rows.Scan(&row.StepID, &row.Status, &row.ActualResult, &row.Notes,
			&row.ItemID, &row.ExecutedAt, &row.TestCaseID, &row.TestCaseTitle); err != nil {
			return nil, fmt.Errorf("failed to scan step result: %w", err)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate step results: %w", err)
	}
	return results, nil
}

// FindStepResultStatuses returns every step result status for a given test_result.
func (r *TestRunRepository) FindStepResultStatuses(testResultID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT status FROM test_step_results WHERE test_result_id = ?
	`, testResultID)
	if err != nil {
		return nil, fmt.Errorf("failed to query step statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	statuses := make([]string, 0)
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return nil, fmt.Errorf("failed to scan step status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate step statuses: %w", err)
	}
	return statuses, nil
}

// SetTestResultStatus sets the status column on a test_results row.
func (r *TestRunRepository) SetTestResultStatus(testResultID int, status string) error {
	_, err := r.db.ExecWrite(`
		UPDATE test_results SET status = ?, updated_at = ? WHERE id = ?
	`, status, time.Now(), testResultID)
	if err != nil {
		return fmt.Errorf("failed to set test result status: %w", err)
	}
	return nil
}

// LinkResultToItem links a work item to a test result via test_result_items.
func (r *TestRunRepository) LinkResultToItem(resultID, itemID int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO test_result_items (test_result_id, item_id, created_at)
		VALUES (?, ?, ?)
	`, resultID, itemID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to link item to test result: %w", err)
	}
	return nil
}

// UnlinkResultFromItem removes the link between a test result and a work item.
func (r *TestRunRepository) UnlinkResultFromItem(resultID, itemID int) error {
	_, err := r.db.ExecWrite(`
		DELETE FROM test_result_items
		WHERE test_result_id = ? AND item_id = ?
	`, resultID, itemID)
	if err != nil {
		return fmt.Errorf("failed to unlink item from test result: %w", err)
	}
	return nil
}

// Helper methods

// testRunScanner abstracts *sql.Row and *sql.Rows so scanTestRun can back both.
type testRunScanner interface {
	Scan(dest ...any) error
}

func scanTestRun(s testRunScanner) (*models.TestRun, error) {
	var run models.TestRun
	var templateID, assigneeID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar string

	err := s.Scan(
		&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
		&run.StartedAt, &run.EndedAt, &run.CreatedAt,
		&assigneeName, &assigneeEmail, &assigneeAvatar,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan test run: %w", err)
	}

	if templateID.Valid {
		run.TemplateID = int(templateID.Int64)
	}
	if assigneeID.Valid {
		id := int(assigneeID.Int64)
		run.AssigneeID = &id
		run.AssigneeName = assigneeName
		run.AssigneeEmail = assigneeEmail
		run.AssigneeAvatar = assigneeAvatar
	}

	return &run, nil
}

func (r *TestRunRepository) scanTestRun(rows *sql.Rows) (*models.TestRun, error) {
	return scanTestRun(rows)
}

func (r *TestRunRepository) scanTestRunRow(row *sql.Row) (*models.TestRun, error) {
	return scanTestRun(row)
}
