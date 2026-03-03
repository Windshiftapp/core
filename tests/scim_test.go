package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestSCIM_PublicEndpoints tests the unauthenticated SCIM discovery endpoints
func TestSCIM_PublicEndpoints(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server) // Need to complete setup first

	t.Run("ServiceProviderConfig", func(t *testing.T) {
		resp := MakeSCIMRequestNoAuth(t, server, http.MethodGet, "/scim/v2/ServiceProviderConfig")
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify required fields
		if schemas, ok := result["schemas"].([]interface{}); ok {
			if len(schemas) == 0 {
				t.Error("Expected schemas array to be non-empty")
			}
		} else {
			t.Error("Expected schemas field in ServiceProviderConfig")
		}

		// Check patch support
		if patch, ok := result["patch"].(map[string]interface{}); ok {
			if supported, ok := patch["supported"].(bool); !ok || !supported {
				t.Error("Expected patch.supported to be true")
			}
		}
	})

	t.Run("ResourceTypes", func(t *testing.T) {
		resp := MakeSCIMRequestNoAuth(t, server, http.MethodGet, "/scim/v2/ResourceTypes")
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// SCIM returns a list response with Resources array
		resources, ok := result["Resources"].([]interface{})
		if !ok {
			t.Fatal("Expected Resources array in response")
		}

		// Should have User and Group resource types
		if len(resources) < 2 {
			t.Errorf("Expected at least 2 resource types (User, Group), got %d", len(resources))
		}

		// Check for User resource type
		foundUser := false
		for _, r := range resources {
			rt := r.(map[string]interface{})
			if name, ok := rt["name"].(string); ok && name == "User" {
				foundUser = true
				break
			}
		}
		if !foundUser {
			t.Error("Expected User resource type")
		}
	})

	t.Run("Schemas", func(t *testing.T) {
		resp := MakeSCIMRequestNoAuth(t, server, http.MethodGet, "/scim/v2/Schemas")
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// SCIM returns a list response with Resources array
		resources, ok := result["Resources"].([]interface{})
		if !ok {
			t.Fatal("Expected Resources array in response")
		}

		// Should have at least User and Group schemas
		if len(resources) < 2 {
			t.Errorf("Expected at least 2 schemas, got %d", len(resources))
		}
	})
}

// TestSCIM_TokenManagement tests SCIM token CRUD via admin API
func TestSCIM_TokenManagement(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	var tokenID int

	t.Run("CreateToken", func(t *testing.T) {
		tokenData := map[string]interface{}{
			"name": "Test SCIM Token",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/admin/scim-tokens", tokenData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Token should be returned on creation
		if token, ok := result["token"].(string); !ok || token == "" {
			t.Error("Expected token to be returned on creation")
		}

		// Should have scim_ prefix
		if token, ok := result["token"].(string); ok {
			if len(token) < 5 || token[:5] != "scim_" {
				t.Errorf("Expected token to start with 'scim_', got prefix: %s", token[:5])
			}
		}

		// Get ID from nested scim_token object
		if scimToken, ok := result["scim_token"].(map[string]interface{}); ok {
			if id, ok := scimToken["id"].(float64); ok {
				tokenID = int(id)
			} else {
				t.Fatal("Token ID not found in scim_token")
			}
		} else {
			t.Fatal("scim_token not found in response")
		}
	})

	t.Run("ListTokens", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/admin/scim-tokens", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result []map[string]interface{}
		DecodeJSON(t, resp, &result)

		if len(result) == 0 {
			t.Error("Expected at least one SCIM token")
		}

		// Verify token hash is NOT exposed in list
		for _, token := range result {
			if _, hasHash := token["token_hash"]; hasHash {
				t.Error("Token hash should not be exposed in list response")
			}
		}
	})

	t.Run("GetToken", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/admin/scim-tokens/%d", tokenID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", "Test SCIM Token")
	})

	t.Run("RevokeToken", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/admin/scim-tokens/%d", tokenID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNoContent)

		// Verify token is revoked
		getResp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/admin/scim-tokens/%d", tokenID), nil)
		defer getResp.Body.Close()

		// Either 404 or inactive token
		if getResp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			DecodeJSON(t, getResp, &result)
			if isActive, ok := result["is_active"].(bool); ok && isActive {
				t.Error("Token should be inactive after revocation")
			}
		}
	})
}

