package tests

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	"windshift/internal/services"
)

func TestDebugFracCreation(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	
	db, err := sql.Open("sqlite", server.DBPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	
	// Create workspace
	result, _ := db.Exec("INSERT INTO workspaces (name, key) VALUES (?, ?)", "Test", "TEST")
	wsID, _ := result.LastInsertId()
	workspaceID := int(wsID)
	
	// Create 10 items and check for duplicates
	for i := 0; i < 10; i++ {
		// Generate frac_index
		fracIndex, err := services.GenerateFracIndexForNewItem(db, workspaceID, nil)
		if err != nil {
			t.Fatalf("Failed to generate frac_index: %v", err)
		}
		
		fmt.Printf("Generated: %s\n", fracIndex)
		
		// Insert item
		_, err = db.Exec("INSERT INTO items (workspace_id, workspace_item_number, title, status, priority, frac_index) VALUES (?, ?, ?, ?, ?, ?)",
			workspaceID, i+1, fmt.Sprintf("Item %d", i+1), "open", "medium", fracIndex)
		if err != nil {
			t.Fatalf("Failed to insert item: %v", err)
		}
		
		// Verify it was inserted
		var count int
		db.QueryRow("SELECT COUNT(*) FROM items WHERE frac_index = ?", fracIndex).Scan(&count)
		fmt.Printf("  Count in DB: %d\n", count)
	}
	
	// Check for duplicates
	rows, _ := db.Query("SELECT frac_index, COUNT(*) as cnt FROM items WHERE workspace_id = ? GROUP BY frac_index HAVING cnt > 1", workspaceID)
	defer rows.Close()
	
	hasDuplicates := false
	for rows.Next() {
		var fracIndex string
		var count int
		rows.Scan(&fracIndex, &count)
		fmt.Printf("DUPLICATE: %s appears %d times\n", fracIndex, count)
		hasDuplicates = true
	}
	
	if hasDuplicates {
		t.Fatal("Found duplicate frac_index values!")
	}
}
