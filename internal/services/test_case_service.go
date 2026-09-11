package services

import (
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// TestCaseService handles test case business logic
type TestCaseService struct {
	db   database.Database
	repo *repository.TestCaseRepository
}

// NewTestCaseService creates a new test case service
func NewTestCaseService(db database.Database) *TestCaseService {
	return &TestCaseService{
		db:   db,
		repo: repository.NewTestCaseRepository(db),
	}
}

// TestCaseListParams contains parameters for listing test cases
type TestCaseListParams struct {
	WorkspaceID int
	FolderID    *int
	All         bool
	Limit       int
	Offset      int
	Search      string
	LabelID     *int
}

// List retrieves test cases with optional folder filtering
func (s *TestCaseService) List(params TestCaseListParams) ([]models.TestCase, error) {
	testCases, err := s.repo.FindAll(repository.TestCaseListParams{
		WorkspaceID: params.WorkspaceID,
		FolderID:    params.FolderID,
		All:         params.All,
		Limit:       params.Limit,
		Offset:      params.Offset,
		Search:      params.Search,
		LabelID:     params.LabelID,
	})
	if err != nil {
		return nil, err
	}

	// Load labels for each test case
	for i := range testCases {
		labels, err := s.repo.FindLabelsByTestCaseID(testCases[i].ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load labels for test case %d: %w", testCases[i].ID, err)
		}
		testCases[i].Labels = labels
	}

	return testCases, nil
}

func (s *TestCaseService) CountAll(workspaceID int) (int, error) {
	return s.repo.CountAll(workspaceID)
}

func (s *TestCaseService) Count(params TestCaseListParams) (int, error) {
	return s.repo.Count(repository.TestCaseListParams{
		WorkspaceID: params.WorkspaceID, FolderID: params.FolderID, All: params.All,
		Search: params.Search, LabelID: params.LabelID,
	})
}

// GetByID retrieves a single test case
func (s *TestCaseService) GetByID(id, workspaceID int) (*models.TestCase, error) {
	return s.repo.FindByID(id, workspaceID)
}

// TestCaseCreateRequest contains data for creating a test case
type TestCaseCreateRequest struct {
	Title             string
	Preconditions     string
	Priority          string
	Status            string
	EstimatedDuration int
	FolderID          *int
}

// Create creates a new test case
func (s *TestCaseService) Create(workspaceID int, req TestCaseCreateRequest) (*models.TestCase, error) {
	// Sanitize input
	req.Title = sanitize.PlainTextField.Sanitize(req.Title)
	req.Preconditions = sanitize.Comment.Sanitize(req.Preconditions)

	if req.Title == "" {
		return nil, &TestManagementValidationError{Msg: "test case title is required"}
	}

	// Set defaults
	if req.Priority == "" {
		req.Priority = "medium"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	// Validate priority
	if !isValidTestCasePriority(req.Priority) {
		return nil, &TestManagementValidationError{Msg: "invalid priority value: must be low, medium, high, or critical"}
	}

	// Validate status
	if !isValidTestCaseStatus(req.Status) {
		return nil, &TestManagementValidationError{Msg: "invalid status value: must be active, inactive, or draft"}
	}

	// Validate estimated duration
	if req.EstimatedDuration < 0 {
		return nil, &TestManagementValidationError{Msg: "estimated duration cannot be negative"}
	}

	if err := s.validateFolderInWorkspace(workspaceID, req.FolderID); err != nil {
		return nil, err
	}

	// Get max sort order
	maxSortOrder, err := s.repo.GetMaxSortOrder(workspaceID, req.FolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sort order: %w", err)
	}

	now := time.Now()
	tc := &models.TestCase{
		WorkspaceID:       workspaceID,
		FolderID:          req.FolderID,
		Title:             req.Title,
		Preconditions:     req.Preconditions,
		Priority:          req.Priority,
		Status:            req.Status,
		EstimatedDuration: req.EstimatedDuration,
		SortOrder:         maxSortOrder + 1000,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestCase, error) {
		id, err := s.repo.Create(tx, tc)
		if err != nil {
			return nil, err
		}
		tc.ID = id
		return tc, nil
	})
}

// TestCaseUpdateRequest contains data for updating a test case
type TestCaseUpdateRequest struct {
	Title             string
	Preconditions     string
	Priority          string
	Status            string
	EstimatedDuration int
	FolderID          *int
	SortOrder         int
}

// Update updates an existing test case
func (s *TestCaseService) Update(id, workspaceID int, req TestCaseUpdateRequest) (*models.TestCase, error) {
	// Sanitize input
	req.Title = sanitize.PlainTextField.Sanitize(req.Title)
	req.Preconditions = sanitize.Comment.Sanitize(req.Preconditions)

	if req.Title == "" {
		return nil, &TestManagementValidationError{Msg: "test case title is required"}
	}

	// Validate priority if provided
	if req.Priority != "" && !isValidTestCasePriority(req.Priority) {
		return nil, &TestManagementValidationError{Msg: "invalid priority value: must be low, medium, high, or critical"}
	}

	// Validate status if provided
	if req.Status != "" && !isValidTestCaseStatus(req.Status) {
		return nil, &TestManagementValidationError{Msg: "invalid status value: must be active, inactive, or draft"}
	}

	// Validate estimated duration
	if req.EstimatedDuration < 0 {
		return nil, &TestManagementValidationError{Msg: "estimated duration cannot be negative"}
	}

	if err := s.validateFolderInWorkspace(workspaceID, req.FolderID); err != nil {
		return nil, err
	}

	tc := &models.TestCase{
		ID:                id,
		WorkspaceID:       workspaceID,
		FolderID:          req.FolderID,
		Title:             req.Title,
		Preconditions:     req.Preconditions,
		Priority:          req.Priority,
		Status:            req.Status,
		EstimatedDuration: req.EstimatedDuration,
		SortOrder:         req.SortOrder,
		UpdatedAt:         time.Now(),
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestCase, error) {
		if err := s.repo.Update(tx, tc); err != nil {
			return nil, err
		}
		return tc, nil
	})
}

