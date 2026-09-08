package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// MaxBatchLinkItems bounds one /links/batch anchor list. Restored to the
// item-batch cap (500) that the frontend chunks against (200 per request).
const MaxBatchLinkItems = 500

type BatchItemLinks struct {
	ItemID          int               `json:"item_id"`
	Outgoing        []models.ItemLink `json:"outgoing"`
	Incoming        []models.ItemLink `json:"incoming"`
	HasMoreLinks    bool              `json:"has_more_links"`
	NextAfterLinkID int               `json:"next_after_link_id,omitempty"`
}

type BatchLinkParams struct {
	QLQuery             string
	ItemIDs             []int
	Page                PaginationParams
	SortBy              string
	SortAsc             bool
	AfterID             int
	IncludeCustomFields bool
}

func (s *ItemLinkService) ListBatch(ctx context.Context, userID int, params BatchLinkParams) ([]BatchItemLinks, int, error) {
	if (strings.TrimSpace(params.QLQuery) == "") == (len(params.ItemIDs) == 0) {
		return nil, 0, fmt.Errorf("exactly one of ql or ids is required")
	}
	ids := dedupInts(params.ItemIDs)
	total := len(ids)
	if params.QLQuery != "" {
		if s.perm == nil {
			return []BatchItemLinks{}, 0, nil
		}
		workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(userID)
		if err != nil {
			return nil, 0, err
		}
		page, err := NewItemCRUDService(s.db).ListIDsWithQLPageContext(ctx, ListWithQLParams{
			QLQuery: params.QLQuery, WorkspaceIDs: workspaceIDs, UserID: userID,
			Pagination: params.Page, SortBy: params.SortBy, SortAsc: params.SortAsc,
		})
		if err != nil {
			return nil, 0, err
		}
		ids, total = page.IDs, page.Total
	}
	if len(ids) == 0 || len(ids) > MaxBatchLinkItems {
		return nil, 0, fmt.Errorf("ids must contain between 1 and %d unique values", MaxBatchLinkItems)
	}
	if params.AfterID < 0 || (params.AfterID > 0 && (params.QLQuery != "" || len(ids) != 1)) {
		return nil, 0, fmt.Errorf("after_id requires exactly one explicit item id")
	}
	groups, err := s.ListOneHopItemLinksPageWithChecks(ctx, userID, ids, params.AfterID, MaxOneHopLinksPerItem, params.IncludeCustomFields)
	if err != nil {
		return nil, 0, err
	}
	result := make([]BatchItemLinks, 0, len(ids))
	for _, id := range ids {
		group := groups[id]
		result = append(result, BatchItemLinks{
			ItemID: id, Outgoing: group.Outgoing, Incoming: group.Incoming,
			HasMoreLinks: group.HasMore, NextAfterLinkID: group.NextAfterID,
		})
	}
	return result, total, nil
}

func (s *ItemLinkService) CreateManagedLink(userID int, params CreateItemLinkParams) (*models.ItemLink, error) {
	multi := true
	if params.CustomFieldID != nil {
		var err error
		params, multi, err = s.prepareFieldLink(params)
		if err != nil {
			return nil, err
		}
	}
	if multi {
		return s.CreateLinkWithChecks(userID, params)
	}
	return s.ReplaceSingleValueFieldLinkWithChecks(userID, params)
}

