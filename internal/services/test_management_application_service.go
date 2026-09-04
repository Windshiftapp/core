package services

import (
	"errors"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/testsummary"
)

var ErrTestManagementForbidden = errors.New("test management access forbidden")

type TestFolderPatch struct {
	Name, Description         *string
	ParentID                  *int
	ParentIDSet, SortOrderSet bool
	SortOrder                 int
}

type TestCasePatch struct {
	Title, Preconditions, Priority, Status *string
	EstimatedDuration, SortOrder           *int
	FolderID                               *int
	FolderIDSet                            bool
}

type TestStepPatch struct {
	Action, Data, Expected *string
}

type TestLabelPatch struct {
	Name, Color, Description *string
}

type TestSetPatch struct {
	Name, Description *string
	MilestoneID       *int
	MilestoneIDSet    bool
}

type TestRunTemplatePatch struct {
	Name, Description *string
	SetID             *int
}

type TestRunPatch struct {
	Name          *string
	AssigneeID    *int
	AssigneeIDSet bool
}

// TestManagementApplicationService owns authorization and orchestration for
// the test-management HTTP surfaces.
type TestManagementApplicationService struct {
	db          database.Database
	permissions *PermissionService
	folders     *TestFolderService
	cases       *TestCaseService
	sets        *TestSetService
	templates   *TestRunTemplateService
	runs        *TestRunService
	summaries   *repository.TestSummaryRepository
	coverage    *repository.TestCoverageRepository
}

func NewTestManagementApplicationService(db database.Database, permissions *PermissionService) *TestManagementApplicationService {
	return &TestManagementApplicationService{
		db:          db,
		permissions: permissions,
		folders:     NewTestFolderService(db),
		cases:       NewTestCaseService(db),
		sets:        NewTestSetService(db),
		templates:   NewTestRunTemplateService(db),
		runs:        NewTestRunService(db),
		summaries:   repository.NewTestSummaryRepository(db),
		coverage:    repository.NewTestCoverageRepository(db),
	}
}

type TestCoverageScope struct {
	WorkspaceID  int
	CollectionID *int
}

type TestCoverageRequirementsFilter struct {
	Covered    string
	ItemTypeID *int
	Search     string
	Limit      int
	Offset     int
}

func (s *TestManagementApplicationService) requireCoverage(userID int, scope TestCoverageScope, permission string) error {
	workspaceID := scope.WorkspaceID
	if scope.CollectionID != nil {
		resolved, err := s.coverage.GetCollectionWorkspaceID(*scope.CollectionID)
		if err != nil {
			return err
		}
		if workspaceID != 0 && resolved != workspaceID {
			return repository.ErrNotFound
		}
		workspaceID = resolved
	}
	return s.require(userID, workspaceID, permission)
}

func (s *TestManagementApplicationService) CoverageConfig(userID int, scope TestCoverageScope) (*models.TestCoverageConfiguration, error) {
	if err := s.requireCoverage(userID, scope, models.PermissionTestView); err != nil {
		return nil, err
	}
	if scope.CollectionID != nil {
		return s.coverage.FindConfigForCollection(*scope.CollectionID)
	}
	return s.coverage.FindConfigForWorkspace(scope.WorkspaceID)
}

func (s *TestManagementApplicationService) CreateCoverageConfig(userID int, scope TestCoverageScope, typeIDs []int) (*models.TestCoverageConfiguration, error) {
	if err := s.requireCoverage(userID, scope, models.PermissionTestManage); err != nil {
		return nil, err
	}
	if scope.CollectionID != nil {
		return s.coverage.CreateConfigForCollection(*scope.CollectionID, typeIDs)
	}
	return s.coverage.CreateConfigForWorkspace(scope.WorkspaceID, typeIDs)
}

func (s *TestManagementApplicationService) UpdateCoverageConfig(userID int, scope TestCoverageScope, configID int, typeIDs []int) (*models.TestCoverageConfiguration, error) {
	if err := s.requireCoverage(userID, scope, models.PermissionTestManage); err != nil {
		return nil, err
	}
	if err := s.requireCoverageConfigScope(scope, configID); err != nil {
		return nil, err
	}
	return s.coverage.UpdateConfig(configID, typeIDs)
}

