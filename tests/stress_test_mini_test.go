package tests

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"
)

// TestMiniStressTest performs a focused stress test on a single hot spot
func TestMiniStressTest(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	// Create a workspace
	workspaceData := map[string]interface{}{
		"name": "Hot Spot Test",
		"key":  shortKey("HOT"),
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
	var workspace map[string]interface{}
	DecodeJSON(t, resp, &workspace)
	resp.Body.Close()
	workspaceID := int(workspace["id"].(float64))

	// Create just 3 items
	t.Log("Creating 3 items...")
	itemIDs := make([]int, 3)
	for i := 0; i < 3; i++ {
		itemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        fmt.Sprintf("Item %d", i+1),
			"status":       "open",
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		var item map[string]interface{}
		DecodeJSON(t, resp, &item)
		resp.Body.Close()

		itemIDs[i] = int(item["id"].(float64))
	}

	// Now repeatedly insert between items 1 and 2 to create a hot spot
	t.Log("Creating hot spot by repeatedly inserting between items 1 and 2...")

	rebalanceCount := 0
	successCount := 0
	maxIterations := 100

	for i := 0; i < maxIterations; i++ {
		// Get current items
		itemsResp := MakeAuthRequest(t, server, http.MethodGet,
			fmt.Sprintf("/items?workspace_id=%d&parent_id=null", workspaceID), nil)
		var itemsResult map[string]interface{}
		DecodeJSON(t, itemsResp, &itemsResult)
		itemsResp.Body.Close()

		items := itemsResult["items"].([]interface{})
		if len(items) < 2 {
			t.Fatal("Not enough items")
		}

		// Always insert between the first two items to stress the same spot
		item1 := items[0].(map[string]interface{})
		item2 := items[1].(map[string]interface{})

		// Pick a random item to move
		itemToMove := items[rand.Intn(len(items))].(map[string]interface{})
		if int(itemToMove["id"].(float64)) == int(item1["id"].(float64)) ||
			int(itemToMove["id"].(float64)) == int(item2["id"].(float64)) {
			// Pick a different item if we picked one of the boundary items
			if len(items) > 2 {
				itemToMove = items[len(items)-1].(map[string]interface{})
			}
		}

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
				if (i+1)%10 == 0 {
					t.Logf("  Iteration %d: frac_index = %s (len=%d)", i+1, fracIndex, len(fracIndex))
				}
			}
		} else {
			var result map[string]interface{}
			DecodeJSON(t, rerankResp, &result)
			if err, ok := result["error"].(string); ok {
				if err == "rebalance required" {
					rebalanceCount++
					t.Logf("  Rebalance required at iteration %d", i+1)
					break
				}
			}
		}
		rerankResp.Body.Close()
	}

	t.Log("========================================")
	t.Logf("HOT SPOT TEST RESULTS:")
	t.Logf("  Successful insertions: %d", successCount)
	t.Logf("  Rebalances required: %d", rebalanceCount)
	if rebalanceCount > 0 {
		t.Logf("  Failed after %d insertions in the same spot", successCount)
	} else {
		t.Logf("  Successfully handled %d insertions in the same spot!", successCount)
	}
	t.Log("========================================")

	// We should be able to handle many more insertions now
	if successCount < 50 {
		t.Errorf("Could only handle %d insertions before rebalance (expected at least 50)", successCount)
	}
}
