package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/logger"
	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

var (
	ErrItemForbidden             = errors.New("item access forbidden")
	ErrItemConflict              = errors.New("item mutation conflict")
	ErrItemTypeMigrationRequired = errors.New("item type migration requires a target status")
)

type ItemIssueSync interface {
	PushStatusToGitHub(context.Context, int, int)
}

type ItemListRequest struct {
	UserID           int
	WorkspaceID      int
	CollectionID     int
	QL               string
	SubQL            string
	Filters          ItemFilters
	Pagination       PaginationParams
	SortBy           string
	SortAsc          bool
	OmitDescriptions bool
	IncludeWatermark bool
}

type ItemListResult struct {
	Items          []models.Item
	Total          int
	NextCursor     string
	Watermark      int64
	SortableFields []string
}

type ItemApplicationService struct {
	db         database.Database
	items      *repository.ItemRepository
	crud       *ItemCRUDService
	perm       *PermissionService
	create     *ItemCreationService
	update     *ItemUpdateApplicationService
	delete     *ItemDeletionApplicationService
	activity   *ActivityTracker
	approvals  *ApprovalService
	hierarchy  *HierarchyService
	resolver   *IDResolverService
	move       *ItemWorkspaceMoveService
	cache      *ItemCacheService
	conditions *ConditionService
	events     *EventCoordinator
	issueSync  ItemIssueSync
	matrix     *TransitionMatrixService
	bulk       *ItemUpdateService
	mentions   *MentionService
}

func NewItemApplicationService(
	db database.Database,
	perm *PermissionService,
	create *ItemCreationService,
	update *ItemUpdateApplicationService,
	deleteService *ItemDeletionApplicationService,
) *ItemApplicationService {
	return &ItemApplicationService{
		db:        db,
		items:     repository.NewItemRepository(db),
		crud:      NewItemCRUDService(db),
		perm:      perm,
		create:    create,
		update:    update,
		delete:    deleteService,
		hierarchy: NewHierarchyService(db),
		resolver:  NewIDResolverService(db),
		move:      NewItemWorkspaceMoveService(db),
		matrix:    NewTransitionMatrixService(db),
		bulk:      NewItemUpdateService(db).WithPermissionService(perm),
	}
}

func (s *ItemApplicationService) WithCache(cache *ItemCacheService) *ItemApplicationService {
	s.cache = cache
	return s
}

func (s *ItemApplicationService) WithWorkflow(conditions *ConditionService, events *EventCoordinator, issueSync ItemIssueSync) *ItemApplicationService {
	s.conditions = conditions
	s.events = events
	s.issueSync = issueSync
	return s
}

func (s *ItemApplicationService) WithMutationEffects(mentions *MentionService) *ItemApplicationService {
	s.mentions = mentions
	return s
}

type ItemWatchStatus struct {
	ItemID   int  `json:"item_id"`
	Watching bool `json:"watching"`
}

type ItemDeleteInfo struct {
	HasChildren     bool   `json:"has_children"`
	DescendantCount int    `json:"descendant_count"`
	ParentID        *int   `json:"parent_id"`
	Title           string `json:"title"`
	ItemTypeID      *int   `json:"item_type_id"`
	WorkspaceID     int    `json:"workspace_id"`
	HierarchyLevel  *int   `json:"hierarchy_level"`
}

type ItemMutationCount struct {
	Count int `json:"count"`
}

type ItemRankInput struct {
	PreviousItemID *int
	NextItemID     *int
}

type ItemTransitionOption struct {
	ID            int    `json:"id"`
	BuiltinKey    string `json:"builtin_key"`
	Name          string `json:"name"`
	Value         string `json:"value"`
	CategoryColor string `json:"category_color,omitempty"`
}

type ItemTransitionSummary struct {
	CurrentStatus        string                  `json:"current_status"`
	AvailableTransitions []ItemTransitionOption  `json:"available_transitions"`
	PendingApproval      *PendingApprovalSummary `json:"pending_approval,omitempty"`
}

type ItemTransitionResult struct {
	Item        *models.Item `json:"item"`
	OldStatusID *int         `json:"old_status_id"`
	NewStatusID *int         `json:"new_status_id"`
	NoOp        bool         `json:"no_op"`
}

type ItemTypeChangeInput struct {
	TargetItemTypeID int
	TargetStatusID   *int
}

type ItemBacklogRequest struct {
	UserID, WorkspaceID, CollectionID int
	QL, SubQL                         string
	Pagination                        PaginationParams
	OmitDescriptions                  bool
	IncludeWatermark                  bool
}

type ItemChangesRequest struct {
	UserID, WorkspaceID, CollectionID int
	Since                             int64
	SinceProvided                     bool
	SubQL                             string
}

type ItemChangesResult struct {
	ChangedItemIDs     []int `json:"changed_item_ids"`
	RemovedItemIDs     []int `json:"removed_item_ids"`
	Watermark          int64 `json:"watermark"`
	RequiresFullReload bool  `json:"requires_full_reload"`
	MembershipDirty    bool  `json:"membership_dirty"`
}

type ItemBulkResult struct {
	Atomic         bool           `json:"atomic"`
	RequestedCount int            `json:"requested_count"`
	UpdatedCount   int            `json:"updated_count"`
	UnchangedCount int            `json:"unchanged_count"`
	Items          []*models.Item `json:"items"`
}

type RoadmapHierarchyDatesResult struct {
	Items     []models.RoadmapHierarchyDate `json:"items"`
	Truncated bool                          `json:"truncated"`
}

func (s *ItemApplicationService) WithReads(activity *ActivityTracker, approvals *ApprovalService) *ItemApplicationService {
	s.activity = activity
	s.approvals = approvals
	return s
}

