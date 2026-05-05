package repository

import (
	"fmt"

	"windshift/internal/database"
)

// WorkspaceResourceRepository handles the small "is this row in this
// workspace?" check that several handlers used to do inline against an
// untyped table parameter. It exists so the helper that performs the
// check (handlers.verifyResourceInWorkspace) can stop taking
// database.Database, which in turn lets those handlers drop the
// internal/database import.
type WorkspaceResourceRepository struct {
	db database.Database
}

// NewWorkspaceResourceRepository creates a WorkspaceResourceRepository.
func NewWorkspaceResourceRepository(db database.Database) *WorkspaceResourceRepository {
	return &WorkspaceResourceRepository{db: db}
}

// allowedWorkspaceResourceTables is a closed set of tables this checker
// will run a SELECT against. The table name is interpolated into the SQL
// (parameterizing identifiers isn't possible in database/sql), so the
// allow-list is the load-bearing safety mechanism — any new caller must
// add its table here, not pass an arbitrary string.
var allowedWorkspaceResourceTables = map[string]bool{
	"test_sets":          true,
	"test_cases":         true,
	"test_run_templates": true,
}

// ExistsInWorkspace reports whether a row with the given id exists in the
// named table AND has workspace_id matching workspaceID. Returns an error
// when table is not in the allow-list (programmer error — fail loud rather
// than silently 404).
func (r *WorkspaceResourceRepository) ExistsInWorkspace(table string, resourceID, workspaceID int) (bool, error) {
	if !allowedWorkspaceResourceTables[table] {
		return false, fmt.Errorf("workspace_resource: table %q is not in the allow-list", table)
	}
	var count int
	// table validated above against the allow-list — the fmt.Sprintf cannot
	// splice attacker-controlled input.
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ? AND workspace_id = ?", table)
	if err := r.db.QueryRow(query, resourceID, workspaceID).Scan(&count); err != nil {
		return false, fmt.Errorf("check %s/%d in workspace %d: %w", table, resourceID, workspaceID, err)
	}
	return count > 0, nil
}
