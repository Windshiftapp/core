package services

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type requirementServiceFixture struct {
	db     database.Database
	pages  *PageService
	reqs   *RequirementService
	userID int
	source int
	dest   int
}

func newRequirementServiceFixture(t *testing.T) *requirementServiceFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "requirements-service.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'req-svc@example.test', 'req-svc', 'Req', 'Service')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'Source', 'CRM')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (2, 'Other', 'OPS')`); err != nil {
		t.Fatal(err)
	}
	return &requirementServiceFixture{
		db:     db,
		pages:  NewPageService(db),
		reqs:   NewRequirementService(db),
		userID: 1,
		source: 1,
		dest:   2,
	}
}

func (f *requirementServiceFixture) createPage(t *testing.T, workspaceID int, title string) *models.Page {
	t.Helper()
	page, err := f.pages.Create(f.userID, CreatePageInput{WorkspaceID: workspaceID, Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestPromoteAllocatesStableWorkspaceNumbers(t *testing.T) {
	f := newRequirementServiceFixture(t)
	first := f.createPage(t, f.source, "First")
	second := f.createPage(t, f.source, "Second")

	got, err := f.reqs.Promote(f.userID, first.ID, models.RequirementTypeUseCase, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequirementNumber != 1 || got.WorkspaceID != first.WorkspaceID || got.PageID != first.ID {
		t.Fatalf("unexpected first requirement: %+v", got)
	}
	if got.Status != models.RequirementStatusDraft {
		t.Fatalf("empty status should default to draft, got %q", got.Status)
	}
	if models.FormatRequirementKey("CRM", got.RequirementNumber) != "CRM-DOC-1" {
		t.Fatalf("display key %s", models.FormatRequirementKey("CRM", got.RequirementNumber))
	}

	if _, err := f.reqs.Promote(f.userID, first.ID, models.RequirementTypeUseCase, "", nil); !errors.Is(err, ErrRequirementAlreadyExists) {
		t.Fatalf("second promote of same page: %v", err)
	}

	got, err = f.reqs.Promote(f.userID, second.ID, models.RequirementTypeBusinessRule, models.RequirementStatusInReview, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequirementNumber != 2 {
		t.Fatalf("second page should be 2, got %d", got.RequirementNumber)
	}

	var historyField string
	if err := f.db.QueryRow(`SELECT field_name FROM requirement_history WHERE requirement_id = ?`, got.ID).Scan(&historyField); err != nil {
		t.Fatal(err)
	}
	if historyField != models.RequirementHistoryFieldPromoted {
		t.Fatalf("history field %q", historyField)
	}
}

func TestPromoteIsWorkspaceScoped(t *testing.T) {
	f := newRequirementServiceFixture(t)
	sourcePage := f.createPage(t, f.source, "Source")
	destPage := f.createPage(t, f.dest, "Dest")

	sourceReq, err := f.reqs.Promote(f.userID, sourcePage.ID, models.RequirementTypeUseCase, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	destReq, err := f.reqs.Promote(f.userID, destPage.ID, models.RequirementTypeUseCase, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sourceReq.RequirementNumber != 1 || destReq.RequirementNumber != 1 {
		t.Fatalf("workspaces must number independently: %d and %d", sourceReq.RequirementNumber, destReq.RequirementNumber)
	}

	repo := repository.NewRequirementRepository(f.db)
	lookedUp, err := repo.GetByWorkspaceAndNumber(f.source, 1)
	if err != nil {
		t.Fatal(err)
	}
	if lookedUp.PageID != sourcePage.ID {
		t.Fatalf("lookup returned page %d", lookedUp.PageID)
	}
}

func TestPromoteRejectsInvalidType(t *testing.T) {
	f := newRequirementServiceFixture(t)
	page := f.createPage(t, f.source, "Invalid")
	if _, err := f.reqs.Promote(f.userID, page.ID, "not-a-type", "", nil); !errors.Is(err, ErrRequirementTypeInvalid) {
		t.Fatalf("got %v", err)
	}
	if _, err := f.reqs.Promote(f.userID, page.ID, models.RequirementTypeUseCase, "shipped", nil); !errors.Is(err, ErrRequirementStatusInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestPromoteConcurrentUniqueNumbers(t *testing.T) {
	f := newRequirementServiceFixture(t)
	const n = 8
	pages := make([]*models.Page, n)
	for i := range pages {
		pages[i] = f.createPage(t, f.source, "Concurrent")
	}

	numbers := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range pages {
		go func(i int) {
			defer wg.Done()
			req, err := f.reqs.Promote(f.userID, pages[i].ID, models.RequirementTypeUseCase, "", nil)
			errs[i] = err
			if err == nil {
				numbers[i] = req.RequirementNumber
			}
		}(i)
	}
	wg.Wait()

	seen := map[int]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("promote %d: %v", i, err)
		}
		if numbers[i] < 1 || numbers[i] > n || seen[numbers[i]] {
			t.Fatalf("duplicate or out of range numbers: %v", numbers)
		}
		seen[numbers[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique numbers, got %v", n, numbers)
	}
}

func TestMoveAcrossWorkspaceBlocksRequirementBackedPages(t *testing.T) {
	f := newRequirementServiceFixture(t)
	page := f.createPage(t, f.source, "Requirement")
	if _, err := f.reqs.Promote(f.userID, page.ID, models.RequirementTypeUseCase, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.MoveAcrossWorkspace(f.userID, page.ID, f.dest, nil, nil, nil); !errors.Is(err, ErrRequirementCrossWorkspaceMove) {
		t.Fatalf("expected cross-workspace block, got %v", err)
	}
}

func TestMoveAcrossWorkspaceAllowsOrdinaryPages(t *testing.T) {
	f := newRequirementServiceFixture(t)
	page := f.createPage(t, f.source, "Wiki")
	moved, err := f.pages.MoveAcrossWorkspace(f.userID, page.ID, f.dest, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if moved.WorkspaceID != f.dest {
		t.Fatalf("expected workspace %d, got %d", f.dest, moved.WorkspaceID)
	}
}
