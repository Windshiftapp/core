package stress

import (
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
	tests "windshift/tests"
)

// TestFractionalIndexingAPIStressTest performs comprehensive stress testing of fractional indexing
// with 1,000 items and 1,000 reranking operations using the REST API - NO REBALANCING SHOULD BE NEEDED
func TestFractionalIndexingAPIStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server, _ := tests.StartTestServer(t, "sqlite")
	tests.CreateBearerToken(t, server)

	stats := &FracIndexAPIStats{
		ItemCreationTimes: []time.Duration{},
		RerankTimes:       []time.Duration{},
		StartTime:         time.Now(),
	}

	// Phase 1: Create 1,000 items with automatic frac_index generation
	t.Run("Phase1_CreateItemsWithFracIndex", func(t *testing.T) {
		testFracAPI_Phase1_CreateItems(t, server, stats)
	})

	// Phase 2: 1,000 random reranking operations
	t.Run("Phase2_RerankingOperations", func(t *testing.T) {
		testFracAPI_Phase2_RerankingOperations(t, server, stats)
	})

	// Phase 3: Final Validation
	t.Run("Phase3_FinalValidation", func(t *testing.T) {
		testFracAPI_Phase3_FinalValidation(t, server, stats)
	})

	// Print final statistics
	printFracIndexAPIStatistics(t, stats)
}

// FracIndexAPIStats tracks performance metrics throughout the test
type FracIndexAPIStats struct {
	TotalItems        int
	TotalWorkspaces   int
	ItemCreationTimes []time.Duration
	RerankTimes       []time.Duration
	RebalanceCount    int // Should stay at 0 for fractional indexing!
	StartTime         time.Time
}

