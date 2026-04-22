package services

import (
	"fmt"
	"strings"
	"testing"

	"windshift/internal/database"
)

// newHierarchyTestDB spins up an in-memory SQLite with just the items and
// item_types schema the hierarchy queries touch. We intentionally avoid
// database.Initialize() so unrelated schema changes don't break these tests.
//
// The DSN uses shared-cache with a unique name per test so the reader and
// writer connection pools in SQLiteDB see the same schema and rows, while
// parallel tests remain isolated from each other.
func newHierarchyTestDB(t *testing.T) database.Database {
	t.Helper()
	dsn := fmt.Sprintf("file:hier-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, name TEXT, key TEXT)`,
		`CREATE TABLE item_types (id INTEGER PRIMARY KEY, name TEXT, color TEXT, icon TEXT, hierarchy_level INTEGER)`,
		`CREATE TABLE items (
			id INTEGER PRIMARY KEY,
			workspace_id INTEGER NOT NULL,
			workspace_item_number INTEGER DEFAULT 1,
			item_type_id INTEGER,
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			is_task INTEGER DEFAULT 0,
			milestone_id INTEGER,
			assignee_id INTEGER,
			creator_id INTEGER,
			custom_field_values TEXT,
			parent_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO workspaces (id, name, key) VALUES (1, 'ws', 'WS')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return db
}

// insertItem creates an item with the given id and optional parent_id (0 = NULL).
func insertItem(t *testing.T, db database.Database, id, parent int) {
	t.Helper()
	if parent == 0 {
		if _, err := db.Exec(`INSERT INTO items (id, workspace_id, parent_id) VALUES (?, 1, NULL)`, id); err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
		return
	}
	if _, err := db.Exec(`INSERT INTO items (id, workspace_id, parent_id) VALUES (?, 1, ?)`, id, parent); err != nil {
		t.Fatalf("insert %d parent=%d: %v", id, parent, err)
	}
}

// setParent forces an item's parent_id, bypassing the validator. Used to
// simulate a stored cycle that the CTE queries must tolerate.
func setParent(t *testing.T, db database.Database, id, parent int) {
	t.Helper()
	if _, err := db.Exec(`UPDATE items SET parent_id = ? WHERE id = ?`, parent, id); err != nil {
		t.Fatalf("set parent %d=%d: %v", id, parent, err)
	}
}

func TestWouldCreateCycle_SelfParent(t *testing.T) {
	db := newHierarchyTestDB(t)
	insertItem(t, db, 1, 0)
	h := NewHierarchyService(db)

	got, err := h.WouldCreateCycle(1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("self-parent must be reported as cycle, got false")
	}
}

func TestWouldCreateCycle_Unrelated(t *testing.T) {
	db := newHierarchyTestDB(t)
	insertItem(t, db, 1, 0) // root A
	insertItem(t, db, 2, 0) // root B — unrelated
	h := NewHierarchyService(db)

	got, err := h.WouldCreateCycle(1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("unrelated reparent should not cycle, got true")
	}
}

func TestWouldCreateCycle_Descendant(t *testing.T) {
	db := newHierarchyTestDB(t)
	// A → B → C. Trying to make C the parent of A would create a cycle.
	insertItem(t, db, 1, 0)
	insertItem(t, db, 2, 1)
	insertItem(t, db, 3, 2)
	h := NewHierarchyService(db)

	got, err := h.WouldCreateCycle(1, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("moving an item under its own descendant must be a cycle, got false")
	}
}

func TestWouldCreateCycle_ExceedsDepthFailsClosed(t *testing.T) {
	db := newHierarchyTestDB(t)
	// Build a chain of length maxHierarchyDepth + 5. Walking from the deepest
	// child upward will never reach the target within the ceiling, so the
	// walker must fail closed (return true).
	const chainLen = maxHierarchyDepth + 5
	insertItem(t, db, 1, 0)
	for i := 2; i <= chainLen; i++ {
		insertItem(t, db, i, i-1)
	}
	h := NewHierarchyService(db)

	// Candidate ancestor is the root (1); we start walking from the deepest
	// node (chainLen). The walk covers only maxHierarchyDepth steps so it
	// never reaches 1.
	got, err := h.WouldCreateCycle(1, chainLen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("depth-exhausted walk must fail closed as cycle, got false")
	}
}

func TestGetRoot_CyclicErrors(t *testing.T) {
	db := newHierarchyTestDB(t)
	// Create a legal chain then inject a cycle by pointing the root back
	// into the chain. GetRoot must surface an error rather than silently
	// returning nil (which would hide data corruption).
	insertItem(t, db, 1, 0)
	insertItem(t, db, 2, 1)
	insertItem(t, db, 3, 2)
	setParent(t, db, 1, 3) // 1 → 3 → 2 → 1 cycle

	h := NewHierarchyService(db)
	_, err := h.GetRoot(3)
	if err == nil {
		t.Fatalf("GetRoot on cyclic hierarchy must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("error should mention cyclic, got: %v", err)
	}
}

func TestGetAncestors_CyclicTerminates(t *testing.T) {
	db := newHierarchyTestDB(t)
	// Inject a 3-cycle. Before Fix B this would loop forever (or until the
	// DB killed the query). The depth-capped CTE must return a bounded
	// result without error.
	insertItem(t, db, 1, 0)
	insertItem(t, db, 2, 1)
	insertItem(t, db, 3, 2)
	setParent(t, db, 1, 3)

	h := NewHierarchyService(db)
	got, err := h.GetAncestors(3)
	if err != nil {
		t.Fatalf("GetAncestors on cyclic hierarchy should not error, got: %v", err)
	}
	// Upper bound: the CTE walks at most maxHierarchyDepth+1 rows (base +
	// recursive steps), minus the item itself which is filtered out.
	if len(got) > maxHierarchyDepth+1 {
		t.Fatalf("GetAncestors returned %d rows, exceeds depth cap %d", len(got), maxHierarchyDepth+1)
	}
}

func TestCountDescendants_CyclicTerminates(t *testing.T) {
	db := newHierarchyTestDB(t)
	insertItem(t, db, 1, 0)
	insertItem(t, db, 2, 1)
	insertItem(t, db, 3, 2)
	setParent(t, db, 1, 3) // cycle

	h := NewHierarchyService(db)
	// Any finite, non-error return is a pass — the pre-fix behavior was to
	// loop until the DB timed out.
	if _, err := h.CountDescendants(1); err != nil {
		t.Fatalf("CountDescendants on cyclic hierarchy should not error, got: %v", err)
	}
}
