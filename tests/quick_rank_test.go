package tests

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"
)

// TestQuickRankingTest performs a quick stress test to verify rebalancing is reduced
func TestQuickRankingTest(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	// Create a workspace
	workspaceData := map[string]interface{}{
		"name": "Quick Test Workspace",
		"key":  shortKey("QUICK"),
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
	var workspace map[string]interface{}
	DecodeJSON(t, resp, &workspace)
	resp.Body.Close()
	workspaceID := int(workspace["id"].(float64))

	// Create 50 items
	t.Log("Creating 50 items...")
	itemIDs := make([]int, 50)
	for i := 0; i < 50; i++ {
		itemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        fmt.Sprintf("Item %d", i+1),
			"status":       "open",
			"priority":     "medium",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		var item map[string]interface{}
		DecodeJSON(t, resp, &item)
		resp.Body.Close()

		itemIDs[i] = int(item["id"].(float64))
	}

	// Perform 200 random reranking operations
	t.Log("Performing 200 reranking operations...")
	rebalanceCount := 0
	successCount := 0

	for i := 0; i < 200; i++ {
		// Get current items
		itemsResp := MakeAuthRequest(t, server, http.MethodGet,
			fmt.Sprintf("/items?workspace_id=%d&parent_id=null&limit=100", workspaceID), nil)
		var itemsResult map[string]interface{}
		DecodeJSON(t, itemsResp, &itemsResult)
		itemsResp.Body.Close()

		items := itemsResult["items"].([]interface{})
		if len(items) < 3 {
			continue
		}

		itemMaps := make([]map[string]interface{}, len(items))
		for j, item := range items {
			itemMaps[j] = item.(map[string]interface{})
		}

		// Random rerank operation
		var itemToMove map[string]interface{}
		var prevItemID, nextItemID *int

		rerankType := rand.Intn(100)
		if rerankType < 20 {
			// Move to beginning
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]
			firstID := int(itemMaps[0]["id"].(float64))
			nextItemID = &firstID
		} else if rerankType < 40 {
			// Move to end
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]
			lastID := int(itemMaps[len(itemMaps)-1]["id"].(float64))
			prevItemID = &lastID
		} else {
			// Move between items
			insertPos := rand.Intn(len(itemMaps) - 1)
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]
			prevID := int(itemMaps[insertPos]["id"].(float64))
			nextID := int(itemMaps[insertPos+1]["id"].(float64))
			prevItemID = &prevID
			nextItemID = &nextID
		}

		itemID := int(itemToMove["id"].(float64))

		// Perform rerank
		rerankData := map[string]interface{}{}
		if prevItemID != nil {
			rerankData["prev_item_id"] = *prevItemID
		}
		if nextItemID != nil {
			rerankData["next_item_id"] = *nextItemID
		}

		rerankResp := MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", itemID), rerankData)

		if rerankResp.StatusCode == http.StatusOK {
			successCount++
			var result map[string]interface{}
			DecodeJSON(t, rerankResp, &result)
		} else {
			var result map[string]interface{}
			DecodeJSON(t, rerankResp, &result)
			if err, ok := result["error"].(string); ok {
				if err == "rebalance required" {
					rebalanceCount++
					t.Logf("Rebalance required at operation %d", i+1)
				} else {
					t.Logf("Error at operation %d: %s", i+1, err)
				}
			}
		}
		rerankResp.Body.Close()

		// Report progress
		if (i+1)%50 == 0 {
			t.Logf("Progress: %d/200 operations, %d successful, %d rebalances needed",
				i+1, successCount, rebalanceCount)
		}
	}

	t.Log("========================================")
	t.Logf("RESULTS: %d successful reranks, %d rebalances required (%.1f%% success rate)",
		successCount, rebalanceCount, float64(successCount)/200*100)
	t.Log("========================================")

	// With lexicographical approach, we should see very few or no rebalances
	if rebalanceCount > 5 {
		t.Errorf("Too many rebalances required: %d (expected < 5 with lexicographical approach)", rebalanceCount)
	}
}