func (s *TestManagementApplicationService) DeleteCoverageConfig(userID int, scope TestCoverageScope, configID int) error {
	if err := s.requireCoverage(userID, scope, models.PermissionTestManage); err != nil {
		return err
	}
	if err := s.requireCoverageConfigScope(scope, configID); err != nil {
		return err
	}
	return s.coverage.DeleteConfig(configID)
}

func (s *TestManagementApplicationService) requireCoverageConfigScope(scope TestCoverageScope, configID int) error {
	config, err := s.coverage.FindConfigByID(configID)
	if err != nil {
		return err
	}
	if scope.CollectionID != nil && config.CollectionID != nil && *scope.CollectionID == *config.CollectionID {
		return nil
	}
	if scope.CollectionID == nil && config.CollectionID == nil && config.WorkspaceID != nil && scope.WorkspaceID == *config.WorkspaceID {
		return nil
	}
	return repository.ErrNotFound
}

func (s *TestManagementApplicationService) CoverageSummary(userID int, scope TestCoverageScope) (models.TestCoverageSummary, error) {
	if err := s.requireCoverage(userID, scope, models.PermissionTestView); err != nil {
		return models.TestCoverageSummary{}, err
	}
	typeIDs, workspaceID, err := s.coverageTypeIDs(scope)
	if errors.Is(err, repository.ErrNotFound) || len(typeIDs) == 0 {
		return models.TestCoverageSummary{}, nil
	}
	if err != nil {
		return models.TestCoverageSummary{}, err
	}
	total, covered, err := s.coverage.GetCoverageSummary(workspaceID, typeIDs)
	return coverageSummary(total, covered), err
}

func (s *TestManagementApplicationService) CoverageRequirements(userID int, scope TestCoverageScope, filter TestCoverageRequirementsFilter) ([]models.RequirementCoverageItem, int, models.TestCoverageSummary, error) {
	if err := s.requireCoverage(userID, scope, models.PermissionTestView); err != nil {
		return nil, 0, models.TestCoverageSummary{}, err
	}
	typeIDs, workspaceID, err := s.coverageTypeIDs(scope)
	if errors.Is(err, repository.ErrNotFound) || len(typeIDs) == 0 {
		return []models.RequirementCoverageItem{}, 0, models.TestCoverageSummary{}, nil
	}
	if err != nil {
		return nil, 0, models.TestCoverageSummary{}, err
	}
	if filter.ItemTypeID != nil {
		matched := false
		for _, id := range typeIDs {
			if id == *filter.ItemTypeID {
				matched = true
				break
			}
		}
		if !matched {
			return []models.RequirementCoverageItem{}, 0, models.TestCoverageSummary{}, nil
		}
		typeIDs = []int{*filter.ItemTypeID}
	}
	params := repository.RequirementListParams{WorkspaceID: workspaceID, TypeIDs: typeIDs, CoveredFilter: filter.Covered, Search: filter.Search, Limit: filter.Limit, Offset: filter.Offset}
	total, err := s.coverage.CountRequirements(params)
	if err != nil {
		return nil, 0, models.TestCoverageSummary{}, err
	}
	items, err := s.coverage.ListRequirements(params)
	if err != nil {
		return nil, 0, models.TestCoverageSummary{}, err
	}
	summaryTotal, summaryCovered, err := s.coverage.GetCoverageSummary(workspaceID, typeIDs)
	return items, total, coverageSummary(summaryTotal, summaryCovered), err
}

func (s *TestManagementApplicationService) coverageTypeIDs(scope TestCoverageScope) (ids []int, workspaceID int, err error) {
	if scope.CollectionID != nil {
		return s.coverage.GetRequirementTypeIDsForCollection(*scope.CollectionID)
	}
	ids, err = s.coverage.GetRequirementTypeIDsForWorkspace(scope.WorkspaceID)
	return ids, scope.WorkspaceID, err
}

func coverageSummary(total, covered int) models.TestCoverageSummary {
	result := models.TestCoverageSummary{Total: total, Covered: covered, NotCovered: total - covered}
	if total > 0 {
		result.CoverageRate = float64(covered) / float64(total) * 100
	}
	return result
}

