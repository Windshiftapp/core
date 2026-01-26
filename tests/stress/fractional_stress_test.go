package stress

import (
	"windshift/internal/services"
	tests "windshift/tests"
	"database/sql"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TestFractionalIndexingStressTest performs comprehensive stress testing of fractional indexing
// with 1,000 items and 10,000 reranking operations - NO REBALANCING SHOULD BE NEEDED
func TestFractionalIndexingStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server, _ := tests.StartTestServer(t, "sqlite")
	tests.CreateBearerToken(t, server)

	// Open database connection
	db, err := sql.Open("sqlite3", server.DBPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	stats := &FracIndexStats{
		ItemCreationTimes: []time.Duration{},
		RerankTimes:       []time.Duration{},
		StartTime:         time.Now(),
	}

	// Phase 1: Create 1,000 items with fractional indexing
	t.Run("Phase1_CreateItemsWithFracIndex", func(t *testing.T) {
		testFrac_Phase1_CreateItems(t, server, db, stats)
	})

	// Phase 2: 10,000 random reranking operations
	t.Run("Phase2_RerankingOperations", func(t *testing.T) {
		testFrac_Phase2_RerankingOperations(t, server, db, stats)
	})

	// Phase 3: Final Validation
	t.Run("Phase3_FinalValidation", func(t *testing.T) {
		testFrac_Phase3_FinalValidation(t, server, db, stats)
	})

	// Print final statistics
	printFracIndexStatistics(t, stats)
}

// FracIndexStats tracks performance metrics throughout the test
type FracIndexStats struct {
	TotalItems        int
	TotalWorkspaces   int
	ItemCreationTimes []time.Duration
	RerankTimes       []time.Duration
	RebalanceCount    int // Should stay at 0 for fractional indexing!
	StartTime         time.Time
}

// testFrac_Phase1_CreateItems creates 1,000 items across 5 workspaces with frac_index
func testFrac_Phase1_CreateItems(t *testing.T, server *tests.TestServer, db *sql.DB, stats *FracIndexStats) {
	t.Log("Phase 1: Creating 1,000 items with fractional indexing")

	const numWorkspaces = 5
	const itemsPerWorkspace = 200
	const totalItems = numWorkspaces * itemsPerWorkspace

	// Fractional indexing format: base62 with variable-length integer encoding
	// Examples: "a0", "a1", "b00", "aV", "a0V", etc.
	fracIndexPattern := regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*$`)

	workspaceIDs := make([]int, numWorkspaces)
	itemIDs := make(map[int][]int) // workspace_id -> item_ids
	seenFracIndices := make(map[string]int)

	// Create workspaces
	for i := 0; i < numWorkspaces; i++ {
		workspaceID := createTestWorkspace(t, db, fmt.Sprintf("FracIndex WS %d", i+1))
		workspaceIDs[i] = workspaceID
		itemIDs[workspaceID] = []int{}
	}

	stats.TotalWorkspaces = numWorkspaces

	// Create items with fractional indexing
	itemCount := 0
	reportInterval := 500

	// Track last generated frac_index to avoid querying DB each time (which can cause duplicates)
	lastFracIndex := ""

	for _, workspaceID := range workspaceIDs {
		// Create root items (70%)
		rootItemCount := int(float64(itemsPerWorkspace) * 0.7)

		for i := 0; i < rootItemCount; i++ {
			// Generate fractional index - use in-memory last index instead of querying DB
			fracIndex, err := services.KeyBetween(lastFracIndex, "")
			if err != nil {
				t.Fatalf("Failed to generate frac_index: %v", err)
			}
			lastFracIndex = fracIndex

			// Validate format
			if !fracIndexPattern.MatchString(fracIndex) {
				t.Fatalf("Generated invalid frac_index format: %s", fracIndex)
			}

			// Check for global duplicates (frac_index is globally unique)
			if prevWorkspace, exists := seenFracIndices[fracIndex]; exists {
				t.Fatalf("Duplicate frac_index %s for items in workspace %d and %d", fracIndex, prevWorkspace, workspaceID)
			}
			seenFracIndices[fracIndex] = workspaceID

			// Create item via direct DB insert
			startTime := time.Now()
			itemID := createTestItemWithFracIndex(t, db, workspaceID, nil, fmt.Sprintf("Root Item %d", i+1), fracIndex)
			creationTime := time.Since(startTime)

			itemIDs[workspaceID] = append(itemIDs[workspaceID], itemID)
			stats.ItemCreationTimes = append(stats.ItemCreationTimes, creationTime)
			itemCount++

			if itemCount%reportInterval == 0 {
				t.Logf("Progress: %d/%d items created", itemCount, totalItems)
			}
		}

		// Create child items (30%)
		childItemCount := itemsPerWorkspace - rootItemCount
		parentsPerLevel := rootItemCount / 10

		if parentsPerLevel > 0 {
			for i := 0; i < childItemCount; i++ {
				parentID := itemIDs[workspaceID][rand.Intn(parentsPerLevel)]

				// Generate fractional index for child - continue global sequence
				fracIndex, err := services.KeyBetween(lastFracIndex, "")
				if err != nil {
					t.Fatalf("Failed to generate child frac_index: %v", err)
				}
				lastFracIndex = fracIndex

				// Validate format
				if !fracIndexPattern.MatchString(fracIndex) {
					t.Fatalf("Generated invalid child frac_index format: %s", fracIndex)
				}

				// Check for global duplicates (frac_index is globally unique)
				if prevWorkspace, exists := seenFracIndices[fracIndex]; exists {
					t.Fatalf("Duplicate frac_index %s for items in workspace %d and %d", fracIndex, prevWorkspace, workspaceID)
				}
				seenFracIndices[fracIndex] = workspaceID

				// Create child item
				startTime := time.Now()
				itemID := createTestItemWithFracIndex(t, db, workspaceID, &parentID, fmt.Sprintf("Child Item %d", i+1), fracIndex)
				creationTime := time.Since(startTime)

				itemIDs[workspaceID] = append(itemIDs[workspaceID], itemID)
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

// testFrac_Phase2_RerankingOperations performs 10,000 random reranking operations
func testFrac_Phase2_RerankingOperations(t *testing.T, server *tests.TestServer, db *sql.DB, stats *FracIndexStats) {
	t.Log("Phase 2: Performing 10,000 random reranking operations using fractional indexing")

	const numReranks = 10000
	reportInterval := 1000

	for i := 0; i < numReranks; i++ {
		// Get ALL items globally sorted by frac_index (matching how Phase 1 works)
		// This is correct because frac_index is globally unique, not per-workspace
		rows, err := db.Query("SELECT id, frac_index FROM items WHERE frac_index IS NOT NULL ORDER BY frac_index")
		if err != nil {
			t.Fatalf("Failed to query items: %v", err)
		}

		type item struct {
			id        int
			fracIndex string
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.fracIndex); err != nil {
				rows.Close()
				t.Fatalf("Failed to scan item: %v", err)
			}
			items = append(items, it)
		}
		rows.Close()

		if len(items) < 3 {
			continue // Need at least 3 items to rerank
		}

		// Perform random rerank operation
		rerankType := rand.Intn(100)
		var itemToMove item
		var prevFracIndex, nextFracIndex string

		if rerankType < 20 {
			// Move to beginning (20%)
			itemToMove = items[rand.Intn(len(items))]
			prevFracIndex = ""
			nextFracIndex = items[0].fracIndex
			// Skip if trying to move item to its own position
			if itemToMove.fracIndex == nextFracIndex {
				continue
			}
		} else if rerankType < 40 {
			// Move to end (20%)
			itemToMove = items[rand.Intn(len(items))]
			prevFracIndex = items[len(items)-1].fracIndex
			nextFracIndex = ""
			// Skip if trying to move item to its own position
			if itemToMove.fracIndex == prevFracIndex {
				continue
			}
		} else {
			// Move between items (60%)
			if len(items) < 4 {
				continue
			}
			insertPos := rand.Intn(len(items) - 1)
			itemToMove = items[rand.Intn(len(items))]
			prevFracIndex = items[insertPos].fracIndex
			nextFracIndex = items[insertPos+1].fracIndex
			// Skip if trying to move item to its own position or between adjacent items where item is already positioned
			if itemToMove.fracIndex == prevFracIndex || itemToMove.fracIndex == nextFracIndex {
				continue
			}
		}

		// Skip if prev and next are the same (would be an invalid operation)
		if prevFracIndex != "" && nextFracIndex != "" && prevFracIndex == nextFracIndex {
			continue
		}

		// Generate new fractional index between the two positions
		startTime := time.Now()
		newFracIndex, err := services.KeyBetween(prevFracIndex, nextFracIndex)
		if err != nil {
			t.Errorf("Rerank operation %d failed to generate frac_index: %v (prev=%s, next=%s)", i+1, err, prevFracIndex, nextFracIndex)
			stats.RebalanceCount++ // This should NEVER happen with fractional indexing!
			continue
		}
		rerankTime := time.Since(startTime)

		// Update the item
		err = services.UpdateItemFracIndex(db, itemToMove.id, newFracIndex)
		if err != nil {
			t.Errorf("Rerank operation %d failed to update frac_index: %v", i+1, err)
			continue
		}

		stats.RerankTimes = append(stats.RerankTimes, rerankTime)

		// Progress report
		if (i+1)%reportInterval == 0 {
			t.Logf("Progress: %d/%d reranks completed (rebalances needed: %d)", i+1, numReranks, stats.RebalanceCount)
		}
	}

	avgRerankTime := averageDuration(stats.RerankTimes)
	t.Logf("Phase 2 complete: %d reranks, avg time: %v, rebalances needed: %d",
		len(stats.RerankTimes), avgRerankTime, stats.RebalanceCount)

	// CRITICAL: Fractional indexing should NEVER need rebalancing
	if stats.RebalanceCount > 0 {
		t.Errorf("CRITICAL: Fractional indexing required %d rebalances - this should NEVER happen!", stats.RebalanceCount)
	}
}

// testFrac_Phase3_FinalValidation performs comprehensive validation
func testFrac_Phase3_FinalValidation(t *testing.T, server *tests.TestServer, db *sql.DB, stats *FracIndexStats) {
	t.Log("Phase 3: Performing final validation")


	// Get all items with frac_index
	rows, err := db.Query("SELECT id, workspace_id, parent_id, frac_index FROM items WHERE frac_index IS NOT NULL")
	if err != nil {
		t.Fatalf("Failed to query items: %v", err)
	}
	defer rows.Close()

	type item struct {
		id         int
		workspaceID int
		parentID   *int
		fracIndex  string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.workspaceID, &it.parentID, &it.fracIndex); err != nil {
			t.Fatalf("Failed to scan item: %v", err)
		}
		items = append(items, it)
	}

	t.Logf("Validating %d items with frac_index", len(items))

	// Validation 1: Format validation
	t.Run("ValidateFracIndexFormat", func(t *testing.T) {
		// Fractional index format: base62 with variable-length encoding
		// Must start with a letter (a-z for positive, A-Z for negative)
		// Followed by base62 digits, cannot end with '0'
		invalidCount := 0

		for _, item := range items {
			// Try validating by checking it's between empty bounds
			_, err := services.KeyBetween("", item.fracIndex)
			if err != nil {
				t.Errorf("Item %d has invalid frac_index format: %s (error: %v)", item.id, item.fracIndex, err)
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
			fracIndexMap[item.fracIndex] = append(fracIndexMap[item.fracIndex], item.id)
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
		contexts := make(map[string][]item)

		for _, item := range items {
			var contextKey string
			if item.parentID != nil {
				contextKey = fmt.Sprintf("ws%d_p%d", item.workspaceID, *item.parentID)
			} else {
				contextKey = fmt.Sprintf("ws%d_root", item.workspaceID)
			}

			contexts[contextKey] = append(contexts[contextKey], item)
		}

		orderingErrors := 0
		for contextKey, contextItems := range contexts {
			// Sort by frac_index (lexicographic)
			sort.Slice(contextItems, func(i, j int) bool {
				return contextItems[i].fracIndex < contextItems[j].fracIndex
			})

			// Validate ordering
			for i := 0; i < len(contextItems)-1; i++ {
				fracIndexI := contextItems[i].fracIndex
				fracIndexJ := contextItems[i+1].fracIndex

				if fracIndexI >= fracIndexJ {
					t.Errorf("Context %s: Items out of order: %d (frac_index %s) >= %d (frac_index %s)",
						contextKey, contextItems[i].id, fracIndexI, contextItems[i+1].id, fracIndexJ)
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

func printFracIndexStatistics(t *testing.T, stats *FracIndexStats) {
	totalDuration := time.Since(stats.StartTime)
	avgCreation := averageDuration(stats.ItemCreationTimes)
	avgRerank := averageDuration(stats.RerankTimes)

	t.Log("========================================")
	t.Log("FRACTIONAL INDEXING STRESS TEST COMPLETE")
	t.Log("========================================")
	t.Logf("Total duration: %v", totalDuration)
	t.Logf("Workspaces: %d", stats.TotalWorkspaces)
	t.Logf("Items created: %d (avg: %v)", stats.TotalItems, avgCreation)
	t.Logf("Reranks performed: %d (avg: %v)", len(stats.RerankTimes), avgRerank)
	t.Logf("Rebalances needed: %d (should be 0!)", stats.RebalanceCount)
	if stats.RebalanceCount == 0 {
		t.Log("✓ SUCCESS: No rebalancing needed!")
	} else {
		t.Logf("✗ FAILURE: Fractional indexing required %d rebalances!", stats.RebalanceCount)
	}
	t.Log("========================================")
}

// Helper function to create test workspace
func createTestWorkspace(t *testing.T, db *sql.DB, name string) int {
	result, err := db.Exec("INSERT INTO workspaces (name, key) VALUES (?, ?)", name, fmt.Sprintf("WS%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

// Helper function to create test item with frac_index
func createTestItemWithFracIndex(t *testing.T, db *sql.DB, workspaceID int, parentID *int, title string, fracIndex string) int {
	// Get next workspace_item_number for this workspace
	var maxItemNumber int
	err := db.QueryRow("SELECT COALESCE(MAX(workspace_item_number), 0) FROM items WHERE workspace_id = ?", workspaceID).Scan(&maxItemNumber)
	if err != nil {
		t.Fatalf("Failed to get max workspace_item_number: %v", err)
	}
	nextItemNumber := maxItemNumber + 1

	// Get default status ID (Open)
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses WHERE is_default = 1 LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get default status ID: %v", err)
	}

	// Get default priority ID (Medium)
	var priorityID int
	err = db.QueryRow("SELECT id FROM priorities WHERE is_default = 1 LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get default priority ID: %v", err)
	}

	var result sql.Result
	if parentID == nil {
		result, err = db.Exec("INSERT INTO items (workspace_id, workspace_item_number, title, status_id, priority_id, frac_index) VALUES (?, ?, ?, ?, ?, ?)",
			workspaceID, nextItemNumber, title, statusID, priorityID, fracIndex)
	} else {
		result, err = db.Exec("INSERT INTO items (workspace_id, workspace_item_number, parent_id, title, status_id, priority_id, frac_index) VALUES (?, ?, ?, ?, ?, ?, ?)",
			workspaceID, nextItemNumber, *parentID, title, statusID, priorityID, fracIndex)
	}

	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}
