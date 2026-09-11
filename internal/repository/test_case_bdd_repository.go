package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/gherkin"
	"windshift/internal/models"
)

// TestCaseBDDRepository provides data access for BDD-format test cases:
// authored Gherkin storage, run specification snapshots, and per-example
// execution results.
type TestCaseBDDRepository struct {
	db database.Database
}

// NewTestCaseBDDRepository creates a new BDD test case repository.
func NewTestCaseBDDRepository(db database.Database) *TestCaseBDDRepository {
	return &TestCaseBDDRepository{db: db}
}

// Upsert stores the authored Gherkin and derived structure for a test case.
func (r *TestCaseBDDRepository) Upsert(tx database.Tx, bdd *models.TestCaseBDD) error {
	now := time.Now()
	bdd.CreatedAt = now
	bdd.UpdatedAt = now
	query := `
		INSERT INTO test_case_bdd (test_case_id, gherkin, feature_name, scenario_keyword, spec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (test_case_id) DO UPDATE SET
			gherkin = excluded.gherkin,
			feature_name = excluded.feature_name,
			scenario_keyword = excluded.scenario_keyword,
			spec = excluded.spec,
			updated_at = excluded.updated_at
	`
	_, err := tx.Exec(query, bdd.TestCaseID, bdd.Gherkin, bdd.FeatureName,
		bdd.ScenarioKeyword, bdd.Spec, bdd.CreatedAt, bdd.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to store BDD content: %w", err)
	}
	return nil
}

// FindBDD returns the authored BDD content for a test case.
func (r *TestCaseBDDRepository) FindBDD(testCaseID int) (*models.TestCaseBDD, error) {
	var bdd models.TestCaseBDD
	err := r.db.QueryRow(`
		SELECT test_case_id, gherkin, feature_name, scenario_keyword, spec, created_at, updated_at
		FROM test_case_bdd
		WHERE test_case_id = ?
	`, testCaseID).Scan(
		&bdd.TestCaseID, &bdd.Gherkin, &bdd.FeatureName, &bdd.ScenarioKeyword,
		&bdd.Spec, &bdd.CreatedAt, &bdd.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find BDD content: %w", err)
	}
	return &bdd, nil
}