// TestSCIM_Authentication tests that SCIM endpoints require proper authentication
func TestSCIM_Authentication(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	t.Run("NoAuthFails", func(t *testing.T) {
		resp := MakeSCIMRequestNoAuth(t, server, http.MethodGet, "/scim/v2/Users")
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusUnauthorized)
	})

	t.Run("InvalidTokenFails", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, "invalid_token", http.MethodGet, "/scim/v2/Users", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusUnauthorized)
	})

	t.Run("ValidTokenSucceeds", func(t *testing.T) {
		scimToken := CreateSCIMToken(t, server, "Auth Test Token")

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)
	})
}

// TestSCIM_UserLifecycle tests the complete SCIM user CRUD lifecycle
func TestSCIM_UserLifecycle(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)
	scimToken := CreateSCIMToken(t, server, "User Lifecycle Token")

	var userID string

	t.Run("CreateUser", func(t *testing.T) {
		userData := map[string]interface{}{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "scim.testuser@example.com",
			"externalId": "ext-12345",
			"name": map[string]string{
				"givenName":  "SCIM",
				"familyName": "TestUser",
			},
			"emails": []map[string]interface{}{
				{
					"value":   "scim.testuser@example.com",
					"primary": true,
				},
			},
			"active": true,
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Users", userData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify user data
		if userName, ok := result["userName"].(string); !ok || userName != "scim.testuser@example.com" {
			t.Errorf("Expected userName 'scim.testuser@example.com', got '%v'", result["userName"])
		}

		// Get ID for subsequent tests
		if id, ok := result["id"].(string); ok {
			userID = id
		} else {
			t.Fatal("User ID not found in response")
		}

		// Verify meta fields
		if meta, ok := result["meta"].(map[string]interface{}); ok {
			if resourceType, ok := meta["resourceType"].(string); !ok || resourceType != "User" {
				t.Error("Expected meta.resourceType to be 'User'")
			}
		} else {
			t.Error("Expected meta object in response")
		}
	})

	t.Run("GetUser", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, fmt.Sprintf("/scim/v2/Users/%s", userID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if userName, ok := result["userName"].(string); !ok || userName != "scim.testuser@example.com" {
			t.Errorf("Expected userName 'scim.testuser@example.com', got '%v'", result["userName"])
		}
	})

	t.Run("ListUsers", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify list response format
		if _, ok := result["totalResults"]; !ok {
			t.Error("Expected totalResults in list response")
		}
		if _, ok := result["Resources"]; !ok {
			t.Error("Expected Resources array in list response")
		}
	})

	t.Run("FilterUsers", func(t *testing.T) {
		// URL encode the filter parameter
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet,
			"/scim/v2/Users?filter=userName%20eq%20%22scim.testuser%40example.com%22", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		resources, ok := result["Resources"].([]interface{})
		if !ok {
			t.Fatal("Expected Resources array")
		}

		if len(resources) != 1 {
			t.Errorf("Expected exactly 1 user matching filter, got %d", len(resources))
		}
	})

	t.Run("UpdateUser_PUT", func(t *testing.T) {
		userData := map[string]interface{}{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "scim.testuser@example.com",
			"externalId": "ext-12345",
			"name": map[string]string{
				"givenName":  "Updated",
				"familyName": "User",
			},
			"emails": []map[string]interface{}{
				{
					"value":   "scim.testuser@example.com",
					"primary": true,
				},
			},
			"active": true,
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPut, fmt.Sprintf("/scim/v2/Users/%s", userID), userData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify update
		if name, ok := result["name"].(map[string]interface{}); ok {
			if givenName, ok := name["givenName"].(string); !ok || givenName != "Updated" {
				t.Errorf("Expected givenName 'Updated', got '%v'", name["givenName"])
			}
		}
	})

	t.Run("PatchUser", func(t *testing.T) {
		patchData := map[string]interface{}{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]interface{}{
				{
					"op":    "replace",
					"path":  "name.givenName",
					"value": "Patched",
				},
			},
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPatch, fmt.Sprintf("/scim/v2/Users/%s", userID), patchData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify patch
		if name, ok := result["name"].(map[string]interface{}); ok {
			if givenName, ok := name["givenName"].(string); !ok || givenName != "Patched" {
				t.Errorf("Expected givenName 'Patched', got '%v'", name["givenName"])
			}
		}
	})

	t.Run("DeactivateUser", func(t *testing.T) {
		// SCIM DELETE should deactivate, not hard delete
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodDelete, fmt.Sprintf("/scim/v2/Users/%s", userID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNoContent)

		// Verify user is deactivated (still exists but inactive)
		getResp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, fmt.Sprintf("/scim/v2/Users/%s", userID), nil)
		defer getResp.Body.Close()

		// User might return 404 or return with active=false depending on implementation
		if getResp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			DecodeJSON(t, getResp, &result)
			if active, ok := result["active"].(bool); ok && active {
				t.Error("User should be inactive after DELETE")
			}
		}
	})
}

