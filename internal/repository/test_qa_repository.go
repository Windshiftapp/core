package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"windshift/internal/database"
)

type TestQARepository struct{ db database.Database }

func NewTestQARepository(db database.Database) *TestQARepository { return &TestQARepository{db: db} }

type LinkedItemTestResult struct {
	ID            int
	RunID         int
	RunName       string
	CaseID        int
	CaseTitle     string
	Status        string
	ExecutedAt    *time.Time
	ActualResult  string
	Notes         string
	TextTruncated bool
}

func (r *TestQARepository) LinkedItemResults(ctx context.Context, workspaceID, itemID, limit int) ([]LinkedItemTestResult, error) {
	return r.linkedItemResults(ctx, LinkedItemTestResultFilter{WorkspaceID: workspaceID, ItemID: itemID, Limit: limit}, false)
}

type LinkedItemTestResultFilter struct {
	WorkspaceID int
	ItemID      int
	ResultID    int
	Status      string
	Limit       int
	Offset      int
}

func (r *TestQARepository) LinkedItemResultDetails(ctx context.Context, filter LinkedItemTestResultFilter) ([]LinkedItemTestResult, error) {
	return r.linkedItemResults(ctx, filter, true)
}

func (r *TestQARepository) linkedItemResults(ctx context.Context, filter LinkedItemTestResultFilter, includeDetails bool) ([]LinkedItemTestResult, error) {
	const textLimit = 1000
	query := `SELECT result.id, result.run_id, run.name, tc.id, tc.title, result.status, result.executed_at`
	args := []any{}
	if includeDetails {
		query += `, SUBSTR(COALESCE(result.actual_result, ''), 1, ?), SUBSTR(COALESCE(result.notes, ''), 1, ?)`
		args = append(args, textLimit+1, textLimit+1)
	}
	query += ` FROM test_result_items link JOIN test_results result ON result.id = link.test_result_id
		JOIN test_runs run ON run.id = result.run_id JOIN test_sets plan ON plan.id = run.set_id
		JOIN test_cases tc ON tc.id = result.test_case_id
		WHERE link.item_id = ? AND run.workspace_id = ? AND plan.workspace_id = ? AND tc.workspace_id = ?
		AND result.status IN ('passed', 'failed', 'blocked', 'skipped')`
	args = append(args, filter.ItemID, filter.WorkspaceID, filter.WorkspaceID, filter.WorkspaceID)
	if filter.ResultID > 0 {
		query += " AND result.id = ?"
		args = append(args, filter.ResultID)
	}
	if filter.Status != "" {
		query += " AND result.status = ?"
		args = append(args, filter.Status)
	}
	query += " ORDER BY result.executed_at DESC NULLS LAST, result.id DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []LinkedItemTestResult
	for rows.Next() {
		var result LinkedItemTestResult
		fields := []any{&result.ID, &result.RunID, &result.RunName, &result.CaseID, &result.CaseTitle, &result.Status, &result.ExecutedAt}
		if includeDetails {
			fields = append(fields, &result.ActualResult, &result.Notes)
		}
		if err := rows.Scan(fields...); err != nil {
			return nil, err
		}
		if includeDetails {
			actual, notes := []rune(result.ActualResult), []rune(result.Notes)
			result.TextTruncated = len(actual) > textLimit || len(notes) > textLimit
			if result.TextTruncated {
				result.ActualResult = string(actual[:min(textLimit, len(actual))])
				result.Notes = string(notes[:min(textLimit, len(notes))])
			}
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

type TestCoverageCase struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type TestCriterionCoverage struct {
	ItemID         int                `json:"item_id"`
	Title          string             `json:"title"`
	Covered        bool               `json:"covered"`
	TestCases      []TestCoverageCase `json:"test_cases"`
	TotalTestCases int                `json:"total_test_cases"`
}

func (r *TestQARepository) CoverageItem(workspaceID, itemID int) (TestCriterionCoverage, error) {
	item := TestCriterionCoverage{ItemID: itemID, TestCases: []TestCoverageCase{}}
	err := r.db.QueryRow("SELECT title FROM items WHERE id = ? AND workspace_id = ?", itemID, workspaceID).Scan(&item.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (r *TestQARepository) CriterionItems(workspaceID, itemID int) ([]TestCriterionCoverage, error) {
	rows, err := r.db.Query("SELECT id, title FROM items WHERE parent_id = ? AND workspace_id = ? ORDER BY id LIMIT 101", itemID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []TestCriterionCoverage{}
	for rows.Next() {
		var item TestCriterionCoverage
		if err := rows.Scan(&item.ItemID, &item.Title); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const criterionCaseWhere = ` FROM test_cases tc WHERE tc.workspace_id = ? AND tc.status <> 'inactive'
	AND EXISTS (SELECT 1 FROM item_links il JOIN link_types lt ON lt.id = il.link_type_id
	WHERE lt.builtin_key = 'tests' AND (
	(il.source_type = 'item' AND il.source_id = ? AND il.target_type = 'test_case' AND il.target_id = tc.id) OR
	(il.target_type = 'item' AND il.target_id = ? AND il.source_type = 'test_case' AND il.source_id = tc.id)))`

func (r *TestQARepository) FillCriterionCases(workspaceID int, item *TestCriterionCoverage) error {
	if err := r.db.QueryRow("SELECT COUNT(*)"+criterionCaseWhere, workspaceID, item.ItemID, item.ItemID).Scan(&item.TotalTestCases); err != nil {
		return err
	}
	item.Covered = item.TotalTestCases > 0
	item.TestCases = []TestCoverageCase{}
	rows, err := r.db.Query("SELECT tc.id, tc.title, tc.status"+criterionCaseWhere+" ORDER BY tc.id LIMIT 50", workspaceID, item.ItemID, item.ItemID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tc TestCoverageCase
		if err := rows.Scan(&tc.ID, &tc.Title, &tc.Status); err != nil {
			return err
		}
		item.TestCases = append(item.TestCases, tc)
	}
	return rows.Err()
}

type TestEffectivenessFilter struct {
	WorkspaceID int
	MilestoneID *int
	Since       time.Time
	Until       time.Time
	Limit       int
	Offset      int
}

type TestCaseEffectiveness struct {
	TestCaseID              int      `json:"test_case_id"`
	Title                   string   `json:"title"`
	Status                  string   `json:"status"`
	Executions              int      `json:"executions"`
	Passed                  int      `json:"passed"`
	Failed                  int      `json:"failed"`
	Blocked                 int      `json:"blocked"`
	Skipped                 int      `json:"skipped"`
	LinkedDefects           int      `json:"linked_defects"`
	DefectFindingExecutions int      `json:"defect_finding_executions"`
	DefectFindingRate       *float64 `json:"defect_finding_rate"`
	Evidence                string   `json:"evidence"`
}

// Effectiveness aggregates executions separately from links to avoid an inflated denominator.
func (r *TestQARepository) Effectiveness(filter TestEffectivenessFilter) ([]TestCaseEffectiveness, int, error) {
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM test_cases WHERE workspace_id = ?", filter.WorkspaceID).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `WITH executions AS (
		SELECT res.id, res.test_case_id, res.status FROM test_results res
		JOIN test_runs run ON run.id = res.run_id JOIN test_sets plan ON plan.id = run.set_id
		JOIN test_cases tc ON tc.id = res.test_case_id
		WHERE run.workspace_id = ? AND tc.workspace_id = ? AND plan.workspace_id = ?
		AND res.executed_at >= ? AND res.executed_at < ? AND res.status IN ('passed', 'failed', 'blocked', 'skipped')`
	args := []any{filter.WorkspaceID, filter.WorkspaceID, filter.WorkspaceID, filter.Since, filter.Until}
	if filter.MilestoneID != nil {
		query += " AND plan.milestone_id = ?"
		args = append(args, *filter.MilestoneID)
	}
	query += `), defects AS (
		SELECT DISTINCT e.id AS result_id, e.test_case_id, i.id AS item_id FROM executions e
		JOIN test_result_items ri ON ri.test_result_id = e.id JOIN items i ON i.id = ri.item_id
		JOIN item_types it ON it.id = i.item_type_id
		WHERE e.status = 'failed' AND i.workspace_id = ? AND it.builtin_key = 'bug'
	), stats AS (
		SELECT test_case_id, COUNT(*) AS executions,
		SUM(CASE WHEN status = 'passed' THEN 1 ELSE 0 END) AS passed,
		SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed,
		SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked,
		SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END) AS skipped FROM executions GROUP BY test_case_id
	), findings AS (
		SELECT test_case_id, COUNT(DISTINCT item_id) AS defects, COUNT(DISTINCT result_id) AS findings FROM defects GROUP BY test_case_id
	)
	SELECT tc.id, tc.title, tc.status, COALESCE(s.executions, 0), COALESCE(s.passed, 0), COALESCE(s.failed, 0),
	COALESCE(s.blocked, 0), COALESCE(s.skipped, 0), COALESCE(f.defects, 0), COALESCE(f.findings, 0)
	FROM test_cases tc LEFT JOIN stats s ON s.test_case_id = tc.id LEFT JOIN findings f ON f.test_case_id = tc.id
	WHERE tc.workspace_id = ? ORDER BY COALESCE(f.findings, 0) DESC, COALESCE(f.defects, 0) DESC, tc.id LIMIT ? OFFSET ?`
	args = append(args, filter.WorkspaceID, filter.WorkspaceID, filter.Limit, filter.Offset)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []TestCaseEffectiveness{}
	for rows.Next() {
		var item TestCaseEffectiveness
		if err := rows.Scan(&item.TestCaseID, &item.Title, &item.Status, &item.Executions, &item.Passed, &item.Failed, &item.Blocked, &item.Skipped, &item.LinkedDefects, &item.DefectFindingExecutions); err != nil {
			return nil, 0, err
		}
		item.Evidence = "no_executions"
		if item.Executions > 0 {
			rate := float64(item.DefectFindingExecutions) / float64(item.Executions)
			item.DefectFindingRate = &rate
			item.Evidence = "no_linked_defects"
			if item.Failed > 0 {
				item.Evidence = "failures_without_linked_defects"
			}
			if item.LinkedDefects > 0 {
				item.Evidence = "observed_defect_finding"
			}
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}
