package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestWorkspaceOperations tests the complete workspace CRUD lifecycle
func TestWorkspaceOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	var workspaceID int

	t.Run("CreateWorkspace", func(t *testing.T) {
		workspaceData := map[string]interface{}{
			"name":        "Test Workspace",
			"key":         shortKey("TEST"),
			"description": "A workspace for testing API functionality",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", "Test Workspace")
		AssertJSONField(t, result, "description", "A workspace for testing API functionality")

		// Store workspace ID for later tests
		if id, ok := result["id"].(float64); ok {
			workspaceID = int(id)
		} else {
			t.Fatal("Workspace ID not found in response")
		}
	})

	t.Run("GetAllWorkspaces", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/workspaces", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var workspaces []map[string]interface{}
		DecodeJSON(t, resp, &workspaces)

		if len(workspaces) == 0 {
			t.Error("Expected at least one workspace")
		}
	})

	t.Run("GetWorkspaceByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/workspaces/%d", workspaceID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var workspace map[string]interface{}
		DecodeJSON(t, resp, &workspace)

		AssertJSONField(t, workspace, "name", "Test Workspace")
	})

	t.Run("UpdateWorkspace", func(t *testing.T) {
		updateData := map[string]interface{}{
			"name":        "Updated Test Workspace",
			"description": "Updated description for testing",
		}

		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/workspaces/%d", workspaceID), updateData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		// Verify the update
		getResp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/workspaces/%d", workspaceID), nil)
		defer getResp.Body.Close()

		var workspace map[string]interface{}
		DecodeJSON(t, getResp, &workspace)

		AssertJSONField(t, workspace, "name", "Updated Test Workspace")
		AssertJSONField(t, workspace, "description", "Updated description for testing")
	})

	t.Run("DeleteWorkspace", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/workspaces/%d", workspaceID), nil)
		defer resp.Body.Close()

		// DELETE returns 204 No Content
		AssertStatusCode(t, resp, http.StatusNoContent)

		// Verify workspace is deleted
		getResp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/workspaces/%d", workspaceID), nil)
		defer getResp.Body.Close()

		AssertStatusCode(t, getResp, http.StatusNotFound)
	})
}

// TestCustomFieldOperations tests custom field CRUD operations
func TestCustomFieldOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	var textFieldID, selectFieldID int

	t.Run("CreateTextField", func(t *testing.T) {
		fieldData := map[string]interface{}{
			"name":        "Test Text Field",
			"field_type":  "text",
			"description": "A text field for testing",
			"required":    false,
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/admin/custom-fields", fieldData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", "Test Text Field")
		AssertJSONField(t, result, "field_type", "text")

		if id, ok := result["id"].(float64); ok {
			textFieldID = int(id)
		}
	})

	t.Run("CreateSelectField", func(t *testing.T) {
		fieldData := map[string]interface{}{
			"name":        "Priority Level",
			"field_type":  "select",
			"description": "Priority selection field",
			"required":    true,
			"options":     `["Low", "Medium", "High", "Critical"]`,
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/admin/custom-fields", fieldData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", "Priority Level")
		AssertJSONField(t, result, "field_type", "select")

		if id, ok := result["id"].(float64); ok {
			selectFieldID = int(id)
		}
	})

	t.Run("GetAllCustomFields", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/custom-fields", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var fields []map[string]interface{}
		DecodeJSON(t, resp, &fields)

		if len(fields) < 2 {
			t.Errorf("Expected at least 2 custom fields, got %d", len(fields))
		}
	})

	t.Run("UpdateCustomField", func(t *testing.T) {
		updateData := map[string]interface{}{
			"name":        "Updated Text Field",
			"field_type":  "text",
			"description": "Updated description",
			"required":    true,
		}

		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/admin/custom-fields/%d", textFieldID), updateData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("DeleteCustomFields", func(t *testing.T) {
		// Delete text field
		resp1 := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/admin/custom-fields/%d", textFieldID), nil)
		defer resp1.Body.Close()
		AssertStatusCode(t, resp1, http.StatusNoContent)

		// Delete select field
		resp2 := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/admin/custom-fields/%d", selectFieldID), nil)
		defer resp2.Body.Close()
		AssertStatusCode(t, resp2, http.StatusNoContent)
	})
}

