package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"windshift/internal/contextkeys"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// These tests exercise the SCIM audit-logging paths end-to-end against an
// in-memory SQLite database. They cover:
//   - per-member audit events for group membership changes (CreateGroup,
//     ReplaceGroup, PatchGroup)
//   - per-attribute change capture in PATCH details
//   - truthful success=false flag when a member INSERT fails
//   - the agent-impact alert emitted when a SCIM deactivation would orphan
//     agent users or tokens
//
// Audit writes happen in a background goroutine. Tests poll audit_logs until
// the expected row count is reached (or a timeout fires) rather than relying
// on a fixed sleep.

// newSCIMTestHandler returns a SCIMHandler wired to a fresh in-memory database
// with just the tables the SCIM paths read/write. No audit batcher is
// initialized, so LogAudit falls through to an immediate-write path, which
// makes audit rows observable after a short poll.
func newSCIMTestHandler(t *testing.T) (*SCIMHandler, database.Database) {
	t.Helper()
	// Each test gets its own shared-cache handle so rows don't leak between tests.
	dsn := "file:scimtest_" + t.Name() + "?mode=memory&cache=shared"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			is_active BOOLEAN NOT NULL DEFAULT 1,
			scim_external_id TEXT,
			scim_managed BOOLEAN NOT NULL DEFAULT 0,
			is_agent BOOLEAN NOT NULL DEFAULT 0,
			agent_owner_user_id INTEGER REFERENCES users(id),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			scim_external_id TEXT,
			scim_managed BOOLEAN NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			scim_managed BOOLEAN NOT NULL DEFAULT 0,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(group_id, user_id)
		)`,
		`CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			user_id INTEGER,
			username TEXT NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			action_type TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id INTEGER,
			resource_name TEXT,
			details TEXT,
			success BOOLEAN NOT NULL DEFAULT 1,
			error_message TEXT
		)`,
		`CREATE TABLE api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			token_prefix TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE user_app_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_name TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Minimal schema for admin lookup + admin notifications. The
		// handleSCIMUserDeactivation cascade queries system.admin permission
		// holders and inserts one notification row per admin.
		`CREATE TABLE permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			permission_key TEXT UNIQUE NOT NULL,
			permission_name TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT 'global'
		)`,
		`CREATE TABLE user_global_permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
			UNIQUE(user_id, permission_id)
		)`,
		`CREATE TABLE notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'info',
			metadata TEXT,
			read BOOLEAN NOT NULL DEFAULT 0,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Seed the system.admin permission so ActiveSystemAdminIDs can join.
		`INSERT INTO permissions (permission_key, permission_name) VALUES ('system.admin', 'System Administrator')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}

	// Permission service is only used for cache invalidation; a live instance
	// with warmup disabled is cheap and keeps the handler surface authentic.
	pcfg := services.DefaultPermissionCacheConfig()
	pcfg.WarmupOnStartup = false
	permService, err := services.NewPermissionService(db, pcfg)
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}

	h := NewSCIMHandler(db, "http://test", permService)
	return h, db
}

// scimReq builds an HTTP request with a fake SCIM token in context so the
// handler's audit log can attribute events to a recognizable token prefix.
func scimReq(method, target string, body interface{}) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/scim+json")
	token := &models.SCIMToken{ID: 1, Name: "test", TokenPrefix: "scim_test_abc", IsActive: true}
	ctx := context.WithValue(req.Context(), contextkeys.SCIMToken, token)
	return req.WithContext(ctx)
}

// setPathValue attaches a path parameter to a request the way Go 1.22's
// http.ServeMux would. Needed because handlers call r.PathValue("id").
func setPathValue(req *http.Request, key, val string) *http.Request {
	req.SetPathValue(key, val)
	return req
}

