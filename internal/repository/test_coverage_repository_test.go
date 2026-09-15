package repository

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

type testCoverageRepoFixture struct {
	db   database.Database
	repo *TestCoverageRepository
	reqs *RequirementRepository
}

func newTestCoverageRepoFixture(t *testing.T) *testCoverageRepoFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "test-coverage-repository.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'coverage@example.test', 'coverage', 'Cov', 'Repo')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'CRM', 'CRM')`); err != nil {
		t.Fatal(err)
	}
	return &testCoverageRepoFixture{
		db:   db,
		repo: NewTestCoverageRepository(db),
		reqs: NewRequirementRepository(db),
	}
}

func (f *testCoverageRepoFixture) insertPage(t *testing.T, title string, archived bool) int {
	t.Helper()
	var id int
	if err := f.db.QueryRow(`
		INSERT INTO pages (
			workspace_id, title, slug, metadata, content, content_hash, excerpt,
			created_by, inherit_permissions, path, depth, created_at, updated_at
		) VALUES (1, ?, ?, '{}', '', '', '', 1, true, '/', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, title, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if archived {
		if _, err := f.db.ExecWrite(`UPDATE pages SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (f *testCoverageRepoFixture) insertRequirement(t *testing.T, pageID int, reqType string) {
	t.Helper()
	err := database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.reqs.NextNumberTx(tx, 1)
		if err != nil {
			return err
		}
		_, err = f.reqs.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       1,
			RequirementNumber: n,
			RequirementType:   reqType,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         1,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *testCoverageRepoFixture) linkPageToTestCase(t *testing.T, pageID int) {
	t.Helper()
	var testsLinkTypeID int
	if err := f.db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = 'tests'`).Scan(&testsLinkTypeID); err != nil {
		t.Fatal(err)
	}
	var testCaseID int
	if err := f.db.QueryRow(`
		INSERT INTO test_cases (workspace_id, title, name)
		VALUES (1, 'Coverage test', 'Coverage test')
		RETURNING id
	`).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by)
		VALUES (?, 'page', ?, 'test_case', ?, 1)
	`, testsLinkTypeID, pageID, testCaseID); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageConfigCreateUsesRequirementTypesMode(t *testing.T) {
	f := newTestCoverageRepoFixture(t)
	config, err := f.repo.CreateConfigForWorkspace(1, CoverageConfigWrite{
		RequirementTypes: []string{models.RequirementTypeUseCase},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !config.UsesPageBackedRequirements() {
		t.Fatalf("expected page-backed config, got %+v", config)
	}
	if len(config.RequirementItemTypeIDs) != 0 {
		t.Fatalf("legacy item type ids should be empty, got %+v", config.RequirementItemTypeIDs)
	}
}

func TestCoverageConfigUpdateSwitchesToRequirementTypes(t *testing.T) {
	f := newTestCoverageRepoFixture(t)
	config, err := f.repo.CreateConfigForWorkspace(1, CoverageConfigWrite{
		RequirementItemTypeIDs: []int{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.repo.UpdateConfig(config.ID, CoverageConfigWrite{
		RequirementTypes: []string{models.RequirementTypeBusinessRule},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UsesPageBackedRequirements() {
		t.Fatalf("expected page-backed config after update, got %+v", updated)
	}
	if len(updated.RequirementItemTypeIDs) != 0 {
		t.Fatalf("legacy item type ids should be cleared, got %+v", updated.RequirementItemTypeIDs)
	}
}

func TestListAllPageBackedRequirementsFiltersTypeAndCoverage(t *testing.T) {
	f := newTestCoverageRepoFixture(t)
	coveredPage := f.insertPage(t, "Covered", false)
	barePage := f.insertPage(t, "Bare", false)
	archivedPage := f.insertPage(t, "Archived", true)
	otherTypePage := f.insertPage(t, "Other type", false)

	f.insertRequirement(t, coveredPage, models.RequirementTypeUseCase)
	f.insertRequirement(t, barePage, models.RequirementTypeUseCase)
	f.insertRequirement(t, archivedPage, models.RequirementTypeUseCase)
	f.insertRequirement(t, otherTypePage, models.RequirementTypeBusinessRule)
	f.linkPageToTestCase(t, coveredPage)

	items, err := f.repo.ListAllPageBackedRequirements(1, []string{models.RequirementTypeUseCase})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected live use cases only, got %+v", items)
	}

	covered := 0
	for _, item := range items {
		if item.RequirementKey == "" {
			t.Fatalf("missing requirement key on %+v", item)
		}
		if item.IsCovered {
			covered++
		}
		if item.Title == "Archived" {
			t.Fatalf("archived page should be excluded, got %+v", item)
		}
	}
	if covered != 1 {
		t.Fatalf("expected one covered requirement, got %d", covered)
	}
}
