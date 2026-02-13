package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
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
}

// List retrieves test cases with optional folder filtering
func (s *TestCaseService) List(params TestCaseListParams) ([]models.TestCase, error) {
	testCases, err := s.repo.FindAll(repository.TestCaseListParams{
		WorkspaceID: params.WorkspaceID,
		FolderID:    params.FolderID,
		All:         params.All,
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

// GetByID retrieves a single test case
func (s *TestCaseService) GetByID(id, workspaceID int) (*models.TestCase, error) {
	return s.repo.FindByID(id, workspaceID)
}

// GetWithSteps retrieves a test case with its steps
func (s *TestCaseService) GetWithSteps(id, workspaceID int) (*models.TestCase, error) {
	tc, err := s.repo.FindByIDWithSteps(id, workspaceID)
	if err != nil {
		return nil, err
	}

	// Also load labels
	labels, err := s.repo.FindLabelsByTestCaseID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load labels: %w", err)
	}
	tc.Labels = labels

	return tc, nil
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
	req.Title = utils.StripHTMLTags(req.Title)
	req.Preconditions = utils.SanitizeCommentContent(req.Preconditions)

	// Set defaults
	if req.Priority == "" {
		req.Priority = "medium"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	// Validate priority
	if !isValidTestCasePriority(req.Priority) {
		return nil, fmt.Errorf("invalid priority value: must be low, medium, high, or critical")
	}

	// Validate status
	if !isValidTestCaseStatus(req.Status) {
		return nil, fmt.Errorf("invalid status value: must be active, inactive, or draft")
	}

	// Validate estimated duration
	if req.EstimatedDuration < 0 {
		return nil, fmt.Errorf("estimated duration cannot be negative")
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := s.repo.Create(tx, tc)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	tc.ID = id
	return tc, nil
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
	req.Title = utils.StripHTMLTags(req.Title)
	req.Preconditions = utils.SanitizeCommentContent(req.Preconditions)

	// Validate priority if provided
	if req.Priority != "" && !isValidTestCasePriority(req.Priority) {
		return nil, fmt.Errorf("invalid priority value: must be low, medium, high, or critical")
	}

	// Validate status if provided
	if req.Status != "" && !isValidTestCaseStatus(req.Status) {
		return nil, fmt.Errorf("invalid status value: must be active, inactive, or draft")
	}

	// Validate estimated duration
	if req.EstimatedDuration < 0 {
		return nil, fmt.Errorf("estimated duration cannot be negative")
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Update(tx, tc); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return tc, nil
}

// Delete removes a test case
func (s *TestCaseService) Delete(id, workspaceID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Delete(tx, id, workspaceID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Move moves a test case to a different folder
func (s *TestCaseService) Move(id, workspaceID int, folderID *int, sortOrder int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Move(tx, id, workspaceID, folderID, sortOrder); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Reorder reorders test cases within a folder
func (s *TestCaseService) Reorder(workspaceID int, testCaseIDs []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Reorder(tx, workspaceID, testCaseIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Exists checks if a test case exists in a workspace
func (s *TestCaseService) Exists(id, workspaceID int) (bool, error) {
	return s.repo.Exists(id, workspaceID)
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
	req.Action = utils.SanitizeCommentContent(req.Action)
	req.Data = utils.SanitizeCommentContent(req.Data)
	req.Expected = utils.SanitizeCommentContent(req.Expected)

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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := s.repo.CreateStep(tx, step)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	step.ID = id
	return step, nil
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
	req.Action = utils.SanitizeCommentContent(req.Action)
	req.Data = utils.SanitizeCommentContent(req.Data)
	req.Expected = utils.SanitizeCommentContent(req.Expected)

	step := &models.TestStep{
		ID:         stepID,
		TestCaseID: testCaseID,
		StepNumber: req.StepNumber,
		Action:     req.Action,
		Data:       req.Data,
		Expected:   req.Expected,
		UpdatedAt:  time.Now(),
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.UpdateStep(tx, step); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return step, nil
}

// DeleteStep deletes a test step
func (s *TestCaseService) DeleteStep(stepID, testCaseID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.DeleteStep(tx, stepID, testCaseID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ReorderSteps reorders test steps
func (s *TestCaseService) ReorderSteps(testCaseID int, stepIDs []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.ReorderSteps(tx, testCaseID, stepIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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

// TestLabelCreateRequest contains data for creating a label
type TestLabelCreateRequest struct {
	Name        string
	Color       string
	Description string
}

// CreateLabel creates a new test label
func (s *TestCaseService) CreateLabel(workspaceID int, req TestLabelCreateRequest) (*models.TestLabel, error) {
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = s.repo.CreateLabel(tx, label)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return label, nil
}

// GetLabel retrieves a single label by ID
func (s *TestCaseService) GetLabel(labelID, workspaceID int) (*models.TestLabel, error) {
	return s.repo.GetLabel(labelID, workspaceID)
}

// TestLabelUpdateRequest contains data for updating a label
type TestLabelUpdateRequest struct {
	Name        string
	Color       string
	Description string
}

// UpdateLabel updates an existing test label
func (s *TestCaseService) UpdateLabel(labelID, workspaceID int, req TestLabelUpdateRequest) (*models.TestLabel, error) {
	label := &models.TestLabel{
		ID:          labelID,
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Color:       req.Color,
		Description: req.Description,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	if err := s.repo.UpdateLabel(tx, label); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Return the updated label
	return s.repo.GetLabel(labelID, workspaceID)
}

// DeleteLabel deletes a test label
func (s *TestCaseService) DeleteLabel(labelID, workspaceID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	if err := s.repo.DeleteLabel(tx, labelID, workspaceID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	if err := s.repo.AddLabelToTestCase(tx, testCaseID, labelID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RemoveLabelFromTestCase removes a label from a test case
func (s *TestCaseService) RemoveLabelFromTestCase(testCaseID, labelID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.RemoveLabelFromTestCase(tx, testCaseID, labelID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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
