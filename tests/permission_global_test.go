package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestGlobalPermissions_SystemAdmin tests that system administrators have full access.
func TestGlobalPermissions_SystemAdmin(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)

	// Create a test workspace (key max 10 chars)
	workspaceID, _ := CreateTestWorkspace(t, server, "Admin Test Workspace", shortKey("AT"))

	t.Run("SystemAdmin_CanAccessAllEndpoints", func(t *testing.T) {
		// Admin should be able to access the users list
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, "/users", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("SystemAdmin_CanAccessAllWorkspaces", func(t *testing.T) {
		// Admin should be able to access any workspace
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("SystemAdmin_CanManageUsers", func(t *testing.T) {
		// Admin can create users
		userData := map[string]interface{}{
			"email":      "sysadmin_test_user@test.com",
			"username":   "sysadmin_test_user",
			"first_name": "Test",
			"last_name":  "User",
			"is_active":  true,
			"password":   "testpass123",
		}
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodPost, "/users", userData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("SystemAdmin_CanCreateWorkspaces", func(t *testing.T) {
		workspaceData := map[string]interface{}{
			"name":        "Admin Created Workspace",
			"key":         shortKey("ACW"),
			"description": "Created by system admin",
		}
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("SystemAdmin_CanAccessAdminEndpoints", func(t *testing.T) {
		// Admin should be able to access permissions endpoint
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, "/permissions", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})
}

// TestGlobalPermissions_WorkspaceCreate tests the workspace.create global permission.
func TestGlobalPermissions_WorkspaceCreate(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a regular user without workspace.create permission
	userID, username, password := CreateTestUserWithCredentials(t, server, "ws_create_user", "ws_create@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("UserWithoutWorkspaceCreate_CannotCreateWorkspace", func(t *testing.T) {
		workspaceData := map[string]interface{}{
			"name":        "User Created Workspace",
			"key":         shortKey("UCW"),
			"description": "Should fail",
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("UserWithWorkspaceCreate_CanCreateWorkspace", func(t *testing.T) {
		// Grant workspace.create permission to user
		GrantGlobalPermission(t, server, userID, "workspace.create")

		// After granting permission, we need to get a new token because the
		// permission check happens on the server with its cached permission service.
		// Get a fresh token for the user to trigger cache refresh.
		freshToken := CreateBearerTokenForUser(t, server, username, password)

		workspaceData := map[string]interface{}{
			"name":        "User Created Workspace After Grant",
			"key":         shortKey("UCWG"),
			"description": "Should succeed",
		}
		resp := MakeAuthRequestWithToken(t, server, freshToken, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})
}

// TestGlobalPermissions_UserManage tests user management permissions.
// Note: Currently user management requires system.admin - the user.manage permission
// exists in the schema but isn't enforced by middleware. This test documents current behavior.
func TestGlobalPermissions_UserManage(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a regular user
	_, username, password := CreateTestUserWithCredentials(t, server, "user_manage_tester", "user_manage@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("AnyAuthenticatedUser_CanListUsers", func(t *testing.T) {
		// Any authenticated user should be able to list users (for issue assignment, mentions, etc.)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, "/users", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("NonAdmin_CannotCreateUsers", func(t *testing.T) {
		// User management requires system.admin, not a separate user.manage permission
		userData := map[string]interface{}{
			"email":      "should_fail@test.com",
			"username":   "should_fail_user",
			"first_name": "Should",
			"last_name":  "Fail",
			"is_active":  true,
			"password":   "testpass123",
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/users", userData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NonAdmin_CannotUpdateUsers", func(t *testing.T) {
		// Create another user to try to update
		targetUserID, _, _ := CreateTestUserWithCredentials(t, server, "target_user", "target@test.com")

		updateData := map[string]interface{}{
			"email":      "updated@test.com",
			"username":   "target_user",
			"first_name": "Updated",
			"last_name":  "Name",
			"is_active":  true,
		}
		endpoint := fmt.Sprintf("/users/%d", targetUserID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("SystemAdmin_CanCreateUsers", func(t *testing.T) {
		// System admin should be able to create users
		userData := map[string]interface{}{
			"email":      "created_by_admin@test.com",
			"username":   "created_by_admin",
			"first_name": "Created",
			"last_name":  "ByAdmin",
			"is_active":  true,
			"password":   "testpass123",
		}
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodPost, "/users", userData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("SystemAdmin_CanUpdateUsers", func(t *testing.T) {
		// Create a user to update
		targetUserID, _, _ := CreateTestUserWithCredentials(t, server, "update_target", "update_target@test.com")

		updateData := map[string]interface{}{
			"email":      "updated_by_admin@test.com",
			"username":   "update_target",
			"first_name": "Updated",
			"last_name":  "ByAdmin",
			"is_active":  true,
		}
		endpoint := fmt.Sprintf("/users/%d", targetUserID)
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})
}

// TestGlobalPermissions_IterationManage tests iteration creation.
// Note: The iteration.manage permission exists but is not enforced by middleware.
// Currently any authenticated user can create iterations (workspace or global).
// This test documents the current behavior.
func TestGlobalPermissions_IterationManage(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a workspace for local iterations
	workspaceID, _ := CreateTestWorkspace(t, server, "Iteration Test WS", shortKey("ITW"))

	// Create a regular user
	_, username, password := CreateTestUserWithCredentials(t, server, "iteration_tester", "iteration@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("AnyAuthenticatedUser_CanCreateGlobalIterations", func(t *testing.T) {
		// Currently any authenticated user can create global iterations
		iterationData := map[string]interface{}{
			"name":        "Global Iteration",
			"description": "Test global iteration",
			"start_date":  time.Now().Format("2006-01-02"),
			"end_date":    time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
			"is_global":   true,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/iterations", iterationData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("AnyAuthenticatedUser_CanCreateWorkspaceIterations", func(t *testing.T) {
		// Currently any authenticated user can create workspace iterations
		iterationData := map[string]interface{}{
			"name":         "Workspace Iteration",
			"description":  "Test workspace iteration",
			"start_date":   time.Now().Format("2006-01-02"),
			"end_date":     time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
			"is_global":    false,
			"workspace_id": workspaceID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/iterations", iterationData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("LocalIterations_RequireWorkspaceID", func(t *testing.T) {
		// Local iterations must have a workspace_id
		iterationData := map[string]interface{}{
			"name":        "Invalid Local Iteration",
			"description": "Should fail - no workspace_id",
			"start_date":  time.Now().Format("2006-01-02"),
			"end_date":    time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
			"is_global":   false,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/iterations", iterationData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("GlobalIterations_CannotHaveWorkspaceID", func(t *testing.T) {
		// Global iterations cannot have a workspace_id
		iterationData := map[string]interface{}{
			"name":         "Invalid Global Iteration",
			"description":  "Should fail - has workspace_id",
			"start_date":   time.Now().Format("2006-01-02"),
			"end_date":     time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
			"is_global":    true,
			"workspace_id": workspaceID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/iterations", iterationData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})
}

// TestGlobalPermissions_NonAdminCannotAccessAdminEndpoints tests that non-admin users
// cannot access admin-only endpoints.
func TestGlobalPermissions_NonAdminCannotAccessAdminEndpoints(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a regular user without any special permissions
	_, username, password := CreateTestUserWithCredentials(t, server, "regular_user", "regular@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("NonAdmin_CannotAccessPermissionsEndpoint", func(t *testing.T) {
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, "/permissions", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NonAdmin_CannotGrantPermissions", func(t *testing.T) {
		grantData := map[string]interface{}{
			"user_id":       1,
			"permission_id": 1,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/permissions/global/grant", grantData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NonAdmin_CannotAssignWorkspaceRoles", func(t *testing.T) {
		assignData := map[string]interface{}{
			"user_id":      1,
			"workspace_id": 1,
			"role_id":      1,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/workspace-roles/assign", assignData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}