// TestSCIM_GroupLifecycle tests the complete SCIM group CRUD lifecycle
func TestSCIM_GroupLifecycle(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)
	scimToken := CreateSCIMToken(t, server, "Group Lifecycle Token")

	var groupID string
	var memberUserID string

	// Create a user to add as a group member
	t.Run("Setup_CreateUser", func(t *testing.T) {
		userData := map[string]interface{}{
			"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName": "group.member@example.com",
			"name": map[string]string{
				"givenName":  "Group",
				"familyName": "Member",
			},
			"emails": []map[string]interface{}{
				{"value": "group.member@example.com", "primary": true},
			},
			"active": true,
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Users", userData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)
		memberUserID = result["id"].(string)
	})

	t.Run("CreateGroup", func(t *testing.T) {
		groupData := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"displayName": "SCIM Test Group",
			"externalId":  "ext-group-123",
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Groups", groupData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if displayName, ok := result["displayName"].(string); !ok || displayName != "SCIM Test Group" {
			t.Errorf("Expected displayName 'SCIM Test Group', got '%v'", result["displayName"])
		}

		groupID = result["id"].(string)
	})

	t.Run("GetGroup", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, fmt.Sprintf("/scim/v2/Groups/%s", groupID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if displayName, ok := result["displayName"].(string); !ok || displayName != "SCIM Test Group" {
			t.Errorf("Expected displayName 'SCIM Test Group', got '%v'", result["displayName"])
		}
	})

	t.Run("ListGroups", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Groups", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if _, ok := result["totalResults"]; !ok {
			t.Error("Expected totalResults in list response")
		}
	})

	t.Run("PatchGroup_AddMember", func(t *testing.T) {
		patchData := map[string]interface{}{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]interface{}{
				{
					"op":   "add",
					"path": "members",
					"value": []map[string]interface{}{
						{"value": memberUserID},
					},
				},
			},
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPatch, fmt.Sprintf("/scim/v2/Groups/%s", groupID), patchData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify member was added
		if members, ok := result["members"].([]interface{}); ok {
			if len(members) == 0 {
				t.Error("Expected at least one member in group")
			}
		}
	})

	t.Run("PatchGroup_RemoveMember", func(t *testing.T) {
		patchData := map[string]interface{}{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]interface{}{
				{
					"op":   "remove",
					"path": fmt.Sprintf("members[value eq \"%s\"]", memberUserID),
				},
			},
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPatch, fmt.Sprintf("/scim/v2/Groups/%s", groupID), patchData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)
	})

	t.Run("UpdateGroup_PUT", func(t *testing.T) {
		groupData := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"displayName": "Updated SCIM Group",
			"externalId":  "ext-group-123",
		}

		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPut, fmt.Sprintf("/scim/v2/Groups/%s", groupID), groupData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if displayName, ok := result["displayName"].(string); !ok || displayName != "Updated SCIM Group" {
			t.Errorf("Expected displayName 'Updated SCIM Group', got '%v'", result["displayName"])
		}
	})

	t.Run("DeleteGroup", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodDelete, fmt.Sprintf("/scim/v2/Groups/%s", groupID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNoContent)

		// Verify group is deleted
		getResp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, fmt.Sprintf("/scim/v2/Groups/%s", groupID), nil)
		defer getResp.Body.Close()

		AssertStatusCode(t, getResp, http.StatusNotFound)
	})
}

