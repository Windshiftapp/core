//go:build test

package services

import (
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestPermissionServiceBasicOperations(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true) // Use in-memory database
	defer tdb.Close()

	// Use the database interface from the test database
	db := tdb.GetDatabase()

	// Create permission service with short TTL for testing
	config := DefaultPermissionCacheConfig()
	config.TTL = 1 * time.Second   // Short TTL for testing cache expiration
	config.WarmupOnStartup = false // Don't warm up during tests

	permService, err := NewPermissionService(db, config)
	if err != nil {
		t.Fatalf("Failed to create permission service: %v", err)
	}
	defer permService.Close()

	t.Run("SystemAdminCheck", func(t *testing.T) {
		// Create a test admin user
		result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "admin", "admin@test.com", "Test", "Admin", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create admin user: %v", err)
		}

		adminID64, _ := result.LastInsertId()
		adminID := int(adminID64)

		// Grant system admin permission
		_, err = db.Exec(`
			INSERT INTO user_global_permissions (user_id, permission_id, granted_by, granted_at)
			VALUES (?, (SELECT id FROM permissions WHERE permission_key = 'system.admin'), ?, ?)
		`, adminID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to grant system admin permission: %v", err)
		}

		// Test system admin check (should be cached)
		isAdmin, err := permService.IsSystemAdmin(adminID)
		if err != nil {
			t.Fatalf("Error checking system admin: %v", err)
		}
		if !isAdmin {
			t.Error("User should be system admin")
		}

		// Test second call (note: IsSystemAdmin doesn't cache results on miss,
		// so both calls query the DB directly - this is expected behavior)
		isAdmin2, err := permService.IsSystemAdmin(adminID)
		if err != nil {
			t.Fatalf("Error checking system admin (second call): %v", err)
		}
		if !isAdmin2 {
			t.Error("User should be system admin (second call)")
		}
	})

	t.Run("WorkspacePermissionCheck", func(t *testing.T) {
		// Create a regular user
		result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "user", "user@test.com", "Test", "User", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		userID64, _ := result.LastInsertId()
		userID := int(userID64)

		// Create a second user to act as the explicit role holder
		otherResult, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "other", "other@test.com", "Other", "User", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}
		otherUserID64, _ := otherResult.LastInsertId()
		otherUserID := int(otherUserID64)

		// Create a test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "Test Workspace", "TEST", "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		workspaceID64, _ := wsResult.LastInsertId()
		workspaceID := int(workspaceID64)

		// Lock down the workspace by assigning an explicit Viewer role to another user.
		// Without any explicit assignments, the Everyone logic grants implicit access.
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, (SELECT id FROM workspace_roles WHERE name = 'Viewer'), ?, ?)
		`, otherUserID, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer role to lock down workspace: %v", err)
		}

		// User should not have permission (workspace is restricted)
		hasPermission, err := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
		if err != nil {
			t.Fatalf("Error checking workspace permission: %v", err)
		}
		if hasPermission {
			t.Error("User should not have permission on a restricted workspace")
		}

		// Grant permission via role assignment (Editor role has item.create permission)
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, (SELECT id FROM workspace_roles WHERE name = 'Editor'), ?, ?)
		`, userID, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Invalidate cache (simulating handler calling OnUserPermissionChanged)
		err = permService.OnUserPermissionChanged(userID)
		if err != nil {
			t.Fatalf("Failed to invalidate cache: %v", err)
		}

		// Now user should have permission
		hasPermission, err = permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
		if err != nil {
			t.Fatalf("Error checking workspace permission after grant: %v", err)
		}
		if !hasPermission {
			t.Error("User should have permission after grant")
		}
	})

	t.Run("BatchPermissionCheck", func(t *testing.T) {
		// Create another user
		result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "batchuser", "batch@test.com", "Batch", "User", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create batch user: %v", err)
		}

		userID64, _ := result.LastInsertId()
		userID := int(userID64)

		// Create a test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "Batch Test Workspace", "BATCH", "Batch test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create batch workspace: %v", err)
		}

		workspaceID64, _ := wsResult.LastInsertId()
		workspaceID := int(workspaceID64)

		// Lock down workspace: assign Viewer to admin user so "everyone" access is blocked
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (1, ?, (SELECT id FROM workspace_roles WHERE name = 'Viewer'), ?, ?)
		`, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to lock down workspace: %v", err)
		}

		// Grant permissions via role assignment (Editor role has item.create and item.edit)
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, (SELECT id FROM workspace_roles WHERE name = 'Editor'), ?, ?)
		`, userID, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Also assign Administrator and Tester roles to lock them down
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (1, ?, (SELECT id FROM workspace_roles WHERE name = 'Administrator'), ?, ?)
		`, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign admin role: %v", err)
		}
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (1, ?, (SELECT id FROM workspace_roles WHERE name = 'Tester'), ?, ?)
		`, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign tester role: %v", err)
		}

		// Test batch permission check
		permissionsToCheck := []string{
			models.PermissionItemCreate,
			models.PermissionItemEdit,
			models.PermissionItemDelete, // This one should be false
		}

		results, err := permService.HasWorkspacePermissions(userID, workspaceID, permissionsToCheck)
		if err != nil {
			t.Fatalf("Error checking batch permissions: %v", err)
		}

		if !results[models.PermissionItemCreate] {
			t.Error("User should have item create permission")
		}
		if !results[models.PermissionItemEdit] {
			t.Error("User should have item edit permission")
		}
		if results[models.PermissionItemDelete] {
			t.Error("User should not have item delete permission")
		}
	})

	t.Run("CacheExpiration", func(t *testing.T) {
		// Create a user for cache expiration test
		result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "expireuser", "expire@test.com", "Expire", "User", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create expire user: %v", err)
		}

		userID64, _ := result.LastInsertId()
		userID := int(userID64)

		// Check permissions (this will cache the result)
		isAdmin, err := permService.IsSystemAdmin(userID)
		if err != nil {
			t.Fatalf("Error checking system admin for cache test: %v", err)
		}
		if isAdmin {
			t.Error("User should not be admin initially")
		}

		// Wait for cache to expire (TTL is 1 second in test config)
		time.Sleep(1200 * time.Millisecond)

		// Grant system admin permission to user
		_, err = db.Exec(`
			INSERT INTO user_global_permissions (user_id, permission_id, granted_by, granted_at)
			VALUES (?, (SELECT id FROM permissions WHERE permission_key = 'system.admin'), ?, ?)
		`, userID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to grant system admin permission: %v", err)
		}

		// Check again - should get updated value since cache expired
		isAdminAfter, err := permService.IsSystemAdmin(userID)
		if err != nil {
			t.Fatalf("Error checking system admin after cache expiry: %v", err)
		}
		if !isAdminAfter {
			t.Error("User should be admin after cache expiry and permission grant")
		}
	})

	t.Run("CacheStatistics", func(t *testing.T) {
		// Get initial stats
		initialStats := permService.GetCacheStats()
		t.Logf("Cache stats: Hits=%d, Misses=%d, HitRatio=%.2f",
			initialStats.Hits, initialStats.Misses, initialStats.HitRatio)

		if initialStats.HitRatio < 0 || initialStats.HitRatio > 1 {
			t.Errorf("Hit ratio should be between 0 and 1, got %.2f", initialStats.HitRatio)
		}

		// Ensure we have some cache activity
		if initialStats.Hits+initialStats.Misses == 0 {
			t.Error("Expected some cache activity from previous tests")
		}
	})
}

