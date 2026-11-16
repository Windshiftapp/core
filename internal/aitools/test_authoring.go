package aitools

import (
	"context"
	"errors"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type createTestCaseArgs struct {
	WorkspaceID int `json:"workspace_id"`
	services.TestCaseCreateRequest
}

type updateTestCaseArgs struct {
	WorkspaceID int `json:"workspace_id"`
	TestCaseID  int `json:"test_case_id"`
	services.TestCaseBulkPatch
	EstimatedDuration *int    `json:"estimated_duration,omitempty"`
	Gherkin           *string `json:"gherkin,omitempty"`
}

type archiveTestCaseArgs struct {
	WorkspaceID int `json:"workspace_id"`
	TestCaseID  int `json:"test_case_id"`
}

type bulkTestCasesArgs struct {
	WorkspaceID int `json:"workspace_id"`
	services.TestCaseBulkRequest
}

type itemTestCoverageArgs struct {
	WorkspaceID int `json:"workspace_id"`
	ItemID      int `json:"item_id"`
}

type testEffectivenessArgs struct {
	WorkspaceID int    `json:"workspace_id"`
	MilestoneID *int   `json:"milestone_id,omitempty"`
	Since       string `json:"since,omitempty" jsonschema:"Inclusive RFC3339 execution timestamp; defaults to 30 days before until"`
	Until       string `json:"until,omitempty" jsonschema:"Exclusive RFC3339 execution timestamp; defaults to now; window maximum 366 days"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Page size: default 50, maximum 100"`
	Offset      int    `json:"offset,omitempty"`
}

func testApplication(env *Env) *services.TestManagementApplicationService {
	return services.NewTestManagementApplicationService(env.DB, env.PermService)
}

func testManagementToolResult(value any, err error, notFound string) (any, error) {
	if err == nil {
		return value, nil
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrTestManagementForbidden) {
		return testToolError(notFound), nil
	}
	var validation *services.TestManagementValidationError
	var bddValidation *services.TestBDDValidationError
	if errors.As(err, &validation) || errors.As(err, &bddValidation) {
		return testToolError(err.Error()), nil
	}
	return nil, err
}

func init() {
	Register(Default, Tool[createTestCaseArgs]{
		Name: "create_test_case", Group: CapabilityTests, Access: AccessWrite, Risk: RiskMedium, Scopes: []string{auth.ScopeTestsWrite},
		Description: "Create a test case with steps (action/data/expected), labels, or BDD Gherkin. Prefer status draft until reviewed. Use only synthetic test data or safe external references, never secrets. Folder and labels must belong to this workspace.",
		Run: func(_ context.Context, env *Env, args createTestCaseArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestManage) {
				return testToolError("workspace not found"), nil
			}
			value, err := testApplication(env).CreateCase(env.UserID, args.WorkspaceID, pageAuditActor(env), args.TestCaseCreateRequest)
			return testManagementToolResult(value, err, "test case not found")
		},
	})
	Register(Default, Tool[updateTestCaseArgs]{
		Name: "update_test_case", Group: CapabilityTests, Access: AccessWrite, Risk: RiskMedium, Scopes: []string{auth.ScopeTestsWrite},
		Description: "Edit a case in its workspace. Omitted fields are preserved; explicit steps/label_ids arrays replace the entire collection (empty clears). clear_folder moves to the root. Format/workspace are immutable. Review drafts before activating.",
		Run: func(_ context.Context, env *Env, args updateTestCaseArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestManage) {
				return testToolError("test case not found"), nil
			}
			if args.ClearFolder && args.FolderID != nil {
				return testToolError("folder_id and clear_folder are mutually exclusive"), nil
			}
			value, err := testApplication(env).UpdateCase(env.UserID, args.WorkspaceID, args.TestCaseID, pageAuditActor(env), services.TestCasePatch{
				Title: args.Title, Preconditions: args.Preconditions, Priority: args.Priority, Status: args.Status, EstimatedDuration: args.EstimatedDuration,
				FolderID: args.FolderID, FolderIDSet: args.ClearFolder || args.FolderID != nil, Steps: args.Steps, LabelIDs: args.LabelIDs, Gherkin: args.Gherkin,
			})
			return testManagementToolResult(value, err, "test case not found")
		},
	})
	Register(Default, Tool[archiveTestCaseArgs]{
		Name: "archive_test_case", Group: CapabilityTests, Access: AccessWrite, Risk: RiskMedium, Scopes: []string{auth.ScopeTestsWrite},
		Description: "Archive a test case reversibly by setting status inactive. Keeps case, links, steps, and execution history; does not hard-delete. Restore with update_test_case status active or draft.",
		Run: func(_ context.Context, env *Env, args archiveTestCaseArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestManage) {
				return testToolError("test case not found"), nil
			}
			status := "inactive"
			value, err := testApplication(env).UpdateCase(env.UserID, args.WorkspaceID, args.TestCaseID, pageAuditActor(env), services.TestCasePatch{Status: &status})
			return testManagementToolResult(value, err, "test case not found")
		},
	})
	Register(Default, Tool[bulkTestCasesArgs]{
		Name: "bulk_test_cases", Group: CapabilityTests, Access: AccessWrite, Risk: RiskMedium, Scopes: []string{auth.ScopeTestsWrite},
		Description: "Create, update (labels/folder/status/steps), or archive 1..50 cases. All references must belong to this workspace and are checked before any write. Returns ordered per-case results; validation failures may coexist with successes. Every successful case mutation is audited.",
		Run: func(_ context.Context, env *Env, args bulkTestCasesArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestManage) {
				return testToolError("workspace not found"), nil
			}
			value, err := testApplication(env).BulkCases(env.UserID, args.WorkspaceID, pageAuditActor(env), args.TestCaseBulkRequest)
			return testManagementToolResult(value, err, "test case not found")
		},
	})
	Register(Default, Tool[itemTestCoverageArgs]{
		Name: "get_item_test_coverage", Group: CapabilityTests, Access: AccessRead, Risk: RiskLow, Scopes: []string{auth.ScopeTestsRead},
		Description: "Get explicit test coverage for one ticket. Direct child items represent acceptance criteria; no children falls back to item-level coverage. Uses existing Tests links, excluding inactive/foreign cases. Does not infer coverage from prose or semantic similarity. Includes uncovered rows, totals and truncation.",
		Run: func(_ context.Context, env *Env, args itemTestCoverageArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestView) {
				return testToolError("item not found"), nil
			}
			value, err := testApplication(env).ItemCoverage(env.UserID, args.WorkspaceID, args.ItemID)
			return testManagementToolResult(value, err, "item not found")
		},
	})
	Register(Default, Tool[testEffectivenessArgs]{
		Name: "get_test_effectiveness", Group: CapabilityTests, Access: AccessRead, Risk: RiskLow, Scopes: []string{auth.ScopeTestsRead},
		Description: "Rank test cases by observed defect-finding executions in a time window. Returns distinct linked Bugs, executions, outcomes, evidence and finding rate with explicit denominator and limitations. Zero-execution and no-link cases remain visible. Offset pages are independent samples of a mutable ranking, not a consistent snapshot; concurrent edits or new links can shift rows. Not proof of causation or ineffectiveness.",
		Run: func(_ context.Context, env *Env, args testEffectivenessArgs) (any, error) {
			if !hasTestPermission(env, args.WorkspaceID, models.PermissionTestView) {
				return testToolError("workspace not found"), nil
			}
			filter := repository.TestEffectivenessFilter{WorkspaceID: args.WorkspaceID, MilestoneID: args.MilestoneID, Limit: args.Limit, Offset: args.Offset}
			var err error
			if args.Since != "" {
				filter.Since, err = time.Parse(time.RFC3339, args.Since)
				if err != nil {
					return testToolError("since must be RFC3339"), nil //nolint:nilerr // validation errors use tool JSON
				}
			}
			if args.Until != "" {
				filter.Until, err = time.Parse(time.RFC3339, args.Until)
				if err != nil {
					return testToolError("until must be RFC3339"), nil //nolint:nilerr // validation errors use tool JSON
				}
			}
			value, err := testApplication(env).Effectiveness(env.UserID, filter)
			return testManagementToolResult(value, err, "workspace not found")
		},
	})
}
