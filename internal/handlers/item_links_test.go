package handlers

import (
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

// These tests cover the deterministic, pure-data helpers added to lock down
// the item-linking permission cluster: resolveEntityScope,
// accessibleAssetSetIDSet, endpointVisible, filterLinksByAccess, and
// canUserViewEntity. Full HTTP-level coverage of CreateLink / DeleteLink /
// GetLinksForItem / SearchLinkableItems depends on a fully seeded
// PermissionService schema that this module does not yet provide; those
// paths are exercised in the e2e suite under core-tests/.

// newTestDB returns a real database.Database (shared in-memory SQLite) with
// just the four tables the helpers read from. Avoids db.Initialize() so
// unrelated schema changes don't break these tests.
func newTestDB(t *testing.T) database.Database {
	t.Helper()
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY, workspace_id INTEGER NOT NULL)`,
		`CREATE TABLE test_cases (id INTEGER PRIMARY KEY, workspace_id INTEGER NOT NULL, title TEXT)`,
		`CREATE TABLE asset_management_sets (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE assets (id INTEGER PRIMARY KEY, set_id INTEGER NOT NULL, title TEXT)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return db
}

// fakeAssetPerm returns pre-declared (userID, setID, permKey) allow-list
// results so tests don't need to seed the asset role tables.
type fakeAssetPerm struct {
	allow map[[3]int]bool // {userID, setID, permKeyHash}
}

func newFakeAssetPerm(entries ...fakeAssetPermEntry) *fakeAssetPerm {
	m := make(map[[3]int]bool, len(entries))
	for _, e := range entries {
		m[[3]int{e.userID, e.setID, permKeyHash(e.key)}] = true
	}
	return &fakeAssetPerm{allow: m}
}

type fakeAssetPermEntry struct {
	userID int
	setID  int
	key    string
}

func (f *fakeAssetPerm) HasAssetSetPermission(userID, setID int, key string) (bool, error) {
	return f.allow[[3]int{userID, setID, permKeyHash(key)}], nil
}

// permKeyHash keeps the fake's lookup map integer-keyed; any stable mapping
// from the handful of permission key strings the handler uses is fine.
func permKeyHash(key string) int {
	switch key {
	case AssetPermissionKeyView:
		return 1
	case AssetPermissionKeyEdit:
		return 2
	case AssetPermissionKeyDelete:
		return 3
	}
	return 0
}

// --- Tests -----------------------------------------------------------------

func TestResolveEntityScope(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, "INSERT INTO items (id, workspace_id) VALUES (10, 7)")
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (20, 8)")
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (30, 9)")

	h := &ItemLinkHandler{db: db}

	wsID, setID, found, err := h.resolveEntityScope("item", 10)
	if err != nil || !found || wsID != 7 || setID != 0 {
		t.Fatalf("item: got (%d,%d,%v,%v), want (7,0,true,nil)", wsID, setID, found, err)
	}

	wsID, setID, found, err = h.resolveEntityScope("test_case", 20)
	if err != nil || !found || wsID != 8 || setID != 0 {
		t.Fatalf("test_case: got (%d,%d,%v,%v), want (8,0,true,nil)", wsID, setID, found, err)
	}

	wsID, setID, found, err = h.resolveEntityScope("asset", 30)
	if err != nil || !found || wsID != 0 || setID != 9 {
		t.Fatalf("asset: got (%d,%d,%v,%v), want (0,9,true,nil)", wsID, setID, found, err)
	}

	// Missing rows return found=false without error — callers turn that into 404.
	if _, _, found, err := h.resolveEntityScope("item", 999); err != nil || found {
		t.Fatalf("missing item: got (_,_,%v,%v), want (false, nil)", found, err)
	}

	// Unknown entity type is a real error (caller fails closed).
	if _, _, _, err := h.resolveEntityScope("bogus", 1); err == nil {
		t.Fatal("unknown entity type: want error, got nil")
	}
}

func TestEndpointVisible_ItemUsesWorkspaceKey(t *testing.T) {
	h := &ItemLinkHandler{db: newTestDB(t)}
	accessibleKeys := map[string]bool{"ok-ws": true}

	if !h.endpointVisible("item", 0, "ok-ws", accessibleKeys, nil, nil) {
		t.Fatal("item in accessible workspace should be visible")
	}
	if h.endpointVisible("item", 0, "hidden-ws", accessibleKeys, nil, nil) {
		t.Fatal("item in inaccessible workspace should be hidden")
	}
	// Empty key retains prior behaviour: trusted, since the upstream join
	// produced no workspace match. Covers legacy rows from the old filter.
	if !h.endpointVisible("item", 0, "", accessibleKeys, nil, nil) {
		t.Fatal("item with empty workspace key should be visible (legacy path)")
	}
}

func TestEndpointVisible_TestCaseUsesWorkspaceID(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (1, 100)")
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (2, 200)")
	h := &ItemLinkHandler{db: db}
	accessibleWs := map[int]bool{100: true}

	if !h.endpointVisible("test_case", 1, "", nil, accessibleWs, nil) {
		t.Fatal("test_case in accessible workspace should be visible")
	}
	if h.endpointVisible("test_case", 2, "", nil, accessibleWs, nil) {
		t.Fatal("test_case in inaccessible workspace should be hidden")
	}
	if h.endpointVisible("test_case", 999, "", nil, accessibleWs, nil) {
		t.Fatal("missing test_case should be hidden (fail-closed)")
	}
}