// Delete removes a test case
func (s *TestCaseService) Delete(id, workspaceID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Delete(tx, id, workspaceID)
	})
}

// Move moves a test case to a different folder
func (s *TestCaseService) Move(id, workspaceID int, folderID *int, sortOrder int) error {
	if err := s.validateFolderInWorkspace(workspaceID, folderID); err != nil {
		return err
	}

	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Move(tx, id, workspaceID, folderID, sortOrder)
	})
}

// Reorder reorders test cases within a folder
func (s *TestCaseService) Reorder(workspaceID int, testCaseIDs []int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Reorder(tx, workspaceID, testCaseIDs)
	})
}

// Exists checks if a test case exists in a workspace
func (s *TestCaseService) Exists(id, workspaceID int) (bool, error) {
	return s.repo.Exists(id, workspaceID)
}

func (s *TestCaseService) validateFolderInWorkspace(workspaceID int, folderID *int) error {
	if folderID == nil {
		return nil
	}

	var exists bool
	if err := s.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM test_folders WHERE id = ? AND workspace_id = ?)",
		*folderID,
		workspaceID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("failed to validate test folder: %w", err)
	}
	if !exists {
		return repository.ErrNotFound
	}
	return nil
}

// Test Step methods

// GetSteps retrieves all steps for a test case
func (s *TestCaseService) GetSteps(testCaseID int) ([]models.TestStep, error) {
	return s.repo.FindSteps(testCaseID)
}

// TestStepCreateRequest contains data for creating a test step
type TestStepCreateRequest struct {
	Action   string
	Data     string
	Expected string
}

// CreateStep creates a new test step
func (s *TestCaseService) CreateStep(testCaseID int, req TestStepCreateRequest) (*models.TestStep, error) {
	// Sanitize input
	req.Action = sanitize.Comment.Sanitize(req.Action)
	req.Data = sanitize.Comment.Sanitize(req.Data)
	req.Expected = sanitize.Comment.Sanitize(req.Expected)

	// Get max step number
	maxStepNumber, err := s.repo.GetMaxStepNumber(testCaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get max step number: %w", err)
	}

	now := time.Now()
	step := &models.TestStep{
		TestCaseID: testCaseID,
		StepNumber: maxStepNumber + 1,
		Action:     req.Action,
		Data:       req.Data,
		Expected:   req.Expected,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestStep, error) {
		id, err := s.repo.CreateStep(tx, step)
		if err != nil {
			return nil, err
		}
		step.ID = id
		return step, nil
	})
}

// TestStepUpdateRequest contains data for updating a test step
type TestStepUpdateRequest struct {
	StepNumber int
	Action     string
	Data       string
	Expected   string
}

