package services

import (
	"context"
	"fmt"
	"strings"

	"windshift/internal/models"
)

// MaxOneHopLinksPerItem bounds each anchor's direct-link page. A future
// traversal layer can call this primitive once per breadth-first frontier.
// Kept at the batch anchor cap so dependency badges are not silently truncated
// for realistic collections.
const MaxOneHopLinksPerItem = 500

// OneHopItemLinksPage contains one anchor item's direct visible links.
type OneHopItemLinksPage struct {
	Outgoing    []models.ItemLink
	Incoming    []models.ItemLink
	HasMore     bool
	NextAfterID int
}

type itemLinkCandidate struct {
	anchorID int
	linkID   int
	outgoing bool
}

// ListOneHopItemLinksPageWithChecks loads one direct-link page for every
// anchor in a fixed number of queries. Links to items outside the caller's
// accessible workspaces are excluded before per-anchor ranking.
func (s *ItemLinkService) ListOneHopItemLinksPageWithChecks(
	ctx context.Context,
	userID int,
	itemIDs []int,
	afterID int,
	limit int,
	includeCustomFields bool,
) (map[int]OneHopItemLinksPage, error) {
	ids := dedupInts(itemIDs)
	result := make(map[int]OneHopItemLinksPage, len(ids))
	for _, itemID := range ids {
		result[itemID] = OneHopItemLinksPage{
			Outgoing: []models.ItemLink{},
			Incoming: []models.ItemLink{},
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	if afterID < 0 {
		return nil, fmt.Errorf("after link id must be non-negative")
	}
	if limit <= 0 || limit > MaxOneHopLinksPerItem {
		limit = MaxOneHopLinksPerItem
	}
	if s.perm == nil {
		return result, nil
	}
	workspaceIDs, err := s.perm.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, fmt.Errorf("load accessible workspaces for link page: %w", err)
	}
	if len(workspaceIDs) == 0 {
		return result, nil
	}

	candidates, err := s.listOneHopItemLinkCandidates(ctx, ids, workspaceIDs, afterID, limit+1, includeCustomFields)
	if err != nil {
		return nil, err
	}

	returned := make([]itemLinkCandidate, 0, len(candidates))
	linkIDSet := make(map[int]struct{}, len(candidates))
	perAnchorCount := make(map[int]int, len(ids))
	for _, candidate := range candidates {
		count := perAnchorCount[candidate.anchorID]
		if count >= limit {
			group := result[candidate.anchorID]
			group.HasMore = true
			result[candidate.anchorID] = group
			continue
		}
		perAnchorCount[candidate.anchorID] = count + 1
		returned = append(returned, candidate)
		linkIDSet[candidate.linkID] = struct{}{}
		group := result[candidate.anchorID]
		group.NextAfterID = candidate.linkID
		result[candidate.anchorID] = group
	}
	if len(returned) == 0 {
		return result, nil
	}

	linkIDs := make([]int, 0, len(linkIDSet))
	for linkID := range linkIDSet {
		linkIDs = append(linkIDs, linkID)
	}
	where := "il.id IN (" + placeholders(len(linkIDs)) + ")"
	links, err := getLinksWhereContext(ctx, s.db, where, toIfaceSlice(linkIDs)...)
	if err != nil {
		return nil, fmt.Errorf("hydrate one-hop item links: %w", err)
	}
	linksByID := make(map[int]models.ItemLink, len(links))
	for _, link := range links {
		linksByID[link.ID] = link
	}

	for _, candidate := range returned {
		link, ok := linksByID[candidate.linkID]
		if !ok {
			continue
		}
		group := result[candidate.anchorID]
		if candidate.outgoing {
			group.Outgoing = append(group.Outgoing, link)
		} else {
			group.Incoming = append(group.Incoming, link)
		}
		result[candidate.anchorID] = group
	}
	return result, nil
}

func (s *ItemLinkService) listOneHopItemLinkCandidates(
	ctx context.Context,
	itemIDs, workspaceIDs []int,
	afterID, fetchLimit int,
	includeCustomFields bool,
) ([]itemLinkCandidate, error) {
	itemPH := placeholders(len(itemIDs))
	workspacePH := placeholders(len(workspaceIDs))
	customFieldFilter := " AND il.custom_field_id IS NULL"
	if includeCustomFields {
		customFieldFilter = ""
	}

	query := `
		WITH candidates AS (
			SELECT il.source_id AS anchor_id, il.id AS link_id, 1 AS outgoing
			FROM item_links il
			JOIN items source_item ON source_item.id = il.source_id
			LEFT JOIN items target_item ON target_item.id = il.target_id AND il.target_type = 'item'
			WHERE il.source_type = 'item'
			  AND il.source_id IN (` + itemPH + `)
			  AND source_item.workspace_id IN (` + workspacePH + `)
			  AND (il.target_type <> 'item' OR target_item.workspace_id IN (` + workspacePH + `))
			  AND il.id > ?` + customFieldFilter + `

			UNION ALL

			SELECT il.target_id AS anchor_id, il.id AS link_id, 0 AS outgoing
			FROM item_links il
			JOIN items target_item ON target_item.id = il.target_id
			LEFT JOIN items source_item ON source_item.id = il.source_id AND il.source_type = 'item'
			WHERE il.target_type = 'item'
			  AND il.target_id IN (` + itemPH + `)
			  AND target_item.workspace_id IN (` + workspacePH + `)
			  AND (il.source_type <> 'item' OR source_item.workspace_id IN (` + workspacePH + `))
			  AND il.id > ?` + customFieldFilter + `
		), ranked AS (
			SELECT anchor_id, link_id, outgoing,
			       ROW_NUMBER() OVER (PARTITION BY anchor_id ORDER BY link_id ASC) AS row_number
			FROM candidates
		)
		SELECT anchor_id, link_id, outgoing
		FROM ranked
		WHERE row_number <= ?
		ORDER BY anchor_id ASC, row_number ASC`

	args := make([]any, 0, len(itemIDs)*2+len(workspaceIDs)*4+3)
	args = append(args, toIfaceSlice(itemIDs)...)
	args = append(args, toIfaceSlice(workspaceIDs)...)
	args = append(args, toIfaceSlice(workspaceIDs)...)
	args = append(args, afterID)
	args = append(args, toIfaceSlice(itemIDs)...)
	args = append(args, toIfaceSlice(workspaceIDs)...)
	args = append(args, toIfaceSlice(workspaceIDs)...)
	args = append(args, afterID, fetchLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query one-hop item link candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	candidates := []itemLinkCandidate{}
	for rows.Next() {
		var candidate itemLinkCandidate
		var outgoing int
		if err := rows.Scan(&candidate.anchorID, &candidate.linkID, &outgoing); err != nil {
			return nil, fmt.Errorf("scan one-hop item link candidate: %w", err)
		}
		candidate.outgoing = outgoing == 1
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate one-hop item link candidates: %w", err)
	}
	return candidates, nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
