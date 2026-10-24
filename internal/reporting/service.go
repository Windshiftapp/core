package reporting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ItemLister is the slice of ItemApplicationService the resolver needs.
type ItemLister interface {
	List(ctx context.Context, request services.ItemListRequest) (services.ItemListResult, error)
}

// AssetLister is the slice of AssetApplicationService the resolver needs.
type AssetLister interface {
	ListAssets(userID, setID int, filter repository.AssetListFilter, ql string) ([]models.Asset, int, error)
}

// LinkProjector projects items linked to assets. It is optional: without it,
// a slot that asks for a projection resolves without linked items rather than
// failing.
type LinkProjector interface {
	ProjectLinkedItems(ctx context.Context, assetIDs, accessibleWorkspaceIDs []int, fields []string) (map[int][]map[string]any, bool, error)
}

// Service resolves widget query slots into normalized rows.
type Service struct {
	items          ItemLister
	assets         AssetLister
	links          LinkProjector
	edgeLinks      LinkEdgeLister
	itemReferences ItemReferenceLister
}

func NewService(items ItemLister, assets AssetLister) *Service {
	return &Service{items: items, assets: assets}
}

// WithLinks enables item projections through asset links.
func (s *Service) WithLinks(links LinkProjector) *Service {
	s.links = links
	return s
}

// Request is one dashboard's worth of slot resolution.
type Request struct {
	UserID int
	// WorkspaceID narrows item queries to one workspace. Zero means every
	// workspace the viewer can read; the item service intersects with the
	// viewer's accessible workspaces either way.
	WorkspaceID int
	// AccessibleWorkspaceIDs bounds link projections. A linked item outside
	// this set is invisible even when the asset itself is readable.
	AccessibleWorkspaceIDs []int
	Slots                  []Slot
}

type groupKey struct {
	entity     Entity
	ql         string
	sort       string
	assetSetID int
	projection string
}

type slotResult struct {
	key    string
	result Result
}

// Resolve runs every slot and returns results keyed by slot key. Identical
// slots are fetched once and sliced per slot limit.
func (s *Service) Resolve(ctx context.Context, request Request) map[string]Result {
	results := make(map[string]Result, len(request.Slots))
	groups := make(map[groupKey][]Slot, len(request.Slots))
	order := make([]groupKey, 0, len(request.Slots))
	for _, slot := range request.Slots {
		if slot.Entity == EntityLinks {
			results[slot.Key] = s.resolveLinks(ctx, request, slot)
			continue
		}
		key := groupKey{
			entity:     slot.Entity,
			ql:         slot.QL,
			sort:       slot.Sort,
			assetSetID: slot.AssetSetID,
			projection: projectionKey(slot.Projection),
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], slot)
	}

	for _, key := range order {
		slots := groups[key]
		fetchLimit := 0
		for _, slot := range slots {
			if slot.Limit > fetchLimit {
				fetchLimit = slot.Limit
			}
		}

		var resolved []slotResult
		var err error
		switch key.entity {
		case EntityItem:
			resolved, err = s.resolveItems(ctx, request, slots, fetchLimit)
		case EntityAsset:
			resolved, err = s.resolveAssets(ctx, request, slots, fetchLimit)
		default:
			err = fmt.Errorf("unsupported entity %q", key.entity)
		}

		if err != nil {
			slog.Warn("custom widget query failed",
				"component", "reporting",
				"entity", key.entity,
				"error", err,
			)
			failure := Result{Status: "error", Error: sourceError(err)}
			for _, slot := range slots {
				results[slot.Key] = failure
			}
			continue
		}
		for _, entry := range resolved {
			results[entry.key] = entry.result
		}
	}
	return results
}

func (s *Service) resolveItems(ctx context.Context, request Request, slots []Slot, fetchLimit int) ([]slotResult, error) {
	slotCtx, cancel := context.WithTimeout(ctx, SlotTimeout)
	defer cancel()

	sortField, ascending := SplitSort(slots[0].Sort)
	listRequest := services.ItemListRequest{
		UserID:           request.UserID,
		QL:               slots[0].QL,
		Pagination:       services.PaginationParams{Limit: fetchLimit},
		SortBy:           sortField,
		SortAsc:          ascending,
		OmitDescriptions: true,
	}
	if request.WorkspaceID > 0 {
		listRequest.WorkspaceIDs = []int{request.WorkspaceID}
	}

	result, err := s.items.List(slotCtx, listRequest)
	if err != nil {
		return nil, err
	}
	rows := make([]ItemRow, 0, len(result.Items))
	for i := range result.Items {
		rows = append(rows, itemRow(&result.Items[i]))
	}
	return buildResults(slots, rows, result.Total), nil
}

