package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// TestLinkTypeRestrictions tests that the "Tests" link type (ID=1)
// only allows links between items and test_cases, not between same entity types
func TestLinkTypeRestrictions(t *testing.T) {
	// Start test server with isolated database
	server, cleanup := StartTestServer(t, GetDBType())
	defer cleanup()

	// Create bearer token for authentication
	token := CreateBearerToken(t, server)

	// Helper function to make authenticated API requests
	makeAuthRequest := func(method, endpoint string, body interface{}) (*http.Response, error) {
		return makeRequest(t, method, server.APIBase+endpoint, token, body, nil), nil
	}

	// Step 1: Create a test workspace
	t.Log("Creating test workspace...")
	workspaceData := map[string]interface{}{
		"name":        "Test Workspace",
		"key":         "TEST",
		"description": "Workspace for link restriction testing",
	}
	workspaceResp, _ := makeAuthRequest(http.MethodPost, "/workspaces", workspaceData)
	if workspaceResp.StatusCode != http.StatusOK && workspaceResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(workspaceResp.Body)
		t.Fatalf("Failed to create workspace: %d - %s", workspaceResp.StatusCode, string(body))
	}

	var workspace struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(workspaceResp.Body).Decode(&workspace); err != nil {
		t.Fatalf("Failed to decode workspace response: %v", err)
	}
	workspaceResp.Body.Close()
	t.Logf("Created workspace with ID: %d", workspace.ID)

	// Step 2: Create work items
	t.Log("Creating work items...")
	item1Data := map[string]interface{}{
		"workspace_id": workspace.ID,
		"title":        "Work Item 1",
		"description":  "First work item for testing",
	}
	item1Resp, _ := makeAuthRequest(http.MethodPost, "/items", item1Data)
	if item1Resp.StatusCode != http.StatusOK && item1Resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(item1Resp.Body)
		t.Fatalf("Failed to create item 1: %d - %s", item1Resp.StatusCode, string(body))
	}

	var item1 struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(item1Resp.Body).Decode(&item1); err != nil {
		t.Fatalf("Failed to decode item 1 response: %v", err)
	}
	item1Resp.Body.Close()
	t.Logf("Created work item 1 with ID: %d", item1.ID)

	item2Data := map[string]interface{}{
		"workspace_id": workspace.ID,
		"title":        "Work Item 2",
		"description":  "Second work item for testing",
	}
	item2Resp, _ := makeAuthRequest(http.MethodPost, "/items", item2Data)
	if item2Resp.StatusCode != http.StatusOK && item2Resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(item2Resp.Body)
		t.Fatalf("Failed to create item 2: %d - %s", item2Resp.StatusCode, string(body))
	}

	var item2 struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(item2Resp.Body).Decode(&item2); err != nil {
		t.Fatalf("Failed to decode item 2 response: %v", err)
	}
	item2Resp.Body.Close()
	t.Logf("Created work item 2 with ID: %d", item2.ID)

	// Step 3: Create test cases
	t.Log("Creating test cases...")
	testCase1Data := map[string]interface{}{
		"title":  "Test Case 1",
		"name":   "TC-001",
		"status": "active",
	}
	testCase1Resp, _ := makeAuthRequest(http.MethodPost, fmt.Sprintf("/workspaces/%d/test-cases", workspace.ID), testCase1Data)
	if testCase1Resp.StatusCode != http.StatusOK && testCase1Resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(testCase1Resp.Body)
		t.Fatalf("Failed to create test case 1: %d - %s", testCase1Resp.StatusCode, string(body))
	}

	var testCase1 struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(testCase1Resp.Body).Decode(&testCase1); err != nil {
		t.Fatalf("Failed to decode test case 1 response: %v", err)
	}
	testCase1Resp.Body.Close()
	t.Logf("Created test case 1 with ID: %d", testCase1.ID)

	testCase2Data := map[string]interface{}{
		"title":  "Test Case 2",
		"name":   "TC-002",
		"status": "active",
	}
	testCase2Resp, _ := makeAuthRequest(http.MethodPost, fmt.Sprintf("/workspaces/%d/test-cases", workspace.ID), testCase2Data)
	if testCase2Resp.StatusCode != http.StatusOK && testCase2Resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(testCase2Resp.Body)
		t.Fatalf("Failed to create test case 2: %d - %s", testCase2Resp.StatusCode, string(body))
	}

	var testCase2 struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(testCase2Resp.Body).Decode(&testCase2); err != nil {
		t.Fatalf("Failed to decode test case 2 response: %v", err)
	}
	testCase2Resp.Body.Close()
	t.Logf("Created test case 2 with ID: %d", testCase2.ID)

	// Now run the actual link restriction tests
	t.Run("Valid: test_case to item", func(t *testing.T) {
		linkData := map[string]interface{}{
			"link_type_id": 1, // "Tests" link type
			"source_type":  "test_case",
			"source_id":    testCase1.ID,
			"target_type":  "item",
			"target_id":    item1.ID,
		}

		linkResp, _ := makeAuthRequest(http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		if linkResp.StatusCode != http.StatusOK && linkResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(linkResp.Body)
			t.Errorf("Expected success for test_case → item link, got %d: %s", linkResp.StatusCode, string(body))
		} else {
			t.Log("✓ Successfully created test_case → item link")
		}
	})

	t.Run("Valid: item to test_case", func(t *testing.T) {
		linkData := map[string]interface{}{
			"link_type_id": 1, // "Tests" link type
			"source_type":  "item",
			"source_id":    item2.ID,
			"target_type":  "test_case",
			"target_id":    testCase2.ID,
		}

		linkResp, _ := makeAuthRequest(http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		if linkResp.StatusCode != http.StatusOK && linkResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(linkResp.Body)
			t.Errorf("Expected success for item → test_case link, got %d: %s", linkResp.StatusCode, string(body))
		} else {
			t.Log("✓ Successfully created item → test_case link")
		}
	})

	t.Run("Invalid: item to item", func(t *testing.T) {
		linkData := map[string]interface{}{
			"link_type_id": 1, // "Tests" link type
			"source_type":  "item",
			"source_id":    item1.ID,
			"target_type":  "item",
			"target_id":    item2.ID,
		}

		linkResp, _ := makeAuthRequest(http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		if linkResp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(linkResp.Body)
			t.Errorf("Expected 400 Bad Request for item → item link with 'Tests' type, got %d: %s", linkResp.StatusCode, string(body))
		} else {
			body, _ := io.ReadAll(linkResp.Body)
			t.Logf("✓ Correctly rejected item → item link: %s", string(body))
		}
	})

	t.Run("Invalid: test_case to test_case", func(t *testing.T) {
		linkData := map[string]interface{}{
			"link_type_id": 1, // "Tests" link type
			"source_type":  "test_case",
			"source_id":    testCase1.ID,
			"target_type":  "test_case",
			"target_id":    testCase2.ID,
		}

		linkResp, _ := makeAuthRequest(http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		if linkResp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(linkResp.Body)
			t.Errorf("Expected 400 Bad Request for test_case → test_case link with 'Tests' type, got %d: %s", linkResp.StatusCode, string(body))
		} else {
			body, _ := io.ReadAll(linkResp.Body)
			t.Logf("✓ Correctly rejected test_case → test_case link: %s", string(body))
		}
	})

	t.Run("Other link types still work for item to item", func(t *testing.T) {
		// Try with link type 2 (should be "Implements" or another non-restricted type)
		linkData := map[string]interface{}{
			"link_type_id": 2,
			"source_type":  "item",
			"source_id":    item1.ID,
			"target_type":  "item",
			"target_id":    item2.ID,
		}

		linkResp, _ := makeAuthRequest(http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		if linkResp.StatusCode != http.StatusOK && linkResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(linkResp.Body)
			t.Errorf("Expected success for item → item link with non-Tests link type, got %d: %s", linkResp.StatusCode, string(body))
		} else {
			t.Log("✓ Other link types still work for item → item links")
		}
	})
}

// TestTestsLinkTypeIsSystemProtected verifies that the "Tests" link type
// cannot be deleted because it has is_system=true
func TestTestsLinkTypeIsSystemProtected(t *testing.T) {
	// Start test server with isolated database
	server, cleanup := StartTestServer(t, GetDBType())
	defer cleanup()

	// Create bearer token for authentication
	token := CreateBearerToken(t, server)

	// Try to delete link type ID=1 ("Tests")
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/admin/link-types/1", server.APIBase), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 403 Forbidden when deleting system link type, got %d: %s", resp.StatusCode, string(body))
	} else {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("✓ System link type correctly protected from deletion: %s", string(body))
	}
}