func (s *ItemApplicationService) List(ctx context.Context, request ItemListRequest) (ItemListResult, error) {
	workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(request.UserID)
	if err != nil {
		return ItemListResult{}, err
	}
	if len(workspaceIDs) == 0 {
		return ItemListResult{Items: []models.Item{}, SortableFields: repository.SystemSortableFieldKeys()}, nil
	}

	page, err := s.crud.ListWithQLPageContext(ctx, ListWithQLParams{
		WorkspaceID: request.WorkspaceID, CollectionID: request.CollectionID,
		QLQuery: request.QL, SubQLQuery: request.SubQL,
		WorkspaceIDs: workspaceIDs, UserID: request.UserID,
		Filters: request.Filters, Pagination: request.Pagination,
		SortBy: request.SortBy, SortAsc: request.SortAsc,
		OmitDescriptions: request.OmitDescriptions,
	})
	if err != nil {
		return ItemListResult{}, err
	}
	if err := s.enrich(ctx, request.UserID, page.Items); err != nil {
		return ItemListResult{}, err
	}

	var watermark int64
	if request.IncludeWatermark {
		watermark, err = repository.NewItemChangeRepository(s.db).CurrentWatermark(workspaceIDs, request.WorkspaceID)
		if err != nil {
			return ItemListResult{}, err
		}
	}
	return ItemListResult{
		Items: page.Items, Total: page.Total, NextCursor: page.NextCursor,
		Watermark: watermark, SortableFields: repository.SystemSortableFieldKeys(),
	}, nil
}

func (s *ItemApplicationService) Backlog(ctx context.Context, request ItemBacklogRequest) (ItemListResult, error) {
	workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(request.UserID)
	if err != nil {
		return ItemListResult{}, err
	}
	items, total, err := s.crud.GetBacklogItemsContext(ctx, BacklogParams{
		WorkspaceID: request.WorkspaceID, CollectionID: request.CollectionID,
		QLQuery: request.QL, SubQLQuery: request.SubQL, WorkspaceIDs: workspaceIDs,
		UserID: request.UserID, Pagination: request.Pagination, OmitDescriptions: request.OmitDescriptions,
	})
	if err != nil {
		return ItemListResult{}, err
	}
	if err := s.enrich(ctx, request.UserID, items); err != nil {
		return ItemListResult{}, err
	}
	var watermark int64
	if request.IncludeWatermark {
		watermark, err = repository.NewItemChangeRepository(s.db).CurrentWatermark(workspaceIDs, request.WorkspaceID)
		if err != nil {
			return ItemListResult{}, err
		}
	}
	return ItemListResult{Items: items, Total: total, Watermark: watermark, SortableFields: repository.SystemSortableFieldKeys()}, nil
}

func (s *ItemApplicationService) Changes(ctx context.Context, request ItemChangesRequest) (ItemChangesResult, error) {
	workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(request.UserID)
	if err != nil {
		return ItemChangesResult{}, err
	}
	result := ItemChangesResult{ChangedItemIDs: []int{}, RemovedItemIDs: []int{}}
	if len(workspaceIDs) == 0 {
		return result, nil
	}
	if request.WorkspaceID > 0 {
		if err := s.require(request.UserID, request.WorkspaceID, models.PermissionItemView); err != nil {
			return ItemChangesResult{}, err
		}
	}
	changes := repository.NewItemChangeRepository(s.db)
	if request.CollectionID > 0 {
		exists, err := changes.CollectionExistsInWorkspace(request.CollectionID, request.WorkspaceID)
		if err != nil {
			return ItemChangesResult{}, err
		}
		if !exists {
			return ItemChangesResult{}, repository.ErrNotFound
		}
	}
	result.Watermark, err = changes.CurrentWatermark(workspaceIDs, request.WorkspaceID)
	if err != nil || !request.SinceProvided || request.Since >= result.Watermark {
		return result, err
	}
	entries, err := changes.QuerySince(workspaceIDs, request.WorkspaceID, request.Since, 501)
	if err != nil {
		return ItemChangesResult{}, err
	}
	if len(entries) > 500 {
		result.RequiresFullReload, result.MembershipDirty = true, true
		return result, nil
	}
	removed := make(map[int]bool)
	changed := make(map[int]bool)
	for _, entry := range entries {
		if entry.ItemID == 0 {
			result.RequiresFullReload, result.MembershipDirty = true, true
			return result, nil
		}
		if entry.Deleted {
			removed[entry.ItemID] = true
			continue
		}
		visible, err := s.itemVisibleInDelta(ctx, request, workspaceIDs, entry.ItemID)
		if err != nil {
			return ItemChangesResult{}, err
		}
		if visible {
			changed[entry.ItemID] = true
		} else {
			removed[entry.ItemID] = true
		}
	}
	for id := range removed {
		result.RemovedItemIDs = append(result.RemovedItemIDs, id)
	}
	for id := range changed {
		if !removed[id] {
			result.ChangedItemIDs = append(result.ChangedItemIDs, id)
		}
	}
	result.MembershipDirty = len(result.RemovedItemIDs) > 0
	return result, nil
}

func (s *ItemApplicationService) BulkUpdate(ctx context.Context, actor AuditActor, itemIDs []int, fields map[string]any) (ItemBulkResult, error) {
	result, err := s.bulk.BulkUpdateItems(ctx, BulkUpdateItemsRequest{
		ItemIDs: itemIDs, Fields: fields, UserID: actor.UserID,
		AuthorizeWorkspace: func(workspaceID int) (bool, error) {
			err := s.require(actor.UserID, workspaceID, models.PermissionItemEdit)
			return err == nil, unwrapItemPermission(err)
		},
	})
	if err != nil {
		return ItemBulkResult{}, err
	}
	items := s.applyBulkEffects(actor, result.Results)
	return ItemBulkResult{Atomic: true, RequestedCount: result.RequestedCount, UpdatedCount: result.UpdatedCount, UnchangedCount: result.UnchangedCount, Items: items}, nil
}

