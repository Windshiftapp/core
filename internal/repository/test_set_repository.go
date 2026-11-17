package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	return r.findWithStats(workspaceID, 0, 0)
}

// FindPageWithStats counts matching plans and aggregates only the selected page.
func (r *TestSetRepository) FindPageWithStats(workspaceID, limit, offset int) ([]models.TestSet, int, error) {
	if limit <= 0 || offset < 0 {
		return nil, 0, errors.New("test set page requires a positive limit and nonnegative offset")
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM test_sets WHERE workspace_id = ?`, workspaceID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count test sets: %w", err)
	}
	sets, err := r.findWithStats(workspaceID, limit, offset)
	return sets, total, err
}

func (r *TestSetRepository) findWithStats(workspaceID, limit, offset int) ([]models.TestSet, error) {
	pageClause := ""
	args := []any{workspaceID}
	if limit > 0 {
		pageClause = " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	args = append(args, workspaceID)
	rows, err := r.db.Query(`
		WITH selected AS (
			SELECT id FROM test_sets WHERE workspace_id = ?
			ORDER BY id DESC`+pageClause+`
		)
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
		LEFT JOIN milestones m ON ts.milestone_id = m.id AND (COALESCE(m.is_global, FALSE) = TRUE OR m.workspace_id = ts.workspace_id)
		LEFT JOIN (
			SELECT set_id, COUNT(*) as count
			FROM set_test_cases
			WHERE set_id IN (SELECT id FROM selected)
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
			WHERE workspace_id = ? AND set_id IN (SELECT id FROM selected)
			GROUP BY set_id
		) run_stats ON ts.id = run_stats.set_id
		WHERE ts.id IN (SELECT id FROM selected)
		ORDER BY ts.id DESC
	`, args...)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test sets: %w", err)
	}
	return sets, nil
}

// FindStatsByMilestoneIDs aggregates test-plan activity grouped by milestone.
// Test-set contributions are restricted to workspaceIDs (fail-closed: empty
// workspaceIDs exclude all test sets) and milestone rows are filtered to the
// caller's visibility (global milestones plus own workspace).
func (r *TestSetRepository) FindStatsByMilestoneIDs(milestoneIDs, workspaceIDs []int) (map[int]models.MilestoneTestStats, error) {
	uniqueIDs := make([]int, 0, len(milestoneIDs))
	seen := make(map[int]struct{}, len(milestoneIDs))
	for _, milestoneID := range milestoneIDs {
		if milestoneID <= 0 {
			continue
		}
		if _, exists := seen[milestoneID]; exists {
			continue
		}
		seen[milestoneID] = struct{}{}
		uniqueIDs = append(uniqueIDs, milestoneID)
	}
	result := make(map[int]models.MilestoneTestStats, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	milestonePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(uniqueIDs)), ",")
	testSetWorkspaceClause := " AND 1=0"
	var testSetWorkspaceArgs []any
	if len(workspaceIDs) > 0 {
		workspacePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
		testSetWorkspaceClause = " AND ts.workspace_id IN (" + workspacePlaceholders + ")"
		testSetWorkspaceArgs = make([]any, len(workspaceIDs))
		for i, workspaceID := range workspaceIDs {
			testSetWorkspaceArgs[i] = workspaceID
		}
	}
	visibilityClause := "m.is_global = true"
	visibilityArgs := []any{}
	if len(workspaceIDs) > 0 {
		workspacePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
		visibilityClause += " OR m.workspace_id IN (" + workspacePlaceholders + ")"
		visibilityArgs = make([]any, len(workspaceIDs))
		for i, workspaceID := range workspaceIDs {
			visibilityArgs[i] = workspaceID
		}
	}
	queryArgs := make([]any, 0, len(testSetWorkspaceArgs)+len(uniqueIDs)+len(visibilityArgs))
	queryArgs = append(queryArgs, testSetWorkspaceArgs...)
	for _, milestoneID := range uniqueIDs {
		queryArgs = append(queryArgs, milestoneID)
	}
	queryArgs = append(queryArgs, visibilityArgs...)

	rows, err := r.db.Query(`
		SELECT
			m.id,
			COUNT(DISTINCT ts.id) as total_test_plans,
			COALESCE(SUM(run_stats.total_runs), 0) as total_test_runs,
			COALESCE(SUM(run_stats.successful_runs), 0) as successful_test_runs,
			COALESCE(SUM(run_stats.failed_runs), 0) as failed_test_runs,
			COALESCE(SUM(run_stats.in_progress_runs), 0) as in_progress_test_runs,
			COALESCE(SUM(tc_counts.test_case_count), 0) as total_test_cases
		FROM milestones m
		LEFT JOIN test_sets ts ON ts.milestone_id = m.id`+testSetWorkspaceClause+`
		LEFT JOIN (
			SELECT
				runs.set_id,
				COUNT(*) as total_runs,
				SUM(CASE
					WHEN runs.ended_at IS NOT NULL
					 AND COALESCE(results.result_count, 0) > 0
					 AND COALESCE(results.non_success_count, 0) = 0
					THEN 1 ELSE 0
				END) as successful_runs,
				SUM(CASE
					WHEN runs.ended_at IS NOT NULL
					 AND (
						COALESCE(results.result_count, 0) = 0
						OR COALESCE(results.non_success_count, 0) > 0
					 )
					THEN 1 ELSE 0
				END) as failed_runs,
				SUM(CASE WHEN runs.ended_at IS NULL THEN 1 ELSE 0 END) as in_progress_runs
			FROM test_runs runs
			LEFT JOIN (
				SELECT
					run_id,
					COUNT(*) as result_count,
					SUM(CASE WHEN status IN ('passed', 'skipped') THEN 0 ELSE 1 END) as non_success_count
				FROM test_results
				GROUP BY run_id
			) results ON results.run_id = runs.id
			GROUP BY runs.set_id
		) run_stats ON ts.id = run_stats.set_id
		LEFT JOIN (
			SELECT
				stc.set_id,
				COUNT(stc.test_case_id) as test_case_count
			FROM set_test_cases stc
			GROUP BY stc.set_id
		) tc_counts ON ts.id = tc_counts.set_id
		WHERE m.id IN (`+milestonePlaceholders+`)
		  AND (`+visibilityClause+`)
		GROUP BY m.id
	`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get milestone test statistics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var milestoneID int
		var stats models.MilestoneTestStats
		if err := rows.Scan(
			&milestoneID,
			&stats.TotalTestPlans,
			&stats.TotalTestRuns,
			&stats.SuccessfulTestRuns,
			&stats.FailedTestRuns,
			&stats.InProgressTestRuns,
			&stats.TotalTestCases,
		); err != nil {
			return nil, fmt.Errorf("failed to scan milestone test statistics: %w", err)
		}
		result[milestoneID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate milestone test statistics: %w", err)
	}
	return result, nil
}

// GetWorkspaceID resolves the owning workspace for a test set.
func (r *TestSetRepository) GetWorkspaceID(id int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow(`SELECT workspace_id FROM test_sets WHERE id = ?`, id).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to resolve test set workspace: %w", err)
	}
	return workspaceID, nil
}

// FindByID returns a single test set scoped to workspace.
func (r *TestSetRepository) FindByID(id, workspaceID int) (*models.TestSet, error) {
	var set models.TestSet
	var milestoneName sql.NullString

	err := r.db.QueryRow(`
		SELECT ts.id, ts.workspace_id, ts.name, ts.description, ts.milestone_id, ts.created_at, ts.updated_at,
		       m.name as milestone_name
		FROM test_sets ts
		LEFT JOIN milestones m ON ts.milestone_id = m.id AND (COALESCE(m.is_global, FALSE) = TRUE OR m.workspace_id = ts.workspace_id)
		WHERE ts.id = ? AND ts.workspace_id = ?
	`, id, workspaceID).Scan(&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.MilestoneID, &set.CreatedAt, &set.UpdatedAt, &milestoneName)

	if errors.Is(err, sql.ErrNoRows) {
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
	result, err := r.db.ExecWrite(`
		UPDATE test_sets
		SET name = ?, description = ?, milestone_id = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, set.Name, set.Description, set.MilestoneID, now, id, workspaceID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update test set: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return time.Time{}, ErrNotFound
	}
	return now, nil
}

// Delete removes a test set.
func (r *TestSetRepository) Delete(id, workspaceID int) error {
	result, err := r.db.ExecWrite(`DELETE FROM test_sets WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete test set: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test cases in set: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runs for set: %w", err)
	}
	return runs, nil
}
