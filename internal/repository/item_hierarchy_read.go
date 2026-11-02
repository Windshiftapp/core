package repository

import (
	"context"
	"fmt"
	"strings"

	"windshift/internal/models"
)

const defaultHierarchyPerRootLimit = 100

// HierarchyLevelCount is one row of the per-level count read.
type HierarchyLevelCount struct {
	Level int    `json:"level"`
	Name  string `json:"name"`
	Total int    `json:"total"`
}

// HierarchyReadParams describes an ancestor-scoped descendant read. Filters
// carry the already-resolved collection/QL predicate so it is evaluated once.
type HierarchyReadParams struct {
	RootIDs          []int
	WorkspaceIDs     []int
	Filters          ItemFilters
	MaxDepth         int // 0 = unlimited
	PerRootLimit     int // <=0 = default 100
	PerRootOffset    int // skip this many matching descendants per root
	OmitDescriptions bool
}

// HierarchyItem is a hydrated item plus its depth relative to the requested
// root (root itself is not returned, so the first level is depth 1).
type HierarchyItem struct {
	models.Item
	Depth int `json:"depth"`
}

// HierarchyRootGroup groups one root's matching descendants.
type HierarchyRootGroup struct {
	RootID  int             `json:"root_id"`
	Total   int             `json:"total"`
	HasMore bool            `json:"has_more"`
	Items   []HierarchyItem `json:"items"`
}

// HasChildrenByItemIDContext returns the subset of itemIDs that have at least
// one child in one of workspaceIDs. Merged duplicates are ignored. The
// workspace scope stops a visible parent from disclosing a child in a workspace
// the caller cannot view.
func (r *ItemRepository) HasChildrenByItemIDContext(ctx context.Context, itemIDs, workspaceIDs []int) (map[int]struct{}, error) {
	result := make(map[int]struct{})
	if len(itemIDs) == 0 || len(workspaceIDs) == 0 {
		return result, nil
	}
	// Chunk the parent ids so a full page cannot exceed the engine's host
	// parameter limit; the workspace scope is re-applied per chunk.
	const chunkSize = 500
	wsPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	for start := 0; start < len(itemIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		chunk := itemIDs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, 0, len(chunk)+len(workspaceIDs))
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range workspaceIDs {
			args = append(args, id)
		}
		rows, err := r.db.QueryContext(ctx,
			"SELECT DISTINCT parent_id FROM items WHERE parent_id IN ("+placeholders+
				") AND workspace_id IN ("+wsPlaceholders+") AND merged_into_item_id IS NULL",
			args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query has_children: %w", err)
		}
		for rows.Next() {
			var parentID int
			if err := rows.Scan(&parentID); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("failed to scan has_children: %w", err)
			}
			result[parentID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to iterate has_children: %w", err)
		}
		_ = rows.Close()
	}
	return result, nil
}

