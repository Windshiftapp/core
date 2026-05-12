package handlers

import (
	"testing"

	"windshift/internal/database"
)

// Regression test for docs/bughunt2.md Run 6 finding #5.
//
// Public boards collect every active workspace ID and pass that whole set
// into ItemCRUDService.ListWithQL. ListWithQL applies the single-workspace
// filter only when no collection was resolved, so a workspace-scoped public
// collection ends up evaluating its QL across every active workspace. The
// PublicBoardHandler.itemBelongsToCollection helper exercises the exact same
// code path (collection + all-active-workspaces) and is a direct, low-cost
// probe for the leak: it returns true for an item that lives in a different
// workspace than the collection.
//
// The test will turn GREEN once both call sites in public_board.go
// (`GetPublicBoard` at ~283 and `itemBelongsToCollection` at ~707) restrict
// WorkspaceIDs to the collection's own workspace.

func seedTwoWorkspacesAndItems(t *testing.T, db database.Database) (foreignItemID, ownedItemID int) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workspaces (id, name, key, active) VALUES (1, 'W1', 'W1', 1), (2, 'W2', 'W2', 1)
	`); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO items (id, workspace_id, workspace_item_number, title, description) VALUES
			(1001, 1, 1, 'Item in W1', ''),
			(1002, 2, 1, 'Item in W2', '')
	`); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	return 1001, 1002
}

// R6-5: itemBelongsToCollection must respect the collection's home workspace.
// A workspace-scoped collection in W2 must NOT report an item from W1 as
// belonging to it just because the collection's QL happens to match.
func TestPublicBoardHandler_ItemBelongsToCollection_RespectsCollectionWorkspaceScope(t *testing.T) {
	db := newNegativeTestDB(t)
	foreignItem, ownedItem := seedTwoWorkspacesAndItems(t, db)

	// Collection lives in W2, is_public=1, with a QL that matches every item.
	if _, err := db.Exec(`
		INSERT INTO collections (id, name, ql_query, is_public, workspace_id, public_slug)
		VALUES (42, 'Public C', 'id > 0', 1, 2, 'test-board')
	`); err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	permService := newNegativeTestPermissionService(t, db)
	handler := NewPublicBoardHandler(db, permService, t.TempDir())

	// Sanity: the W2-owned item still belongs to its collection (positive case).
	belongs, err := handler.itemBelongsToCollection(ownedItem, 42)
	if err != nil {
		t.Fatalf("itemBelongsToCollection(owned): %v", err)
	}
	if !belongs {
		t.Fatalf("expected the W2-owned item to belong to the W2-scoped collection; got false")
	}

	// The bug: a W1 item should NOT belong to a W2-scoped collection.
	belongs, err = handler.itemBelongsToCollection(foreignItem, 42)
	if err != nil {
		t.Fatalf("itemBelongsToCollection(foreign): %v", err)
	}
	if belongs {
		t.Fatalf("itemBelongsToCollection returned true for an item in a different workspace than the collection; pre-fix bug.")
	}
}
