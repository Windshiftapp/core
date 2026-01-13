package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestPortalWorkflow tests the complete portal functionality:
// 1. Create a portal channel
// 2. Create request types linked to item types
// 3. Configure fields for request types
// 4. Submit work items through the portal
// 5. Verify items are created correctly
func TestPortalWorkflow(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()

	// Variables to store IDs throughout the test
	var (
		workspaceID     int
		workspaceKey    string
		channelID       int
		configSetID     int
		bugItemTypeID   int
		storyItemTypeID int
		taskItemTypeID  int
		environmentFieldID int
		priorityFieldID    int
		bugReportTypeID    int
		featureRequestTypeID int
		supportRequestTypeID int
		portalSlug      string
	)

	// ===== SETUP PHASE =====
	t.Run("Setup", func(t *testing.T) {
		// Get default configuration set
		configSetID = GetDefaultConfigurationSet(t, server)
		t.Logf("Using configuration set: ID=%d", configSetID)

		// Create workspace
		workspaceID, workspaceKey = CreateTestWorkspace(t, server, fmt.Sprintf("Portal Test Workspace %d", timestamp), shortKey("PRT"))
		t.Logf("Created workspace: ID=%d, Key=%s", workspaceID, workspaceKey)

		// Get item types
		itemTypeMap := GetItemTypes(t, server, configSetID)
		bugItemTypeID = itemTypeMap["Bug"]
		storyItemTypeID = itemTypeMap["Story"]
		taskItemTypeID = itemTypeMap["Task"]

		if bugItemTypeID == 0 || storyItemTypeID == 0 || taskItemTypeID == 0 {
			t.Fatalf("Required item types not found. Bug=%d, Story=%d, Task=%d", bugItemTypeID, storyItemTypeID, taskItemTypeID)
		}

		t.Logf("Found item types: Bug=%d, Story=%d, Task=%d", bugItemTypeID, storyItemTypeID, taskItemTypeID)

		// Create custom fields
		environmentFieldID = CreateTestCustomField(t, server, fmt.Sprintf("Environment %d", timestamp), "select", `["Production", "Staging", "Development"]`)
		priorityFieldID = CreateTestCustomField(t, server, fmt.Sprintf("Priority Level %d", timestamp), "select", `["Low", "Medium", "High", "Critical"]`)

		t.Logf("Created custom fields: Environment=%d, Priority=%d", environmentFieldID, priorityFieldID)
	})

	// ===== CHANNEL CREATION =====
	t.Run("CreatePortalChannel", func(t *testing.T) {
		portalSlug = fmt.Sprintf("test-portal-%d", timestamp)

		// Create the channel first
		channelData := map[string]interface{}{
			"name":        fmt.Sprintf("Test Portal %d", timestamp),
			"type":        "portal",
			"direction":   "inbound",
			"description": "Portal for testing request type submissions",
			"status":      "active",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/channels", channelData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		channelID = ExtractIDFromResponse(t, result)

		t.Logf("Created portal channel: ID=%d", channelID)
	})

	t.Run("ConfigurePortalSettings", func(t *testing.T) {
		// Configure the portal with settings
		configData := map[string]interface{}{
			"name":   fmt.Sprintf("Test Portal %d", timestamp),
			"type":   "portal",
			"status": "active",
			"config": map[string]interface{}{
				"portal_slug":          portalSlug,
				"portal_enabled":       true,
				"portal_title":         "Test Support Portal",
				"portal_description":   "Submit bugs, feature requests, and support tickets",
				"portal_workspace_ids": []int{workspaceID},
			},
		}

		// Marshal config to JSON string
		configJSON, _ := json.Marshal(configData["config"])
		updateData := map[string]interface{}{
			"name":   configData["name"],
			"type":   configData["type"],
			"status": configData["status"],
			"config": string(configJSON),
		}

		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/channels/%d", channelID), updateData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		t.Logf("Configured portal with slug: %s", portalSlug)
	})

	// ===== REQUEST TYPE CREATION =====
	t.Run("CreateBugReportRequestType", func(t *testing.T) {
		requestTypeData := map[string]interface{}{
			"name":         "Bug Report",
			"description":  "Report a software bug or defect",
			"item_type_id": bugItemTypeID,
			"icon":         "Bug",
			"color":        "#ef4444",
			"is_active":    true,
		}

		endpoint := fmt.Sprintf("/channels/%d/request-types", channelID)
		resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, requestTypeData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			bugReportTypeID = int(id)
		} else {
			t.Fatal("Bug Report request type ID not found")
		}

		// Verify it's linked to the Bug item type
		if itemTypeID, ok := result["item_type_id"].(float64); ok {
			if int(itemTypeID) != bugItemTypeID {
				t.Errorf("Expected item_type_id=%d, got %d", bugItemTypeID, int(itemTypeID))
			}
		}

		t.Logf("Created Bug Report request type: ID=%d", bugReportTypeID)
	})

	t.Run("CreateFeatureRequestRequestType", func(t *testing.T) {
		requestTypeData := map[string]interface{}{
			"name":         "Feature Request",
			"description":  "Request a new feature or enhancement",
			"item_type_id": storyItemTypeID,
			"icon":         "Lightbulb",
			"color":        "#3b82f6",
			"is_active":    true,
		}

		endpoint := fmt.Sprintf("/channels/%d/request-types", channelID)
		resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, requestTypeData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			featureRequestTypeID = int(id)
		} else {
			t.Fatal("Feature Request type ID not found")
		}

		t.Logf("Created Feature Request type: ID=%d", featureRequestTypeID)
	})

	t.Run("CreateSupportRequestRequestType", func(t *testing.T) {
		requestTypeData := map[string]interface{}{
			"name":         "Support Request",
			"description":  "Get help with a technical issue",
			"item_type_id": taskItemTypeID,
			"icon":         "HelpCircle",
			"color":        "#8b5cf6",
			"is_active":    true,
		}

		endpoint := fmt.Sprintf("/channels/%d/request-types", channelID)
		resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, requestTypeData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			supportRequestTypeID = int(id)
		} else {
			t.Fatal("Support Request type ID not found")
		}

		t.Logf("Created Support Request type: ID=%d", supportRequestTypeID)
	})

	// ===== FIELD CONFIGURATION =====
	t.Run("ConfigureBugReportFields", func(t *testing.T) {
		fields := []map[string]interface{}{
			{
				"field_identifier": "title",
				"field_type":       "default",
				"display_order":    1,
				"is_required":      true,
			},
			{
				"field_identifier": "description",
				"field_type":       "default",
				"display_order":    2,
				"is_required":      true,
			},
			{
				"field_identifier": fmt.Sprintf("%d", environmentFieldID),
				"field_type":       "custom",
				"display_order":    3,
				"is_required":      true,
			},
			{
				"field_identifier": fmt.Sprintf("%d", priorityFieldID),
				"field_type":       "custom",
				"display_order":    4,
				"is_required":      false,
			},
		}

		endpoint := fmt.Sprintf("/request-types/%d/fields", bugReportTypeID)
		resp := MakeAuthRequest(t, server, http.MethodPut, endpoint, fields)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		t.Log("Configured Bug Report fields")
	})

	t.Run("ConfigureFeatureRequestFields", func(t *testing.T) {
		fields := []map[string]interface{}{
			{
				"field_identifier": "title",
				"field_type":       "default",
				"display_order":    1,
				"is_required":      true,
			},
			{
				"field_identifier": "description",
				"field_type":       "default",
				"display_order":    2,
				"is_required":      true,
			},
			{
				"field_identifier": fmt.Sprintf("%d", priorityFieldID),
				"field_type":       "custom",
				"display_order":    3,
				"is_required":      false,
			},
		}

		endpoint := fmt.Sprintf("/request-types/%d/fields", featureRequestTypeID)
		resp := MakeAuthRequest(t, server, http.MethodPut, endpoint, fields)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		t.Log("Configured Feature Request fields")
	})

	// ===== PORTAL SUBMISSION TESTS =====
	var createdBugItemID int

	t.Run("SubmitBugReportThroughPortal", func(t *testing.T) {
		submissionData := map[string]interface{}{
			"request_type_id": bugReportTypeID,
			"title":           "Login page crashes on mobile",
			"description":     "When attempting to login on mobile devices, the app crashes immediately after entering credentials.",
			"name":            "Test User",
			"email":           "testuser@example.com",
			"custom_fields": map[string]interface{}{
				fmt.Sprintf("%d", environmentFieldID): "Production",
				fmt.Sprintf("%d", priorityFieldID):    "High",
			},
		}

		endpoint := fmt.Sprintf("/portal/%s/submit", portalSlug)
		resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, submissionData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["item_id"].(float64); ok {
			createdBugItemID = int(id)
		} else {
			t.Fatal("Created item ID not found in portal submission response")
		}

		t.Logf("Submitted bug report through portal: Item ID=%d", createdBugItemID)
	})

	// ===== VALIDATION =====
	t.Run("VerifyCreatedBugItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", createdBugItemID)
		resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var item map[string]interface{}
		DecodeJSON(t, resp, &item)

		// Verify basic fields
		AssertJSONField(t, item, "title", "Login page crashes on mobile")
		AssertJSONField(t, item, "workspace_id", float64(workspaceID))

		// Verify item type
		if itemTypeID, ok := item["item_type_id"].(float64); ok {
			if int(itemTypeID) != bugItemTypeID {
				t.Errorf("Expected item_type_id=%d (Bug), got %d", bugItemTypeID, int(itemTypeID))
			}
		} else {
			t.Error("item_type_id not found in item response")
		}

		// Verify request type tracking
		if requestTypeID, ok := item["request_type_id"].(float64); ok {
			if int(requestTypeID) != bugReportTypeID {
				t.Errorf("Expected request_type_id=%d, got %d", bugReportTypeID, int(requestTypeID))
			}
		} else {
			t.Error("request_type_id not set on created item")
		}

		// Verify channel tracking
		if chanID, ok := item["channel_id"].(float64); ok {
			if int(chanID) != channelID {
				t.Errorf("Expected channel_id=%d, got %d", channelID, int(chanID))
			}
		} else {
			t.Error("channel_id not set on created item")
		}

		// Verify portal customer is set
		if _, ok := item["creator_portal_customer_id"]; !ok {
			t.Error("creator_portal_customer_id not set for anonymous portal submission")
		}

		// Verify custom field values
		if customFields, ok := item["custom_field_values"].(map[string]interface{}); ok {
			envFieldKey := fmt.Sprintf("%d", environmentFieldID)
			if env, ok := customFields[envFieldKey].(string); ok {
				if env != "Production" {
					t.Errorf("Expected environment='Production', got '%s'", env)
				}
			} else {
				t.Error("Environment custom field not found in custom_field_values")
			}

			priorityFieldKey := fmt.Sprintf("%d", priorityFieldID)
			if priority, ok := customFields[priorityFieldKey].(string); ok {
				if priority != "High" {
					t.Errorf("Expected priority='High', got '%s'", priority)
				}
			} else {
				t.Error("Priority custom field not found in custom_field_values")
			}
		} else {
			t.Error("custom_field_values not found or wrong type in item response")
		}

		t.Log("Successfully verified created bug item with all expected fields")
	})

	var createdFeatureItemID int

	t.Run("SubmitFeatureRequestThroughPortal", func(t *testing.T) {
		submissionData := map[string]interface{}{
			"request_type_id": featureRequestTypeID,
			"title":           "Add dark mode support",
			"description":     "Users have requested a dark mode option for better viewing at night.",
			"name":            "Another User",
			"email":           "anotheruser@example.com",
			"custom_fields": map[string]interface{}{
				fmt.Sprintf("%d", priorityFieldID): "Medium",
			},
		}

		endpoint := fmt.Sprintf("/portal/%s/submit", portalSlug)
		resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, submissionData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["item_id"].(float64); ok {
			createdFeatureItemID = int(id)
		}

		t.Logf("Submitted feature request through portal: Item ID=%d", createdFeatureItemID)
	})

	t.Run("VerifyCreatedFeatureItem", func(t *testing.T) {
		endpoint := fmt.Sprintf("/items/%d", createdFeatureItemID)
		resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var item map[string]interface{}
		DecodeJSON(t, resp, &item)

		AssertJSONField(t, item, "title", "Add dark mode support")

		// Verify it created a Story item type
		if itemTypeID, ok := item["item_type_id"].(float64); ok {
			if int(itemTypeID) != storyItemTypeID {
				t.Errorf("Expected item_type_id=%d (Story), got %d", storyItemTypeID, int(itemTypeID))
			}
		}

		// Verify request type
		if requestTypeID, ok := item["request_type_id"].(float64); ok {
			if int(requestTypeID) != featureRequestTypeID {
				t.Errorf("Expected request_type_id=%d, got %d", featureRequestTypeID, int(requestTypeID))
			}
		}

		t.Log("Successfully verified feature request item with Story item type")
	})

	t.Run("VerifyRequestTypeListing", func(t *testing.T) {
		endpoint := fmt.Sprintf("/channels/%d/request-types", channelID)
		resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var requestTypes []map[string]interface{}
		DecodeJSON(t, resp, &requestTypes)

		if len(requestTypes) != 3 {
			t.Errorf("Expected 3 request types, got %d", len(requestTypes))
		}

		// Verify all request types are present
		foundBug := false
		foundFeature := false
		foundSupport := false

		for _, rt := range requestTypes {
			name, _ := rt["name"].(string)
			switch name {
			case "Bug Report":
				foundBug = true
			case "Feature Request":
				foundFeature = true
			case "Support Request":
				foundSupport = true
			}
		}

		if !foundBug || !foundFeature || !foundSupport {
			t.Error("Not all request types found in listing")
		}

		t.Log("Successfully verified request type listing")
	})
}
