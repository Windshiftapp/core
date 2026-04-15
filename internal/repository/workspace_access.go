package repository

import "windshift/internal/database"

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access based
// on direct role assignments, group memberships, active status, and personal ownership.
// This is the single-query implementation that resolves access in SQL.
func GetAccessibleWorkspaceIDs(db database.Database, userID int) ([]int, error) {
	rows, err := db.Query(`
		SELECT DISTINCT w.id
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = true
		   OR (w.active = false AND uwr.role_id IS NOT NULL)
		   OR (w.active = false AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = true AND w.owner_id = ?)
	`, userID, userID, userID)
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
