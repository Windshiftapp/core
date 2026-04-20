package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestRunTemplateRepository provides data access for test run templates.
type TestRunTemplateRepository struct {
	db database.Database
}

// NewTestRunTemplateRepository creates a new test run template repository.
func NewTestRunTemplateRepository(db database.Database) *TestRunTemplateRepository {
	return &TestRunTemplateRepository{db: db}
}

// FindAll returns all templates for a workspace with joined test_set name.
func (r *TestRunTemplateRepository) FindAll(workspaceID int) ([]models.TestRunTemplate, error) {
	rows, err := r.db.Query(`
		SELECT
			trt.id, trt.workspace_id, trt.set_id, trt.name, trt.description, trt.created_at, trt.updated_at,
			ts.name as set_name
		FROM test_run_templates trt
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE trt.workspace_id = ?
		ORDER BY trt.id DESC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test run templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	templates := make([]models.TestRunTemplate, 0)
	for rows.Next() {
		var template models.TestRunTemplate
		var setName sql.NullString
		if err := rows.Scan(
			&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description,
			&template.CreatedAt, &template.UpdatedAt, &setName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan test run template: %w", err)
		}
		if setName.Valid {
			template.SetName = setName.String
		}
		templates = append(templates, template)
	}
	return templates, nil
}

// FindByID returns a single test run template with joined set name.
func (r *TestRunTemplateRepository) FindByID(id, workspaceID int) (*models.TestRunTemplate, error) {
	var template models.TestRunTemplate
	var setName sql.NullString
	err := r.db.QueryRow(`
		SELECT
			trt.id, trt.workspace_id, trt.set_id, trt.name, trt.description, trt.created_at, trt.updated_at,
			ts.name as set_name
		FROM test_run_templates trt
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE trt.id = ? AND trt.workspace_id = ?
	`, id, workspaceID).Scan(
		&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description,
		&template.CreatedAt, &template.UpdatedAt, &setName,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test run template: %w", err)
	}
	if setName.Valid {
		template.SetName = setName.String
	}
	return &template, nil
}

// FindCore returns a template's core fields (no joined data) — used before executing.
func (r *TestRunTemplateRepository) FindCore(id, workspaceID int) (*models.TestRunTemplate, error) {
	var template models.TestRunTemplate
	err := r.db.QueryRow(`
		SELECT id, workspace_id, set_id, name, description
		FROM test_run_templates
		WHERE id = ? AND workspace_id = ?
	`, id, workspaceID).Scan(
		&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test run template: %w", err)
	}
	return &template, nil
}

// Create inserts a new template and returns its id and the timestamp used.
func (r *TestRunTemplateRepository) Create(workspaceID int, template *models.TestRunTemplate) (int, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO test_run_templates (workspace_id, set_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, template.SetID, template.Name, template.Description, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to create test run template: %w", err)
	}
	return int(id), now, nil
}

// Update applies new values to a template and returns the new updated_at timestamp.
func (r *TestRunTemplateRepository) Update(id, workspaceID int, template *models.TestRunTemplate) (time.Time, error) {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE test_run_templates
		SET set_id = ?, name = ?, description = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, template.SetID, template.Name, template.Description, now, id, workspaceID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update test run template: %w", err)
	}
	return now, nil
}

// Delete removes a template.
func (r *TestRunTemplateRepository) Delete(id, workspaceID int) error {
	_, err := r.db.ExecWrite(
		"DELETE FROM test_run_templates WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete test run template: %w", err)
	}
	return nil
}

// FindExecutions returns the test runs created from a template, newest first.
func (r *TestRunTemplateRepository) FindExecutions(templateID, workspaceID int) ([]models.TestRun, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_id, template_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE template_id = ? AND workspace_id = ?
		ORDER BY id DESC
	`, templateID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query template executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		var templateIDNullable sql.NullInt64
		if err := rows.Scan(&run.ID, &run.WorkspaceID, &templateIDNullable, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan template execution: %w", err)
		}
		if templateIDNullable.Valid {
			run.TemplateID = int(templateIDNullable.Int64)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// CountExecutions returns the number of runs created from a template.
func (r *TestRunTemplateRepository) CountExecutions(templateID, workspaceID int) (int, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM test_runs WHERE template_id = ? AND workspace_id = ?",
		templateID, workspaceID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count template executions: %w", err)
	}
	return count, nil
}

// Execute creates a new test run from a template in a single transaction:
// - inserts the test_runs row
// - creates a pending test_results row for each test case in the set
// It then re-reads the created run (with the joined fields) and returns it.
func (r *TestRunTemplateRepository) Execute(workspaceID, templateID, setID int, runName string) (*models.TestRun, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	var runID int64
	if err := tx.QueryRow(`
		INSERT INTO test_runs (workspace_id, template_id, set_id, name, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, templateID, setID, runName, now, now).Scan(&runID); err != nil {
		return nil, fmt.Errorf("failed to create test run: %w", err)
	}

	caseRows, err := tx.Query(`
		SELECT tc.id
		FROM test_cases tc
		JOIN set_test_cases stc ON tc.id = stc.test_case_id
		WHERE stc.set_id = ? AND tc.workspace_id = ?
		ORDER BY tc.id
	`, setID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query set test cases: %w", err)
	}

	var testCaseIDs []int
	for caseRows.Next() {
		var id int
		if err := caseRows.Scan(&id); err != nil {
			_ = caseRows.Close()
			return nil, fmt.Errorf("failed to scan test case id: %w", err)
		}
		testCaseIDs = append(testCaseIDs, id)
	}
	_ = caseRows.Close()

	for _, testCaseID := range testCaseIDs {
		if _, err := tx.Exec(`
			INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
			VALUES (?, ?, 'pending', ?, ?)
		`, runID, testCaseID, now, now); err != nil {
			return nil, fmt.Errorf("failed to create test result: %w", err)
		}
	}

	var run models.TestRun
	var templateIDNullable sql.NullInt64
	if err := tx.QueryRow(`
		SELECT id, workspace_id, template_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE id = ?
	`, runID).Scan(&run.ID, &run.WorkspaceID, &templateIDNullable, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt); err != nil {
		return nil, fmt.Errorf("failed to reload created test run: %w", err)
	}
	if templateIDNullable.Valid {
		run.TemplateID = int(templateIDNullable.Int64)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit execute: %w", err)
	}

	return &run, nil
}
