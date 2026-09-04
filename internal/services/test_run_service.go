package services

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// ErrTestRunItemNotFound identifies an item link that is missing or belongs to
// a different workspace. Handlers use it to preserve resource-specific 404s.
var ErrTestRunItemNotFound = errors.New("test run item not found")

// ErrInvalidTestResultStatus identifies an unsupported case or step status.
var ErrInvalidTestResultStatus = errors.New("invalid status: must be passed, failed, blocked, skipped, or not_run")

// TestRunService handles test run business logic
type TestRunService struct {
	db       database.Database
	repo     *repository.TestRunRepository
	itemRepo *repository.ItemRepository
}

// NewTestRunService creates a new test run service
func NewTestRunService(db database.Database) *TestRunService {
	return &TestRunService{
		db:       db,
		repo:     repository.NewTestRunRepository(db),
		itemRepo: repository.NewItemRepository(db),
	}
}

// TestRunResultWithCaseTitle is the shared test-result response shape.
type TestRunResultWithCaseTitle struct {
	models.TestResult
	TestCaseTitle string `json:"test_case_title"`
}

// TestRunStepResult is the shared step-result response shape.
type TestRunStepResult struct {
	StepID       int        `json:"step_id"`
	TestCaseID   int        `json:"test_case_id"`
	Status       string     `json:"status"`
	ActualResult string     `json:"actual_result"`
	Notes        string     `json:"notes"`
	ItemID       *int       `json:"item_id"`
	ExecutedAt   *time.Time `json:"executed_at"`
}

// TestRunDetail is the complete read model shared by both HTTP surfaces.
type TestRunDetail struct {
	Run         *models.TestRun              `json:"run"`
	TestCases   []models.TestCase            `json:"test_cases"`
	Results     []TestRunResultWithCaseTitle `json:"results"`
	StepResults map[string]TestRunStepResult `json:"step_results"`
}

// TestRunListFilters contains filter parameters for listing test runs
type TestRunListFilters struct {
	AssigneeID   *int
	Unassigned   bool
	TemplateID   *int
	SetID        *int
	IncludeEnded bool
}

// List retrieves test runs with optional filters
func (s *TestRunService) List(workspaceID int, filters TestRunListFilters) ([]models.TestRun, error) {
	return s.repo.FindAll(workspaceID, repository.TestRunFilters{
		AssigneeID:   filters.AssigneeID,
		Unassigned:   filters.Unassigned,
		TemplateID:   filters.TemplateID,
		SetID:        filters.SetID,
		IncludeEnded: filters.IncludeEnded,
	})
}

// ListPage retrieves a repository-bounded page of test runs.
func (s *TestRunService) ListPage(workspaceID int, filters TestRunListFilters, limit, offset int) ([]models.TestRun, int, error) {
	return s.repo.FindPage(workspaceID, repository.TestRunFilters{
		AssigneeID: filters.AssigneeID, Unassigned: filters.Unassigned,
		TemplateID: filters.TemplateID, SetID: filters.SetID, IncludeEnded: filters.IncludeEnded,
		Limit: limit, Offset: offset,
	})
}

// GetByID retrieves a single test run
func (s *TestRunService) GetByID(id, workspaceID int) (*models.TestRun, error) {
	return s.repo.FindByID(id, workspaceID)
}

// GetDetail returns a complete test-run execution snapshot.
func (s *TestRunService) GetDetail(id, workspaceID int) (*TestRunDetail, error) {
	run, err := s.repo.FindByID(id, workspaceID)
	if err != nil {
		return nil, err
	}
	testCases, err := s.repo.FindCasesWithStepsForRun(id, workspaceID)
	if err != nil {
		return nil, err
	}
	results, err := s.ListResults(id, workspaceID)
	if err != nil {
		return nil, err
	}
	stepResults, err := s.ListStepResults(id, workspaceID)
	if err != nil {
		return nil, err
	}
	return &TestRunDetail{
		Run: run, TestCases: testCases, Results: results, StepResults: stepResults,
	}, nil
}

// TestRunCreateRequest contains data for creating a test run
type TestRunCreateRequest struct {
	Name       string
	TemplateID int
	SetID      int
	AssigneeID *int
}