func (s *ItemApplicationService) BulkPatch(ctx context.Context, actor AuditActor, patches []BulkItemPatch) (ItemBulkResult, error) {
	result, err := s.bulk.BulkPatchItems(ctx, BulkPatchItemsRequest{
		Patches: patches, UserID: actor.UserID,
		AuthorizeWorkspace: func(workspaceID int) (bool, error) {
			err := s.require(actor.UserID, workspaceID, models.PermissionItemEdit)
			return err == nil, unwrapItemPermission(err)
		},
	})
	if err != nil {
		return ItemBulkResult{}, err
	}
	items := s.applyBulkEffects(actor, result.Results)
	return ItemBulkResult{Atomic: true, RequestedCount: result.RequestedCount, UpdatedCount: result.UpdatedCount, UnchangedCount: result.UnchangedCount, Items: items}, nil
}

func (s *ItemApplicationService) RoadmapHierarchyDates(ctx context.Context, userID int, rootIDs []int) (RoadmapHierarchyDatesResult, error) {
	rootWorkspaces, err := s.items.GetRoadmapHierarchyRootWorkspaceIDs(ctx, rootIDs)
	if err != nil {
		return RoadmapHierarchyDatesResult{}, err
	}
	seen := make(map[int]bool)
	permissions := make(map[int]bool)
	authorized := make([]int, 0, len(rootIDs))
	for _, id := range rootIDs {
		workspaceID, exists := rootWorkspaces[id]
		if id < 1 || seen[id] || !exists {
			continue
		}
		seen[id] = true
		allowed, known := permissions[workspaceID]
		if !known {
			allowed, err = s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
			if err != nil {
				return RoadmapHierarchyDatesResult{}, err
			}
			permissions[workspaceID] = allowed
		}
		if allowed {
			authorized = append(authorized, id)
		}
	}
	items, truncated, err := s.items.GetRoadmapHierarchyDates(ctx, authorized)
	if err != nil {
		return RoadmapHierarchyDatesResult{}, err
	}
	filtered := items[:0]
	for _, item := range items {
		if permissions[item.WorkspaceID] {
			filtered = append(filtered, item)
		}
	}
	return RoadmapHierarchyDatesResult{Items: filtered, Truncated: truncated}, nil
}

func (s *ItemApplicationService) Get(ctx context.Context, userID, itemID int, trackView bool) (*models.Item, error) {
	result, err := s.crud.GetByIDWithWorkspaceStatus(itemID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.canView(ctx, userID, result.ID, result.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}
	if !result.WorkspaceActive {
		allowed, err = s.perm.HasWorkspacePermission(userID, result.WorkspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, repository.ErrNotFound
		}
	}

	item, err := s.crud.GetWithEffectiveProject(itemID)
	if err != nil {
		return nil, err
	}
	items := []models.Item{*item}
	if err := s.enrich(ctx, userID, items); err != nil {
		return nil, err
	}
	item = &items[0]
	item.DescriptionHTML, err = markdown.Render(item.Description)
	if err != nil {
		return nil, fmt.Errorf("render item description: %w", err)
	}
	if trackView && s.activity != nil {
		_ = s.activity.TrackItemActivity(userID, itemID, ActivityView)
	}
	return item, nil
}

func (s *ItemApplicationService) GetByKey(ctx context.Context, userID int, workspaceKey string, itemNumber int) (*models.Item, error) {
	id, err := s.items.FindIDByKeyAndNumber(workspaceKey, itemNumber)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id, true)
}

func (s *ItemApplicationService) Batch(ctx context.Context, userID int, ids []int) ([]models.Item, error) {
	loaded, err := s.items.FindByIDsWithDetails(ids)
	if err != nil {
		return nil, err
	}
	items := make([]models.Item, 0, len(loaded))
	permissions := make(map[int]bool)
	for _, item := range loaded {
		allowed, ok := permissions[item.WorkspaceID]
		if !ok {
			allowed, err = s.perm.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemView)
			if err != nil {
				return nil, err
			}
			permissions[item.WorkspaceID] = allowed
		}
		if allowed {
			items = append(items, *item)
		}
	}
	if err := s.enrich(ctx, userID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *ItemApplicationService) Create(ctx context.Context, actor AuditActor, input ItemCreateInput) (*models.Item, error) {
	if err := s.require(actor.UserID, input.WorkspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	result, err := s.create.Create(actor.UserID, actor.Username, input)
	if err != nil {
		return nil, err
	}
	items := []models.Item{*result.Item}
	if err := s.enrich(ctx, actor.UserID, items); err != nil {
		return nil, err
	}
	items[0].DescriptionHTML, err = markdown.Render(items[0].Description)
	if err != nil {
		return nil, fmt.Errorf("render item description: %w", err)
	}
	return &items[0], nil
}

func (s *ItemApplicationService) Patch(ctx context.Context, actor AuditActor, itemID int, fields map[string]json.RawMessage) (*models.Item, error) {
	allowed, err := s.update.CanUserEditItem(actor.UserID, itemID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrItemForbidden
	}
	result, err := s.update.UpdateJSONFields(actor.UserID, actor.Username, itemID, fields)
	if err != nil {
		return nil, err
	}
	items := []models.Item{*result.Item}
	if err := s.enrich(ctx, actor.UserID, items); err != nil {
		return nil, err
	}
	items[0].DescriptionHTML, err = markdown.Render(items[0].Description)
	if err != nil {
		return nil, fmt.Errorf("render item description: %w", err)
	}
	return &items[0], nil
}

func (s *ItemApplicationService) Delete(actor AuditActor, itemID int) error {
	result, err := s.delete.Delete(ItemDeletionRequest{
		ItemID: itemID, ActorUserID: actor.UserID, ActorUsername: actor.Username, Mode: ItemDeletionSingle,
	})
	if err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionItemDelete, logger.ResourceItem, &itemID, result.Item.Title, map[string]any{
		"workspace_id": result.Item.WorkspaceID, "item_type_id": result.Item.ItemTypeID,
		"parent_id": result.Item.ParentID, "status_id": result.Item.StatusID,
	})
	return nil
}

