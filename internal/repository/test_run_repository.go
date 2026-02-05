package repository

import (
	"database/sql"
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

// TestRunFilters contains filter parameters for listing test runs
type TestRunFilters struct {
	AssigneeID   *int // Filter by specific assignee
	Unassigned   bool // Filter for unassigned runs
	TemplateID   *int // Filter by template
	SetID        *int // Filter by test set
	IncludeEnded bool // Include ended runs
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
		WHERE tr.workspace_id = ?
	`
	args := []interface{}{workspaceID}

	if filters.Unassigned {
		query += " AND tr.assignee_id IS NULL"
	} else if filters.AssigneeID != nil {
		query += " AND tr.assignee_id = ?"
		args = append(args, *filters.AssigneeID)
	}

	if filters.TemplateID != nil {
		query += " AND tr.template_id = ?"
		args = append(args, *filters.TemplateID)
	}

	if filters.SetID != nil {
		query += " AND tr.set_id = ?"
		args = append(args, *filters.SetID)
	}

	if !filters.IncludeEnded {
		query += " AND tr.ended_at IS NULL"
	}

	query += " ORDER BY tr.id DESC"

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

// FindByIDWithResults retrieves a test run with all its results
func (r *TestRunRepository) FindByIDWithResults(id, workspaceID int) (*models.TestRun, []models.TestResult, error) {
	run, err := r.FindByID(id, workspaceID)
	if err != nil {
		return nil, nil, err
	}

	results, err := r.FindResults(id)
	if err != nil {
		return nil, nil, err
	}

	return run, results, nil
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

// FindResults returns all results for a test run
func (r *TestRunRepository) FindResults(runID int) ([]models.TestResult, error) {
	query := `
		SELECT tr.id, tr.run_id, tr.test_case_id, tr.status,
		       COALESCE(tr.actual_result, '') as actual_result,
		       COALESCE(tr.notes, '') as notes,
		       tr.executed_at, tr.created_at, tr.updated_at
		FROM test_results tr
		WHERE tr.run_id = ?
		ORDER BY tr.test_case_id
	`

	rows, err := r.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []models.TestResult
	for rows.Next() {
		var result models.TestResult
		err := rows.Scan(
			&result.ID, &result.RunID, &result.TestCaseID, &result.Status,
			&result.ActualResult, &result.Notes, &result.ExecutedAt,
			&result.CreatedAt, &result.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test result: %w", err)
		}
		results = append(results, result)
	}

	return results, nil
}

// FindResultByTestCase retrieves a single result by run ID and test case ID
func (r *TestRunRepository) FindResultByTestCase(runID, testCaseID int) (*models.TestResult, error) {
	query := `
		SELECT id, run_id, test_case_id, status,
		       COALESCE(actual_result, '') as actual_result,
		       COALESCE(notes, '') as notes,
		       executed_at, created_at, updated_at
		FROM test_results
		WHERE run_id = ? AND test_case_id = ?
	`

	var result models.TestResult
	err := r.db.QueryRow(query, runID, testCaseID).Scan(
		&result.ID, &result.RunID, &result.TestCaseID, &result.Status,
		&result.ActualResult, &result.Notes, &result.ExecutedAt,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test result: %w", err)
	}

	return &result, nil
}

// CreateResult inserts a new test result
func (r *TestRunRepository) CreateResult(tx database.Tx, result *models.TestResult) (int, error) {
	now := time.Now()
	var id int64
	err := tx.QueryRow(`
		INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, result.RunID, result.TestCaseID, result.Status, now, now).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create test result: %w", err)
	}

	return int(id), nil
}

// UpdateResult updates an existing test result
func (r *TestRunRepository) UpdateResult(tx database.Tx, result *models.TestResult) error {
	now := time.Now()
	_, err := tx.Exec(`
		UPDATE test_results
		SET status = ?, actual_result = ?, notes = ?, executed_at = ?, updated_at = ?
		WHERE id = ?
	`, result.Status, result.ActualResult, result.Notes, result.ExecutedAt, now, result.ID)

	if err != nil {
		return fmt.Errorf("failed to update test result: %w", err)
	}

	return nil
}

// CreateResultsFromSet creates test results for all test cases in a set
func (r *TestRunRepository) CreateResultsFromSet(tx database.Tx, runID, setID int) error {
	rows, err := tx.Query(`
		SELECT test_case_id FROM set_test_cases WHERE set_id = ?
	`, setID)
	if err != nil {
		return fmt.Errorf("failed to query set test cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	now := time.Now()
	for rows.Next() {
		var testCaseID int
		if err = rows.Scan(&testCaseID); err != nil {
			return fmt.Errorf("failed to scan test case ID: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
			VALUES (?, ?, 'not_run', ?, ?)
		`, runID, testCaseID, now, now)

		if err != nil {
			return fmt.Errorf("failed to create test result: %w", err)
		}
	}

	return nil
}

// GetResultSummary returns a summary of results for a test run
func (r *TestRunRepository) GetResultSummary(runID int) (map[string]int, error) {
	rows, err := r.db.Query(`
		SELECT status, COUNT(*) as count
		FROM test_results
		WHERE run_id = ?
		GROUP BY status
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query result summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	summary := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		summary[status] = count
	}

	return summary, nil
}

// Helper methods

func (r *TestRunRepository) scanTestRun(rows *sql.Rows) (*models.TestRun, error) {
	var run models.TestRun
	var templateID, assigneeID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar string

	err := rows.Scan(
		&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
		&run.StartedAt, &run.EndedAt, &run.CreatedAt,
		&assigneeName, &assigneeEmail, &assigneeAvatar,
	)
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

func (r *TestRunRepository) scanTestRunRow(row *sql.Row) (*models.TestRun, error) {
	var run models.TestRun
	var templateID, assigneeID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar string

	err := row.Scan(
		&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &assigneeID,
		&run.StartedAt, &run.EndedAt, &run.CreatedAt,
		&assigneeName, &assigneeEmail, &assigneeAvatar,
	)
	if err == sql.ErrNoRows {
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
