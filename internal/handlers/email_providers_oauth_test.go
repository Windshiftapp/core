package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"windshift/internal/contextkeys"
	"windshift/internal/database"
	"windshift/internal/models"
)

// These tests cover the provider-backed email OAuth start route from
// docs/bughunt6.md Run 10 #1. Invariant being defended: the path params,
// query reads, and generated redirect URI must stay aligned with the
// registered routes — otherwise the flow can't complete a round trip.

// newEmailProviderTestHandler returns an EmailProviderHandler against a fresh
// in-memory SQLite DB with the minimum schema StartEmailOAuth touches. No
// encryptor is wired (nil) — handler skips decryption when the row's
// oauth_client_secret_encrypted is NULL, which is the test fixture below.
func newEmailProviderTestHandler(t *testing.T) (*EmailProviderHandler, database.Database) {
	t.Helper()
	dsn := "file:emailoauthtest_" + t.Name() + "?mode=memory&cache=shared"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL
		)`,
		`CREATE TABLE channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE email_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			type TEXT NOT NULL,
			is_enabled BOOLEAN NOT NULL DEFAULT 0,
			oauth_client_id TEXT,
			oauth_client_secret_encrypted TEXT,
			oauth_scopes TEXT,
			oauth_tenant_id TEXT
		)`,
		`CREATE TABLE email_oauth_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			channel_id INTEGER,
			state TEXT UNIQUE NOT NULL,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}

	h := NewEmailProviderHandler(db, nil, "http://test.local")
	return h, db
}

