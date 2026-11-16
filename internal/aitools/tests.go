// Test-management tools share v2 services and enforce workspace test permissions
// on top of the accessible-workspace gate. Denials use not-found JSON errors.
package aitools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ----------------------------------------------------------------------------
// DTOs
// ----------------------------------------------------------------------------

type testCaseSummaryDTO struct {
	ID                int      `json:"id"`
	WorkspaceID       int      `json:"workspace_id"`
	Title             string   `json:"title"`
	Format            string   `json:"format"`
	FolderID          *int     `json:"folder_id,omitempty"`
	FolderName        string   `json:"folder_name,omitempty"`
	Priority          string   `json:"priority,omitempty"`
	Status            string   `json:"status,omitempty"`
	EstimatedDuration int      `json:"estimated_duration_seconds,omitempty"`
	Labels            []string `json:"labels,omitempty"`
}

type testStepDTO struct {
	ID         int    `json:"id"`
	StepNumber int    `json:"step_number"`
	Action     string `json:"action"`
	Data       string `json:"data,omitempty"`
	Expected   string `json:"expected,omitempty"`
}

type testCaseDetailDTO struct {
	testCaseSummaryDTO
	Preconditions string              `json:"preconditions,omitempty"`
	Steps         []testStepDTO       `json:"steps"`
	BDD           *models.TestCaseBDD `json:"bdd,omitempty"`
}

