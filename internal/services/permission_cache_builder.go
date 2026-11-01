package services

import (
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

type permissionCacheBuilder struct {
	service *PermissionService
	userID  int
	cached  *models.UserPermissionCache
}

func newPermissionCacheBuilder(service *PermissionService, userID int) *permissionCacheBuilder {
	now := time.Now()
	return &permissionCacheBuilder{
		service: service,
		userID:  userID,
		cached: &models.UserPermissionCache{
			UserID: userID, GlobalPermissions: make(map[string]bool),
			WorkspacePermissions: make(map[int]map[string]bool),
			GroupMemberships:     make([]int, 0),
			RoleAssignments:      make(map[int][]int), DirectPermissions: make(map[int][]string),
			PermissionSources:  make(map[int]map[string]string),
			UsesSharedEveryone: true,
			CachedAt:           now, ExpiresAt: now.Add(service.ttl),
		},
	}
}

func (ps *PermissionService) inheritedAgentPermissionCache(userID int) (*models.UserPermissionCache, bool, error) {
	var ownerID sql.NullInt64
	var isAgent sql.NullBool
	err := ps.db.QueryRow(
		"SELECT COALESCE(is_agent, false), agent_owner_user_id FROM users WHERE id = ?",
		userID,
	).Scan(&isAgent, &ownerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("error loading user for permission resolution: %w", err)
	}
	if !isAgent.Valid || !isAgent.Bool || !ownerID.Valid {
		return nil, false, nil
	}

	ownerCache, err := ps.buildUserPermissionCache(int(ownerID.Int64))
	if err != nil {
		return nil, false, err
	}
	agentCache := cloneUserPermissionCache(ownerCache)
	agentCache.UserID = userID
	return agentCache, true, nil
}

// cloneUserPermissionCache returns a deep copy so a cached snapshot is never
// mutated through an alias. The typed cache hands out shared pointers, so the
// agent snapshot must not share maps or slices with the owner snapshot.
func cloneUserPermissionCache(source *models.UserPermissionCache) *models.UserPermissionCache {
	if source == nil {
		return nil
	}
	clone := *source
	clone.GlobalPermissions = maps.Clone(source.GlobalPermissions)
	clone.WorkspacePermissions = cloneWorkspacePermissionMap(source.WorkspacePermissions)
	clone.WorkspaceEveryone = cloneWorkspacePermissionMap(source.WorkspaceEveryone)
	clone.GroupMemberships = slices.Clone(source.GroupMemberships)
	clone.RoleAssignments = make(map[int][]int, len(source.RoleAssignments))
	for workspaceID, roles := range source.RoleAssignments {
		clone.RoleAssignments[workspaceID] = slices.Clone(roles)
	}
	clone.DirectPermissions = make(map[int][]string, len(source.DirectPermissions))
	for workspaceID, permissions := range source.DirectPermissions {
		clone.DirectPermissions[workspaceID] = slices.Clone(permissions)
	}
	clone.PermissionSources = make(map[int]map[string]string, len(source.PermissionSources))
	for workspaceID, sources := range source.PermissionSources {
		clone.PermissionSources[workspaceID] = maps.Clone(sources)
	}
	return &clone
}

func cloneWorkspacePermissionMap(source map[int]map[string]bool) map[int]map[string]bool {
	if source == nil {
		return nil
	}
	clone := make(map[int]map[string]bool, len(source))
	for workspaceID, permissions := range source {
		clone[workspaceID] = maps.Clone(permissions)
	}
	return clone
}

func (b *permissionCacheBuilder) loadSystemAdmin() (bool, error) {
	var hasSystemAdmin bool
	err := b.service.db.QueryRow(repository.SystemAdminGrantQuery, b.userID, b.userID).Scan(&hasSystemAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("error checking system admin permission: %w", err)
	}
	b.cached.IsSystemAdmin = hasSystemAdmin
	return hasSystemAdmin, nil
}

type defaultWorkspaceRoleIDs struct {
	viewer int
	editor int
	tester int
}

type globalPermissionSource struct {
	name  string
	query string
}

func (b *permissionCacheBuilder) loadGlobalPermissions() error {
	sources := []globalPermissionSource{
		{name: "global permissions", query: `
			SELECT p.permission_key
			FROM user_global_permissions ugp
			JOIN permissions p ON ugp.permission_id = p.id
			WHERE ugp.user_id = ?
		`},
		{name: "group global permissions", query: `
			SELECT DISTINCT p.permission_key
			FROM group_members gm
			JOIN groups g ON g.id = gm.group_id
			JOIN group_global_permissions ggp ON ggp.group_id = gm.group_id
			JOIN permissions p ON p.id = ggp.permission_id
			WHERE gm.user_id = ? AND g.is_active = true
		`},
	}
	for _, source := range sources {
		if err := b.loadGlobalPermissionSource(source); err != nil {
			return err
		}
	}
	return nil
}

func (b *permissionCacheBuilder) loadGlobalPermissionSource(source globalPermissionSource) error {
	rows, err := b.service.db.Query(source.query, b.userID)
	if err != nil {
		return fmt.Errorf("error loading %s: %w", source.name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var permissionKey string
		if err := rows.Scan(&permissionKey); err != nil {
			return fmt.Errorf("scan %s: %w", source.name, err)
		}
		b.cached.GlobalPermissions[permissionKey] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating %s: %w", source.name, err)
	}
	return nil
}

func (b *permissionCacheBuilder) loadGroupMemberships() error {
	rows, err := b.service.db.Query(`
		SELECT gm.group_id
		FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		WHERE gm.user_id = ? AND g.is_active = true
	`, b.userID)
	if err != nil {
		return fmt.Errorf("error loading group memberships: %w", err)
	}
	defer rows.Close()
	memberships, err := scanIntColumn(rows)
	if err != nil {
		return fmt.Errorf("error iterating group memberships: %w", err)
	}
	if memberships == nil {
		memberships = make([]int, 0)
	}
	b.cached.GroupMemberships = memberships
	return nil
}

func (b *permissionCacheBuilder) loadUserRoleAssignments() error {
	rows, err := b.service.db.Query(
		"SELECT workspace_id, role_id FROM user_workspace_roles WHERE user_id = ?",
		b.userID,
	)
	if err != nil {
		return fmt.Errorf("error loading role assignments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID, roleID int
		if err := rows.Scan(&workspaceID, &roleID); err != nil {
			return fmt.Errorf("error scanning role assignment: %w", err)
		}
		roles := b.cached.RoleAssignments[workspaceID]
		if !slices.Contains(roles, roleID) {
			b.cached.RoleAssignments[workspaceID] = append(roles, roleID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating role assignments: %w", err)
	}
	return nil
}

func (b *permissionCacheBuilder) loadWorkspaceRolePermissions() error {
	return b.loadWorkspacePermissionGrants(`
		SELECT uwr.workspace_id, p.permission_key
		FROM user_workspace_roles uwr
		JOIN workspace_roles wr ON wr.id = uwr.role_id AND wr.permissions_enabled = true
		JOIN role_permissions rp ON uwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE uwr.user_id = ?
	`, "role", "role permissions", b.userID)
}

func (b *permissionCacheBuilder) loadGroupRolePermissions() error {
	if len(b.cached.GroupMemberships) == 0 {
		return nil
	}
	groupIDs := make([]string, len(b.cached.GroupMemberships))
	for i, groupID := range b.cached.GroupMemberships {
		groupIDs[i] = strconv.Itoa(groupID)
	}
	query := fmt.Sprintf(`
		SELECT gwr.workspace_id, p.permission_key
		FROM group_workspace_roles gwr
		JOIN workspace_roles wr ON wr.id = gwr.role_id AND wr.permissions_enabled = true
		JOIN role_permissions rp ON gwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE gwr.group_id IN (%s)
	`, strings.Join(groupIDs, ","))
	return b.loadWorkspacePermissionGrants(query, "group", "group role permissions")
}

func (b *permissionCacheBuilder) loadWorkspacePermissionGrants(
	query, source, iterationName string,
	args ...any,
) error {
	rows, err := b.service.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("error loading %s: %w", iterationName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID int
		var permissionKey string
		if err := rows.Scan(&workspaceID, &permissionKey); err != nil {
			return fmt.Errorf("error scanning %s: %w", iterationName, err)
		}
		b.grantWorkspacePermission(workspaceID, permissionKey, source)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating %s: %w", iterationName, err)
	}
	return nil
}

func (b *permissionCacheBuilder) grantWorkspacePermission(workspaceID int, permissionKey, source string) {
	permissions := b.workspacePermissions(workspaceID)
	permissions[permissionKey] = true
	sources := b.permissionSources(workspaceID)
	if sources[permissionKey] == "" {
		sources[permissionKey] = source
	}
}

func (b *permissionCacheBuilder) workspacePermissions(workspaceID int) map[string]bool {
	if b.cached.WorkspacePermissions[workspaceID] == nil {
		b.cached.WorkspacePermissions[workspaceID] = make(map[string]bool)
	}
	return b.cached.WorkspacePermissions[workspaceID]
}

func (b *permissionCacheBuilder) permissionSources(workspaceID int) map[string]string {
	if b.cached.PermissionSources[workspaceID] == nil {
		b.cached.PermissionSources[workspaceID] = make(map[string]string)
	}
	return b.cached.PermissionSources[workspaceID]
}

func (b *permissionCacheBuilder) loadPersonalWorkspacePermissions() error {
	rows, err := b.service.db.Query(`
		SELECT w.id FROM workspaces w WHERE w.is_personal = true AND w.owner_id = ? AND w.active = true
	`, b.userID)
	if err != nil {
		return fmt.Errorf("error loading personal workspaces: %w", err)
	}
	defer rows.Close()
	if len(b.service.allPermissionKeys) == 0 {
		if err := b.service.loadAllPermissionKeys(); err != nil {
			return fmt.Errorf("load permissions for personal workspace grant: %w", err)
		}
	}
	if len(b.service.allPermissionKeys) == 0 {
		return fmt.Errorf("load permissions for personal workspace grant: no permission keys found")
	}
	for rows.Next() {
		var workspaceID int
		if err := rows.Scan(&workspaceID); err != nil {
			return fmt.Errorf("scan personal workspace: %w", err)
		}
		permissions := b.workspacePermissions(workspaceID)
		for _, key := range b.service.allPermissionKeys {
			permissions[key] = true
		}
		b.permissionSources(workspaceID)["_source"] = "personal_owner"
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating personal workspaces: %w", err)
	}
	return nil
}
