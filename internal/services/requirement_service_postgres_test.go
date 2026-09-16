package services

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/testdb"
)

type requirementServicePostgresFixture struct {
	db     database.Database
	pages  *PageService
	reqs   *RequirementService
	userID int
	source int
	dest   int
}

func newRequirementServicePostgresFixture(t *testing.T) *requirementServicePostgresFixture {
	t.Helper()
	db := testdb.OpenPostgres(t)
	t.Cleanup(func() { _ = db.Close() })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID int
	if err := db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name)
		VALUES (?, ?, 'Req', 'Service')
		RETURNING id
	`, "req-svc-pg-"+suffix+"@example.test", "req-svc-pg-"+suffix).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var source int
	if err := db.QueryRow(`
		INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
	`, "Source "+suffix, "SRC"+suffix).Scan(&source); err != nil {
		t.Fatal(err)
	}
	var dest int
	if err := db.QueryRow(`
		INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
	`, "Dest "+suffix, "DST"+suffix).Scan(&dest); err != nil {
		t.Fatal(err)
	}
	pages := NewPageService(db)
	return &requirementServicePostgresFixture{
		db:     db,
		pages:  pages,
		reqs:   NewRequirementService(db, pages),
		userID: userID,
		source: source,
		dest:   dest,
	}
}

func (f *requirementServicePostgresFixture) createPage(t *testing.T, workspaceID int, title string) *models.Page {
	t.Helper()
	page, err := f.pages.Create(f.userID, CreatePageInput{WorkspaceID: workspaceID, Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestPromoteConcurrentUniqueNumbersPostgres(t *testing.T) {
	f := newRequirementServicePostgresFixture(t)
	const n = 8
	pages := make([]*models.Page, n)
	for i := range pages {
		pages[i] = f.createPage(t, f.source, fmt.Sprintf("Concurrent-%d", i))
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

func TestCreateRequirementIsAtomicPostgres(t *testing.T) {
	f := newRequirementServicePostgresFixture(t)
	req, err := f.reqs.Create(f.userID, CreateRequirementInput{
		WorkspaceID:     f.source,
		Title:           "New requirement",
		Content:         "Body",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.RequirementNumber != 1 || req.PageID == 0 {
		t.Fatalf("unexpected requirement: %+v", req)
	}
	page, err := f.pages.GetByID(req.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "New requirement" {
		t.Fatalf("page title %q", page.Title)
	}
}

func TestMoveAcrossWorkspaceBlocksRequirementBackedPagesPostgres(t *testing.T) {
	f := newRequirementServicePostgresFixture(t)
	page := f.createPage(t, f.source, "Requirement")
	if _, err := f.reqs.Promote(f.userID, page.ID, models.RequirementTypeUseCase, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pages.MoveAcrossWorkspace(f.userID, page.ID, f.dest, nil, nil, nil); !errors.Is(err, ErrRequirementCrossWorkspaceMove) {
		t.Fatalf("expected cross-workspace block, got %v", err)
	}
}

func TestPromoteAllocatesStableWorkspaceNumbersPostgres(t *testing.T) {
	f := newRequirementServicePostgresFixture(t)
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

func TestUpdateRequirementWritesPerFieldHistoryPostgres(t *testing.T) {
	f := newRequirementServicePostgresFixture(t)
	created, err := f.reqs.Create(f.userID, CreateRequirementInput{
		WorkspaceID:     f.source,
		Title:           "Mutable",
		RequirementType: models.RequirementTypeUseCase,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerID := f.userID
	status := models.RequirementStatusInReview
	updated, err := f.reqs.Update(f.userID, f.source, created.RequirementNumber, RequirementUpdateInput{
		Status:     &status,
		OwnerIDSet: true,
		OwnerID:    &ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.RequirementStatusInReview || updated.OwnerID == nil || *updated.OwnerID != ownerID {
		t.Fatalf("unexpected update: %+v", updated)
	}

	rows, err := f.db.Query(`SELECT field_name FROM requirement_history WHERE requirement_id = ? ORDER BY id`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var fields []string
	for rows.Next() {
		var field string
		if err := rows.Scan(&field); err != nil {
			t.Fatal(err)
		}
		fields = append(fields, field)
	}
	if len(fields) != 3 {
		t.Fatalf("expected promoted + status + owner history, got %v", fields)
	}
	if fields[0] != models.RequirementHistoryFieldPromoted {
		t.Fatalf("first history field %q", fields[0])
	}
}
