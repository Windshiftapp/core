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
