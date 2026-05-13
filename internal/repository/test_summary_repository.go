package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// TestSummaryRepository provides read-only aggregate queries for test run
// reporting (markdown summaries and the workspace-level test reports page).
type TestSummaryRepository struct {
	db database.Database
}

// NewTestSummaryRepository creates a new test summary repository.
func NewTestSummaryRepository(db database.Database) *TestSummaryRepository {
	return &TestSummaryRepository{db: db}
}

// MarkdownRunHeader is the run-level metadata rendered above a markdown summary.
type MarkdownRunHeader struct {
	RunName   string
	SetName   string
	StartedAt sql.NullTime
	EndedAt   sql.NullTime
}

// MarkdownResult represents a single test case result in the markdown summary.
type MarkdownResult struct {
	Title        string
	Status       string
	ActualResult string
	Notes        string
}

// FindMarkdownRunHeader returns the run/set names and timestamps used as the
// header of the markdown summary. Returns ErrNotFound if the run does not exist
// or does not belong to the given workspace.
func (r *TestSummaryRepository) FindMarkdownRunHeader(runID, workspaceID int) (*MarkdownRunHeader, error) {
	var header MarkdownRunHeader
	err := r.db.QueryRow(`
		SELECT tr.name, tr.started_at, tr.ended_at, ts.name
		FROM test_runs tr
		JOIN test_sets ts ON tr.set_id = ts.id
		WHERE tr.id = ? AND tr.workspace_id = ?
	`, runID, workspaceID).Scan(&header.RunName, &header.StartedAt, &header.EndedAt, &header.SetName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load markdown run header: %w", err)
	}
	return &header, nil
}

