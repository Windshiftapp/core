package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

var (
	ErrCollectionNotFound               = errors.New("collection not found")
	ErrQLQuery                          = errors.New("QL query error")
	ErrItemHasProtectedIntegrationLinks = errors.New("item has provider-managed integration links")
)

// ItemCRUDService handles item CRUD operations
type ItemCRUDService struct {
	db                    database.Database
	repo                  *repository.ItemRepository
	workspaceRepo         *repository.WorkspaceRepository
	integrationLinkGuards *IntegrationLinkGuards
}

// NewItemCRUDService creates a new item CRUD service
func NewItemCRUDService(db database.Database) *ItemCRUDService {
	return &ItemCRUDService{
		db:                    db,
		repo:                  repository.NewItemRepository(db),
		workspaceRepo:         repository.NewWorkspaceRepository(db),
		integrationLinkGuards: NewIntegrationLinkGuards(db),
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
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
func (s *ItemCRUDService) GetByIDBasic(id int) (*models.Item, error) {
	return s.repo.FindByID(id)
}

// Exists checks if an item exists
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
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

// DeleteSingle removes only the requested item and shared related rows. It
// preserves the legacy non-cascade delete endpoint semantics; use Delete for
// item + descendants cleanup.
func (s *ItemCRUDService) DeleteSingle(itemID int) error {
	return s.DeleteSingleWithMetadata(itemID, itemevents.System("application"))
}

// DeleteSingleWithMetadata deletes one item and records its canonical fact.
func (s *ItemCRUDService) DeleteSingleWithMetadata(itemID int, metadata itemevents.Metadata) error {
	// Capture the parent BEFORE the destructive write so we can refresh the
	// parent's child list after commit (WI-483). Best-effort: a lookup failure
	// just means no parent refresh.
	item, err := s.repo.FindByID(itemID)
	if err != nil {
		return err
	}
	parentID := item.ParentID
	workspaceID := item.WorkspaceID
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		locked, err := s.repo.FindByIDForUpdate(tx, itemID)
		if err != nil {
			return err
		}
		hasProtectedLinks, err := s.integrationLinkGuards.HasLinksForItemsTx(tx, []int{itemID})
		if err != nil {
			return err
		}
		if hasProtectedLinks {
			return ErrItemHasProtectedIntegrationLinks
		}
		if metadata.OccurredAt.IsZero() {
			metadata.OccurredAt = time.Now()
		}
		if err := recordRemovedItemLinks(context.Background(), s.db, tx, []int{itemID}, metadata); err != nil {
			return err
		}
		if _, err := itemevents.NewRecorder(s.db).Deleted(context.Background(), tx, locked, 0, metadata); err != nil {
			return err
		}
		if err := s.repo.DeleteItemLinks(tx, itemID); err != nil {
			return err
		}
		if err := s.repo.ClearWorklogItemReferences(tx, itemID); err != nil {
			return err
		}
		return s.repo.Delete(tx, itemID)
	}); err != nil {
		return err
	}
	s.finishItemDeletion(workspaceID, []int{itemID}, parentID)
	return nil
}

// Delete removes an item and all its descendants
func (s *ItemCRUDService) Delete(itemID int) (*DeleteResult, error) {
	return s.DeleteWithMetadata(itemID, itemevents.System("application"))
}

// DeleteWithMetadata deletes an item subtree and records every removed item.
func (s *ItemCRUDService) DeleteWithMetadata(itemID int, metadata itemevents.Metadata) (*DeleteResult, error) {
	parentID, err := s.repo.GetParentID(itemID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("item not found")
		}
		return nil, err
	}
	workspaceID, _ := s.repo.GetWorkspaceID(itemID)
	descendantIDs, err := s.repo.GetDescendantIDs(itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}
	allIDs := append([]int{itemID}, descendantIDs...)
	if err := s.deleteItemSubtree(itemID, allIDs, len(descendantIDs), metadata); err != nil {
		return nil, err
	}
	s.finishItemDeletion(workspaceID, allIDs, parentID)
	return &DeleteResult{
		DeletedCount:   len(allIDs),
		DescendantIDs:  descendantIDs,
		AffectedParent: parentID,
	}, nil
}