func TestEndpointVisible_AssetUsesSetID(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (1, 500)")
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (2, 600)")
	h := &ItemLinkHandler{db: db}
	accessibleSets := map[int]bool{500: true}

	if !h.endpointVisible("asset", 1, "", nil, nil, accessibleSets) {
		t.Fatal("asset in accessible set should be visible")
	}
	if h.endpointVisible("asset", 2, "", nil, nil, accessibleSets) {
		t.Fatal("asset in inaccessible set should be hidden")
	}
	if h.endpointVisible("asset", 999, "", nil, nil, accessibleSets) {
		t.Fatal("missing asset should be hidden (fail-closed)")
	}
}

func TestFilterLinksByAccess_DropsMixedInaccessibleEndpoints(t *testing.T) {
	db := newTestDB(t)
	// test_case 1 is visible (ws 100), test_case 2 is not (ws 200).
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (1, 100)")
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (2, 200)")
	// asset 10 is visible (set 500), asset 20 is not (set 600).
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (10, 500)")
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (20, 600)")
	h := &ItemLinkHandler{db: db}

	accessibleKeys := map[string]bool{"ok": true}
	accessibleWs := map[int]bool{100: true}
	accessibleSets := map[int]bool{500: true}

	links := []models.ItemLink{
		{ID: 1, SourceType: "item", SourceID: 1, SourceWorkspaceKey: "ok",
			TargetType: "test_case", TargetID: 1},
		{ID: 2, SourceType: "item", SourceID: 1, SourceWorkspaceKey: "ok",
			TargetType: "test_case", TargetID: 2}, // dropped: tc in hidden ws
		{ID: 3, SourceType: "item", SourceID: 1, SourceWorkspaceKey: "ok",
			TargetType: "asset", TargetID: 10},
		{ID: 4, SourceType: "asset", SourceID: 20,
			TargetType: "item", TargetID: 1, TargetWorkspaceKey: "ok"}, // dropped: asset in hidden set
		{ID: 5, SourceType: "item", SourceID: 1, SourceWorkspaceKey: "hidden",
			TargetType: "item", TargetID: 1, TargetWorkspaceKey: "ok"}, // dropped: source item ws hidden
	}

	got := h.filterLinksByAccess(links, accessibleKeys, accessibleWs, accessibleSets)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("want IDs [1,3], got %v", linkIDs(got))
	}
}

func TestAccessibleAssetSetIDSet_UsesChecker(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, "INSERT INTO asset_management_sets (id, name) VALUES (1, 'ok')")
	mustExec(t, db, "INSERT INTO asset_management_sets (id, name) VALUES (2, 'hidden')")
	mustExec(t, db, "INSERT INTO asset_management_sets (id, name) VALUES (3, 'other')")

	checker := newFakeAssetPerm(
		fakeAssetPermEntry{userID: 42, setID: 1, key: AssetPermissionKeyView},
		fakeAssetPermEntry{userID: 42, setID: 3, key: AssetPermissionKeyView},
	)
	h := &ItemLinkHandler{db: db, assetPerm: checker}
	got := h.accessibleAssetSetIDSet(&models.User{ID: 42})

	if len(got) != 2 || !got[1] || got[2] || !got[3] {
		t.Fatalf("want {1:true,3:true}, got %v", got)
	}

	// Nil checker → fail-closed: empty set.
	h2 := &ItemLinkHandler{db: db}
	if len(h2.accessibleAssetSetIDSet(&models.User{ID: 42})) != 0 {
		t.Fatal("nil assetPerm must produce empty set (fail-closed)")
	}

	// Nil user → empty set.
	if len(h.accessibleAssetSetIDSet(nil)) != 0 {
		t.Fatal("nil user must produce empty set")
	}
}

func TestCanUserViewEntity(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, "INSERT INTO items (id, workspace_id) VALUES (1, 100)")
	mustExec(t, db, "INSERT INTO test_cases (id, workspace_id) VALUES (1, 200)")
	mustExec(t, db, "INSERT INTO assets (id, set_id) VALUES (1, 300)")
	h := &ItemLinkHandler{db: db}
	accessibleWs := map[int]bool{100: true}
	accessibleSets := map[int]bool{}

	if !h.canUserViewEntity(1, "item", 1, accessibleWs, accessibleSets) {
		t.Fatal("item in accessible ws should be viewable")
	}
	if h.canUserViewEntity(1, "test_case", 1, accessibleWs, accessibleSets) {
		t.Fatal("test_case in inaccessible ws must not be viewable")
	}
	if h.canUserViewEntity(1, "asset", 1, accessibleWs, accessibleSets) {
		t.Fatal("asset in inaccessible set must not be viewable")
	}
}

// --- helpers ---------------------------------------------------------------

func mustExec(t *testing.T, db database.Database, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func linkIDs(links []models.ItemLink) []int {
	ids := make([]int, len(links))
	for i, l := range links {
		ids[i] = l.ID
	}
	return ids
}
