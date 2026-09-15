package services

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type testCoverageApplicationFixture struct {
	db      database.Database
	app     *TestManagementApplicationService
	reqApp  *RequirementApplicationService
	pages   *PageService
	adminID int
	viewer  int
	wsID    int
}

func newTestCoverageApplicationFixture(t *testing.T) *testCoverageApplicationFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "test-coverage-app.db"))
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
	assignWorkspaceRole(t, db, 2, 1, "Tester")

	pages := NewPageService(db)
	permService, err := NewPermissionService(db, PermissionCacheConfig{})
	if err != nil {
		t.Fatal(err)
	}
	pageAuth := NewPagePermissionService(db, permService)
	reqs := NewRequirementService(db, pages)
	reqApp := NewRequirementApplicationService(
		reqs,
		pages,
		pageAuth,
		repository.NewWorkspaceRepository(db),
		logger.NewAuditor(db),
	)
	app := NewTestManagementApplicationService(db, permService, pageAuth)
	return &testCoverageApplicationFixture{
		db:      db,
		app:     app,
		reqApp:  reqApp,
		pages:   pages,
		adminID: 1,
		viewer:  2,
		wsID:    1,
	}
}

func (f *testCoverageApplicationFixture) createRequirement(t *testing.T, title, reqType string) *RequirementView {
	t.Helper()
	view, err := f.reqApp.Create(AuditActor{UserID: f.adminID}, CreateRequirementInput{
		WorkspaceID:     f.wsID,
		Title:           title,
		RequirementType: reqType,
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestCoverageConfigRejectsLegacyItemTypesOnly(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	_, err := f.app.CreateCoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, CoverageConfigInput{
		RequirementItemTypeIDs: []int{1},
	})
	var validation *TestManagementValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestCoverageConfigRejectsEmptyPayload(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	_, err := f.app.CreateCoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, CoverageConfigInput{})
	var validation *TestManagementValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestCoverageConfigRejectsInvalidRequirementType(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	_, err := f.app.CreateCoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, CoverageConfigInput{
		RequirementTypes: []string{"not_a_real_type"},
	})
	var validation *TestManagementValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestCoverageConfigCreatePageBackedClearsLegacyIDs(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	cfg, err := f.app.CreateCoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, CoverageConfigInput{
		RequirementItemTypeIDs: []int{9},
		RequirementTypes:       []string{models.RequirementTypeUseCase, models.RequirementTypeBusinessRule},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UsesPageBackedRequirements() {
		t.Fatalf("expected page-backed config, got %#v", cfg)
	}
	if len(cfg.RequirementItemTypeIDs) != 0 {
		t.Fatalf("expected legacy IDs cleared, got %#v", cfg.RequirementItemTypeIDs)
	}
}

func TestCoverageConfigUpdateSwitchesLegacyRowToRequirementTypes(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	if _, err := f.db.ExecWrite(`
		INSERT INTO test_coverage_configurations (workspace_id, requirement_item_type_ids, requirement_types)
		VALUES (?, '[1,2]', NULL)
	`, f.wsID); err != nil {
		t.Fatal(err)
	}
	existing, err := f.app.CoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID})
	if err != nil {
		t.Fatal(err)
	}
	if !existing.UsesLegacyItemRequirements() {
		t.Fatalf("expected legacy config, got %#v", existing)
	}

	updated, err := f.app.UpdateCoverageConfig(
		f.adminID,
		TestCoverageScope{WorkspaceID: f.wsID},
		existing.ID,
		CoverageConfigInput{RequirementTypes: []string{models.RequirementTypeFunctionalRequirement}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UsesPageBackedRequirements() {
		t.Fatalf("expected page-backed config, got %#v", updated)
	}
	if len(updated.RequirementItemTypeIDs) != 0 {
		t.Fatalf("expected legacy IDs cleared, got %#v", updated.RequirementItemTypeIDs)
	}
}

func TestCoverageApplicationPageBackedExcludesHiddenPages(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	open := f.createRequirement(t, "Open", models.RequirementTypeUseCase)
	hidden := f.createRequirement(t, "Hidden", models.RequirementTypeUseCase)

	if _, err := f.pages.SetInheritPermissions(f.adminID, hidden.PageID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, 'user', ?, 'view', ?)
	`, hidden.PageID, f.adminID, f.adminID); err != nil {
		t.Fatal(err)
	}

	_, err := f.app.CreateCoverageConfig(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, CoverageConfigInput{
		RequirementTypes: []string{models.RequirementTypeUseCase},
	})
	if err != nil {
		t.Fatal(err)
	}

	items, total, summary, err := f.app.CoverageRequirements(f.viewer, TestCoverageScope{WorkspaceID: f.wsID}, TestCoverageRequirementsFilter{
		Limit:  50,
		Offset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || summary.Total != 1 {
		t.Fatalf("viewer should only see open requirement, got total=%d items=%+v summary=%+v", total, items, summary)
	}
	if items[0].RequirementNumber != open.RequirementNumber {
		t.Fatalf("unexpected visible requirement %+v", items[0])
	}
}

func TestCoverageApplicationLegacyItemModeStillReadable(t *testing.T) {
	f := newTestCoverageApplicationFixture(t)
	var itemTypeID int
	if err := f.db.QueryRow(`SELECT id FROM item_types ORDER BY id LIMIT 1`).Scan(&itemTypeID); err != nil {
		t.Fatal(err)
	}
	var statusID int
	if err := f.db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, item_type_id, status_id, creator_id, last_active_at)
		VALUES (1, 1, 'Legacy requirement', '', 'a0', ?, ?, 1, CURRENT_TIMESTAMP)
	`, itemTypeID, statusID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO test_coverage_configurations (workspace_id, requirement_item_type_ids, requirement_types)
		VALUES (?, ?, NULL)
	`, f.wsID, fmt.Sprintf("[%d]", itemTypeID)); err != nil {
		t.Fatal(err)
	}

	summary, err := f.app.CoverageSummary(f.adminID, TestCoverageScope{WorkspaceID: f.wsID})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 1 {
		t.Fatalf("expected one legacy requirement, got %+v", summary)
	}

	items, total, _, err := f.app.CoverageRequirements(f.adminID, TestCoverageScope{WorkspaceID: f.wsID}, TestCoverageRequirementsFilter{
		Limit:  50,
		Offset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ItemID == 0 {
		t.Fatalf("expected legacy item coverage row, got total=%d items=%+v", total, items)
	}
}
