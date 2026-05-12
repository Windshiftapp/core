package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// Regression tests for docs/bughunt1.md Run 2 finding #3.
//
// Today these tests fail because TimeProjectHandler.GetByCustomer and
// GetByWorkspace dispatch through the bare respondTimeProjects helper, which
// has no permission filter — unlike GetAll which inlines the
// GetAccessibleProjects IN-list. The fix centralizes the filter inside
// respondTimeProjects (or its callers), at which point only accessible
// projects appear in either response.

func newTimeProjectHandler(t *testing.T, db database.Database) *TimeProjectHandler {
	t.Helper()
	permService := newNegativeTestPermissionService(t, db)
	timePermService := services.NewTimePermissionService(db, permService)
	keyCache := NewWorkspaceKeyCache(repository.NewWorkspaceRepository(db))
	return NewTimeProjectHandler(db, timePermService, nil, keyCache)
}

// Finding #3, GetByCustomer arm: GET /api/customers/{id}/time-projects must
// omit projects the caller cannot view.
func TestTimeProjectHandler_GetByCustomer_HidesInaccessible(t *testing.T) {
	const (
		userID     = 1
		customerID = 1
	)
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	openPID, _ := seedTwoTimeProjectsForUser(t, db)

	handler := newTimeProjectHandler(t, db)

	req := authedRequest(http.MethodGet, "/api/customers/1/time-projects", userID, nil)
	req.SetPathValue("id", strconv.Itoa(customerID))
	rr := httptest.NewRecorder()
	handler.GetByCustomer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
	var resp []models.TimeProject
	decodeJSONBody(t, rr, &resp)

	if len(resp) != 1 {
		t.Fatalf("got %d projects, want 1 (only the open project); pre-fix this returns both. body=%s", len(resp), rr.Body.String())
	}
	if resp[0].ID != openPID {
		t.Errorf("returned project id=%d, want %d", resp[0].ID, openPID)
	}
}

// Finding #3, GetByWorkspace arm: GET /api/workspaces/{id}/time-projects
// must omit projects the caller cannot view. The current handler doesn't
// even authenticate; the post-fix path adds RequireAuth + accessibility
// filtering, so the test exercises both behaviors at once.
func TestTimeProjectHandler_GetByWorkspace_HidesInaccessible(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	openPID, _ := seedTwoTimeProjectsForUser(t, db)

	// Seed a workspace; do NOT seed a workspace_time_project_categories
	// row so the handler returns every project (its category-filter branch
	// is bypassed). That makes the response a clean canary for the missing
	// permission gate.
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, key, active) VALUES (1, 'WS', 'WS', 1)`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	handler := newTimeProjectHandler(t, db)

	req := authedRequest(http.MethodGet, "/api/workspaces/1/time-projects", userID, nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	handler.GetByWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
	var resp []models.TimeProject
	decodeJSONBody(t, rr, &resp)

	if len(resp) != 1 {
		t.Fatalf("got %d projects, want 1 (only the open project); pre-fix this returns both. body=%s", len(resp), rr.Body.String())
	}
	if resp[0].ID != openPID {
		t.Errorf("returned project id=%d, want %d", resp[0].ID, openPID)
	}
}
