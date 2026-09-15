package services

import (
	"errors"
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

func newItemLinkServiceFixture(t *testing.T) (*ItemLinkService, database.Database) {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "item-link-service.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'link-test@example.test', 'link-test', 'Link', 'Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'CRM', 'CRM')`); err != nil {
		t.Fatal(err)
	}
	return NewItemLinkService(db), db
}

func insertPage(t *testing.T, db database.Database, title string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`
		INSERT INTO pages (
			workspace_id, title, slug, metadata, content, content_hash, excerpt,
			created_by, inherit_permissions, path, depth, created_at, updated_at
		) VALUES (1, ?, ?, '{}', '', '', '', 1, true, '/', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, title, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertItem(t *testing.T, db database.Database) int {
	t.Helper()
	var statusID int
	if err := db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := db.QueryRow(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (1, 1, 'Story', '', 'a0', ?, 1, CURRENT_TIMESTAMP)
		RETURNING id
	`, statusID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func linkTypeID(t *testing.T, db database.Database, builtinKey string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = ?`, builtinKey).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestItemLinkServiceImplementsAllowsItemToPage(t *testing.T) {
	svc, db := newItemLinkServiceFixture(t)
	itemID := insertItem(t, db)
	pageID := insertPage(t, db, "Requirement page")

	_, err := svc.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID(t, db, "implements"),
		SourceType: "item",
		SourceID:   itemID,
		TargetType: "page",
		TargetID:   pageID,
		CreatedBy:  testUserID(),
	})
	if err != nil {
		t.Fatalf("expected item→page implements link to succeed: %v", err)
	}
}

func TestItemLinkServiceImplementsRejectsPageToPage(t *testing.T) {
	svc, db := newItemLinkServiceFixture(t)
	childPage := insertPage(t, db, "Child")
	parentPage := insertPage(t, db, "Parent")

	_, err := svc.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID(t, db, "implements"),
		SourceType: "page",
		SourceID:   childPage,
		TargetType: "page",
		TargetID:   parentPage,
		CreatedBy:  testUserID(),
	})
	if !errors.Is(err, ErrInvalidLinkTypeForEntities) {
		t.Fatalf("expected ErrInvalidLinkTypeForEntities, got %v", err)
	}
}

func TestItemLinkServiceSpecifiesAllowsPageToPage(t *testing.T) {
	svc, db := newItemLinkServiceFixture(t)
	childPage := insertPage(t, db, "Child requirement")
	parentPage := insertPage(t, db, "Parent requirement")

	_, err := svc.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID(t, db, "specifies"),
		SourceType: "page",
		SourceID:   childPage,
		TargetType: "page",
		TargetID:   parentPage,
		CreatedBy:  testUserID(),
	})
	if err != nil {
		t.Fatalf("expected page→page specifies link to succeed: %v", err)
	}
}

func TestItemLinkServiceSpecifiesRejectsItemToPage(t *testing.T) {
	svc, db := newItemLinkServiceFixture(t)
	itemID := insertItem(t, db)
	pageID := insertPage(t, db, "Requirement page")

	_, err := svc.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID(t, db, "specifies"),
		SourceType: "item",
		SourceID:   itemID,
		TargetType: "page",
		TargetID:   pageID,
		CreatedBy:  testUserID(),
	})
	if !errors.Is(err, ErrInvalidLinkTypeForEntities) {
		t.Fatalf("expected ErrInvalidLinkTypeForEntities, got %v", err)
	}
}

func testUserID() *int {
	id := 1
	return &id
}
