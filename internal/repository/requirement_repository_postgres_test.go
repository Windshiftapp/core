package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/testdb"
)

type requirementRepoPostgresFixture struct {
	db       database.Database
	repo     *RequirementRepository
	userID   int
	sourceWS int
	otherWS  int
}

func newRequirementRepoPostgresFixture(t *testing.T) *requirementRepoPostgresFixture {
	t.Helper()
	db := testdb.OpenPostgres(t)
	t.Cleanup(func() { _ = db.Close() })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID int
	if err := db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name)
		VALUES (?, ?, 'Req', 'Repo')
		RETURNING id
	`, "req-repo-pg-"+suffix+"@example.test", "req-repo-pg-"+suffix).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var sourceWS int
	if err := db.QueryRow(`
		INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
	`, "Source "+suffix, "SRC"+suffix).Scan(&sourceWS); err != nil {
		t.Fatal(err)
	}
	var otherWS int
	if err := db.QueryRow(`
		INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
	`, "Other "+suffix, "OPS"+suffix).Scan(&otherWS); err != nil {
		t.Fatal(err)
	}
	return &requirementRepoPostgresFixture{
		db:       db,
		repo:     NewRequirementRepository(db),
		userID:   userID,
		sourceWS: sourceWS,
		otherWS:  otherWS,
	}
}

func (f *requirementRepoPostgresFixture) insertPage(t *testing.T, workspaceID int, title string) int {
	t.Helper()
	var id int
	if err := f.db.QueryRow(`
		INSERT INTO pages (
			workspace_id, title, slug, metadata, content, content_hash, excerpt,
			created_by, inherit_permissions, path, depth, created_at, updated_at
		) VALUES (?, ?, ?, '{}', '', '', '', ?, true, '/', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, workspaceID, title, title, f.userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRequirementInsertRejectsWorkspaceMismatchPostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	pageID := f.insertPage(t, f.sourceWS, "Mismatch")

	err := database.WithTx(f.db, func(tx database.Tx) error {
		_, err := f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       f.otherWS,
			RequirementNumber: 1,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         f.userID,
		})
		return err
	})
	if err == nil {
		t.Fatal("expected composite foreign key to reject a page/workspace mismatch")
	}
}

func TestRequirementNumbersAreNotReusedAfterDeletePostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	first := f.insertPage(t, f.sourceWS, "First")
	second := f.insertPage(t, f.sourceWS, "Second")
	third := f.insertPage(t, f.sourceWS, "Third")

	insert := func(pageID int) int {
		t.Helper()
		var number int
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, f.sourceWS)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       f.sourceWS,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         f.userID,
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

func TestRequirementAllocateRollsBackWithTransactionPostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	pageID := f.insertPage(t, f.sourceWS, "Rollback")

	err := database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, f.sourceWS)
		if err != nil {
			return err
		}
		if _, err := f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       f.sourceWS,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         f.userID,
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM requirements WHERE workspace_id = ?`, f.sourceWS).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("requirement row survived rollback: %d", count)
	}
	var sequences int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM requirement_sequences WHERE workspace_id = ?`, f.sourceWS).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if sequences != 0 {
		t.Fatalf("sequence row survived rollback: %d", sequences)
	}

	var number int
	err = database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, f.sourceWS)
		if err != nil {
			return err
		}
		number = n
		_, err = f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       f.sourceWS,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         f.userID,
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

