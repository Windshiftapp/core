package services

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// PagePermissionService evaluates grant-only, uncached page ACLs. System,
// workspace, and page admins bypass live-page ACLs; otherwise inherited ACLs
// govern restricted pages and workspace roles govern open pages.
type PagePermissionService struct {
	db    database.Database
	perm  *PermissionService
	pages *repository.PageRepository
}

// NewPagePermissionService creates a page-permission evaluator.
func NewPagePermissionService(db database.Database, perm *PermissionService) *PagePermissionService {
	return &PagePermissionService{
		db:    db,
		perm:  perm,
		pages: repository.NewPageRepository(db),
	}
}

// Page operations evaluated by Can.
const (
	PageOpView    = "view"
	PageOpEdit    = "edit"
	PageOpAdmin   = "admin"
	PageOpRestore = "restore"
)

// HasWorkspacePermissionFor checks a workspace-level page permission.
func (s *PagePermissionService) HasWorkspacePermissionFor(userID, workspaceID int, key string) (bool, error) {
	return s.perm.HasWorkspacePermission(userID, workspaceID, key)
}

// IsSystemAdmin checks access to workspace-wide admin surfaces.
func (s *PagePermissionService) IsSystemAdmin(userID int) (bool, error) {
	return s.perm.IsSystemAdmin(userID)
}

// Can reports whether userID may perform op on pageID. Cross-workspace and
// missing pages return false to avoid leaking existence. Archived pages allow
// view and restore only to system or workspace admins.
func (s *PagePermissionService) Can(userID, workspaceID, pageID int, op string) (bool, error) {
	if !isValidPageOp(op) {
		return false, fmt.Errorf("invalid page op %q", op)
	}
	if userID == 0 {
		return false, nil
	}

	// Load the page first so the archived check can run before any admin
	// short-circuit. Cross-workspace and not-found are both 404-equivalent.
	page, err := s.pages.GetByID(pageID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if page.WorkspaceID != workspaceID {
		return false, nil
	}

	subjectID, err := s.delegatedPagePrincipalUserID(userID)
	if err != nil {
		return false, err
	}

	if page.ArchivedAt != nil {
		// Archived pages are frozen except for the explicit restore operation.
		// View and restore on archived pages are admin-only: system.admin or
		// workspace.admin. Page-level ACL grants do NOT apply to archived pages.
		if op != PageOpView && op != PageOpRestore {
			return false, nil
		}
		if isAdmin, ierr := s.perm.IsSystemAdmin(userID); ierr != nil {
			return false, ierr
		} else if isAdmin {
			return true, nil
		}
		return s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	}

	if isAdmin, ierr := s.perm.IsSystemAdmin(userID); ierr != nil {
		return false, ierr
	} else if isAdmin {
		return true, nil
	}

	hasWsAdmin, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return false, err
	}
	hasPageAdmin, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionPageAdmin)
	if err != nil {
		return false, err
	}
	if hasWsAdmin || hasPageAdmin {
		return true, nil
	}

	acl, err := s.collectEffectiveACL(page)
	if err != nil {
		return false, err
	}

	if len(acl) > 0 {
		// Restricted pages need a matching ACL and page.view membership so stale
		// ACL rows cannot grant access after a user leaves the workspace.
		matched, err := s.matchesACL(subjectID, workspaceID, acl, op)
		if err != nil || !matched {
			return matched, err
		}
		return s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionPageView)
	}

	// Empty ACL with inheritance disabled is admin-only; admins returned above.
	if !page.InheritPermissions {
		return false, nil
	}

	// Open page: fall back to workspace-scoped page.* permissions.
	return s.perm.HasWorkspacePermission(userID, workspaceID, workspacePermKeyForOp(op))
}

