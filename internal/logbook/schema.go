package logbook

import (
	_ "embed"
	"fmt"
	"windshift/internal/database"
)

//go:embed schema/logbook_postgres.sql
var logbookSchema string

// InitializeSchema creates the logbook tables in the database.
// This is called by the logbook binary on startup.
func InitializeSchema(db database.Database) error {
	_, err := db.Exec(logbookSchema)
	if err != nil {
		return fmt.Errorf("failed to initialize logbook schema: %w", err)
	}
	return nil
}
