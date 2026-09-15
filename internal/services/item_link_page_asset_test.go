package services

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type itemLinkPageAssetFixture struct {
	db     database.Database
	pages  *PageService
	links  *ItemLinkService
	userID int
	viewerID int
	wsID   int
}

func newItemLinkPageAssetFixture(t *testing.T) *itemLinkPageAssetFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "item-link-page-asset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'linker@example.test', 'linker', 'Link', 'One'),
		       (2, 'viewer@example.test', 'viewer', 'Link', 'Two')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'Links', 'LNK')`); err != nil {
		t.Fatal(err)
	}

	perm, err := NewPermissionService(db, DefaultPermissionCacheConfig())
	if err != nil {
		t.Fatal(err)
	}
	pagePerm := NewPagePermissionService(db, perm)
	assetRepo := repository.NewAssetRepository(db)
	assetPerm := NewAssetPermissionService(assetRepo, perm)
	links := NewItemLinkService(db).
		WithPermissionService(perm).
		WithPagePermissionChecker(pagePerm).
		WithAssetPermissionChecker(assetPerm)

	return &itemLinkPageAssetFixture{
		db:     db,
		pages:  NewPageService(db),
		links:  links,
		userID: 1,
		viewerID: 2,
		wsID:   1,
	}
}

func relatesToLinkTypeID(t *testing.T, db database.Database) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = 'relates_to'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *itemLinkPageAssetFixture) createPage(t *testing.T, title string) *models.Page {
	t.Helper()
	page, err := f.pages.Create(f.userID, CreatePageInput{WorkspaceID: f.wsID, Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestCreatePageToPageLink(t *testing.T) {
	f := newItemLinkPageAssetFixture(t)
	left := f.createPage(t, "Left")
	right := f.createPage(t, "Right")
	linkTypeID := relatesToLinkTypeID(t, f.db)

	id, err := f.links.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID,
		SourceType: "page",
		SourceID:   left.ID,
		TargetType: "page",
		TargetID:   right.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected link id")
	}

	outgoing, incoming, err := f.links.ListLinksForEntityWithChecks(f.userID, "page", left.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 || outgoing[0].TargetID != right.ID {
		t.Fatalf("unexpected outgoing: %+v", outgoing)
	}
	if len(incoming) != 0 {
		t.Fatalf("unexpected incoming: %+v", incoming)
	}
}

func TestCreatePageToAssetLink(t *testing.T) {
	f := newItemLinkPageAssetFixture(t)
	page := f.createPage(t, "Requirement page")
	if _, err := f.db.ExecWrite(`INSERT INTO asset_management_sets (id, name, is_default) VALUES (1, 'Default', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`INSERT INTO asset_types (id, set_id, name) VALUES (1, 1, 'Server')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`INSERT INTO assets (id, set_id, asset_type_id, title) VALUES (1, 1, 1, 'Prod DB')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`INSERT INTO asset_set_everyone_roles (set_id, role_id, granted_by)
		SELECT 1, id, 1 FROM asset_roles WHERE name = 'Viewer'`); err != nil {
		t.Fatal(err)
	}
	linkTypeID := relatesToLinkTypeID(t, f.db)

	id, err := f.links.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID,
		SourceType: "page",
		SourceID:   page.ID,
		TargetType: "asset",
		TargetID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected link id")
	}

	outgoing, _, err := f.links.ListLinksForEntityWithChecks(f.userID, "page", page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 || outgoing[0].TargetType != "asset" || outgoing[0].TargetID != 1 {
		t.Fatalf("unexpected outgoing: %+v", outgoing)
	}
}

func TestListPageLinksHidesRestrictedPageEndpoint(t *testing.T) {
	f := newItemLinkPageAssetFixture(t)
	publicPage := f.createPage(t, "Public")
	secretPage := f.createPage(t, "Secret")
	if _, err := f.pages.GrantPermission(f.userID, secretPage.ID, models.PagePrincipalTypeUser, f.userID, models.PagePermissionLevelView); err != nil {
		t.Fatal(err)
	}

	linkTypeID := relatesToLinkTypeID(t, f.db)
	if _, err := f.links.CreateLink(CreateItemLinkParams{
		LinkTypeID: linkTypeID,
		SourceType: "page",
		SourceID:   publicPage.ID,
		TargetType: "page",
		TargetID:   secretPage.ID,
	}); err != nil {
		t.Fatal(err)
	}

	outgoing, _, err := f.links.ListLinksForEntityWithChecks(f.viewerID, "page", publicPage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 0 {
		t.Fatalf("viewer without secret page ACL should not see link: %+v", outgoing)
	}

	outgoing, _, err = f.links.ListLinksForEntityWithChecks(f.userID, "page", publicPage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 || outgoing[0].TargetID != secretPage.ID {
		t.Fatalf("owner should see link: %+v", outgoing)
	}
}
