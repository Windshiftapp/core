package database

import (
	"fmt"
	"log/slog"
	"strconv"
)

var sqliteIndexableCustomFieldTypes = map[string]bool{
	"number": true,
	"date":   true,
	"text":   true,
}

var sqliteCustomFieldIndexTargetTables = map[string]bool{
	"items":  true,
	"assets": true,
}

// createDeferredSQLiteCustomFieldIndexes creates physical expression indexes
// for rows recorded in custom_field_indexes that are not present in
// sqlite_master yet. Admin requests only record desired indexes on SQLite so
// large CREATE INDEX operations run during startup instead of blocking a live
// server request.
func createDeferredSQLiteCustomFieldIndexes(db Database) {
	rows, err := db.Query(`
		SELECT cfi.custom_field_id, cfi.target_table, cfi.index_name, cfd.field_type
		FROM custom_field_indexes cfi
		JOIN custom_field_definitions cfd ON cfd.id = cfi.custom_field_id
		WHERE cfi.target_table IN ('items', 'assets')
	`)
	if err != nil {
		slog.Warn("failed to load deferred custom field indexes", slog.String("component", "database"), slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var fieldID int
		var targetTable, indexName, fieldType string
		if err := rows.Scan(&fieldID, &targetTable, &indexName, &fieldType); err != nil {
			slog.Warn("failed to scan deferred custom field index", slog.String("component", "database"), slog.Any("error", err))
			continue
		}
		if !sqliteCustomFieldIndexTargetTables[targetTable] || !sqliteIndexableCustomFieldTypes[fieldType] {
			continue
		}

		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name = ?`, indexName).Scan(&exists); err != nil {
			slog.Warn("failed to check deferred custom field index", slog.String("component", "database"), slog.String("index", indexName), slog.Any("error", err))
			continue
		}
		if exists > 0 {
			continue
		}

		createSQL := buildSQLiteCustomFieldIndexSQL(fieldID, fieldType, targetTable, indexName)
		slog.Info("creating deferred custom field index", slog.String("component", "database"), slog.String("index", indexName), slog.String("table", targetTable))
		if _, err := db.ExecWrite(createSQL); err != nil {
			slog.Warn("deferred custom field index creation failed", slog.String("component", "database"), slog.String("index", indexName), slog.String("table", targetTable), slog.Any("error", err))
			continue
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("failed to iterate deferred custom field indexes", slog.String("component", "database"), slog.Any("error", err))
	}
}

func buildSQLiteCustomFieldIndexSQL(fieldID int, fieldType, targetTable, indexName string) string {
	castType := "TEXT"
	if fieldType == "number" {
		castType = "NUMERIC"
	}
	fieldIDStr := strconv.Itoa(fieldID)
	// %q would Go-quote the field ID and escape characters, breaking the
	// JSON path literal embedded in the SQL.
	return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(NULLIF(custom_field_values,'') ->> '$."%s"' AS %s))`, //nolint:gocritic // see comment above
		indexName, targetTable, fieldIDStr, castType)
}
