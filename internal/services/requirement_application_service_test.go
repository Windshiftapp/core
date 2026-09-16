package services

import (
	"errors"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type requirementApplicationFixture struct {
	db      database.Database
	app     *RequirementApplicationService
	reqs    *RequirementService
	pages   *PageService
	adminID int
	viewer  int
	wsID    int
}

func newRequirementApplicationFixture(t *testing.T) *requirementApplicationFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "requirements-app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name) VALUES
		(1, 'admin@example.test', 'admin', 'Admin', 'User'),
		(2, 'viewer@example.test', 'viewer', 'View', 'Only')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'CRM', 'CRM')`); err != nil {
		t.Fatal(err)
	}
	grantSystemAdmin(t, db, 1)
	assignWorkspaceRole(t, db, 2, 1, "Viewer")

	pages := NewPageService(db)
	permService, err := NewPermissionService(db, PermissionCacheConfig{})
	if err != nil {
		t.Fatal(err)
	}
	pageAuth := NewPagePermissionService(db, permService)
	reqs := NewRequirementService(db, pages)
	app := NewRequirementApplicationService(
		reqs,
		pages,
		pageAuth,
		repository.NewWorkspaceRepository(db),
		logger.NewAuditor(db),
	)
	return &requirementApplicationFixture{db: db, app: app, reqs: reqs, pages: pages, adminID: 1, viewer: 2, wsID: 1}
}

func grantSystemAdmin(t *testing.T, db database.Database, userID int) {
	t.Helper()
	if _, err := db.ExecWrite(`
		INSERT INTO user_global_permissions (user_id, permission_id)
		SELECT ?, id FROM permissions WHERE permission_key = 'system.admin'
	`, userID); err != nil {
		t.Fatal(err)
	}
}

func assignWorkspaceRole(t *testing.T, db database.Database, userID, workspaceID int, roleName string) {
	t.Helper()
	if _, err := db.ExecWrite(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id)
		SELECT ?, ?, id FROM workspace_roles WHERE name = ? LIMIT 1
	`, userID, workspaceID, roleName); err != nil {
		t.Fatal(err)
	}
}

func TestRequirementApplicationCreateAndGet(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	view, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Authored",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Key != "CRM-DOC-1" || view.PageTitle != "Authored" {
		t.Fatalf("unexpected view %+v", view)
	}

	got, err := f.app.Get(f.adminID, f.wsID, view.RequirementNumber)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != view.Key {
		t.Fatalf("get %+v", got)
	}
}

func TestRequirementApplicationGetByPage(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	view, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Backed page",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := f.pages.Create(f.adminID, CreatePageInput{WorkspaceID: f.wsID, Title: "Plain page"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.app.GetByPage(f.adminID, f.wsID, view.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != view.Key || got.PageID != view.PageID {
		t.Fatalf("unexpected view %+v", got)
	}

	_, err = f.app.GetByPage(f.adminID, f.wsID, plain.ID)
	if !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected ErrRequirementNotFound for plain page, got %v", err)
	}

	_, err = f.app.GetByPage(f.adminID, 999, view.PageID)
	if !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque not-found for wrong workspace, got %v", err)
	}
}

func TestRequirementApplicationGetDeniedIsOpaque(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	view, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Hidden",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.SetInheritPermissions(f.adminID, view.PageID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, view.PageID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.app.Get(f.viewer, f.wsID, view.RequirementNumber); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque not found, got %v", err)
	}

	if _, err := f.app.GetByPage(f.viewer, f.wsID, view.PageID); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque not found for GetByPage, got %v", err)
	}
}