func (s *TestManagementApplicationService) RunMarkdownSummary(userID, workspaceID, runID int) (map[string]string, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	header, err := s.summaries.FindMarkdownRunHeader(runID, workspaceID)
	if err != nil {
		return nil, err
	}
	results, err := s.summaries.FindMarkdownResults(runID, workspaceID)
	if err != nil {
		return nil, err
	}
	return map[string]string{"markdown": testsummary.RenderMarkdown(header, results)}, nil
}

func (s *TestManagementApplicationService) ReportsSummary(userID, workspaceID int, milestoneID *int, days int) (map[string]any, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	filter := repository.ReportFilter{WorkspaceID: workspaceID, MilestoneID: milestoneID, StartDate: time.Now().AddDate(0, 0, -days)}
	stats, err := s.summaries.GetOverallStats(filter)
	if err != nil {
		return nil, err
	}
	trend, err := s.summaries.GetTrend(filter)
	if err != nil {
		return nil, err
	}
	failures, err := s.summaries.GetRecentFailures(filter, 20)
	if err != nil {
		return nil, err
	}
	blocked, err := s.summaries.GetRecentBlocked(filter, 20)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"overall": map[string]any{
			"total_runs": stats.TotalRuns, "total_tests": stats.TotalTests, "passed": stats.Passed,
			"failed": stats.Failed, "blocked": stats.Blocked, "skipped": stats.Skipped,
			"not_run": stats.NotRun, "pass_rate": stats.PassRate(),
		},
		"trend": trend, "recent_failures": failures, "recent_blocked": blocked,
	}, nil
}

func (s *TestManagementApplicationService) require(userID, workspaceID int, permission string) error {
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTestManagementForbidden
	}
	return nil
}

func (s *TestManagementApplicationService) ListFolders(userID, workspaceID int) ([]models.TestFolder, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.folders.List(workspaceID)
}

func (s *TestManagementApplicationService) GetFolder(userID, workspaceID, id int) (*models.TestFolder, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.folders.Get(workspaceID, id)
}

func (s *TestManagementApplicationService) CreateFolder(userID, workspaceID int, actor AuditActor, folder models.TestFolder) (*models.TestFolder, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	created, err := s.folders.Create(workspaceID, folder)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestFolderCreate, logger.ResourceTestFolder, &created.ID, created.Name, nil)
	return &created, nil
}

func (s *TestManagementApplicationService) UpdateFolder(userID, workspaceID, id int, actor AuditActor, patch TestFolderPatch) (*models.TestFolder, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	existing, err := s.folders.Get(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}
	if patch.ParentIDSet {
		existing.ParentID = patch.ParentID
	}
	if patch.SortOrderSet {
		existing.SortOrder = patch.SortOrder
	}
	updated, err := s.folders.Update(workspaceID, id, TestFolderUpdateInput{
		Folder: *existing, ParentProvided: patch.ParentIDSet, SortOrderProvided: patch.SortOrderSet,
	})
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestFolderUpdate, logger.ResourceTestFolder, &updated.ID, updated.Name, nil)
	return &updated, nil
}

func (s *TestManagementApplicationService) DeleteFolder(userID, workspaceID, id int, actor AuditActor) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	folder, err := s.folders.Get(workspaceID, id)
	if err != nil {
		return err
	}
	if err := s.folders.Delete(workspaceID, id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestFolderDelete, logger.ResourceTestFolder, &id, folder.Name, nil)
	return nil
}

func (s *TestManagementApplicationService) ReorderFolders(userID, workspaceID int, ids []int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.folders.Reorder(workspaceID, ids)
}

func (s *TestManagementApplicationService) ListCases(userID int, params TestCaseListParams) ([]models.TestCase, int, error) {
	if err := s.require(userID, params.WorkspaceID, models.PermissionTestView); err != nil {
		return nil, 0, err
	}
	items, err := s.cases.List(params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.cases.Count(params)
	return items, total, err
}

func (s *TestManagementApplicationService) GetCase(userID, workspaceID, id int) (*models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.cases.GetByID(id, workspaceID)
}

func (s *TestManagementApplicationService) CreateCase(userID, workspaceID int, actor AuditActor, input TestCaseCreateRequest) (*models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	created, err := s.cases.Create(workspaceID, input)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestCaseCreate, logger.ResourceTestCase, &created.ID, created.Title, nil)
	return created, nil
}

