package services

import (
	"fmt"
	"strings"
	"testing"

	"windshift/internal/database"
)

// permTestEnv is the minimal setup for exercising group-derived permissions.
// SQLite in-memory + initialized schema (which seeds the permissions table
// including 'system.admin' and the default workspace roles).
type permTestEnv struct {
	t       *testing.T
	db      database.Database
	service *PermissionService
}

func newPermTestEnv(t *testing.T) *permTestEnv {
	t.Helper()

	dsn := fmt.Sprintf("file:permcache-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	svc, err := NewPermissionService(db, DefaultPermissionCacheConfig())
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	return &permTestEnv{t: t, db: db, service: svc}
}

func (e *permTestEnv) insertUser(email string) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, '')`, email, email, email)
	if err != nil {
		e.t.Fatalf("insert user %s: %v", email, err)
	}
	uid, _ := res.LastInsertId()
	return int(uid)
}

func (e *permTestEnv) insertGroup(name string, active bool) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO groups (name, is_active) VALUES (?, ?)`, name, active)
	if err != nil {
		e.t.Fatalf("insert group %s: %v", name, err)
	}
	gid, _ := res.LastInsertId()
	return int(gid)
}

func (e *permTestEnv) addGroupMember(groupID, userID int) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO group_members (group_id, user_id) VALUES (?, ?)`, groupID, userID); err != nil {
		e.t.Fatalf("add group member: %v", err)
	}
}

func (e *permTestEnv) permissionID(key string) int {
	e.t.Helper()
	var id int
	if err := e.db.QueryRow(`SELECT id FROM permissions WHERE permission_key = ?`, key).Scan(&id); err != nil {
		e.t.Fatalf("permission %s: %v", key, err)
	}
	return id
}

func (e *permTestEnv) grantUserGlobal(userID int, permKey string) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO user_global_permissions (user_id, permission_id) VALUES (?, ?)`, userID, e.permissionID(permKey)); err != nil {
		e.t.Fatalf("grant user global %s: %v", permKey, err)
	}
}

func (e *permTestEnv) grantGroupGlobal(groupID int, permKey string) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO group_global_permissions (group_id, permission_id) VALUES (?, ?)`, groupID, e.permissionID(permKey)); err != nil {
		e.t.Fatalf("grant group global %s: %v", permKey, err)
	}
}

func (e *permTestEnv) insertWorkspace(name string) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO workspaces (name, key, active, is_personal) VALUES (?, ?, 1, 0)`, name, name)
	if err != nil {
		e.t.Fatalf("insert workspace: %v", err)
	}
	wid, _ := res.LastInsertId()
	return int(wid)
}

func (e *permTestEnv) roleID(name string) int {
	e.t.Helper()
	var id int
	if err := e.db.QueryRow(`SELECT id FROM workspace_roles WHERE name = ?`, name).Scan(&id); err != nil {
		e.t.Fatalf("role %s: %v", name, err)
	}
	return id
}