func (s *ItemCRUDService) deleteItemSubtree(rootID int, itemIDs []int, descendantCount int, metadata itemevents.Metadata) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		ctx := context.Background()
		items, err := s.repo.FindByIDsForUpdateContext(ctx, tx, itemIDs)
		if err != nil {
			return err
		}
		if len(items) != len(itemIDs) {
			return repository.ErrNotFound
		}
		hasProtectedLinks, err := s.integrationLinkGuards.HasLinksForItemsTx(tx, itemIDs)
		if err != nil {
			return err
		}
		if hasProtectedLinks {
			return ErrItemHasProtectedIntegrationLinks
		}
		if metadata.OccurredAt.IsZero() {
			metadata.OccurredAt = time.Now()
		}
		recorder := itemevents.NewRecorder(s.db)
		if err := recordRemovedItemLinks(ctx, s.db, tx, itemIDs, metadata); err != nil {
			return err
		}
		for _, item := range items {
			removedDescendants := 0
			if item.ID == rootID {
				removedDescendants = descendantCount
			}
			if _, err := recorder.Deleted(ctx, tx, item, removedDescendants, metadata); err != nil {
				return err
			}
		}
		for _, id := range itemIDs {
			if err := s.deleteItemRelationsTx(tx, id); err != nil {
				return err
			}
			if err := s.repo.Delete(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ItemCRUDService) deleteItemRelationsTx(tx database.Tx, itemID int) error {
	if err := s.repo.DeleteItemWatches(tx, itemID); err != nil {
		return err
	}
	if err := s.repo.DeleteItemHistory(tx, itemID); err != nil {
		return err
	}
	if err := s.repo.DeleteItemLinks(tx, itemID); err != nil {
		return err
	}
	return s.repo.ClearWorklogItemReferences(tx, itemID)
}

func (s *ItemCRUDService) finishItemDeletion(workspaceID int, itemIDs []int, parentID *int) {
	repository.InvalidateItemListCountCache(s.db, workspaceID)
	for _, id := range itemIDs {
		PublishItemChange(id, ItemChangeDeleted)
	}
	if parentID != nil {
		PublishItemChange(*parentID, ItemChangeUpdated)
	}
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

// Copy creates a copy of an item.
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
func (s *ItemCRUDService) Copy(itemID int, opts CopyOptions) (*CopyResult, error) {
	source, err := s.repo.FindByID(itemID)
	if err != nil {
		return nil, fmt.Errorf("source item not found: %w", err)
	}

	parentID := opts.NewParentID
	if parentID == nil {
		parentID = source.ParentID
	}
	newItem := &models.Item{
		WorkspaceID:       source.WorkspaceID,
		ItemTypeID:        source.ItemTypeID,
		Title:             opts.NewTitle,
		Description:       source.Description,
		StatusID:          source.StatusID,
		PriorityID:        source.PriorityID,
		DueDate:           source.DueDate,
		StartDate:         source.StartDate,
		EndDate:           source.EndDate,
		IsTask:            source.IsTask,
		IterationID:       source.IterationID,
		ProjectID:         source.ProjectID,
		InheritProject:    source.InheritProject,
		AssigneeID:        source.AssigneeID,
		CreatorID:         &opts.CreatorID,
		CustomFieldValues: source.CustomFieldValues,
		ParentID:          parentID,
		RelatedWorkItemID: source.RelatedWorkItemID,
		StoryPoints:       source.StoryPoints,
	}
	if err := validation.ValidateTaskState(s.db, newItem.WorkspaceID, opts.CreatorID, newItem.IsTask, newItem.StatusID); err != nil {
		return nil, err
	}

	newID, err := s.repo.CreateWithRetry(context.Background(), newItem, func(tx database.Tx, itemID int) error {
		now := time.Now()
		if _, err := tx.Exec(`
			INSERT INTO item_milestones (item_id, milestone_id, created_at)
			SELECT ?, milestone_id, ? FROM item_milestones WHERE item_id = ?
		`, itemID, now, source.ID); err != nil {
			return fmt.Errorf("copy item milestones: %w", err)
		}
		created, err := s.repo.FindByIDForUpdate(tx, itemID)
		if err != nil {
			return err
		}
		metadata := itemevents.User(opts.CreatorID, "application")
		metadata.OccurredAt = now
		milestoneIDs, err := itemMilestoneIDsInTx(tx, itemID)
		if err != nil {
			return err
		}
		if err := s.repo.RecordHistoryBatch(tx, creationHistoryEntries(*created, opts.CreatorID, metadata.OccurredAt)); err != nil {
			return fmt.Errorf("record copied item creation history: %w", err)
		}
		_, err = itemevents.NewRecorder(s.db).Created(context.Background(), tx, created, milestoneIDs, metadata)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("copy item %d: %w", source.ID, err)
	}

	// Live-update publish (WI-483): the copy committed. Announce the new item and
	// refresh the destination parent's child list.
	PublishItemChange(newID, ItemChangeCreated)
	effectiveParent := opts.NewParentID
	if effectiveParent == nil {
		effectiveParent = source.ParentID
	}
	if effectiveParent != nil {
		PublishItemChange(*effectiveParent, ItemChangeUpdated)
	}

	return &CopyResult{NewItemID: newID, CopyCount: 1}, nil
}

// GetChildren returns direct children of an item
func (s *ItemCRUDService) GetChildren(parentID int) ([]*models.Item, error) {
	return s.repo.GetChildren(parentID)
}

// GetDescendants returns all descendants of an item
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
func (s *ItemCRUDService) GetDescendants(parentID int) ([]*models.Item, error) {
	return s.repo.GetDescendants(parentID)
}

// GetAncestors returns the ancestors of an item (path to root)
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
func (s *ItemCRUDService) GetAncestors(itemID int) ([]*models.Item, error) {
	return s.repo.GetAncestors(itemID)
}

// GetRootItems returns all root items for a workspace
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
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

// ListContext is the request-aware form of List. HTTP handlers should use it
// so disconnected clients do not leave SQL work occupying pool connections.
func (s *ItemCRUDService) ListContext(ctx context.Context, params ItemListParams) ([]models.Item, int, error) {
	return s.repo.FindAllWithDetailsContext(ctx, params)
}

// ListByIDsContext loads item summaries through the canonical workspace-scoped
// list query. IDs outside the supplied workspace scope are silently omitted.
func (s *ItemCRUDService) ListByIDsContext(ctx context.Context, itemIDs, workspaceIDs []int) ([]models.Item, error) {
	if len(itemIDs) == 0 || len(workspaceIDs) == 0 {
		return []models.Item{}, nil
	}

	const batchSize = 500
	items := make([]models.Item, 0, len(itemIDs))
	for start := 0; start < len(itemIDs); start += batchSize {
		end := min(start+batchSize, len(itemIDs))
		batch, _, err := s.ListContext(ctx, ItemListParams{
			WorkspaceIDs: workspaceIDs,
			Filters: ItemFilters{
				ItemIDs: itemIDs[start:end],
			},
			Pagination:       PaginationParams{Limit: end - start},
			OmitDescriptions: true,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}

	if err := repository.NewMilestoneAttachRepository(s.db).LoadForItemsContext(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// Search searches items by title and description
func (s *ItemCRUDService) Search(query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	return s.repo.Search(query, workspaceIDs, pagination)
}

// SearchContext is the request-aware form of Search.
func (s *ItemCRUDService) SearchContext(ctx context.Context, query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	return s.repo.SearchContext(ctx, query, workspaceIDs, pagination)
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
	return s.SearchWithFiltersContext(context.Background(), params)
}

// SearchWithFiltersContext is the request-aware form of SearchWithFilters.
func (s *ItemCRUDService) SearchWithFiltersContext(ctx context.Context, params SearchParams) ([]models.Item, int, error) {
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
		isKeyPattern := len(parts) == 2 && parts[0] != "" && parts[1] != ""
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

	return s.repo.FindAllWithDetailsContext(ctx, ItemListParams{
		WorkspaceIDs: params.WorkspaceIDs,
		Filters:      filters,
		Pagination:   params.Pagination,
		SortBy:       "updated_at",
	})
}

func (s *ItemCRUDService) resolveCollectionQLContext(ctx context.Context, qlQuery string, collectionID int) (resolvedQL string, isCollection bool, err error) {
	if qlQuery != "" {
		return qlQuery, false, nil
	}
	if collectionID <= 0 {
		return "", false, nil
	}
	_, collectionQL, err := s.workspaceRepo.GetCollectionQueryContext(ctx, collectionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", false, fmt.Errorf("%w: %d", ErrCollectionNotFound, collectionID)
		}
		return "", false, fmt.Errorf("failed to get collection query: %w", err)
	}
	if strings.TrimSpace(collectionQL) != "" {
		return collectionQL, true, nil
	}
	return "", true, nil
}

func (s *ItemCRUDService) evaluateQLContext(requestCtx context.Context, qlQuery string, functionCtx cql.FunctionContext) (qlSQL string, qlArgs []any, err error) {
	if qlQuery == "" {
		return "", nil, nil
	}
	qlQuery = cql.SubstituteFunctions(qlQuery, functionCtx)
	workspaceMap, err := s.workspaceRepo.BuildWorkspaceMapContext(requestCtx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build workspace map: %w", err)
	}
	customFieldMap, err := s.repo.GetCQLCustomFieldMapContext(requestCtx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build custom field map: %w", err)
	}
	evaluator := cql.NewEvaluator(workspaceMap, customFieldMap, s.db.GetDriverName())
	qlSQL, qlArgs, err = evaluator.EvaluateToSQL(qlQuery)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrQLQuery, err)
	}
	return qlSQL, qlArgs, nil
}

type resolvedItemListQL struct {
	sql                string
	args               []any
	collectionResolved bool
}

func (s *ItemCRUDService) resolveItemListQLContext(ctx context.Context, qlQuery string, collectionID int, subQL string, userID int) (resolvedItemListQL, error) {
	qlQuery, collectionResolved, err := s.resolveCollectionQLContext(ctx, qlQuery, collectionID)
	if err != nil {
		return resolvedItemListQL{}, err
	}
	if subQL = strings.TrimSpace(subQL); subQL != "" {
		if qlQuery == "" {
			qlQuery = subQL
		} else {
			qlQuery = "(" + qlQuery + ") AND (" + subQL + ")"
		}
	}
	qlSQL, qlArgs, err := s.evaluateQLContext(ctx, qlQuery, cql.UserContext(userID))
	if err != nil {
		return resolvedItemListQL{}, err
	}
	return resolvedItemListQL{sql: qlSQL, args: qlArgs, collectionResolved: collectionResolved}, nil
}

// BacklogParams contains parameters for retrieving backlog items
type BacklogParams struct {
	WorkspaceID      int    // 0 if not specified (collection-only query)
	CollectionID     int    // 0 if not specified
	QLQuery          string // Direct QL query, overrides collection
	SubQLQuery       string // Sub-filter QL query (ANDed with collection/direct QL)
	WorkspaceIDs     []int  // Accessible workspace IDs for security filtering
	UserID           int    // Authenticated user ID for currentUser() resolution
	Pagination       PaginationParams
	OmitDescriptions bool
}

// GetBacklogItems retrieves items with non-completed statuses for a workspace/collection
func (s *ItemCRUDService) GetBacklogItems(params BacklogParams) ([]models.Item, int, error) {
	return s.GetBacklogItemsContext(context.Background(), params)
}

// GetBacklogItemsContext is the request-aware form of GetBacklogItems.
func (s *ItemCRUDService) GetBacklogItemsContext(ctx context.Context, params BacklogParams) ([]models.Item, int, error) {
	if len(params.WorkspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	// Resolve backlog status IDs
	backlogStatusIDs, err := s.repo.GetBacklogStatusIDsContext(ctx, params.WorkspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get backlog statuses: %w", err)
	}
	if len(backlogStatusIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	filters := ItemFilters{
		StatusIDs: backlogStatusIDs,
	}

	resolvedQL, err := s.resolveItemListQLContext(ctx, params.QLQuery, params.CollectionID, params.SubQLQuery, params.UserID)
	if err != nil {
		return nil, 0, err
	}
	if resolvedQL.sql != "" {
		filters.QLQuery = resolvedQL.sql
		filters.QLArgs = resolvedQL.args
	}

	// Apply workspace_id filter only when no collection was resolved
	if !resolvedQL.collectionResolved && params.WorkspaceID > 0 {
		filters.WorkspaceID = &params.WorkspaceID
	}

	return s.repo.FindAllWithDetailsContext(ctx, ItemListParams{
		WorkspaceIDs:     params.WorkspaceIDs,
		Filters:          filters,
		Pagination:       params.Pagination,
		OmitDescriptions: params.OmitDescriptions,
	})
}

// ListWithQLParams contains parameters for listing items with QL support
type ListWithQLParams struct {
	WorkspaceID      int    // Single workspace filter (0 = all accessible)
	CollectionID     int    // Collection to resolve QL from (0 = none)
	QLQuery          string // Direct QL query (overrides collection)
	SubQLQuery       string // Sub-filter QL query (ANDed with base QL)
	WorkspaceIDs     []int  // Accessible workspace IDs for security filtering
	UserID           int    // Authenticated user ID for currentUser() resolution
	Filters          ItemFilters
	Pagination       PaginationParams
	SortBy           string
	SortAsc          bool
	OmitDescriptions bool
}

// ListWithQL retrieves items with QL evaluation and collection resolution
func (s *ItemCRUDService) ListWithQL(params ListWithQLParams) ([]models.Item, int, error) {
	return s.ListWithQLContext(context.Background(), params)
}

// ListWithQLContext is the request-aware form of ListWithQL.
func (s *ItemCRUDService) ListWithQLContext(ctx context.Context, params ListWithQLParams) ([]models.Item, int, error) {
	page, err := s.ListWithQLPageContext(ctx, params)
	return page.Items, page.Total, err
}

// ListWithQLPageContext preserves the shared collection/workspace resolution
// path while exposing the optional cursor continuation produced by the
// repository. Existing callers should keep using ListWithQLContext unless
// they need cursor metadata.
func (s *ItemCRUDService) ListWithQLPageContext(ctx context.Context, params ListWithQLParams) (repository.ItemListPage, error) {
	if len(params.WorkspaceIDs) == 0 {
		return repository.ItemListPage{Items: []models.Item{}}, nil
	}

	filters := params.Filters

	resolvedQL, err := s.resolveItemListQLContext(ctx, params.QLQuery, params.CollectionID, params.SubQLQuery, params.UserID)
	if err != nil {
		return repository.ItemListPage{}, err
	}
	if resolvedQL.sql != "" {
		filters.QLQuery = resolvedQL.sql
		filters.QLArgs = resolvedQL.args
	}

	// If collection was resolved but produced no effective query, return empty results.
	// A collection with no filter means "nothing to show yet."
	if resolvedQL.collectionResolved && filters.QLQuery == "" {
		return repository.ItemListPage{Items: []models.Item{}}, nil
	}

	// Apply workspace_id filter only when no collection was resolved
	if !resolvedQL.collectionResolved && params.WorkspaceID > 0 {
		filters.WorkspaceID = &params.WorkspaceID
	}

	return s.repo.FindAllWithDetailsPageContext(ctx, ItemListParams{
		WorkspaceIDs:     params.WorkspaceIDs,
		Filters:          filters,
		Pagination:       params.Pagination,
		SortBy:           params.SortBy,
		SortAsc:          params.SortAsc,
		OmitDescriptions: params.OmitDescriptions,
	})
}

// ListIDsWithQLPageContext evaluates CQL and returns only the matching item-ID
// page. It is intended for set-oriented enrichment such as batched link reads.
func (s *ItemCRUDService) ListIDsWithQLPageContext(ctx context.Context, params ListWithQLParams) (repository.ItemIDPage, error) {
	if len(params.WorkspaceIDs) == 0 {
		return repository.ItemIDPage{IDs: []int{}}, nil
	}

	qlQuery := strings.TrimSpace(params.QLQuery)
	qlSQL, qlArgs, err := s.evaluateQLContext(ctx, qlQuery, cql.UserContext(params.UserID))
	if err != nil {
		return repository.ItemIDPage{}, err
	}

	filters := params.Filters
	filters.QLQuery = qlSQL
	filters.QLArgs = qlArgs
	if params.WorkspaceID > 0 {
		filters.WorkspaceID = &params.WorkspaceID
	}

	return s.repo.FindIDPageContext(ctx, ItemListParams{
		WorkspaceIDs: params.WorkspaceIDs,
		Filters:      filters,
		Pagination:   params.Pagination,
		SortBy:       params.SortBy,
		SortAsc:      params.SortAsc,
	})
}

// ListDistinctWorkspaceIDsWithQLContext evaluates a CQL expression against
// the caller's accessible workspaces and returns only the workspace IDs that
// have matching items. This supports metadata views that need workspace-scoped
// catalogs without transferring every matching item first.
func (s *ItemCRUDService) ListDistinctWorkspaceIDsWithQLContext(
	ctx context.Context,
	qlQuery string,
	workspaceIDs []int,
	userID int,
) ([]int, error) {
	if len(workspaceIDs) == 0 || strings.TrimSpace(qlQuery) == "" {
		return []int{}, nil
	}

	qlSQL, qlArgs, err := s.evaluateQLContext(ctx, qlQuery, cql.UserContext(userID))
	if err != nil {
		return nil, err
	}
	return s.repo.FindDistinctWorkspaceIDsContext(ctx, ItemListParams{
		WorkspaceIDs: workspaceIDs,
		Filters: ItemFilters{
			QLQuery: qlSQL,
			QLArgs:  qlArgs,
		},
	})
}

// GetWithEffectiveProject retrieves an item with effective project calculated
// This is the most comprehensive Get method, used by the handler
// deadcode-keep: called by core-tests/internal/services/item_crud_service_test.go
func (s *ItemCRUDService) GetWithEffectiveProject(id int) (*models.Item, error) {
	item, err := s.repo.FindByIDWithDetails(id)
	if err != nil {
		return nil, err
	}

	// Resolve the effective project through the canonical resolver so this
	// shares the single source of truth with the item handler and the cache
	// (it keys off the inherit_project boolean and stops at the first ancestor
	// with a direct project).
	res, err := s.repo.ResolveEffectiveProject(id)
	if err != nil {
		return nil, err
	}
	switch {
	case res.DirectProjectID == nil && !res.InheritProject:
		item.ProjectInheritanceMode = "none"
	case res.InheritProject:
		item.EffectiveProjectID = res.EffectiveProjectID
		item.ProjectInheritanceMode = "inherit"
	default:
		item.EffectiveProjectID = res.EffectiveProjectID
		item.ProjectInheritanceMode = "direct"
	}

	// Populate the effective project name when one resolved.
	if item.EffectiveProjectID != nil {
		if item.ProjectID != nil && *item.ProjectID == *item.EffectiveProjectID {
			item.EffectiveProjectName = item.ProjectName
		} else {
			var name sql.NullString
			if err := s.db.QueryRow("SELECT name FROM time_projects WHERE id = ?", *item.EffectiveProjectID).Scan(&name); err != nil {
				slog.Warn("failed to look up project name", slog.Any("error", err))
			}
			if name.Valid {
				item.EffectiveProjectName = name.String
			}
		}
	}

	return item, nil
}

// GetHistory retrieves the change history for an item
func (s *ItemCRUDService) GetHistory(itemID int) ([]models.ItemHistory, error) {
	rows, err := s.db.Query(`
		SELECT h.id, h.item_id, h.user_id, h.changed_at, h.field_name, h.old_value, h.new_value,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM item_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.item_id = ?
		ORDER BY h.changed_at DESC, h.id DESC
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
			slog.Error("failed to scan item history row", slog.Int("item_id", itemID), slog.Any("error", err))
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate item history: %w", err)
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
		WHERE a.item_id = ? AND COALESCE(a.entity_type, 'item') = 'item'
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
			slog.Error("failed to scan attachment row", slog.Any("error", err))
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate attachments: %w", err)
	}

	if attachments == nil {
		attachments = []models.Attachment{}
	}

	return attachments, nil
}
