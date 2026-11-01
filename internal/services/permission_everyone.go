package services

import (
	"fmt"

	"windshift/internal/models"
)

// everyoneTable is the user-independent "Everyone" fallback for active,
// non-personal workspaces. Every user shares one table instead of embedding a
// per-workspace permission map in each snapshot, which made snapshot size scale
// with the total number of workspaces in the instance.
//
// The fallback mirrors the derived-role hierarchy: when the Viewer role has no
// explicit assignment in a workspace, everyone gets Viewer; Editor is added
// when Editor is unassigned, and Tester only when both Editor and Tester are
// unassigned. Administrator is never implicit.
type everyoneTable struct {
	generation uint64

	viewerOpen map[int]bool
	editorOpen map[int]bool
	testerOpen map[int]bool

	viewerPerms map[string]bool
	editorPerms map[string]bool
	testerPerms map[string]bool
}

// has reports whether the Everyone fallback grants permission in workspaceID.
func (t *everyoneTable) has(workspaceID int, permission string) bool {
	if t == nil || !t.viewerOpen[workspaceID] {
		return false
	}
	if t.viewerPerms[permission] {
		return true
	}
	if !t.editorOpen[workspaceID] {
		return false
	}
	if t.editorPerms[permission] {
		return true
	}
	return t.testerOpen[workspaceID] && t.testerPerms[permission]
}

// materialize expands the table into the per-workspace maps the permission
// profile response still uses. Only that response consumes it; nothing caches
// the result.
func (t *everyoneTable) materialize() map[int]map[string]bool {
	if t == nil {
		return nil
	}
	out := make(map[int]map[string]bool, len(t.viewerOpen))
	for workspaceID, viewerOpen := range t.viewerOpen {
		if !viewerOpen {
			continue
		}
		perms := make(map[string]bool, len(t.viewerPerms))
		for key := range t.viewerPerms {
			perms[key] = true
		}
		if t.editorOpen[workspaceID] {
			for key := range t.editorPerms {
				perms[key] = true
			}
			if t.testerOpen[workspaceID] {
				for key := range t.testerPerms {
					perms[key] = true
				}
			}
		}
		out[workspaceID] = perms
	}
	return out
}

// defaultRoleIDs resolves the built-in workspace roles the Everyone fallback
// derives from.
func (ps *PermissionService) defaultRoleIDs() (defaultWorkspaceRoleIDs, error) {
	var ids defaultWorkspaceRoleIDs
	roles := []struct {
		key string
		id  *int
	}{
		{key: models.RoleBuiltinViewer, id: &ids.viewer},
		{key: models.RoleBuiltinEditor, id: &ids.editor},
		{key: models.RoleBuiltinTester, id: &ids.tester},
	}
	for _, role := range roles {
		if err := ps.db.QueryRow(
			"SELECT id FROM workspace_roles WHERE builtin_key = ? LIMIT 1",
			role.key,
		).Scan(role.id); err != nil {
			return ids, fmt.Errorf("resolve built-in workspace role %q: %w", role.key, err)
		}
	}
	return ids, nil
}

