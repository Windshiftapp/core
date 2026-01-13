package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestWorkspaceRoles_Viewer tests that users with the Viewer role have correct permissions.
func TestWorkspaceRoles_Viewer(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a test workspace and lock it down
	workspaceID, _ := CreateTestWorkspace(t, server, "Viewer Test Workspace", shortKey("VTW"))
	LockDownWorkspace(t, server, workspaceID)

	// Create a viewer user and assign role
	viewerID, viewerUsername, viewerPassword := CreateTestUserWithCredentials(t, server, "viewer_user", "viewer@test.com")
	AssignWorkspaceRole(t, server, viewerID, workspaceID, "Viewer")
	viewerToken := CreateBearerTokenForUser(t, server, viewerUsername, viewerPassword)

	// Create a test item as admin for viewer to try to access
	testItemID := CreateTestItem(t, server, workspaceID, "Test Item for Viewer")

	t.Run("Viewer_CanViewWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Viewer_CanViewItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Viewer_CanViewSpecificItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Viewer_CannotCreateItems", func(t *testing.T) {
		// Get item type for creating item
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		itemData := map[string]interface{}{
			"title":        "Viewer Created Item",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("Viewer_CannotEditItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		updateData := map[string]interface{}{
			"title": "Updated by Viewer",
		}
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("Viewer_CannotDeleteItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodDelete, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("Viewer_CannotAdministerWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		updateData := map[string]interface{}{
			"name":        "Updated by Viewer",
			"key":         shortKey("VTW"),
			"description": "Should fail",
		}
		resp := MakeAuthRequestWithToken(t, server, viewerToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestWorkspaceRoles_Editor tests that users with the Editor role have correct permissions.
func TestWorkspaceRoles_Editor(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a test workspace and lock it down
	workspaceID, _ := CreateTestWorkspace(t, server, "Editor Test Workspace", shortKey("ETW"))
	LockDownWorkspace(t, server, workspaceID)

	// Create an editor user and assign role
	editorID, editorUsername, editorPassword := CreateTestUserWithCredentials(t, server, "editor_user", "editor@test.com")
	AssignWorkspaceRole(t, server, editorID, workspaceID, "Editor")
	editorToken := CreateBearerTokenForUser(t, server, editorUsername, editorPassword)

	// Get item type for creating items
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	t.Run("Editor_CanViewWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Editor_CanViewItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	var editorCreatedItemID int

	t.Run("Editor_CanCreateItems", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Editor Created Item",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)
		editorCreatedItemID = ExtractIDFromResponse(t, result)
	})

	t.Run("Editor_CanEditItems", func(t *testing.T) {
		if editorCreatedItemID == 0 {
			t.Skip("No item created to edit")
		}
		endpoint := fmt.Sprintf("/items/%d", editorCreatedItemID)
		updateData := map[string]interface{}{
			"title": "Updated by Editor",
		}
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Editor_CannotDeleteItems", func(t *testing.T) {
		if editorCreatedItemID == 0 {
			t.Skip("No item created to delete")
		}
		endpoint := fmt.Sprintf("/items/%d", editorCreatedItemID)
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodDelete, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("Editor_CannotAdministerWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		updateData := map[string]interface{}{
			"name":        "Updated by Editor",
			"key":         shortKey("ETW"),
			"description": "Should fail",
		}
		resp := MakeAuthRequestWithToken(t, server, editorToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestWorkspaceRoles_Administrator tests that users with the Administrator role have correct permissions.
func TestWorkspaceRoles_Administrator(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a test workspace and lock it down
	workspaceID, workspaceKey := CreateTestWorkspace(t, server, "Admin Test Workspace", shortKey("ATW"))
	LockDownWorkspace(t, server, workspaceID)

	// Create a workspace admin user and assign role
	wsAdminID, wsAdminUsername, wsAdminPassword := CreateTestUserWithCredentials(t, server, "ws_admin_user", "ws_admin@test.com")
	AssignWorkspaceRole(t, server, wsAdminID, workspaceID, "Administrator")
	wsAdminToken := CreateBearerTokenForUser(t, server, wsAdminUsername, wsAdminPassword)

	// Get item type for creating items
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	t.Run("Administrator_CanViewWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	var adminCreatedItemID int

	t.Run("Administrator_CanCreateItems", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Admin Created Item",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)
		adminCreatedItemID = ExtractIDFromResponse(t, result)
	})

	t.Run("Administrator_CanEditItems", func(t *testing.T) {
		if adminCreatedItemID == 0 {
			t.Skip("No item created to edit")
		}
		endpoint := fmt.Sprintf("/items/%d", adminCreatedItemID)
		updateData := map[string]interface{}{
			"title": "Updated by WS Admin",
		}
		resp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("Administrator_CanDeleteItems", func(t *testing.T) {
		// Create a new item to delete
		itemData := map[string]interface{}{
			"title":        "Item to Delete",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		createResp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodPost, "/items", itemData)
		var result map[string]interface{}
		DecodeJSON(t, createResp, &result)
		createResp.Body.Close()
		itemToDeleteID := ExtractIDFromResponse(t, result)

		endpoint := fmt.Sprintf("/items/%d", itemToDeleteID)
		resp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodDelete, endpoint, nil)
		defer resp.Body.Close()
		// Accept both 200 and 204 as success
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 200 or 204, got %d", resp.StatusCode)
		}
	})

	t.Run("Administrator_CanAdministerWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		updateData := map[string]interface{}{
			"name":        "Updated by WS Admin",
			"key":         workspaceKey, // Keep the same key
			"description": "Updated description",
		}
		resp := MakeAuthRequestWithToken(t, server, wsAdminToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})
}

// TestWorkspaceRoles_EveryoneRole tests the Everyone role feature.
func TestWorkspaceRoles_EveryoneRole(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a test workspace (starts with default Everyone role = Viewer)
	workspaceID, _ := CreateTestWorkspace(t, server, "Everyone Role Test", shortKey("ERTW"))

	// Create a user with no explicit role assignment
	_, noRoleUsername, noRolePassword := CreateTestUserWithCredentials(t, server, "no_role_user", "no_role@test.com")
	noRoleToken := CreateBearerTokenForUser(t, server, noRoleUsername, noRolePassword)

	t.Run("DefaultEveryoneRole_GrantsViewerAccess", func(t *testing.T) {
		// By default, all authenticated users should have Viewer access
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("DefaultEveryoneRole_CanViewItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("DefaultEveryoneRole_CannotCreateItems", func(t *testing.T) {
		// Default Everyone role is Viewer, so should not be able to create items
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		itemData := map[string]interface{}{
			"title":        "Everyone Created Item",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("EveryoneRoleRemoved_DeniesAccessToNoRoleUser", func(t *testing.T) {
		// Lock down the workspace (remove Everyone role)
		LockDownWorkspace(t, server, workspaceID)

		// User with no explicit role should now be denied
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("EveryoneRoleChanged_ToEditor_GrantsEditorToAll", func(t *testing.T) {
		// Create a new workspace for this test
		workspaceID2, _ := CreateTestWorkspace(t, server, "Editor Everyone Test", shortKey("EETW"))

		// Get Editor role ID
		roles := GetWorkspaceRoles(t, server)
		editorRoleID := roles["Editor"]

		// Set Everyone role to Editor
		SetEveryoneRole(t, server, workspaceID2, &editorRoleID)

		// Create another user with no explicit role
		_, anotherUsername, anotherPassword := CreateTestUserWithCredentials(t, server, "another_user", "another@test.com")
		anotherToken := CreateBearerTokenForUser(t, server, anotherUsername, anotherPassword)

		// Get item type for creating items
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		// This user should now be able to create items (Editor permission)
		itemData := map[string]interface{}{
			"title":        "Created via Everyone Editor Role",
			"workspace_id": workspaceID2,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, anotherToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})
}

// TestWorkspaceRoles_NoRole tests that users without any role cannot access locked workspaces.
func TestWorkspaceRoles_NoRole(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create a test workspace and lock it down immediately
	workspaceID, _ := CreateTestWorkspace(t, server, "No Role Test Workspace", shortKey("NRTW"))
	LockDownWorkspace(t, server, workspaceID)

	// Create a user with no role assignment
	_, noRoleUsername, noRolePassword := CreateTestUserWithCredentials(t, server, "norole_user", "norole@test.com")
	noRoleToken := CreateBearerTokenForUser(t, server, noRoleUsername, noRolePassword)

	// Create a test item as admin
	testItemID := CreateTestItem(t, server, workspaceID, "Test Item for No Role")

	t.Run("NoRole_CannotViewWorkspace", func(t *testing.T) {
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NoRole_CannotViewItems", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NoRole_CannotViewSpecificItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("NoRole_CannotCreateItems", func(t *testing.T) {
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		itemData := map[string]interface{}{
			"title":        "No Role Created Item",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}