func (s *ItemApplicationService) DeleteInfo(userID, itemID int) (ItemDeleteInfo, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return ItemDeleteInfo{}, err
	}
	if err := s.require(userID, item.WorkspaceID, models.PermissionItemEdit); err != nil {
		return ItemDeleteInfo{}, err
	}
	descendants, err := s.items.GetDescendantIDs(itemID)
	if err != nil {
		return ItemDeleteInfo{}, err
	}
	var level sql.NullInt64
	if item.ItemTypeID != nil {
		if err := s.db.QueryRow("SELECT hierarchy_level FROM item_types WHERE id = ?", *item.ItemTypeID).Scan(&level); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ItemDeleteInfo{}, err
		}
	}
	var hierarchyLevel *int
	if level.Valid {
		value := int(level.Int64)
		hierarchyLevel = &value
	}
	return ItemDeleteInfo{
		HasChildren: len(descendants) > 0, DescendantCount: len(descendants), ParentID: item.ParentID,
		Title: item.Title, ItemTypeID: item.ItemTypeID, WorkspaceID: item.WorkspaceID, HierarchyLevel: hierarchyLevel,
	}, nil
}

func (s *ItemApplicationService) DeleteCascade(actor AuditActor, itemID int) (ItemMutationCount, error) {
	result, err := s.delete.Delete(ItemDeletionRequest{
		ItemID: itemID, ActorUserID: actor.UserID, ActorUsername: actor.Username, Mode: ItemDeletionCascade,
	})
	if err != nil {
		return ItemMutationCount{}, err
	}
	emitServiceAudit(s.db, actor, logger.ActionItemDeleteCascade, logger.ResourceItem, &itemID, result.Item.Title, map[string]any{
		"workspace_id": result.Item.WorkspaceID, "deleted_count": result.DeletedCount, "descendant_count": result.DescendantCount,
	})
	return ItemMutationCount{Count: result.DeletedCount}, nil
}

func (s *ItemApplicationService) ReparentChildren(ctx context.Context, actor AuditActor, itemID int, parentID *int) (ItemMutationCount, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return ItemMutationCount{}, err
	}
	if err := s.require(actor.UserID, item.WorkspaceID, models.PermissionItemEdit); err != nil {
		return ItemMutationCount{}, err
	}
	if parentID != nil {
		parent, err := s.items.FindByID(*parentID)
		if err != nil {
			return ItemMutationCount{}, err
		}
		if parent.WorkspaceID != item.WorkspaceID {
			return ItemMutationCount{}, &validation.ValidationError{Field: "parent_id", Message: "parent must be in the same workspace"}
		}
	}
	children, err := s.items.GetChildren(itemID)
	if err != nil || len(children) == 0 {
		return ItemMutationCount{Count: len(children)}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ItemMutationCount{}, err
	}
	defer tx.Rollback()
	if parentID != nil {
		cycle, err := s.hierarchy.WouldCreateCycleTx(tx, itemID, *parentID)
		if err != nil {
			return ItemMutationCount{}, err
		}
		if cycle {
			return ItemMutationCount{}, &validation.ValidationError{Field: "parent_id", Message: "reparenting would create a hierarchy cycle"}
		}
	}
	records := make([]itemevents.UpdateRecord, 0, len(children))
	metadata := itemevents.User(actor.UserID, "application")
	metadata.OccurredAt = time.Now()
	for _, child := range children {
		if child.ItemTypeID != nil {
			if err := validation.ValidateParentForItemType(tx, *child.ItemTypeID, parentID); err != nil {
				return ItemMutationCount{}, err
			}
		}
		if err := s.items.UpdateParent(tx, child.ID, parentID); err != nil {
			return ItemMutationCount{}, err
		}
		updated := *child
		updated.ParentID = parentID
		records = append(records, itemevents.UpdateRecord{Item: &updated, Changes: itemevents.Changes(child, &updated), Metadata: metadata})
	}
	if _, err := itemevents.NewRecorder(s.db).UpdatedBatch(ctx, tx, records); err != nil {
		return ItemMutationCount{}, err
	}
	if err := tx.Commit(); err != nil {
		return ItemMutationCount{}, err
	}
	if s.cache != nil {
		for _, child := range children {
			_ = s.cache.InvalidateItemHierarchy(child.ID, nil)
		}
	}
	return ItemMutationCount{Count: len(children)}, nil
}

func (s *ItemApplicationService) Copy(ctx context.Context, actor AuditActor, itemID int) (*models.Item, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.require(actor.UserID, item.WorkspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	titleRunes := []rune(fmt.Sprintf("COPY - %s", item.Title))
	if len(titleRunes) > validation.TitleMaxRunes {
		titleRunes = titleRunes[:validation.TitleMaxRunes]
	}
	title, err := validation.NormalizeTitle(string(titleRunes))
	if err != nil {
		return nil, err
	}
	result, err := s.crud.Copy(itemID, CopyOptions{NewTitle: title, CreatorID: actor.UserID})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, actor.UserID, result.NewItemID, false)
}

func (s *ItemApplicationService) MovePreview(userID, itemID int, input ItemWorkspaceMoveInput) (*ItemWorkspaceMovePreview, error) {
	if err := s.requireWorkspaceMove(userID, itemID, input.DestinationWorkspaceID); err != nil {
		return nil, err
	}
	return s.move.Preview(itemID, input)
}