// SnapshotSet copies the current specification of every BDD case in a set
// into the run's snapshot table and eagerly creates one not_run example
// result per Examples row, so the case-level aggregate reflects examples
// that have not been executed yet. Idempotent per run.
func (r *TestCaseBDDRepository) SnapshotSet(tx database.Tx, runID, setID int) error {
	rows, err := tx.Query(`
		SELECT tc.id, tc.title, COALESCE(tc.preconditions, ''), b.gherkin, b.spec
		FROM set_test_cases stc
		JOIN test_cases tc ON tc.id = stc.test_case_id
		JOIN test_case_bdd b ON b.test_case_id = tc.id
		WHERE stc.set_id = ?
	`, setID)
	if err != nil {
		return fmt.Errorf("failed to query BDD cases for run: %w", err)
	}
	type bddCase struct {
		id            int
		title         string
		preconditions string
		gherkin       string
		spec          string
	}
	cases := []bddCase{}
	for rows.Next() {
		var c bddCase
		if err := rows.Scan(&c.id, &c.title, &c.preconditions, &c.gherkin, &c.spec); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan BDD case for run: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate BDD cases for run: %w", err)
	}
	_ = rows.Close()

	now := time.Now()
	for _, c := range cases {
		if _, err := tx.Exec(`
			INSERT INTO test_run_case_snapshots (run_id, test_case_id, title, preconditions, gherkin, spec, created_at)
			SELECT ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM test_run_case_snapshots s
				WHERE s.run_id = ? AND s.test_case_id = ?
			)
		`, runID, c.id, c.title, c.preconditions, c.gherkin, c.spec, now, runID, c.id); err != nil {
			return fmt.Errorf("failed to snapshot BDD case %d for run: %w", c.id, err)
		}

		// One not_run result per Examples row, with the row's input values
		// persisted alongside it. A plain Scenario (no Examples) gets a
		// single implicit execution row so step-level results have a
		// uniform home for every BDD case.
		var spec gherkin.ScenarioSpec
		if err := json.Unmarshal([]byte(c.spec), &spec); err != nil {
			return fmt.Errorf("stored BDD spec for case %d is unreadable: %w", c.id, err)
		}
		type exampleRow struct {
			index  int
			values map[string]string
		}
		rowsToCreate := make([]exampleRow, 0)
		index := 0
		for _, block := range spec.Examples {
			for _, row := range block.Rows {
				values := map[string]string{}
				for i, column := range block.Header {
					if i < len(row) {
						values[column] = row[i]
					}
				}
				rowsToCreate = append(rowsToCreate, exampleRow{index: index, values: values})
				index++
			}
		}
		if len(rowsToCreate) == 0 {
			rowsToCreate = append(rowsToCreate, exampleRow{index: 0, values: map[string]string{}})
		}
		for _, row := range rowsToCreate {
			encoded, err := json.Marshal(row.values)
			if err != nil {
				return fmt.Errorf("failed to encode example values for case %d: %w", c.id, err)
			}
			if _, err := tx.Exec(`
				INSERT INTO test_example_results (run_id, test_case_id, example_index, row_values, status, actual_result, notes, created_at, updated_at)
				SELECT ?, ?, ?, ?, 'not_run', '', '', ?, ?
				WHERE NOT EXISTS (
					SELECT 1 FROM test_example_results e
					WHERE e.run_id = ? AND e.test_case_id = ? AND e.example_index = ?
				)
			`, runID, c.id, row.index, string(encoded), now, now, runID, c.id, row.index); err != nil {
				return fmt.Errorf("failed to create example result for case %d: %w", c.id, err)
			}
		}
	}
	return nil
}

// FindSnapshotsForRun returns the run's BDD specification snapshots.
func (r *TestCaseBDDRepository) FindSnapshotsForRun(runID int) ([]models.TestRunCaseSnapshot, error) {
	rows, err := r.db.Query(`
		SELECT id, run_id, test_case_id, title, COALESCE(preconditions, ''), gherkin, spec, created_at
		FROM test_run_case_snapshots
		WHERE run_id = ?
		ORDER BY test_case_id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query BDD run snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	snapshots := make([]models.TestRunCaseSnapshot, 0)
	for rows.Next() {
		var snap models.TestRunCaseSnapshot
		if err := rows.Scan(&snap.ID, &snap.RunID, &snap.TestCaseID, &snap.Title,
			&snap.Preconditions, &snap.Gherkin, &snap.Spec, &snap.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan BDD run snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate BDD run snapshots: %w", err)
	}
	return snapshots, nil
}

// FindExampleResultsForRun returns every example execution result recorded
// for a workspace-scoped run.
func (r *TestCaseBDDRepository) FindExampleResultsForRun(runID, workspaceID int) ([]models.TestExampleResult, error) {
	rows, err := r.db.Query(`
		SELECT er.id, er.run_id, er.test_case_id, er.example_index, er.row_values,
		       er.status, COALESCE(er.actual_result, ''), COALESCE(er.notes, ''),
		       er.executed_at, er.created_at, er.updated_at
		FROM test_example_results er
		JOIN test_runs run ON run.id = er.run_id
		WHERE er.run_id = ? AND run.workspace_id = ?
		ORDER BY er.test_case_id, er.example_index
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query example results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]models.TestExampleResult, 0)
	for rows.Next() {
		result, err := scanExampleResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate example results: %w", err)
	}
	return results, nil
}

type exampleResultScanner interface {
	Scan(dest ...any) error
}