func seedEmailProvider(t *testing.T, db database.Database, slug, providerType string, enabled bool) int {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO email_providers
		    (name, slug, type, is_enabled, oauth_client_id, oauth_scopes, oauth_tenant_id)
		VALUES (?, ?, ?, ?, 'client-abc', 'openid email', 'common')
	`, slug, slug, providerType, enabled)
	if err != nil {
		t.Fatalf("seed provider %s: %v", slug, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func seedChannel(t *testing.T, db database.Database, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO channels (name) VALUES (?)`, name)
	if err != nil {
		t.Fatalf("seed channel %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func seedEmailTestUser(t *testing.T, db database.Database, username string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, username) VALUES (?, ?)`,
		username+"@example.com", username)
	if err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// emailStartReq builds a POST request shaped like the route
// `POST /api/channels/{channel_id}/email-providers/{slug}/oauth/start`,
// with PathValues bound and the authenticated user injected into context.
// userID = 0 means "no user in context" (exercises the 401 path).
func emailStartReq(channelID int, slug string, userID int) *http.Request {
	target := "/api/channels/" + strconv.Itoa(channelID) + "/email-providers/" + slug + "/oauth/start"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.SetPathValue("channel_id", strconv.Itoa(channelID))
	req.SetPathValue("slug", slug)
	if userID != 0 {
		ctx := context.WithValue(req.Context(), contextkeys.User, &models.User{ID: userID})
		req = req.WithContext(ctx)
	}
	return req
}

func countEmailOAuthStates(t *testing.T, db database.Database, providerID int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_oauth_state WHERE provider_id = ?`, providerID).Scan(&n); err != nil {
		t.Fatalf("count states: %v", err)
	}
	return n
}

func TestStartEmailOAuth_EmitsAuthURLAndPersistsState(t *testing.T) {
	h, db := newEmailProviderTestHandler(t)
	uid := seedEmailTestUser(t, db, "alice")
	chID := seedChannel(t, db, "support")
	pid := seedEmailProvider(t, db, "ms-main", "microsoft", true)

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, emailStartReq(chID, "ms-main", uid))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v — body: %s", err, rec.Body.String())
	}
	authURL := resp["auth_url"]
	if authURL == "" {
		t.Fatalf("auth_url empty: %v", resp)
	}

	// One scm_oauth_state row should have landed under the provider and channel.
	if n := countEmailOAuthStates(t, db, pid); n != 1 {
		t.Errorf("email_oauth_state rows for provider: got %d want 1", n)
	}
	var stateChannelID, stateUserID int
	if err := db.QueryRow(`
		SELECT channel_id, user_id FROM email_oauth_state WHERE provider_id = ?
	`, pid).Scan(&stateChannelID, &stateUserID); err != nil {
		t.Fatalf("read state row: %v", err)
	}
	if stateChannelID != chID {
		t.Errorf("state.channel_id: got %d want %d", stateChannelID, chID)
	}
	if stateUserID != uid {
		t.Errorf("state.user_id: got %d want %d", stateUserID, uid)
	}
}

func TestStartEmailOAuth_RedirectURIMatchesCallbackRoute(t *testing.T) {
	// The redirect_uri embedded in the OAuth auth URL must match
	// the registered callback route at /api/email/oauth/{slug}/callback.
	// If it drifts, the OAuth provider will reject the callback as
	// redirect_uri mismatch.
	h, db := newEmailProviderTestHandler(t)
	uid := seedEmailTestUser(t, db, "bob")
	chID := seedChannel(t, db, "billing")
	_ = seedEmailProvider(t, db, "ms-billing", "microsoft", true)

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, emailStartReq(chID, "ms-billing", uid))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	authURL, err := url.Parse(resp["auth_url"])
	if err != nil {
		t.Fatalf("parse auth_url: %v", err)
	}
	gotRedirect := authURL.Query().Get("redirect_uri")
	wantRedirect := "http://test.local/api/email/oauth/ms-billing/callback"
	if gotRedirect != wantRedirect {
		t.Errorf("redirect_uri: got %q want %q — handler and route are misaligned", gotRedirect, wantRedirect)
	}
}

func TestStartEmailOAuth_NonNumericChannelID_Returns400(t *testing.T) {
	h, _ := newEmailProviderTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/channels/notanumber/email-providers/ms-main/oauth/start", nil)
	req.SetPathValue("channel_id", "notanumber")
	req.SetPathValue("slug", "ms-main")
	ctx := context.WithValue(req.Context(), contextkeys.User, &models.User{ID: 1})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 — non-numeric channel_id must be rejected", rec.Code)
	}
}

func TestStartEmailOAuth_NoUserInContext_Returns401(t *testing.T) {
	h, db := newEmailProviderTestHandler(t)
	chID := seedChannel(t, db, "anon")
	_ = seedEmailProvider(t, db, "ms-anon", "microsoft", true)

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, emailStartReq(chID, "ms-anon", 0)) // userID=0 → no context user

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401 — missing user context must 401", rec.Code)
	}
}

func TestStartEmailOAuth_MissingSlug_Returns400(t *testing.T) {
	h, db := newEmailProviderTestHandler(t)
	uid := seedEmailTestUser(t, db, "carol")
	chID := seedChannel(t, db, "ch-missing-slug")

	req := httptest.NewRequest(http.MethodPost, "/api/channels/"+strconv.Itoa(chID)+"/email-providers//oauth/start", nil)
	req.SetPathValue("channel_id", strconv.Itoa(chID))
	req.SetPathValue("slug", "") // explicit empty slug
	ctx := context.WithValue(req.Context(), contextkeys.User, &models.User{ID: uid})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 — empty slug must be rejected", rec.Code)
	}
}

func TestStartEmailOAuth_DisabledProvider_Returns404(t *testing.T) {
	h, db := newEmailProviderTestHandler(t)
	uid := seedEmailTestUser(t, db, "dave")
	chID := seedChannel(t, db, "ch-disabled")
	_ = seedEmailProvider(t, db, "ms-disabled", "microsoft", false) // is_enabled = false

	rec := httptest.NewRecorder()
	h.StartEmailOAuth(rec, emailStartReq(chID, "ms-disabled", uid))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404 — disabled provider must be invisible", rec.Code)
	}
	// auth_url payload must not contain a Microsoft auth URL leak.
	if strings.Contains(rec.Body.String(), "login.microsoftonline.com") {
		t.Errorf("body leaked Microsoft auth URL despite disabled provider: %s", rec.Body.String())
	}
}
