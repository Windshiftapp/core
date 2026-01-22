//go:build test

package database_test

import (
	"testing"

	"windshift/internal/testutils"
)

func TestDatabaseIntegrity_ForeignKeyConstraints(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Seed test data
	data := tdb.SeedTestData(t)

	tests := []struct {
		name        string
		table       string
		query       string
		args        []interface{}
		shouldFail  bool
		description string
	}{
		{
			name:        "Valid workspace reference in items",
			table:       "items",
			query:       "INSERT INTO items (workspace_id, workspace_item_number, title, status_id) VALUES (?, 1, ?, ?)",
			args:        []interface{}{data.WorkspaceID, "Test Item", data.StatusID},
			shouldFail:  false,
			description: "Should allow valid workspace_id",
		},
		{
			name:        "Invalid workspace reference in items",
			table:       "items",
			query:       "INSERT INTO items (workspace_id, workspace_item_number, title, status_id) VALUES (?, 2, ?, ?)",
			args:        []interface{}{9999, "Test Item", data.StatusID},
			shouldFail:  true,
			description: "Should reject invalid workspace_id",
		},
		{
			name:        "Valid status category reference in statuses",
			table:       "statuses",
			query:       "INSERT INTO statuses (name, description, category_id) VALUES (?, ?, ?)",
			args:        []interface{}{"New Status", "Test status", data.StatusCategoryID},
			shouldFail:  false,
			description: "Should allow valid category_id",
		},
		{
			name:        "Invalid status category reference in statuses",
			table:       "statuses",
			query:       "INSERT INTO statuses (name, description, category_id) VALUES (?, ?, ?)",
			args:        []interface{}{"New Status", "Test status", 9999},
			shouldFail:  true,
			description: "Should reject invalid category_id",
		},
		{
			name:        "Valid user reference in items",
			table:       "items",
			query:       "INSERT INTO items (workspace_id, workspace_item_number, title, status_id, assignee_id) VALUES (?, 3, ?, ?, ?)",
			args:        []interface{}{data.WorkspaceID, "Assigned Item", data.StatusID, data.UserID},
			shouldFail:  false,
			description: "Should allow valid assignee_id",
		},
		{
			name:        "Invalid user reference in items",
			table:       "items",
			query:       "INSERT INTO items (workspace_id, workspace_item_number, title, status_id, assignee_id) VALUES (?, 4, ?, ?, ?)",
			args:        []interface{}{data.WorkspaceID, "Assigned Item", data.StatusID, 9999},
			shouldFail:  true,
			description: "Should reject invalid assignee_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tdb.Exec(tt.query, tt.args...)

			if tt.shouldFail && err == nil {
				t.Errorf("%s: Expected query to fail but it succeeded", tt.description)
			} else if !tt.shouldFail && err != nil {
				t.Errorf("%s: Expected query to succeed but it failed: %v", tt.description, err)
			}
		})
	}
}

func TestDatabaseIntegrity_CascadeDeletes(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Seed test data
	data := tdb.SeedTestData(t)

	// Create test items hierarchy: workspace -> item -> child item
	itemResult, err := tdb.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, status_id, creator_id)
		VALUES (?, 1, 'Parent Item', ?, ?)
	`, data.WorkspaceID, data.StatusID, data.UserID)
	if err != nil {
		t.Fatalf("Failed to create parent item: %v", err)
	}
	parentItemID, _ := itemResult.LastInsertId()

	_, err = tdb.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, status_id, parent_id, creator_id)
		VALUES (?, 2, 'Child Item', ?, ?, ?)
	`, data.WorkspaceID, data.StatusID, parentItemID, data.UserID)
	if err != nil {
		t.Fatalf("Failed to create child item: %v", err)
	}

	// Create comments on the parent item
	_, err = tdb.Exec(`
		INSERT INTO comments (item_id, author_id, content)
		VALUES (?, ?, 'Test comment')
	`, parentItemID, data.UserID)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// Verify initial counts
	var itemCount, commentCount int
	tdb.QueryRow("SELECT COUNT(*) FROM items WHERE workspace_id = ?", data.WorkspaceID).Scan(&itemCount)
	tdb.QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)

	if itemCount != 2 {
		t.Fatalf("Expected 2 items initially, got %d", itemCount)
	}
	if commentCount != 1 {
		t.Fatalf("Expected 1 comment initially, got %d", commentCount)
	}

	// Delete workspace (should cascade to items and comments)
	_, err = tdb.Exec("DELETE FROM workspaces WHERE id = ?", data.WorkspaceID)
	if err != nil {
		t.Fatalf("Failed to delete workspace: %v", err)
	}

	// Verify cascade deletions
	tdb.QueryRow("SELECT COUNT(*) FROM items").Scan(&itemCount)
	tdb.QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)

	if itemCount != 0 {
		t.Errorf("Expected 0 items after workspace deletion, got %d", itemCount)
	}
	if commentCount != 0 {
		t.Errorf("Expected 0 comments after workspace deletion, got %d", commentCount)
	}
}