func TestRequirementListByWorkspaceExcludesArchivedPagesPostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	livePage := f.insertPage(t, f.sourceWS, "Live")
	archivedPage := f.insertPage(t, f.sourceWS, "Archived")
	if _, err := f.db.ExecWrite(`UPDATE pages SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, archivedPage); err != nil {
		t.Fatal(err)
	}

	insert := func(pageID int) {
		t.Helper()
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, f.sourceWS)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       f.sourceWS,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         f.userID,
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(livePage)
	insert(archivedPage)

	rows, err := f.repo.ListByWorkspace(f.sourceWS, RequirementListFilter{ExcludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PageTitle != "Live" {
		t.Fatalf("expected only live requirement, got %+v", rows)
	}
}

func TestRequirementUpdateTxPersistsMutableFieldsPostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	pageID := f.insertPage(t, f.sourceWS, "Mutable")
	var reqID int
	err := database.WithTx(f.db, func(tx database.Tx) error {
		n, err := f.repo.NextNumberTx(tx, f.sourceWS)
		if err != nil {
			return err
		}
		reqID, err = f.repo.InsertTx(tx, &models.Requirement{
			PageID:            pageID,
			WorkspaceID:       f.sourceWS,
			RequirementNumber: n,
			RequirementType:   models.RequirementTypeUseCase,
			Status:            models.RequirementStatusDraft,
			CreatedBy:         f.userID,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	ownerID := f.userID
	err = database.WithTx(f.db, func(tx database.Tx) error {
		return f.repo.UpdateTx(tx, reqID, RequirementUpdatePatch{
			RequirementType: models.RequirementTypeBusinessRule,
			Status:          models.RequirementStatusApproved,
			OwnerID:         &ownerID,
		}, f.userID)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.GetByWorkspaceAndNumber(f.sourceWS, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequirementType != models.RequirementTypeBusinessRule ||
		got.Status != models.RequirementStatusApproved ||
		got.OwnerID == nil || *got.OwnerID != ownerID {
		t.Fatalf("unexpected requirement after update: %+v", got)
	}
}

func TestRequirementListByWorkspaceLinkFiltersAndCountsPostgres(t *testing.T) {
	f := newRequirementRepoPostgresFixture(t)
	linkedPage := f.insertPage(t, f.sourceWS, "Linked")
	barePage := f.insertPage(t, f.sourceWS, "Bare")
	testedPage := f.insertPage(t, f.sourceWS, "Tested")

	insertRequirement := func(pageID int) {
		t.Helper()
		err := database.WithTx(f.db, func(tx database.Tx) error {
			n, err := f.repo.NextNumberTx(tx, f.sourceWS)
			if err != nil {
				return err
			}
			_, err = f.repo.InsertTx(tx, &models.Requirement{
				PageID:            pageID,
				WorkspaceID:       f.sourceWS,
				RequirementNumber: n,
				RequirementType:   models.RequirementTypeUseCase,
				Status:            models.RequirementStatusDraft,
				CreatedBy:         f.userID,
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, pageID := range []int{linkedPage, barePage, testedPage} {
		insertRequirement(pageID)
	}

	var itemID int
	var statusID int
	if err := f.db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (?, 1, 'Linked item', '', 'a0', ?, ?, CURRENT_TIMESTAMP)
		RETURNING id
	`, f.sourceWS, statusID, f.userID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	var pageLinkTypeID int
	if err := f.db.QueryRow(`SELECT id FROM link_types WHERE name = 'Page'`).Scan(&pageLinkTypeID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by)
		VALUES (?, 'item', ?, 'page', ?, ?)
	`, pageLinkTypeID, itemID, linkedPage, f.userID); err != nil {
		t.Fatal(err)
	}

	var testsLinkTypeID int
	if err := f.db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = 'tests'`).Scan(&testsLinkTypeID); err != nil {
		t.Fatal(err)
	}
	var testCaseID int
	if err := f.db.QueryRow(`
		INSERT INTO test_cases (workspace_id, title, name)
		VALUES (?, 'Login test', 'Login test')
		RETURNING id
	`, f.sourceWS).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by)
		VALUES (?, 'page', ?, 'test_case', ?, ?)
	`, testsLinkTypeID, testedPage, testCaseID, f.userID); err != nil {
		t.Fatal(err)
	}

	hasItems := true
	rows, err := f.repo.ListByWorkspace(f.sourceWS, RequirementListFilter{ExcludeArchived: true, HasItemLinks: &hasItems})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requirement.PageID != linkedPage || rows[0].LinkedItemCount != 1 {
		t.Fatalf("expected one item-linked requirement, got %+v", rows)
	}

	hasTests := true
	rows, err = f.repo.ListByWorkspace(f.sourceWS, RequirementListFilter{ExcludeArchived: true, HasTestLinks: &hasTests})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requirement.PageID != testedPage || rows[0].LinkedTestCount != 1 {
		t.Fatalf("expected one test-linked requirement, got %+v", rows)
	}
}
