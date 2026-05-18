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

// MaterializeDeferredSQLiteCustomFieldIndexes is the exported entrypoint
// callers (currently: tests) can use to trigger the same deferred-index
// build that runs at server startup. The production path goes through
// Initialize → createDeferredSQLiteCustomFieldIndexes; integration tests
// need a way to drive that without restarting the whole DB, since the
// HTTP handler only records the desired state and never builds the
// physical index synchronously on SQLite.
func MaterializeDeferredSQLiteCustomFieldIndexes(db Database) {
	createDeferredSQLiteCustomFieldIndexes(db)
}

// createDeferredSQLiteCustomFieldIndexes creates physical expression indexes
// for rows recorded in custom_field_indexes that are not present in
// sqlite_master yet. Admin requests only record desired indexes on SQLite so
// large CREATE INDEX operations run during startup instead of blocking a live
// server request.
//
// Pending rows are collected into a slice and the read cursor is closed
// before any DDL runs — holding an open SELECT while issuing CREATE INDEX
// can deadlock SQLite (the read cursor pins a snapshot the writer needs to
// invalidate). This was reproducible when called outside of Initialize on
// shared-cache in-memory DBs (which test fixtures use).
func createDeferredSQLiteCustomFieldIndexes(db Database) {
	type pending struct {
		fieldID                           int
		targetTable, indexName, fieldType string
	}
	var queue []pending

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
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.fieldID, &p.targetTable, &p.indexName, &p.fieldType); err != nil {
			slog.Warn("failed to scan deferred custom field index", slog.String("component", "database"), slog.Any("error", err))
			continue
		}
		queue = append(queue, p)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("failed to iterate deferred custom field indexes", slog.String("component", "database"), slog.Any("error", err))
	}
	_ = rows.Close()

	for _, p := range queue {
		if !sqliteCustomFieldIndexTargetTables[p.targetTable] || !sqliteIndexableCustomFieldTypes[p.fieldType] {
			continue
		}

		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name = ?`, p.indexName).Scan(&exists); err != nil {
			slog.Warn("failed to check deferred custom field index", slog.String("component", "database"), slog.String("index", p.indexName), slog.Any("error", err))
			continue
		}
		if exists > 0 {
			continue
		}

		createSQL := buildSQLiteCustomFieldIndexSQL(p.fieldID, p.fieldType, p.targetTable, p.indexName)
		slog.Info("creating deferred custom field index", slog.String("component", "database"), slog.String("index", p.indexName), slog.String("table", p.targetTable))
		if _, err := db.ExecWrite(createSQL); err != nil {
			slog.Warn("deferred custom field index creation failed", slog.String("component", "database"), slog.String("index", p.indexName), slog.String("table", p.targetTable), slog.Any("error", err))
			continue
		}
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
