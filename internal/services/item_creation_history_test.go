//go:build test

package services

import (
	"testing"

	"windshift/internal/handlers/testutils"
)

func TestRecordItemCreationHistory(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewItemUpdateService(tdb.GetDatabase())

	// Setup test data
	testData := setupUpdateServiceTestData(t, tdb)

	t.Run("RecordCreationHistory", func(t *testing.T) {
		// Record creation history for the test item
		err := service.RecordItemCreationHistory(tdb.GetDatabase(), testData.ItemID, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error recording creation history, got: %v", err)
		}

		// Verify history entries were created
		rows, err := tdb.DB.Query(`
			SELECT field_name, old_value, new_value, user_id
			FROM item_history
			WHERE item_id = ?
			ORDER BY id
		`, testData.ItemID)
		if err != nil {
			t.Fatalf("Failed to query history: %v", err)
		}
		defer rows.Close()

		var entries []struct {
			FieldName string
			OldValue string
			NewValue string
			UserID   int
		}

		for rows.Next() {
			var entry struct {
				FieldName string
				OldValue string
				NewValue string
				UserID   int
			}
			err := rows.Scan(&entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.UserID)
			if err != nil {
				t.Fatalf("Failed to scan history row: %v", err)
			}
			entries = append(entries, entry)
		}

		// Should have history entries for multiple fields
		if len(entries) == 0 {
			t.Error("Expected at least one history entry")
		}

		// Verify old_value is empty (it's a creation event)
		for _, entry := range entries {
			if entry.OldValue != "" {
				t.Errorf("Old value should be empty for creation history, got '%s'", entry.OldValue)
			}
		}

		// Verify user_id is correct
		for _, entry := range entries {
			if entry.UserID != testData.UserID {
				t.Errorf("Expected user_id %d, got %d", testData.UserID, entry.UserID)
			}
		}

		// Verify we have entries for expected fields
		foundTitle := false
		foundDescription := false
		for _, entry := range entries {
			if entry.FieldName == "title" {
				foundTitle = true
				if entry.NewValue != "Test Item" {
					t.Errorf("Expected new value 'Test Item', got '%s'", entry.NewValue)
				}
			}
			if entry.FieldName == "description" {
				foundDescription = true
				if entry.NewValue != "Test Description" {
					t.Errorf("Expected new value 'Test Description', got '%s'", entry.NewValue)
				}
			}
		}

		if !foundTitle {
			t.Error("Expected to find title history entry")
		}
		if !foundDescription {
			t.Error("Expected to find description history entry")
		}
	})

	t.Run("RecordCreationHistoryNonExistentItem", func(t *testing.T) {
		// Try to record history for a non-existent item
		err := service.RecordItemCreationHistory(tdb.GetDatabase(), 99999, testData.UserID)
		if err == nil {
			t.Error("Expected error when recording history for non-existent item")
		}
	})
}