func TestDatabaseIntegrity_UniqueConstraints(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tests := []struct {
		name       string
		table      string
		setupQuery string
		setupArgs  []interface{}
		testQuery  string
		testArgs   []interface{}
	}{
		{
			name:       "Unique workspace key",
			table:      "workspaces",
			setupQuery: "INSERT INTO workspaces (name, key, description) VALUES (?, ?, ?)",
			setupArgs:  []interface{}{"First Workspace", "UNIQUE", "First workspace"},
			testQuery:  "INSERT INTO workspaces (name, key, description) VALUES (?, ?, ?)",
			testArgs:   []interface{}{"Second Workspace", "UNIQUE", "Duplicate key"},
		},
		{
			name:       "Unique user email",
			table:      "users",
			setupQuery: "INSERT INTO users (email, username, first_name, last_name, password_hash) VALUES (?, ?, ?, ?, ?)",
			setupArgs:  []interface{}{"unique@example.com", "user1", "User", "One", "hash1"},
			testQuery:  "INSERT INTO users (email, username, first_name, last_name, password_hash) VALUES (?, ?, ?, ?, ?)",
			testArgs:   []interface{}{"unique@example.com", "user2", "User", "Two", "hash2"},
		},
		{
			name:       "Unique user username",
			table:      "users",
			setupQuery: "INSERT INTO users (email, username, first_name, last_name, password_hash) VALUES (?, ?, ?, ?, ?)",
			setupArgs:  []interface{}{"user1@example.com", "uniqueuser", "User", "One", "hash1"},
			testQuery:  "INSERT INTO users (email, username, first_name, last_name, password_hash) VALUES (?, ?, ?, ?, ?)",
			testArgs:   []interface{}{"user2@example.com", "uniqueuser", "User", "Two", "hash2"},
		},
		{
			name:       "Unique status category name",
			table:      "status_categories",
			setupQuery: "INSERT INTO status_categories (name, color, description) VALUES (?, ?, ?)",
			setupArgs:  []interface{}{"Unique Category", "#ff0000", "Unique category"},
			testQuery:  "INSERT INTO status_categories (name, color, description) VALUES (?, ?, ?)",
			testArgs:   []interface{}{"Unique Category", "#00ff00", "Duplicate name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear table to start fresh
			tdb.Exec("DELETE FROM " + tt.table)

			// Insert first record
			_, err := tdb.Exec(tt.setupQuery, tt.setupArgs...)
			if err != nil {
				t.Fatalf("Failed to insert setup record: %v", err)
			}

			// Try to insert duplicate record
			_, err = tdb.Exec(tt.testQuery, tt.testArgs...)
			if err == nil {
				t.Errorf("Expected unique constraint violation, but insert succeeded")
			}
		})
	}
}

func TestDatabaseIntegrity_SystemSettingsTypes(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Test system settings have correct types
	tests := []struct {
		key          string
		expectedType string
		testValue    string
		shouldWork   bool
	}{
		{
			key:          "setup_completed",
			expectedType: "boolean",
			testValue:    "true",
			shouldWork:   true,
		},
		{
			key:          "setup_completed",
			expectedType: "boolean",
			testValue:    "invalid",
			shouldWork:   true, // SQLite is flexible with types
		},
		{
			key:          "time_tracking_enabled",
			expectedType: "boolean",
			testValue:    "false",
			shouldWork:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.key+"_"+tt.testValue, func(t *testing.T) {
			// Update the setting
			_, err := tdb.Exec("UPDATE system_settings SET value = ? WHERE key = ?", tt.testValue, tt.key)
			if err != nil {
				if tt.shouldWork {
					t.Errorf("Expected update to work but got error: %v", err)
				}
				return
			}

			// Verify the value was stored
			var storedValue, storedType string
			err = tdb.QueryRow("SELECT value, value_type FROM system_settings WHERE key = ?", tt.key).Scan(&storedValue, &storedType)
			if err != nil {
				t.Fatalf("Failed to query updated setting: %v", err)
			}

			if storedValue != tt.testValue {
				t.Errorf("Expected value %s, got %s", tt.testValue, storedValue)
			}
			if storedType != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, storedType)
			}
		})
	}
}

