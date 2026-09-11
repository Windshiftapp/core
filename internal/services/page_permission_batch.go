package services

import (
	"database/sql"
	"fmt"
	"strings"

	"windshift/internal/models"
)

type pageVisibilityAccess struct {
	isSystemAdmin     bool
	hasWorkspaceAdmin bool
	hasPageAdmin      bool
	hasPageView       bool
}

func (a pageVisibilityAccess) hasLivePageAdmin() bool {
	return a.isSystemAdmin || a.hasWorkspaceAdmin || a.hasPageAdmin
}

func (s *PagePermissionService) loadPageVisibilityAccess(userID, workspaceID int) (pageVisibilityAccess, error) {
	var access pageVisibilityAccess
	var err error
	access.isSystemAdmin, err = s.perm.IsSystemAdmin(userID)
	if err != nil {
		return pageVisibilityAccess{}, err
	}
	access.hasWorkspaceAdmin, err = s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return pageVisibilityAccess{}, err
	}
	access.hasPageAdmin, err = s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionPageAdmin)
	if err != nil {
		return pageVisibilityAccess{}, err
	}
	access.hasPageView, err = s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionPageView)
	if err != nil {
		return pageVisibilityAccess{}, err
	}
	return access, nil
}

func filterLivePagesForACL(pages []models.Page, workspaceID int, access pageVisibilityAccess, visible map[int]bool) []models.Page {
	livePages := make([]models.Page, 0, len(pages))
	for _, page := range pages {
		if page.WorkspaceID != workspaceID {
			continue
		}
		if page.ArchivedAt != nil {
			visible[page.ID] = access.isSystemAdmin || access.hasWorkspaceAdmin
			continue
		}
		if access.hasLivePageAdmin() {
			visible[page.ID] = true
			continue
		}
		livePages = append(livePages, page)
	}
	return livePages
}

func pageVisibilityCandidateIDs(pages []models.Page) []int {
	candidates := make(map[int]struct{}, len(pages)*2)
	for _, page := range pages {
		candidates[page.ID] = struct{}{}
		if page.InheritPermissions {
			for _, ancestorID := range splitPathIDs(page.Path) {
				candidates[ancestorID] = struct{}{}
			}
		}
	}
	ids := make([]int, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	return ids
}

func markVisibleLivePages(
	visible map[int]bool,
	pages []models.Page,
	inheritFlags map[int]bool,
	permissionsByPage map[int][]models.PagePermission,
	userID int,
	groupIDs, roleIDs []int,
	hasPageView bool,
) {
	viewLevels := pagePermissionLevelSet(PageOpView)
	for _, page := range pages {
		acl := collectACLInMemory(page, inheritFlags, permissionsByPage)
		if len(acl) > 0 {
			visible[page.ID] = hasPageView && matchesACLInMemory(userID, groupIDs, roleIDs, acl, viewLevels)
			continue
		}
		if page.InheritPermissions {
			visible[page.ID] = hasPageView
		}
	}
}

func pagePermissionLevelSet(op string) map[string]bool {
	levels := allowedLevelsForOp(op)
	set := make(map[string]bool, len(levels))
	for _, level := range levels {
		set[level] = true
	}
	return set
}

func pagePermissionQueryArgs(ids []int) (placeholders string, args []any) {
	args = make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", "), args
}

func scanPagePermissionRows(rows *sql.Rows) ([]models.PagePermission, error) {
	var permissions []models.PagePermission
	for rows.Next() {
		var permission models.PagePermission
		var grantedBy sql.NullInt64
		if err := rows.Scan(&permission.ID, &permission.PageID, &permission.PrincipalType, &permission.PrincipalID,
			&permission.PermissionLevel, &grantedBy, &permission.GrantedAt); err != nil {
			return nil, fmt.Errorf("scan ACL row: %w", err)
		}
		if grantedBy.Valid {
			id := int(grantedBy.Int64)
			permission.GrantedBy = &id
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (s *PagePermissionService) loadPagePermissions(ids []int, errorContext string) ([]models.PagePermission, error) {
	placeholders, args := pagePermissionQueryArgs(ids)
	rows, err := s.db.Query(`
		SELECT id, page_id, principal_type, principal_id, permission_level, granted_by, granted_at
		FROM page_permissions
		WHERE page_id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorContext, err)
	}
	defer func() { _ = rows.Close() }()
	return scanPagePermissionRows(rows)
}

func scanIntegerRows(rows *sql.Rows) ([]int, error) {
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
