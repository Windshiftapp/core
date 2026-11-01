package reporting

import (
	"context"
	"fmt"

	"windshift/internal/repository"
	"windshift/internal/services"
)

const (
	// MaxNodesPerSide bounds each seed set of an edge slot.
	MaxNodesPerSide = 1000
	// MaxEdges bounds the resolved edge set.
	MaxEdges = 10000
)

// GraphNode is one endpoint of the bipartite graph.
type GraphNode struct {
	ID       string `json:"id"`
	EntityID int    `json:"entity_id"`
	Type     Entity `json:"type"`
	Title    string `json:"title"`
	Key      string `json:"key,omitempty"`
	Status   string `json:"status,omitempty"`
	Group    string `json:"group,omitempty"`
}

// GraphEdge connects an item node to an asset node.
type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
}

// GraphCounts reports what the edge slot produced.
type GraphCounts struct {
	Items  int `json:"items"`
	Assets int `json:"assets"`
	Edges  int `json:"edges"`
}

// GraphResult is the payload of a links slot.
type GraphResult struct {
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
	Counts GraphCounts `json:"counts"`
}

// LinkEdgeLister reads item-to-asset links between two already-authorized sets.
type LinkEdgeLister interface {
	ListAssetItemLinks(ctx context.Context, itemIDs, assetIDs []int, maxEdges int) ([]repository.LinkEdge, bool, error)
}

// ItemReferenceLister reads asset references held in item custom fields.
type ItemReferenceLister interface {
	ListItemAssetReferences(ctx context.Context, itemIDs []int) (map[int][]int, error)
}

// WithEdges enables the bipartite edge slot.
func (s *Service) WithEdges(links LinkEdgeLister, references ItemReferenceLister) *Service {
	s.edgeLinks = links
	s.itemReferences = references
	return s
}

// resolveLinks builds the bipartite graph for one links slot. Both seed sets
// are resolved through the normal slot paths, so each is already filtered by
// the viewer's permissions; an edge can only join two visible endpoints.
func (s *Service) resolveLinks(ctx context.Context, request Request, slot Slot) Result {
	if s.edgeLinks == nil {
		return Result{Status: "error", Error: &SourceError{Code: "unavailable", Message: "Graph queries are not available."}}
	}
	if slot.Links == nil {
		return Result{Status: "error", Error: &SourceError{Code: "invalid", Message: "Graph slot is missing its seed queries."}}
	}

	items, err := s.edgeSeedItems(ctx, request, slot.Links.Item)
	if err != nil {
		return Result{Status: "error", Error: sourceError(err)}
	}
	assets, err := s.edgeSeedAssets(ctx, request, slot.Links.Asset)
	if err != nil {
		return Result{Status: "error", Error: sourceError(err)}
	}

	itemIDs := make([]int, 0, len(items))
	assetIDs := make([]int, 0, len(assets))
	for i := range items {
		itemIDs = append(itemIDs, items[i].ID)
	}
	for i := range assets {
		assetIDs = append(assetIDs, assets[i].ID)
	}

	edges, truncated, err := s.edgeLinks.ListAssetItemLinks(ctx, itemIDs, assetIDs, slot.Links.MaxEdges)
	if err != nil {
		return Result{Status: "error", Error: sourceError(err)}
	}

	assetSet := make(map[int]bool, len(assetIDs))
	for _, id := range assetIDs {
		assetSet[id] = true
	}
	if s.itemReferences != nil {
		references, err := s.itemReferences.ListItemAssetReferences(ctx, itemIDs)
		if err != nil {
			return Result{Status: "error", Error: sourceError(err)}
		}
		for itemID, referenced := range references {
			for _, assetID := range referenced {
				if !assetSet[assetID] {
					continue
				}
				edges = append(edges, repository.LinkEdge{
					ItemID:  itemID,
					AssetID: assetID,
					Kind:    "field_reference",
					Label:   "Reference",
				})
			}
		}
	}

	graph := GraphResult{
		Nodes:  make([]GraphNode, 0, len(items)+len(assets)),
		Edges:  make([]GraphEdge, 0, len(edges)),
		Counts: GraphCounts{Items: len(items), Assets: len(assets)},
	}
	for i := range items {
		item := &items[i]
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:       itemNodeID(item.ID),
			EntityID: item.ID,
			Type:     EntityItem,
			Title:    item.Title,
			Key:      item.Key,
			Status:   item.StatusName,
			Group:    item.WorkspaceName,
		})
	}
	for i := range assets {
		asset := &assets[i]
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:       assetNodeID(asset.ID),
			EntityID: asset.ID,
			Type:     EntityAsset,
			Title:    asset.Title,
			Key:      asset.AssetTag,
			Status:   asset.StatusName,
			Group:    asset.SetName,
		})
	}

	seen := make(map[string]bool, len(edges))
	for _, edge := range edges {
		if len(graph.Edges) >= slot.Links.MaxEdges {
			truncated = true
			break
		}
		source, target := itemNodeID(edge.ItemID), assetNodeID(edge.AssetID)
		dedup := source + "|" + target + "|" + edge.Kind
		if seen[dedup] {
			continue
		}
		seen[dedup] = true
		graph.Edges = append(graph.Edges, GraphEdge{
			ID:     fmt.Sprintf("e%d", len(graph.Edges)+1),
			Source: source,
			Target: target,
			Label:  edge.Label,
			Kind:   edge.Kind,
		})
	}
	graph.Counts.Edges = len(graph.Edges)

	return Result{Status: "ready", Graph: &graph, Truncated: truncated}
}