// ListVisiblePageIDs batch-evaluates view permission for tree rendering. It
// preserves Can's archived, ACL, inheritance, membership-floor, and
// cross-workspace semantics while bulk-loading pages, ancestors, and ACLs.
func (s *PagePermissionService) ListVisiblePageIDs(userID, workspaceID int, pageIDs []int) (map[int]bool, error) {
	out := make(map[int]bool, len(pageIDs))
	if len(pageIDs) == 0 {
		return out, nil
	}
	for _, id := range pageIDs {
		out[id] = false
	}
	if userID == 0 {
		return out, nil
	}

	access, err := s.loadPageVisibilityAccess(userID, workspaceID)
	if err != nil {
		return nil, err
	}

	pages, err := s.pages.GetByIDs(pageIDs)
	if err != nil {
		return nil, err
	}
	livePages := filterLivePagesForACL(pages, workspaceID, access, out)
	if len(livePages) == 0 {
		return out, nil
	}

	candidateIDs := pageVisibilityCandidateIDs(livePages)
	inheritFlags, err := s.loadAncestorInheritFlags(candidateIDs)
	if err != nil {
		return nil, err
	}
	aclsByPage, err := s.loadPagePermissionsByPage(candidateIDs)
	if err != nil {
		return nil, err
	}

	subjectID, err := s.delegatedPagePrincipalUserID(userID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := s.userGroupIDs(subjectID)
	if err != nil {
		return nil, err
	}
	roleIDs, err := s.userWorkspaceRoleIDs(subjectID, workspaceID)
	if err != nil {
		return nil, err
	}

	markVisibleLivePages(out, livePages, inheritFlags, aclsByPage, subjectID, groupIDs, roleIDs, access.hasPageView)
	return out, nil
}

// loadPagePermissionsByPage bulk-loads ACLs, omitting pages without rows.
func (s *PagePermissionService) loadPagePermissionsByPage(ids []int) (map[int][]models.PagePermission, error) {
	if len(ids) == 0 {
		return map[int][]models.PagePermission{}, nil
	}
	permissions, err := s.loadPagePermissions(ids, "bulk load page permissions")
	if err != nil {
		return nil, err
	}
	out := make(map[int][]models.PagePermission, len(ids))
	for _, p := range permissions {
		out[p.PageID] = append(out[p.PageID], p)
	}
	return out, nil
}

// collectACLInMemory builds an effective ACL from preloaded rows, stopping
// closest-first at a missing or non-inheriting ancestor (fail-closed).
func collectACLInMemory(p models.Page, inheritFlags map[int]bool, aclsByPage map[int][]models.PagePermission) []models.PagePermission {
	out := append([]models.PagePermission(nil), aclsByPage[p.ID]...)
	if !p.InheritPermissions {
		return out
	}
	ancestorIDs := splitPathIDs(p.Path)
	for i := len(ancestorIDs) - 1; i >= 0; i-- {
		aid := ancestorIDs[i]
		out = append(out, aclsByPage[aid]...)
		if inh, ok := inheritFlags[aid]; !ok || !inh {
			break
		}
	}
	return out
}

// matchesACLInMemory mirrors matchesACL's principal-match loop but uses
// pre-computed group/role memberships so we don't requery per page.
func matchesACLInMemory(userID int, groupIDs, roleIDs []int, acl []models.PagePermission, wantLevels map[string]bool) bool {
	for _, row := range acl {
		if !wantLevels[row.PermissionLevel] {
			continue
		}
		switch row.PrincipalType {
		case models.PagePrincipalTypeUser:
			if row.PrincipalID == userID {
				return true
			}
		case models.PagePrincipalTypeGroup:
			for _, gid := range groupIDs {
				if gid == row.PrincipalID {
					return true
				}
			}
		case models.PagePrincipalTypeRole:
			for _, rid := range roleIDs {
				if rid == row.PrincipalID {
					return true
				}
			}
		}
	}
	return false
}

// collectEffectiveACL gathers page and ancestor ACLs closest-first, including
// the first non-inheriting ancestor.
func (s *PagePermissionService) collectEffectiveACL(page *models.Page) ([]models.PagePermission, error) {
	// Always include the page's own ACL rows.
	ids := []int{page.ID}

	if page.InheritPermissions {
		ancestorIDs := splitPathIDs(page.Path)
		if len(ancestorIDs) > 0 {
			// Load the inherit_permissions flag for every ancestor in one
			// query, then walk closest-to-furthest in memory so we can
			// stop at the first ancestor that breaks inheritance.
			ancestorInherit, err := s.loadAncestorInheritFlags(ancestorIDs)
			if err != nil {
				return nil, err
			}
			// path is "/a/b/c/" listing ancestors root-first; walk in
			// reverse so the closest parent is considered first.
			for i := len(ancestorIDs) - 1; i >= 0; i-- {
				aid := ancestorIDs[i]
				ids = append(ids, aid)
				// If the ancestor row is missing (deleted concurrently)
				// or breaks inheritance, stop walking further up. The
				// missing-row case is fail-closed: don't pretend to
				// inherit through a gap in the chain.
				if inherits, ok := ancestorInherit[aid]; !ok || !inherits {
					break
				}
			}
		}
	}

	return s.loadPagePermissions(ids, "load effective ACL")
}

// loadAncestorInheritFlags bulk-loads inheritance flags. Missing pages remain
// absent so ACL collection stops fail-closed.
func (s *PagePermissionService) loadAncestorInheritFlags(ids []int) (map[int]bool, error) {
	if len(ids) == 0 {
		return map[int]bool{}, nil
	}
	placeholders, args := pagePermissionQueryArgs(ids)
	rows, err := s.db.Query(
		"SELECT id, inherit_permissions FROM pages WHERE id IN ("+placeholders+")",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("load ancestor inherit flags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int]bool, len(ids))
	for rows.Next() {
		var id int
		var inherit bool
		if err := rows.Scan(&id, &inherit); err != nil {
			return nil, fmt.Errorf("scan ancestor row: %w", err)
		}
		out[id] = inherit
	}
	return out, rows.Err()
}

// matchesACL returns true when the user has at least one effective ACL row
// (by direct user grant, group membership, or workspace role assignment)
// at a level that satisfies the requested op.
func (s *PagePermissionService) matchesACL(userID, workspaceID int, acl []models.PagePermission, op string) (bool, error) {
	groupIDs, err := s.userGroupIDs(userID)
	if err != nil {
		return false, err
	}
	roleIDs, err := s.userWorkspaceRoleIDs(userID, workspaceID)
	if err != nil {
		return false, err
	}
	return matchesACLInMemory(userID, groupIDs, roleIDs, acl, pagePermissionLevelSet(op)), nil
}

// delegatedPagePrincipalUserID returns the human principal whose page ACLs an
// authenticated user should use. Owned agents inherit their owner's effective
// permissions everywhere else via PermissionService; restricted-page ACLs must
// follow the same delegation or an owner-visible page becomes invisible to the
// owner's API token. Service users (agents without an owner) keep their own
// principal identity and must be granted page/workspace access explicitly.
func (s *PagePermissionService) delegatedPagePrincipalUserID(userID int) (int, error) {
	var isAgent sql.NullBool
	var ownerID sql.NullInt64
	err := s.db.QueryRow(
		`SELECT COALESCE(is_agent, false), agent_owner_user_id FROM users WHERE id = ?`,
		userID,
	).Scan(&isAgent, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return userID, nil
		}
		return 0, fmt.Errorf("resolve page principal delegation: %w", err)
	}
	if isAgent.Valid && isAgent.Bool && ownerID.Valid {
		return int(ownerID.Int64), nil
	}
	return userID, nil
}

// userGroupIDs returns the user's active group memberships. Mirrors the
// is_active filter applied by PermissionService.buildUserPermissionCache
// — inactive groups must not satisfy ACL grants targeted at "group N".
func (s *PagePermissionService) userGroupIDs(userID int) ([]int, error) {
	rows, err := s.db.Query(`
		SELECT gm.group_id FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		WHERE gm.user_id = ? AND g.is_active = TRUE
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("load user groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntegerRows(rows)
}

// userWorkspaceRoleIDs returns direct and group-derived workspace roles to
// match PermissionService's effective permissions.
func (s *PagePermissionService) userWorkspaceRoleIDs(userID, workspaceID int) ([]int, error) {
	rows, err := s.db.Query(`
		SELECT role_id FROM user_workspace_roles
		WHERE user_id = ? AND workspace_id = ?
		UNION
		SELECT gwr.role_id FROM group_workspace_roles gwr
		JOIN group_members gm ON gm.group_id = gwr.group_id
		JOIN groups g ON g.id = gwr.group_id
		WHERE gm.user_id = ? AND gwr.workspace_id = ? AND g.is_active = TRUE
	`, userID, workspaceID, userID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load user workspace roles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntegerRows(rows)
}

func isValidPageOp(op string) bool {
	return op == PageOpView || op == PageOpEdit || op == PageOpAdmin || op == PageOpRestore
}

func allowedLevelsForOp(op string) []string {
	switch op {
	case PageOpView:
		return []string{models.PagePermissionLevelView, models.PagePermissionLevelEdit, models.PagePermissionLevelAdmin}
	case PageOpEdit, PageOpRestore:
		return []string{models.PagePermissionLevelEdit, models.PagePermissionLevelAdmin}
	case PageOpAdmin:
		return []string{models.PagePermissionLevelAdmin}
	default:
		return nil
	}
}

func workspacePermKeyForOp(op string) string {
	switch op {
	case PageOpView:
		return models.PermissionPageView
	case PageOpEdit, PageOpRestore:
		return models.PermissionPageEdit
	case PageOpAdmin:
		return models.PermissionPageAdmin
	default:
		return models.PermissionPageView
	}
}

// splitPathIDs parses a materialized path like "/12/45/" into [12, 45].
// Returns nil for "" or "/".
func splitPathIDs(path string) []int {
	if path == "" || path == "/" {
		return nil
	}
	var out []int
	var cur int
	hasDigit := false
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			hasDigit = true
			continue
		}
		if hasDigit {
			out = append(out, cur)
			cur = 0
			hasDigit = false
		}
	}
	if hasDigit {
		out = append(out, cur)
	}
	return out
}
