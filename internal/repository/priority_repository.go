package repository

import (
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

type PriorityRepository struct {
	db database.Database
}

func NewPriorityRepository(db database.Database) *PriorityRepository {
	return &PriorityRepository{db: db}
}

// ListForWorkspace returns priorities enabled through the workspace's
// configuration set, falling back to the global catalog when none are mapped.
func (r *PriorityRepository) ListForWorkspace(workspaceID int) ([]models.Priority, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT p.id, COALESCE(p.builtin_key, ''), p.name, COALESCE(p.description, ''), COALESCE(p.icon, ''), COALESCE(p.color, ''),
		       p.sort_order, p.is_default
		FROM priorities p
		WHERE NOT EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_priorities csp ON wcs.configuration_set_id = csp.configuration_set_id
			WHERE wcs.workspace_id = ?
		)
		OR EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_priorities csp ON wcs.configuration_set_id = csp.configuration_set_id
			WHERE wcs.workspace_id = ? AND csp.priority_id = p.id
		)
		ORDER BY p.sort_order, p.name
	`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list priorities for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.Priority, 0)
	for rows.Next() {
		var priority models.Priority
		if err := rows.Scan(&priority.ID, &priority.BuiltinKey, &priority.Name, &priority.Description,
			&priority.Icon, &priority.Color, &priority.SortOrder, &priority.IsDefault); err != nil {
			return nil, fmt.Errorf("scan workspace priority: %w", err)
		}
		out = append(out, priority)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace priorities: %w", err)
	}
	return out, nil
}
