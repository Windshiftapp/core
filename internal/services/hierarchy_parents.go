package services

import (
	"context"
	"fmt"

	"windshift/internal/models"
)

// ParentRef identifies one child's parent link for batch, permission-aware
// resolution. The caller supplies the child's workspace and key plus the raw
// parent number/title already loaded with the child; a same-workspace parent
// then needs no permission lookup because the caller was already authorized on
// the child, possibly through an approval pool rather than membership.
type ParentRef struct {
	ChildID                   int
	ParentID                  int
	ChildWorkspaceID          int
	ChildWorkspaceKey         string
	ParentWorkspaceItemNumber int
	ParentTitle               string
}

// ParentSummary is the renderable parent context: the parent's display key,
// workspace-scoped number, and title. It is only produced for a parent the
// caller is allowed to view.
type ParentSummary struct {
	Key        string
	ItemNumber int
	Title      string
}

// ResolveParentSummaries returns a viewable parent summary per child ID. A
// child whose parent is missing or lives in a workspace the caller cannot view
// is omitted, so the caller clears that child's parent fields. Cross-workspace
// parents are gated on item.view, mirroring validateParentHierarchy; a
// same-workspace parent is always allowed.
//
// This is the single implementation of permission-aware parent disclosure, so
// item reads and AI features cannot drift apart on which parents they reveal.
func (h *HierarchyService) ResolveParentSummaries(ctx context.Context, userID int, refs []ParentRef) (map[int]ParentSummary, error) {
	if len(refs) == 0 {
		return map[int]ParentSummary{}, nil
	}
	if h.perm == nil {
		// Fail closed rather than disclose a parent no permission check covers.
		return map[int]ParentSummary{}, nil
	}

	parentIDs := make([]int, 0, len(refs))
	seen := make(map[int]struct{}, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref.ParentID]; ok {
			continue
		}
		seen[ref.ParentID] = struct{}{}
		parentIDs = append(parentIDs, ref.ParentID)
	}

	parentWorkspaces, err := h.repo.GetWorkspaceIDsContext(ctx, parentIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve parent workspaces: %w", err)
	}

	// viewable caches the cross-workspace permission decision per workspace so a
	// milestone with many items under one parent checks each workspace once.
	viewable := make(map[int]bool)
	// keys caches the parent workspace key for cross-workspace parents.
	keys := make(map[int]string)
	out := make(map[int]ParentSummary, len(refs))
	for _, ref := range refs {
		parentWorkspaceID, ok := parentWorkspaces[ref.ParentID]
		if !ok {
			continue
		}

		crossWorkspace := parentWorkspaceID != ref.ChildWorkspaceID
		if crossWorkspace {
			allowed, ok := viewable[parentWorkspaceID]
			if !ok {
				allowed, err = h.perm.HasWorkspacePermission(userID, parentWorkspaceID, models.PermissionItemView)
				if err != nil {
					return nil, fmt.Errorf("check parent workspace permission: %w", err)
				}
				viewable[parentWorkspaceID] = allowed
			}
			if !allowed {
				continue
			}
		}

		parentWorkspaceKey := ref.ChildWorkspaceKey
		if crossWorkspace {
			parentWorkspaceKey = keys[parentWorkspaceID]
			if parentWorkspaceKey == "" {
				parentWorkspaceKey, err = h.workspaces.GetKey(parentWorkspaceID)
				if err != nil {
					continue
				}
				keys[parentWorkspaceID] = parentWorkspaceKey
			}
		}

		out[ref.ChildID] = ParentSummary{
			Key:        fmt.Sprintf("%s-%d", parentWorkspaceKey, ref.ParentWorkspaceItemNumber),
			ItemNumber: ref.ParentWorkspaceItemNumber,
			Title:      ref.ParentTitle,
		}
	}
	return out, nil
}

// ResolveParentSummary resolves a single child's parent link. It is a
// convenience wrapper for detail reads that carry exactly one item.
func (h *HierarchyService) ResolveParentSummary(ctx context.Context, userID int, ref ParentRef) (ParentSummary, bool, error) {
	summaries, err := h.ResolveParentSummaries(ctx, userID, []ParentRef{ref})
	if err != nil {
		return ParentSummary{}, false, err
	}
	summary, ok := summaries[ref.ChildID]
	return summary, ok, nil
}
