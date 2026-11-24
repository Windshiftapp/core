package validation

import (
	"database/sql"
	"errors"
	"fmt"
)

// WorkspaceConfigurationQueryer is implemented by database connections and
// transactions that can read workspace configuration assignments.
type WorkspaceConfigurationQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// IsPriorityAllowedInWorkspace verifies that a priority exists and is enabled
// by the workspace's configuration set. A workspace without an explicit
// priority catalog uses the global catalog.
func IsPriorityAllowedInWorkspace(q WorkspaceConfigurationQueryer, workspaceID, priorityID int) (bool, error) {
	var priorityExists bool
	if err := q.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM priorities WHERE id = ?)",
		priorityID,
	).Scan(&priorityExists); err != nil {
		return false, fmt.Errorf("failed to check priority existence: %w", err)
	}
	if !priorityExists {
		return false, nil
	}

	var configSetID *int
	err := q.QueryRow(
		"SELECT configuration_set_id FROM workspace_configuration_sets WHERE workspace_id = ?",
		workspaceID,
	).Scan(&configSetID)
	if errors.Is(err, sql.ErrNoRows) {
		// An unassigned workspace uses the global catalog.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to query workspace config set: %w", err)
	}
	if configSetID == nil {
		// A NULL assignment is normal for an unassigned workspace.
		return true, nil
	}

	var allowed bool
	if err := q.QueryRow(
		`SELECT
			NOT EXISTS(SELECT 1 FROM configuration_set_priorities WHERE configuration_set_id = ?)
			OR EXISTS(SELECT 1 FROM configuration_set_priorities WHERE configuration_set_id = ? AND priority_id = ?)`,
		*configSetID, *configSetID, priorityID,
	).Scan(&allowed); err != nil {
		return false, fmt.Errorf("failed to check priority in config set: %w", err)
	}
	return allowed, nil
}