func (s *Service) resolveAssets(ctx context.Context, request Request, slots []Slot, fetchLimit int) ([]slotResult, error) {
	// The asset list service has no context-taking variant, so this path relies
	// on the request context rather than a per-slot deadline.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	assets, total, err := s.assets.ListAssets(
		request.UserID,
		slots[0].AssetSetID,
		repository.AssetListFilter{Limit: fetchLimit},
		slots[0].QL,
	)
	if err != nil {
		return nil, err
	}
	rows := make([]AssetRow, 0, len(assets))
	for i := range assets {
		rows = append(rows, assetRow(&assets[i]))
	}

	projected := false
	if projection := slots[0].Projection; projection != nil && projection.Entity == "item" && s.links != nil {
		projectCtx, cancel := context.WithTimeout(ctx, SlotTimeout)
		defer cancel()
		links, truncated, err := s.projectLinkedItems(projectCtx, rows, request.AccessibleWorkspaceIDs, projection.Fields)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].LinkedItems = links[rows[i].ID]
		}
		projected = truncated
	}

	results := buildResults(slots, rows, total)
	if projected {
		for i := range results {
			results[i].result.Truncated = true
		}
	}
	return results, nil
}

// projectLinkedItems attaches the requested item fields to each asset row. An
// asset with no visible links simply gets none.
func (s *Service) projectLinkedItems(ctx context.Context, rows []AssetRow, accessibleWorkspaceIDs []int, fields []string) (links map[int][]map[string]any, truncated bool, err error) {
	assetIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		assetIDs = append(assetIDs, row.ID)
	}
	links, truncated, err = s.links.ProjectLinkedItems(ctx, assetIDs, accessibleWorkspaceIDs, fields)
	if err != nil {
		return nil, false, err
	}
	return links, truncated, nil
}

// buildResults slices one fetch down to each slot's limit. Truncation is
// reported against the slot limit, not the fetch limit, because the component
// only ever sees its own slice.
func buildResults[T any](slots []Slot, rows []T, total int) []slotResult {
	out := make([]slotResult, 0, len(slots))
	for _, slot := range slots {
		limit := slot.Limit
		if limit > len(rows) {
			limit = len(rows)
		}
		out = append(out, slotResult{
			key: slot.Key,
			result: Result{
				Status:    "ready",
				Rows:      rows[:limit],
				Total:     total,
				Truncated: total > limit,
			},
		})
	}
	return out
}

func itemRow(item *models.Item) ItemRow {
	row := ItemRow{
		ID:          item.ID,
		Key:         item.Key,
		Title:       item.Title,
		Status:      item.StatusName,
		Type:        item.ItemTypeName,
		Priority:    item.PriorityName,
		Assignee:    item.AssigneeName,
		Workspace:   item.WorkspaceName,
		WorkspaceID: item.WorkspaceID,
		CreatedAt:   &item.CreatedAt,
		UpdatedAt:   &item.UpdatedAt,
		DueDate:     item.DueDate,
		Completed:   item.CompletedAt,
	}
	if item.StoryPoints != nil {
		points := *item.StoryPoints
		row.StoryPoint = &points
	}
	return row
}

func assetRow(asset *models.Asset) AssetRow {
	return AssetRow{
		ID:        asset.ID,
		Key:       asset.AssetTag,
		Title:     asset.Title,
		Status:    asset.StatusName,
		Type:      asset.AssetTypeName,
		Category:  asset.CategoryName,
		Set:       asset.SetName,
		CreatedAt: &asset.CreatedAt,
		UpdatedAt: &asset.UpdatedAt,
	}
}

func projectionKey(projection *Projection) string {
	if projection == nil {
		return ""
	}
	return projection.Entity + "|" + projection.Mode + "|" + fmt.Sprint(projection.Fields)
}

// sourceError maps an internal failure to a client-facing error. The real
// error is logged, never returned, so query internals stay private.
func sourceError(err error) *SourceError {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &SourceError{Code: "timeout", Message: "This widget's query took too long."}
	}
	return &SourceError{Code: "unavailable", Message: "This widget's query could not be completed."}
}