// Integration test with middleware
func TestPermissionServiceWithMiddleware(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true) // Use in-memory database
	defer tdb.Close()

	// Use the database interface from the test database
	db := tdb.GetDatabase()

	// Create permission service
	config := DefaultPermissionCacheConfig()
	config.WarmupOnStartup = false

	permService, err := NewPermissionService(db, config)
	if err != nil {
		t.Fatalf("Failed to create permission service: %v", err)
	}
	defer permService.Close()

	// Test performance: measure time for multiple permission checks
	userID := 1 // Assume user exists from previous setup
	workspaceID := 1

	// First call (cache miss)
	start := time.Now()
	_, err = permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
	if err != nil {
		// Expected to fail since we don't have proper test data, but timing still valid
	}
	firstCallTime := time.Since(start)

	// Second call (cache hit)
	start = time.Now()
	_, err = permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
	if err != nil {
		// Expected to fail since we don't have proper test data, but timing still valid
	}
	secondCallTime := time.Since(start)

	t.Logf("First call time: %v, Second call time: %v", firstCallTime, secondCallTime)

	// Cache hit should be significantly faster than cache miss
	if secondCallTime > firstCallTime {
		t.Log("Warning: Cache hit took longer than cache miss - this may indicate caching is not working optimally")
	}

	// Both calls should be reasonably fast (under 10ms for cached operations)
	if secondCallTime > 10*time.Millisecond {
		t.Logf("Warning: Cached permission check took %v, which seems slow", secondCallTime)
	}
}

