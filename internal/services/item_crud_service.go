package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ItemCRUDService handles item CRUD operations
type ItemCRUDService struct {
	db            database.Database
	repo          *repository.ItemRepository
	workspaceRepo *repository.WorkspaceRepository
}

// NewItemCRUDService creates a new item CRUD service
func NewItemCRUDService(db database.Database) *ItemCRUDService {
	return &ItemCRUDService{
		db:            db,
		repo:          repository.NewItemRepository(db),
		workspaceRepo: repository.NewWorkspaceRepository(db),
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
	defer func() { _ = tx.Rollback() }()

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
	defer func() { _ = tx.Rollback() }()

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

	// Record item creation history for the copied item
	updateService := NewItemUpdateService(s.db)
	if err := updateService.recordItemCreationHistory(s.db, newID, opts.CreatorID); err != nil {
		slog.Warn("failed to record item creation history", "error", err, "item_id", newID)
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

// ItemListParams re-exports repository.ItemListParams for service layer consumers
type ItemListParams = repository.ItemListParams

// ItemFilters re-exports repository.ItemFilters for service layer consumers
type ItemFilters = repository.ItemFilters

// PaginationParams re-exports repository.PaginationParams for service layer consumers
type PaginationParams = repository.PaginationParams

// List retrieves items with filters and pagination using the repository
func (s *ItemCRUDService) List(params ItemListParams) ([]models.Item, int, error) {
	return s.repo.FindAllWithDetails(params)
}

// Search searches items by title and description
func (s *ItemCRUDService) Search(query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	return s.repo.Search(query, workspaceIDs, pagination)
}

// SearchParams contains parameters for the advanced Search handler
type SearchParams struct {
	TextQuery    string
	WorkspaceIDs []int
	StatusIDs    []int
	PriorityIDs  []int
	Pagination   PaginationParams
}

// SearchWithFilters searches items with multiple filter criteria
func (s *ItemCRUDService) SearchWithFilters(params SearchParams) ([]models.Item, int, error) {
	if len(params.WorkspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	filters := ItemFilters{
		StatusIDs:   params.StatusIDs,
		PriorityIDs: params.PriorityIDs,
	}

	// Detect workspace key pattern (e.g. "OK-40")
	if params.TextQuery != "" {
		parts := strings.Split(strings.ToUpper(params.TextQuery), "-")
		isKeyPattern := len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0
		if isKeyPattern {
			if _, err := strconv.Atoi(parts[1]); err == nil {
				filters.ItemKeyQuery = params.TextQuery
			} else {
				filters.TextQuery = params.TextQuery
			}
		} else {
			filters.TextQuery = params.TextQuery
		}
	}

	return s.repo.FindAllWithDetails(ItemListParams{
		WorkspaceIDs: params.WorkspaceIDs,
		Filters:      filters,
		Pagination:   params.Pagination,
		SortBy:       "updated_at",
	})
}

// BacklogParams contains parameters for retrieving backlog items
type BacklogParams struct {
	WorkspaceID  int    // 0 if not specified (collection-only query)
	CollectionID int    // 0 if not specified
	QLQuery      string // Direct QL query, overrides collection
	WorkspaceIDs []int  // Accessible workspace IDs for security filtering
	Pagination   PaginationParams
}

// GetBacklogItems retrieves items with non-completed statuses for a workspace/collection
func (s *ItemCRUDService) GetBacklogItems(params BacklogParams) ([]models.Item, int, error) {
	if len(params.WorkspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	// Resolve backlog status IDs
	backlogStatusIDs, err := s.repo.GetBacklogStatusIDs(params.WorkspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get backlog statuses: %w", err)
	}
	if len(backlogStatusIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	filters := ItemFilters{
		StatusIDs: backlogStatusIDs,
	}

	// Resolve QL query from collection or direct parameter
	qlQuery := params.QLQuery
	collectionResolved := false
	if qlQuery == "" && params.CollectionID > 0 {
		collectionResolved = true
		_, collectionQL, err := s.workspaceRepo.GetCollectionQuery(params.CollectionID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, 0, fmt.Errorf("collection not found")
			}
			return nil, 0, fmt.Errorf("failed to get collection query: %w", err)
		}
		if strings.TrimSpace(collectionQL) != "" {
			qlQuery = collectionQL
		}
	}

	// Apply QL query if present
	if qlQuery != "" {
		workspaceMap, err := s.workspaceRepo.BuildWorkspaceMap()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to build workspace map: %w", err)
		}

		evaluator := cql.NewEvaluator(workspaceMap)
		qlSQL, qlArgs, err := evaluator.EvaluateToSQL(qlQuery)
		if err != nil {
			return nil, 0, fmt.Errorf("QL query error: %w", err)
		}

		if qlSQL != "" {
			filters.QLQuery = qlSQL
			filters.QLArgs = qlArgs
		}
	}

	// Apply workspace_id filter only when no collection was resolved
	if !collectionResolved && params.WorkspaceID > 0 {
		filters.WorkspaceID = &params.WorkspaceID
	}

	return s.repo.FindAllWithDetails(ItemListParams{
		WorkspaceIDs: params.WorkspaceIDs,
		Filters:      filters,
		Pagination:   params.Pagination,
	})
}

// ListWithQLParams contains parameters for listing items with QL support
type ListWithQLParams struct {
	WorkspaceID  int    // Single workspace filter (0 = all accessible)
	CollectionID int    // Collection to resolve QL from (0 = none)
	QLQuery      string // Direct QL query (overrides collection)
	WorkspaceIDs []int  // Accessible workspace IDs for security filtering
	Filters      ItemFilters
	Pagination   PaginationParams
	SortBy       string
	SortAsc      bool
}

// ListWithQL retrieves items with QL evaluation and collection resolution
func (s *ItemCRUDService) ListWithQL(params ListWithQLParams) ([]models.Item, int, error) {
	if len(params.WorkspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	filters := params.Filters

	// Resolve QL query from collection or direct parameter
	qlQuery := params.QLQuery
	collectionResolved := false
	if qlQuery == "" && params.CollectionID > 0 {
		collectionResolved = true
		_, collectionQL, err := s.workspaceRepo.GetCollectionQuery(params.CollectionID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, 0, fmt.Errorf("collection not found")
			}
			return nil, 0, fmt.Errorf("failed to get collection query: %w", err)
		}
		if strings.TrimSpace(collectionQL) != "" {
			qlQuery = collectionQL
		}
	}

	// Evaluate QL query
	if qlQuery != "" {
		workspaceMap, err := s.workspaceRepo.BuildWorkspaceMap()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to build workspace map: %w", err)
		}

		evaluator := cql.NewEvaluator(workspaceMap)
		qlSQL, qlArgs, err := evaluator.EvaluateToSQL(qlQuery)
		if err != nil {
			return nil, 0, fmt.Errorf("QL query error: %w", err)
		}

		if qlSQL != "" {
			filters.QLQuery = qlSQL
			filters.QLArgs = qlArgs
		}
	}

	// Apply workspace_id filter only when no collection was resolved
	if !collectionResolved && params.WorkspaceID > 0 {
		filters.WorkspaceID = &params.WorkspaceID
	}

	return s.repo.FindAllWithDetails(ItemListParams{
		WorkspaceIDs: params.WorkspaceIDs,
		Filters:      filters,
		Pagination:   params.Pagination,
		SortBy:       params.SortBy,
		SortAsc:      params.SortAsc,
	})
}

// GetWithEffectiveProject retrieves an item with effective project calculated
// This is the most comprehensive Get method, used by the handler
func (s *ItemCRUDService) GetWithEffectiveProject(id int) (*models.Item, error) {
	item, err := s.repo.FindByIDWithDetails(id)
	if err != nil {
		return nil, err
	}

	// Calculate effective project if inherit_project is true
	switch {
	case item.InheritProject && item.ParentID != nil:
		effectiveProjectID, err := s.calculateEffectiveProject(id)
		if err == nil && effectiveProjectID != nil {
			item.EffectiveProjectID = effectiveProjectID
			// Fetch project name
			var name sql.NullString
			_ = s.db.QueryRow("SELECT name FROM time_projects WHERE id = ?", *effectiveProjectID).Scan(&name)
			if name.Valid {
				item.EffectiveProjectName = name.String
			}
			item.ProjectInheritanceMode = "inherit"
		}
	case item.ProjectID != nil:
		item.EffectiveProjectID = item.ProjectID
		item.EffectiveProjectName = item.ProjectName
		item.ProjectInheritanceMode = "direct"
	default:
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

// GetHistory retrieves the change history for an item
func (s *ItemCRUDService) GetHistory(itemID int) ([]models.ItemHistory, error) {
	rows, err := s.db.Query(`
		SELECT h.id, h.item_id, h.user_id, h.changed_at, h.field_name, h.old_value, h.new_value,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM item_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.item_id = ?
		ORDER BY h.changed_at DESC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch item history: %w", err)
	}
	defer rows.Close()

	var history []models.ItemHistory
	for rows.Next() {
		var h models.ItemHistory
		var userName, userEmail sql.NullString
		err := rows.Scan(&h.ID, &h.ItemID, &h.UserID, &h.ChangedAt, &h.FieldName, &h.OldValue, &h.NewValue,
			&userName, &userEmail)
		if err != nil {
			continue
		}
		if userName.Valid {
			h.UserName = userName.String
		}
		if userEmail.Valid {
			h.UserEmail = userEmail.String
		}
		history = append(history, h)
	}

	if history == nil {
		history = []models.ItemHistory{}
	}

	return history, nil
}

// GetAttachments retrieves all attachments for an item
func (s *ItemCRUDService) GetAttachments(itemID int) ([]models.Attachment, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.has_thumbnail, a.uploaded_by, a.created_at,
		       u.first_name || ' ' || u.last_name as uploader_name, u.email as uploader_email
		FROM attachments a
		LEFT JOIN users u ON a.uploaded_by = u.id
		WHERE a.item_id = ?
		ORDER BY a.created_at DESC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch attachments: %w", err)
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var a models.Attachment
		var itemID sql.NullInt64
		var uploaderID sql.NullInt64
		var uploaderName, uploaderEmail sql.NullString
		err := rows.Scan(&a.ID, &itemID, &a.Filename, &a.OriginalFilename, &a.MimeType, &a.FileSize,
			&a.HasThumbnail, &uploaderID, &a.CreatedAt, &uploaderName, &uploaderEmail)
		if err != nil {
			continue
		}
		if itemID.Valid {
			id := int(itemID.Int64)
			a.ItemID = &id
		}
		if uploaderID.Valid {
			id := int(uploaderID.Int64)
			a.UploadedBy = &id
		}
		if uploaderName.Valid {
			a.UploaderName = uploaderName.String
		}
		if uploaderEmail.Valid {
			a.UploaderEmail = uploaderEmail.String
		}
		attachments = append(attachments, a)
	}

	if attachments == nil {
		attachments = []models.Attachment{}
	}

	return attachments, nil
}
