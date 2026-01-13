package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// TestQLChildrenOf tests the childrenOf() QL function
func TestQLChildrenOf(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	var workspaceID, parentItemID, child1ID, child2ID, grandchildID, unrelatedID int

	// Create workspace
	t.Run("Setup", func(t *testing.T) {
		workspaceData := map[string]interface{}{
			"name":        "QL Test Workspace",
			"key":         "QLTEST",
			"description": "Workspace for testing QL functions",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			workspaceID = int(id)
		} else {
			t.Fatal("Workspace ID not found in response")
		}

		// Create parent item with high priority (priority_id: 1=Critical, 2=High, 3=Medium, 4=Low)
		parentData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Parent Task",
			"priority_id":  2, // High
		}

		parentResp := MakeAuthRequest(t, server, http.MethodPost, "/items", parentData)
		defer parentResp.Body.Close()

		AssertStatusCode(t, parentResp, http.StatusCreated)

		var parentResult map[string]interface{}
		DecodeJSON(t, parentResp, &parentResult)

		if id, ok := parentResult["id"].(float64); ok {
			parentItemID = int(id)
		} else {
			t.Fatal("Parent item ID not found")
		}

		// Create child 1 with medium priority
		child1Data := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Child 1",
			"parent_id":    parentItemID,
			"priority_id":  3, // Medium
		}

		child1Resp := MakeAuthRequest(t, server, http.MethodPost, "/items", child1Data)
		defer child1Resp.Body.Close()

		AssertStatusCode(t, child1Resp, http.StatusCreated)

		var child1Result map[string]interface{}
		DecodeJSON(t, child1Resp, &child1Result)

		if id, ok := child1Result["id"].(float64); ok {
			child1ID = int(id)
		} else {
			t.Fatal("Child 1 ID not found")
		}

		// Create child 2 with low priority
		child2Data := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Child 2",
			"parent_id":    parentItemID,
			"priority_id":  4, // Low
		}

		child2Resp := MakeAuthRequest(t, server, http.MethodPost, "/items", child2Data)
		defer child2Resp.Body.Close()

		AssertStatusCode(t, child2Resp, http.StatusCreated)

		var child2Result map[string]interface{}
		DecodeJSON(t, child2Resp, &child2Result)

		if id, ok := child2Result["id"].(float64); ok {
			child2ID = int(id)
		} else {
			t.Fatal("Child 2 ID not found")
		}

		// Create grandchild with high priority
		grandchildData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Grandchild",
			"parent_id":    child1ID,
			"priority_id":  2, // High
		}

		grandchildResp := MakeAuthRequest(t, server, http.MethodPost, "/items", grandchildData)
		defer grandchildResp.Body.Close()

		AssertStatusCode(t, grandchildResp, http.StatusCreated)

		var grandchildResult map[string]interface{}
		DecodeJSON(t, grandchildResp, &grandchildResult)

		if id, ok := grandchildResult["id"].(float64); ok {
			grandchildID = int(id)
		} else {
			t.Fatal("Grandchild ID not found")
		}

		// Create unrelated item with critical priority
		unrelatedData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Unrelated Task",
			"priority_id":  1, // Critical
		}

		unrelatedResp := MakeAuthRequest(t, server, http.MethodPost, "/items", unrelatedData)
		defer unrelatedResp.Body.Close()

		AssertStatusCode(t, unrelatedResp, http.StatusCreated)

		var unrelatedResult map[string]interface{}
		DecodeJSON(t, unrelatedResp, &unrelatedResult)

		if id, ok := unrelatedResult["id"].(float64); ok {
			unrelatedID = int(id)
		} else {
			t.Fatal("Unrelated item ID not found")
		}
	})

	// Test basic childrenOf query
	t.Run("BasicChildrenOf", func(t *testing.T) {
		// Find all descendants of high priority items
		qlQuery := `childrenOf("priority = high")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should return child1, child2, and grandchild (all descendants of parent which has priority=high)
		// Should NOT return parent itself or unrelated item
		foundChild1, foundChild2, foundGrandchild := false, false, false
		foundParent, foundUnrelated := false, false

		for _, item := range items {
			if id, ok := item["id"].(float64); ok {
				itemID := int(id)
				if itemID == child1ID {
					foundChild1 = true
				} else if itemID == child2ID {
					foundChild2 = true
				} else if itemID == grandchildID {
					foundGrandchild = true
				} else if itemID == parentItemID {
					foundParent = true
				} else if itemID == unrelatedID {
					foundUnrelated = true
				}
			}
		}

		if !foundChild1 {
			t.Error("Expected to find child1 in results")
		}
		if !foundChild2 {
			t.Error("Expected to find child2 in results")
		}
		if !foundGrandchild {
			t.Error("Expected to find grandchild in results")
		}
		if foundParent {
			t.Error("Should not find parent in results")
		}
		if foundUnrelated {
			t.Error("Should not find unrelated item in results")
		}
	})

	// Test childrenOf with empty result
	t.Run("ChildrenOfEmpty", func(t *testing.T) {
		// Query for children of items with priority that doesn't exist - use quoted string
		qlQuery := `childrenOf("priority = \"nonexistent\"")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)

		if len(result.Items) != 0 {
			t.Errorf("Expected 0 items, got %d", len(result.Items))
		}
	})

	// Test childrenOf combined with other filters
	t.Run("ChildrenOfCombined", func(t *testing.T) {
		// Find high priority descendants of high priority items
		qlQuery := `priority = high AND childrenOf("priority = high")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should only return grandchild (high priority descendant)
		if len(items) != 1 {
			t.Errorf("Expected 1 item, got %d", len(items))
		}

		if len(items) > 0 {
			if id, ok := items[0]["id"].(float64); ok {
				if int(id) != grandchildID {
					t.Errorf("Expected grandchild (ID %d), got ID %d", grandchildID, int(id))
				}
			}
		}
	})
}

// TestQLLinkedOf tests the linkedOf() QL function
func TestQLLinkedOf(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	var workspaceID, linkTypeID, parentItemID, linkedItemID int

	// Create workspace and link type
	t.Run("Setup", func(t *testing.T) {
		workspaceData := map[string]interface{}{
			"name":        "QL Link Test",
			"key":         "QLLINK",
			"description": "Workspace for testing QL link functions",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		if id, ok := result["id"].(float64); ok {
			workspaceID = int(id)
		} else {
			t.Fatal("Workspace ID not found in response")
		}

		// Create link type with forward/reverse labels
		linkTypeData := map[string]interface{}{
			"name":          "blocks",
			"forward_label": "blocks",
			"reverse_label": "is blocked by",
			"description":   "Blocking relationship",
		}

		linkTypeResp := MakeAuthRequest(t, server, http.MethodPost, "/link-types", linkTypeData)
		defer linkTypeResp.Body.Close()

		AssertStatusCode(t, linkTypeResp, http.StatusCreated)

		var linkTypeResult map[string]interface{}
		DecodeJSON(t, linkTypeResp, &linkTypeResult)

		if id, ok := linkTypeResult["id"].(float64); ok {
			linkTypeID = int(id)
		} else {
			t.Fatal("Link type ID not found")
		}

		// Create parent item with high priority (priority_id: 1=Critical, 2=High, 3=Medium, 4=Low)
		parentData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Parent Task",
			"priority_id":  2, // High
		}

		parentResp := MakeAuthRequest(t, server, http.MethodPost, "/items", parentData)
		defer parentResp.Body.Close()

		AssertStatusCode(t, parentResp, http.StatusCreated)

		var parentResult map[string]interface{}
		DecodeJSON(t, parentResp, &parentResult)

		if id, ok := parentResult["id"].(float64); ok {
			parentItemID = int(id)
		} else {
			t.Fatal("Parent item ID not found")
		}

		// Create linked item with medium priority
		linkedData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        "Linked Task",
			"priority_id":  3, // Medium
		}

		linkedResp := MakeAuthRequest(t, server, http.MethodPost, "/items", linkedData)
		defer linkedResp.Body.Close()

		AssertStatusCode(t, linkedResp, http.StatusCreated)

		var linkedResult map[string]interface{}
		DecodeJSON(t, linkedResp, &linkedResult)

		if id, ok := linkedResult["id"].(float64); ok {
			linkedItemID = int(id)
		} else {
			t.Fatal("Linked item ID not found")
		}

		// Create link from parent to linked item (parent blocks linked)
		linkData := map[string]interface{}{
			"link_type_id": linkTypeID,
			"source_type":  "item",
			"source_id":    parentItemID,
			"target_type":  "item",
			"target_id":    linkedItemID,
		}

		linkResp := MakeAuthRequest(t, server, http.MethodPost, "/links", linkData)
		defer linkResp.Body.Close()

		AssertStatusCode(t, linkResp, http.StatusCreated)
	})

	// Test linkedOf with forward direction
	t.Run("LinkedOfForward", func(t *testing.T) {
		// Find items blocked by high priority items
		qlQuery := `linkedOf("blocks", "priority = high")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should return the linked item (target of the link from parent)
		foundLinked := false
		for _, item := range items {
			if id, ok := item["id"].(float64); ok {
				if int(id) == linkedItemID {
					foundLinked = true
				}
			}
		}

		if !foundLinked {
			t.Error("Expected to find linked item in results")
		}
	})

	// Test linkedOf with reverse direction
	t.Run("LinkedOfReverse", func(t *testing.T) {
		// Find items that block medium priority items
		qlQuery := `linkedOf("is blocked by", "priority = medium")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should return the parent item (source of the link to linked item which has medium priority)
		foundParent := false
		for _, item := range items {
			if id, ok := item["id"].(float64); ok {
				if int(id) == parentItemID {
					foundParent = true
				}
			}
		}

		if !foundParent {
			t.Error("Expected to find parent item in results")
		}
	})

	// Test linkedOf with empty result
	t.Run("LinkedOfEmpty", func(t *testing.T) {
		// Query for links to items with priority that doesn't exist - use quoted string
		qlQuery := `linkedOf("blocks", "priority = \"nonexistent\"")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)

		if len(result.Items) != 0 {
			t.Errorf("Expected 0 items, got %d", len(result.Items))
		}
	})

	// Test linkedOf combined with other filters
	t.Run("LinkedOfCombined", func(t *testing.T) {
		// Find medium priority items linked from high priority items
		qlQuery := `priority = medium AND linkedOf("blocks", "priority = high")`
		encodedQL := url.QueryEscape(qlQuery)

		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &result)
		items := result.Items

		// Should return linked item (medium priority and is blocked by high priority parent)
		if len(items) != 1 {
			t.Errorf("Expected 1 item, got %d", len(items))
		}

		if len(items) > 0 {
			if id, ok := items[0]["id"].(float64); ok {
				if int(id) != linkedItemID {
					t.Errorf("Expected linked item (ID %d), got ID %d", linkedItemID, int(id))
				}
			}
		}
	})
}
