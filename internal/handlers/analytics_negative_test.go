package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"windshift/internal/repository"
	"windshift/internal/services"
)

// Regression test for docs/bughunt1.md Run 2 finding #4.
//
// Today the test fails because AnalyticsHandler.GetAnalytics validates the
// caller's permission only against the path workspaceID — if collection_id
// points at a collection in a different workspace, the AnalyticsService picks
// up that collection's workspace_id and computes panels for it, leaking
// aggregate data. The post-fix handler must short-circuit to 404 when the
// collection's workspace does not match the path workspace.

// Finding #4: collection in a foreign workspace must be rejected (404).
func TestAnalyticsHandler_GetAnalytics_RejectsCrossWorkspaceCollection(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999) // other user; gates W2 so it isn't an open workspace

	// W1: open (no role assignments) → caller has implicit view.
	// W2: gated (assigning ANY workspace role flips it into gated mode per
	//     the "workspace permissions open by default" invariant). User 1
	//     has no role on W2, so any attempt to read W2 should be denied.
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, key, active) VALUES (1, 'W1', 'W1', 1), (2, 'W2', 'W2', 1)`); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	var adminRoleID int
	if err := db.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Administrator'`).Scan(&adminRoleID); err != nil {
		t.Fatalf("look up Administrator role: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_at)
		VALUES (999, 2, ?, CURRENT_TIMESTAMP)
	`, adminRoleID); err != nil {
		t.Fatalf("gate W2 by assigning a role to a stranger: %v", err)
	}

	// Collection C1 lives in W2.
	if _, err := db.Exec(`
		INSERT INTO collections (id, name, ql_query, workspace_id)
		VALUES (1, 'C1 in W2', '', 2)
	`); err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	permService := newNegativeTestPermissionService(t, db)
	analyticsService := services.NewAnalyticsService(db)
	keyCache := NewWorkspaceKeyCache(repository.NewWorkspaceRepository(db))
	handler := NewAnalyticsHandler(analyticsService, permService, keyCache)

	req := authedRequest(http.MethodGet, "/api/workspaces/1/analytics?collection_id=1", userID, nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	handler.GetAnalytics(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("got 200 OK for a collection that lives in a workspace the caller has no view on; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}
}
