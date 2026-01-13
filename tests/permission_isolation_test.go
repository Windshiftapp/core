package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestCrossWorkspaceIsolation tests that users cannot access workspaces they don't have roles in.
func TestCrossWorkspaceIsolation(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create two test workspaces and lock them down
	workspaceA_ID, _ := CreateTestWorkspace(t, server, "Workspace A", shortKey("WSA"))
	workspaceB_ID, _ := CreateTestWorkspace(t, server, "Workspace B", shortKey("WSB"))
	LockDownWorkspace(t, server, workspaceA_ID)
	LockDownWorkspace(t, server, workspaceB_ID)

	// Create items in both workspaces as admin
	itemInA_ID := CreateTestItem(t, server, workspaceA_ID, "Item in Workspace A")
	itemInB_ID := CreateTestItem(t, server, workspaceB_ID, "Item in Workspace B")

	// Create a user with Editor role in workspace A only
	userID, username, password := CreateTestUserWithCredentials(t, server, "ws_isolation_user", "ws_isolation@test.com")
	AssignWorkspaceRole(t, server, userID, workspaceA_ID, "Editor")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("UserWithRoleInWorkspaceA_CanAccessWorkspaceA", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceA_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("UserWithRoleInWorkspaceA_CannotAccessWorkspaceB", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceB_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("UserWithRoleInWorkspaceA_CanViewItemsInA", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", itemInA_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("UserWithRoleInWorkspaceA_CannotViewItemsInWorkspaceB", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", itemInB_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("UserWithRoleInWorkspaceA_CannotEditItemsInWorkspaceB", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", itemInB_ID)
		updateData := map[string]interface{}{
			"title": "Should Not Update",
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("UserWithRoleInWorkspaceA_CannotCreateItemsInWorkspaceB", func(t *testing.T) {
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		itemData := map[string]interface{}{
			"title":        "Should Not Create",
			"workspace_id": workspaceB_ID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestCrossWorkspaceIsolation_DifferentRolesPerWorkspace tests correct permissions per workspace.
func TestCrossWorkspaceIsolation_DifferentRolesPerWorkspace(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create two workspaces
	workspaceA_ID, workspaceA_Key := CreateTestWorkspace(t, server, "WS A Different Roles", shortKey("WSADR"))
	workspaceB_ID, _ := CreateTestWorkspace(t, server, "WS B Different Roles", shortKey("WSBDR"))
	LockDownWorkspace(t, server, workspaceA_ID)
	LockDownWorkspace(t, server, workspaceB_ID)

	// Create a user with Editor in A and Viewer in B
	userID, username, password := CreateTestUserWithCredentials(t, server, "multi_role_user", "multi_role@test.com")
	AssignWorkspaceRole(t, server, userID, workspaceA_ID, "Editor")
	AssignWorkspaceRole(t, server, userID, workspaceB_ID, "Viewer")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	// Get item type for creating items
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	t.Run("UserWithEditorInA_CanCreateItemsInA", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Created in A",
			"workspace_id": workspaceA_ID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("UserWithViewerInB_CannotCreateItemsInB", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Should Not Create in B",
			"workspace_id": workspaceB_ID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("UserWithViewerInB_CanViewItemsInB", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceB_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("UserWithAdminInA_NoRoleInC_CannotAdminC", func(t *testing.T) {
		// Create a third workspace and make user Admin in A
		workspaceC_ID, _ := CreateTestWorkspace(t, server, "WS C Admin Test", shortKey("WSC"))
		LockDownWorkspace(t, server, workspaceC_ID)

		// Create another user with Admin in A but no role in C
		adminUserID, adminUsername, adminPassword := CreateTestUserWithCredentials(t, server, "admin_in_a", "admin_in_a@test.com")
		AssignWorkspaceRole(t, server, adminUserID, workspaceA_ID, "Administrator")
		adminUserToken := CreateBearerTokenForUser(t, server, adminUsername, adminPassword)

		// Try to update workspace C (should fail)
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceC_ID)
		updateData := map[string]interface{}{
			"name":        "Should Not Update",
			"key":         workspaceA_Key, // Using any key
			"description": "Unauthorized update",
		}
		resp := MakeAuthRequestWithToken(t, server, adminUserToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestWorkspaceListFiltering tests that workspace list only returns accessible workspaces.
func TestWorkspaceListFiltering(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create three workspaces and lock them all down
	workspaceA_ID, _ := CreateTestWorkspace(t, server, "Filter WS A", shortKey("FWSA"))
	workspaceB_ID, _ := CreateTestWorkspace(t, server, "Filter WS B", shortKey("FWSB"))
	workspaceC_ID, _ := CreateTestWorkspace(t, server, "Filter WS C", shortKey("FWSC"))
	LockDownWorkspace(t, server, workspaceA_ID)
	LockDownWorkspace(t, server, workspaceB_ID)
	LockDownWorkspace(t, server, workspaceC_ID)

	// Create a user with Viewer in A only
	userID, username, password := CreateTestUserWithCredentials(t, server, "filter_test_user", "filter_test@test.com")
	AssignWorkspaceRole(t, server, userID, workspaceA_ID, "Viewer")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("ViewerInOneWorkspace_OnlySeesAccessibleWorkspaces", func(t *testing.T) {
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, "/workspaces", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var workspaces []map[string]interface{}
		DecodeJSON(t, resp, &workspaces)

		// User should only see workspace A
		foundA := false
		foundB := false
		foundC := false
		for _, ws := range workspaces {
			if id, ok := ws["id"].(float64); ok {
				if int(id) == workspaceA_ID {
					foundA = true
				}
				if int(id) == workspaceB_ID {
					foundB = true
				}
				if int(id) == workspaceC_ID {
					foundC = true
				}
			}
		}

		if !foundA {
			t.Errorf("Expected to find workspace A in results")
		}
		if foundB {
			t.Errorf("Should not find workspace B in results (no role)")
		}
		if foundC {
			t.Errorf("Should not find workspace C in results (no role)")
		}
	})

	t.Run("UserWithNoRoles_SeesNoWorkspaces_WhenAllLockedDown", func(t *testing.T) {
		// Create a user with no roles at all
		_, noRoleUsername, noRolePassword := CreateTestUserWithCredentials(t, server, "no_role_filter_user", "no_role_filter@test.com")
		noRoleToken := CreateBearerTokenForUser(t, server, noRoleUsername, noRolePassword)

		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, "/workspaces", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var workspaces []map[string]interface{}
		DecodeJSON(t, resp, &workspaces)

		// Filter out any workspaces this user might have access to
		// (should be empty since all 3 workspaces we created are locked down)
		accessibleCount := 0
		for _, ws := range workspaces {
			if id, ok := ws["id"].(float64); ok {
				wsID := int(id)
				if wsID == workspaceA_ID || wsID == workspaceB_ID || wsID == workspaceC_ID {
					accessibleCount++
				}
			}
		}

		if accessibleCount > 0 {
			t.Errorf("User with no roles should not see any of the locked-down workspaces, but found %d", accessibleCount)
		}
	})
}

// TestItemListFiltering tests that item list only returns items from accessible workspaces.
func TestItemListFiltering(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create two workspaces
	workspaceA_ID, _ := CreateTestWorkspace(t, server, "Item Filter WS A", shortKey("IFWSA"))
	workspaceB_ID, _ := CreateTestWorkspace(t, server, "Item Filter WS B", shortKey("IFWSB"))
	LockDownWorkspace(t, server, workspaceA_ID)
	LockDownWorkspace(t, server, workspaceB_ID)

	// Create items in both workspaces as admin
	itemInA_1 := CreateTestItem(t, server, workspaceA_ID, "Item A1")
	itemInA_2 := CreateTestItem(t, server, workspaceA_ID, "Item A2")
	itemInB_1 := CreateTestItem(t, server, workspaceB_ID, "Item B1")
	itemInB_2 := CreateTestItem(t, server, workspaceB_ID, "Item B2")

	// Create a user with Viewer in A only
	userID, username, password := CreateTestUserWithCredentials(t, server, "item_filter_user", "item_filter@test.com")
	AssignWorkspaceRole(t, server, userID, workspaceA_ID, "Viewer")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	t.Run("UserCanOnlySeeItemsFromAccessibleWorkspaces", func(t *testing.T) {
		// Request items filtered to workspace A
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceA_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should find items from A
		foundA1 := false
		foundA2 := false
		for _, item := range items {
			if id, ok := item["id"].(float64); ok {
				if int(id) == itemInA_1 {
					foundA1 = true
				}
				if int(id) == itemInA_2 {
					foundA2 = true
				}
			}
		}

		if !foundA1 || !foundA2 {
			t.Errorf("Expected to find both items from workspace A")
		}
	})

	t.Run("UserCannotQueryItemsFromInaccessibleWorkspace", func(t *testing.T) {
		// Try to request items from workspace B
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceB_ID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("GlobalItemQuery_ReturnsOnlyAccessibleItems", func(t *testing.T) {
		// Request all items (no workspace filter)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, "/items", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		items := result.Items

		// Check we only see items from workspace A, not B
		for _, item := range items {
			if id, ok := item["id"].(float64); ok {
				itemID := int(id)
				if itemID == itemInB_1 || itemID == itemInB_2 {
					t.Errorf("Should not see items from workspace B (item ID %d)", itemID)
				}
			}
		}
	})
}

// TestSystemAdminBypassesIsolation tests that system admins can access all workspaces.
func TestSystemAdminBypassesIsolation(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a locked-down workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Admin Bypass Test", shortKey("ABT"))
	LockDownWorkspace(t, server, workspaceID)

	// Create item in the workspace
	itemID := CreateTestItem(t, server, workspaceID, "Admin Bypass Item")

	t.Run("SystemAdmin_CanAccessLockedWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("SystemAdmin_CanAccessItemsInLockedWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", itemID)
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("SystemAdmin_CanModifyItemsInLockedWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", itemID)
		updateData := map[string]interface{}{
			"title": "Updated by System Admin",
		}
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("SystemAdmin_SeesAllWorkspacesInList", func(t *testing.T) {
		resp := MakeAuthRequestWithToken(t, server, adminToken, http.MethodGet, "/workspaces", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var workspaces []map[string]interface{}
		DecodeJSON(t, resp, &workspaces)

		// Admin should see the locked workspace
		found := false
		for _, ws := range workspaces {
			if id, ok := ws["id"].(float64); ok && int(id) == workspaceID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("System admin should see all workspaces including locked ones")
		}
	})
}