func (s *ItemLinkService) prepareFieldLink(params CreateItemLinkParams) (CreateItemLinkParams, bool, error) {
	fieldID := *params.CustomFieldID
	var optionsJSON sql.NullString
	var fieldType string
	if err := s.db.QueryRow("SELECT field_type, options FROM custom_field_definitions WHERE id = ?", fieldID).Scan(&fieldType, &optionsJSON); err != nil {
		return params, false, fmt.Errorf("custom field not found")
	}
	if fieldType != "linking" || !optionsJSON.Valid {
		return params, false, fmt.Errorf("custom field is not configured for linking")
	}
	var options struct {
		LinkTypeID         int      `json:"link_type_id"`
		AllowedItemTypeIDs []int    `json:"allowed_item_type_ids"`
		AllowedEntityTypes []string `json:"allowed_entity_types"`
		Multi              bool     `json:"multi"`
		MirrorOfFieldID    int      `json:"mirror_of_field_id"`
	}
	if err := json.Unmarshal([]byte(optionsJSON.String), &options); err != nil {
		return params, false, fmt.Errorf("invalid field options")
	}
	if options.MirrorOfFieldID > 0 {
		params.SourceType, params.TargetType = params.TargetType, params.SourceType
		params.SourceID, params.TargetID = params.TargetID, params.SourceID
		params.CustomFieldID = &options.MirrorOfFieldID
		if err := s.db.QueryRow("SELECT options FROM custom_field_definitions WHERE id = ?", options.MirrorOfFieldID).Scan(&optionsJSON); err != nil {
			return params, false, fmt.Errorf("primary field not found")
		}
		if !optionsJSON.Valid || json.Unmarshal([]byte(optionsJSON.String), &options) != nil {
			return params, false, fmt.Errorf("invalid primary field options")
		}
	}
	if params.LinkTypeID != 0 && params.LinkTypeID != options.LinkTypeID {
		return params, false, fmt.Errorf("link type does not match field configuration")
	}
	params.LinkTypeID = options.LinkTypeID
	if len(options.AllowedEntityTypes) > 0 && !slices.Contains(options.AllowedEntityTypes, params.TargetType) {
		return params, false, fmt.Errorf("target entity type not allowed for this field")
	}
	if len(options.AllowedItemTypeIDs) > 0 && params.TargetType == "item" {
		target, err := repository.NewItemRepository(s.db).FindByID(params.TargetID)
		if err != nil || target.ItemTypeID == nil || !slices.Contains(options.AllowedItemTypeIDs, *target.ItemTypeID) {
			return params, false, fmt.Errorf("target item type not allowed for this field")
		}
	}
	return params, options.Multi, nil
}

func (s *ItemLinkService) SearchLinkable(userID int, query, entityType string, limit int, itemTypeIDs []int) ([]models.LinkableItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if entityType != "" && entityType != "item" && entityType != "test_case" && entityType != "asset" {
		return nil, ErrLinkInvalidEntityType
	}
	if s.perm == nil {
		return []models.LinkableItem{}, nil
	}
	workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	result := []models.LinkableItem{}
	if entityType == "" || entityType == "item" {
		items, err := repository.NewItemRepository(s.db).SearchLinkableItems(query, workspaceIDs, itemTypeIDs, limit)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	if entityType == "" || entityType == "test_case" {
		repo := repository.NewTestCaseRepository(s.db)
		candidates, err := repo.FindWorkspacesWithMatchingCases(query)
		if err != nil {
			return nil, err
		}
		visible := make([]int, 0, len(candidates))
		for _, workspaceID := range candidates {
			allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionTestView)
			if err == nil && allowed {
				visible = append(visible, workspaceID)
			}
		}
		if len(visible) > 0 {
			items, err := repo.Search(query, visible, limit)
			if err != nil {
				return nil, err
			}
			result = append(result, items...)
		}
	}
	if entityType == "" || entityType == "asset" {
		sets := s.AccessibleAssetSetIDs(userID)
		setIDs := make([]int, 0, len(sets))
		for id := range sets {
			setIDs = append(setIDs, id)
		}
		if len(setIDs) > 0 {
			items, err := repository.NewAssetRepository(s.db).Search(query, setIDs, limit)
			if err != nil {
				return nil, err
			}
			result = append(result, items...)
		}
	}
	return result, nil
}

func (s *ItemLinkService) ListFieldLinks(userID, itemID, fieldID int) ([]models.ItemLink, error) {
	if s.perm == nil {
		return nil, &EntityNotAccessibleError{EntityType: "item", EntityID: itemID}
	}
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.perm.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		return nil, &EntityNotAccessibleError{EntityType: "item", EntityID: itemID}
	}
	var optionsJSON sql.NullString
	var fieldType string
	if err := s.db.QueryRow("SELECT field_type, options FROM custom_field_definitions WHERE id = ?", fieldID).Scan(&fieldType, &optionsJSON); err != nil {
		return nil, err
	}
	if fieldType != "linking" {
		return nil, fmt.Errorf("field is not a linking type")
	}
	var options struct {
		MirrorOfFieldID int `json:"mirror_of_field_id"`
	}
	if optionsJSON.Valid {
		_ = json.Unmarshal([]byte(optionsJSON.String), &options)
	}
	primaryFieldID, mirror := fieldID, false
	if options.MirrorOfFieldID > 0 {
		primaryFieldID, mirror = options.MirrorOfFieldID, true
	}
	links, err := s.ListLinksByCustomField(primaryFieldID, itemID, mirror)
	if err != nil {
		return nil, err
	}
	return s.FilterLinksForUser(userID, links), nil
}

func IsQLLinkError(err error) bool { return errors.Is(err, ErrQLQuery) }
