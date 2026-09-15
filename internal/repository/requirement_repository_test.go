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