func (s *ItemApplicationService) MoveWorkspace(ctx context.Context, actor AuditActor, itemID int, input ItemWorkspaceMoveInput) (*ItemWorkspaceMoveResult, error) {
	if err := s.requireWorkspaceMove(actor.UserID, itemID, input.DestinationWorkspaceID); err != nil {
		return nil, err
	}
	result, err := s.move.MoveContext(ctx, itemID, actor.UserID, input)
	if err != nil {
		return nil, err
	}
	s.invalidateHierarchy(itemID)
	for _, childID := range result.DetachedChildIDs {
		s.invalidateHierarchy(childID)
	}
	emitServiceAudit(s.db, actor, logger.ActionItemMoveWorkspace, logger.ResourceItem, &itemID, result.Item.Title, map[string]any{
		"old_key": result.OldKey, "new_key": result.NewKey, "fields": result.Preview.Fields,
		"labels_kept": result.Preview.LabelsKept, "labels_dropped": result.Preview.LabelsDropped,
	})
	return result, nil
}

func (s *ItemApplicationService) UpdateRank(ctx context.Context, actor AuditActor, itemID int, input ItemRankInput) (*models.Item, error) {
	workspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.require(actor.UserID, workspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	var previous, next string
	for _, neighbor := range []struct {
		id    *int
		value *string
	}{{input.PreviousItemID, &previous}, {input.NextItemID, &next}} {
		if neighbor.id == nil {
			continue
		}
		value, err := s.items.GetFracIndex(*neighbor.id)
		if err != nil || value == nil {
			return nil, ErrItemConflict
		}
		*neighbor.value = *value
	}
	current, err := s.items.GetFracIndex(itemID)
	if err != nil {
		return nil, err
	}
	if previous != "" && next != "" && previous == next || current != nil && (previous == "" || *current > previous) && (next == "" || *current < next) {
		return s.Get(ctx, actor.UserID, itemID, false)
	}
	if _, err := repository.MoveItemBetween(s.db, itemID, input.PreviousItemID, input.NextItemID); err != nil {
		if repository.IsFracIndexUniqueViolation(err) {
			return nil, ErrItemConflict
		}
		return nil, err
	}
	return s.Get(ctx, actor.UserID, itemID, false)
}

func (s *ItemApplicationService) Transition(ctx context.Context, actor AuditActor, itemID, statusID int) (*ItemTransitionResult, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.require(actor.UserID, item.WorkspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	if s.activity != nil {
		_ = s.activity.TrackItemActivity(actor.UserID, itemID, ActivityEdit)
	}
	result, err := NewWorkflowService(s.db).PerformTransition(ctx, PerformTransitionRequest{
		ItemID: itemID, ToStatusID: statusID, ActorUserID: actor.UserID, Modes: []string{"validator", "condition"},
	}, s.items, s.conditions, s.approvals)
	if err != nil {
		return nil, err
	}
	if !result.NoOp && s.events != nil {
		s.events.EmitStatusChanged(result.Item, result.OldStatusID, result.NewStatusID, actor.UserID, actor.Username)
	}
	s.syncStatus(ctx, result.Item.ID, result.NewStatusID, result.NoOp)
	items := []models.Item{*result.Item}
	if err := s.enrich(ctx, actor.UserID, items); err != nil {
		return nil, err
	}
	return &ItemTransitionResult{
		Item: &items[0], OldStatusID: result.OldStatusID, NewStatusID: result.NewStatusID, NoOp: result.NoOp,
	}, nil
}

func (s *ItemApplicationService) AvailableTransitions(ctx context.Context, userID, itemID int) (ItemTransitionSummary, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return ItemTransitionSummary{}, err
	}
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return ItemTransitionSummary{}, err
	}
	workflow := NewWorkflowService(s.db)
	response := ItemTransitionSummary{AvailableTransitions: []ItemTransitionOption{}}
	if item.StatusID != nil {
		response.CurrentStatus, err = workflow.GetStatusName(int64(*item.StatusID))
		if err != nil {
			return ItemTransitionSummary{}, err
		}
	}
	workflowID, err := workflow.GetWorkflowIDForItem(item.WorkspaceID, item.ItemTypeID)
	if err != nil || workflowID == nil {
		return response, err
	}
	if item.StatusID == nil {
		return response, nil
	}
	if current, err := workflow.GetStatusTransitionOption(int64(*item.StatusID)); err != nil {
		return ItemTransitionSummary{}, err
	} else if current != nil {
		response.AvailableTransitions = append(response.AvailableTransitions, itemTransitionOption(*current))
	}
	options, err := workflow.ListAvailableTransitionOptions(*workflowID, int64(*item.StatusID))
	if err != nil {
		return ItemTransitionSummary{}, err
	}
	if s.approvals != nil {
		gatedIDs, summary, gateErr := s.approvals.GetGatedTransitionsForItem(ctx, itemID, userID)
		if gateErr == nil {
			gated := make(map[int]bool, len(gatedIDs))
			for _, id := range gatedIDs {
				gated[id] = true
			}
			kept := options[:0]
			for _, option := range options {
				if !gated[option.TransitionID] {
					kept = append(kept, option)
				}
			}
			options = kept
			response.PendingApproval = summary
		}
	}
	if s.conditions != nil {
		conditionSetID, conditionErr := s.conditions.GetConditionSetIDForItem(item.WorkspaceID, item.ItemTypeID)
		if conditionErr == nil && conditionSetID != nil {
			candidates := make([]TransitionWithID, 0, len(options))
			for _, option := range options {
				color := ""
				if option.CategoryColor != nil {
					color = *option.CategoryColor
				}
				candidates = append(candidates, TransitionWithID{TransitionID: option.TransitionID, StatusID: option.StatusID, BuiltinKey: option.BuiltinKey, StatusName: option.StatusName, CategoryColor: color})
			}
			filtered, filterErr := s.conditions.FilterTransitionsByConditions(ctx, *conditionSetID, candidates, userID, BuildItemContextFromIDs(s.db, itemID, item.WorkspaceID, item.StatusID, item.ItemTypeID))
			if filterErr == nil {
				options = options[:0]
				for _, option := range filtered {
					var color *string
					if option.CategoryColor != "" {
						value := option.CategoryColor
						color = &value
					}
					options = append(options, StatusTransitionOption{TransitionID: option.TransitionID, StatusID: option.StatusID, BuiltinKey: option.BuiltinKey, StatusName: option.StatusName, CategoryColor: color})
				}
			}
		}
	}
	seen := map[int]bool{*item.StatusID: true}
	for _, option := range options {
		if !seen[option.StatusID] {
			response.AvailableTransitions = append(response.AvailableTransitions, itemTransitionOption(option))
			seen[option.StatusID] = true
		}
	}
	return response, nil
}

