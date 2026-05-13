package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

// Regression tests for docs/bughunt1.md Run 5 finding #1 (privilege split)
// and docs/bughunt10.md finding #3 (workspace-default writes raised to
// `workspace.admin`).
//
// The workspace arm now requires `workspace.admin` — `item.edit` is not
// enough. The board-configuration writes reshape columns, backlog, list/card
// fields, and roadmap for every viewer of the workspace, so this is an
// admin-only surface. The collection arm continues to require ownership
// (`created_by == currentUser.ID`); `is_public = true` does not grant write.
//
// A Viewer (`item.view`) must be rejected on both arms; that is what these
// tests assert.

const boardConfigJSONBody = `{
	"backlog_status_ids": [],
	"list_columns": [],
	"card_fields": [],
	"columns": []
}`

// seedWorkspaceWithViewerUser seeds workspace W1 and grants user U1 the
// Viewer role (item.view + item.comment) on it. A second user (UID=999) gets
// Administrator on W1 so the workspace itself is gated (not in open-by-default
// mode) — without that, HasWorkspacePermission would return true for any
// permission key on the open workspace.
func seedWorkspaceWithViewerUser(t *testing.T, db database.Database, workspaceID, viewerUserID int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, key, active) VALUES (?, 'Test', 'TEST', 1)`, workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	var viewerRoleID int
	if err := db.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Viewer'`).Scan(&viewerRoleID); err != nil {
		t.Fatalf("look up Viewer role: %v", err)
	}
	var adminRoleID int
	if err := db.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Administrator'`).Scan(&adminRoleID); err != nil {
		t.Fatalf("look up Administrator role: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP), (999, ?, ?, CURRENT_TIMESTAMP)
	`, viewerUserID, workspaceID, viewerRoleID, workspaceID, adminRoleID); err != nil {
		t.Fatalf("assign workspace roles: %v", err)
	}
}

// R5-1 workspace arm — POST.
func TestBoardConfigurationHandler_CreateForCollection_RejectsViewerOnWorkspace(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	seedWorkspaceWithViewerUser(t, db, 1, userID)

	permService := newNegativeTestPermissionService(t, db)
	handler := NewBoardConfigurationHandler(db, permService)

	req := authedRequest(http.MethodPost, "/collections/default/board-configuration?workspace_id=1", userID,
		decodeRawJSONForBoardConfig(t))
	req.SetPathValue("id", "default")
	rr := httptest.NewRecorder()
	handler.CreateForCollection(rr, req)

	if rr.Code == http.StatusCreated {
		t.Fatalf("Create succeeded (201) for a user with only item.view on the workspace; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 404 or 403 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}
}

// R5-1 workspace arm — PUT.
func TestBoardConfigurationHandler_UpdateForCollection_RejectsViewerOnWorkspace(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	seedWorkspaceWithViewerUser(t, db, 1, userID)

	var configID int
	if err := db.QueryRow(`
		INSERT INTO board_configurations (workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at)
		VALUES (1, '[]', '[]', '[]', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`).Scan(&configID); err != nil {
		t.Fatalf("seed board config: %v", err)
	}

	permService := newNegativeTestPermissionService(t, db)
	handler := NewBoardConfigurationHandler(db, permService)

	req := authedRequest(http.MethodPut, "/collections/default/board-configuration/"+strconv.Itoa(configID), userID,
		decodeRawJSONForBoardConfig(t))
	req.SetPathValue("collectionId", "default")
	req.SetPathValue("configId", strconv.Itoa(configID))
	rr := httptest.NewRecorder()
	handler.UpdateForCollection(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("Update succeeded (200) for a user with only item.view; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 404 or 403. body=%s", rr.Code, rr.Body.String())
	}
}

// R5-1 workspace arm — DELETE.
func TestBoardConfigurationHandler_DeleteForCollection_RejectsViewerOnWorkspace(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999)
	seedWorkspaceWithViewerUser(t, db, 1, userID)

	var configID int
	if err := db.QueryRow(`
		INSERT INTO board_configurations (workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at)
		VALUES (1, '[]', '[]', '[]', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`).Scan(&configID); err != nil {
		t.Fatalf("seed board config: %v", err)
	}

	permService := newNegativeTestPermissionService(t, db)
	handler := NewBoardConfigurationHandler(db, permService)

	req := authedRequest(http.MethodDelete, "/collections/default/board-configuration/"+strconv.Itoa(configID), userID, nil)
	req.SetPathValue("collectionId", "default")
	req.SetPathValue("configId", strconv.Itoa(configID))
	rr := httptest.NewRecorder()
	handler.DeleteForCollection(rr, req)

	if rr.Code == http.StatusNoContent {
		t.Fatalf("Delete succeeded (204) for a user with only item.view; pre-fix bug.")
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 404 or 403. body=%s", rr.Code, rr.Body.String())
	}

	// Belt-and-braces: the config must still be there.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM board_configurations WHERE id = ?`, configID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n == 0 {
		t.Errorf("board configuration was deleted despite the rejection")
	}
}

// R5-1 collection arm: writing to a public collection's board config without
// ownership/edit. Today checkCollectionAccess treats is_public=true as a free
// pass for any auth'd user.
func TestBoardConfigurationHandler_CreateForCollection_RejectsNonOwnerOnPublicCollection(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, 999) // owner of the public collection

	// Public collection owned by 999 (not by user 1).
	if _, err := db.Exec(`
		INSERT INTO collections (id, name, ql_query, is_public, workspace_id, created_by)
		VALUES (42, 'Public C', '', 1, NULL, 999)
	`); err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	permService := newNegativeTestPermissionService(t, db)
	handler := NewBoardConfigurationHandler(db, permService)

	req := authedRequest(http.MethodPost, "/collections/42/board-configuration", userID,
		decodeRawJSONForBoardConfig(t))
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	handler.CreateForCollection(rr, req)

	if rr.Code == http.StatusCreated {
		t.Fatalf("Create succeeded (201) for a non-owner on a public collection; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 404 or 403. body=%s", rr.Code, rr.Body.String())
	}
}

// decodeRawJSONForBoardConfig returns a typed value for authedRequest's body
// param so encoding/json marshals our prefab JSON without re-quoting it.
// Using map[string]interface{} matches the shape consumed by the handler.
func decodeRawJSONForBoardConfig(t *testing.T) models.BoardConfigurationRequest {
	t.Helper()
	return models.BoardConfigurationRequest{
		BacklogStatusIDs: []int{},
		ListColumns:      []models.ListColumn{},
		CardFields:       []models.ListColumn{},
		Columns:          []models.BoardColumnRequest{},
	}
}