func (s *TestManagementApplicationService) UpdateCase(userID, workspaceID, id int, actor AuditActor, patch TestCasePatch) (*models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	existing, err := s.cases.GetByID(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if patch.Title != nil {
		existing.Title = *patch.Title
	}
	if patch.Preconditions != nil {
		existing.Preconditions = *patch.Preconditions
	}
	if patch.Priority != nil {
		existing.Priority = *patch.Priority
	}
	if patch.Status != nil {
		existing.Status = *patch.Status
	}
	if patch.EstimatedDuration != nil {
		existing.EstimatedDuration = *patch.EstimatedDuration
	}
	if patch.SortOrder != nil {
		existing.SortOrder = *patch.SortOrder
	}
	if patch.FolderIDSet {
		existing.FolderID = patch.FolderID
	}
	updated, err := s.cases.Update(id, workspaceID, TestCaseUpdateRequest{
		Title: existing.Title, Preconditions: existing.Preconditions, Priority: existing.Priority,
		Status: existing.Status, EstimatedDuration: existing.EstimatedDuration,
		FolderID: existing.FolderID, SortOrder: existing.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestCaseUpdate, logger.ResourceTestCase, &updated.ID, updated.Title, nil)
	return updated, nil
}

func (s *TestManagementApplicationService) DeleteCase(userID, workspaceID, id int, actor AuditActor) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	item, err := s.cases.GetByID(id, workspaceID)
	if err != nil {
		return err
	}
	if err := s.cases.Delete(id, workspaceID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestCaseDelete, logger.ResourceTestCase, &id, item.Title, nil)
	return nil
}

func (s *TestManagementApplicationService) MoveCase(userID, workspaceID, id int, folderID *int, sortOrder int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.cases.Move(id, workspaceID, folderID, sortOrder)
}

func (s *TestManagementApplicationService) ReorderCases(userID, workspaceID int, ids []int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.cases.Reorder(workspaceID, ids)
}

func (s *TestManagementApplicationService) CaseConnections(userID, workspaceID, id int) (any, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.cases.GetConnections(id, workspaceID)
}

func (s *TestManagementApplicationService) ListSteps(userID, workspaceID, caseID int) ([]models.TestStep, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return nil, err
	}
	return s.cases.GetSteps(caseID)
}

func (s *TestManagementApplicationService) CreateStep(userID, workspaceID, caseID int, input TestStepCreateRequest) (*models.TestStep, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return nil, err
	}
	return s.cases.CreateStep(caseID, input)
}

func (s *TestManagementApplicationService) UpdateStep(userID, workspaceID, caseID, stepID int, patch TestStepPatch) (*models.TestStep, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return nil, err
	}
	steps, err := s.cases.GetSteps(caseID)
	if err != nil {
		return nil, err
	}
	var current *models.TestStep
	for i := range steps {
		if steps[i].ID == stepID {
			current = &steps[i]
			break
		}
	}
	if current == nil {
		return nil, repository.ErrNotFound
	}
	if patch.Action != nil {
		current.Action = *patch.Action
	}
	if patch.Data != nil {
		current.Data = *patch.Data
	}
	if patch.Expected != nil {
		current.Expected = *patch.Expected
	}
	return s.cases.UpdateStep(stepID, caseID, TestStepUpdateRequest{
		StepNumber: current.StepNumber, Action: current.Action, Data: current.Data, Expected: current.Expected,
	})
}

func (s *TestManagementApplicationService) DeleteStep(userID, workspaceID, caseID, stepID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return err
	}
	return s.cases.DeleteStep(stepID, caseID)
}

func (s *TestManagementApplicationService) ReorderSteps(userID, workspaceID, caseID int, ids []int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return err
	}
	return s.cases.ReorderSteps(caseID, ids)
}

func (s *TestManagementApplicationService) ListLabels(userID, workspaceID int) ([]models.TestLabel, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.cases.GetAllLabels(workspaceID)
}

func (s *TestManagementApplicationService) CreateLabel(userID, workspaceID int, input TestLabelCreateRequest) (*models.TestLabel, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	return s.cases.CreateLabel(workspaceID, input)
}