func TestDatabaseIntegrity_HierarchicalData(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Seed test data
	data := tdb.SeedTestData(t)

	// Create hierarchical items: Root -> Level1 -> Level2
	rootResult, err := tdb.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, status_id, path)
		VALUES (?, 1, 'Root Item', ?, '/')
	`, data.WorkspaceID, data.StatusID)
	if err != nil {
		t.Fatalf("Failed to create root item: %v", err)
	}
	rootID, _ := rootResult.LastInsertId()

	level1Result, err := tdb.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, status_id, parent_id, path)
		VALUES (?, 2, 'Level 1 Item', ?, ?, ?)
	`, data.WorkspaceID, data.StatusID, rootID, "/"+string(rune(rootID))+"/")
	if err != nil {
		t.Fatalf("Failed to create level 1 item: %v", err)
	}
	level1ID, _ := level1Result.LastInsertId()

	_, err = tdb.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, status_id, parent_id, path)
		VALUES (?, 3, 'Level 2 Item', ?, ?, ?)
	`, data.WorkspaceID, data.StatusID, level1ID, "/"+string(rune(rootID))+"/"+string(rune(level1ID))+"/")
	if err != nil {
		t.Fatalf("Failed to create level 2 item: %v", err)
	}

	// Test hierarchy queries
	// Get children of root
	var childCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM items WHERE parent_id = ?", rootID).Scan(&childCount)
	if err != nil {
		t.Fatalf("Failed to query children: %v", err)
	}
	if childCount != 1 {
		t.Errorf("Expected 1 child of root, got %d", childCount)
	}

	// Get all items with parent (simulating level >= 1)
	var nestedCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM items WHERE parent_id IS NOT NULL AND workspace_id = ?", data.WorkspaceID).Scan(&nestedCount)
	if err != nil {
		t.Fatalf("Failed to query nested items: %v", err)
	}
	if nestedCount != 2 {
		t.Errorf("Expected 2 nested items, got %d", nestedCount)
	}

	// Test cascade deletion preserves hierarchy integrity
	_, err = tdb.Exec("DELETE FROM items WHERE id = ?", rootID)
	if err != nil {
		t.Fatalf("Failed to delete root item: %v", err)
	}

	// Verify all descendants are deleted
	var remainingCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM items WHERE workspace_id = ?", data.WorkspaceID).Scan(&remainingCount)
	if err != nil {
		t.Fatalf("Failed to count remaining items: %v", err)
	}
	if remainingCount != 0 {
		t.Errorf("Expected 0 items after root deletion, got %d", remainingCount)
	}
}

func TestDatabaseIntegrity_TransactionSafety(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Test transaction rollback on error using manual transaction
	tx, err := tdb.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Insert a valid workspace
	_, err = tx.Exec("INSERT INTO workspaces (name, key, description) VALUES (?, ?, ?)",
		"Transaction Test", "TRANS", "Test workspace")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert first workspace: %v", err)
	}

	// Insert another workspace with the same key (should cause constraint violation)
	_, err = tx.Exec("INSERT INTO workspaces (name, key, description) VALUES (?, ?, ?)",
		"Duplicate Key", "TRANS", "Should fail")

	// This should fail and cause rollback
	if err != nil {
		// Expected: rollback the transaction
		tx.Rollback()
	} else {
		tx.Commit()
		t.Fatal("Expected constraint violation but insert succeeded")
	}

	// Verify no workspaces were created due to rollback
	var workspaceCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspaces WHERE key = 'TRANS'").Scan(&workspaceCount)
	if err != nil {
		t.Fatalf("Failed to count workspaces: %v", err)
	}
	if workspaceCount != 0 {
		t.Errorf("Expected 0 workspaces after failed transaction, got %d", workspaceCount)
	}
}

func TestDatabaseIntegrity_IndexPerformance(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Seed some test data
	data := tdb.SeedTestData(t)

	// Create many items to test index effectiveness
	for i := 0; i < 100; i++ {
		_, err := tdb.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, title, status_id)
			VALUES (?, ?, ?, ?)
		`, data.WorkspaceID, i+1, "Test Item "+string(rune(i)), data.StatusID)
		if err != nil {
			t.Fatalf("Failed to create test item %d: %v", i, err)
		}
	}

	// Test that indexes are being used for common queries
	// This is a basic test - in a real scenario you'd use EXPLAIN QUERY PLAN

	// Query by workspace_id (should use idx_items_workspace_id)
	var count int
	err := tdb.QueryRow("SELECT COUNT(*) FROM items WHERE workspace_id = ?", data.WorkspaceID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query items by workspace: %v", err)
	}
	if count != 100 {
		t.Errorf("Expected 100 items, got %d", count)
	}

	// Query by status_id (should use idx_items_status_id)
	err = tdb.QueryRow("SELECT COUNT(*) FROM items WHERE status_id = ?", data.StatusID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query items by status: %v", err)
	}
	if count != 100 {
		t.Errorf("Expected 100 items with status, got %d", count)
	}

	// Query system settings by key (should use idx_system_settings_key)
	var value string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'setup_completed'").Scan(&value)
	if err != nil {
		t.Fatalf("Failed to query system setting: %v", err)
	}
	if value != "false" {
		t.Errorf("Expected setup_completed 'false', got '%s'", value)
	}
}