func (s *ItemApplicationService) TransitionMatrix(ctx context.Context, userID, workspaceID int) (map[string][]ItemTransitionOption, error) {
	if err := s.require(userID, workspaceID, models.PermissionItemView); err != nil {
		return nil, err
	}
	matrix, err := s.matrix.Load(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]ItemTransitionOption)
	for itemTypeID, byStatus := range matrix.ByItemType {
		for statusID, options := range byStatus {
			key := strconv.Itoa(itemTypeID) + ":" + strconv.Itoa(statusID)
			result[key] = make([]ItemTransitionOption, 0, len(options))
			for _, option := range options {
				result[key] = append(result[key], itemTransitionOption(option))
			}
		}
	}
	return result, nil
}

func (s *ItemApplicationService) AnalyzeTypeChange(userID, itemID, targetTypeID int) (*ItemTypeChangeAnalysis, error) {
	item, err := s.items.FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, item.WorkspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	return s.typeChangeService().Analyze(item, targetTypeID)
}

func (s *ItemApplicationService) ChangeType(ctx context.Context, actor AuditActor, itemID int, input ItemTypeChangeInput) (*models.Item, error) {
	original, err := s.items.FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.require(actor.UserID, original.WorkspaceID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	service := s.typeChangeService()
	analysis, err := service.Analyze(original, input.TargetItemTypeID)
	if err != nil {
		return nil, err
	}
	if original.ItemTypeID != nil && *original.ItemTypeID == input.TargetItemTypeID && !analysis.RequiresMigration {
		return s.Get(ctx, actor.UserID, itemID, false)
	}
	var nextStatusID *int
	if analysis.RequiresMigration {
		if input.TargetStatusID == nil {
			return nil, ErrItemTypeMigrationRequired
		}
		if analysis.TargetWorkflowID != nil {
			allowed, err := service.IsStatusInWorkflow(*input.TargetStatusID, *analysis.TargetWorkflowID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, &validation.ValidationError{Field: "target_status_id", Message: "status is not part of the target workflow"}
			}
		}
		if err := service.ValidateStatusMapping(ctx, original, input.TargetItemTypeID, analysis.TargetWorkflowID, input.TargetStatusID); err != nil {
			return nil, err
		}
		nextStatusID = input.TargetStatusID
	}
	if s.activity != nil {
		_ = s.activity.TrackItemActivity(actor.UserID, itemID, ActivityEdit)
	}
	history, err := service.ApplyChange(itemID, actor.UserID, input.TargetItemTypeID, nextStatusID, original)
	if err != nil {
		return nil, err
	}
	updated, err := s.items.FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	statusChanged := nextStatusID != nil && !equalItemIntPointers(original.StatusID, updated.StatusID)
	if s.events != nil {
		s.events.EmitItemUpdated(original, updated, statusChanged, false, actor.UserID, history, actor.Username)
	}
	s.syncStatus(ctx, updated.ID, updated.StatusID, !statusChanged)
	return s.Get(ctx, actor.UserID, itemID, false)
}

func (s *ItemApplicationService) Children(ctx context.Context, userID, itemID int) ([]models.Item, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	items, err := s.hierarchy.GetChildrenContext(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, userID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *ItemApplicationService) Ancestors(ctx context.Context, userID, itemID int) ([]models.Item, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	items, err := s.hierarchy.GetAncestorsContext(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, userID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *ItemApplicationService) Descendants(ctx context.Context, userID, itemID, maxDepth int) ([]models.Item, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	items, err := s.hierarchy.GetDescendantsContext(ctx, itemID, maxDepth)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, userID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *ItemApplicationService) TimeRollup(ctx context.Context, userID, itemID, maxDepth int) (*models.TimeRollup, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	return s.items.GetTimeRollup(itemID, maxDepth, 0)
}

func (s *ItemApplicationService) StatusDurations(ctx context.Context, userID, itemID int) (*models.ItemStatusDurations, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	return s.items.GetStatusDurations(ctx, itemID, time.Now())
}

func (s *ItemApplicationService) History(ctx context.Context, userID, itemID int) ([]models.ItemHistory, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	includeAgentOwner, _ := s.perm.IsSystemAdmin(userID)
	if !includeAgentOwner {
		includeAgentOwner, _ = s.perm.HasGlobalPermission(userID, models.PermissionUserList)
	}
	history, err := s.items.GetHistoryWithApprovals(itemID, includeAgentOwner)
	if err != nil {
		return nil, err
	}
	allowedProjects := s.allowedProjectIDs(userID)
	for i := range history {
		s.resolveHistory(&history[i], allowedProjects)
	}
	return history, nil
}

func (s *ItemApplicationService) Watch(ctx context.Context, userID, itemID int, reason string) (ItemWatchStatus, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return ItemWatchStatus{}, err
	}
	if strings.TrimSpace(reason) == "" {
		reason = "User subscribed to item notifications"
	}
	if err := s.items.Watch(userID, itemID, reason); err != nil {
		return ItemWatchStatus{}, err
	}
	if s.activity != nil {
		_ = s.activity.InvalidateUserCache(userID)
	}
	return ItemWatchStatus{ItemID: itemID, Watching: true}, nil
}

func (s *ItemApplicationService) Unwatch(ctx context.Context, userID, itemID int) (ItemWatchStatus, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return ItemWatchStatus{}, err
	}
	if err := s.items.Unwatch(userID, itemID); err != nil {
		return ItemWatchStatus{}, err
	}
	if s.activity != nil {
		_ = s.activity.InvalidateUserCache(userID)
	}
	return ItemWatchStatus{ItemID: itemID, Watching: false}, nil
}

func (s *ItemApplicationService) WatchStatus(ctx context.Context, userID, itemID int) (ItemWatchStatus, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return ItemWatchStatus{}, err
	}
	watching, err := s.items.IsWatching(userID, itemID)
	return ItemWatchStatus{ItemID: itemID, Watching: watching}, err
}