func (s *TestManagementApplicationService) UpdateLabel(userID, workspaceID, id int, patch TestLabelPatch) (*models.TestLabel, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	labels, err := s.cases.GetAllLabels(workspaceID)
	if err != nil {
		return nil, err
	}
	var current *models.TestLabel
	for i := range labels {
		if labels[i].ID == id {
			current = &labels[i]
			break
		}
	}
	if current == nil {
		return nil, repository.ErrNotFound
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Color != nil {
		current.Color = *patch.Color
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	return s.cases.UpdateLabel(id, workspaceID, TestLabelUpdateRequest{
		Name: current.Name, Color: current.Color, Description: current.Description,
	})
}

func (s *TestManagementApplicationService) DeleteLabel(userID, workspaceID, id int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.cases.DeleteLabel(id, workspaceID)
}

func (s *TestManagementApplicationService) ListCaseLabels(userID, workspaceID, caseID int) ([]models.TestLabel, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return nil, err
	}
	return s.cases.GetLabelsForTestCase(caseID)
}

func (s *TestManagementApplicationService) AddCaseLabel(userID, workspaceID, caseID, labelID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.cases.AddLabelToTestCase(caseID, labelID, workspaceID)
}

func (s *TestManagementApplicationService) RemoveCaseLabel(userID, workspaceID, caseID, labelID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	if _, err := s.cases.GetByID(caseID, workspaceID); err != nil {
		return err
	}
	return s.cases.RemoveLabelFromTestCase(caseID, labelID)
}

func (s *TestManagementApplicationService) ListSets(userID, workspaceID int) ([]models.TestSet, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.sets.List(workspaceID)
}

func (s *TestManagementApplicationService) GetSet(userID, workspaceID, id int) (*models.TestSet, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.sets.Get(id, workspaceID)
}

func (s *TestManagementApplicationService) CreateSet(userID, workspaceID int, actor AuditActor, input models.TestSet) (*models.TestSet, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	created, err := s.sets.Create(workspaceID, input)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestSetCreate, logger.ResourceTestSet, &created.ID, created.Name, nil)
	return created, nil
}

func (s *TestManagementApplicationService) UpdateSet(userID, workspaceID, id int, actor AuditActor, patch TestSetPatch) (*models.TestSet, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	existing, err := s.sets.Get(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}
	if patch.MilestoneIDSet {
		existing.MilestoneID = patch.MilestoneID
	}
	updated, err := s.sets.Update(id, workspaceID, *existing)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestSetUpdate, logger.ResourceTestSet, &updated.ID, updated.Name, nil)
	return updated, nil
}

func (s *TestManagementApplicationService) DeleteSet(userID, workspaceID, id int, actor AuditActor) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	item, err := s.sets.Get(id, workspaceID)
	if err != nil {
		return err
	}
	if err := s.sets.Delete(id, workspaceID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestSetDelete, logger.ResourceTestSet, &id, item.Name, nil)
	return nil
}

func (s *TestManagementApplicationService) ListSetCases(userID, workspaceID, id int) ([]models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.sets.ListCases(id, workspaceID)
}

func (s *TestManagementApplicationService) AddSetCase(userID, workspaceID, id, caseID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.sets.AddCase(id, caseID, workspaceID)
}

func (s *TestManagementApplicationService) RemoveSetCase(userID, workspaceID, id, caseID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.sets.RemoveCase(id, caseID, workspaceID)
}

func (s *TestManagementApplicationService) ListSetRuns(userID, workspaceID, id int) ([]models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.sets.ListRuns(id, workspaceID)
}

func (s *TestManagementApplicationService) ListTemplates(userID, workspaceID int) ([]models.TestRunTemplate, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.templates.List(workspaceID)
}

func (s *TestManagementApplicationService) GetTemplate(userID, workspaceID, id int) (*models.TestRunTemplate, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.templates.Get(id, workspaceID)
}

func (s *TestManagementApplicationService) CreateTemplate(userID, workspaceID int, input models.TestRunTemplate) (*models.TestRunTemplate, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	return s.templates.Create(workspaceID, input)
}

