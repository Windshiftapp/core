package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// Regression tests for docs/bughunt1.md Run 5 findings #2 and #3.
//
//  • #2: PUT .../test-runs/{runID}/results/{resultID} does not verify
//    `resultID` belongs to `runID`/{workspaceID}. A user with execute access
//    in their own workspace can submit their authorized run ID together with
//    a foreign `resultID` and update that other result.
//  • #3: POST .../test-runs accepts a `template_id` from a different
//    workspace. The service validates the set and assignee workspaces but
//    skips the template workspace.

// seedTwoWorkspacesWithTestData seeds two workspaces with one test set each
// containing one test case. Returns IDs the tests use.
type twoTestWorkspaces struct {
	ws1, ws2                int
	set1, set2              int
	testCase1, testCase2    int
	template1, template2    int
	run1, run2              int
	result1, result2        int
}

func seedTwoTestWorkspaces(t *testing.T, db database.Database) twoTestWorkspaces {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workspaces (id, name, key, active) VALUES (1, 'W1', 'W1', 1), (2, 'W2', 'W2', 1)
	`); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO test_cases (id, workspace_id, title, priority)
		VALUES
			(101, 1, 'Case W1', 'medium'),
			(102, 2, 'Case W2', 'medium')
	`); err != nil {
		t.Fatalf("seed test cases: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO test_sets (id, workspace_id, name, description, created_at, updated_at)
		VALUES (201, 1, 'Set W1', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		       (202, 2, 'Set W2', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed test sets: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO set_test_cases (set_id, test_case_id) VALUES (201, 101), (202, 102)
	`); err != nil {
		t.Fatalf("seed set_test_cases: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO test_run_templates (id, workspace_id, set_id, name) VALUES
			(301, 1, 201, 'Template W1'),
			(302, 2, 202, 'Template W2')
	`); err != nil {
		t.Fatalf("seed templates: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO test_runs (id, workspace_id, template_id, set_id, name) VALUES
			(401, 1, NULL, 201, 'Run A in W1'),
			(402, 1, NULL, 201, 'Run B in W1')
	`); err != nil {
		t.Fatalf("seed test_runs: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO test_results (id, run_id, test_case_id, status) VALUES
			(501, 401, 101, 'not_run'),
			(502, 402, 101, 'not_run')
	`); err != nil {
		t.Fatalf("seed test_results: %v", err)
	}

	return twoTestWorkspaces{
		ws1: 1, ws2: 2,
		set1: 201, set2: 202,
		testCase1: 101, testCase2: 102,
		template1: 301, template2: 302,
		run1: 401, run2: 402,
		result1: 501, result2: 502,
	}
}

func newTestRunHandler(t *testing.T, db database.Database) *TestRunHandler {
	t.Helper()
	service := services.NewTestRunService(db)
	testRunRepo := repository.NewTestRunRepository(db)
	itemRepo := repository.NewItemRepository(db)
	auditor := logger.NewAuditor(db)
	return NewTestRunHandlerWithPool(service, testRunRepo, itemRepo, auditor)
}

// R5-2: PUT /workspaces/{ws}/test-runs/{run1}/results/{result-of-run2} must
// not update result-of-run2 just because run1 belongs to {ws}.
func TestTestRunHandler_UpdateResult_RejectsResultFromDifferentRun(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	d := seedTwoTestWorkspaces(t, db)

	handler := newTestRunHandler(t, db)

	body := map[string]string{"status": "passed", "actual_result": "via mismatch", "notes": ""}
	req := authedRequest(http.MethodPut,
		fmt.Sprintf("/workspaces/%d/test-runs/%d/results/%d", d.ws1, d.run1, d.result2),
		userID, body)
	req.SetPathValue("workspaceId", strconv.Itoa(d.ws1))
	req.SetPathValue("id", strconv.Itoa(d.run1))
	req.SetPathValue("resultId", strconv.Itoa(d.result2))
	rr := httptest.NewRecorder()
	handler.UpdateResult(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("UpdateResult succeeded (200) when resultId belongs to a different run; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}

	// Belt-and-braces: result2 must still be in its original status.
	var status string
	if err := db.QueryRow(`SELECT status FROM test_results WHERE id = ?`, d.result2).Scan(&status); err != nil {
		t.Fatalf("re-read result: %v", err)
	}
	if status != "not_run" {
		t.Errorf("result2 status was rewritten to %q despite the rejection", status)
	}
}

// R5-3: POST /workspaces/{wsA}/test-runs with template_id belonging to wsB
// must be rejected.
func TestTestRunHandler_Create_RejectsCrossWorkspaceTemplate(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	d := seedTwoTestWorkspaces(t, db)

	handler := newTestRunHandler(t, db)

	body := map[string]interface{}{
		"name":        "Cross-WS run",
		"template_id": d.template2, // template lives in ws2
		"set_id":      d.set1,      // but the set is in ws1
	}
	req := authedRequest(http.MethodPost,
		fmt.Sprintf("/workspaces/%d/test-runs", d.ws1), userID, body)
	req.SetPathValue("workspaceId", strconv.Itoa(d.ws1))
	rr := httptest.NewRecorder()
	handler.Create(rr, req)

	if rr.Code == http.StatusCreated {
		t.Fatalf("Create succeeded (201) with a template from another workspace; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 404 / 400 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}
}
