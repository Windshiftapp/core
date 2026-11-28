package aitools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type itemTestResultsArgs struct {
	itemRef
	ResultID int    `json:"result_id,omitempty" jsonschema:"Exact result ID already linked to this item"`
	Status   string `json:"status,omitempty" jsonschema:"Optional recorded status: passed, failed, blocked, or skipped"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Page size: default 10, maximum 20"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Offset returned by the previous page"`
}

type itemTestResult struct {
	ID            int        `json:"result_id"`
	RunID         int        `json:"run_id"`
	Run           string     `json:"run"`
	CaseID        int        `json:"test_case_id"`
	Case          string     `json:"case"`
	Status        string     `json:"status"`
	ExecutedAt    *time.Time `json:"executed_at,omitempty"`
	ActualResult  string     `json:"actual_result,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	TextTruncated bool       `json:"text_truncated"`
	URL           string     `json:"url"`
}

func listItemTestResults(ctx context.Context, env *Env, args itemTestResultsArgs) (any, error) {
	if args.ResultID < 0 || args.Limit < 0 || args.Offset < 0 {
		return nil, errors.New("result_id, limit, and offset must be nonnegative")
	}
	switch args.Status {
	case "", "passed", "failed", "blocked", "skipped":
	default:
		return nil, errors.New("status must be passed, failed, blocked, or skipped")
	}
	item, err := args.read(ctx, env)
	if err != nil {
		return nil, err
	}
	allowed, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionTestView)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}

	limit, offset := args.Limit, args.Offset
	if limit == 0 {
		limit = 10
	}
	limit = min(limit, 20)
	if args.ResultID > 0 {
		limit, offset = 1, 0
	}
	rows, err := repository.NewTestQARepository(env.DB).LinkedItemResultDetails(ctx, repository.LinkedItemTestResultFilter{
		WorkspaceID: item.WorkspaceID, ItemID: item.ID, ResultID: args.ResultID, Status: args.Status,
		Limit: limit + 1, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	if args.ResultID > 0 && len(rows) == 0 {
		return nil, repository.ErrNotFound
	}
	results := make([]itemTestResult, 0, min(len(rows), limit))
	for _, row := range rows[:min(len(rows), limit)] {
		results = append(results, itemTestResult{
			ID: row.ID, RunID: row.RunID, Run: row.RunName, CaseID: row.CaseID, Case: row.CaseTitle,
			Status: row.Status, ExecutedAt: row.ExecutedAt, ActualResult: row.ActualResult, Notes: row.Notes,
			TextTruncated: row.TextTruncated, URL: fmt.Sprintf("/workspaces/%d/tests/runs/%d", item.WorkspaceID, row.RunID),
		})
	}
	out := map[string]any{"item_url": itemURL(item), "results": results, "has_more": len(rows) > limit}
	if len(rows) > limit {
		out["next_offset"] = offset + limit
	}
	return out, nil
}

func init() {
	registerRead("list_item_test_results",
		"Read recorded test results explicitly linked to a task, including notes, observed results, and links to the runs. Use result_id from a delivery summary to read one result, or filter and page through several.",
		CapabilityTests, []string{auth.ScopeItemsRead, auth.ScopeTestsRead}, listItemTestResults)
}
