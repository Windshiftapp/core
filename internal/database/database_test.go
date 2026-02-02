//go:build test

package database_test

import (
	"testing"

	"windshift/internal/database"
	"windshift/internal/testutils"
)

func TestDatabase_Initialize_FreshDatabase(t *testing.T) {
	// Create fresh database without initialization
	tdb := testutils.CreateFreshDB(t, true)
	defer tdb.Close()

	// Initialize the database
	if err := tdb.Initialize(); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Verify all core tables exist
	coreTable := []string{
		"workspaces", "items", "users", "workflows", "system_settings",
		"status_categories", "statuses", "screens", "configuration_sets",
	}

	for _, table := range coreTable {
		tdb.AssertTableExists(t, table)
	}

	// Verify foreign key constraints are enabled
	tdb.AssertForeignKeyEnabled(t)

	// Verify core indexes exist
	coreIndexes := []string{
		"idx_items_workspace_id",
		"idx_users_email",
		"idx_system_settings_key",
		"idx_statuses_category_id",
	}

	for _, index := range coreIndexes {
		tdb.AssertIndexExists(t, index)
	}
}

func TestDatabase_Initialize_ExistingDatabase(t *testing.T) {
	// Create and initialize database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Get initial table count
	initialCount, err := tdb.GetTableCount()
	if err != nil {
		t.Fatalf("Failed to get initial table count: %v", err)
	}

	// Initialize again (should not recreate tables)
	if err := tdb.Initialize(); err != nil {
		t.Fatalf("Failed to reinitialize database: %v", err)
	}

	// Verify table count is unchanged
	finalCount, err := tdb.GetTableCount()
	if err != nil {
		t.Fatalf("Failed to get final table count: %v", err)
	}

	if finalCount != initialCount {
		t.Errorf("Table count changed from %d to %d after reinitialization", initialCount, finalCount)
	}
}

func TestDatabase_NewDB_ConnectionString(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		shouldFail bool
	}{
		{
			name: "Memory database",
			dsn:  ":memory:",
			shouldFail: false,
		},
		{
			name: "File database with WAL mode",
			dsn:  "test.db?_journal=WAL",
			shouldFail: false,
		},
		{
			name: "Invalid file path",
			dsn:  "/nonexistent/path/test.db",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := database.NewDB(tt.dsn)
			
			if tt.shouldFail {
				if err == nil {
					db.Close()
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer db.Close()

			// Verify foreign keys are enabled
			var fkEnabled int
			if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
				t.Fatalf("Failed to check foreign key status: %v", err)
			}
			if fkEnabled == 0 {
				t.Error("Foreign key constraints not enabled")
			}

			// If not memory database, verify WAL mode
			if tt.dsn != ":memory:" {
				var journalMode string
				if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
					t.Fatalf("Failed to check journal mode: %v", err)
				}
				if journalMode != "wal" {
					t.Errorf("Expected WAL mode, got %s", journalMode)
				}
			}
		})
	}
}

func TestDatabase_DefaultData_SystemSettings(t *testing.T) {
	// Create and initialize database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Verify system settings were created
	expectedSettings := map[string]struct {
		value     string
		valueType string
		category  string
	}{
		"setup_completed":         {"false", "boolean", "setup"},
		"admin_user_created":      {"false", "boolean", "setup"},
		"time_tracking_enabled":   {"true", "boolean", "modules"},
		"test_management_enabled": {"true", "boolean", "modules"},
		"calendar_feed_enabled":   {"true", "boolean", "security"},
	}

	for key, expected := range expectedSettings {
		var value, valueType, category string
		err := tdb.QueryRow(`
			SELECT value, value_type, category 
			FROM system_settings 
			WHERE key = ?
		`, key).Scan(&value, &valueType, &category)
		
		if err != nil {
			t.Fatalf("Failed to query system setting %s: %v", key, err)
		}
		
		if value != expected.value {
			t.Errorf("Setting %s: expected value %s, got %s", key, expected.value, value)
		}
		if valueType != expected.valueType {
			t.Errorf("Setting %s: expected type %s, got %s", key, expected.valueType, valueType)
		}
		if category != expected.category {
			t.Errorf("Setting %s: expected category %s, got %s", key, expected.category, category)
		}
	}
}

