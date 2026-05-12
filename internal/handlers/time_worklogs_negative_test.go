package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// Regression tests for docs/bughunt1.md Run 2 findings #1 and #2.
//
// Today these tests fail because:
//  • Finding #1: TimeWorklogHandler.GetAll/Get return worklogs for projects
//    the caller can't access, only stripping item-related fields.
//  • Finding #2: TimeWorklogHandler.Update accepts a project_id switch
//    without checking CanBookTimeOnProject on the new project.
//
// They pass once the handler applies the fixes described in the plan.

// seedTwoTimeProjectsForUser inserts the standard fixture used by all three
// tests: one open-access project (no managers, no members) and one project
// that is member-restricted to a stranger.
func seedTwoTimeProjectsForUser(t *testing.T, db database.Database) (openPID, restrictedPID int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO customer_organisations (id, name) VALUES (1, 'Acme') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO time_projects (id, customer_id, name, description) VALUES (1, 1, 'Open', 'open project'), (2, 1, 'Restricted', 'restricted project') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	// Restricted project: stranger (user_id 999) is the only listed member AND
	// manager. Both lists must be populated so TimePermissionService treats P2
	// as fully restricted — with only one list populated, the "no X configured
	// → open to all" branch in IsTimeProjectManager / CanBookTimeOnProject
	// would let the caller in via the unpopulated axis.
	if _, err := db.Exec(`INSERT INTO time_project_members (project_id, member_type, member_id) VALUES (2, 'user', 999) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed members: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO time_project_managers (project_id, manager_type, manager_id) VALUES (2, 'user', 999) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed managers: %v", err)
	}
	return 1, 2
}

func insertNegativeWorklog(t *testing.T, db database.Database, projectID, userID int, description string) int {
	t.Helper()
	now := time.Now().Unix()
	res, err := db.Exec(`
		INSERT INTO time_worklogs (project_id, customer_id, user_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, 1, ?, ?, ?, ?, ?, 60, ?, ?)
	`, projectID, userID, description, now, now, now+3600, now, now)
	if err != nil {
		t.Fatalf("insert worklog on project %d: %v", projectID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return int(id)
}

func newWorklogTestHandler(t *testing.T, db database.Database) *TimeWorklogHandler {
	t.Helper()
	permService := newNegativeTestPermissionService(t, db)
	timePermService := services.NewTimePermissionService(db, permService)
	return NewTimeWorklogHandler(db, permService, timePermService)
}

// Finding #1, GetAll arm: GET /api/worklogs must not include rows from
// projects the user can't access. Pre-fix the handler returns every worklog.
func TestTimeWorklogHandler_GetAll_HidesInaccessibleProjects(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999) // stranger who owns the restricted-project worklog (FK target)
	openPID, restrictedPID := seedTwoTimeProjectsForUser(t, db)

	insertNegativeWorklog(t, db, openPID, userID, "log on open project")
	insertNegativeWorklog(t, db, restrictedPID, 999, "log on restricted project")

	handler := newWorklogTestHandler(t, db)

	req := authedRequest(http.MethodGet, "/api/worklogs", userID, nil)
	rr := httptest.NewRecorder()
	handler.GetAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
	var resp []models.Worklog
	decodeJSONBody(t, rr, &resp)

	if len(resp) != 1 {
		t.Fatalf("got %d worklogs, want 1 (only the accessible-project worklog); pre-fix this returns both. body=%s", len(resp), rr.Body.String())
	}
	if resp[0].ProjectID != openPID {
		t.Errorf("returned worklog has project_id=%d, want %d", resp[0].ProjectID, openPID)
	}
}

// Finding #1, Get arm: GET /api/worklogs/{id} must return 404 when the
// worklog belongs to a project the user can't access. Pre-fix the handler
// returns the row (only item-related fields are blanked).
func TestTimeWorklogHandler_Get_NotFoundOnInaccessibleProject(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999) // stranger who owns the restricted-project worklog (FK target)
	_, restrictedPID := seedTwoTimeProjectsForUser(t, db)

	wlID := insertNegativeWorklog(t, db, restrictedPID, 999, "log on restricted project")

	handler := newWorklogTestHandler(t, db)

	req := authedRequest(http.MethodGet, fmt.Sprintf("/api/worklogs/%d", wlID), userID, nil)
	req.SetPathValue("id", strconv.Itoa(wlID))
	rr := httptest.NewRecorder()
	handler.Get(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("got 200 OK for a worklog on a project the user can't access; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}
}

// Finding #2: PUT /api/worklogs/{id} must reject a project_id switch to a
// project the caller can't book on, even when they retain edit access on the
// existing worklog. Pre-fix the handler decodes the request and writes the
// update without any check on the new project_id.
func TestTimeWorklogHandler_Update_RejectsCrossProjectMove(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	openPID, restrictedPID := seedTwoTimeProjectsForUser(t, db)

	wlID := insertNegativeWorklog(t, db, openPID, userID, "own worklog on open project")

	handler := newWorklogTestHandler(t, db)

	body := WorklogRequest{
		ProjectID:     restrictedPID, // user cannot book on this project
		Description:   "moved",
		Date:          time.Now().Format("2006-01-02"),
		DurationInput: "1h",
	}
	req := authedRequest(http.MethodPut, fmt.Sprintf("/api/worklogs/%d", wlID), userID, body)
	req.SetPathValue("id", strconv.Itoa(wlID))
	rr := httptest.NewRecorder()
	handler.Update(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("Update succeeded (200) when moving worklog to a project the user can't book on; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 403 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}

	// Belt-and-braces: confirm the worklog's project_id was NOT changed.
	var got int
	if err := db.QueryRow(`SELECT project_id FROM time_worklogs WHERE id = ?`, wlID).Scan(&got); err != nil {
		t.Fatalf("re-read worklog: %v", err)
	}
	if got != openPID {
		t.Errorf("worklog project_id was rewritten to %d despite the rejection; want %d", got, openPID)
	}
}