func scanExampleResult(rows exampleResultScanner) (models.TestExampleResult, error) {
	var result models.TestExampleResult
	var rowValues string
	if err := rows.Scan(&result.ID, &result.RunID, &result.TestCaseID, &result.ExampleIndex,
		&rowValues, &result.Status, &result.ActualResult, &result.Notes,
		&result.ExecutedAt, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return result, fmt.Errorf("failed to scan example result: %w", err)
	}
	result.RowValues = map[string]string{}
	if rowValues != "" && rowValues != "{}" {
		if err := json.Unmarshal([]byte(rowValues), &result.RowValues); err != nil {
			return result, fmt.Errorf("failed to decode example row values: %w", err)
		}
	}
	return result, nil
}

// FindExampleResultByID returns one example result, scoped to the run's
// workspace.
func (r *TestCaseBDDRepository) FindExampleResultByID(runID, workspaceID, exampleResultID int) (*models.TestExampleResult, error) {
	row := r.db.QueryRow(`
		SELECT er.id, er.run_id, er.test_case_id, er.example_index, er.row_values,
		       er.status, COALESCE(er.actual_result, ''), COALESCE(er.notes, ''),
		       er.executed_at, er.created_at, er.updated_at
		FROM test_example_results er
		JOIN test_runs run ON run.id = er.run_id
		WHERE er.id = ? AND er.run_id = ? AND run.workspace_id = ?
	`, exampleResultID, runID, workspaceID)
	result, err := scanExampleResult(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// FindStepResultsForExample returns the recorded step results of one example
// execution ordered by step number.
func (r *TestCaseBDDRepository) FindStepResultsForExample(exampleResultID int) ([]models.TestExampleStepResult, error) {
	rows, err := r.db.Query(`
		SELECT id, example_result_id, step_number, status, COALESCE(actual_result, ''),
		       COALESCE(notes, ''), item_id, executed_at
		FROM test_example_step_results
		WHERE example_result_id = ?
		ORDER BY step_number
	`, exampleResultID)
	if err != nil {
		return nil, fmt.Errorf("failed to query example step results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]models.TestExampleStepResult, 0)
	for rows.Next() {
		var step models.TestExampleStepResult
		if err := rows.Scan(&step.ID, &step.ExampleResultID, &step.StepNumber,
			&step.Status, &step.ActualResult, &step.Notes, &step.ItemID, &step.ExecutedAt); err != nil {
			return nil, fmt.Errorf("failed to scan example step result: %w", err)
		}
		results = append(results, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate example step results: %w", err)
	}
	return results, nil
}

// UpsertExampleStepResult inserts or updates one step result of an example
// execution and derives the example status from its step results.
func (r *TestCaseBDDRepository) UpsertExampleStepResult(tx database.Tx, input *models.TestExampleStepResult) (exampleResultID int, err error) {
	now := time.Now()
	query := `
		INSERT INTO test_example_step_results (example_result_id, step_number, status, actual_result, notes, item_id, executed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (example_result_id, step_number) DO UPDATE SET
			status = excluded.status,
			actual_result = excluded.actual_result,
			notes = excluded.notes,
			item_id = excluded.item_id,
			executed_at = excluded.executed_at,
			updated_at = excluded.updated_at
	`
	_, err = tx.Exec(query, input.ExampleResultID, input.StepNumber, input.Status,
		input.ActualResult, input.Notes, input.ItemID, now, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to store example step result: %w", err)
	}

	// Derive the example status from its step statuses, mirroring the
	// existing step-to-case aggregation.
	if err := r.deriveExampleStatus(tx, input.ExampleResultID); err != nil {
		return 0, err
	}
	return input.ExampleResultID, nil
}

// deriveExampleStatus recomputes an example result's status from its step
// results using the same precedence as the step-based flow.
func (r *TestCaseBDDRepository) deriveExampleStatus(tx database.Tx, exampleResultID int) error {
	rows, err := tx.Query(`
		SELECT status FROM test_example_step_results WHERE example_result_id = ?
	`, exampleResultID)
	if err != nil {
		return fmt.Errorf("failed to load example step statuses: %w", err)
	}
	statuses := []string{}
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan example step status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate example step statuses: %w", err)
	}
	_ = rows.Close()
	if len(statuses) == 0 {
		return nil
	}
	return r.setExampleResultStatus(tx, exampleResultID, aggregateStatuses(statuses))
}

func (r *TestCaseBDDRepository) setExampleResultStatus(tx database.Tx, exampleResultID int, status string) error {
	_, err := tx.Exec(`
		UPDATE test_example_results
		SET status = ?, executed_at = COALESCE(executed_at, ?), updated_at = ?
		WHERE id = ?
	`, status, time.Now(), time.Now(), exampleResultID)
	if err != nil {
		return fmt.Errorf("failed to update example result status: %w", err)
	}
	return nil
}

// UpdateExampleResult records a direct (non-step) result for one Examples
// row, creating the row on first write, and returns its ID.
func (r *TestCaseBDDRepository) UpdateExampleResult(tx database.Tx, result *models.TestExampleResult) (int, error) {
	now := time.Now()
	rowValues, err := json.Marshal(result.RowValues)
	if err != nil {
		return 0, fmt.Errorf("failed to encode example row values: %w", err)
	}

	var id int
	err = tx.QueryRow(`
		INSERT INTO test_example_results (run_id, test_case_id, example_index, row_values, status, actual_result, notes, executed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, test_case_id, example_index) DO UPDATE SET
			status = excluded.status,
			actual_result = excluded.actual_result,
			notes = excluded.notes,
			executed_at = excluded.executed_at,
			updated_at = excluded.updated_at
		RETURNING id
	`, result.RunID, result.TestCaseID, result.ExampleIndex, string(rowValues),
		result.Status, result.ActualResult, result.Notes, now, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to store example result: %w", err)
	}
	result.ID = id
	return id, nil
}

// RecomputeCaseResult derives the case-level result in test_results from the
// example results of that case in the run, using the existing aggregation
// precedence. A failing example therefore stays visible in the aggregate.
func (r *TestCaseBDDRepository) RecomputeCaseResult(tx database.Tx, runID, testCaseID int) error {
	rows, err := tx.Query(`
		SELECT status FROM test_example_results
		WHERE run_id = ? AND test_case_id = ?
		ORDER BY example_index
	`, runID, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to load example result statuses: %w", err)
	}
	statuses := []string{}
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan example result status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate example result statuses: %w", err)
	}
	_ = rows.Close()
	if len(statuses) == 0 {
		return nil
	}

	var resultID int
	err = tx.QueryRow(`
		SELECT id FROM test_results WHERE run_id = ? AND test_case_id = ?
	`, runID, testCaseID).Scan(&resultID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find case result: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE test_results SET status = ?, updated_at = ? WHERE id = ?
	`, aggregateStatuses(statuses), time.Now(), resultID)
	if err != nil {
		return fmt.Errorf("failed to update case result: %w", err)
	}
	return nil
}

// aggregateStatuses applies the shared result precedence: any failure fails
// the aggregate, then blocked, then skipped; all-passed otherwise. Empty
// input maps to not_run.
func aggregateStatuses(statuses []string) string {
	if len(statuses) == 0 {
		return "not_run"
	}
	allPassed := true
	hasFailed, hasBlocked, hasSkipped := false, false, false
	for _, status := range statuses {
		switch status {
		case "failed":
			hasFailed, allPassed = true, false
		case "blocked":
			hasBlocked, allPassed = true, false
		case "skipped":
			hasSkipped, allPassed = true, false
		case "not_run":
			allPassed = false
		}
	}
	switch {
	case hasFailed:
		return "failed"
	case hasBlocked:
		return "blocked"
	case hasSkipped:
		return "skipped"
	case allPassed:
		return "passed"
	default:
		return "not_run"
	}
}