type testRunDTO struct {
	ID           int    `json:"id"`
	WorkspaceID  int    `json:"workspace_id"`
	Name         string `json:"name"`
	SetID        int    `json:"set_id,omitempty"`
	TemplateID   int    `json:"template_id,omitempty"`
	AssigneeID   *int   `json:"assignee_id,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	EndedAt      string `json:"ended_at,omitempty"`
}

type testResultDTO struct {
	ID            int    `json:"id"`
	TestCaseID    int    `json:"test_case_id"`
	TestCaseTitle string `json:"test_case_title,omitempty"`
	Status        string `json:"status"`
	ActualResult  string `json:"actual_result,omitempty"`
	Notes         string `json:"notes,omitempty"`
	ExecutedAt    string `json:"executed_at,omitempty"`
}

type testRunResultSummaryDTO struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Blocked int `json:"blocked"`
	Skipped int `json:"skipped"`
	NotRun  int `json:"not_run"`
}

func testCaseToSummary(tc *models.TestCase) testCaseSummaryDTO {
	s := testCaseSummaryDTO{
		ID:                tc.ID,
		WorkspaceID:       tc.WorkspaceID,
		Title:             tc.Title,
		Format:            tc.Format,
		FolderID:          tc.FolderID,
		FolderName:        tc.FolderName,
		Priority:          tc.Priority,
		Status:            tc.Status,
		EstimatedDuration: tc.EstimatedDuration,
	}
	for _, l := range tc.Labels {
		s.Labels = append(s.Labels, l.Name)
	}
	return s
}

func testRunToDTO(run *models.TestRun) testRunDTO {
	d := testRunDTO{
		ID:           run.ID,
		WorkspaceID:  run.WorkspaceID,
		Name:         run.Name,
		SetID:        run.SetID,
		TemplateID:   run.TemplateID,
		AssigneeID:   run.AssigneeID,
		AssigneeName: run.AssigneeName,
	}
	if !run.StartedAt.IsZero() {
		d.StartedAt = run.StartedAt.Format(time.RFC3339)
	}
	if run.EndedAt != nil {
		d.EndedAt = run.EndedAt.Format(time.RFC3339)
	}
	return d
}

func testResultToDTO(res *models.TestResult, caseTitle string) testResultDTO {
	d := testResultDTO{
		ID:            res.ID,
		TestCaseID:    res.TestCaseID,
		TestCaseTitle: caseTitle,
		Status:        res.Status,
		ActualResult:  res.ActualResult,
		Notes:         res.Notes,
	}
	if res.ExecutedAt != nil {
		d.ExecutedAt = res.ExecutedAt.Format(time.RFC3339)
	}
	return d
}

func summarizeTestResults(rows []repository.TestResultWithTestCase) testRunResultSummaryDTO {
	s := testRunResultSummaryDTO{Total: len(rows)}
	for _, row := range rows {
		switch row.Status {
		case "passed":
			s.Passed++
		case "failed":
			s.Failed++
		case "blocked":
			s.Blocked++
		case "skipped":
			s.Skipped++
		default:
			s.NotRun++
		}
	}
	return s
}

// ----------------------------------------------------------------------------
// Permission helpers
// ----------------------------------------------------------------------------

// Permission lookup failures deny access without exposing workspace existence.
func hasTestPermission(env *Env, workspaceID int, permission string) bool {
	if workspaceID <= 0 || !env.HasWorkspaceAccess(workspaceID) {
		return false
	}
	allowed, err := env.PermService.HasWorkspacePermission(env.UserID, workspaceID, permission)
	if err != nil {
		return false
	}
	return allowed
}

// resolveTestEntityWorkspace finds the workspace that owns a test entity
// when the caller didn't pass workspace_id. It only probes workspaces the
// caller holds `permission` in, so an entity in an inaccessible workspace
// stays indistinguishable from a nonexistent one. existsIn is the
// workspace-scoped existence check for the entity type.
func resolveTestEntityWorkspace(env *Env, workspaceID *int, permission string, existsIn func(workspaceID int) (bool, error)) (int, bool) {
	if workspaceID != nil && *workspaceID > 0 {
		if !hasTestPermission(env, *workspaceID, permission) {
			return 0, false
		}
		exists, err := existsIn(*workspaceID)
		if err != nil || !exists {
			return 0, false
		}
		return *workspaceID, true
	}
	for _, wsID := range env.AccessibleWorkspaceIDs {
		if !hasTestPermission(env, wsID, permission) {
			continue
		}
		exists, err := existsIn(wsID)
		if err == nil && exists {
			return wsID, true
		}
	}
	return 0, false
}

func testToolError(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// ----------------------------------------------------------------------------
// Args types
// ----------------------------------------------------------------------------

type listTestCasesArgs struct {
	WorkspaceID     int    `json:"workspace_id" jsonschema:"Workspace ID to list test cases from"`
	FolderID        *int   `json:"folder_id,omitempty" jsonschema:"Filter to a single test folder ID"`
	IncludeArchived bool   `json:"include_archived,omitempty" jsonschema:"Include archived test cases"`
	Query           string `json:"q,omitempty" jsonschema:"Case-insensitive lexical substring search across title, preconditions, steps, Gherkin, priority, status, and labels; not semantic similarity"`
	LabelID         *int   `json:"label_id,omitempty" jsonschema:"Filter by test label ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Page size: default 50, maximum 100"`
	Offset          int    `json:"offset,omitempty" jsonschema:"Nonnegative page offset"`
}

type getTestCaseArgs struct {
	TestCaseID  int  `json:"test_case_id" jsonschema:"Test case ID"`
	WorkspaceID *int `json:"workspace_id,omitempty" jsonschema:"Workspace ID the case belongs to (resolved automatically if omitted)"`
}

type listTestRunsArgs struct {
	WorkspaceID int  `json:"workspace_id" jsonschema:"Workspace ID to list test runs from"`
	SetID       *int `json:"set_id,omitempty" jsonschema:"Filter to runs started from a specific test set ID"`
	AssigneeID  *int `json:"assignee_id,omitempty" jsonschema:"Filter to runs assigned to a specific user ID"`
	Unassigned  bool `json:"unassigned,omitempty" jsonschema:"Only runs with no assignee (mutually exclusive with assignee_id)"`
}

type getTestRunArgs struct {
	RunID       int  `json:"run_id" jsonschema:"Test run ID"`
	WorkspaceID *int `json:"workspace_id,omitempty" jsonschema:"Workspace ID the run belongs to (resolved automatically if omitted)"`
}

type startTestRunArgs struct {
	SetID       *int   `json:"set_id,omitempty" jsonschema:"Test set ID to start the run from (either set_id or template_id is required)"`
	TemplateID  *int   `json:"template_id,omitempty" jsonschema:"Test run template ID to execute (alternative to set_id; run name is auto-generated)"`
	Name        string `json:"name,omitempty" jsonschema:"Custom run name (set_id runs only; defaults to '<set name> - Run')"`
	AssigneeID  *int   `json:"assignee_id,omitempty" jsonschema:"User ID to assign the run to (set_id runs only; must be a workspace member)"`
	WorkspaceID *int   `json:"workspace_id,omitempty" jsonschema:"Workspace ID (resolved automatically from the set or template if omitted)"`
}

type endTestRunArgs struct {
	RunID       int  `json:"run_id" jsonschema:"Test run ID to mark as ended"`
	WorkspaceID *int `json:"workspace_id,omitempty" jsonschema:"Workspace ID the run belongs to (resolved automatically if omitted)"`
}

type recordTestResultArgs struct {
	RunID        int    `json:"run_id" jsonschema:"Test run ID"`
	TestCaseID   int    `json:"test_case_id" jsonschema:"Test case ID whose result in the run is being recorded"`
	Status       string `json:"status" jsonschema:"Result status: passed, failed, blocked, or skipped"`
	Notes        string `json:"notes,omitempty" jsonschema:"Notes about the result (Markdown)"`
	ActualResult string `json:"actual_result,omitempty" jsonschema:"Actual observed result (Markdown)"`
	WorkspaceID  *int   `json:"workspace_id,omitempty" jsonschema:"Workspace ID the run belongs to (resolved automatically if omitted)"`
}

// ----------------------------------------------------------------------------
// Tools
// ----------------------------------------------------------------------------

func init() {
	// ------------------------------------------------------------------------
	// list_test_cases
	// ------------------------------------------------------------------------
	Register(Default, Tool[listTestCasesArgs]{
		Name:        "list_test_cases",
		Group:       CapabilityTests,
		Access:      AccessRead,
		Risk:        RiskLow,
		Scopes:      []string{auth.ScopeTestsRead},
		Description: "Search test-case summaries with bounded lexical retrieval (not semantic overlap). Defaults to 50 cases across all folders, excluding inactive cases; page using offset and total. Fetch full steps with get_test_case.",
		Run: func(_ context.Context, env *Env, args listTestCasesArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestView) {
				return testToolError("workspace not found"), nil
			}
			if args.Limit == 0 {
				args.Limit = 50
			}
			if args.Limit < 1 || args.Limit > 100 || args.Offset < 0 {
				return testToolError("limit must be 1..100 and offset must be nonnegative"), nil
			}
			if (args.FolderID != nil && *args.FolderID <= 0) || (args.LabelID != nil && *args.LabelID <= 0) {
				return testToolError("folder_id and label_id must be positive"), nil
			}
			caseSvc := services.NewTestCaseService(env.DB)
			params := services.TestCaseListParams{
				WorkspaceID: args.WorkspaceID,
				FolderID:    args.FolderID,
				All:         args.FolderID == nil, ExcludeInactive: !args.IncludeArchived,
				Search: args.Query, LabelID: args.LabelID, Limit: args.Limit, Offset: args.Offset,
			}
			cases, err := caseSvc.List(params)
			if err != nil {
				return nil, err
			}
			out := make([]testCaseSummaryDTO, 0, len(cases))
			for i := range cases {
				out = append(out, testCaseToSummary(&cases[i]))
			}
			total, err := caseSvc.Count(params)
			if err != nil {
				return nil, err
			}
			return map[string]any{"test_cases": out, "total": total, "limit": args.Limit, "offset": args.Offset, "retrieval": "lexical"}, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_test_case
	// ------------------------------------------------------------------------
	Register(Default, Tool[getTestCaseArgs]{
		Name:        "get_test_case",
		Group:       CapabilityTests,
		Access:      AccessRead,
		Risk:        RiskLow,
		Scopes:      []string{auth.ScopeTestsRead},
		Description: "Get a test case by ID, including its steps (action / data / expected result).",
		Run: func(_ context.Context, env *Env, args getTestCaseArgs) (any, error) {
			caseSvc := services.NewTestCaseService(env.DB)
			wsID, ok := resolveTestEntityWorkspace(env, args.WorkspaceID, models.PermissionTestView, func(wsID int) (bool, error) {
				return caseSvc.Exists(args.TestCaseID, wsID)
			})
			if !ok {
				return testToolError("test case not found"), nil
			}
			tc, err := caseSvc.GetByID(args.TestCaseID, wsID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return testToolError("test case not found"), nil
				}
				return nil, err
			}
			steps, err := caseSvc.GetSteps(args.TestCaseID)
			if err != nil {
				return nil, err
			}
			labels, err := caseSvc.GetLabelsForTestCase(args.TestCaseID)
			if err == nil {
				tc.Labels = labels
			}
			detail := testCaseDetailDTO{
				testCaseSummaryDTO: testCaseToSummary(tc),
				Preconditions:      tc.Preconditions,
				Steps:              make([]testStepDTO, 0, len(steps)),
			}
			for _, step := range steps {
				detail.Steps = append(detail.Steps, testStepDTO{
					ID:         step.ID,
					StepNumber: step.StepNumber,
					Action:     step.Action,
					Data:       step.Data,
					Expected:   step.Expected,
				})
			}
			if tc.Format == services.TestFormatBDD {
				detail.BDD, err = caseSvc.GetBDDContent(tc.ID)
				if err != nil {
					return nil, err
				}
			}
			return detail, nil
		},
	})

	// ------------------------------------------------------------------------
	// list_test_runs
	// ------------------------------------------------------------------------
	Register(Default, Tool[listTestRunsArgs]{
		Name:        "list_test_runs",
		Group:       CapabilityTests,
		Access:      AccessRead,
		Risk:        RiskLow,
		Scopes:      []string{auth.ScopeTestsRead},
		Description: "List test runs in a workspace, optionally filtered by test set or assignee.",
		Run: func(_ context.Context, env *Env, args listTestRunsArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestView) {
				return testToolError("workspace not found"), nil
			}
			runs, err := services.NewTestRunService(env.DB).List(args.WorkspaceID, services.TestRunListFilters{
				AssigneeID:   args.AssigneeID,
				Unassigned:   args.Unassigned,
				SetID:        args.SetID,
				IncludeEnded: true,
			})
			if err != nil {
				return nil, err
			}
			out := make([]testRunDTO, 0, len(runs))
			for i := range runs {
				out = append(out, testRunToDTO(&runs[i]))
			}
			return map[string]any{"test_runs": out, "total": len(out)}, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_test_run
	// ------------------------------------------------------------------------
	Register(Default, Tool[getTestRunArgs]{
		Name:        "get_test_run",
		Group:       CapabilityTests,
		Access:      AccessRead,
		Risk:        RiskLow,
		Scopes:      []string{auth.ScopeTestsRead},
		Description: "Get a test run by ID, including its per-test-case results and a pass/fail summary.",
		Run: func(_ context.Context, env *Env, args getTestRunArgs) (any, error) {
			runSvc := services.NewTestRunService(env.DB)
			wsID, ok := resolveTestEntityWorkspace(env, args.WorkspaceID, models.PermissionTestView, func(wsID int) (bool, error) {
				return runSvc.Exists(args.RunID, wsID)
			})
			if !ok {
				return testToolError("test run not found"), nil
			}
			run, err := runSvc.GetByID(args.RunID, wsID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return testToolError("test run not found"), nil
				}
				return nil, err
			}
			rows, err := repository.NewTestRunRepository(env.DB).FindResultsWithTestCase(args.RunID, wsID)
			if err != nil {
				return nil, err
			}
			results := make([]testResultDTO, 0, len(rows))
			for i := range rows {
				results = append(results, testResultToDTO(&rows[i].TestResult, rows[i].TestCaseTitle))
			}
			return map[string]any{
				"run":     testRunToDTO(run),
				"results": results,
				"summary": summarizeTestResults(rows),
			}, nil
		},
	})

	// ------------------------------------------------------------------------
	// start_test_run
	// ------------------------------------------------------------------------
	Register(Default, Tool[startTestRunArgs]{
		Name:        "start_test_run",
		Group:       CapabilityTests,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Scopes:      []string{auth.ScopeTestsWrite},
		Description: "Start a new test run from a test set (set_id) or a saved run template (template_id). Initializes a not_run result for every test case in the set.",
		Run: func(_ context.Context, env *Env, args startTestRunArgs) (any, error) {
			switch {
			case args.TemplateID != nil && *args.TemplateID > 0:
				return startTestRunFromTemplate(env, args)
			case args.SetID != nil && *args.SetID > 0:
				return startTestRunFromSet(env, args)
			default:
				return testToolError("either set_id or template_id is required"), nil
			}
		},
	})

	// ------------------------------------------------------------------------
	// end_test_run
	// ------------------------------------------------------------------------
	Register(Default, Tool[endTestRunArgs]{
		Name:        "end_test_run",
		Group:       CapabilityTests,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Scopes:      []string{auth.ScopeTestsWrite},
		Description: "Mark a test run as ended and return its final result summary.",
		Run: func(_ context.Context, env *Env, args endTestRunArgs) (any, error) {
			runSvc := services.NewTestRunService(env.DB)
			wsID, ok := resolveTestEntityWorkspace(env, args.WorkspaceID, models.PermissionTestExecute, func(wsID int) (bool, error) {
				return runSvc.Exists(args.RunID, wsID)
			})
			if !ok {
				return testToolError("test run not found"), nil
			}
			if err := runSvc.Complete(args.RunID, wsID); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return testToolError("test run not found"), nil
				}
				return nil, err
			}
			env.AuditWrite(logger.ResourceTestRun, args.RunID, "end_test_run", "")
			run, err := runSvc.GetByID(args.RunID, wsID)
			if err != nil {
				return map[string]any{"success": true}, nil //nolint:nilerr // run ended; summary fetch is best-effort
			}
			rows, err := repository.NewTestRunRepository(env.DB).FindResultsWithTestCase(args.RunID, wsID)
			if err != nil {
				return map[string]any{"success": true, "run": testRunToDTO(run)}, nil //nolint:nilerr // run ended; summary fetch is best-effort
			}
			return map[string]any{
				"success": true,
				"run":     testRunToDTO(run),
				"summary": summarizeTestResults(rows),
			}, nil
		},
	})

	// ------------------------------------------------------------------------
	// record_test_result
	// ------------------------------------------------------------------------
	Register(Default, Tool[recordTestResultArgs]{
		Name:        "record_test_result",
		Group:       CapabilityTests,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Scopes:      []string{auth.ScopeTestsWrite},
		Description: "Record the result of a test case in a test run (status passed | failed | blocked | skipped, with optional notes and actual result).",
		Run: func(_ context.Context, env *Env, args recordTestResultArgs) (any, error) {
			switch args.Status {
			case "passed", "failed", "blocked", "skipped":
			default:
				return testToolError("invalid status: must be passed, failed, blocked, or skipped"), nil
			}
			runSvc := services.NewTestRunService(env.DB)
			wsID, ok := resolveTestEntityWorkspace(env, args.WorkspaceID, models.PermissionTestExecute, func(wsID int) (bool, error) {
				return runSvc.Exists(args.RunID, wsID)
			})
			if !ok {
				return testToolError("test run not found"), nil
			}
			// Resolve test case -> result row in this run, mirroring how the
			// CLI's `ws test result` finds the record before the v1 PUT.
			rows, err := repository.NewTestRunRepository(env.DB).FindResultsWithTestCase(args.RunID, wsID)
			if err != nil {
				return nil, err
			}
			var target *repository.TestResultWithTestCase
			for i := range rows {
				if rows[i].TestCaseID == args.TestCaseID {
					target = &rows[i]
					break
				}
			}
			if target == nil {
				return testToolError(fmt.Sprintf("test case %d not found in run %d", args.TestCaseID, args.RunID)), nil
			}
			updatedFields, err := runSvc.UpdateResult(wsID, args.RunID, target.ID, services.TestResultUpdateRequest{
				Status:       args.Status,
				ActualResult: args.ActualResult,
				Notes:        args.Notes,
			})
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return testToolError("test result not found"), nil
				}
				return testToolError(fmt.Sprintf("failed to record result: %s", err.Error())), nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			env.AuditWrite(resourceTestResult, target.ID, "record_test_result",
				fmt.Sprintf("%s: %s", target.TestCaseTitle, args.Status))
			updated := target.TestResult
			updated.Status = updatedFields.Status
			updated.ActualResult = updatedFields.ActualResult
			updated.Notes = updatedFields.Notes
			updated.ExecutedAt = updatedFields.ExecutedAt
			return map[string]any{
				"success": true,
				"result":  testResultToDTO(&updated, target.TestCaseTitle),
			}, nil
		},
	})
}

// ----------------------------------------------------------------------------
// start_test_run helpers
// ----------------------------------------------------------------------------

// startTestRunFromSet mirrors POST /workspaces/{id}/test-runs (and the CLI's
// `ws test run start <set>`): the run name defaults to "<set name> - Run".
func startTestRunFromSet(env *Env, args startTestRunArgs) (any, error) {
	setID := *args.SetID
	setSvc := services.NewTestSetService(env.DB)
	wsID, err := setSvc.GetWorkspaceID(setID)
	if err != nil || !hasTestPermission(env, wsID, models.PermissionTestExecute) {
		return testToolError("test set not found"), nil //nolint:nilerr // 404-style JSON error, never reveal existence
	}
	if args.WorkspaceID != nil && *args.WorkspaceID > 0 && *args.WorkspaceID != wsID {
		return testToolError("test set not found"), nil
	}
	name := args.Name
	if name == "" {
		set, err := setSvc.Get(setID, wsID)
		if err != nil {
			return testToolError("test set not found"), nil //nolint:nilerr // 404-style JSON error
		}
		name = set.Name + " - Run"
	}
	run, err := services.NewTestRunService(env.DB).Create(wsID, services.TestRunCreateRequest{
		Name:       name,
		SetID:      setID,
		AssigneeID: args.AssigneeID,
	})
	if err != nil {
		return testToolError(err.Error()), nil //nolint:nilerr // validation error as tool JSON
	}
	env.AuditWrite(logger.ResourceTestRun, run.ID, "start_test_run", run.Name)
	return map[string]any{"success": true, "run": testRunToDTO(run)}, nil
}

// startTestRunFromTemplate mirrors POST /workspaces/{id}/test-run-templates/{id}/execute:
// the run name is auto-generated as "<template name> - Run <N>".
func startTestRunFromTemplate(env *Env, args startTestRunArgs) (any, error) {
	templateID := *args.TemplateID
	templateSvc := services.NewTestRunTemplateService(env.DB)
	wsID, ok := resolveTestEntityWorkspace(env, args.WorkspaceID, models.PermissionTestExecute, func(wsID int) (bool, error) {
		return templateSvc.Exists(templateID, wsID)
	})
	if !ok {
		return testToolError("test run template not found"), nil
	}
	run, err := templateSvc.Execute(templateID, wsID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return testToolError("test run template not found"), nil //nolint:nilerr // 404-style JSON error
		}
		return nil, err
	}
	env.AuditWrite(logger.ResourceTestRun, run.ID, "start_test_run", run.Name)
	return map[string]any{"success": true, "run": testRunToDTO(run)}, nil
}