func (s *ItemApplicationService) PersonalTasks(ctx context.Context, userID, itemID int) ([]models.Item, error) {
	if err := s.requireView(ctx, userID, itemID); err != nil {
		return nil, err
	}
	workspaceID, err := repository.NewWorkspaceRepository(s.db).GetActivePersonalWorkspaceID(userID)
	if errors.Is(err, repository.ErrNotFound) {
		return []models.Item{}, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := s.items.ListRelatedPersonalItems(itemID, workspaceID)
	if items == nil {
		items = []models.Item{}
	}
	return items, err
}

func (s *ItemApplicationService) UnlinkPersonalTask(userID, itemID int) error {
	ownership, err := s.items.GetItemWorkspaceOwnership(itemID)
	if err != nil {
		return err
	}
	if !ownership.IsPersonal || ownership.OwnerID == nil || *ownership.OwnerID != userID {
		return ErrItemForbidden
	}
	return s.items.ClearRelatedWorkItem(itemID)
}

func (s *ItemApplicationService) require(userID, workspaceID int, permission string) error {
	if s.perm == nil {
		return ErrItemForbidden
	}
	allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrItemForbidden
	}
	return nil
}

func (s *ItemApplicationService) requireWorkspaceMove(userID, itemID, destinationWorkspaceID int) error {
	if destinationWorkspaceID < 1 {
		return &validation.ValidationError{Field: "destination_workspace_id", Message: "destination_workspace_id is required"}
	}
	sourceWorkspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		return err
	}
	if err := s.require(userID, sourceWorkspaceID, models.PermissionItemEdit); err != nil {
		return err
	}
	return s.require(userID, destinationWorkspaceID, models.PermissionItemCreate)
}

func (s *ItemApplicationService) invalidateHierarchy(itemID int) {
	if s.cache == nil {
		return
	}
	_ = s.cache.InvalidateItemHierarchy(itemID, nil)
	descendants, err := s.hierarchy.GetDescendants(itemID, 0)
	if err != nil {
		return
	}
	for i := range descendants {
		_ = s.cache.InvalidateItemHierarchy(descendants[i].ID, nil)
	}
}

func (s *ItemApplicationService) typeChangeService() *ItemTypeChangeService {
	service := NewItemTypeChangeService(s.db)
	if s.conditions != nil {
		service = service.WithConditionService(s.conditions)
	}
	return service
}

func (s *ItemApplicationService) syncStatus(ctx context.Context, itemID int, statusID *int, skip bool) {
	if skip || statusID == nil || s.issueSync == nil {
		return
	}
	go func(status int) {
		syncContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		s.issueSync.PushStatusToGitHub(syncContext, itemID, status)
	}(*statusID)
}

func itemTransitionOption(option StatusTransitionOption) ItemTransitionOption {
	result := ItemTransitionOption{
		ID: option.StatusID, BuiltinKey: option.BuiltinKey, Name: option.StatusName,
		Value: strings.ToLower(strings.ReplaceAll(option.StatusName, " ", "_")),
	}
	if option.CategoryColor != nil {
		result.CategoryColor = *option.CategoryColor
	}
	return result
}

