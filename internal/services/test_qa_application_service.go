package services

import (
	"errors"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

type ItemTestCoverage struct {
	ItemID             int                                `json:"item_id"`
	Basis              string                             `json:"basis"`
	Criteria           []repository.TestCriterionCoverage `json:"criteria"`
	ItemTestCases      []repository.TestCoverageCase      `json:"item_test_cases"`
	ItemTotalTestCases int                                `json:"item_total_test_cases"`
	Covered            int                                `json:"covered"`
	Uncovered          int                                `json:"uncovered"`
	Truncated          bool                               `json:"truncated"`
	Limitation         string                             `json:"limitation"`
}

func (s *TestManagementApplicationService) ItemCoverage(userID, workspaceID, itemID int) (ItemTestCoverage, error) {
	result := ItemTestCoverage{ItemID: itemID, Basis: "direct_children", Limitation: "Explicit Tests links only; direct children represent criteria. Prose is not parsed. A link is not proof of test adequacy. At most 100 criteria and 50 cases per criterion are returned; counts include all matching cases."}
	for _, permission := range []string{models.PermissionTestView, models.PermissionItemView} {
		if err := s.require(userID, workspaceID, permission); err != nil {
			return result, err
		}
	}
	repo := repository.NewTestQARepository(s.db)
	item, err := repo.CoverageItem(workspaceID, itemID)
	if err != nil {
		return result, err
	}
	if err := repo.FillCriterionCases(workspaceID, &item); err != nil {
		return result, err
	}
	result.ItemTestCases, result.ItemTotalTestCases = item.TestCases, item.TotalTestCases
	criteria, err := repo.CriterionItems(workspaceID, itemID)
	if err != nil {
		return result, err
	}
	if len(criteria) == 0 {
		criteria = []repository.TestCriterionCoverage{item}
		result.Basis = "item"
	}
	if len(criteria) > 100 {
		result.Truncated = true
		criteria = criteria[:100]
	}
	for i := range criteria {
		if err := repo.FillCriterionCases(workspaceID, &criteria[i]); err != nil {
			return result, err
		}
		if criteria[i].Covered {
			result.Covered++
		} else {
			result.Uncovered++
		}
		if criteria[i].TotalTestCases > len(criteria[i].TestCases) {
			result.Truncated = true
		}
	}
	result.Truncated = result.Truncated || item.TotalTestCases > len(item.TestCases)
	result.Criteria = criteria
	return result, nil
}

type TestEffectivenessReport struct {
	Cases       []repository.TestCaseEffectiveness `json:"cases"`
	Total       int                                `json:"total"`
	Limit       int                                `json:"limit"`
	Offset      int                                `json:"offset"`
	Since       time.Time                          `json:"since"`
	Until       time.Time                          `json:"until"`
	MilestoneID *int                               `json:"milestone_id"`
	Definition  string                             `json:"definition"`
}

func (s *TestManagementApplicationService) Effectiveness(userID int, filter repository.TestEffectivenessFilter) (TestEffectivenessReport, error) {
	result := TestEffectivenessReport{}
	if err := s.require(userID, filter.WorkspaceID, models.PermissionTestView); err != nil {
		return result, err
	}
	if filter.Until.IsZero() {
		filter.Until = time.Now().UTC()
	}
	if filter.Since.IsZero() {
		filter.Since = filter.Until.AddDate(0, 0, -30)
	}
	if !filter.Since.Before(filter.Until) || filter.Until.Sub(filter.Since) > 366*24*time.Hour {
		return result, &TestManagementValidationError{Msg: "since must precede until by at most 366 days"}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 {
		return result, &TestManagementValidationError{Msg: "limit must be 1..100 and offset must be nonnegative"}
	}
	if filter.MilestoneID != nil {
		var exists bool
		if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM milestones WHERE id = ? AND (workspace_id = ? OR is_global = true))", *filter.MilestoneID, filter.WorkspaceID).Scan(&exists); err != nil {
			return result, err
		}
		if !exists {
			return result, repository.ErrNotFound
		}
	}
	cases, total, err := repository.NewTestQARepository(s.db).Effectiveness(filter)
	if err != nil {
		return result, err
	}
	return TestEffectivenessReport{Cases: cases, Total: total, Limit: filter.Limit, Offset: filter.Offset, Since: filter.Since, Until: filter.Until, MilestoneID: filter.MilestoneID,
		Definition: "One execution is one persisted run/case result with executed_at in [since, until) and status passed, failed, blocked, or skipped. Findings are failed executions linked to same-workspace Bug items (builtin_key=bug). Rate is findings/executions (0..1), null for zero executions. Defects are distinct per case. All workspace cases, including inactive and zero-execution cases, remain in the denominator population; milestone filters executions via their plan. Observational evidence only: missing links are not proof of ineffectiveness; repeated edits to one result are not separate executions. Offset pages independently sample a mutable ranking, not a consistent snapshot; concurrent executions, edits or links can shift rows. Deduplicate IDs and report sampling limits; a fixed time window does not freeze edits or links."}, nil
}