// TestWorkItemOperations tests work item creation and management
func TestWorkItemOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	// Create test workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Item Test Workspace", shortKey("ITM"))

	var itemID int

	t.Run("CreateBasicItem", func(t *testing.T) {
		itemData := map[string]interface{}{
			"title":        "Test Item",
			"description":  "This is a test item for API validation",
			"workspace_id": workspaceID,
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "title", "Test Item")
		// Status and priority are now referenced by ID, check that status_name exists
		if _, ok := result["status_name"]; !ok {
			t.Log("Note: status_name not in response, item may use default status")
		}

		if id, ok := result["id"].(float64); ok {
			itemID = int(id)
		}
	})

	t.Run("GetAllItems", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/items", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatal("Response should have items array")
		}

		if len(items) == 0 {
			t.Error("Expected at least one item")
		}
	})

	t.Run("GetItemsByWorkspace", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?workspace_id=%d", workspaceID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		items := result["items"].([]interface{})
		for _, item := range items {
			itemMap := item.(map[string]interface{})
			wsID := int(itemMap["workspace_id"].(float64))
			if wsID != workspaceID {
				t.Errorf("Expected workspace_id %d, got %d", workspaceID, wsID)
			}
		}
	})

	t.Run("UpdateItemStatus", func(t *testing.T) {
		// Update the item title (status and priority are managed via status_id/priority_id)
		updateData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Updated Test Item",
			"description":  "Updated description for API validation",
		}

		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/items/%d", itemID), updateData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		// Verify update
		getResp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items/%d", itemID), nil)
		defer getResp.Body.Close()

		var item map[string]interface{}
		DecodeJSON(t, getResp, &item)

		AssertJSONField(t, item, "title", "Updated Test Item")
		AssertJSONField(t, item, "description", "Updated description for API validation")
	})

	t.Run("DeleteItem", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/items/%d", itemID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}

// TestWorkItemHierarchy tests parent-child relationships
func TestWorkItemHierarchy(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	// Create test workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Hierarchy Test Workspace", shortKey("HIR"))

	var epicID, storyID int

	t.Run("CreateEpic", func(t *testing.T) {
		epicData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Epic: User Authentication System",
			"description":  "Main epic for implementing user authentication",
			"status":       "open",
			"priority":     "high",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", epicData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			epicID = int(id)
		}

		// Verify no parent
		if parentID, exists := result["parent_id"]; exists && parentID != nil {
			t.Error("Epic should have no parent")
		}
	})

	t.Run("CreateStory", func(t *testing.T) {
		storyData := map[string]interface{}{
			"workspace_id": workspaceID,
			"parent_id":    epicID,
			"title":        "Story: User Registration Form",
			"description":  "Create user registration form with validation",
			"status":       "open",
			"priority":     "medium",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", storyData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			storyID = int(id)
		}

		// Verify parent
		parentID := int(result["parent_id"].(float64))
		if parentID != epicID {
			t.Errorf("Story should have epic as parent. Expected %d, got %d", epicID, parentID)
		}
	})

	t.Run("CreateTask", func(t *testing.T) {
		taskData := map[string]interface{}{
			"workspace_id": workspaceID,
			"parent_id":    storyID,
			"title":        "Task: Implement email validation",
			"description":  "Add email format validation to registration form",
			"status":       "open",
			"priority":     "low",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", taskData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify parent
		parentID := int(result["parent_id"].(float64))
		if parentID != storyID {
			t.Errorf("Task should have story as parent. Expected %d, got %d", storyID, parentID)
		}
	})

	t.Run("GetChildren", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items/%d/children", epicID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var children []map[string]interface{}
		DecodeJSON(t, resp, &children)

		if len(children) == 0 {
			t.Error("Epic should have children")
		}

		// All children should have epic as parent
		for _, child := range children {
			parentID := int(child["parent_id"].(float64))
			if parentID != epicID {
				t.Errorf("Child should have epic as parent. Expected %d, got %d", epicID, parentID)
			}
		}
	})

	t.Run("GetDescendants", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items/%d/descendants", epicID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var descendants []map[string]interface{}
		DecodeJSON(t, resp, &descendants)

		if len(descendants) < 2 {
			t.Errorf("Epic should have at least 2 descendants (story + task), got %d", len(descendants))
		}
	})

	t.Run("GetTree", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items/%d/tree", epicID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var tree map[string]interface{}
		DecodeJSON(t, resp, &tree)

		// Verify tree structure
		treeID := int(tree["id"].(float64))
		if treeID != epicID {
			t.Errorf("Tree root should be epic. Expected %d, got %d", epicID, treeID)
		}

		children, ok := tree["children"].([]interface{})
		if !ok {
			t.Fatal("Tree should have children array")
		}

		if len(children) == 0 {
			t.Error("Tree should have children")
		}
	})
}

// TestErrorHandling tests API error responses
func TestErrorHandling(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	t.Run("InvalidWorkspaceCreation", func(t *testing.T) {
		invalidData := map[string]interface{}{
			"name":        "", // Empty name should fail
			"description": "Invalid workspace",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", invalidData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("NonExistentResource", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/workspaces/99999", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidParentID", func(t *testing.T) {
		// First create a workspace
		workspaceData := map[string]interface{}{
			"name": "Error Test Workspace",
			"key":  shortKey("ERR"),
		}

		wsResp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		defer wsResp.Body.Close()

		var workspace map[string]interface{}
		DecodeJSON(t, wsResp, &workspace)
		workspaceID := int(workspace["id"].(float64))

		// Try to create item with invalid parent
		invalidItemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"parent_id":    99999, // Non-existent parent
			"title":        "Invalid Child Item",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", invalidItemData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusBadRequest)
	})
}

// TestBearerTokenAuth verifies that bearer tokens work for authentication,
// including state-changing requests (which are CSRF-exempt for bearer token auth).
func TestBearerTokenAuth(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	t.Run("BearerTokenStateChangingRequest", func(t *testing.T) {
		// Create a workspace using bearer token authentication (POST request)
		// Bearer auth is exempt from CSRF protection via ContextKeyCSRFExempt
		workspaceData := map[string]interface{}{
			"name":        "Bearer Auth Test Workspace",
			"key":         shortKey("BTKN"),
			"description": "Testing bearer token auth on POST",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", "Bearer Auth Test Workspace")
		AssertJSONField(t, result, "description", "Testing bearer token auth on POST")
	})

	t.Run("BearerTokenReadRequest", func(t *testing.T) {
		// Make a simple GET request to verify authentication works
		resp := MakeAuthRequest(t, server, http.MethodGet, "/workspaces", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)
	})
}
