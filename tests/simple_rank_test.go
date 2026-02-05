package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestSimpleRankingTest performs a simple test to verify lexicographical ranking works
func TestSimpleRankingTest(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	// Create a workspace
	workspaceData := map[string]interface{}{
		"name": "Simple Test Workspace",
		"key":  shortKey("SIMPLE"),
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
	var workspace map[string]interface{}
	DecodeJSON(t, resp, &workspace)
	resp.Body.Close()
	workspaceID := int(workspace["id"].(float64))

	// Create just 10 items
	t.Log("Creating 10 items...")
	var items []map[string]interface{}
	for i := 0; i < 10; i++ {
		itemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        fmt.Sprintf("Item %d", i+1),
			"status":       "open",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		var item map[string]interface{}
		DecodeJSON(t, resp, &item)
		resp.Body.Close()
		items = append(items, item)
	}

	// Test: insert between every pair of items
	t.Log("Testing insertions between every pair...")
	rebalanceCount := 0
	successCount := 0

	// Get fresh item list
	itemsResp := MakeAuthRequest(t, server, http.MethodGet,
		fmt.Sprintf("/items?workspace_id=%d&parent_id=null", workspaceID), nil)
	var itemsResult map[string]interface{}
	DecodeJSON(t, itemsResp, &itemsResult)
	itemsResp.Body.Close()

	itemsList := itemsResult["items"].([]interface{})
	items = nil
	for _, item := range itemsList {
		items = append(items, item.(map[string]interface{}))
	}

	// Try inserting between each pair
	for i := 0; i < len(items)-1; i++ {
		item1 := items[i]
		item2 := items[i+1]

		// Move item 0 between item1 and item2
		itemToMove := items[0]

		rerankData := map[string]interface{}{
			"prev_item_id": int(item1["id"].(float64)),
			"next_item_id": int(item2["id"].(float64)),
		}

		rerankResp := MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", int(itemToMove["id"].(float64))), rerankData)

		if rerankResp.StatusCode == http.StatusOK {
			successCount++
			var result map[string]interface{}
			DecodeJSON(t, rerankResp, &result)
			// Response returns full item with frac_index field
			if fracIndex, ok := result["frac_index"].(string); ok {
				t.Logf("  Inserted between frac_index %v and %v -> got %s",
					item1["frac_index"], item2["frac_index"], fracIndex)
			}
		} else {
			var result map[string]interface{}
			DecodeJSON(t, rerankResp, &result)
			if err, ok := result["error"].(string); ok {
				if err == "rebalance required" {
					rebalanceCount++
					t.Logf("  Rebalance required between %v and %v",
						item1["frac_index"], item2["frac_index"])
				}
			}
		}
		rerankResp.Body.Close()
	}

	t.Log("========================================")
	t.Logf("RESULTS: %d successful insertions, %d rebalances required",
		successCount, rebalanceCount)
	t.Log("========================================")

	if rebalanceCount > 0 {
		t.Errorf("Rebalances required: %d (expected 0 with lexicographical approach)", rebalanceCount)
	}
}
