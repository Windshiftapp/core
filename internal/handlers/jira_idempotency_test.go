package handlers

import (
	"testing"

	"windshift/internal/database"
)

// TestStartImport_AlwaysCreatesNewJob_CurrentBehavior pins the current B24
// behavior — the schema and INSERT path both allow an unbounded number of
// jobs targeting the same (connection_id, project_keys) tuple. When Phase 2.4
// adds the idempotency guard at the handler layer, invert this test (assert
// the second call returns a conflict) and unskip the placeholder below.
//
// Tests at the SQL layer rather than via the HTTP handler because StartImport
// spawns a background goroutine that would try to reach Jira; the guard B24
// will install is itself a pre-INSERT check, so the SQL contract is the right
// boundary to characterize.
func TestStartImport_AlwaysCreatesNewJob_CurrentBehavior(t *testing.T) {
	t.Parallel()
	db := newJiraImportTestDB(t)

	insertConn := func(id string) {
		t.Helper()
		_, err := db.Exec(`
			INSERT INTO jira_import_connections (id, instance_url, email, encrypted_credentials)
			VALUES (?, 'https://example.atlassian.net', 'op@example.com', 'enc')
		`, id)
		if err != nil {
			t.Fatalf("seed connection: %v", err)
		}
	}
	insertJob := func(jobID, connID string) {
		t.Helper()
		_, err := db.Exec(`
			INSERT INTO jira_import_jobs (id, connection_id, status, config_json)
			VALUES (?, ?, 'queued', '{"project_keys":["TEST"]}')
		`, jobID, connID)
		if err != nil {
			t.Fatalf("insert job %s: %v", jobID, err)
		}
	}

	insertConn("conn-1")
	insertJob("job-A", "conn-1")
	insertJob("job-B", "conn-1") // Same connection + project; B24 will reject this.

	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM jira_import_jobs WHERE connection_id = ?
	`, "conn-1").Scan(&count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 jobs (no idempotency guard), got %d", count)
	}
}

// TestStartImport_RejectsDuplicate_WhenGuardImplemented is the Phase 2.4
// destination test. Symmetric to the placeholder ADF B12 tests — leaving it
// here makes the future change a deliberate flip, not a brand-new file.
func TestStartImport_RejectsDuplicate_WhenGuardImplemented(t *testing.T) {
	t.Skip("B24: idempotency guard not yet implemented")
}

// newJiraImportTestDB returns an in-memory SQLite with just the jira_import_*
// tables this file exercises. Uses cache=shared with a per-test DSN so the
// DB driver's multi-connection pool sees the same schema, while parallel
// tests stay isolated.
func newJiraImportTestDB(t *testing.T) database.Database {
	t.Helper()
	dsn := "file:" + t.Name() + "?cache=shared&mode=memory"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE jira_import_connections (
			id TEXT PRIMARY KEY,
			instance_url TEXT NOT NULL,
			email TEXT NOT NULL,
			encrypted_credentials TEXT NOT NULL
		)`,
		`CREATE TABLE jira_import_jobs (
			id TEXT PRIMARY KEY,
			connection_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			config_json TEXT NOT NULL,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return db
}