func (e *permTestEnv) assignGroupWorkspaceRole(groupID, workspaceID, roleID int) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO group_workspace_roles (group_id, workspace_id, role_id) VALUES (?, ?, ?)`, groupID, workspaceID, roleID); err != nil {
		e.t.Fatalf("assign group workspace role: %v", err)
	}
}

func TestPermissionCache_IsSystemAdmin_DirectGrant(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("alice@example.com")
	env.grantUserGlobal(uid, "system.admin")

	got, err := env.service.IsSystemAdmin(uid)
	if err != nil {
		t.Fatalf("IsSystemAdmin: %v", err)
	}
	if !got {
		t.Fatal("expected direct grant to make user system admin")
	}
}

func TestPermissionCache_IsSystemAdmin_ViaActiveGroup(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("bob@example.com")
	gid := env.insertGroup("admins", true)
	env.addGroupMember(gid, uid)
	env.grantGroupGlobal(gid, "system.admin")

	got, err := env.service.IsSystemAdmin(uid)
	if err != nil {
		t.Fatalf("IsSystemAdmin: %v", err)
	}
	if !got {
		t.Fatal("expected group-granted system.admin (active group) to make user system admin")
	}
}

func TestPermissionCache_IsSystemAdmin_ViaInactiveGroup(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("carol@example.com")
	gid := env.insertGroup("disabled-admins", false)
	env.addGroupMember(gid, uid)
	env.grantGroupGlobal(gid, "system.admin")

	got, err := env.service.IsSystemAdmin(uid)
	if err != nil {
		t.Fatalf("IsSystemAdmin: %v", err)
	}
	if got {
		t.Fatal("expected inactive group not to confer system admin")
	}
}

func TestPermissionCache_GroupGlobalPermission_LoadedIntoCache(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("dave@example.com")
	gid := env.insertGroup("creators", true)
	env.addGroupMember(gid, uid)
	env.grantGroupGlobal(gid, "workspace.create")

	cache, err := env.service.GetUserEffectivePermissions(uid)
	if err != nil {
		t.Fatalf("GetUserPermissionCache: %v", err)
	}
	if !cache.GlobalPermissions["workspace.create"] {
		t.Fatalf("expected workspace.create in cache; got %v", cache.GlobalPermissions)
	}
}

func TestPermissionCache_GroupWorkspaceRole_InactiveGroupDenies(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("eve@example.com")
	wsID := env.insertWorkspace("ws-eve")
	gid := env.insertGroup("ws-viewers", false)
	env.addGroupMember(gid, uid)
	env.assignGroupWorkspaceRole(gid, wsID, env.roleID("Viewer"))

	cache, err := env.service.GetUserEffectivePermissions(uid)
	if err != nil {
		t.Fatalf("GetUserPermissionCache: %v", err)
	}
	if cache.WorkspacePermissions[wsID]["item.view"] {
		t.Fatalf("inactive group must not grant item.view; got %v", cache.WorkspacePermissions[wsID])
	}
	for _, m := range cache.GroupMemberships {
		if m == gid {
			t.Fatalf("inactive group %d should not appear in cached memberships", gid)
		}
	}
}

func TestPermissionCache_GroupWorkspaceRole_ActiveGroupGrants(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("frank@example.com")
	wsID := env.insertWorkspace("ws-frank")
	gid := env.insertGroup("ws-viewers", true)
	env.addGroupMember(gid, uid)
	env.assignGroupWorkspaceRole(gid, wsID, env.roleID("Viewer"))

	cache, err := env.service.GetUserEffectivePermissions(uid)
	if err != nil {
		t.Fatalf("GetUserPermissionCache: %v", err)
	}
	if !cache.WorkspacePermissions[wsID]["item.view"] {
		t.Fatalf("active group should grant item.view; got %v", cache.WorkspacePermissions[wsID])
	}
}

// Regression: getGroupMembers used to query a non-existent `user_groups` table.
// OnGroupPermissionChanged calls it via InvalidateGroupMemberCaches; if the
// helper raises a SQL error, no caches get invalidated and stale grants leak.
func TestPermissionCache_OnGroupPermissionChanged_InvalidatesMembers(t *testing.T) {
	env := newPermTestEnv(t)
	uid := env.insertUser("grace@example.com")
	gid := env.insertGroup("globals", true)
	env.addGroupMember(gid, uid)

	// Prime the cache so we can observe invalidation. getUserPermissionCache
	// returns an error on cache miss; nil error == cached.
	if _, err := env.service.GetUserEffectivePermissions(uid); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if _, err := env.service.getUserPermissionCache(uid); err != nil {
		t.Fatalf("expected cache primed for user, got %v", err)
	}

	if err := env.service.OnGroupPermissionChanged(gid); err != nil {
		t.Fatalf("OnGroupPermissionChanged: %v", err)
	}
	if _, err := env.service.getUserPermissionCache(uid); err == nil {
		t.Fatal("expected user cache invalidated after OnGroupPermissionChanged")
	}
}
