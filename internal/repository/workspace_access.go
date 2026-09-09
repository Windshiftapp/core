package repository

import "windshift/internal/database"

// legacyUngatedWorkspaceAccessJoin is a compatibility fallback for callers
// without PermissionService. It is not an authorization primitive because it
// admits every active non-personal workspace.
const legacyUngatedWorkspaceAccessJoin = `
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE (w.active = true AND (w.is_personal = false OR w.is_personal IS NULL))
		   OR (w.active = false AND uwr.role_id IS NOT NULL)
		   OR (w.active = false AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = true AND w.owner_id = ?)
	`

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access based
// on direct role assignments, group memberships, active status, and personal ownership.
// This is the single-query implementation that resolves access in SQL.
func GetAccessibleWorkspaceIDs(db database.Database, userID int) ([]int, error) {
	rows, err := db.Query("SELECT DISTINCT w.id"+legacyUngatedWorkspaceAccessJoin, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// FilterSharedWorkspaceIDs removes personal workspaces from accessible IDs
// when integrations opt out of personal items with exclude_personal.
func FilterSharedWorkspaceIDs(db database.Database, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	rows, err := db.Query(`
		SELECT id FROM workspaces
		WHERE id IN (`+string(placeholders)+`) AND is_personal = true
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	personal := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		personal[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	shared := make([]int, 0, len(ids))
	for _, id := range ids {
		if !personal[id] {
			shared = append(shared, id)
		}
	}
	return shared, nil
}

// IsPersonalWorkspace reports whether the workspace is a personal workspace.
func IsPersonalWorkspace(db database.Database, workspaceID int) (bool, error) {
	var isPersonal bool
	err := db.QueryRow(
		`SELECT COALESCE(is_personal, false) FROM workspaces WHERE id = ?`,
		workspaceID,
	).Scan(&isPersonal)
	if err != nil {
		return false, err
	}
	return isPersonal, nil
}