func (s *TestManagementApplicationService) UpdateTemplate(userID, workspaceID, id int, patch TestRunTemplatePatch) (*models.TestRunTemplate, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	existing, err := s.templates.Get(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}
	if patch.SetID != nil {
		existing.SetID = *patch.SetID
	}
	return s.templates.Update(id, workspaceID, *existing)
}

func (s *TestManagementApplicationService) DeleteTemplate(userID, workspaceID, id int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.templates.Delete(id, workspaceID)
}

func (s *TestManagementApplicationService) ListTemplateExecutions(userID, workspaceID, id int) ([]models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.templates.ListExecutions(id, workspaceID)
}

func (s *TestManagementApplicationService) ExecuteTemplate(userID, workspaceID, id int) (*models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return nil, err
	}
	return s.templates.Execute(id, workspaceID)
}

func (s *TestManagementApplicationService) ListRuns(userID, workspaceID int, filters TestRunListFilters) ([]models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.List(workspaceID, filters)
}

func (s *TestManagementApplicationService) GetRun(userID, workspaceID, id int) (*models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.GetByID(id, workspaceID)
}

func (s *TestManagementApplicationService) GetRunDetail(userID, workspaceID, id int) (*TestRunDetail, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.GetDetail(id, workspaceID)
}

func (s *TestManagementApplicationService) CreateRun(userID, workspaceID int, actor AuditActor, input TestRunCreateRequest) (*models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return nil, err
	}
	created, err := s.runs.Create(workspaceID, input)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestRunCreate, logger.ResourceTestRun, &created.ID, created.Name, nil)
	return created, nil
}

func (s *TestManagementApplicationService) UpdateRun(userID, workspaceID, id int, actor AuditActor, patch TestRunPatch) (*models.TestRun, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return nil, err
	}
	existing, err := s.runs.GetByID(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.AssigneeIDSet {
		existing.AssigneeID = patch.AssigneeID
	}
	updated, err := s.runs.Update(id, workspaceID, TestRunUpdateRequest{Name: existing.Name, AssigneeID: existing.AssigneeID})
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestRunUpdate, logger.ResourceTestRun, &updated.ID, updated.Name, nil)
	return updated, nil
}

func (s *TestManagementApplicationService) DeleteRun(userID, workspaceID, id int, actor AuditActor) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	run, err := s.runs.GetByID(id, workspaceID)
	if err != nil {
		return err
	}
	if err := s.runs.Delete(id, workspaceID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestRunDelete, logger.ResourceTestRun, &id, run.Name, nil)
	return nil
}

func (s *TestManagementApplicationService) EndRun(userID, workspaceID, id int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return err
	}
	return s.runs.Complete(id, workspaceID)
}

func (s *TestManagementApplicationService) ListResults(userID, workspaceID, runID int) ([]TestRunResultWithCaseTitle, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.ListResults(runID, workspaceID)
}

func (s *TestManagementApplicationService) UpdateResult(userID, workspaceID, runID, resultID int, input TestResultUpdateRequest) (*models.TestResult, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return nil, err
	}
	return s.runs.UpdateResult(workspaceID, runID, resultID, input)
}

func (s *TestManagementApplicationService) ListStepResults(userID, workspaceID, runID int) (map[string]TestRunStepResult, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.ListStepResults(runID, workspaceID)
}

func (s *TestManagementApplicationService) UpdateStepResult(userID, workspaceID, runID, stepID int, input TestStepResultUpdateRequest) error {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return err
	}
	return s.runs.UpdateStepResult(workspaceID, runID, stepID, input)
}

func (s *TestManagementApplicationService) LinkResultItem(userID, workspaceID, resultID, itemID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return err
	}
	return s.runs.LinkResultItem(workspaceID, resultID, itemID)
}

func (s *TestManagementApplicationService) UnlinkResultItem(userID, workspaceID, resultID, itemID int) error {
	if err := s.require(userID, workspaceID, models.PermissionTestExecute); err != nil {
		return err
	}
	return s.runs.UnlinkResultItem(workspaceID, resultID, itemID)
}

func (s *TestManagementApplicationService) ListResultItems(userID, workspaceID, resultID int) ([]models.Item, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.ListResultItems(workspaceID, resultID)
}