// FindMarkdownResults returns the per-test results for a markdown summary.
// Results are restricted to runs belonging to the given workspace.
func (r *TestSummaryRepository) FindMarkdownResults(runID, workspaceID int) ([]MarkdownResult, error) {
	rows, err := r.db.Query(`
		SELECT tc.title, tres.status, tres.actual_result, tres.notes
		FROM test_results tres
		JOIN test_cases tc ON tres.test_case_id = tc.id
		JOIN test_runs tr ON tres.run_id = tr.id
		WHERE tres.run_id = ? AND tr.workspace_id = ?
		ORDER BY tc.id
	`, runID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query markdown results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]MarkdownResult, 0)
	for rows.Next() {
		var res MarkdownResult
		var actualResult, notes sql.NullString
		if err := rows.Scan(&res.Title, &res.Status, &actualResult, &notes); err != nil {
			return nil, fmt.Errorf("scan markdown result: %w", err)
		}
		if actualResult.Valid {
			res.ActualResult = actualResult.String
		}
		if notes.Valid {
			res.Notes = notes.String
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

// ReportFilter scopes the workspace-level test reports queries.
type ReportFilter struct {
	WorkspaceID int
	MilestoneID *int
	StartDate   time.Time
}

// OverallStats is the per-filter aggregate count used by the reports header.
type OverallStats struct {
	TotalRuns  int
	TotalTests int
	Passed     int64
	Failed     int64
	Blocked    int64
	Skipped    int64
	NotRun     int64
}

// PassRate returns the overall pass rate as a percentage, or 0 when there are no tests.
func (s OverallStats) PassRate() float64 {
	if s.TotalTests == 0 {
		return 0
	}
	return float64(s.Passed) / float64(s.TotalTests) * 100
}

// TrendPoint represents the daily pass rate in the reports trend chart.
type TrendPoint struct {
	Date     string  `json:"date"`
	PassRate float64 `json:"pass_rate"`
	Total    int     `json:"total"`
}

// RecentFailure is one row in the reports "recent failures" section.
type RecentFailure struct {
	TestCaseID    int        `json:"test_case_id"`
	TestCaseTitle string     `json:"test_case_title"`
	RunID         int        `json:"run_id"`
	RunName       string     `json:"run_name"`
	FailedAt      *time.Time `json:"failed_at"`
}

// RecentBlocked is one row in the reports "recent blocked" section.
type RecentBlocked struct {
	TestCaseID    int        `json:"test_case_id"`
	TestCaseTitle string     `json:"test_case_title"`
	RunID         int        `json:"run_id"`
	RunName       string     `json:"run_name"`
	Reason        string     `json:"reason"`
	BlockedAt     *time.Time `json:"blocked_at"`
}

// GetOverallStats returns aggregate counts for the filter.
func (r *TestSummaryRepository) GetOverallStats(filter ReportFilter) (*OverallStats, error) {
	baseWhere, baseArgs := reportBase(filter)

	query := `
		SELECT
			COUNT(DISTINCT tr.id) as total_runs,
			COUNT(tres.id) as total_tests,
			SUM(CASE WHEN tres.status = 'passed' THEN 1 ELSE 0 END) as passed,
			SUM(CASE WHEN tres.status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN tres.status = 'blocked' THEN 1 ELSE 0 END) as blocked,
			SUM(CASE WHEN tres.status = 'skipped' THEN 1 ELSE 0 END) as skipped,
			SUM(CASE WHEN tres.status = 'not_run' THEN 1 ELSE 0 END) as not_run
		` + reportBaseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		` + baseWhere

	var stats OverallStats
	var passed, failed, blocked, skipped, notRun sql.NullInt64
	err := r.db.QueryRow(query, baseArgs...).Scan(
		&stats.TotalRuns, &stats.TotalTests, &passed, &failed, &blocked, &skipped, &notRun,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load overall stats: %w", err)
	}
	stats.Passed = passed.Int64
	stats.Failed = failed.Int64
	stats.Blocked = blocked.Int64
	stats.Skipped = skipped.Int64
	stats.NotRun = notRun.Int64
	return &stats, nil
}

// GetTrend returns the daily pass-rate time series for the filter.
func (r *TestSummaryRepository) GetTrend(filter ReportFilter) ([]TrendPoint, error) {
	baseWhere, baseArgs := reportBase(filter)

	// Postgres' DATE() returns a date value that pq surfaces as time.Time, which
	// will not scan into a Go string. Cast to text explicitly on Postgres so the
	// scan target stays uniform across backends. SQLite's DATE() already returns
	// TEXT in ISO format, matching TO_CHAR's output for ORDER BY purposes.
	dateExpr := "DATE(tr.started_at)"
	if r.db.GetDriverName() == "postgres" {
		dateExpr = "TO_CHAR(tr.started_at, 'YYYY-MM-DD')"
	}

	query := `
		SELECT
			` + dateExpr + ` as date,
			COUNT(tres.id) as total,
			SUM(CASE WHEN tres.status = 'passed' THEN 1 ELSE 0 END) as passed
		` + reportBaseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		` + baseWhere + `
		GROUP BY ` + dateExpr + `
		ORDER BY date
	`

	rows, err := r.db.Query(query, baseArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	trend := make([]TrendPoint, 0)
	for rows.Next() {
		var date string
		var total int
		var passedCount sql.NullInt64
		if err := rows.Scan(&date, &total, &passedCount); err != nil {
			return nil, fmt.Errorf("scan trend row: %w", err)
		}
		var rate float64
		if total > 0 {
			rate = float64(passedCount.Int64) / float64(total) * 100
		}
		trend = append(trend, TrendPoint{Date: date, PassRate: rate, Total: total})
	}
	return trend, rows.Err()
}

// GetRecentFailures returns the most recent failed test results for the filter.
func (r *TestSummaryRepository) GetRecentFailures(filter ReportFilter, limit int) ([]RecentFailure, error) {
	baseWhere, baseArgs := reportBase(filter)

	query := `
		SELECT
			tc.id as test_case_id,
			tc.title as test_case_title,
			tr.id as run_id,
			tr.name as run_name,
			tres.executed_at as failed_at
		` + reportBaseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		LEFT JOIN test_cases tc ON tres.test_case_id = tc.id
		` + baseWhere + `
		AND tres.status = 'failed'
		ORDER BY tres.executed_at DESC
		LIMIT ?
	`

	args := append(append([]interface{}{}, baseArgs...), limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent failures: %w", err)
	}
	defer func() { _ = rows.Close() }()

	failures := make([]RecentFailure, 0)
	for rows.Next() {
		var f RecentFailure
		var failedAt sql.NullTime
		if err := rows.Scan(&f.TestCaseID, &f.TestCaseTitle, &f.RunID, &f.RunName, &failedAt); err != nil {
			return nil, fmt.Errorf("scan recent failure: %w", err)
		}
		if failedAt.Valid {
			f.FailedAt = &failedAt.Time
		}
		failures = append(failures, f)
	}
	return failures, rows.Err()
}

// GetRecentBlocked returns the most recent blocked test results for the filter.
func (r *TestSummaryRepository) GetRecentBlocked(filter ReportFilter, limit int) ([]RecentBlocked, error) {
	baseWhere, baseArgs := reportBase(filter)

	query := `
		SELECT
			tc.id as test_case_id,
			tc.title as test_case_title,
			tr.id as run_id,
			tr.name as run_name,
			tres.notes as reason,
			tres.executed_at as blocked_at
		` + reportBaseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		LEFT JOIN test_cases tc ON tres.test_case_id = tc.id
		` + baseWhere + `
		AND tres.status = 'blocked'
		ORDER BY tres.executed_at DESC
		LIMIT ?
	`

	args := append(append([]interface{}{}, baseArgs...), limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent blocked: %w", err)
	}
	defer func() { _ = rows.Close() }()

	blocked := make([]RecentBlocked, 0)
	for rows.Next() {
		var b RecentBlocked
		var reason sql.NullString
		var blockedAt sql.NullTime
		if err := rows.Scan(&b.TestCaseID, &b.TestCaseTitle, &b.RunID, &b.RunName, &reason, &blockedAt); err != nil {
			return nil, fmt.Errorf("scan recent blocked: %w", err)
		}
		if reason.Valid {
			b.Reason = reason.String
		}
		if blockedAt.Valid {
			b.BlockedAt = &blockedAt.Time
		}
		blocked = append(blocked, b)
	}
	return blocked, rows.Err()
}

// reportBaseFrom is the shared FROM/JOIN clause used by every reports query.
const reportBaseFrom = `
	FROM test_runs tr
	JOIN test_sets ts ON tr.set_id = ts.id
`

func reportBase(filter ReportFilter) (where string, args []interface{}) {
	where = `
		WHERE tr.workspace_id = ?
		AND tr.started_at >= ?
	`
	args = []interface{}{filter.WorkspaceID, filter.StartDate}
	if filter.MilestoneID != nil {
		where += " AND ts.milestone_id = ?"
		args = append(args, *filter.MilestoneID)
	}
	return where, args
}
