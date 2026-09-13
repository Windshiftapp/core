package database

import (
	"path/filepath"
	"testing"
)

func TestFreshSQLiteSchemaIncludesGitLabReleaseAndWebhookTables(t *testing.T) {
	db, err := NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "windshift.db"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}

	for _, column := range []string{"workspace_repository_id", "tag_url", "release_status", "released_at", "assets_json", "last_synced_at"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('milestone_releases') WHERE name = ?`, column).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("milestone_releases column %q is missing", column)
		}
	}
	for _, table := range []string{"scm_webhooks", "scm_webhook_deliveries"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q is missing", table)
		}
	}
}