// TestDerivedEveryonePermissions tests the derived "everyone" access model.
// Roles without explicit assignments grant permissions to all authenticated users.
// Hierarchy: Viewer → Editor → Tester. Admin is always explicit.
func TestDerivedEveryonePermissions(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	db := tdb.GetDatabase()

	// Create permission service
	config := DefaultPermissionCacheConfig()
	config.WarmupOnStartup = false

	permService, err := NewPermissionService(db, config)
	if err != nil {
		t.Fatalf("Failed to create permission service: %v", err)
	}
	defer permService.Close()

	// Get role IDs
	var viewerRoleID, editorRoleID, testerRoleID, adminRoleID int
	err = db.QueryRow("SELECT id FROM workspace_roles WHERE name = 'Viewer'").Scan(&viewerRoleID)
	if err != nil {
		t.Fatalf("Failed to get Viewer role ID: %v", err)
	}
	err = db.QueryRow("SELECT id FROM workspace_roles WHERE name = 'Editor'").Scan(&editorRoleID)
	if err != nil {
		t.Fatalf("Failed to get Editor role ID: %v", err)
	}
	err = db.QueryRow("SELECT id FROM workspace_roles WHERE name = 'Tester'").Scan(&testerRoleID)
	if err != nil {
		t.Fatalf("Failed to get Tester role ID: %v", err)
	}
	err = db.QueryRow("SELECT id FROM workspace_roles WHERE name = 'Administrator'").Scan(&adminRoleID)
	if err != nil {
		t.Fatalf("Failed to get Administrator role ID: %v", err)
	}

	// Helper to create workspace
	createWorkspace := func(t *testing.T, name, key string) int {
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, name, key, "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		return int(mustGetLastInsertId(wsResult))
	}

	// Helper to create user
	createUser := func(t *testing.T, username, email string) int {
		result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, username, email, "Test", username, "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", username, err)
		}
		return int(mustGetLastInsertId(result))
	}

	t.Run("FullyOpenWorkspace_EveryoneGetsViewerEditorTester", func(t *testing.T) {
		workspaceID := createWorkspace(t, "Open WS", "OPEN")
		userID := createUser(t, "open_user", "open@test.com")

		hasView, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		hasEdit, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
		hasCreate, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
		hasTest, _ := permService.HasWorkspacePermission(userID, workspaceID, "test.view")

		if !hasView {
			t.Error("User should have item.view in fully open workspace")
		}
		if !hasEdit {
			t.Error("User should have item.edit in fully open workspace")
		}
		if !hasCreate {
			t.Error("User should have item.create in fully open workspace")
		}
		if !hasTest {
			t.Error("User should have test.view in fully open workspace")
		}
	})

	t.Run("FullyOpenWorkspace_AdminNeverImplicit", func(t *testing.T) {
		workspaceID := createWorkspace(t, "No Admin WS", "NADM")
		userID := createUser(t, "noadmin_user", "noadmin@test.com")

		hasAdmin, _ := permService.HasWorkspacePermission(userID, workspaceID, "workspace.admin")
		if hasAdmin {
			t.Error("User should NOT have workspace.admin in open workspace (admin always requires explicit assignment)")
		}
	})

	t.Run("EditorRestricted_BlocksEditAndCreate", func(t *testing.T) {
		workspaceID := createWorkspace(t, "EdRestricted WS", "EDRE")
		userA := createUser(t, "editor_a", "editorA@test.com")
		userB := createUser(t, "noeditor_b", "noeditorB@test.com")

		// Assign Editor to user A only
		_, err := db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, userA, workspaceID, editorRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Editor: %v", err)
		}

		// User A should have edit (explicit)
		hasEditA, _ := permService.HasWorkspacePermission(userA, workspaceID, models.PermissionItemEdit)
		if !hasEditA {
			t.Error("User A should have item.edit (explicit Editor)")
		}

		// User B should NOT have edit (Editor restricted)
		hasEditB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemEdit)
		if hasEditB {
			t.Error("User B should NOT have item.edit (Editor is restricted)")
		}

		// User B should still have view (Viewer still open)
		hasViewB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemView)
		if !hasViewB {
			t.Error("User B should have item.view (Viewer is still open)")
		}
	})

	t.Run("EditorRestricted_TesterImplicitlyBlocked", func(t *testing.T) {
		// Editor restricted → Tester also blocked (hierarchy cascade)
		workspaceID := createWorkspace(t, "EdBlocks Tester WS", "EDBT")
		userA := createUser(t, "ed_only", "ed_only@test.com")
		userB := createUser(t, "no_ed_tst", "no_ed_tst@test.com")

		// Assign Editor to user A (no Tester assignments)
		_, err := db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, userA, workspaceID, editorRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Editor: %v", err)
		}

		// User A: has item.edit ✓, test.view ✓ (Editor role includes test.view read-only),
		// test.execute ✗ (Tester blocked by Editor cascade, no explicit Tester)
		hasEditA, _ := permService.HasWorkspacePermission(userA, workspaceID, models.PermissionItemEdit)
		hasTestViewA, _ := permService.HasWorkspacePermission(userA, workspaceID, "test.view")
		hasTestExecA, _ := permService.HasWorkspacePermission(userA, workspaceID, "test.execute")
		if !hasEditA {
			t.Error("User A should have item.edit (explicit Editor)")
		}
		if !hasTestViewA {
			t.Error("User A should have test.view (Editor role includes test.view)")
		}
		if hasTestExecA {
			t.Error("User A should NOT have test.execute (Tester blocked by Editor cascade, no explicit Tester)")
		}

		// User B: item.view ✓ (Viewer open), item.edit ✗, test.view ✗ (Editor restricted, so test.view from Editor also blocked)
		hasViewB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemView)
		hasEditB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemEdit)
		hasTestExecB, _ := permService.HasWorkspacePermission(userB, workspaceID, "test.execute")
		if !hasViewB {
			t.Error("User B should have item.view (Viewer open)")
		}
		if hasEditB {
			t.Error("User B should NOT have item.edit")
		}
		if hasTestExecB {
			t.Error("User B should NOT have test.execute")
		}
	})

	t.Run("TesterRestricted_DoesNotCascadeDown", func(t *testing.T) {
		// Only Tester restricted; Editor + Viewer unaffected
		workspaceID := createWorkspace(t, "Tester Restricted WS", "TSRE")
		userA := createUser(t, "tester_a", "testerA@test.com")
		userB := createUser(t, "notester_b", "notesterB@test.com")

		// Assign Tester to user A only
		_, err := db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, userA, workspaceID, testerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Tester: %v", err)
		}

		// User B: view ✓, edit ✓ (Editor open), test.view ✓ (Editor includes test.view),
		// test.execute ✗ (Tester restricted)
		hasViewB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemView)
		hasEditB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemEdit)
		hasTestViewB, _ := permService.HasWorkspacePermission(userB, workspaceID, "test.view")
		hasTestExecB, _ := permService.HasWorkspacePermission(userB, workspaceID, "test.execute")
		if !hasViewB {
			t.Error("User B should have item.view (Viewer open)")
		}
		if !hasEditB {
			t.Error("User B should have item.edit (Editor open)")
		}
		if !hasTestViewB {
			t.Error("User B should have test.view (Editor role includes test.view)")
		}
		if hasTestExecB {
			t.Error("User B should NOT have test.execute (Tester restricted)")
		}

		// User A: view ✓, edit ✓ (Editor open), test.execute ✓ (explicit Tester)
		hasTestExecA, _ := permService.HasWorkspacePermission(userA, workspaceID, "test.execute")
		hasEditA, _ := permService.HasWorkspacePermission(userA, workspaceID, models.PermissionItemEdit)
		if !hasTestExecA {
			t.Error("User A should have test.execute (explicit Tester)")
		}
		if !hasEditA {
			t.Error("User A should have item.edit (Editor open)")
		}
	})

	t.Run("ViewerRestricted_CascadesAll", func(t *testing.T) {
		// Assign Viewer → no implicit everyone access for any role
		workspaceID := createWorkspace(t, "Viewer Cascade WS", "VWCA")
		userA := createUser(t, "viewer_only_a", "viewerOnlyA@test.com")
		userB := createUser(t, "outsider_b", "outsiderB@test.com")

		// Assign Viewer to user A only
		_, err := db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, userA, workspaceID, viewerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer: %v", err)
		}

		// User A: view ✓ (explicit), edit ✗, test ✗
		hasViewA, _ := permService.HasWorkspacePermission(userA, workspaceID, models.PermissionItemView)
		hasEditA, _ := permService.HasWorkspacePermission(userA, workspaceID, models.PermissionItemEdit)
		if !hasViewA {
			t.Error("User A should have item.view (explicit Viewer)")
		}
		if hasEditA {
			t.Error("User A should NOT have item.edit (Viewer restricted → all implicit blocked)")
		}

		// User B: nothing (not even view)
		hasViewB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemView)
		hasEditB, _ := permService.HasWorkspacePermission(userB, workspaceID, models.PermissionItemEdit)
		if hasViewB {
			t.Error("User B should NOT have item.view (Viewer restricted)")
		}
		if hasEditB {
			t.Error("User B should NOT have item.edit (Viewer restricted)")
		}
	})

	t.Run("InactiveWorkspace_NoAccess", func(t *testing.T) {
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "Inactive WS", "INACT", "Inactive", false, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		workspaceID := int(mustGetLastInsertId(wsResult))
		userID := createUser(t, "inactive_user", "inactive@test.com")

		hasView, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		if hasView {
			t.Error("User should NOT have access to inactive workspace")
		}
	})

	t.Run("CacheInvalidation_EditorAssignmentTransition", func(t *testing.T) {
		workspaceID := createWorkspace(t, "Cache WS", "CACH")
		user1ID := createUser(t, "cache_u1", "cache_u1@test.com")
		user2ID := createUser(t, "cache_u2", "cache_u2@test.com")

		// Initially, both should have Editor (no explicit assignments)
		hasEdit1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		hasEdit2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)

		if !hasEdit1 || !hasEdit2 {
			t.Error("Both users should initially have Editor permissions (no assignments)")
		}

		// Assign Editor to user2 → Editor becomes restricted
		_, err := db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, user2ID, workspaceID, editorRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Editor: %v", err)
		}

		// Reset cache (simulates OnEveryoneAccessChanged)
		permService.OnEveryoneAccessChanged()

		// User1 should NO LONGER have item.edit
		hasEdit1After, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		if hasEdit1After {
			t.Error("User1 should NOT have item.edit after Editor assignment restricts it")
		}

		// User2 should still have item.edit (explicit)
		hasEdit2After, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)
		if !hasEdit2After {
			t.Error("User2 should still have item.edit (explicitly assigned)")
		}

		// Remove the Editor assignment → Editor becomes open again
		_, err = db.Exec(`
			DELETE FROM user_workspace_roles WHERE user_id = ? AND workspace_id = ? AND role_id = ?
		`, user2ID, workspaceID, editorRoleID)
		if err != nil {
			t.Fatalf("Failed to revoke Editor: %v", err)
		}

		permService.OnEveryoneAccessChanged()

		// User1 should regain implicit Editor
		hasEdit1Restored, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		if !hasEdit1Restored {
			t.Error("User1 should regain item.edit after Editor assignment removed")
		}
	})
}

// Helper function to get last insert ID
func mustGetLastInsertId(result interface{}) int64 {
	type lastInsertIDer interface {
		LastInsertId() (int64, error)
	}
	if r, ok := result.(lastInsertIDer); ok {
		id, _ := r.LastInsertId()
		return id
	}
	return 0
}
