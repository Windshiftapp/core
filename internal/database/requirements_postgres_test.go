package database_test

import (
	"testing"

	"windshift/internal/database"
	"windshift/internal/testdb"
)

func TestFreshPostgresSchemaIncludesRequirementsTables(t *testing.T) {
	testdb.RunPostgres(t, func(db database.Database) {
		for _, table := range []string{"requirements", "requirement_sequences", "requirement_history"} {
			var count int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = ?
			`, table).Scan(&count)
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("table %q is missing", table)
			}
		}

		var indexCount int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM pg_indexes
			WHERE schemaname = 'public' AND indexname = 'idx_pages_id_workspace_id'
		`).Scan(&indexCount); err != nil {
			t.Fatal(err)
		}
		if indexCount != 1 {
			t.Fatal("idx_pages_id_workspace_id is missing")
		}

		for _, version := range []string{
			"20260915_requirements",
			"20260917_test_coverage_legacy_sunset",
		} {
			var migrationCount int
			if err := db.QueryRow(`
				SELECT COUNT(*) FROM schema_migrations WHERE version = ?
			`, version).Scan(&migrationCount); err != nil {
				t.Fatal(err)
			}
			if migrationCount != 1 {
				t.Fatalf("migration %q was not applied", version)
			}
		}

		var legacyOnlyCount int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM test_coverage_configurations
			WHERE requirement_item_type_ids IS NOT NULL
				AND requirement_item_type_ids != ''
				AND requirement_item_type_ids != '[]'
				AND (requirement_types IS NULL OR requirement_types = '' OR requirement_types = '[]')
		`).Scan(&legacyOnlyCount); err != nil {
			t.Fatal(err)
		}
		if legacyOnlyCount != 0 {
			t.Fatalf("expected no legacy-only coverage configs after migration, got %d", legacyOnlyCount)
		}
	})
}