// TestSCIM_ErrorResponses tests that SCIM returns proper error responses
func TestSCIM_ErrorResponses(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)
	scimToken := CreateSCIMToken(t, server, "Error Test Token")

	t.Run("UserNotFound", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users/99999", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNotFound)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Verify SCIM error format
		if schemas, ok := result["schemas"].([]interface{}); ok {
			found := false
			for _, s := range schemas {
				if s == "urn:ietf:params:scim:api:messages:2.0:Error" {
					found = true
					break
				}
			}
			if !found {
				t.Error("Expected SCIM error schema in response")
			}
		}
	})

	t.Run("GroupNotFound", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Groups/99999", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DuplicateUser", func(t *testing.T) {
		userData := map[string]interface{}{
			"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName": "duplicate@example.com",
			"name":     map[string]string{"givenName": "Duplicate", "familyName": "User"},
			"emails":   []map[string]interface{}{{"value": "duplicate@example.com", "primary": true}},
			"active":   true,
		}

		// Create first user
		resp1 := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Users", userData)
		resp1.Body.Close()
		AssertStatusCode(t, resp1, http.StatusCreated)

		// Try to create duplicate
		resp2 := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Users", userData)
		defer resp2.Body.Close()

		AssertStatusCode(t, resp2, http.StatusConflict)
	})
}

// TestSCIM_Pagination tests SCIM pagination parameters
func TestSCIM_Pagination(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)
	scimToken := CreateSCIMToken(t, server, "Pagination Test Token")

	// Create multiple users
	for i := 0; i < 5; i++ {
		userData := map[string]interface{}{
			"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName": fmt.Sprintf("page.user%d@example.com", i),
			"name":     map[string]string{"givenName": fmt.Sprintf("User%d", i), "familyName": "Page"},
			"emails":   []map[string]interface{}{{"value": fmt.Sprintf("page.user%d@example.com", i), "primary": true}},
			"active":   true,
		}
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodPost, "/scim/v2/Users", userData)
		resp.Body.Close()
	}

	t.Run("CountParameter", func(t *testing.T) {
		resp := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users?count=2", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		resources, ok := result["Resources"].([]interface{})
		if !ok {
			t.Fatal("Expected Resources array")
		}

		if len(resources) > 2 {
			t.Errorf("Expected at most 2 resources with count=2, got %d", len(resources))
		}
	})

	t.Run("StartIndexParameter", func(t *testing.T) {
		// Get first page
		resp1 := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users?count=2&startIndex=1", nil)
		defer resp1.Body.Close()

		var result1 map[string]interface{}
		DecodeJSON(t, resp1, &result1)

		// Get second page
		resp2 := MakeSCIMRequest(t, server, scimToken, http.MethodGet, "/scim/v2/Users?count=2&startIndex=3", nil)
		defer resp2.Body.Close()

		var result2 map[string]interface{}
		DecodeJSON(t, resp2, &result2)

		// Results should be different
		resources1, _ := result1["Resources"].([]interface{})
		resources2, _ := result2["Resources"].([]interface{})

		if len(resources1) > 0 && len(resources2) > 0 {
			user1 := resources1[0].(map[string]interface{})
			user2 := resources2[0].(map[string]interface{})
			if user1["id"] == user2["id"] {
				t.Error("Expected different users on different pages")
			}
		}
	})
}
