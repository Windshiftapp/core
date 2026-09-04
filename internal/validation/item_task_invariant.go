package validation

import (
	"database/sql"
	"fmt"

	"windshift/internal/constants"
)

type itemTaskWorkspaceQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// ValidateTaskState enforces the final persisted state for personal tasks.
func ValidateTaskState(db itemTaskWorkspaceQueryer, workspaceID, userID int, isTask bool, statusID *int) error {
	if !isTask {
		return nil
	}

	var isPersonal bool
	var ownerID sql.NullInt64
	if err := db.QueryRow(`
		SELECT COALESCE(is_personal, false), owner_id
		FROM workspaces
		WHERE id = ?
	`, workspaceID).Scan(&isPersonal, &ownerID); err != nil {
		return fmt.Errorf("validate task workspace: %w", err)
	}
	if !isPersonal || userID <= 0 || !ownerID.Valid || int(ownerID.Int64) != userID {
		return &ValidationError{
			Field:   "is_task",
			Message: "is_task=true is only valid in your own personal workspace",
		}
	}
	if statusID == nil || *statusID != constants.StatusIDOpen && *statusID != constants.StatusIDDone {
		return &ValidationError{
			Field:   "is_task",
			Message: "Personal tasks can only use the Open or Done status",
		}
	}
	return nil
}
