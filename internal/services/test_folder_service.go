package services

import (
	"errors"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrTestFolderNameRequired      = errors.New("folder name is required")
	ErrTestFolderParentNotFound    = errors.New("parent folder not found")
	ErrTestFolderNestedDepth       = errors.New("nested folders deeper than two levels are not allowed")
	ErrTestFolderParentSelf        = errors.New("folder cannot be its own parent")
	ErrTestFolderParentHasChildren = errors.New("folders with subfolders cannot be nested under another folder")
)

// TestFolderUpdateInput carries a folder update plus JSON-field presence flags
// so callers can preserve parent_id / sort_order when they were omitted.
type TestFolderUpdateInput struct {
	Folder            models.TestFolder
	ParentProvided    bool
	SortOrderProvided bool
}

// TestFolderService owns test-folder use cases shared by HTTP surfaces.
type TestFolderService struct {
	repo *repository.TestFolderRepository
}

// NewTestFolderService creates a test-folder service.
func NewTestFolderService(db database.Database) *TestFolderService {
	return &TestFolderService{repo: repository.NewTestFolderRepository(db)}
}

func (s *TestFolderService) List(workspaceID int) ([]models.TestFolder, error) {
	return s.repo.FindAllWithCounts(workspaceID)
}

func (s *TestFolderService) Get(workspaceID, id int) (*models.TestFolder, error) {
	return s.repo.FindByIDWithCount(id, workspaceID)
}

func (s *TestFolderService) Create(workspaceID int, folder models.TestFolder) (models.TestFolder, error) {
	folder.Name = sanitize.PlainTextField.Sanitize(folder.Name)
	folder.Description = sanitize.RichText.Sanitize(folder.Description)
	if folder.Name == "" {
		return models.TestFolder{}, ErrTestFolderNameRequired
	}
	folder.WorkspaceID = workspaceID
	if err := s.validateParentFolder(workspaceID, folder.ParentID, nil); err != nil {
		return models.TestFolder{}, err
	}
	maxSortOrder, err := s.repo.MaxSortOrder(workspaceID)
	if err != nil {
		return models.TestFolder{}, err
	}
	folder.SortOrder = maxSortOrder + 1000
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()
	id, err := s.repo.Create(&folder)
	if err != nil {
		return models.TestFolder{}, err
	}
	folder.ID = id
	folder.TestCaseCount = 0
	return folder, nil
}

func (s *TestFolderService) Update(workspaceID, id int, in TestFolderUpdateInput) (models.TestFolder, error) {
	folder := in.Folder
	folder.Name = sanitize.PlainTextField.Sanitize(folder.Name)
	folder.Description = sanitize.RichText.Sanitize(folder.Description)
	if folder.Name == "" {
		return models.TestFolder{}, ErrTestFolderNameRequired
	}
	existingParent, existingSortOrder, err := s.repo.FindParentAndSortOrder(id, workspaceID)
	if err != nil {
		return models.TestFolder{}, err
	}
	if !in.ParentProvided && existingParent.Valid {
		parentID := int(existingParent.Int64)
		folder.ParentID = &parentID
	}
	if !in.SortOrderProvided {
		folder.SortOrder = existingSortOrder
	}
	if in.ParentProvided && folder.ParentID != nil {
		if err := s.validateParentFolder(workspaceID, folder.ParentID, &id); err != nil {
			return models.TestFolder{}, err
		}
	}
	folder.UpdatedAt = time.Now()
	if err := s.repo.Update(id, workspaceID, &folder); err != nil {
		return models.TestFolder{}, err
	}
	folder.ID = id
	folder.WorkspaceID = workspaceID
	return folder, nil
}

func (s *TestFolderService) Delete(workspaceID, id int) error {
	return s.repo.DeleteWithCascade(id, workspaceID)
}

func (s *TestFolderService) Reorder(workspaceID int, folderIDs []int) error {
	return s.repo.Reorder(workspaceID, folderIDs)
}

func (s *TestFolderService) validateParentFolder(workspaceID int, parentID, currentFolderID *int) error {
	if parentID == nil {
		return nil
	}
	if currentFolderID != nil && *parentID == *currentFolderID {
		return ErrTestFolderParentSelf
	}
	parentParentID, err := s.repo.GetParentID(*parentID, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrTestFolderParentNotFound
	}
	if err != nil {
		return err
	}
	if parentParentID.Valid {
		return ErrTestFolderNestedDepth
	}
	if currentFolderID != nil {
		childCount, err := s.repo.CountChildren(*currentFolderID, workspaceID)
		if err != nil {
			return err
		}
		if childCount > 0 {
			return ErrTestFolderParentHasChildren
		}
	}
	return nil
}
