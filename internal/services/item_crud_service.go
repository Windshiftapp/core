package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ItemCRUDService handles item CRUD operations
type ItemCRUDService struct {
	db   database.Database
	repo *repository.ItemRepository
}

// NewItemCRUDService creates a new item CRUD service
func NewItemCRUDService(db database.Database) *ItemCRUDService {
	return &ItemCRUDService{
		db:   db,
		repo: repository.NewItemRepository(db),
	}
}

// GetByID retrieves an item by ID with all details
func (s *ItemCRUDService) GetByID(id int) (*models.Item, error) {
	return s.repo.FindByIDWithDetails(id)
}

// GetByIDWithWorkspaceStatus retrieves an item with workspace active status for permission checks
func (s *ItemCRUDService) GetByIDWithWorkspaceStatus(id int) (*repository.ItemWithWorkspaceStatus, error) {
	return s.repo.FindByIDWithWorkspaceStatus(id)
}

// GetByIDBasic retrieves an item by ID without joins
func (s *ItemCRUDService) GetByIDBasic(id int) (*models.Item, error) {
	return s.repo.FindByID(id)
}

// Exists checks if an item exists
func (s *ItemCRUDService) Exists(id int) (bool, error) {
	return s.repo.Exists(id)
}

// GetWorkspaceID returns the workspace ID for an item
func (s *ItemCRUDService) GetWorkspaceID(itemID int) (int, error) {
	return s.repo.GetWorkspaceID(itemID)
}

// DeleteResult contains the result of a delete operation
type DeleteResult struct {
	DeletedCount   int
	DescendantIDs  []int
	AffectedParent *int
}

// Delete removes an item and all its descendants
func (s *ItemCRUDService) Delete(itemID int) (*DeleteResult, error) {
	// Get parent ID before deleting
	parentID, err := s.repo.GetParentID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("item not found")
		}
		return nil, err
	}

	// Get all descendant IDs for cascade operations
	descendantIDs, err := s.repo.GetDescendantIDs(itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete all related data for item and descendants
	allIDs := append([]int{itemID}, descendantIDs...)
	for _, id := range allIDs {
		// Delete watches
		if err := s.repo.DeleteItemWatches(tx, id); err != nil {
			return nil, err
		}

		// Delete history
		if err := s.repo.DeleteItemHistory(tx, id); err != nil {
			return nil, err
		}

		// Delete links
		if err := s.repo.DeleteItemLinks(tx, id); err != nil {
			return nil, err
		}

		// Clear worklog references
		if err := s.repo.ClearWorklogItemReferences(tx, id); err != nil {
			return nil, err
		}

		// Delete the item itself
		if err := s.repo.Delete(tx, id); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &DeleteResult{
		DeletedCount:   len(allIDs),
		DescendantIDs:  descendantIDs,
		AffectedParent: parentID,
	}, nil
}

// CopyOptions contains options for copying an item
type CopyOptions struct {
	IncludeChildren bool
	NewParentID     *int
	NewTitle        string
	CreatorID       int
}

// CopyResult contains the result of a copy operation
type CopyResult struct {
	NewItemID int
	CopyCount int
}

// Copy creates a copy of an item
func (s *ItemCRUDService) Copy(itemID int, opts CopyOptions) (*CopyResult, error) {
	// Get the source item
	source, err := s.repo.FindByID(itemID)
	if err != nil {
		return nil, fmt.Errorf("source item not found: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get next item number
	nextNum, err := s.repo.GetNextWorkspaceItemNumber(tx, source.WorkspaceID)
	if err != nil {
		return nil, err
	}

	// Create the copy
	newItem := &models.Item{
		WorkspaceID:         source.WorkspaceID,
		WorkspaceItemNumber: nextNum,
		ItemTypeID:          source.ItemTypeID,
		Title:               opts.NewTitle,
		Description:         source.Description,
		StatusID:            source.StatusID,
		PriorityID:          source.PriorityID,
		DueDate:             source.DueDate,
		IsTask:              source.IsTask,
		MilestoneID:         source.MilestoneID,
		IterationID:         source.IterationID,
		ProjectID:           source.ProjectID,
		InheritProject:      source.InheritProject,
		AssigneeID:          source.AssigneeID,
		CreatorID:           &opts.CreatorID,
		CustomFieldValues:   source.CustomFieldValues,
		ParentID:            opts.NewParentID,
	}

	if newItem.ParentID == nil {
		newItem.ParentID = source.ParentID
	}

	newID, err := s.repo.Create(tx, newItem)
	if err != nil {
		return nil, fmt.Errorf("failed to create copy: %w", err)
	}

	copyCount := 1

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &CopyResult{
		NewItemID: newID,
		CopyCount: copyCount,
	}, nil
}

// GetChildren returns direct children of an item
func (s *ItemCRUDService) GetChildren(parentID int) ([]*models.Item, error) {
	return s.repo.GetChildren(parentID)
}

// GetDescendants returns all descendants of an item
func (s *ItemCRUDService) GetDescendants(parentID int) ([]*models.Item, error) {
	return s.repo.GetDescendants(parentID)
}

// GetAncestors returns the ancestors of an item (path to root)
func (s *ItemCRUDService) GetAncestors(itemID int) ([]*models.Item, error) {
	return s.repo.GetAncestors(itemID)
}

// GetRootItems returns all root items for a workspace
func (s *ItemCRUDService) GetRootItems(workspaceID int) ([]*models.Item, error) {
	return s.repo.GetRootItems(workspaceID)
}

// GetWithEffectiveProject retrieves an item with effective project calculated
// This is the most comprehensive Get method, used by the handler
func (s *ItemCRUDService) GetWithEffectiveProject(id int) (*models.Item, error) {
	item, err := s.repo.FindByIDWithDetails(id)
	if err != nil {
		return nil, err
	}

	// Calculate effective project if inherit_project is true
	if item.InheritProject && item.ParentID != nil {
		effectiveProjectID, err := s.calculateEffectiveProject(id)
		if err == nil && effectiveProjectID != nil {
			item.EffectiveProjectID = effectiveProjectID
			// Fetch project name
			var name sql.NullString
			s.db.QueryRow("SELECT name FROM time_projects WHERE id = ?", *effectiveProjectID).Scan(&name)
			if name.Valid {
				item.EffectiveProjectName = name.String
			}
			item.ProjectInheritanceMode = "inherit"
		}
	} else if item.ProjectID != nil {
		item.EffectiveProjectID = item.ProjectID
		item.EffectiveProjectName = item.ProjectName
		item.ProjectInheritanceMode = "direct"
	} else {
		item.ProjectInheritanceMode = "none"
	}

	return item, nil
}

// calculateEffectiveProject walks up the hierarchy to find an inherited project
func (s *ItemCRUDService) calculateEffectiveProject(itemID int) (*int, error) {
	ancestors, err := s.repo.GetAncestors(itemID)
	if err != nil {
		return nil, err
	}

	// Walk up ancestors (already ordered from immediate parent to root)
	for _, ancestor := range ancestors {
		if ancestor.ProjectID != nil {
			return ancestor.ProjectID, nil
		}
	}

	return nil, nil
}
