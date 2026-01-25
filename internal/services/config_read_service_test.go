package services

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// createConfigTestDB creates a test database for config read service tests
func createConfigTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "config_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

func TestConfigReadService_ListItemTypes(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	t.Run("ReturnsItemTypes", func(t *testing.T) {
		types, err := service.ListItemTypes()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Database initialization creates default item types
		if len(types) == 0 {
			t.Error("Expected at least one item type from default data")
		}
	})
}

func TestConfigReadService_GetItemType(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	// Get the first item type ID
	var typeID int
	err := db.QueryRow("SELECT id FROM item_types LIMIT 1").Scan(&typeID)
	if err != nil {
		t.Fatalf("Failed to get item type ID: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		itemType, err := service.GetItemType(typeID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if itemType.ID != typeID {
			t.Errorf("Expected item type ID %d, got %d", typeID, itemType.ID)
		}
		if itemType.Name == "" {
			t.Error("Expected item type name to be populated")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetItemType(99999)
		if err == nil {
			t.Error("Expected error for non-existent item type")
		}
	})
}

func TestConfigReadService_ListPriorities(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	t.Run("ReturnsPriorities", func(t *testing.T) {
		priorities, err := service.ListPriorities()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Database initialization creates default priorities
		if len(priorities) == 0 {
			t.Error("Expected at least one priority from default data")
		}
	})
}

func TestConfigReadService_GetPriority(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	// Get the first priority ID
	var priorityID int
	err := db.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get priority ID: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		priority, err := service.GetPriority(priorityID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if priority.ID != priorityID {
			t.Errorf("Expected priority ID %d, got %d", priorityID, priority.ID)
		}
		if priority.Name == "" {
			t.Error("Expected priority name to be populated")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetPriority(99999)
		if err == nil {
			t.Error("Expected error for non-existent priority")
		}
	})
}

func TestConfigReadService_ListCustomFields(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	t.Run("ReturnsCustomFields", func(t *testing.T) {
		fields, err := service.ListCustomFields()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// May be empty if no custom fields are defined
		if fields == nil {
			t.Error("Expected non-nil result")
		}
	})
}

func TestConfigReadService_GetCustomField(t *testing.T) {
	db := createConfigTestDB(t)
	defer db.Close()

	service := NewConfigReadService(db)

	// Create a custom field for testing
	result, err := db.Exec(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, display_order, created_at, updated_at)
		VALUES ('Test Field', 'text', 'A test field', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create custom field: %v", err)
	}
	fieldID, _ := result.LastInsertId()

	t.Run("Success", func(t *testing.T) {
		field, err := service.GetCustomField(int(fieldID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if field.ID != int(fieldID) {
			t.Errorf("Expected field ID %d, got %d", fieldID, field.ID)
		}
		if field.Name != "Test Field" {
			t.Errorf("Expected field name 'Test Field', got '%s'", field.Name)
		}
		if field.FieldType != "text" {
			t.Errorf("Expected field type 'text', got '%s'", field.FieldType)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetCustomField(99999)
		if err == nil {
			t.Error("Expected error for non-existent custom field")
		}
	})
}