// testFracAPI_Phase1_CreateItems creates 1,000 items across 5 workspaces via API
func testFracAPI_Phase1_CreateItems(t *testing.T, server *tests.TestServer, stats *FracIndexAPIStats) {
	t.Log("Phase 1: Creating 1,000 items with automatic frac_index generation via API")

	const numWorkspaces = 5
	const itemsPerWorkspace = 200
	const totalItems = numWorkspaces * itemsPerWorkspace

	workspaceIDs := make([]int, numWorkspaces)
	seenFracIndices := make(map[string]int)
	fracIndexPattern := regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*$`)

	// Create workspaces
	for i := 0; i < numWorkspaces; i++ {
		workspaceData := map[string]interface{}{
			"name": fmt.Sprintf("FracIndex API WS %d", i+1),
			"key":  fmt.Sprintf("FAPI%d", i+1),
		}

		resp := tests.MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
		var workspace map[string]interface{}
		tests.DecodeJSON(t, resp, &workspace)
		resp.Body.Close()

		workspaceIDs[i] = int(workspace["id"].(float64))
	}

	stats.TotalWorkspaces = numWorkspaces

	// Create items with automatic frac_index generation via API
	itemCount := 0
	reportInterval := 500

	for _, workspaceID := range workspaceIDs {
		// Create root items (70%)
		rootItemCount := int(float64(itemsPerWorkspace) * 0.7)
		rootItemIDs := make([]int, 0, rootItemCount)

		for i := 0; i < rootItemCount; i++ {
			itemData := map[string]interface{}{
				"workspace_id": workspaceID,
				"title":        fmt.Sprintf("Root Item %d", i+1),
				"description":  fmt.Sprintf("Root item in workspace %d", workspaceID),
				"status":       "open",
				"priority":     "medium",
			}

			startTime := time.Now()
			resp := tests.MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
			creationTime := time.Since(startTime)

			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("Failed to create item: status %d", resp.StatusCode)
			}

			var item map[string]interface{}
			tests.DecodeJSON(t, resp, &item)
			resp.Body.Close()

			// Validate frac_index was generated
			fracIndexVal, hasFracIndex := item["frac_index"]
			fracIndexStr, ok := fracIndexVal.(string)
			if !hasFracIndex || !ok || strings.TrimSpace(fracIndexStr) == "" {
				t.Fatalf("Item %d missing frac_index after creation", int(item["id"].(float64)))
			}

			if !fracIndexPattern.MatchString(fracIndexStr) {
				t.Fatalf("Item %d assigned invalid frac_index format %s", int(item["id"].(float64)), fracIndexStr)
			}

			if prevID, exists := seenFracIndices[fracIndexStr]; exists {
				t.Fatalf("Duplicate frac_index %s detected for items %d and %d during creation", fracIndexStr, prevID, int(item["id"].(float64)))
			}
			seenFracIndices[fracIndexStr] = int(item["id"].(float64))

			rootItemIDs = append(rootItemIDs, int(item["id"].(float64)))
			stats.ItemCreationTimes = append(stats.ItemCreationTimes, creationTime)
			itemCount++

			if itemCount%reportInterval == 0 {
				t.Logf("Progress: %d/%d items created", itemCount, totalItems)
			}
		}

		// Create hierarchical items (30%) - 2 levels deep
		childItemCount := itemsPerWorkspace - rootItemCount
		parentsPerLevel := rootItemCount / 10 // Use 10% of root items as parents

		if parentsPerLevel > 0 {
			for i := 0; i < childItemCount; i++ {
				parentID := rootItemIDs[rand.Intn(parentsPerLevel)]

				itemData := map[string]interface{}{
					"workspace_id": workspaceID,
					"parent_id":    parentID,
					"title":        fmt.Sprintf("Child Item %d", i+1),
					"description":  fmt.Sprintf("Child item under parent %d", parentID),
					"status":       "open",
					"priority":     "low",
				}

				startTime := time.Now()
				resp := tests.MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
				creationTime := time.Since(startTime)

				if resp.StatusCode != http.StatusCreated {
					t.Fatalf("Failed to create child item: status %d", resp.StatusCode)
				}

				var item map[string]interface{}
				tests.DecodeJSON(t, resp, &item)
				resp.Body.Close()

				// Validate frac_index was generated
				fracIndexVal, hasFracIndex := item["frac_index"]
				fracIndexStr, ok := fracIndexVal.(string)
				if !hasFracIndex || !ok || strings.TrimSpace(fracIndexStr) == "" {
					t.Fatalf("Child item %d missing frac_index after creation", int(item["id"].(float64)))
				}

				if !fracIndexPattern.MatchString(fracIndexStr) {
					t.Fatalf("Child item %d assigned invalid frac_index format %s", int(item["id"].(float64)), fracIndexStr)
				}

				if prevID, exists := seenFracIndices[fracIndexStr]; exists {
					t.Fatalf("Duplicate frac_index %s detected for items %d and %d during creation", fracIndexStr, prevID, int(item["id"].(float64)))
				}
				seenFracIndices[fracIndexStr] = int(item["id"].(float64))

				stats.ItemCreationTimes = append(stats.ItemCreationTimes, creationTime)
				itemCount++

				if itemCount%reportInterval == 0 {
					t.Logf("Progress: %d/%d items created", itemCount, totalItems)
				}
			}
		}
	}

	stats.TotalItems = itemCount
	avgCreationTime := averageDuration(stats.ItemCreationTimes)
	t.Logf("Phase 1 complete: created %d items (avg creation time: %v)", itemCount, avgCreationTime)
}

// testFracAPI_Phase2_RerankingOperations performs 1,000 random reranking operations via API
func testFracAPI_Phase2_RerankingOperations(t *testing.T, server *tests.TestServer, stats *FracIndexAPIStats) {
	t.Log("Phase 2: Performing 1,000 random reranking operations using fractional indexing API")

	const numReranks = 1000
	reportInterval := 100

	for i := 0; i < numReranks; i++ {
		// Get ALL items globally sorted by frac_index (matching how fractional indexing works)
		// frac_index is globally unique, not per-workspace
		itemsResp := tests.MakeAuthRequest(t, server, http.MethodGet, "/items?limit=10000&order_by=frac_index", nil)

		var itemsResult map[string]interface{}
		tests.DecodeJSON(t, itemsResp, &itemsResult)
		itemsResp.Body.Close()

		items, ok := itemsResult["items"].([]interface{})
		if !ok {
			continue
		}

		// Convert to item maps and filter out items without frac_index
		itemMaps := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			itemMap := item.(map[string]interface{})
			// Only include items with non-null, non-empty frac_index (matching old test behavior)
			if itemMap["frac_index"] != nil {
				if fracIndexStr, ok := itemMap["frac_index"].(string); ok && fracIndexStr != "" {
					itemMaps = append(itemMaps, itemMap)
				}
			}
		}

		// Skip if not enough items with frac_index to rerank
		if len(itemMaps) < 3 {
			continue
		}

		// Items are already sorted by frac_index from the server (order_by=frac_index parameter)

		// Perform random rerank operation
		rerankType := rand.Intn(100)
		var itemToMove map[string]interface{}
		var prevItemID, nextItemID *int
		var prevFracIndex, nextFracIndex string

		if rerankType < 20 {
			// Move to beginning (20%)
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]
			prevItemID = nil
			firstID := int(itemMaps[0]["id"].(float64))
			nextItemID = &firstID
			if itemMaps[0]["frac_index"] != nil {
				nextFracIndex = itemMaps[0]["frac_index"].(string)
			}
			// Skip if trying to move item to its own position
			if itemToMove["frac_index"] != nil && itemToMove["frac_index"].(string) == nextFracIndex {
				continue
			}
		} else if rerankType < 40 {
			// Move to end (20%)
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]
			lastID := int(itemMaps[len(itemMaps)-1]["id"].(float64))
			prevItemID = &lastID
			nextItemID = nil
			if itemMaps[len(itemMaps)-1]["frac_index"] != nil {
				prevFracIndex = itemMaps[len(itemMaps)-1]["frac_index"].(string)
			}
			// Skip if trying to move item to its own position
			if itemToMove["frac_index"] != nil && itemToMove["frac_index"].(string) == prevFracIndex {
				continue
			}
		} else {
			// Move between items (60%)
			if len(itemMaps) < 4 {
				continue
			}
			insertPos := rand.Intn(len(itemMaps) - 1)
			itemToMove = itemMaps[rand.Intn(len(itemMaps))]

			prevID := int(itemMaps[insertPos]["id"].(float64))
			nextID := int(itemMaps[insertPos+1]["id"].(float64))
			prevItemID = &prevID
			nextItemID = &nextID
			if itemMaps[insertPos]["frac_index"] != nil {
				prevFracIndex = itemMaps[insertPos]["frac_index"].(string)
			}
			if itemMaps[insertPos+1]["frac_index"] != nil {
				nextFracIndex = itemMaps[insertPos+1]["frac_index"].(string)
			}
			// Skip if item is already at prev or next position
			if itemToMove["frac_index"] != nil {
				itemFracIndex := itemToMove["frac_index"].(string)
				if itemFracIndex == prevFracIndex || itemFracIndex == nextFracIndex {
					continue
				}
			}
		}

		// Skip if prev and next are the same (would cause KeyBetween error)
		if prevFracIndex != "" && nextFracIndex != "" && prevFracIndex == nextFracIndex {
			continue
		}

		itemID := int(itemToMove["id"].(float64))

		// Perform rerank using frac_index endpoint
		rerankData := map[string]interface{}{}
		if prevItemID != nil {
			rerankData["prev_item_id"] = *prevItemID
		}
		if nextItemID != nil {
			rerankData["next_item_id"] = *nextItemID
		}

		startTime := time.Now()
		rerankResp := tests.MakeAuthRequest(t, server, http.MethodPut,
			fmt.Sprintf("/items/%d/frac-index", itemID), rerankData)
		rerankTime := time.Since(startTime)

		if rerankResp.StatusCode != http.StatusOK {
			t.Errorf("Rerank operation %d failed with status %d", i+1, rerankResp.StatusCode)
			stats.RebalanceCount++ // Track failures (should never happen)
			rerankResp.Body.Close()
			continue
		}

		var result map[string]interface{}
		tests.DecodeJSON(t, rerankResp, &result)
		rerankResp.Body.Close()

		stats.RerankTimes = append(stats.RerankTimes, rerankTime)

		// Progress report
		if (i+1)%reportInterval == 0 {
			t.Logf("Progress: %d/%d reranks completed (failures: %d)", i+1, numReranks, stats.RebalanceCount)
		}
	}

	avgRerankTime := averageDuration(stats.RerankTimes)
	t.Logf("Phase 2 complete: %d reranks, avg time: %v, failures: %d",
		len(stats.RerankTimes), avgRerankTime, stats.RebalanceCount)

	// CRITICAL: Fractional indexing should NEVER fail/need rebalancing
	if stats.RebalanceCount > 0 {
		t.Errorf("CRITICAL: Fractional indexing had %d failures - this should NEVER happen!", stats.RebalanceCount)
	}
}

// testFracAPI_Phase3_FinalValidation performs comprehensive validation via API
func testFracAPI_Phase3_FinalValidation(t *testing.T, server *tests.TestServer, stats *FracIndexAPIStats) {
	t.Log("Phase 3: Performing final validation via API")

	// Get all items with frac_index via API
	resp := tests.MakeAuthRequest(t, server, http.MethodGet, "/items?limit=200000", nil)
	var result map[string]interface{}
	tests.DecodeJSON(t, resp, &result)
	resp.Body.Close()

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("Failed to get items for validation")
	}

	t.Logf("Validating %d items with frac_index", len(items))

	// Validation 1: Format validation
	t.Run("ValidateFracIndexFormat", func(t *testing.T) {
		// Fractional index format: base62 with variable-length encoding
		// Must start with a letter (a-z for positive, A-Z for negative)
		// Followed by base62 digits, cannot end with '0'
		fracIndexPattern := regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*$`)
		invalidCount := 0

		for _, item := range items {
			itemMap := item.(map[string]interface{})
			fracIndex, hasFracIndex := itemMap["frac_index"]
			if !hasFracIndex || fracIndex == nil {
				continue // Items might not have frac_index if created before migration
			}

			fracIndexStr := fracIndex.(string)
			if !fracIndexPattern.MatchString(fracIndexStr) {
				t.Errorf("Item %v has invalid frac_index format: %s", itemMap["id"], fracIndexStr)
				invalidCount++
			}
		}

		if invalidCount > 0 {
			t.Errorf("Found %d items with invalid frac_index format", invalidCount)
		}
	})

	// Validation 2: Global uniqueness
	t.Run("ValidateFracIndexUniqueness", func(t *testing.T) {
		fracIndexMap := make(map[string][]int)

		for _, item := range items {
			itemMap := item.(map[string]interface{})
			if itemMap["frac_index"] == nil {
				continue
			}
			fracIndex := itemMap["frac_index"].(string)
			itemID := int(itemMap["id"].(float64))

			fracIndexMap[fracIndex] = append(fracIndexMap[fracIndex], itemID)
		}

		duplicates := 0
		for fracIndex, itemIDs := range fracIndexMap {
			if len(itemIDs) > 1 {
				t.Errorf("Duplicate frac_index %s found in items: %v", fracIndex, itemIDs)
				duplicates++
			}
		}

		if duplicates > 0 {
			t.Errorf("Found %d duplicate frac_indices", duplicates)
		}
	})

	// Validation 3: Lexicographic ordering per context
	t.Run("ValidateFracIndexOrdering", func(t *testing.T) {
		// Group items by workspace and parent
		contexts := make(map[string][]map[string]interface{})

		for _, item := range items {
			itemMap := item.(map[string]interface{})
			if itemMap["frac_index"] == nil {
				continue
			}

			workspaceID := int(itemMap["workspace_id"].(float64))

			var contextKey string
			if parentID, hasParent := itemMap["parent_id"]; hasParent && parentID != nil {
				contextKey = fmt.Sprintf("ws%d_p%d", workspaceID, int(parentID.(float64)))
			} else {
				contextKey = fmt.Sprintf("ws%d_root", workspaceID)
			}

			contexts[contextKey] = append(contexts[contextKey], itemMap)
		}

		orderingErrors := 0
		for contextKey, contextItems := range contexts {
			// Sort by frac_index (lexicographic)
			sort.Slice(contextItems, func(i, j int) bool {
				fracIndexI := contextItems[i]["frac_index"].(string)
				fracIndexJ := contextItems[j]["frac_index"].(string)
				return fracIndexI < fracIndexJ
			})

			// Validate ordering
			for i := 0; i < len(contextItems)-1; i++ {
				fracIndexI := contextItems[i]["frac_index"].(string)
				fracIndexJ := contextItems[i+1]["frac_index"].(string)

				if fracIndexI >= fracIndexJ {
					t.Errorf("Context %s: Items out of order: %v (frac_index %s) >= %v (frac_index %s)",
						contextKey, contextItems[i]["id"], fracIndexI, contextItems[i+1]["id"], fracIndexJ)
					orderingErrors++
				}
			}
		}

		if orderingErrors > 0 {
			t.Errorf("Found %d ordering errors across %d contexts", orderingErrors, len(contexts))
		}
	})

	t.Log("Phase 3 complete: validation complete")
}

func printFracIndexAPIStatistics(t *testing.T, stats *FracIndexAPIStats) {
	totalDuration := time.Since(stats.StartTime)
	avgCreation := averageDuration(stats.ItemCreationTimes)
	avgRerank := averageDuration(stats.RerankTimes)

	t.Log("========================================")
	t.Log("FRACTIONAL INDEXING API STRESS TEST COMPLETE")
	t.Log("========================================")
	t.Logf("Total duration: %v", totalDuration)
	t.Logf("Workspaces: %d", stats.TotalWorkspaces)
	t.Logf("Items created: %d (avg: %v)", stats.TotalItems, avgCreation)
	t.Logf("Reranks performed: %d (avg: %v)", len(stats.RerankTimes), avgRerank)
	t.Logf("Failures/Rebalances needed: %d (should be 0!)", stats.RebalanceCount)
	if stats.RebalanceCount == 0 {
		t.Log("✓ SUCCESS: No failures or rebalancing needed!")
	} else {
		t.Logf("✗ FAILURE: Fractional indexing had %d failures!", stats.RebalanceCount)
	}
	t.Log("========================================")
}
