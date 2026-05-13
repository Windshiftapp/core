package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

// These tests exercise the scim_managed enforcement on the SCIM group
// surface. The invariant being defended: SCIM only ever sees, mutates, or
// references the rows it provisioned. A locally-managed group, a local
// membership inside an otherwise SCIM-managed group, or a local user ID
// passed as a member value must all be invisible/inert to a SCIM client.
//
// Companion findings: docs/bughunt4.md Run 8 #1–#4.

// seedLocalGroup inserts a group row with scim_managed = false.
func seedLocalGroup(t *testing.T, db database.Database, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO groups (name, scim_managed) VALUES (?, false)`, name)
	if err != nil {
		t.Fatalf("seed local group %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// seedLocalUser inserts a user row with scim_managed = false.
func seedLocalUser(t *testing.T, db database.Database, username string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, username, scim_managed) VALUES (?, ?, false)`,
		username+"@example.com", username)
	if err != nil {
		t.Fatalf("seed local user %s: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// seedLocalGroupMember inserts a group_members row with scim_managed = false.
func seedLocalGroupMember(t *testing.T, db database.Database, groupID, userID int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO group_members (group_id, user_id, scim_managed) VALUES (?, ?, false)`, groupID, userID); err != nil {
		t.Fatalf("seed local group_member: %v", err)
	}
}

// decodeSCIMListResponse parses a SCIM ListResponse from a response recorder
// or fails the test.
func decodeSCIMListResponse(t *testing.T, body []byte) models.SCIMListResponse {
	t.Helper()
	var resp models.SCIMListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal list response: %v — body: %s", err, string(body))
	}
	return resp
}

// decodeSCIMGroup parses a SCIM Group resource from a response recorder.
func decodeSCIMGroup(t *testing.T, body []byte) models.SCIMGroup {
	t.Helper()
	var g models.SCIMGroup
	if err := json.Unmarshal(body, &g); err != nil {
		t.Fatalf("unmarshal group: %v — body: %s", err, string(body))
	}
	return g
}

// ---- Finding 1: list/search exposure ----

func TestListGroups_ExcludesLocalGroups(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	_ = seedGroup(t, db, "scim-team")
	_ = seedLocalGroup(t, db, "local-team")

	rec := httptest.NewRecorder()
	h.ListGroups(rec, scimReq(http.MethodGet, "/scim/v2/Groups", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeSCIMListResponse(t, rec.Body.Bytes())
	if resp.TotalResults != 1 {
		t.Errorf("totalResults: got %d want 1 — leaked local group", resp.TotalResults)
	}
	if len(resp.Resources) != 1 {
		t.Fatalf("resources len: got %d want 1", len(resp.Resources))
	}
	res := resp.Resources[0].(map[string]interface{})
	if res["displayName"] != "scim-team" {
		t.Errorf("displayName: got %v want scim-team", res["displayName"])
	}
}

func TestSearchGroups_ExcludesLocalGroups(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	_ = seedGroup(t, db, "scim-team")
	_ = seedLocalGroup(t, db, "local-team")

	body := models.SCIMSearchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:SearchRequest"},
	}
	rec := httptest.NewRecorder()
	h.SearchGroups(rec, scimReq(http.MethodPost, "/scim/v2/Groups/.search", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeSCIMListResponse(t, rec.Body.Bytes())
	if resp.TotalResults != 1 {
		t.Errorf("totalResults: got %d want 1 — search leaked local group", resp.TotalResults)
	}
}

// ---- Finding 2: GetGroup + member expansion ----

func TestGetGroup_LocalGroup_Returns404(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	gid := seedLocalGroup(t, db, "local-team")

	req := setPathValue(scimReq(http.MethodGet, "/scim/v2/Groups/"+strconv.Itoa(gid), nil), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.GetGroup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d want %d — local group leaked through direct GET", rec.Code, http.StatusNotFound)
	}
}

func TestGetGroup_SCIMGroup_OmitsLocalMembers(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	scimUser := seedUser(t, db, "alice")
	localUser := seedLocalUser(t, db, "carol-local")
	gid := seedGroup(t, db, "scim-team")
	seedGroupMember(t, db, gid, scimUser)
	seedLocalGroupMember(t, db, gid, localUser)

	req := setPathValue(scimReq(http.MethodGet, "/scim/v2/Groups/"+strconv.Itoa(gid), nil), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.GetGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	g := decodeSCIMGroup(t, rec.Body.Bytes())
	if len(g.Members) != 1 {
		t.Fatalf("members len: got %d want 1 — local membership leaked", len(g.Members))
	}
	if g.Members[0].Value != strconv.Itoa(scimUser) {
		t.Errorf("member value: got %s want %d", g.Members[0].Value, scimUser)
	}
}

// ---- Finding 3: PUT/PATCH/DELETE on local groups ----

// auditRefusal returns (success, error_message) for the most recent audit row
// of action_type whose details mention reason=target_not_scim_managed.
func auditRefusal(t *testing.T, db database.Database, actionType string) (bool, string) {
	t.Helper()
	var success bool
	var errMsg *string
	err := db.QueryRow(`
		SELECT success, error_message FROM audit_logs
		WHERE action_type = ? AND details LIKE '%target_not_scim_managed%'
		ORDER BY id DESC LIMIT 1
	`, actionType).Scan(&success, &errMsg)
	if err != nil {
		t.Fatalf("query refusal audit for %s: %v", actionType, err)
	}
	msg := ""
	if errMsg != nil {
		msg = *errMsg
	}
	return success, msg
}

func TestReplaceGroup_LocalGroup_Refused(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	gid := seedLocalGroup(t, db, "local-team")
	originalName := "local-team"

	body := models.SCIMGroup{DisplayName: "hijacked"}
	req := setPathValue(scimReq(http.MethodPut, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.ReplaceGroup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusNotFound)
	}

	// Group must be unchanged in the DB.
	var name string
	var scimManaged bool
	_ = db.QueryRow(`SELECT name, scim_managed FROM groups WHERE id = ?`, gid).Scan(&name, &scimManaged)
	if name != originalName {
		t.Errorf("name was modified: got %q want %q", name, originalName)
	}
	if scimManaged {
		t.Errorf("scim_managed flag was flipped — group hijacked")
	}

	_ = waitForAuditCount(t, db, "scim.group.update", 1)
	success, msg := auditRefusal(t, db, "scim.group.update")
	if success {
		t.Errorf("refusal audit success: got true want false")
	}
	if msg == "" {
		t.Errorf("refusal audit error_message is empty")
	}
}

func TestPatchGroup_LocalGroup_Refused(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	gid := seedLocalGroup(t, db, "local-team")

	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{Op: "replace", Path: "displayName", Value: "hijacked"},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.PatchGroup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusNotFound)
	}

	var name string
	_ = db.QueryRow(`SELECT name FROM groups WHERE id = ?`, gid).Scan(&name)
	if name != "local-team" {
		t.Errorf("name was modified: got %q want %q", name, "local-team")
	}

	_ = waitForAuditCount(t, db, "scim.group.update", 1)
	success, _ := auditRefusal(t, db, "scim.group.update")
	if success {
		t.Errorf("refusal audit success: got true want false")
	}
}

func TestDeleteGroup_LocalGroup_Refused(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	gid := seedLocalGroup(t, db, "local-team")

	req := setPathValue(scimReq(http.MethodDelete, "/scim/v2/Groups/"+strconv.Itoa(gid), nil), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.DeleteGroup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusNotFound)
	}

	// Group must still exist.
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM groups WHERE id = ?`, gid).Scan(&n)
	if n != 1 {
		t.Errorf("group count: got %d want 1 — SCIM token wiped a local group", n)
	}

	_ = waitForAuditCount(t, db, "scim.group.delete", 1)
	success, _ := auditRefusal(t, db, "scim.group.delete")
	if success {
		t.Errorf("refusal audit success: got true want false")
	}
}

// ---- Finding 4: member writes reject non-SCIM users; PATCH remove can't wipe local rows ----

func TestCreateGroup_NonSCIMUserMember_NotInserted(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	scimUser := seedUser(t, db, "alice")
	localUser := seedLocalUser(t, db, "admin-local")

	body := models.SCIMGroup{
		DisplayName: "engineering",
		Members: []models.SCIMGroupMember{
			{Value: strconv.Itoa(scimUser)},
			{Value: strconv.Itoa(localUser)},
		},
	}
	rec := httptest.NewRecorder()
	h.CreateGroup(rec, scimReq(http.MethodPost, "/scim/v2/Groups", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Only the SCIM-visible user should be a member; the local user must not
	// have been attached.
	var gid int
	if err := db.QueryRow(`SELECT id FROM groups WHERE name = ?`, "engineering").Scan(&gid); err != nil {
		t.Fatalf("locate created group: %v", err)
	}
	var memberIDs []int
	rows, err := db.Query(`SELECT user_id FROM group_members WHERE group_id = ?`, gid)
	if err != nil {
		t.Fatalf("query members: %v", err)
	}
	for rows.Next() {
		var uid int
		_ = rows.Scan(&uid)
		memberIDs = append(memberIDs, uid)
	}
	_ = rows.Close()
	if len(memberIDs) != 1 || memberIDs[0] != scimUser {
		t.Errorf("members: got %v want [%d] — local user was attached", memberIDs, scimUser)
	}

	// One member add audit row must record the rejected local-user insert as success=false.
	_ = waitForAuditCount(t, db, "scim.group.add_member", 2)
	var failCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM audit_logs
		WHERE action_type = 'scim.group.add_member' AND success = false
	`).Scan(&failCount)
	if err != nil {
		t.Fatalf("count failed add_member rows: %v", err)
	}
	if failCount != 1 {
		t.Errorf("failed add_member rows: got %d want 1", failCount)
	}
}

func TestPatchGroup_RemoveLocalMembership_KeepsRow(t *testing.T) {
	h, db := newSCIMTestHandler(t)
	scimUser := seedUser(t, db, "alice")
	localUser := seedLocalUser(t, db, "carol-local")
	gid := seedGroup(t, db, "team")
	seedGroupMember(t, db, gid, scimUser)
	seedLocalGroupMember(t, db, gid, localUser)

	// SCIM tries to remove the local user. Must be a no-op against the local row.
	body := models.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []models.SCIMPatchOp{
			{
				Op:    "remove",
				Path:  "members",
				Value: []interface{}{map[string]interface{}{"value": strconv.Itoa(localUser)}},
			},
		},
	}
	req := setPathValue(scimReq(http.MethodPatch, "/scim/v2/Groups/"+strconv.Itoa(gid), body), "id", strconv.Itoa(gid))
	rec := httptest.NewRecorder()
	h.PatchGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Local membership must still exist.
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?`, gid, localUser).Scan(&n)
	if n != 1 {
		t.Errorf("local membership rows: got %d want 1 — SCIM PATCH wiped a local membership", n)
	}
}
