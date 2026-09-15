package repository

import (
	"errors"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

type requirementRepoFixture struct {
	db   database.Database
	repo *RequirementRepository
}

func newRequirementRepoFixture(t *testing.T) *requirementRepoFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "requirements-repository.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'req-repo@example.test', 'req-repo', 'Req', 'Repo')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'Source', 'CRM')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (2, 'Other', 'OPS')`); err != nil {
		t.Fatal(err)
	}
	return &requirementRepoFixture{db: db, repo: NewRequirementRepository(db)}
}

func (f *requirementRepoFixture) insertPage(t *testing.T, workspaceID int, title string) int {
	t.Helper()
	var id int
	if err := f.db.QueryRow(`
		INSERT INTO pages (
			workspace_id, title, slug, metadata, content, content_hash, excerpt,
			created_by, inherit_permissions, path, depth, created_at, updated_at
		) VALUES (?, ?, ?, '{}', '', '', '', 1, true, '/', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, workspaceID, title, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRequirementInsertRejectsWorkspaceMismatch(t *testing.T) {
	f := newRequirementRepoFixture(t)
	pageID := f.insertPage(t, 1, "Mismatch")

	err := database.WithTx(f.db, func(tx database.Tx) error {
		_, err := f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       2,
			RequirementNumber: 1,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         1,
		})
		return err
	})
	if err == nil {
		t.Fatal("expected composite foreign key to reject a page/workspace mismatch")
	}
}

func TestRequirementNumbersAreNotReusedAfterDelete(t *testing.T) {
	f := newRequirementRepoFixture(t)
	first := f.insertPage(t, 1, "First")
	second := f.insertPage(t, 1, "Second")
	third := f.insertPage(t, 1, "Third")

	insert := func(pageID int) int {
		t.Helper()
		var number int
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, 1)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       1,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         1,
			})
			number = n
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return number
	}

	if n := insert(first); n != 1 {
		t.Fatalf("first number: got %d", n)
	}
	if n := insert(second); n != 2 {
		t.Fatalf("second number: got %d", n)
	}
	if _, err := f.db.ExecWrite(`DELETE FROM requirements WHERE page_id = ?`, first); err != nil {
		t.Fatal(err)
	}
	if n := insert(third); n != 3 {
		t.Fatalf("deleted numbers must not be reused, got %d", n)
	}
}

func TestRequirementAllocateRollsBackWithTransaction(t *testing.T) {
	f := newRequirementRepoFixture(t)
	pageID := f.insertPage(t, 1, "Rollback")

	err := database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, 1)
		if err != nil {
			return err
		}
		if _, err := f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       1,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         1,
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM requirements`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("requirement row survived rollback: %d", count)
	}
	var sequences int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM requirement_sequences`).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if sequences != 0 {
		t.Fatalf("sequence row survived rollback: %d", sequences)
	}

	var number int
	err = database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, 1)
		if err != nil {
			return err
		}
		number = n
		_, err = f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       1,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         1,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if number != 1 {
		t.Fatalf("after rollback next number should be 1, got %d", number)
	}
}

func TestRequirementListByWorkspaceExcludesArchivedPages(t *testing.T) {
	f := newRequirementRepoFixture(t)
	livePage := f.insertPage(t, 1, "Live")
	archivedPage := f.insertPage(t, 1, "Archived")
	if _, err := f.db.ExecWrite(`UPDATE pages SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, archivedPage); err != nil {
		t.Fatal(err)
	}

	insert := func(pageID int) {
		t.Helper()
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, 1)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       1,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         1,
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(livePage)
	insert(archivedPage)

	rows, err := f.repo.ListByWorkspace(1, RequirementListFilter{ExcludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PageTitle != "Live" {
		t.Fatalf("expected only live requirement, got %+v", rows)
	}
}

func TestRequirementUpdateTxPersistsMutableFields(t *testing.T) {
	f := newRequirementRepoFixture(t)
	pageID := f.insertPage(t, 1, "Mutable")
	var reqID int
	err := database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, 1)
		if err != nil {
			return err
		}
		reqID, err = f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       1,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         1,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	ownerID := 1
	err = database.WithTx(f.db, func(tx database.Tx) error {
		return f.repo.UpdateTx(tx, reqID, RequirementUpdatePatch{
			RequirementType: models.RequirementTypeBusinessRule,
			Status:          models.RequirementStatusApproved,
			OwnerID:         &ownerID,
		}, 1)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.GetByWorkspaceAndNumber(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequirementType != models.RequirementTypeBusinessRule ||
		got.Status != models.RequirementStatusApproved ||
		got.OwnerID == nil || *got.OwnerID != ownerID {
		t.Fatalf("unexpected requirement after update: %+v", got)
	}
}

func TestRequirementListByWorkspaceLinkFiltersAndCounts(t *testing.T) {
	f := newRequirementRepoFixture(t)
	linkedPage := f.insertPage(t, 1, "Linked")
	barePage := f.insertPage(t, 1, "Bare")
	testedPage := f.insertPage(t, 1, "Tested")
	labeledPage := f.insertPage(t, 1, "Labeled")

	insertRequirement := func(pageID int) {
		t.Helper()
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, 1)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       1,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         1,
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, pageID := range []int{linkedPage, barePage, testedPage, labeledPage} {
		insertRequirement(pageID)
	}

	var itemID int
	var statusID int
	if err := f.db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (1, 1, 'Linked item', '', 'a0', ?, 1, CURRENT_TIMESTAMP)
		RETURNING id
	`, statusID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	var pageLinkTypeID int
	if err := f.db.QueryRow(`SELECT id FROM link_types WHERE name = 'Page'`).Scan(&pageLinkTypeID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by)
		VALUES (?, 'item', ?, 'page', ?, 1)
	`, pageLinkTypeID, itemID, linkedPage); err != nil {
		t.Fatal(err)
	}

	var testsLinkTypeID int
	if err := f.db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = 'tests'`).Scan(&testsLinkTypeID); err != nil {
		t.Fatal(err)
	}

	var testCaseID int
	if err := f.db.QueryRow(`
		INSERT INTO test_cases (workspace_id, title, name)
		VALUES (1, 'Login test', 'Login test')
		RETURNING id
	`).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by)
		VALUES (?, 'page', ?, 'test_case', ?, 1)
	`, testsLinkTypeID, testedPage, testCaseID); err != nil {
		t.Fatal(err)
	}

	var labelID int
	if err := f.db.QueryRow(`
		INSERT INTO page_labels (name, color, workspace_id)
		VALUES ('Spec', '#3B82F6', 1)
		RETURNING id
	`).Scan(&labelID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO page_label_assignments (page_id, page_label_id)
		VALUES (?, ?)
	`, labeledPage, labelID); err != nil {
		t.Fatal(err)
	}

	hasItems := true
	rows, err := f.repo.ListByWorkspace(1, RequirementListFilter{ExcludeArchived: true, HasItemLinks: &hasItems})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requirement.PageID != linkedPage || rows[0].LinkedItemCount != 1 {
		t.Fatalf("expected one item-linked requirement, got %+v", rows)
	}

	hasTests := true
	rows, err = f.repo.ListByWorkspace(1, RequirementListFilter{ExcludeArchived: true, HasTestLinks: &hasTests})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requirement.PageID != testedPage || rows[0].LinkedTestCount != 1 {
		t.Fatalf("expected one test-linked requirement, got %+v", rows)
	}

	noTests := false
	rows, err = f.repo.ListByWorkspace(1, RequirementListFilter{ExcludeArchived: true, HasTestLinks: &noTests})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected three requirements without test links, got %d", len(rows))
	}

	rows, err = f.repo.ListByWorkspace(1, RequirementListFilter{ExcludeArchived: true, LabelIDs: []int{labelID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requirement.PageID != labeledPage {
		t.Fatalf("expected one labeled requirement, got %+v", rows)
	}

	counts, err := f.repo.GetLinkCountsByPageID(linkedPage)
	if err != nil {
		t.Fatal(err)
	}
	if counts.LinkedItemCount != 1 || counts.LinkedTestCount != 0 {
		t.Fatalf("unexpected link counts: %+v", counts)
	}
}