func TestDatabase_DefaultData_StatusSystem(t *testing.T) {
	// Create and initialize database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Verify status categories were created
	var categoryCount int
	err := tdb.QueryRow("SELECT COUNT(*) FROM status_categories").Scan(&categoryCount)
	if err != nil {
		t.Fatalf("Failed to count status categories: %v", err)
	}
	if categoryCount < 3 {
		t.Errorf("Expected at least 3 status categories, got %d", categoryCount)
	}

	// Verify statuses were created
	var statusCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM statuses").Scan(&statusCount)
	if err != nil {
		t.Fatalf("Failed to count statuses: %v", err)
	}
	if statusCount < 3 {
		t.Errorf("Expected at least 3 statuses, got %d", statusCount)
	}

	// Verify default workflow exists
	var workflowCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM workflows WHERE is_default = 1").Scan(&workflowCount)
	if err != nil {
		t.Fatalf("Failed to count default workflows: %v", err)
	}
	if workflowCount != 1 {
		t.Errorf("Expected 1 default workflow, got %d", workflowCount)
	}

	// Verify workflow transitions exist
	var transitionCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM workflow_transitions").Scan(&transitionCount)
	if err != nil {
		t.Fatalf("Failed to count workflow transitions: %v", err)
	}
	if transitionCount < 4 {
		t.Errorf("Expected at least 4 workflow transitions, got %d", transitionCount)
	}
}

func TestDatabase_DefaultData_ScreenSystem(t *testing.T) {
	// Create and initialize database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Verify default screen exists
	var screenCount int
	err := tdb.QueryRow("SELECT COUNT(*) FROM screens").Scan(&screenCount)
	if err != nil {
		t.Fatalf("Failed to count screens: %v", err)
	}
	if screenCount < 1 {
		t.Error("Expected at least 1 default screen")
	}

	// Verify screen has fields
	var fieldCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM screen_fields").Scan(&fieldCount)
	if err != nil {
		t.Fatalf("Failed to count screen fields: %v", err)
	}
	if fieldCount < 3 {
		t.Errorf("Expected at least 3 screen fields, got %d", fieldCount)
	}

	// Verify configuration set exists
	var configSetCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE is_default = 1").Scan(&configSetCount)
	if err != nil {
		t.Fatalf("Failed to count configuration sets: %v", err)
	}
	if configSetCount != 1 {
		t.Errorf("Expected 1 default configuration set, got %d", configSetCount)
	}
}

func TestDatabase_DefaultData_LinkTypes(t *testing.T) {
	// Create and initialize database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Verify link types were created
	var linkTypeCount int
	err := tdb.QueryRow("SELECT COUNT(*) FROM link_types WHERE is_system = 1").Scan(&linkTypeCount)
	if err != nil {
		t.Fatalf("Failed to count system link types: %v", err)
	}
	if linkTypeCount < 5 {
		t.Errorf("Expected at least 5 system link types, got %d", linkTypeCount)
	}

	// Verify specific link types exist
	expectedLinkTypes := []string{"Tests", "Implements", "Depends On", "Relates To", "Links To"}
	for _, linkTypeName := range expectedLinkTypes {
		var exists bool
		err := tdb.QueryRow("SELECT EXISTS(SELECT 1 FROM link_types WHERE name = ?)", linkTypeName).Scan(&exists)
		if err != nil {
			t.Fatalf("Failed to check link type %s: %v", linkTypeName, err)
		}
		if !exists {
			t.Errorf("Expected link type '%s' to exist", linkTypeName)
		}
	}
}

func TestDatabase_SchemaColumns(t *testing.T) {
	// Create fresh database
	tdb := testutils.CreateFreshDB(t, true)
	defer tdb.Close()

	// Initialize database
	if err := tdb.Initialize(); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Verify expected columns exist in the schema
	tdb.AssertColumnExists(t, "items", "rank")
	tdb.AssertColumnExists(t, "items", "assignee_id")
	tdb.AssertColumnExists(t, "items", "creator_id")
	tdb.AssertColumnExists(t, "workspaces", "time_project_id")
	tdb.AssertColumnExists(t, "time_worklogs", "item_id")
}