func (s *Service) edgeSeedItems(ctx context.Context, request Request, part *QueryPart) ([]itemSeed, error) {
	if part == nil {
		return nil, nil
	}
	seedCtx, cancel := context.WithTimeout(ctx, SlotTimeout)
	defer cancel()

	sortField, ascending := SplitSort(part.Sort)
	listRequest := services.ItemListRequest{
		UserID:           request.UserID,
		QL:               part.QL,
		Pagination:       services.PaginationParams{Limit: min(part.Limit, MaxNodesPerSide)},
		SortBy:           sortField,
		SortAsc:          ascending,
		OmitDescriptions: true,
	}
	if request.WorkspaceID > 0 {
		listRequest.WorkspaceIDs = []int{request.WorkspaceID}
	}

	result, err := s.items.List(seedCtx, listRequest)
	if err != nil {
		return nil, err
	}
	seeds := make([]itemSeed, 0, len(result.Items))
	for i := range result.Items {
		item := &result.Items[i]
		seeds = append(seeds, itemSeed{
			ID:            item.ID,
			Key:           item.Key,
			Title:         item.Title,
			StatusName:    item.StatusName,
			WorkspaceName: item.WorkspaceName,
		})
	}
	return seeds, nil
}

func (s *Service) edgeSeedAssets(ctx context.Context, request Request, part *QueryPart) ([]assetSeed, error) {
	if part == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	assets, _, err := s.assets.ListAssets(
		request.UserID,
		part.AssetSetID,
		repository.AssetListFilter{Limit: min(part.Limit, MaxNodesPerSide)},
		part.QL,
	)
	if err != nil {
		return nil, err
	}
	seeds := make([]assetSeed, 0, len(assets))
	for i := range assets {
		asset := &assets[i]
		seeds = append(seeds, assetSeed{
			ID:         asset.ID,
			AssetTag:   asset.AssetTag,
			Title:      asset.Title,
			StatusName: asset.StatusName,
			SetName:    asset.SetName,
		})
	}
	return seeds, nil
}

// itemSeed and assetSeed are the minimal node fields the graph needs, kept
// local so the graph path never carries descriptions or custom fields.
type itemSeed struct {
	ID            int
	Key           string
	Title         string
	StatusName    string
	WorkspaceName string
}

type assetSeed struct {
	ID         int
	AssetTag   string
	Title      string
	StatusName string
	SetName    string
}

func itemNodeID(id int) string  { return fmt.Sprintf("item-%d", id) }
func assetNodeID(id int) string { return fmt.Sprintf("asset-%d", id) }
