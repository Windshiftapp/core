package repository

import (
	"database/sql"
)

// ApplyActionNullFieldsToPtr sets nullable fields on an action struct given field pointers.
// This avoids duplicating the same null-handling logic in multiple repositories.
func ApplyActionNullFieldsToPtr(
	description, triggerConfig *string,
	createdBy **int,
	descVal, triggerVal sql.NullString,
	createdVal sql.NullInt64,
) {
	if descVal.Valid {
		*description = descVal.String
	}
	if triggerVal.Valid {
		*triggerConfig = triggerVal.String
	}
	if createdVal.Valid {
		val := int(createdVal.Int64)
		*createdBy = &val
	}
}
