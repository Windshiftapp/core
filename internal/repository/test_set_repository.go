package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestSetRepository provides data access methods for test sets.
type TestSetRepository struct {
	db database.Database
}

// NewTestSetRepository creates a new test set repository.
func NewTestSetRepository(db database.Database) *TestSetRepository {
	return &TestSetRepository{db: db}
}

// FindAllWithStats returns all test sets for a workspace with aggregated test case
// counts and run statistics joined in.
func (r *TestSetRepository) FindAllWithStats(workspaceID int) ([]models.TestSet, error) {
	rows, err := r.db.Query(`
		SELECT
			ts.id, ts.workspace_id, ts.name, ts.description, ts.milestone_id, ts.created_at, ts.updated_at,
			m.name as milestone_name,
			COALESCE(tc_count.count, 0) as test_case_count,
			COALESCE(run_stats.total_runs, 0) as total_runs,
			COALESCE(run_stats.successful_runs, 0) as successful_runs,
			COALESCE(run_stats.failed_runs, 0) as failed_runs,
			run_stats.last_run_status,
			run_stats.last_run_date
		FROM test_sets ts
		LEFT JOIN milestones m ON ts.milestone_id = m.id
		LEFT JOIN (
			SELECT set_id, COUNT(*) as count
			FROM set_test_cases
			GROUP BY set_id
		) tc_count ON ts.id = tc_count.set_id
		LEFT JOIN (
			SELECT
				set_id,
				COUNT(*) as total_runs,
				SUM(CASE WHEN ended_at IS NOT NULL THEN 1 ELSE 0 END) as successful_runs,
				SUM(CASE WHEN ended_at IS NULL THEN 1 ELSE 0 END) as failed_runs,
				CASE
					WHEN MAX(ended_at) IS NOT NULL THEN 'completed'
					WHEN COUNT(*) > 0 THEN 'in_progress'
					ELSE NULL
				END as last_run_status,
				MAX(started_at) as last_run_date
			FROM test_runs
			WHERE workspace_id = ?
			GROUP BY set_id
		) run_stats ON ts.id = run_stats.set_id
		WHERE ts.workspace_id = ?
		ORDER BY ts.id DESC
	`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	sets := make([]models.TestSet, 0)
	for rows.Next() {
		var set models.TestSet
		var milestoneName, lastRunStatus, lastRunDateStr sql.NullString

		if err := rows.Scan(
			&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.MilestoneID, &set.CreatedAt, &set.UpdatedAt,
			&milestoneName, &set.TestCaseCount, &set.TotalRuns, &set.SuccessfulRuns, &set.FailedRuns,
			&lastRunStatus, &lastRunDateStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan test set: %w", err)
		}

		if milestoneName.Valid {
			set.MilestoneName = milestoneName.String
		}
		if lastRunStatus.Valid {
			set.LastRunStatus = lastRunStatus.String
		}
		if lastRunDateStr.Valid {
			if parsed, err := time.Parse("2006-01-02 15:04:05.999999-07:00", lastRunDateStr.String); err == nil {
				set.LastRunDate = &parsed
			}
		}

		sets = append(sets, set)
	}
	return sets, nil
}

// FindByID returns a single test set scoped to workspace.
func (r *TestSetRepository) FindByID(id, workspaceID int) (*models.TestSet, error) {
	var set models.TestSet
	var milestoneName sql.NullString

	err := r.db.QueryRow(`
		SELECT ts.id, ts.workspace_id, ts.name, ts.description, ts.milestone_id, ts.created_at, ts.updated_at,
		       m.name as milestone_name
		FROM test_sets ts
		LEFT JOIN milestones m ON ts.milestone_id = m.id
		WHERE ts.id = ? AND ts.workspace_id = ?
	`, id, workspaceID).Scan(&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.MilestoneID, &set.CreatedAt, &set.UpdatedAt, &milestoneName)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test set: %w", err)
	}

	if milestoneName.Valid {
		set.MilestoneName = milestoneName.String
	}
	return &set, nil
}

// Create inserts a new test set and returns its id and timestamps.
func (r *TestSetRepository) Create(workspaceID int, set *models.TestSet) (id int, createdAt time.Time, err error) {
	now := time.Now()
	var newID int64
	err = r.db.QueryRow(`
		INSERT INTO test_sets (workspace_id, name, description, milestone_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, set.Name, set.Description, set.MilestoneID, now, now).Scan(&newID)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to create test set: %w", err)
	}
	return int(newID), now, nil
}

// Update updates an existing test set and returns the new updated_at timestamp.
func (r *TestSetRepository) Update(id, workspaceID int, set *models.TestSet) (time.Time, error) {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE test_sets
		SET name = ?, description = ?, milestone_id = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, set.Name, set.Description, set.MilestoneID, now, id, workspaceID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update test set: %w", err)
	}
	return now, nil
}

// Delete removes a test set.
func (r *TestSetRepository) Delete(id, workspaceID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM test_sets WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete test set: %w", err)
	}
	return nil
}

// FindTestCases returns test cases that belong to a set (scoped to workspace).
func (r *TestSetRepository) FindTestCases(setID, workspaceID int) ([]models.TestCase, error) {
	rows, err := r.db.Query(`
		SELECT tc.id, tc.workspace_id, tc.title, tc.preconditions, tc.created_at, tc.updated_at
		FROM test_cases tc
		JOIN set_test_cases stc ON tc.id = stc.test_case_id
		WHERE stc.set_id = ? AND tc.workspace_id = ?
		ORDER BY tc.id
	`, setID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test cases in set: %w", err)
	}
	defer func() { _ = rows.Close() }()

	testCases := make([]models.TestCase, 0)
	for rows.Next() {
		var tc models.TestCase
		if err := rows.Scan(&tc.ID, &tc.WorkspaceID, &tc.Title, &tc.Preconditions, &tc.CreatedAt, &tc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan test case row: %w", err)
		}
		testCases = append(testCases, tc)
	}
	return testCases, nil
}

// AddTestCase attaches a test case to a set.
func (r *TestSetRepository) AddTestCase(setID, testCaseID int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO set_test_cases (set_id, test_case_id)
		VALUES (?, ?)
	`, setID, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to add test case to set: %w", err)
	}
	return nil
}

// RemoveTestCase detaches a test case from a set.
func (r *TestSetRepository) RemoveTestCase(setID, testCaseID int) error {
	_, err := r.db.ExecWrite(`
		DELETE FROM set_test_cases
		WHERE set_id = ? AND test_case_id = ?
	`, setID, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to remove test case from set: %w", err)
	}
	return nil
}

// FindRuns returns test runs for a set within a workspace.
func (r *TestSetRepository) FindRuns(setID, workspaceID int) ([]models.TestRun, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE set_id = ? AND workspace_id = ?
		ORDER BY id DESC
	`, setID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query runs for set: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		if err := rows.Scan(&run.ID, &run.WorkspaceID, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan test run row: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}
