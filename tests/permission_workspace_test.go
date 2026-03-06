package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestWorkspaceRoles_Viewer tests that users with the Viewer role have correct permissions.
func TestWorkspaceRoles_Viewer(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
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
	server, _ := StartTestServer(t, GetDBType())
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
	server, _ := StartTestServer(t, GetDBType())
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

// TestWorkspaceRoles_DerivedEveryone tests the derived "everyone" access model.
// A workspace with no explicit role assignments gives everyone Viewer+Editor+Tester.
// Adding the first assignment for a role restricts that role (and roles below it in the hierarchy).
func TestWorkspaceRoles_DerivedEveryone(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Get item type for creating items (shared across subtests)
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	t.Run("FullyOpenWorkspace_EveryoneGetsViewerEditorTester", func(t *testing.T) {
		// A brand-new workspace with zero assignments → everyone can view+edit+test
		workspaceID, _ := CreateTestWorkspace(t, server, "Fully Open WS", shortKey("FOWS"))

		_, username, password := CreateTestUserWithCredentials(t, server, "open_ws_user", "open_ws@test.com")
		userToken := CreateBearerTokenForUser(t, server, username, password)

		// Can view workspace
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		// Can create items (Editor perms)
		itemData := map[string]interface{}{
			"title":        "Created in open WS",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp2 := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp2.Body.Close()
		AssertStatusCode(t, resp2, http.StatusCreated)
	})

	t.Run("LockDown_DeniesAccess", func(t *testing.T) {
		// Assign Viewer to admin → restricts everyone else
		workspaceID, _ := CreateTestWorkspace(t, server, "Locked WS", shortKey("LKWS"))
		LockDownWorkspace(t, server, workspaceID)

		_, username, password := CreateTestUserWithCredentials(t, server, "locked_ws_user", "locked_ws@test.com")
		userToken := CreateBearerTokenForUser(t, server, username, password)

		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("EditorRestricted_BlocksCreate_AllowsView", func(t *testing.T) {
		// Assign Editor to a specific user → Editor restricted; Viewer still open
		workspaceID, _ := CreateTestWorkspace(t, server, "Editor Restricted WS", shortKey("ERWS"))
		editorID, _, _ := CreateTestUserWithCredentials(t, server, "editor_only", "editor_only@test.com")
		AssignWorkspaceRole(t, server, editorID, workspaceID, "Editor")

		_, noRoleUsername, noRolePassword := CreateTestUserWithCredentials(t, server, "norole_er", "norole_er@test.com")
		noRoleToken := CreateBearerTokenForUser(t, server, noRoleUsername, noRolePassword)

		// Can view (Viewer still open)
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		// Cannot create items (Editor restricted)
		itemData := map[string]interface{}{
			"title":        "Should Fail",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp2 := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPost, "/items", itemData)
		defer resp2.Body.Close()
		AssertStatusCode(t, resp2, http.StatusForbidden)
	})

	t.Run("ViewerRestricted_BlocksAll", func(t *testing.T) {
		// Assigning Viewer to a user restricts everyone else from all access
		workspaceID, _ := CreateTestWorkspace(t, server, "Viewer Restricted WS", shortKey("VRWS"))
		viewerID, _, _ := CreateTestUserWithCredentials(t, server, "viewer_only", "viewer_only@test.com")
		AssignWorkspaceRole(t, server, viewerID, workspaceID, "Viewer")

		_, outsiderUsername, outsiderPassword := CreateTestUserWithCredentials(t, server, "outsider", "outsider@test.com")
		outsiderToken := CreateBearerTokenForUser(t, server, outsiderUsername, outsiderPassword)

		// Cannot view
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, outsiderToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)

		// Cannot create
		itemData := map[string]interface{}{
			"title":        "Should Fail",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp2 := MakeAuthRequestWithToken(t, server, outsiderToken, http.MethodPost, "/items", itemData)
		defer resp2.Body.Close()
		AssertStatusCode(t, resp2, http.StatusForbidden)
	})

	t.Run("EditorRestricted_AdminNeverImplicit", func(t *testing.T) {
		// Even with fully open workspace, admin requires explicit assignment
		workspaceID, workspaceKey := CreateTestWorkspace(t, server, "Admin Never Implicit WS", shortKey("ANWS"))

		_, username, password := CreateTestUserWithCredentials(t, server, "not_admin", "not_admin@test.com")
		userToken := CreateBearerTokenForUser(t, server, username, password)

		// Cannot administer workspace
		endpoint := fmt.Sprintf("/workspaces/%d", workspaceID)
		updateData := map[string]interface{}{
			"name":        "Updated by Non-Admin",
			"key":         workspaceKey,
			"description": "Should fail",
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestWorkspaceRoles_OpenWorkspace_CanEditItems tests that a user with no explicit role
// can view and edit items in a fully open workspace (no role assignments).
func TestWorkspaceRoles_OpenWorkspace_CanEditItems(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create workspace with no role assignments → everyone can view+edit
	workspaceID, _ := CreateTestWorkspace(t, server, "Open Workspace Edit Test", shortKey("OWET"))

	// Create a user with no explicit role
	_, username, password := CreateTestUserWithCredentials(t, server, "open_ws_editor", "open_ws_editor@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	// Create an item as admin
	testItemID := CreateTestItem(t, server, workspaceID, "Item for Open WS Editor")

	t.Run("CanViewItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("CanEditItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		updateData := map[string]interface{}{
			"title": "Updated in open workspace",
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("CanCreateItem", func(t *testing.T) {
		configSetID := GetDefaultConfigurationSet(t, server)
		itemTypes := GetItemTypes(t, server, configSetID)
		var itemTypeID int
		for _, id := range itemTypes {
			itemTypeID = id
			break
		}

		itemData := map[string]interface{}{
			"title":        "Created in open workspace",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("CannotDeleteItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodDelete, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})
}

// TestWorkspaceRoles_EditorRoleRemoval_RegainsImplicitAccess tests that removing the
// last Editor assignment makes Editor open to everyone again.
func TestWorkspaceRoles_EditorRoleRemoval_RegainsImplicitAccess(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	adminToken := CreateBearerToken(t, server)
	server.BearerToken = adminToken

	// Create workspace (fully open by default)
	workspaceID, _ := CreateTestWorkspace(t, server, "Role Removal Test", shortKey("RRWS"))
	roles := GetWorkspaceRoles(t, server)
	editorRoleID := roles["Editor"]

	// Create a user
	userID, username, password := CreateTestUserWithCredentials(t, server, "role_removal_user", "role_removal@test.com")
	userToken := CreateBearerTokenForUser(t, server, username, password)

	// Create a second user who won't have explicit assignments
	_, noRoleUsername, noRolePassword := CreateTestUserWithCredentials(t, server, "norole_rr", "norole_rr@test.com")
	noRoleToken := CreateBearerTokenForUser(t, server, noRoleUsername, noRolePassword)

	// Create an item
	testItemID := CreateTestItem(t, server, workspaceID, "Item for Role Removal")

	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	t.Run("InitiallyEveryoneCanEdit", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Created by norole initially",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	// Assign Editor to user → Editor becomes restricted
	AssignWorkspaceRole(t, server, userID, workspaceID, "Editor")

	t.Run("AfterEditorAssignment_NoRoleCannotCreate", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Should fail after restriction",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusForbidden)
	})

	t.Run("AfterEditorAssignment_ExplicitEditorCanCreate", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Created by explicit editor",
			"workspace_id": workspaceID,
			"item_type_id": itemTypeID,
		}
		resp := MakeAuthRequestWithToken(t, server, userToken, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusCreated)
	})

	t.Run("AfterEditorAssignment_NoRoleCanStillView", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	// Revoke the Editor role → Editor becomes open again
	RevokeWorkspaceRole(t, server, userID, workspaceID, editorRoleID)

	t.Run("AfterEditorRevoked_EveryoneCanEditAgain", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", testItemID)
		updateData := map[string]interface{}{
			"title": "Updated after editor revoked",
		}
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodPut, endpoint, updateData)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})
}

// TestWorkspaceRoles_NoRole tests that users without any role cannot access locked workspaces.
func TestWorkspaceRoles_NoRole(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
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
		// Items endpoint returns 200 with empty list for inaccessible workspaces
		// (doesn't leak information about workspace existence)
		endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
		resp := MakeAuthRequestWithToken(t, server, noRoleToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		items, ok := result["items"].([]interface{})
		if !ok || len(items) != 0 {
			t.Error("Expected empty items list for inaccessible workspace")
		}
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
