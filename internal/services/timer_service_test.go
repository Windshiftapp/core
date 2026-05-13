package services

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// timerTestEnv is a minimal in-memory environment for TimerService:
// two workspaces (one open, one gated), one user, one time project
// (open booking), and an item in each workspace.
type timerTestEnv struct {
	t           *testing.T
	db          database.Database
	svc         *TimerService
	repo        *repository.ActiveTimerRepository
	itemRepo    *repository.ItemRepository
	timePerm    *TimePermissionService
	permService *PermissionService

	userID         int
	otherUserID    int
	openWSID       int
	gatedWSID      int // a third user has the only role here → gated for userID
	openItemID     int // belongs to openWSID
	gatedItemID    int // belongs to gatedWSID
	customerID     int
	projectID      int
	inactiveProjID int
	itemTypeID     int
	statusID       int
}

func newTimerTestEnv(t *testing.T) *timerTestEnv {
	t.Helper()

	dsn := fmt.Sprintf("file:timersvc-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	permService, err := NewPermissionService(db, DefaultPermissionCacheConfig())
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	timePerm := NewTimePermissionService(db, permService)
	repo := repository.NewActiveTimerRepository(db)
	itemRepo := repository.NewItemRepository(db)

	env := &timerTestEnv{
		t:           t,
		db:          db,
		repo:        repo,
		itemRepo:    itemRepo,
		permService: permService,
		timePerm:    timePerm,
		svc:         NewTimerService(repo, itemRepo, timePerm, permService),
	}
	env.bootstrap()
	return env
}

func (e *timerTestEnv) bootstrap() {
	t := e.t
	t.Helper()
	exec := func(q string, args ...interface{}) int64 {
		t.Helper()
		res, err := e.db.Exec(q, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	// Users.
	e.userID = int(exec(`INSERT INTO users (email, username, first_name, last_name) VALUES ('alice@test', 'alice', 'alice', '')`))
	e.otherUserID = int(exec(`INSERT INTO users (email, username, first_name, last_name) VALUES ('bob@test', 'bob', 'bob', '')`))

	// Two workspaces. openWS stays open (no role assignments). gatedWS has
	// otherUser assigned to a permissions-bearing role, which flips it into
	// gated mode for everyone else.
	prefix := strings.ReplaceAll(t.Name(), "/", "_")
	e.openWSID = int(exec(`INSERT INTO workspaces (name, key, active, is_personal) VALUES (?, ?, 1, 0)`, prefix+"-Open", prefix+"O"))
	e.gatedWSID = int(exec(`INSERT INTO workspaces (name, key, active, is_personal) VALUES (?, ?, 1, 0)`, prefix+"-Gated", prefix+"G"))
	// Assign the seeded "Viewer" role to otherUser in the gated workspace.
	// Per project policy (see MEMORY: "Workspaces open by default") this
	// flips the workspace into gated mode and disables the everyone-Viewer
	// fallback for all other users — so HasWorkspacePermission(userID, …,
	// "item.view") will return false for the test user.
	var viewerRoleID int
	if err := e.db.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Viewer'`).Scan(&viewerRoleID); err != nil {
		t.Fatalf("lookup viewer role: %v", err)
	}
	exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?)`, e.otherUserID, e.gatedWSID, viewerRoleID)

	// Item type + status, then one item in each workspace. Items table requires
	// status_id; reuse the default-seeded status if any, otherwise create one.
	var catID int
	if err := e.db.QueryRow(`SELECT id FROM status_categories LIMIT 1`).Scan(&catID); err != nil {
		t.Fatalf("lookup status_categories: %v", err)
	}
	e.statusID = int(exec(`INSERT INTO statuses (name, category_id) VALUES (?, ?)`, prefix+"-Open", catID))
	e.itemTypeID = int(exec(`INSERT INTO item_types (name, hierarchy_level) VALUES (?, 3)`, prefix+"-Task"))

	e.openItemID = int(exec(`
		INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
		                   status_id, assignee_id, creator_id, custom_field_values)
		VALUES (?, 1, ?, 'Open Item', '', ?, ?, ?, '{}')`,
		e.openWSID, e.itemTypeID, e.statusID, e.userID, e.userID))
	e.gatedItemID = int(exec(`
		INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
		                   status_id, assignee_id, creator_id, custom_field_values)
		VALUES (?, 1, ?, 'Gated Item', '', ?, ?, ?, '{}')`,
		e.gatedWSID, e.itemTypeID, e.statusID, e.userID, e.userID))

	// Customer + project. No member rows → open booking for everyone.
	e.customerID = int(exec(`INSERT INTO customer_organisations (name) VALUES ('Acme')`))
	e.projectID = int(exec(`INSERT INTO time_projects (customer_id, name, status) VALUES (?, 'Project', 'Active')`, e.customerID))
	e.inactiveProjID = int(exec(`INSERT INTO time_projects (customer_id, name, status) VALUES (?, 'Old', 'Archived')`, e.customerID))
}

func TestTimerService_StartTimer_RejectsInaccessibleWorkspace(t *testing.T) {
	env := newTimerTestEnv(t)
	_, err := env.svc.StartTimer(env.userID, env.gatedWSID, env.projectID, nil, "work")
	if !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("expected ErrTimerNotFound, got %v", err)
	}
}

func TestTimerService_StartTimer_RejectsItemFromOtherWorkspace(t *testing.T) {
	env := newTimerTestEnv(t)
	// Caller has access to openWSID but supplies an item that lives in
	// gatedWSID. The cross-workspace check must reject this even though
	// the workspace itself is accessible.
	otherWSItem := env.gatedItemID
	_, err := env.svc.StartTimer(env.userID, env.openWSID, env.projectID, &otherWSItem, "work")
	if !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("expected ErrTimerNotFound, got %v", err)
	}
}

func TestTimerService_StartTimer_RejectsItemInGatedWorkspace(t *testing.T) {
	env := newTimerTestEnv(t)
	// Supplying both workspace and item from a gated workspace — the
	// workspace check fires first and we still get a 404-style error.
	gatedItem := env.gatedItemID
	_, err := env.svc.StartTimer(env.userID, env.gatedWSID, env.projectID, &gatedItem, "work")
	if !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("expected ErrTimerNotFound, got %v", err)
	}
}

func TestTimerService_StartTimer_HappyPath(t *testing.T) {
	env := newTimerTestEnv(t)
	item := env.openItemID
	timer, err := env.svc.StartTimer(env.userID, env.openWSID, env.projectID, &item, "work")
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if timer.WorkspaceID != env.openWSID {
		t.Errorf("workspace_id = %d, want %d", timer.WorkspaceID, env.openWSID)
	}
	if timer.ItemID == nil || *timer.ItemID != env.openItemID {
		t.Errorf("item_id = %v, want %d", timer.ItemID, env.openItemID)
	}
	if timer.Description != "work" {
		t.Errorf("description = %q, want 'work'", timer.Description)
	}
}

func TestTimerService_StartTimer_RejectsInactiveProject(t *testing.T) {
	env := newTimerTestEnv(t)
	_, err := env.svc.StartTimer(env.userID, env.openWSID, env.inactiveProjID, nil, "work")
	if !errors.Is(err, ErrTimerProjectInactive) {
		t.Fatalf("expected ErrTimerProjectInactive, got %v", err)
	}
}

func TestTimerService_StopActiveForUser_DropsRevokedItemLink(t *testing.T) {
	env := newTimerTestEnv(t)

	// Seed an active_timers row directly with an item from a workspace
	// the user can no longer access. (Simulates either a forged row from
	// the old buggy start_timer or mid-timer access revocation.)
	exec := func(q string, args ...interface{}) int64 {
		env.t.Helper()
		res, err := env.db.Exec(q, args...)
		if err != nil {
			env.t.Fatalf("exec %q: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	exec(`INSERT INTO active_timers (workspace_id, item_id, project_id, user_id, description, start_time_utc, created_at)
	      VALUES (?, ?, ?, ?, 'leaked', ?, ?)`,
		env.openWSID, env.gatedItemID, env.projectID, env.userID, 1000, 1000)

	res, err := env.svc.StopActiveForUser(env.userID)
	if err != nil {
		t.Fatalf("StopActiveForUser: %v", err)
	}
	if !res.WorklogCreated {
		t.Fatal("worklog not created")
	}
	// Verify the persisted worklog has no item_id — the suspect link was
	// dropped, not flushed into history.
	var itemID *int
	if err := env.db.QueryRow(`SELECT item_id FROM time_worklogs WHERE user_id = ? ORDER BY id DESC LIMIT 1`, env.userID).Scan(&itemID); err != nil {
		t.Fatalf("query worklog: %v", err)
	}
	if itemID != nil {
		t.Errorf("worklog item_id = %d, want NULL (revoked link should be dropped)", *itemID)
	}
}

func TestTimerService_StopActiveForUser_PreservesValidItemLink(t *testing.T) {
	env := newTimerTestEnv(t)
	item := env.openItemID
	if _, err := env.svc.StartTimer(env.userID, env.openWSID, env.projectID, &item, "work"); err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	res, err := env.svc.StopActiveForUser(env.userID)
	if err != nil {
		t.Fatalf("StopActiveForUser: %v", err)
	}
	if !res.WorklogCreated {
		t.Fatal("worklog not created")
	}
	var itemID *int
	if err := env.db.QueryRow(`SELECT item_id FROM time_worklogs WHERE user_id = ? ORDER BY id DESC LIMIT 1`, env.userID).Scan(&itemID); err != nil {
		t.Fatalf("query worklog: %v", err)
	}
	if itemID == nil || *itemID != env.openItemID {
		t.Errorf("worklog item_id = %v, want %d", itemID, env.openItemID)
	}
}

func TestTimerService_StopActiveForUser_NoTimer(t *testing.T) {
	env := newTimerTestEnv(t)
	_, err := env.svc.StopActiveForUser(env.userID)
	if !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("expected ErrTimerNotFound, got %v", err)
	}
}
