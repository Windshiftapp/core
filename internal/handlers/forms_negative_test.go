package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// Regression test for docs/bughunt2.md Run 6 finding #1.
//
// Today UpdateRequestTypeConfig only checks that the request type row
// exists, then overwrites its config JSON. The fix gates the write behind
// channel-management or system admin. This test seeds a request type whose
// owning channel is managed by a stranger (UID 999) and asserts that a
// non-manager (UID 1) gets a not-found-style rejection (404) when they try
// to overwrite the config.

func seedChannelWithStrangerManager(t *testing.T, db database.Database, channelID, strangerUserID int) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO channels (id, name, type, direction, status)
		VALUES (?, 'Form Channel', 'form', 'inbound', 'enabled')
	`, channelID); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO channel_managers (channel_id, manager_type, manager_id, added_by)
		VALUES (?, 'user', ?, ?)
	`, channelID, strangerUserID, strangerUserID); err != nil {
		t.Fatalf("seed channel manager: %v", err)
	}
}

func seedRequestTypeForChannel(t *testing.T, db database.Database, requestTypeID, channelID int) {
	t.Helper()
	// Use any existing item_type seeded during Initialize.
	var itemTypeID int
	if err := db.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&itemTypeID); err != nil {
		t.Fatalf("no seeded item_type available: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO request_types (id, channel_id, name, item_type_id, config)
		VALUES (?, ?, 'Test RT', ?, '{}')
	`, requestTypeID, channelID, itemTypeID); err != nil {
		t.Fatalf("seed request_type: %v", err)
	}
}

// R6-1: a non-manager must not be able to overwrite a request type's form config.
func TestFormHandler_UpdateRequestTypeConfig_RejectsNonChannelManager(t *testing.T) {
	const (
		userID    = 1
		stranger  = 999
		channelID = 10
		rtID      = 20
	)
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	seedNegativeTestUser(t, db, stranger)
	seedChannelWithStrangerManager(t, db, channelID, stranger)
	seedRequestTypeForChannel(t, db, rtID, channelID)

	permService := newNegativeTestPermissionService(t, db)
	channelService := services.NewChannelService(db, permService)
	handler := NewFormHandler(db, nil, nil, nil, channelService)

	body := models.RequestTypeConfig{SuccessMessage:"haha", RedirectURL: "https://evil.example.com"}
	req := authedRequest(http.MethodPut, "/request-types/"+strconv.Itoa(rtID)+"/config", userID, body)
	req.SetPathValue("id", strconv.Itoa(rtID))
	rr := httptest.NewRecorder()
	handler.UpdateRequestTypeConfig(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("UpdateRequestTypeConfig succeeded (200) for a non-manager; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 404 or 403 (post-fix behavior). body=%s", rr.Code, rr.Body.String())
	}

	// Belt-and-braces: config must still be the original empty object.
	var cfg string
	if err := db.QueryRow(`SELECT config FROM request_types WHERE id = ?`, rtID).Scan(&cfg); err != nil {
		t.Fatalf("re-read request type: %v", err)
	}
	if cfg != "{}" {
		t.Errorf("request type config was overwritten to %q despite the rejection", cfg)
	}
}

// Positive sanity: a manager of the channel still succeeds. Guards against
// the fix over-restricting and breaking legitimate use.
func TestFormHandler_UpdateRequestTypeConfig_AllowsChannelManager(t *testing.T) {
	const (
		userID    = 1
		channelID = 10
		rtID      = 20
	)
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	// Manager is the test user themselves.
	seedChannelWithStrangerManager(t, db, channelID, userID)
	seedRequestTypeForChannel(t, db, rtID, channelID)

	permService := newNegativeTestPermissionService(t, db)
	channelService := services.NewChannelService(db, permService)
	handler := NewFormHandler(db, nil, nil, nil, channelService)

	body := models.RequestTypeConfig{SuccessMessage:"thanks", RedirectURL: "https://example.com"}
	req := authedRequest(http.MethodPut, "/request-types/"+strconv.Itoa(rtID)+"/config", userID, body)
	req.SetPathValue("id", strconv.Itoa(rtID))
	rr := httptest.NewRecorder()
	handler.UpdateRequestTypeConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Manager got status %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
}