func TestRequirementApplicationListExcludesHiddenPages(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	open, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Open",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Restricted",
		RequirementType: models.RequirementTypeBusinessRule,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.SetInheritPermissions(f.adminID, hidden.PageID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, hidden.PageID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}

	views, total, err := f.app.List(f.viewer, f.wsID, RequirementListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(views) != 1 || views[0].RequirementNumber != open.RequirementNumber {
		t.Fatalf("expected only open requirement, got total=%d views=%+v", total, views)
	}
}

func TestRequirementApplicationListKeysExcludesHiddenPages(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	open, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Open key",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Hidden key",
		RequirementType: models.RequirementTypeBusinessRule,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.SetInheritPermissions(f.adminID, hidden.PageID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, hidden.PageID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}

	keys, err := f.app.ListKeys(f.viewer, f.wsID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].PageID != open.PageID || keys[0].Key != open.Key {
		t.Fatalf("expected only open key, got %+v", keys)
	}
	if keys[0].RequirementType != models.RequirementTypeUseCase {
		t.Fatalf("expected visible requirement type in key metadata, got %+v", keys[0])
	}
}

func TestRequirementApplicationUpdateDeniedIsOpaque(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	view, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Locked",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.SetInheritPermissions(f.adminID, view.PageID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, view.PageID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}

	status := models.RequirementStatusApproved
	if _, err := f.app.Update(AuditActor{UserID: f.viewer}, f.wsID, view.RequirementNumber, RequirementUpdateInput{
		Status: &status,
	}); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque denial, got %v", err)
	}
}

func TestRequirementApplicationCreateRequiresPermission(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	_, err := f.app.Create(AuditActor{UserID: f.viewer}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Denied",
		RequirementType: models.RequirementTypeUseCase,
	})
	if !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque denial, got %v", err)
	}
}

func TestRequirementApplicationPromote(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	page, err := f.pages.Create(f.adminID, CreatePageInput{WorkspaceID: f.wsID, Title: "Promote me"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.app.Promote(AuditActor{UserID: f.adminID}, f.wsID, page.ID, models.RequirementTypeUseCase, models.RequirementStatusDraft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Key != "CRM-DOC-1" || view.PageID != page.ID {
		t.Fatalf("unexpected promote view %+v", view)
	}

	plain, err := f.pages.Create(f.adminID, CreatePageInput{WorkspaceID: f.wsID, Title: "Hidden promote"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.SetInheritPermissions(f.adminID, plain.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, plain.ID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Promote(AuditActor{UserID: f.viewer}, f.wsID, plain.ID, models.RequirementTypeUseCase, "", nil); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("expected opaque promote denial, got %v", err)
	}
}

func TestRequirementApplicationUpdateHappyPath(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	created, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Mutable",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := models.RequirementStatusApproved
	updated, err := f.app.Update(AuditActor{UserID: f.adminID}, f.wsID, created.RequirementNumber, RequirementUpdateInput{
		Status: &status,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.RequirementStatusApproved {
		t.Fatalf("unexpected status %q", updated.Status)
	}
}

func TestRequirementApplicationListHistory(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	created, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "History",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := models.RequirementStatusInReview
	if _, err := f.app.Update(AuditActor{UserID: f.adminID}, f.wsID, created.RequirementNumber, RequirementUpdateInput{
		Status: &status,
	}); err != nil {
		t.Fatal(err)
	}

	history, err := f.app.ListHistory(f.adminID, f.wsID, created.RequirementNumber)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("expected promoted + status history, got %d entries", len(history))
	}

	if _, err := f.app.ListHistory(f.viewer, f.wsID, created.RequirementNumber); err != nil {
		t.Fatalf("viewer should read history on inherited page: %v", err)
	}
}

func TestRequirementApplicationListFiltersByTypeAndStatus(t *testing.T) {
	f := newRequirementApplicationFixture(t)
	if _, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Draft use case",
		RequirementType: models.RequirementTypeUseCase,
		Status:          models.RequirementStatusDraft,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           "Approved rule",
		RequirementType: models.RequirementTypeBusinessRule,
		Status:          models.RequirementStatusApproved,
	}); err != nil {
		t.Fatal(err)
	}

	views, total, err := f.app.List(f.adminID, f.wsID, RequirementListFilter{
		RequirementType: models.RequirementTypeBusinessRule,
		Status:          models.RequirementStatusApproved,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(views) != 1 || views[0].RequirementType != models.RequirementTypeBusinessRule {
		t.Fatalf("expected one approved business rule, got total=%d views=%+v", total, views)
	}
}
