package database

import (
	"fmt"
	"testing"
)

// schemaMigrationsDDL mirrors the bootstrap DDL in DB.Initialize and
// PostgresDB.Initialize. Tests create the table directly so they don't
// depend on the full schema bootstrap.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	checksum   TEXT NOT NULL DEFAULT '',
	applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

func openTestDB(t *testing.T) Database {
	t.Helper()
	// Per-test on-disk temp DB. file::memory: would also work but on-disk
	// keeps the write-connection pragmas honest under WAL.
	dsn := fmt.Sprintf("file:%s/test.db?mode=memory&cache=shared", t.TempDir())
	db, err := NewSQLiteDBWithPoolSizes(dsn, 4, 1)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(schemaMigrationsDDL); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	return db
}

func countRows(t *testing.T, db Database, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestRunPendingMigrations_NoopWithEmptyCatalog(t *testing.T) {
	db := openTestDB(t)
	if err := runPendingMigrations(db, nil); err != nil {
		t.Fatalf("empty catalog: %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations"); n != 0 {
		t.Fatalf("expected 0 stamped rows, got %d", n)
	}
}

func TestRunPendingMigrations_AppliesNewMigration(t *testing.T) {
	db := openTestDB(t)

	catalog := []Migration{{
		Version: "test_001_create_widget",
		Name:    "create widget table",
		SQLite:  "CREATE TABLE widget (id INTEGER PRIMARY KEY)",
	}}

	if err := runPendingMigrations(db, catalog); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if n := countRows(t, db, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='widget'"); n != 1 {
		t.Fatalf("widget table not created (count=%d)", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations WHERE version='test_001_create_widget'"); n != 1 {
		t.Fatalf("stamp not recorded (count=%d)", n)
	}
}

func TestRunPendingMigrations_RetroactiveStampWhenCheckTrue(t *testing.T) {
	db := openTestDB(t)

	// Simulate an existing install: the table is already there.
	if _, err := db.Exec("CREATE TABLE widget (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("seed widget table: %v", err)
	}

	catalog := []Migration{{
		Version:     "test_001_create_widget",
		Name:        "create widget table",
		CheckSQLite: "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='widget'",
		// If the runner mistakenly re-ran this body, the second CREATE would error.
		SQLite: "CREATE TABLE widget (id INTEGER PRIMARY KEY)",
	}}

	if err := runPendingMigrations(db, catalog); err != nil {
		t.Fatalf("retroactive stamp: %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations WHERE version='test_001_create_widget'"); n != 1 {
		t.Fatalf("retroactive stamp not recorded (count=%d)", n)
	}
}

func TestRunPendingMigrations_IdempotentOnRerun(t *testing.T) {
	db := openTestDB(t)

	catalog := []Migration{{
		Version: "test_001_create_widget",
		Name:    "create widget table",
		// No Check: would re-run and conflict if the runner ignored
		// schema_migrations on the second pass.
		SQLite: "CREATE TABLE widget (id INTEGER PRIMARY KEY)",
	}}

	if err := runPendingMigrations(db, catalog); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runPendingMigrations(db, catalog); err != nil {
		t.Fatalf("second run should be a no-op: %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations"); n != 1 {
		t.Fatalf("expected exactly 1 stamped row, got %d", n)
	}
}

// TestCatalog_FreshInstallStampsEveryEntry verifies that on a fresh SQLite
// install, every catalog entry's CheckSQLite predicate reports the effect
// is already present (so the runner stamps without re-running the body).
// Catches typos in table or column names: if a Check returns 0 on a fresh
// install, the runner would try to ALTER and fail with "duplicate column"
// (since fresh schema files already contain it).
func TestCatalog_FreshInstallStampsEveryEntry(t *testing.T) {
	dsn := fmt.Sprintf("file:%s/fresh.db?mode=memory&cache=shared", t.TempDir())
	db, err := NewSQLiteDBWithPoolSizes(dsn, 4, 1)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Initialize(); err != nil {
		t.Fatalf("initialize fresh db: %v", err)
	}

	// Every catalog entry should be stamped exactly once. Postgres-only
	// entries (empty SQLite body) still get stamped on SQLite — the runner
	// stamps without DDL when the body for this backend is empty.
	stamped := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations")
	if stamped != len(Catalog) {
		t.Fatalf("schema_migrations row count = %d, want %d (len Catalog)", stamped, len(Catalog))
	}
}

func TestRunPendingMigrations_AbortsOnFailingMigration(t *testing.T) {
	db := openTestDB(t)

	catalog := []Migration{
		{
			Version: "test_001_ok",
			Name:    "good migration",
			SQLite:  "CREATE TABLE good (id INTEGER PRIMARY KEY)",
		},
		{
			Version: "test_002_bad",
			Name:    "bad migration",
			SQLite:  "CREATE TABLE bad (id INTEGER PRIMARY KEY)", // succeeds…
			// …but then we deliberately break it with bogus SQL.
		},
	}
	catalog[1].SQLite = "this is not valid sql"

	err := runPendingMigrations(db, catalog)
	if err == nil {
		t.Fatalf("expected error from bad migration, got nil")
	}

	// First migration stamped and applied; second is rolled back.
	if n := countRows(t, db, "SELECT COUNT(*) FROM schema_migrations"); n != 1 {
		t.Fatalf("expected 1 stamped row after partial failure, got %d", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='bad'"); n != 0 {
		t.Fatalf("expected 'bad' table to be rolled back, found %d", n)
	}
}
