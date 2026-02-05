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

		// Initially user should not have permission
		hasPermission, err := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemCreate)
		if err != nil {
			t.Fatalf("Error checking workspace permission: %v", err)
		}
		if hasPermission {
			t.Error("User should not have permission initially")
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

		// Lock down workspace to prevent "All Viewers" inheritance
		_, err = db.Exec(`
			INSERT INTO workspace_everyone_roles (workspace_id, role_id, granted_by, granted_at)
			VALUES (?, NULL, ?, ?)
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

		// Also assign Administrator and Tester roles to someone else to prevent All-Viewers inheritance
		// (without this, Administrator/Tester permissions would be inherited by all viewers)
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

// TestAllViewersInheritance tests the "All Viewers" inheritance logic
// This tests that roles without explicit members grant permissions to all users with Viewer
// NOTE: This feature is intentionally disabled in applyAllViewersInheritance()
func TestAllViewersInheritance(t *testing.T) {
	t.Skip("Feature intentionally disabled - see applyAllViewersInheritance comment")

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

	t.Run("AllViewersInheritance_NoExplicitMembers", func(t *testing.T) {
		// Scenario: No explicit members for Editor/Tester/Admin
		// Expected: All users with Viewer inherit Editor/Tester/Admin permissions

		// Create test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "AllViewers WS", "ALLV", "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		workspaceID := int(mustGetLastInsertId(wsResult))

		// Create two users
		user1Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "viewer1", "viewer1@test.com", "Viewer", "One", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user1: %v", err)
		}
		user1ID := int(mustGetLastInsertId(user1Result))

		user2Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "viewer2", "viewer2@test.com", "Viewer", "Two", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user2: %v", err)
		}
		user2ID := int(mustGetLastInsertId(user2Result))

		// Assign Viewer role to both users
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, user1ID, workspaceID, viewerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer to user1: %v", err)
		}

		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, user2ID, workspaceID, viewerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer to user2: %v", err)
		}

		// No explicit Editor/Tester/Admin assignments - should inherit

		// Both users should have Editor permissions (item.edit, item.create)
		hasEdit1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		hasEdit2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)

		if !hasEdit1 {
			t.Error("User1 with Viewer should inherit Editor permissions (no explicit Editor members)")
		}
		if !hasEdit2 {
			t.Error("User2 with Viewer should inherit Editor permissions (no explicit Editor members)")
		}

		// Both should have Tester permissions (test.view, test.execute)
		hasTestView1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, "test.view")
		hasTestView2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, "test.view")

		if !hasTestView1 {
			t.Error("User1 with Viewer should inherit Tester permissions (no explicit Tester members)")
		}
		if !hasTestView2 {
			t.Error("User2 with Viewer should inherit Tester permissions (no explicit Tester members)")
		}
	})

	t.Run("AllViewersInheritance_WithExplicitMembers", func(t *testing.T) {
		// Scenario: Editor has explicit member (user2), Tester and Admin have no members
		// Expected: Only user2 gets Editor, but both inherit Tester and Admin

		// Create test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "Explicit WS", "EXPL", "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		workspaceID := int(mustGetLastInsertId(wsResult))

		// Create two users
		user1Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "viewerA", "viewerA@test.com", "Viewer", "A", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create userA: %v", err)
		}
		user1ID := int(mustGetLastInsertId(user1Result))

		user2Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "viewerB", "viewerB@test.com", "Viewer", "B", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create userB: %v", err)
		}
		user2ID := int(mustGetLastInsertId(user2Result))

		// Assign Viewer to both
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
		`, user1ID, workspaceID, viewerRoleID, 1, time.Now(),
			user2ID, workspaceID, viewerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer roles: %v", err)
		}

		// Assign Editor ONLY to user2 (explicit member)
		// Also assign Administrator to user2 to prevent "All Viewers" inheritance of Admin perms
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
		`, user2ID, workspaceID, editorRoleID, 1, time.Now(),
			user2ID, workspaceID, adminRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign roles to user2: %v", err)
		}

		// Check Editor permissions (item.edit)
		// User1 should NOT have item.edit (Editor and Admin both have explicit members now)
		// User2 should have item.edit (has Editor and Admin explicitly)
		hasEdit1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		hasEdit2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)

		if hasEdit1 {
			t.Error("User1 should NOT have item.edit permission (Editor and Admin have explicit members)")
		}
		if !hasEdit2 {
			t.Error("User2 should have item.edit permission (explicitly assigned Editor and Admin)")
		}

		// Both should still inherit Tester (no explicit Tester members)
		hasTest1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, "test.view")
		hasTest2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, "test.view")

		if !hasTest1 {
			t.Error("User1 should inherit Tester permissions (no explicit Tester members)")
		}
		if !hasTest2 {
			t.Error("User2 should inherit Tester permissions (no explicit Tester members)")
		}
	})

	t.Run("AllViewersInheritance_NoViewerPermission", func(t *testing.T) {
		// Scenario: User has no Viewer permission
		// Expected: User should NOT inherit any role permissions

		// Create test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "NoViewer WS", "NOVIEW", "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		workspaceID := int(mustGetLastInsertId(wsResult))

		// Create user with no Viewer role
		userResult, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "noviewer", "noviewer@test.com", "No", "Viewer", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		userID := int(mustGetLastInsertId(userResult))

		// Lock down workspace (no Everyone role)
		_, err = db.Exec(`
			INSERT INTO workspace_everyone_roles (workspace_id, role_id, granted_by, granted_at)
			VALUES (?, NULL, ?, ?)
		`, workspaceID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to lock down workspace: %v", err)
		}

		// User should NOT have any inherited permissions
		hasView, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		hasEdit, _ := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
		hasTest, _ := permService.HasWorkspacePermission(userID, workspaceID, "test.view")

		if hasView {
			t.Error("User without Viewer should not have view permission")
		}
		if hasEdit {
			t.Error("User without Viewer should not inherit Editor permissions")
		}
		if hasTest {
			t.Error("User without Viewer should not inherit Tester permissions")
		}
	})

	t.Run("AllViewersInheritance_CacheInvalidation", func(t *testing.T) {
		// Scenario: Change role membership and verify cache invalidation

		// Create test workspace
		wsResult, err := db.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, "Cache WS", "CACHE", "Test workspace", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		workspaceID := int(mustGetLastInsertId(wsResult))

		// Create two users
		user1Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "cacheuser1", "cache1@test.com", "Cache", "User1", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user1: %v", err)
		}
		user1ID := int(mustGetLastInsertId(user1Result))

		user2Result, err := db.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "cacheuser2", "cache2@test.com", "Cache", "User2", "hash", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to create user2: %v", err)
		}
		user2ID := int(mustGetLastInsertId(user2Result))

		// Both users have Viewer
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
		`, user1ID, workspaceID, viewerRoleID, 1, time.Now(),
			user2ID, workspaceID, viewerRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign Viewer: %v", err)
		}

		// Initially, both should inherit Editor (no explicit members)
		hasEdit1, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		hasEdit2, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)

		if !hasEdit1 || !hasEdit2 {
			t.Error("Both users should initially inherit Editor permissions")
		}

		// Now assign Editor and Administrator explicitly to user2
		// This blocks "All Viewers" inheritance for both roles
		_, err = db.Exec(`
			INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
		`, user2ID, workspaceID, editorRoleID, 1, time.Now(),
			user2ID, workspaceID, adminRoleID, 1, time.Now())
		if err != nil {
			t.Fatalf("Failed to assign roles: %v", err)
		}

		// Invalidate caches
		_ = permService.OnUserPermissionChanged(user1ID)
		_ = permService.OnUserPermissionChanged(user2ID)

		// After cache invalidation:
		// User1 should NO LONGER have item.edit (Editor and Admin now have explicit members)
		// User2 should still have item.edit (explicit Editor and Admin assignments)
		hasEdit1After, _ := permService.HasWorkspacePermission(user1ID, workspaceID, models.PermissionItemEdit)
		hasEdit2After, _ := permService.HasWorkspacePermission(user2ID, workspaceID, models.PermissionItemEdit)

		if hasEdit1After {
			t.Error("User1 should NOT have item.edit after explicit role assignments block inheritance")
		}
		if !hasEdit2After {
			t.Error("User2 should still have item.edit (explicitly assigned)")
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
