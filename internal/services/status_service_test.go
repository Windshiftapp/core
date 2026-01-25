package services

import (
	"path/filepath"
	"strings"
	"testing"

	"windshift/internal/database"
)

// createStatusTestDB creates a test database for status service tests
func createStatusTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "status_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

func TestStatusService_ListStatuses(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	statuses, err := service.ListStatuses()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Database initialization creates default statuses
	if len(statuses) == 0 {
		t.Error("Expected at least one status from default data")
	}

	// Check that category details are populated
	for _, status := range statuses {
		if status.CategoryName == "" {
			t.Error("Expected category name to be populated")
		}
	}
}

func TestStatusService_GetStatus(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	// Get the first status ID
	var statusID int
	err := db.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status ID: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		status, err := service.GetStatus(statusID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if status.ID != statusID {
			t.Errorf("Expected status ID %d, got %d", statusID, status.ID)
		}
		if status.Name == "" {
			t.Error("Expected status name to be populated")
		}
		if status.CategoryName == "" {
			t.Error("Expected category name to be populated")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetStatus(99999)
		if err == nil {
			t.Error("Expected error for non-existent status")
		}
	})
}

func TestStatusService_ListCategories(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	categories, err := service.ListCategories()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Database initialization creates default status categories
	if len(categories) == 0 {
		t.Error("Expected at least one category from default data")
	}

	// Check that required fields are populated
	for _, cat := range categories {
		if cat.Name == "" {
			t.Error("Expected category name to be populated")
		}
		if cat.Color == "" {
			t.Error("Expected category color to be populated")
		}
	}
}

func TestStatusService_GetCategory(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	// Get the first category ID
	var categoryID int
	err := db.QueryRow("SELECT id FROM status_categories LIMIT 1").Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to get category ID: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		category, err := service.GetCategory(categoryID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if category.ID != categoryID {
			t.Errorf("Expected category ID %d, got %d", categoryID, category.ID)
		}
		if category.Name == "" {
			t.Error("Expected category name to be populated")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetCategory(99999)
		if err == nil {
			t.Error("Expected error for non-existent category")
		}
	})
}

func TestStatusService_GetStatus_ZeroID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetStatus(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestStatusService_GetCategory_ZeroID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetCategory(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestStatusService_GetStatus_NegativeID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetStatus(-1)
	if err == nil {
		t.Error("Expected error for negative ID")
	}
}

func TestStatusService_GetCategory_NegativeID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetCategory(-1)
	if err == nil {
		t.Error("Expected error for negative ID")
	}
}

func TestStatusService_GetStatus_ErrorMessageContainsID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetStatus(99999)
	if err == nil {
		t.Fatal("Expected error for non-existent status")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "99999") {
		t.Errorf("Expected error message to contain ID '99999', got: %s", errMsg)
	}
}

func TestStatusService_GetCategory_ErrorMessageContainsID(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	_, err := service.GetCategory(99999)
	if err == nil {
		t.Fatal("Expected error for non-existent category")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "99999") {
		t.Errorf("Expected error message to contain ID '99999', got: %s", errMsg)
	}
}

func TestStatusService_ListStatuses_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	// Delete all statuses and categories to test empty result
	db.Exec("DELETE FROM statuses")

	service := NewStatusService(db)

	statuses, err := service.ListStatuses()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return empty slice, not nil
	if statuses == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(statuses) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(statuses))
	}
}

func TestStatusService_ListCategories_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	// Delete all categories to test empty result (statuses first due to FK)
	db.Exec("DELETE FROM statuses")
	db.Exec("DELETE FROM status_categories")

	service := NewStatusService(db)

	categories, err := service.ListCategories()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return empty slice, not nil
	if categories == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(categories) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(categories))
	}
}

func TestStatusService_GetStatus_PopulatesAllFields(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	// Get an existing status
	var statusID int
	err := db.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status ID: %v", err)
	}

	status, err := service.GetStatus(statusID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify all expected fields are populated
	if status.ID == 0 {
		t.Error("Expected ID to be populated")
	}
	if status.Name == "" {
		t.Error("Expected Name to be populated")
	}
	if status.CategoryID == 0 {
		t.Error("Expected CategoryID to be populated")
	}
	if status.CategoryName == "" {
		t.Error("Expected CategoryName to be populated")
	}
	if status.CategoryColor == "" {
		t.Error("Expected CategoryColor to be populated")
	}
	if status.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be populated")
	}
	if status.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be populated")
	}
}

func TestStatusService_GetCategory_PopulatesAllFields(t *testing.T) {
	db := createStatusTestDB(t)
	defer db.Close()

	service := NewStatusService(db)

	// Get an existing category
	var categoryID int
	err := db.QueryRow("SELECT id FROM status_categories LIMIT 1").Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to get category ID: %v", err)
	}

	category, err := service.GetCategory(categoryID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify all expected fields are populated
	if category.ID == 0 {
		t.Error("Expected ID to be populated")
	}
	if category.Name == "" {
		t.Error("Expected Name to be populated")
	}
	if category.Color == "" {
		t.Error("Expected Color to be populated")
	}
}
