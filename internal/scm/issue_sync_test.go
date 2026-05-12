package scm

import (
	"context"
	"testing"

	"windshift/internal/database"
)

// TestIssueSync_ItemLabelInsert_Idempotent verifies the ON CONFLICT DO NOTHING
// patch on the two item_labels inserts in syncLabels (issue_sync.go ~lines 1003
// and 1036). Re-inserting the same (item_id, label_id) must succeed without
// error and without producing a duplicate row.
//
// syncLabels is private and reaches the SQL through SCM/OAuth-heavy plumbing,
// so this test exercises the schema+SQL contract directly: the exact INSERT
// string from issue_sync.go run against the production item_labels schema.
func TestIssueSync_ItemLabelInsert_Idempotent(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE item_labels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id INTEGER NOT NULL,
			label_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(item_id, label_id)
		)
	`); err != nil {
		t.Fatalf("create item_labels: %v", err)
	}

	// Same SQL string used in internal/scm/issue_sync.go.
	const insertSQL = "INSERT INTO item_labels (item_id, label_id) VALUES (?, ?) ON CONFLICT DO NOTHING"

	ctx := context.Background()
	exec := func(itemID, labelID int) {
		t.Helper()
		if _, err := db.ExecContext(ctx, insertSQL, itemID, labelID); err != nil {
			t.Fatalf("exec (%d,%d): %v", itemID, labelID, err)
		}
	}
	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM item_labels`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	exec(7, 100)
	if got := count(); got != 1 {
		t.Fatalf("after first insert: got %d, want 1", got)
	}

	// Duplicate: ON CONFLICT DO NOTHING must silently skip.
	exec(7, 100)
	if got := count(); got != 1 {
		t.Fatalf("after duplicate insert: got %d, want 1", got)
	}

	// Different label on same item.
	exec(7, 101)
	if got := count(); got != 2 {
		t.Fatalf("after second label on same item: got %d, want 2", got)
	}

	// Same label on different item.
	exec(8, 100)
	if got := count(); got != 3 {
		t.Fatalf("after same label on different item: got %d, want 3", got)
	}
}
