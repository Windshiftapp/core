package services

import (
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	// ErrTestSetMilestoneNotFound identifies a milestone that cannot be used in
	// the target workspace.
	ErrTestSetMilestoneNotFound = errors.New("milestone not found in workspace")
	// ErrTestSetCaseNotFound identifies a test case outside the set workspace.
	ErrTestSetCaseNotFound = errors.New("test case not found in workspace")
)

// TestSetService owns test-set validation, normalization, and relationships.
type TestSetService struct {
	repo         *repository.TestSetRepository
	planningRepo *repository.PlanningRepository
	caseSvc      *TestCaseService
}

func NewTestSetService(db database.Database) *TestSetService {
	return &TestSetService{
		repo:         repository.NewTestSetRepository(db),
		planningRepo: repository.NewPlanningRepository(db),
		caseSvc:      NewTestCaseService(db),
	}
}

func (s *TestSetService) List(workspaceID int) ([]models.TestSet, error) {
	return s.repo.FindAllWithStats(workspaceID)
}

func (s *TestSetService) ListPage(workspaceID, limit, offset int) ([]models.TestSet, int, error) {
	return s.repo.FindPageWithStats(workspaceID, limit, offset)
}

func (s *TestSetService) Get(id, workspaceID int) (*models.TestSet, error) {
	return s.repo.FindByID(id, workspaceID)
}

func (s *TestSetService) GetWorkspaceID(id int) (int, error) {
	return s.repo.GetWorkspaceID(id)
}

func (s *TestSetService) Create(workspaceID int, set models.TestSet) (*models.TestSet, error) {
	if err := s.normalizeAndValidate(workspaceID, &set); err != nil {
		return nil, err
	}
	id, createdAt, err := s.repo.Create(workspaceID, &set)
	if err != nil {
		return nil, err
	}
	set.ID = id
	set.WorkspaceID = workspaceID
	set.CreatedAt = createdAt
	set.UpdatedAt = createdAt
	return &set, nil
}

func (s *TestSetService) Update(id, workspaceID int, set models.TestSet) (*models.TestSet, error) {
	if err := s.normalizeAndValidate(workspaceID, &set); err != nil {
		return nil, err
	}
	updatedAt, err := s.repo.Update(id, workspaceID, &set)
	if err != nil {
		return nil, err
	}
	set.ID = id
	set.WorkspaceID = workspaceID
	set.UpdatedAt = updatedAt
	return &set, nil
}

func (s *TestSetService) Delete(id, workspaceID int) error {
	return s.repo.Delete(id, workspaceID)
}

func (s *TestSetService) ListCases(id, workspaceID int) ([]models.TestCase, error) {
	if _, err := s.repo.FindByID(id, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.FindTestCases(id, workspaceID)
}

func (s *TestSetService) AddCase(id, testCaseID, workspaceID int) error {
	if _, err := s.repo.FindByID(id, workspaceID); err != nil {
		return err
	}
	exists, err := s.caseSvc.Exists(testCaseID, workspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTestSetCaseNotFound
	}
	return s.repo.AddTestCase(id, testCaseID)
}

func (s *TestSetService) RemoveCase(id, testCaseID, workspaceID int) error {
	if _, err := s.repo.FindByID(id, workspaceID); err != nil {
		return err
	}
	exists, err := s.caseSvc.Exists(testCaseID, workspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTestSetCaseNotFound
	}
	return s.repo.RemoveTestCase(id, testCaseID)
}

func (s *TestSetService) ListRuns(id, workspaceID int) ([]models.TestRun, error) {
	if _, err := s.repo.FindByID(id, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.FindRuns(id, workspaceID)
}

func (s *TestSetService) normalizeAndValidate(workspaceID int, set *models.TestSet) error {
	set.Name = sanitize.PlainTextField.Sanitize(set.Name)
	set.Description = sanitize.Comment.Sanitize(set.Description)
	if set.MilestoneID == nil {
		return nil
	}
	exists, err := s.planningRepo.MilestoneUsableInWorkspace(*set.MilestoneID, workspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTestSetMilestoneNotFound
	}
	return nil
}