// validateAssignee verifies the assignee, when set, is an existing active user.
func (s *TestRunService) validateAssignee(assigneeID *int) error {
	if assigneeID == nil || *assigneeID <= 0 {
		return nil
	}
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND is_active = true`, *assigneeID).Scan(&count)
	if err != nil || count == 0 {
		return fmt.Errorf("assignee not found")
	}
	return nil
}

// Create creates a new test run and initializes results for all test cases in the set
func (s *TestRunService) Create(workspaceID int, req TestRunCreateRequest) (*models.TestRun, error) {
	req.Name = sanitize.PlainTextField.Sanitize(req.Name)
	// Verify test set belongs to workspace
	if req.SetID > 0 {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", req.SetID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("test set not found in workspace")
		}
	}

	// Verify template belongs to the same workspace as the run being created.
	// Without this, a caller could attach a template from another workspace
	// to a run in their own — creating cross-workspace metadata links.
	if req.TemplateID > 0 {
		var count int
		err := s.db.QueryRow(
			"SELECT COUNT(*) FROM test_run_templates WHERE id = ? AND workspace_id = ?",
			req.TemplateID, workspaceID,
		).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("test run template not found in workspace")
		}
	}

	// Validate assignee is an active user if provided. Workspace membership
	// is deliberately not required: workspaces are open by default (members
	// of an open workspace have no user_workspace_roles row), and assignment
	// pickers offer all active users — same contract as item assignment.
	if err := s.validateAssignee(req.AssigneeID); err != nil {
		return nil, err
	}

	run := &models.TestRun{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		TemplateID:  req.TemplateID,
		SetID:       req.SetID,
		AssigneeID:  req.AssigneeID,
		StartedAt:   time.Now(),
		CreatedAt:   time.Now(),
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestRun, error) {
		runID, err := s.repo.Create(tx, run)
		if err != nil {
			return nil, err
		}

		// Create results for all test cases in the set
		if err := s.repo.CreateResultsFromSet(tx, runID, req.SetID); err != nil {
			return nil, fmt.Errorf("failed to create test results: %w", err)
		}

		run.ID = runID
		return run, nil
	})
}

// TestRunUpdateRequest contains data for updating a test run
type TestRunUpdateRequest struct {
	Name       string
	AssigneeID *int
}

// Update updates an existing test run
func (s *TestRunService) Update(id, workspaceID int, req TestRunUpdateRequest) (*models.TestRun, error) {
	// Get existing run
	run, err := s.repo.FindByID(id, workspaceID)
	if err != nil {
		return nil, err
	}

	// Validate assignee if provided (see Create for why membership is not checked)
	if err := s.validateAssignee(req.AssigneeID); err != nil {
		return nil, err
	}

	run.Name = sanitize.PlainTextField.Sanitize(req.Name)
	run.AssigneeID = req.AssigneeID

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestRun, error) {
		if err := s.repo.Update(tx, run); err != nil {
			return nil, err
		}
		return run, nil
	})
}

// Delete removes a test run and its results
func (s *TestRunService) Delete(id, workspaceID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Delete(tx, id, workspaceID)
	})
}

// Complete marks a test run as completed
func (s *TestRunService) Complete(id, workspaceID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Complete(tx, id, workspaceID)
	})
}

// Exists checks if a test run exists
func (s *TestRunService) Exists(id, workspaceID int) (bool, error) {
	return s.repo.Exists(id, workspaceID)
}

// Test Result methods

// TestResultUpdateRequest contains data for updating a test result
type TestResultUpdateRequest struct {
	Status       string
	ActualResult string
	Notes        string
}

// TestStepResultUpdateRequest contains data for creating or updating a step result.
type TestStepResultUpdateRequest struct {
	Status       string
	ActualResult string
	Notes        string
	ItemID       *int
}

// ListResults returns every per-case result for a workspace-scoped run.
func (s *TestRunService) ListResults(runID, workspaceID int) ([]TestRunResultWithCaseTitle, error) {
	if _, err := s.repo.FindByID(runID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.repo.FindResultsWithTestCase(runID, workspaceID)
	if err != nil {
		return nil, err
	}
	results := make([]TestRunResultWithCaseTitle, 0, len(rows))
	for _, row := range rows {
		results = append(results, TestRunResultWithCaseTitle{
			TestResult: row.TestResult, TestCaseTitle: row.TestCaseTitle,
		})
	}
	return results, nil
}

// UpdateResult updates a test result. runID scopes the update — if the
// result doesn't belong to that run, the repo returns ErrNotFound and the
// handler renders 404.
func (s *TestRunService) UpdateResult(workspaceID, runID, resultID int, req TestResultUpdateRequest) (*models.TestResult, error) {
	if _, err := s.repo.FindByID(runID, workspaceID); err != nil {
		return nil, err
	}
	// Validate status
	if !isValidTestResultStatus(req.Status) {
		return nil, ErrInvalidTestResultStatus
	}
	req.ActualResult = sanitize.RichText.Sanitize(req.ActualResult)
	req.Notes = sanitize.RichText.Sanitize(req.Notes)

	now := time.Now()
	result := &models.TestResult{
		ID:           resultID,
		Status:       req.Status,
		ActualResult: req.ActualResult,
		Notes:        req.Notes,
		ExecutedAt:   &now,
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.UpdateResult(tx, runID, result)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// ListStepResults returns step results keyed by test-case and step ID.
func (s *TestRunService) ListStepResults(runID, workspaceID int) (map[string]TestRunStepResult, error) {
	if _, err := s.repo.FindByID(runID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.repo.FindStepResultsForRun(runID, workspaceID)
	if err != nil {
		return nil, err
	}
	results := make(map[string]TestRunStepResult, len(rows))
	for _, row := range rows {
		key := fmt.Sprintf("%d_%d", row.TestCaseID, row.StepID)
		results[key] = TestRunStepResult{
			StepID: row.StepID, TestCaseID: row.TestCaseID, Status: row.Status,
			ActualResult: row.ActualResult, Notes: row.Notes, ItemID: row.ItemID,
			ExecutedAt: row.ExecutedAt,
		}
	}
	return results, nil
}

// UpdateStepResult upserts a step result and derives the parent case status.
func (s *TestRunService) UpdateStepResult(workspaceID, runID, stepID int, req TestStepResultUpdateRequest) error {
	if !isValidTestResultStatus(req.Status) {
		return ErrInvalidTestResultStatus
	}
	if req.ItemID != nil {
		itemWorkspaceID, err := s.itemRepo.GetWorkspaceID(*req.ItemID)
		if err != nil || itemWorkspaceID != workspaceID {
			return ErrTestRunItemNotFound
		}
	}
	testResultID, err := s.repo.FindTestResultIDForStep(runID, stepID, workspaceID)
	if err != nil {
		return err
	}
	existingID, findErr := s.repo.FindStepResultID(testResultID, stepID)
	input := repository.StepResultInput{
		TestResultID: testResultID,
		StepID:       stepID,
		Status:       req.Status,
		ActualResult: sanitize.RichText.Sanitize(req.ActualResult),
		Notes:        sanitize.RichText.Sanitize(req.Notes),
		ItemID:       req.ItemID,
	}
	switch {
	case errors.Is(findErr, repository.ErrNotFound):
		err = s.repo.CreateStepResult(input)
	case findErr == nil:
		err = s.repo.UpdateStepResult(existingID, input)
	default:
		err = findErr
	}
	if err != nil {
		return err
	}
	if err := s.updateCaseStatusFromSteps(testResultID); err != nil {
		slog.Warn("failed to update test case status", slog.Any("error", err), slog.Int("test_result_id", testResultID))
	}
	return nil
}

func (s *TestRunService) updateCaseStatusFromSteps(testResultID int) error {
	statuses, err := s.repo.FindStepResultStatuses(testResultID)
	if err != nil || len(statuses) == 0 {
		return err
	}

	finalStatus := "not_run"
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
		finalStatus = "failed"
	case hasBlocked:
		finalStatus = "blocked"
	case hasSkipped:
		finalStatus = "skipped"
	case allPassed:
		finalStatus = "passed"
	}
	return s.repo.SetTestResultStatus(testResultID, finalStatus)
}

// LinkResultItem links an item after enforcing workspace ownership on both records.
func (s *TestRunService) LinkResultItem(workspaceID, resultID, itemID int) error {
	itemWorkspaceID, err := s.itemRepo.GetWorkspaceID(itemID)
	if err != nil || itemWorkspaceID != workspaceID {
		return ErrTestRunItemNotFound
	}
	owned, err := s.repo.TestResultBelongsToWorkspace(resultID, workspaceID)
	if err != nil {
		return err
	}
	if !owned {
		return repository.ErrNotFound
	}
	return s.repo.LinkResultToItem(resultID, itemID)
}

// UnlinkResultItem removes an item link from a workspace-owned test result.
func (s *TestRunService) UnlinkResultItem(workspaceID, resultID, itemID int) error {
	owned, err := s.repo.TestResultBelongsToWorkspace(resultID, workspaceID)
	if err != nil {
		return err
	}
	if !owned {
		return repository.ErrNotFound
	}
	return s.repo.UnlinkResultFromItem(resultID, itemID)
}

// ListResultItems returns items linked to a workspace-owned test result.
func (s *TestRunService) ListResultItems(workspaceID, resultID int) ([]models.Item, error) {
	owned, err := s.repo.TestResultBelongsToWorkspace(resultID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, repository.ErrNotFound
	}
	return s.itemRepo.ListItemsLinkedToTestResult(resultID, workspaceID)
}

// Helper functions

func isValidTestResultStatus(status string) bool {
	validStatuses := map[string]bool{
		"passed":  true,
		"failed":  true,
		"blocked": true,
		"skipped": true,
		"not_run": true,
	}
	return validStatuses[status]
}