const MaxTestCaseBatch = 50

type TestCaseBulkPatch struct {
	Title         *string                  `json:"title,omitempty"`
	Preconditions *string                  `json:"preconditions,omitempty"`
	Priority      *string                  `json:"priority,omitempty"`
	Status        *string                  `json:"status,omitempty"`
	FolderID      *int                     `json:"folder_id,omitempty"`
	ClearFolder   bool                     `json:"clear_folder,omitempty"`
	Steps         *[]TestStepCreateRequest `json:"steps,omitempty"`
	LabelIDs      *[]int                   `json:"label_ids,omitempty"`
}

type TestCaseBulkRequest struct {
	Operation   string                  `json:"operation"`
	Cases       []TestCaseCreateRequest `json:"cases,omitempty"`
	TestCaseIDs []int                   `json:"test_case_ids,omitempty"`
	Patch       *TestCaseBulkPatch      `json:"patch,omitempty"`
}

type TestCaseBulkItemResult struct {
	Index      int    `json:"index"`
	TestCaseID int    `json:"test_case_id,omitempty"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type TestCaseBulkResult struct {
	Results []TestCaseBulkItemResult `json:"results"`
}

func (s *TestManagementApplicationService) BulkCases(userID, workspaceID int, actor AuditActor, input TestCaseBulkRequest) (TestCaseBulkResult, error) {
	result := TestCaseBulkResult{Results: []TestCaseBulkItemResult{}}
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return result, err
	}
	count := len(input.TestCaseIDs)
	switch input.Operation {
	case "create":
		count = len(input.Cases)
		if len(input.TestCaseIDs) > 0 || input.Patch != nil {
			return result, &TestManagementValidationError{Msg: "create accepts only cases"}
		}
	case "update", "archive":
		if len(input.Cases) > 0 || (input.Operation == "update" && input.Patch == nil) || (input.Operation == "archive" && input.Patch != nil) {
			return result, &TestManagementValidationError{Msg: "update requires a patch; archive accepts only test_case_ids"}
		}
	default:
		return result, &TestManagementValidationError{Msg: "operation must be create, update, or archive"}
	}
	if count < 1 || count > MaxTestCaseBatch {
		return result, &TestManagementValidationError{Msg: "batch must contain 1..50 cases"}
	}
	// Preflight every reference; authored-content validation remains per case so other cases can succeed.
	seen := map[int]bool{}
	for _, id := range input.TestCaseIDs {
		if seen[id] {
			return result, &TestManagementValidationError{Msg: "test_case_ids must be unique"}
		}
		seen[id] = true
		if _, err := s.cases.GetByID(id, workspaceID); err != nil {
			return result, err
		}
	}
	for _, tc := range input.Cases {
		if err := s.cases.validateFolderInWorkspace(workspaceID, tc.FolderID); err != nil {
			return result, err
		}
		if err := s.cases.validateAuthoredContent(workspaceID, tc.Format, nil, &tc.LabelIDs); err != nil {
			return result, err
		}
	}
	patch := TestCasePatch{}
	if input.Patch != nil {
		p := input.Patch
		if p.ClearFolder && p.FolderID != nil {
			return result, &TestManagementValidationError{Msg: "folder_id and clear_folder are mutually exclusive"}
		}
		if err := s.cases.validateFolderInWorkspace(workspaceID, p.FolderID); err != nil {
			return result, err
		}
		if err := s.cases.validateAuthoredContent(workspaceID, "", nil, p.LabelIDs); err != nil {
			return result, err
		}
		patch = TestCasePatch{Title: p.Title, Preconditions: p.Preconditions, Priority: p.Priority, Status: p.Status, FolderID: p.FolderID, FolderIDSet: p.ClearFolder || p.FolderID != nil, Steps: p.Steps, LabelIDs: p.LabelIDs}
	}
	if input.Operation == "archive" {
		status := "inactive"
		patch.Status = &status
	}
	for i := 0; i < count; i++ {
		entry := TestCaseBulkItemResult{Index: i}
		var err error
		if input.Operation == "create" {
			var created *models.TestCase
			created, err = s.CreateCase(userID, workspaceID, actor, input.Cases[i])
			if err == nil {
				entry.TestCaseID = created.ID
			}
		} else {
			entry.TestCaseID = input.TestCaseIDs[i]
			_, err = s.UpdateCase(userID, workspaceID, entry.TestCaseID, actor, patch)
		}
		entry.Success = err == nil
		if err != nil {
			entry.Error = "operation failed"
			var validation *TestManagementValidationError
			var bddValidation *TestBDDValidationError
			if errors.As(err, &validation) || errors.As(err, &bddValidation) {
				entry.Error = err.Error()
			}
			if errors.Is(err, repository.ErrNotFound) || errors.Is(err, ErrTestManagementForbidden) {
				entry.Error = "test case not found"
			}
		}
		result.Results = append(result.Results, entry)
	}
	return result, nil
}
