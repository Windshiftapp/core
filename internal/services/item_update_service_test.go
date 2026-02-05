//go:build test

package services

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestItemUpdateService(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewItemUpdateService(tdb.GetDatabase())

	// Setup test data
	testData := setupUpdateServiceTestData(t, tdb)

	t.Run("UpdateItemTitle", func(t *testing.T) {
		updateData := map[string]interface{}{
			"title": "Updated Test Item",
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Item.Title != "Updated Test Item" {
			t.Errorf("Expected title 'Updated Test Item', got '%s'", result.Item.Title)
		}

		// Verify history was recorded
		if len(result.FieldChanges) == 0 {
			t.Error("Expected history entries for title change")
		}

		foundTitleChange := false
		for _, change := range result.FieldChanges {
			if change.FieldName == "title" {
				foundTitleChange = true
				if change.NewValue != "Updated Test Item" {
					t.Errorf("Expected new value 'Updated Test Item', got '%s'", change.NewValue)
				}
			}
		}
		if !foundTitleChange {
			t.Error("Expected title change in history")
		}
	})

	t.Run("UpdateItemStatus", func(t *testing.T) {
		updateData := map[string]interface{}{
			"status_id": float64(testData.StatusID),
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Item.StatusID == nil {
			t.Error("Expected status ID to be set")
		} else if *result.Item.StatusID != testData.StatusID {
			t.Errorf("Expected status ID %d, got %d", testData.StatusID, *result.Item.StatusID)
		}

		if !result.StatusChanged {
			t.Error("Expected status changed flag to be true")
		}
	})

	t.Run("UpdateMultipleFields", func(t *testing.T) {
		updateData := map[string]interface{}{
			"title":       "Multi-Field Update",
			"description": "Testing multiple field updates",
			"priority_id": float64(testData.PriorityID),
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Item.Title != "Multi-Field Update" {
			t.Errorf("Expected title 'Multi-Field Update', got '%s'", result.Item.Title)
		}

		if result.Item.Description != "Testing multiple field updates" {
			t.Errorf("Expected description to be updated")
		}

		if result.Item.PriorityID == nil || *result.Item.PriorityID != testData.PriorityID {
			t.Error("Expected priority ID to be set")
		}

		// Verify multiple history entries
		if len(result.FieldChanges) < 2 {
			t.Errorf("Expected at least 2 history entries, got %d", len(result.FieldChanges))
		}
	})

	t.Run("ClearNullableField", func(t *testing.T) {
		// First set a priority
		setupData := map[string]interface{}{
			"priority_id": float64(testData.PriorityID),
		}
		service.UpdateItem(UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: setupData,
			UserID:     testData.UserID,
		})

		// Now clear it
		updateData := map[string]interface{}{
			"priority_id": nil,
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Item.PriorityID != nil {
			t.Error("Expected priority ID to be cleared (nil)")
		}
	})

	t.Run("UpdateWithProjectInheritance", func(t *testing.T) {
		// Set a direct project first
		setupData := map[string]interface{}{
			"project_id": float64(testData.ProjectID),
		}
		service.UpdateItem(UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: setupData,
			UserID:     testData.UserID,
		})

		// Now set inherit_project
		updateData := map[string]interface{}{
			"inherit_project": true,
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Item.InheritProject {
			t.Error("Expected inherit_project to be true")
		}

		if result.Item.ProjectID != nil {
			t.Error("Expected project_id to be cleared when inherit_project is set")
		}
	})

	t.Run("UpdateItemNotFound", func(t *testing.T) {
		updateData := map[string]interface{}{
			"title": "Non-existent Item",
		}

		req := UpdateItemRequest{
			ItemID:     99999,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		_, err := service.UpdateItem(req)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})

	t.Run("UpdateWithValidationError", func(t *testing.T) {
		updateData := map[string]interface{}{
			"title": "   ", // Empty title should fail validation
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		_, err := service.UpdateItem(req)
		if err == nil {
			t.Error("Expected validation error for empty title")
		}
	})

	t.Run("UpdateWithCustomFields", func(t *testing.T) {
		updateData := map[string]interface{}{
			"custom_field_values": map[string]interface{}{
				"custom1": "value1",
				"custom2": 42,
			},
		}

		req := UpdateItemRequest{
			ItemID:     testData.ItemID,
			UpdateData: updateData,
			UserID:     testData.UserID,
		}

		result, err := service.UpdateItem(req)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Item.CustomFieldValues == nil {
			t.Error("Expected custom field values to be set")
		}

		if len(result.Item.CustomFieldValues) != 2 {
			t.Errorf("Expected 2 custom fields, got %d", len(result.Item.CustomFieldValues))
		}
	})
}

func TestHistoryGeneration(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewItemUpdateService(tdb.GetDatabase())

	t.Run("CompareAndGenerateHistory", func(t *testing.T) {
		original := &models.Item{
			ID:          1,
			Title:       "Original Title",
			Description: "Original Description",
			WorkspaceID: 1,
		}

		updated := &models.Item{
			ID:          1,
			Title:       "Updated Title",
			Description: "Original Description", // Unchanged
			WorkspaceID: 1,
		}

		history := service.compareAndGenerateHistory(original, updated, 1)

		// Should have exactly 1 change (title)
		if len(history) != 1 {
			t.Errorf("Expected 1 history entry, got %d", len(history))
		}

		if len(history) > 0 {
			if history[0].FieldName != "title" {
				t.Errorf("Expected field name 'title', got '%s'", history[0].FieldName)
			}
			if history[0].OldValue != "Original Title" {
				t.Errorf("Expected old value 'Original Title', got '%s'", history[0].OldValue)
			}
			if history[0].NewValue != "Updated Title" {
				t.Errorf("Expected new value 'Updated Title', got '%s'", history[0].NewValue)
			}
		}
	})

	t.Run("NullableFieldChanges", func(t *testing.T) {
		statusID1 := 1
		statusID2 := 2

		original := &models.Item{
			ID:       1,
			StatusID: &statusID1,
		}

		updated := &models.Item{
			ID:       1,
			StatusID: &statusID2,
		}

		history := service.compareAndGenerateHistory(original, updated, 1)

		foundStatusChange := false
		for _, entry := range history {
			if entry.FieldName == "status_id" {
				foundStatusChange = true
				if entry.OldValue != "1" {
					t.Errorf("Expected old value '1', got '%s'", entry.OldValue)
				}
				if entry.NewValue != "2" {
					t.Errorf("Expected new value '2', got '%s'", entry.NewValue)
				}
			}
		}

		if !foundStatusChange {
			t.Error("Expected status_id change in history")
		}
	})

	t.Run("NullToValueChange", func(t *testing.T) {
		newStatusID := 1

		original := &models.Item{
			ID:       1,
			StatusID: nil,
		}

		updated := &models.Item{
			ID:       1,
			StatusID: &newStatusID,
		}

		history := service.compareAndGenerateHistory(original, updated, 1)

		foundStatusChange := false
		for _, entry := range history {
			if entry.FieldName == "status_id" {
				foundStatusChange = true
				if entry.OldValue != "" {
					t.Errorf("Expected empty old value, got '%s'", entry.OldValue)
				}
				if entry.NewValue != "1" {
					t.Errorf("Expected new value '1', got '%s'", entry.NewValue)
				}
			}
		}

		if !foundStatusChange {
			t.Error("Expected status_id change in history")
		}
	})
}

func TestStatusChangeDetection(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewItemUpdateService(tdb.GetDatabase())

	t.Run("StatusChanged_NullToValue", func(t *testing.T) {
		statusID := 1
		original := &models.Item{StatusID: nil}
		updated := &models.Item{StatusID: &statusID}

		if !service.hasStatusChanged(original, updated) {
			t.Error("Expected status changed to be true")
		}
	})

	t.Run("StatusChanged_ValueToNull", func(t *testing.T) {
		statusID := 1
		original := &models.Item{StatusID: &statusID}
		updated := &models.Item{StatusID: nil}

		if !service.hasStatusChanged(original, updated) {
			t.Error("Expected status changed to be true")
		}
	})

	t.Run("StatusChanged_DifferentValues", func(t *testing.T) {
		statusID1 := 1
		statusID2 := 2
		original := &models.Item{StatusID: &statusID1}
		updated := &models.Item{StatusID: &statusID2}

		if !service.hasStatusChanged(original, updated) {
			t.Error("Expected status changed to be true")
		}
	})

	t.Run("StatusNotChanged_SameValue", func(t *testing.T) {
		statusID := 1
		original := &models.Item{StatusID: &statusID}
		updated := &models.Item{StatusID: &statusID}

		if service.hasStatusChanged(original, updated) {
			t.Error("Expected status changed to be false")
		}
	})

	t.Run("StatusNotChanged_BothNull", func(t *testing.T) {
		original := &models.Item{StatusID: nil}
		updated := &models.Item{StatusID: nil}

		if service.hasStatusChanged(original, updated) {
			t.Error("Expected status changed to be false")
		}
	})
}

// setupUpdateServiceTestData creates test data for update service tests
func setupUpdateServiceTestData(t *testing.T, tdb *testutils.TestDB) *UpdateServiceTestData {
	now := time.Now()

	// Create workspace
	result, err := tdb.DB.Exec(`
		INSERT INTO workspaces (name, key, description, created_at, updated_at)
		VALUES ('Test Workspace', 'TEST', 'Test workspace', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := result.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get status and priority from default data
	var statusID, priorityID int
	err = tdb.DB.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	err = tdb.DB.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get priority: %v", err)
	}

	// Get or create user
	var userID int
	err = tdb.DB.QueryRow("SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		var userResult sql.Result
		userResult, err = tdb.DB.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES ('testuser', 'test@example.com', 'Test', 'User', 'hash', ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		userID64, _ := userResult.LastInsertId()
		userID = int(userID64)
	}

	// Get or create time project
	var projectID int
	err = tdb.DB.QueryRow("SELECT id FROM time_projects LIMIT 1").Scan(&projectID)
	if err != nil {
		var projectResult sql.Result
		projectResult, err = tdb.DB.Exec(`
			INSERT INTO time_projects (name, description, created_at, updated_at)
			VALUES ('Test Project', 'Test project', ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create time project: %v", err)
		}
		projectID64, _ := projectResult.LastInsertId()
		projectID = int(projectID64)
	}

	// Create test item (without status_id so we can test status changes)
	itemResult, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, is_task,
		                   frac_index, path, created_at, updated_at)
		VALUES (?, 1, 'Test Item', 'Test Description', 0, 'a0', '/1/', ?, ?)
	`, workspaceID, now, now)
	if err != nil {
		t.Fatalf("Failed to create test item: %v", err)
	}

	itemID64, _ := itemResult.LastInsertId()
	itemID := int(itemID64)

	// Update the path with the actual item ID
	_, err = tdb.DB.Exec(`UPDATE items SET path = ? WHERE id = ?`, fmt.Sprintf("/%d/", itemID), itemID)
	if err != nil {
		t.Fatalf("Failed to update item path: %v", err)
	}

	return &UpdateServiceTestData{
		WorkspaceID: workspaceID,
		ItemID:      itemID,
		StatusID:    statusID,
		PriorityID:  priorityID,
		UserID:      userID,
		ProjectID:   projectID,
	}
}

type UpdateServiceTestData struct {
	WorkspaceID int
	ItemID      int
	StatusID    int
	PriorityID  int
	UserID      int
	ProjectID   int
}
