package services

import (
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// TestRunService handles test run business logic
type TestRunService struct {
	db   database.Database
	repo *repository.TestRunRepository
}

// NewTestRunService creates a new test run service
func NewTestRunService(db database.Database) *TestRunService {
	return &TestRunService{
		db:   db,
		repo: repository.NewTestRunRepository(db),
	}
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

// GetByID retrieves a single test run
func (s *TestRunService) GetByID(id, workspaceID int) (*models.TestRun, error) {
	return s.repo.FindByID(id, workspaceID)
}

// TestRunCreateRequest contains data for creating a test run
type TestRunCreateRequest struct {
	Name       string
	TemplateID int
	SetID      int
	AssigneeID *int
}

// Create creates a new test run and initializes results for all test cases in the set
func (s *TestRunService) Create(workspaceID int, req TestRunCreateRequest) (*models.TestRun, error) {
	// Verify test set belongs to workspace
	if req.SetID > 0 {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", req.SetID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("test set not found in workspace")
		}
	}

	// Validate assignee belongs to workspace if provided
	if req.AssigneeID != nil && *req.AssigneeID > 0 {
		var count int
		err := s.db.QueryRow(`
			SELECT COUNT(*) FROM user_workspace_roles WHERE user_id = ? AND workspace_id = ?
		`, *req.AssigneeID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("assignee is not a member of this workspace")
		}
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

	// Validate assignee if provided
	if req.AssigneeID != nil && *req.AssigneeID > 0 {
		var count int
		err = s.db.QueryRow(`
			SELECT COUNT(*) FROM user_workspace_roles WHERE user_id = ? AND workspace_id = ?
		`, *req.AssigneeID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("assignee is not a member of this workspace")
		}
	}

	run.Name = req.Name
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

// UpdateResult updates a test result
func (s *TestRunService) UpdateResult(resultID int, req TestResultUpdateRequest) error {
	// Validate status
	if !isValidTestResultStatus(req.Status) {
		return fmt.Errorf("invalid status: must be passed, failed, blocked, skipped, or not_run")
	}

	now := time.Now()
	result := &models.TestResult{
		ID:           resultID,
		Status:       req.Status,
		ActualResult: req.ActualResult,
		Notes:        req.Notes,
		ExecutedAt:   &now,
	}

	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.UpdateResult(tx, result)
	})
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
