package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestRankingDemo demonstrates the reranking log output format
func TestRankingDemo(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	// Create a workspace
	workspaceData := map[string]interface{}{
		"name": "Demo Workspace",
		"key":  shortKey("DEMO"),
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
	var workspace map[string]interface{}
	DecodeJSON(t, resp, &workspace)
	resp.Body.Close()
	workspaceID := int(workspace["id"].(float64))

	// Create 5 items
	t.Log("Creating 5 demo items...")
	itemIDs := make([]int, 5)
	for i := 0; i < 5; i++ {
		itemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        fmt.Sprintf("Demo Item %d", i+1),
			"description":  fmt.Sprintf("Item for demonstrating rerank logging"),
			"status":       "open",
			"priority":     "medium",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		var item map[string]interface{}
		DecodeJSON(t, resp, &item)
		resp.Body.Close()

		itemIDs[i] = int(item["id"].(float64))
		fracIndex := ""
		if fi, ok := item["frac_index"].(string); ok {
			fracIndex = fi
		}
		t.Logf("Created item ID=%d with initial frac_index=%s", itemIDs[i], fracIndex)
	}

	// Helper to refresh item states
	refreshItems := func() []map[string]interface{} {
		itemsResp := MakeAuthRequest(t, server, http.MethodGet,
			fmt.Sprintf("/items?workspace_id=%d&parent_id=null", workspaceID), nil)
		var itemsResult map[string]interface{}
		DecodeJSON(t, itemsResp, &itemsResult)
		itemsResp.Body.Close()

		items := itemsResult["items"].([]interface{})
		result := make([]map[string]interface{}, len(items))
		for j, item := range items {
			result[j] = item.(map[string]interface{})
		}
		return result
	}

	// Get current item states
	itemMaps := refreshItems()

	t.Log("========================================")
	t.Log("DEMONSTRATING RERANKING LOG OUTPUT")
	t.Log("========================================")

	// Demo 1: Move item to beginning
	{
		itemToMove := itemMaps[3] // Move 4th item
		itemID := int(itemToMove["id"].(float64))
		oldFracIndex := ""
		if fi, ok := itemToMove["frac_index"].(string); ok {
			oldFracIndex = fi
		}
		nextFracIndex := ""
		if fi, ok := itemMaps[0]["frac_index"].(string); ok {
			nextFracIndex = fi
		}

		rerankData := map[string]interface{}{
			"next_item_id": int(itemMaps[0]["id"].(float64)),
		}

		rerankResp := MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", itemID), rerankData)

		var result map[string]interface{}
		DecodeJSON(t, rerankResp, &result)
		rerankResp.Body.Close()

		newFracIndex := ""
		if fi, ok := result["frac_index"].(string); ok {
			newFracIndex = fi
		}

		t.Logf("Rerank #1: ID=%d, OldFracIndex=%s → NewFracIndex=%s (before: %s)",
			itemID, oldFracIndex, newFracIndex, nextFracIndex)
	}

	// Refresh items after demo 1
	itemMaps = refreshItems()

	// Demo 2: Move item to end
	{
		itemToMove := itemMaps[1] // Move 2nd item
		itemID := int(itemToMove["id"].(float64))
		oldFracIndex := ""
		if fi, ok := itemToMove["frac_index"].(string); ok {
			oldFracIndex = fi
		}
		prevFracIndex := ""
		if fi, ok := itemMaps[len(itemMaps)-1]["frac_index"].(string); ok {
			prevFracIndex = fi
		}

		rerankData := map[string]interface{}{
			"prev_item_id": int(itemMaps[len(itemMaps)-1]["id"].(float64)),
		}

		rerankResp := MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", itemID), rerankData)

		var result map[string]interface{}
		DecodeJSON(t, rerankResp, &result)
		rerankResp.Body.Close()

		newFracIndex := ""
		if fi, ok := result["frac_index"].(string); ok {
			newFracIndex = fi
		}

		t.Logf("Rerank #2: ID=%d, OldFracIndex=%s → NewFracIndex=%s (after: %s)",
			itemID, oldFracIndex, newFracIndex, prevFracIndex)
	}

	// Refresh items after demo 2
	itemMaps = refreshItems()

	// Demo 3: Move item between two others
	{
		itemToMove := itemMaps[0] // Move 1st item
		itemID := int(itemToMove["id"].(float64))
		oldFracIndex := ""
		if fi, ok := itemToMove["frac_index"].(string); ok {
			oldFracIndex = fi
		}
		prevFracIndex := ""
		if fi, ok := itemMaps[2]["frac_index"].(string); ok {
			prevFracIndex = fi
		}
		nextFracIndex := ""
		if fi, ok := itemMaps[3]["frac_index"].(string); ok {
			nextFracIndex = fi
		}

		rerankData := map[string]interface{}{
			"prev_item_id": int(itemMaps[2]["id"].(float64)),
			"next_item_id": int(itemMaps[3]["id"].(float64)),
		}

		rerankResp := MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", itemID), rerankData)

		var result map[string]interface{}
		DecodeJSON(t, rerankResp, &result)
		rerankResp.Body.Close()

		newFracIndex := ""
		if fi, ok := result["frac_index"].(string); ok {
			newFracIndex = fi
		}

		t.Logf("Rerank #3: ID=%d, OldFracIndex=%s → NewFracIndex=%s (between: %s and %s)",
			itemID, oldFracIndex, newFracIndex, prevFracIndex, nextFracIndex)
	}

	t.Log("========================================")
	t.Log("DEMO COMPLETE")
	t.Log("========================================")
}