func equalItemIntPointers(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func unwrapItemPermission(err error) error {
	if errors.Is(err, ErrItemForbidden) {
		return nil
	}
	return err
}

func (s *ItemApplicationService) itemVisibleInDelta(ctx context.Context, request ItemChangesRequest, workspaceIDs []int, itemID int) (bool, error) {
	items, _, err := s.crud.ListWithQLContext(ctx, ListWithQLParams{
		WorkspaceID: request.WorkspaceID, CollectionID: request.CollectionID, SubQLQuery: request.SubQL,
		WorkspaceIDs: workspaceIDs, UserID: request.UserID, Filters: ItemFilters{ItemID: &itemID},
		Pagination: PaginationParams{Limit: 1},
	})
	if errors.Is(err, ErrCollectionNotFound) {
		return false, nil
	}
	return len(items) > 0, err
}

func (s *ItemApplicationService) applyBulkEffects(actor AuditActor, results []UpdateItemResult) []*models.Item {
	items := make([]models.Item, 0, len(results))
	for i := range results {
		result := &results[i]
		if result.OriginalItem == nil || result.Item == nil {
			continue
		}
		original, updated := result.OriginalItem, result.Item
		if s.activity != nil {
			_ = s.activity.TrackItemActivity(actor.UserID, updated.ID, ActivityEdit)
		}
		if s.cache != nil && (original.InheritProject != updated.InheritProject || !equalItemIntPointers(original.ProjectID, updated.ProjectID) || !equalItemIntPointers(original.ParentID, updated.ParentID)) {
			s.invalidateHierarchy(updated.ID)
		}
		if s.events != nil {
			s.events.EmitItemUpdated(original, updated, result.StatusChanged, !equalItemIntPointers(original.AssigneeID, updated.AssigneeID), actor.UserID, result.FieldChanges, actor.Username)
		}
		if s.mentions != nil && original.Description != updated.Description {
			_ = s.mentions.ProcessMentions(ProcessMentionsParams{
				SourceType: "item_description", SourceID: updated.ID, Content: updated.Description,
				ItemID: updated.ID, WorkspaceID: updated.WorkspaceID, ActorUserID: actor.UserID,
			})
		}
		items = append(items, *updated)
	}
	NewTimePermissionService(s.db, s.perm).MaskInaccessibleProjectNames(actor.UserID, items)
	MaskInaccessibleRelatedWorkItems(actor.UserID, items, s.perm)
	output := make([]*models.Item, len(items))
	for i := range items {
		output[i] = &items[i]
	}
	return output
}

func (s *ItemApplicationService) canView(ctx context.Context, userID, itemID, workspaceID int) (bool, error) {
	if err := s.require(userID, workspaceID, models.PermissionItemView); err == nil {
		return true, nil
	} else if !errors.Is(err, ErrItemForbidden) {
		return false, err
	}
	if s.approvals == nil {
		return false, nil
	}
	return s.approvals.UserHasActivePoolMembershipOnItem(ctx, userID, itemID, nil)
}

func (s *ItemApplicationService) requireView(ctx context.Context, userID, itemID int) error {
	workspaceID, err := s.items.GetWorkspaceIDCtx(ctx, itemID)
	if err != nil {
		return err
	}
	allowed, err := s.canView(ctx, userID, itemID, workspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		return repository.ErrNotFound
	}
	return nil
}

func (s *ItemApplicationService) allowedProjectIDs(userID int) map[int]struct{} {
	projects, err := NewTimePermissionService(s.db, s.perm).GetAccessibleProjects(userID)
	if err != nil {
		return map[int]struct{}{}
	}
	if projects == nil {
		return nil
	}
	allowed := make(map[int]struct{}, len(projects))
	for _, id := range projects {
		allowed[id] = struct{}{}
	}
	return allowed
}

func (s *ItemApplicationService) resolveHistory(entry *models.ItemHistory, allowedProjects map[int]struct{}) {
	if entry.OldValue != nil && *entry.OldValue != "" {
		if value := s.resolveHistoryValue(entry.FieldName, *entry.OldValue, allowedProjects); value != "" {
			entry.ResolvedOldValue = &value
		}
	}
	if entry.NewValue != nil && *entry.NewValue != "" {
		if value := s.resolveHistoryValue(entry.FieldName, *entry.NewValue, allowedProjects); value != "" {
			entry.ResolvedNewValue = &value
		}
	}
}

func (s *ItemApplicationService) resolveHistoryValue(field, value string, allowedProjects map[int]struct{}) string {
	if field == "milestones" {
		names := make([]string, 0)
		for _, part := range strings.Split(value, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				if name := s.resolver.ResolveMilestoneName(id); name != "" {
					names = append(names, name)
				}
			}
		}
		return strings.Join(names, ", ")
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		return ""
	}
	switch field {
	case "assignee_id":
		return s.resolver.ResolveUserName(id)
	case "priority_id":
		return s.resolver.ResolvePriorityName(id)
	case "status_id":
		return s.resolver.ResolveStatusName(id)
	case "parent_id":
		return s.resolver.ResolveItemKey(id)
	case "project_id":
		if allowedProjects != nil {
			if _, allowed := allowedProjects[id]; !allowed {
				return ""
			}
		}
		return s.resolver.ResolveProjectName(id)
	case "milestone_id":
		return s.resolver.ResolveMilestoneName(id)
	case "item_type_id":
		return s.resolver.ResolveItemTypeName(id)
	default:
		return ""
	}
}

func (s *ItemApplicationService) enrich(ctx context.Context, userID int, items []models.Item) error {
	NewTimePermissionService(s.db, s.perm).MaskInaccessibleProjectNamesContext(ctx, userID, items)
	MaskInaccessibleRelatedWorkItems(userID, items, s.perm)
	s.decorateItemKeys(userID, items)
	if err := repository.NewLabelRepository(s.db).LoadForItemsContext(ctx, items); err != nil {
		return err
	}
	if err := LoadPersonalLabelsForItems(ctx, s.db, items, userID); err != nil {
		return err
	}
	return repository.NewMilestoneAttachRepository(s.db).LoadForItemsContext(ctx, items)
}

func (s *ItemApplicationService) decorateItemKeys(userID int, items []models.Item) {
	workspaceKeys := make(map[int]string)
	workspaces := repository.NewWorkspaceRepository(s.db)
	for i := range items {
		item := &items[i]
		if item.WorkspaceKey != "" && item.WorkspaceItemNumber > 0 {
			item.Key = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
		}
		if item.ParentID == nil || item.ParentWorkspaceItemNumber == nil {
			continue
		}
		parentWorkspaceID, err := s.items.GetWorkspaceID(*item.ParentID)
		if err != nil {
			item.ParentTitle = ""
			item.ParentWorkspaceItemNumber = nil
			continue
		}
		parentWorkspaceKey := item.WorkspaceKey
		if parentWorkspaceID != item.WorkspaceID {
			allowed, permissionErr := s.perm.HasWorkspacePermission(userID, parentWorkspaceID, models.PermissionItemView)
			if permissionErr != nil || !allowed {
				item.ParentTitle = ""
				item.ParentWorkspaceItemNumber = nil
				continue
			}
			parentWorkspaceKey = workspaceKeys[parentWorkspaceID]
			if parentWorkspaceKey == "" {
				parentWorkspaceKey, err = workspaces.GetKey(parentWorkspaceID)
				if err != nil {
					item.ParentTitle = ""
					item.ParentWorkspaceItemNumber = nil
					continue
				}
				workspaceKeys[parentWorkspaceID] = parentWorkspaceKey
			}
		}
		item.ParentKey = fmt.Sprintf("%s-%d", parentWorkspaceKey, *item.ParentWorkspaceItemNumber)
	}
}
