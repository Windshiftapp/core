package repository

import (
	"fmt"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

func newMilestoneAttachTestDB(t *testing.T) database.Database {
	t.Helper()
	dsn := fmt.Sprintf("file:msattach-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, name TEXT, key TEXT)`,
		`CREATE TABLE items (
			id INTEGER PRIMARY KEY,
			workspace_id INTEGER NOT NULL,
			title TEXT DEFAULT ''
		)`,
		`CREATE TABLE milestones (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			target_date TEXT,
			status TEXT NOT NULL DEFAULT 'planning',
			category_id INTEGER,
			is_global INTEGER NOT NULL DEFAULT 1,
			workspace_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE item_milestones (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id INTEGER NOT NULL,
			milestone_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(item_id, milestone_id)
		)`,
		`INSERT INTO workspaces (id, name, key) VALUES (1, 'ws', 'WS')`,
		`INSERT INTO items (id, workspace_id, title) VALUES (1, 1, 'a'), (2, 1, 'b')`,
		`INSERT INTO milestones (id, name) VALUES (10, 'Alpha'), (11, 'Beta'), (12, 'Gamma')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return db
}

func TestReplaceItemMilestones_SetsAndDiffs(t *testing.T) {
	db := newMilestoneAttachTestDB(t)
	r := NewMilestoneAttachRepository(db)

	// Initially empty.
	got, err := r.ListForItem(1)
	if err != nil {
		t.Fatalf("ListForItem: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(got))
	}

	// Replace with [10, 11].
	if err := r.ReplaceItemMilestones(1, []int{10, 11}); err != nil {
		t.Fatalf("ReplaceItemMilestones: %v", err)
	}
	got, _ = r.ListForItem(1)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	// Replace with just [10] — milestone 11 should disappear.
	if err := r.ReplaceItemMilestones(1, []int{10}); err != nil {
		t.Fatalf("ReplaceItemMilestones (shrink): %v", err)
	}
	got, _ = r.ListForItem(1)
	if len(got) != 1 || got[0].ID != 10 {
		t.Fatalf("expected only milestone 10, got %+v", got)
	}

	// Empty slice clears.
	if err := r.ReplaceItemMilestones(1, []int{}); err != nil {
		t.Fatalf("ReplaceItemMilestones (clear): %v", err)
	}
	got, _ = r.ListForItem(1)
	if len(got) != 0 {
		t.Fatalf("expected empty after clear, got %d entries", len(got))
	}
}

func TestAddRemoveItemMilestone(t *testing.T) {
	db := newMilestoneAttachTestDB(t)
	r := NewMilestoneAttachRepository(db)

	if err := r.AddItemMilestone(1, 10); err != nil {
		t.Fatalf("AddItemMilestone: %v", err)
	}
	// Adding the same pair again must surface ErrDuplicateEntry.
	if err := r.AddItemMilestone(1, 10); err != ErrDuplicateEntry {
		t.Fatalf("expected ErrDuplicateEntry, got %v", err)
	}
	if err := r.RemoveItemMilestone(1, 10); err != nil {
		t.Fatalf("RemoveItemMilestone: %v", err)
	}
	got, _ := r.ListForItem(1)
	if len(got) != 0 {
		t.Fatalf("expected empty after remove, got %d entries", len(got))
	}
	// Removing again is a silent no-op.
	if err := r.RemoveItemMilestone(1, 10); err != nil {
		t.Fatalf("RemoveItemMilestone idempotent: %v", err)
	}
}

func TestLoadForItems_BulkPopulate(t *testing.T) {
	db := newMilestoneAttachTestDB(t)
	r := NewMilestoneAttachRepository(db)

	if err := r.ReplaceItemMilestones(1, []int{10, 12}); err != nil {
		t.Fatalf("seed item 1: %v", err)
	}
	if err := r.ReplaceItemMilestones(2, []int{11}); err != nil {
		t.Fatalf("seed item 2: %v", err)
	}

	items := []models.Item{{ID: 1}, {ID: 2}}
	if err := r.LoadForItems(items); err != nil {
		t.Fatalf("LoadForItems: %v", err)
	}
	if len(items[0].Milestones) != 2 {
		t.Fatalf("item 1: expected 2 milestones, got %d", len(items[0].Milestones))
	}
	if len(items[1].Milestones) != 1 || items[1].Milestones[0].ID != 11 {
		t.Fatalf("item 2: unexpected milestones %+v", items[1].Milestones)
	}
}