// UpdateStep updates an existing test step
func (s *TestCaseService) UpdateStep(stepID, testCaseID int, req TestStepUpdateRequest) (*models.TestStep, error) {
	// Sanitize input
	req.Action = sanitize.Comment.Sanitize(req.Action)
	req.Data = sanitize.Comment.Sanitize(req.Data)
	req.Expected = sanitize.Comment.Sanitize(req.Expected)

	step := &models.TestStep{
		ID:         stepID,
		TestCaseID: testCaseID,
		StepNumber: req.StepNumber,
		Action:     req.Action,
		Data:       req.Data,
		Expected:   req.Expected,
		UpdatedAt:  time.Now(),
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestStep, error) {
		if err := s.repo.UpdateStep(tx, step); err != nil {
			return nil, err
		}
		return step, nil
	})
}

// DeleteStep deletes a test step
func (s *TestCaseService) DeleteStep(stepID, testCaseID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.DeleteStep(tx, stepID, testCaseID)
	})
}

// ReorderSteps reorders test steps
func (s *TestCaseService) ReorderSteps(testCaseID int, stepIDs []int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.ReorderSteps(tx, testCaseID, stepIDs)
	})
}

// Test Label methods

// GetAllLabels returns all labels for a workspace
func (s *TestCaseService) GetAllLabels(workspaceID int) ([]models.TestLabel, error) {
	return s.repo.FindAllLabels(workspaceID)
}

// GetLabelsForTestCase returns labels for a specific test case
func (s *TestCaseService) GetLabelsForTestCase(testCaseID int) ([]models.TestLabel, error) {
	return s.repo.FindLabelsByTestCaseID(testCaseID)
}

var (
	ErrTestLabelNameRequired = &TestManagementValidationError{Msg: "label name is required"}
)

// TestLabelCreateRequest contains data for creating a label
type TestLabelCreateRequest struct {
	Name        string
	Color       string
	Description string
}

// CreateLabel creates a new test label
func (s *TestCaseService) CreateLabel(workspaceID int, req TestLabelCreateRequest) (*models.TestLabel, error) {
	req.Name = sanitize.ShortIdentifier.Sanitize(req.Name)
	req.Description = sanitize.RichText.Sanitize(req.Description)
	if req.Name == "" {
		return nil, ErrTestLabelNameRequired
	}
	if req.Color == "" {
		req.Color = "#3B82F6" // Default blue
	}

	now := time.Now()
	label := &models.TestLabel{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Color:       req.Color,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestLabel, error) {
		if _, err := s.repo.CreateLabel(tx, label); err != nil {
			return nil, err
		}
		return label, nil
	})
}

// TestLabelUpdateRequest contains data for updating a label
type TestLabelUpdateRequest struct {
	Name        string
	Color       string
	Description string
}

// UpdateLabel updates an existing test label
func (s *TestCaseService) UpdateLabel(labelID, workspaceID int, req TestLabelUpdateRequest) (*models.TestLabel, error) {
	req.Name = sanitize.ShortIdentifier.Sanitize(req.Name)
	req.Description = sanitize.RichText.Sanitize(req.Description)
	if req.Name == "" {
		return nil, ErrTestLabelNameRequired
	}
	label := &models.TestLabel{
		ID:          labelID,
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Color:       req.Color,
		Description: req.Description,
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.UpdateLabel(tx, label)
	}); err != nil {
		return nil, err
	}

	// Return the updated label
	return s.repo.GetLabel(labelID, workspaceID)
}

// DeleteLabel deletes a test label
func (s *TestCaseService) DeleteLabel(labelID, workspaceID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.DeleteLabel(tx, labelID, workspaceID)
	})
}

// AddLabelToTestCase adds a label to a test case
func (s *TestCaseService) AddLabelToTestCase(testCaseID, labelID, workspaceID int) error {
	// Verify label belongs to workspace
	exists, err := s.repo.LabelExists(labelID, workspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return repository.ErrNotFound
	}

	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.AddLabelToTestCase(tx, testCaseID, labelID)
	})
}

// RemoveLabelFromTestCase removes a label from a test case
func (s *TestCaseService) RemoveLabelFromTestCase(testCaseID, labelID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.RemoveLabelFromTestCase(tx, testCaseID, labelID)
	})
}

// GetConnections returns related sets, templates, and executions for a test case
func (s *TestCaseService) GetConnections(testCaseID, workspaceID int) (*repository.TestCaseConnections, error) {
	return s.repo.GetConnections(testCaseID, workspaceID)
}

// Helper functions

func isValidTestCasePriority(priority string) bool {
	validPriorities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	return validPriorities[priority]
}

func isValidTestCaseStatus(status string) bool {
	validStatuses := map[string]bool{"active": true, "inactive": true, "draft": true}
	return validStatuses[status]
}
