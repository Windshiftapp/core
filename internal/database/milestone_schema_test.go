package database

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	milestoneCommentsMigrationVersion     = "20261109_milestone_comments"
	milestoneActivityMigrationVersion     = "20261109_milestone_activity"
	milestoneLastActivityMigrationVersion = "20261109_milestone_last_activity"
)

func sqliteObjectCount(t *testing.T, db Database, kind, name string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?", kind, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func sqliteColumnCount(t *testing.T, db Database, table, column string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func catalogMigration(t *testing.T, version string) Migration {
	t.Helper()
	for _, migration := range Catalog {
		if migration.Version == version {
			return migration
		}
	}
	t.Fatalf("migration %s is missing from the catalog", version)
	return Migration{}
}

func TestMilestoneSchemaFreshInstallAndUpgrade(t *testing.T) {
	db, err := NewSQLiteDB(filepath.Join(t.TempDir(), "windshift-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Initialize(); err != nil {
		t.Fatalf("initialize SQLite schema: %v", err)
	}
	for _, table := range []string{"milestone_comments", "milestone_history"} {
		if sqliteObjectCount(t, db, "table", table) != 1 {
			t.Fatalf("expected %s on a fresh install", table)
		}
	}
	if sqliteObjectCount(t, db, "index", "idx_item_history_milestones") != 1 {
		t.Fatal("expected idx_item_history_milestones on a fresh install")
	}
	if sqliteColumnCount(t, db, "milestones", "last_activity_at") != 1 {
		t.Fatal("expected milestones.last_activity_at on a fresh install")
	}
	for _, version := range []string{milestoneCommentsMigrationVersion, milestoneActivityMigrationVersion, milestoneLastActivityMigrationVersion} {
		var stamped int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", version).Scan(&stamped); err != nil {
			t.Fatal(err)
		}
		if stamped != 1 {
			t.Fatalf("migration %s was not stamped on a fresh install", version)
		}
	}

	// Simulate an existing install from before the migrations: no tables, no
	// item_history index, no last_activity_at column, and no stamps. The
	// catalog must create them all.
	if _, err := db.Exec(`
		DROP TABLE milestone_comments;
		DROP TABLE milestone_history;
		DROP INDEX idx_item_history_milestones;
		ALTER TABLE milestones DROP COLUMN last_activity_at;
		DELETE FROM schema_migrations WHERE version IN ('` + milestoneCommentsMigrationVersion + `', '` + milestoneActivityMigrationVersion + `', '` + milestoneLastActivityMigrationVersion + `');
	`); err != nil {
		t.Fatal(err)
	}
	if err := runPendingMigrations(db, Catalog); err != nil {
		t.Fatalf("run milestone migrations: %v", err)
	}
	for _, table := range []string{"milestone_comments", "milestone_history"} {
		if sqliteObjectCount(t, db, "table", table) != 1 {
			t.Fatalf("expected the migration to create %s", table)
		}
	}
	for _, index := range []string{
		"idx_milestone_comments_milestone", "idx_milestone_comments_author",
		"idx_milestone_history_milestone", "idx_milestone_history_user", "idx_item_history_milestones",
	} {
		if sqliteObjectCount(t, db, "index", index) != 1 {
			t.Fatalf("expected index %s after migration", index)
		}
	}
	if sqliteColumnCount(t, db, "milestones", "last_activity_at") != 1 {
		t.Fatal("expected the migration to add milestones.last_activity_at")
	}
}

func TestMilestoneMigrationsBackendParity(t *testing.T) {
	for version, fragments := range map[string][]string{
		milestoneCommentsMigrationVersion: {
			"CREATE TABLE milestone_comments",
			"REFERENCES milestones(id) ON DELETE CASCADE",
			"REFERENCES users(id) ON DELETE CASCADE",
			"idx_milestone_comments_milestone",
		},
		milestoneActivityMigrationVersion: {
			"CREATE TABLE milestone_history",
			"REFERENCES milestones(id) ON DELETE CASCADE",
			"REFERENCES users(id) ON DELETE RESTRICT",
			"idx_milestone_history_milestone",
			"idx_item_history_milestones",
			"WHERE field_name = 'milestones'",
		},
		milestoneLastActivityMigrationVersion: {
			"ALTER TABLE milestones ADD COLUMN",
			"last_activity_at",
		},
	} {
		migration := catalogMigration(t, version)
		for name, body := range map[string]string{"sqlite": migration.SQLite, "postgres": migration.Postgres} {
			for _, fragment := range fragments {
				if !strings.Contains(body, fragment) {
					t.Fatalf("%s %s migration body is missing %q", version, name, fragment)
				}
			}
		}
	}
	for name, schema := range map[string]string{
		"milestone_comments.sql":          milestoneCommentsSchema,
		"milestone_comments_postgres.sql": milestoneCommentsSchemaPostgres,
		"milestone_history.sql":           milestoneHistorySchema,
		"milestone_history_postgres.sql":  milestoneHistorySchemaPostgres,
	} {
		table := strings.TrimSuffix(strings.TrimSuffix(name, ".sql"), "_postgres")
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected %s in %s", table, name)
		}
	}
	if !strings.Contains(milestonesSchema, "last_activity_at") || !strings.Contains(milestonesSchemaPostgres, "last_activity_at") {
		t.Fatal("expected milestones.last_activity_at in both canonical milestones schema files")
	}
	if !strings.Contains(itemsSchema, "idx_item_history_milestones") || !strings.Contains(itemsSchemaPostgres, "idx_item_history_milestones") {
		t.Fatal("expected idx_item_history_milestones in both canonical items schema files")
	}
}