// CountByHierarchyLevelContext returns the filtered item count per item-type
// hierarchy level. Levels with no matches are omitted.
func (r *ItemRepository) CountByHierarchyLevelContext(ctx context.Context, params ItemListParams) ([]HierarchyLevelCount, error) {
	whereClause, args := r.buildWhereClause(params)
	query := `SELECT COALESCE(it.hierarchy_level, 0) AS level, COALESCE(hl.name, '') AS name, COUNT(*) AS total ` +
		ItemListFilterFromClause() +
		` LEFT JOIN hierarchy_levels hl ON hl.level = COALESCE(it.hierarchy_level, 0) ` +
		whereClause +
		` GROUP BY COALESCE(it.hierarchy_level, 0), hl.name ORDER BY level`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to count hierarchy levels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make([]HierarchyLevelCount, 0)
	for rows.Next() {
		var count HierarchyLevelCount
		if err := rows.Scan(&count.Level, &count.Name, &count.Total); err != nil {
			return nil, fmt.Errorf("failed to scan hierarchy level count: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate hierarchy level counts: %w", err)
	}
	return counts, nil
}

// HierarchyDescendantsContext returns the matching descendants of each root,
// grouped by root and capped per root. The recursive walk is not pruned by the
// QL so a matching descendant under a filtered-out intermediate is still found;
// the QL applies only to the selected descendants. Breadth-first ordering
// (depth, frac_index) means a truncated root keeps complete shallow levels.
func (r *ItemRepository) HierarchyDescendantsContext(ctx context.Context, params HierarchyReadParams) ([]HierarchyRootGroup, error) {
	if len(params.RootIDs) == 0 || len(params.WorkspaceIDs) == 0 {
		return []HierarchyRootGroup{}, nil
	}
	perRoot := params.PerRootLimit
	if perRoot <= 0 {
		perRoot = defaultHierarchyPerRootLimit
	}
	if perRoot > 1000 {
		perRoot = 1000
	}
	perRootOffset := params.PerRootOffset
	if perRootOffset < 0 {
		perRootOffset = 0
	}

	rootPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(params.RootIDs)), ",")
	wsPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(params.WorkspaceIDs)), ",")

	args := make([]any, 0, len(params.RootIDs)+2*len(params.WorkspaceIDs)+len(params.Filters.QLArgs)+1)
	for _, id := range params.RootIDs {
		args = append(args, id)
	}
	for _, id := range params.WorkspaceIDs {
		args = append(args, id)
	}
	depthGuard := ""
	if params.MaxDepth > 0 {
		depthGuard = " AND t.depth < ?"
		args = append(args, params.MaxDepth)
	}
	for _, id := range params.WorkspaceIDs {
		args = append(args, id)
	}
	ql := strings.TrimSpace(params.Filters.QLQuery)
	qlClause := ""
	if ql != "" {
		qlClause = " AND (" + ql + ")"
		args = append(args, params.Filters.QLArgs...)
	}
	args = append(args, perRootOffset, perRootOffset+perRoot)

	query := "WITH RECURSIVE tree(root_id, id, depth) AS (" +
		" SELECT i.id, i.id, 0 FROM items i WHERE i.id IN (" + rootPlaceholders +
		") AND i.workspace_id IN (" + wsPlaceholders + ") AND i.merged_into_item_id IS NULL" +
		" UNION ALL" +
		" SELECT t.root_id, c.id, t.depth + 1 FROM items c JOIN tree t ON c.parent_id = t.id" +
		" WHERE 1=1" + depthGuard + " AND c.merged_into_item_id IS NULL" +
		"), ranked AS (" +
		" SELECT t.root_id, t.id, t.depth," +
		" ROW_NUMBER() OVER (PARTITION BY t.root_id ORDER BY t.depth, i.frac_index) AS rn," +
		" COUNT(*) OVER (PARTITION BY t.root_id) AS root_total" +
		" FROM tree t JOIN items i ON i.id = t.id" + ItemListFilterJoins() +
		" WHERE t.depth > 0 AND i.workspace_id IN (" + wsPlaceholders + ")" + qlClause +
		"), page AS (" +
		" SELECT root_id, id, depth, root_total FROM ranked WHERE rn > ? AND rn <= ?" +
		") " +
		itemListSelectClause(params.OmitDescriptions) +
		", page.root_id AS hierarchy_root_id, page.depth AS hierarchy_depth, page.root_total AS hierarchy_root_total " +
		itemListFromClause() +
		" JOIN page ON page.id = i.id" +
		" ORDER BY page.root_id, page.depth, i.frac_index"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query hierarchy descendants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := make([]HierarchyRootGroup, 0, len(params.RootIDs))
	index := make(map[int]int, len(params.RootIDs))
	for _, id := range params.RootIDs {
		index[id] = len(groups)
		groups = append(groups, HierarchyRootGroup{RootID: id, Items: []HierarchyItem{}})
	}
	for rows.Next() {
		var rootID, depth, rootTotal int
		item, err := scanItemRow(rows, &rootID, &depth, &rootTotal)
		if err != nil {
			return nil, err
		}
		idx, ok := index[rootID]
		if !ok {
			continue
		}
		groups[idx].Total = rootTotal
		groups[idx].Items = append(groups[idx].Items, HierarchyItem{Item: item, Depth: depth})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate hierarchy descendants: %w", err)
	}
	for i := range groups {
		groups[i].HasMore = groups[i].Total > len(groups[i].Items)
	}
	return groups, nil
}