// explicitRoleAssignments returns every workspace's explicitly assigned role
// IDs across users and groups. The Everyone fallback depends on whether anyone
// holds a built-in role, not on who that is, so this is instance-global.
func (ps *PermissionService) explicitRoleAssignments() (map[int]map[int]bool, error) {
	assignments := make(map[int]map[int]bool)
	rows, err := ps.db.Query(`
		SELECT DISTINCT workspace_id, role_id FROM user_workspace_roles
		UNION
		SELECT DISTINCT workspace_id, role_id FROM group_workspace_roles
	`)
	if err != nil {
		return nil, fmt.Errorf("load explicit role assignments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID, roleID int
		if err := rows.Scan(&workspaceID, &roleID); err != nil {
			return nil, fmt.Errorf("scan explicit role assignment: %w", err)
		}
		if assignments[workspaceID] == nil {
			assignments[workspaceID] = make(map[int]bool)
		}
		assignments[workspaceID][roleID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate explicit role assignments: %w", err)
	}
	return assignments, nil
}

func (ps *PermissionService) buildEveryoneTable(generation uint64) (*everyoneTable, error) {
	activeWorkspaces, err := ps.getWorkspaceActiveMap()
	if err != nil {
		return nil, fmt.Errorf("error loading workspace states: %w", err)
	}
	roleIDs, err := ps.defaultRoleIDs()
	if err != nil {
		return nil, err
	}
	explicitAssignments, err := ps.explicitRoleAssignments()
	if err != nil {
		return nil, err
	}
	viewerPerms, err := ps.getRolePermissions(roleIDs.viewer)
	if err != nil {
		return nil, fmt.Errorf("load permissions for workspace role %d: %w", roleIDs.viewer, err)
	}
	editorPerms, err := ps.getRolePermissions(roleIDs.editor)
	if err != nil {
		return nil, fmt.Errorf("load permissions for workspace role %d: %w", roleIDs.editor, err)
	}
	testerPerms, err := ps.getRolePermissions(roleIDs.tester)
	if err != nil {
		return nil, fmt.Errorf("load permissions for workspace role %d: %w", roleIDs.tester, err)
	}

	table := &everyoneTable{
		generation:  generation,
		viewerOpen:  make(map[int]bool, len(activeWorkspaces)),
		editorOpen:  make(map[int]bool, len(activeWorkspaces)),
		testerOpen:  make(map[int]bool, len(activeWorkspaces)),
		viewerPerms: viewerPerms,
		editorPerms: editorPerms,
		testerPerms: testerPerms,
	}
	for workspaceID, active := range activeWorkspaces {
		if !active {
			continue
		}
		explicit := explicitAssignments[workspaceID]
		table.viewerOpen[workspaceID] = !explicit[roleIDs.viewer]
		table.editorOpen[workspaceID] = !explicit[roleIDs.editor]
		table.testerOpen[workspaceID] = !explicit[roleIDs.tester]
	}
	return table, nil
}

// refreshEveryoneTable rebuilds the shared table from current database state.
// Snapshot builds call it so a workspace or role assignment written since the
// last build is visible without waiting for a TTL.
func (ps *PermissionService) refreshEveryoneTable() (*everyoneTable, error) {
	generation := ps.everyoneGeneration.Add(1)
	table, err := ps.buildEveryoneTable(generation)
	if err != nil {
		return nil, err
	}
	ps.everyone.Store(table)
	return table, nil
}

// ensureEveryoneTable returns the shared Everyone table, rebuilding it after an
// authorization mutation advanced the generation. Concurrent callers share one
// build.
func (ps *PermissionService) ensureEveryoneTable() (*everyoneTable, error) {
	generation := ps.everyoneGeneration.Load()
	if table := ps.everyone.Load(); table != nil && table.generation == generation {
		return table, nil
	}
	result, err, _ := ps.everyoneLoads.Do(fmt.Sprintf("generation:%d", generation), func() (any, error) {
		if table := ps.everyone.Load(); table != nil && table.generation == generation {
			return table, nil
		}
		table, err := ps.buildEveryoneTable(generation)
		if err != nil {
			return nil, err
		}
		if ps.everyoneGeneration.Load() == generation {
			ps.everyone.Store(table)
		}
		return table, nil
	})
	if err != nil {
		return nil, err
	}
	table, ok := result.(*everyoneTable)
	if !ok {
		return nil, fmt.Errorf("unexpected everyone table type %T", result)
	}
	return table, nil
}

// EveryonePermissions returns the materialized Everyone fallback for the
// permission profile response. It is never cached.
func (ps *PermissionService) EveryonePermissions() (map[int]map[string]bool, error) {
	table, err := ps.ensureEveryoneTable()
	if err != nil {
		return nil, err
	}
	return table.materialize(), nil
}