// waitForAuditCount polls audit_logs for rows matching actionType until the
// expected count is reached or the deadline fires. Returns the final count so
// assertions produce a clear diff on mismatch.
func waitForAuditCount(t *testing.T, db database.Database, actionType string, want int) int {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	var got int
	for time.Now().Before(deadline) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action_type = ?`, actionType).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", actionType, err)
		}
		if got == want {
			return got
		}
		time.Sleep(15 * time.Millisecond)
	}
	return got
}

// fetchAuditDetails returns the parsed details JSON for the most recent audit
// row of the given action type. Fails the test if no row is found.
func fetchAuditDetails(t *testing.T, db database.Database, actionType string) map[string]interface{} {
	t.Helper()
	var raw *string
	err := db.QueryRow(`
		SELECT details FROM audit_logs
		WHERE action_type = ?
		ORDER BY id DESC LIMIT 1
	`, actionType).Scan(&raw)
	if err != nil {
		t.Fatalf("fetch details for %s: %v", actionType, err)
	}
	if raw == nil {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		t.Fatalf("unmarshal details for %s: %v", actionType, err)
	}
	return out
}

// seedUser inserts a bare-minimum user row and returns its ID.
func seedUser(t *testing.T, db database.Database, username string) int {
	t.Helper()
	// SCIM write paths refuse non-SCIM-managed targets with 404, so every
	// helper-seeded user must carry scim_managed = true to stand in for an
	// IdP-provisioned identity.
	res, err := db.Exec(`INSERT INTO users (email, username, scim_managed) VALUES (?, ?, true)`,
		username+"@example.com", username)
	if err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// seedGroup inserts a bare-minimum SCIM-managed group row and returns its ID.
func seedGroup(t *testing.T, db database.Database, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO groups (name, scim_managed) VALUES (?, true)`, name)
	if err != nil {
		t.Fatalf("seed group %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// seedGroupMember adds a SCIM-managed row to group_members.
func seedGroupMember(t *testing.T, db database.Database, groupID, userID int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO group_members (group_id, user_id, scim_managed) VALUES (?, ?, true)`, groupID, userID)
	if err != nil {
		t.Fatalf("seed group_member: %v", err)
	}
}

// ---- Group membership: per-member audit events ----

func TestCreateGroup_EmitsPerMemberAddAudit(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	u1 := seedUser(t, db, "alice")
	u2 := seedUser(t, db, "bob")

	body := models.SCIMGroup{
		DisplayName: "engineering",
		Members: []models.SCIMGroupMember{
			{Value: strconv.Itoa(u1)},
			{Value: strconv.Itoa(u2)},
		},
	}
	rec := httptest.NewRecorder()
	h.CreateGroup(rec, scimReq(http.MethodPost, "/scim/v2/Groups", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	if got := waitForAuditCount(t, db, "scim.group.create", 1); got != 1 {
		t.Errorf("group.create rows: got %d want 1", got)
	}
	if got := waitForAuditCount(t, db, "scim.group.add_member", 2); got != 2 {
		t.Errorf("add_member rows: got %d want 2", got)
	}

	// A random add_member row should carry the member's user_id.
	details := fetchAuditDetails(t, db, "scim.group.add_member")
	if details["user_id"] == nil {
		t.Errorf("add_member details missing user_id: %v", details)
	}
}

func TestReplaceGroup_EmitsRemoveAndAddAudit(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	u1 := seedUser(t, db, "alice")
	u2 := seedUser(t, db, "bob")
	u3 := seedUser(t, db, "carol")
	gid := seedGroup(t, db, "team")
	seedGroupMember(t, db, gid, u1)
	seedGroupMember(t, db, gid, u2)

	// Full replace: remove alice+bob, add carol.
	body := models.SCIMGroup{
		DisplayName: "team",
		Members:     []models.SCIMGroupMember{{Value: strconv.Itoa(u3)}},
	}
	req := setPathValue(scimReq(http.MethodPut, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.ReplaceGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := waitForAuditCount(t, db, "scim.group.remove_member", 2); got != 2 {
		t.Errorf("remove_member rows: got %d want 2", got)
	}
	if got := waitForAuditCount(t, db, "scim.group.add_member", 1); got != 1 {
		t.Errorf("add_member rows: got %d want 1", got)
	}
	if got := waitForAuditCount(t, db, "scim.group.update", 1); got != 1 {
		t.Errorf("group.update rows: got %d want 1", got)
	}
}

func TestPatchGroup_MemberOps_EmitsPerMemberAudit(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	u1 := seedUser(t, db, "alice")
	u2 := seedUser(t, db, "bob")
	u3 := seedUser(t, db, "carol")
	gid := seedGroup(t, db, "team")
	seedGroupMember(t, db, gid, u1)

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{
				Op:   "add",
				Path: "members",
				Value: []interface{}{
					map[string]interface{}{"value": strconv.Itoa(u2)},
					map[string]interface{}{"value": strconv.Itoa(u3)},
				},
			},
			{
				Op:   "remove",
				Path: "members",
				Value: []interface{}{
					map[string]interface{}{"value": strconv.Itoa(u1)},
				},
			},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.PatchGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := waitForAuditCount(t, db, "scim.group.add_member", 2); got != 2 {
		t.Errorf("add_member rows: got %d want 2", got)
	}
	if got := waitForAuditCount(t, db, "scim.group.remove_member", 1); got != 1 {
		t.Errorf("remove_member rows: got %d want 1", got)
	}
}

func TestPatchGroup_AttributeChange_RecordsChanges(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	gid := seedGroup(t, db, "old-name")

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{Op: "replace", Path: "displayName", Value: "new-name"},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.PatchGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	_ = waitForAuditCount(t, db, "scim.group.update", 1)
	details := fetchAuditDetails(t, db, "scim.group.update")
	changes, ok := details["changes"].([]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 change, got %v", details["changes"])
	}
	c := changes[0].(map[string]interface{})
	if c["path"] != "displayName" || c["old_value"] != "old-name" || c["new_value"] != "new-name" {
		t.Errorf("unexpected change entry: %v", c)
	}
}

func TestPatchUser_AttributeChange_RecordsChanges(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	uid := seedUser(t, db, "dave")

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{Op: "replace", Path: "active", Value: false},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Users/"+strconv.Itoa(uid), body), "id", strconv.Itoa(uid))
	rec := httptest.NewRecorder()
	h.PatchUser(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	_ = waitForAuditCount(t, db, "scim.user.update", 1)
	details := fetchAuditDetails(t, db, "scim.user.update")
	changes, ok := details["changes"].([]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 change, got %v", details["changes"])
	}
	c := changes[0].(map[string]interface{})
	if c["path"] != "active" || c["old_value"] != true || c["new_value"] != false {
		t.Errorf("unexpected change entry: %v", c)
	}
}

// TestPatchUser_UnknownPath_AuditsNoop verifies that an IdP pushing an
// attribute we don't handle (e.g. "phoneNumbers.work") succeeds as an audited
// no-op — not a silent drop. The aggregate PATCH audit row must carry an
// "<unsupported>" breadcrumb so operators can grep for IdP misconfiguration.
func TestPatchUser_UnknownPath_AuditsNoop(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	uid := seedUser(t, db, "erin")

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{Op: "replace", Path: "phoneNumbers.work", Value: "+1-555-0100"},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Users/"+strconv.Itoa(uid), body), "id", strconv.Itoa(uid))
	rec := httptest.NewRecorder()
	h.PatchUser(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	_ = waitForAuditCount(t, db, "scim.user.update", 1)
	details := fetchAuditDetails(t, db, "scim.user.update")
	changes, ok := details["changes"].([]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 breadcrumb change, got %v", details["changes"])
	}
	c := changes[0].(map[string]interface{})
	if c["new_value"] != "<unsupported>" {
		t.Errorf("expected breadcrumb new_value=<unsupported>, got %v", c["new_value"])
	}
	if c["path"] != "phoneNumbers.work" {
		t.Errorf("expected breadcrumb path to preserve original op.Path, got %v", c["path"])
	}
}

// ---- Member insert failure: success flag reflects DB error ----

func TestCreateGroup_MemberInsertFailure_AuditsSuccessFalse(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	// FK on group_members.user_id → users.id is enforced (schema above) but
	// SQLite requires PRAGMA foreign_keys = ON per connection to actually
	// reject inserts. Enable it for this test.
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable fk: %v", err)
	}

	body := models.SCIMGroup{
		DisplayName: "ghosts",
		Members:     []models.SCIMGroupMember{{Value: "99999"}}, // no such user
	}
	rec := httptest.NewRecorder()
	h.CreateGroup(rec, scimReq(http.MethodPost, "/scim/v2/Groups", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// One add_member row was emitted, and its success flag should be false.
	_ = waitForAuditCount(t, db, "scim.group.add_member", 1)
	var success bool
	var errMsg *string
	err := db.QueryRow(`
		SELECT success, error_message FROM audit_logs
		WHERE action_type = 'scim.group.add_member' ORDER BY id DESC LIMIT 1
	`).Scan(&success, &errMsg)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if success {
		t.Errorf("expected success=false for failed FK insert")
	}
	if errMsg == nil || *errMsg == "" {
		t.Errorf("expected non-empty error_message, got %v", errMsg)
	}
}

// ---- Agent-impact alerting ----

// seedAgent inserts an agent user owned by ownerID and returns its ID.
func seedAgent(t *testing.T, db database.Database, ownerID int, username string) int {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO users (email, username, is_agent, agent_owner_user_id)
		VALUES (?, ?, true, ?)
	`, username+"@agents.local", username, ownerID)
	if err != nil {
		t.Fatalf("seed agent %s: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// seedAdmin creates an active user with the 'system.admin' permission so the
// cascade notification has a recipient to land on.
func seedAdmin(t *testing.T, db database.Database, username string) int {
	t.Helper()
	id := seedUser(t, db, username)
	if _, err := db.Exec(`
		INSERT INTO user_global_permissions (user_id, permission_id)
		SELECT ?, id FROM permissions WHERE permission_key = 'system.admin'
	`, id); err != nil {
		t.Fatalf("grant admin to %s: %v", username, err)
	}
	return id
}

func TestDeleteUser_AgentOwner_CascadesAndNotifies(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	adminID := seedAdmin(t, db, "rootadmin")
	ownerID := seedUser(t, db, "owner")
	bot1 := seedAgent(t, db, ownerID, "bot1")
	bot2 := seedAgent(t, db, ownerID, "bot2")
	// One api_token on the owner, one on an agent — both should be revoked.
	if _, err := db.Exec(`
		INSERT INTO api_tokens (user_id, name, token_hash, token_prefix)
		VALUES (?, 'ci', ?, 'pre-a'), (?, 'bot-tok', ?, 'pre-b')
	`, ownerID, "hash-a", bot1, "hash-b"); err != nil {
		t.Fatalf("seed api_tokens: %v", err)
	}

	req := setPathValue(scimReq(http.MethodDelete, "/scim/v2/Users/"+strconv.Itoa(ownerID), nil), "id", strconv.Itoa(ownerID))
	rec := httptest.NewRecorder()
	h.DeleteUser(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Agents must be flipped inactive by the cascade (this is the "immediate
	// offboarding" invariant that prevents orphaned agent credentials).
	var bot1Active, bot2Active bool
	_ = db.QueryRow(`SELECT is_active FROM users WHERE id = ?`, bot1).Scan(&bot1Active)
	_ = db.QueryRow(`SELECT is_active FROM users WHERE id = ?`, bot2).Scan(&bot2Active)
	if bot1Active || bot2Active {
		t.Errorf("agents should be inactive after cascade: bot1=%v bot2=%v", bot1Active, bot2Active)
	}
	// api_tokens should be hard-deleted (no is_active column to flip).
	var tokenCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE user_id IN (?, ?, ?)`, ownerID, bot1, bot2).Scan(&tokenCount)
	if tokenCount != 0 {
		t.Errorf("api_tokens should be revoked, got %d remaining", tokenCount)
	}

	// Aggregate audit row summarizes the cascade.
	if got := waitForAuditCount(t, db, "scim.user.agent_impact", 1); got != 1 {
		t.Fatalf("agent_impact rows: got %d want 1", got)
	}
	details := fetchAuditDetails(t, db, "scim.user.agent_impact")
	if details["trigger"] != "scim_delete" {
		t.Errorf("trigger: got %v want scim_delete", details["trigger"])
	}
	agents, _ := details["deactivated_agent_ids"].([]interface{})
	if len(agents) != 2 {
		t.Errorf("deactivated_agent_ids: got %v want len 2", agents)
	}
	if n, _ := details["revoked_api_tokens"].(float64); int(n) != 2 {
		t.Errorf("revoked_api_tokens: got %v want 2", details["revoked_api_tokens"])
	}

	// Per-agent and per-token audit rows so security can reconstruct.
	if got := waitForAuditCount(t, db, "agent.deactivate", 2); got != 2 {
		t.Errorf("agent.deactivate rows: got %d want 2", got)
	}
	if got := waitForAuditCount(t, db, "api_token.auto_revoke", 2); got != 2 {
		t.Errorf("api_token.auto_revoke rows: got %d want 2", got)
	}

	// Baked-in admin notification landed on the admin row we seeded.
	var notifTitle, notifType string
	var notifMeta *string
	err := db.QueryRow(`
		SELECT title, type, metadata FROM notifications
		WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, adminID).Scan(&notifTitle, &notifType, &notifMeta)
	if err != nil {
		t.Fatalf("fetch admin notification: %v", err)
	}
	if notifType != "warning" {
		t.Errorf("notification type: got %q want warning", notifType)
	}
	if !strings.Contains(notifTitle, "SCIM offboarding") {
		t.Errorf("notification title: got %q", notifTitle)
	}
	if notifMeta == nil {
		t.Errorf("notification metadata missing")
	}
}

func TestDeleteUser_NoAgents_NoCascadeOrNotification(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	_ = seedAdmin(t, db, "rootadmin")
	uid := seedUser(t, db, "loneoak")

	req := setPathValue(scimReq(http.MethodDelete, "/scim/v2/Users/"+strconv.Itoa(uid), nil), "id", strconv.Itoa(uid))
	rec := httptest.NewRecorder()
	h.DeleteUser(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusNoContent)
	}

	// Nothing owned by this user — no aggregate audit row, no admin notification.
	time.Sleep(100 * time.Millisecond)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action_type = 'scim.user.agent_impact'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("unexpected agent_impact rows: %d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM notifications`).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if n != 0 {
		t.Errorf("unexpected notifications: %d", n)
	}
}

func TestPatchUser_ActiveFalse_WithAgents_CascadesAndNotifies(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	adminID := seedAdmin(t, db, "rootadmin")
	ownerID := seedUser(t, db, "owner2")
	botID := seedAgent(t, db, ownerID, "bot3")

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{Op: "replace", Path: "active", Value: false},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Users/"+strconv.Itoa(ownerID), body), "id", strconv.Itoa(ownerID))
	rec := httptest.NewRecorder()
	h.PatchUser(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Agent cascaded inactive.
	var botActive bool
	_ = db.QueryRow(`SELECT is_active FROM users WHERE id = ?`, botID).Scan(&botActive)
	if botActive {
		t.Errorf("agent should be inactive after PATCH active=false cascade")
	}

	if got := waitForAuditCount(t, db, "scim.user.agent_impact", 1); got != 1 {
		t.Errorf("agent_impact rows: got %d want 1", got)
	}
	details := fetchAuditDetails(t, db, "scim.user.agent_impact")
	if details["trigger"] != "scim_patch" {
		t.Errorf("trigger: got %v want scim_patch", details["trigger"])
	}

	var notifCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ?`, adminID).Scan(&notifCount)
	if notifCount != 1 {
		t.Errorf("admin notifications: got %d want 1", notifCount)
	}
}
