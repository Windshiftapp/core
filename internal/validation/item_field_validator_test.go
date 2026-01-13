// go:build test

package validation

import (
	"testing"
	"time"

	"windshift/internal/handlers/testutils"
	"windshift/internal/models"
)

// TestData holds IDs for test entities
type TestData struct {
	WorkspaceID  int
	StatusID     int
	PriorityID   int
	UserID       int
	ProjectID    int
	MilestoneID  int
	IterationID  int
}

func TestItemFieldValidator(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true) // Use in-memory database
	defer tdb.Close()

	validator := NewItemFieldValidator(tdb.GetDatabase())

	// Setup test data and get IDs
	testData := setupTestData(t, tdb)

	t.Run("ValidateTitle", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"title": "  Test Item  ",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.Title == "" {
			t.Error("Title should be set and trimmed")
		}
	})

	t.Run("ValidateEmptyTitle", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"title": "   ",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err == nil {
			t.Error("Expected validation error for empty title")
		}

		if valErr, ok := err.(*ValidationError); ok {
			if valErr.Field != "title" {
				t.Errorf("Expected field 'title', got '%s'", valErr.Field)
			}
		} else {
			t.Error("Expected ValidationError type")
		}
	})

	t.Run("ValidateDescription", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"description": "Test description",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.Description == "" {
			t.Error("Description should be set")
		}
	})

	t.Run("ValidateTaskStatus", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID, IsTask: true, Status: "open"}
		updateData := map[string]interface{}{
			"status": "in_progress",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err == nil {
			t.Error("Expected validation error for invalid task status")
		}

		// Valid task statuses should work
		item.Status = "open"
		updateData["status"] = "completed"
		err = validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Errorf("Expected no error for valid task status, got: %v", err)
		}
	})

	t.Run("ValidateStatusID", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"status_id": float64(testData.StatusID),
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.StatusID == nil || *item.StatusID != testData.StatusID {
			t.Errorf("Status ID should be set to %d", testData.StatusID)
		}
	})

	t.Run("ValidateInvalidStatusID", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"status_id": float64(9999), // Non-existent status
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err == nil {
			t.Error("Expected validation error for invalid status ID")
		}

		if valErr, ok := err.(*ValidationError); ok {
			if valErr.Field != "status_id" {
				t.Errorf("Expected field 'status_id', got '%s'", valErr.Field)
			}
		}
	})

	t.Run("ValidateNullableStatusID", func(t *testing.T) {
		statusID := testData.StatusID
		item := &models.Item{WorkspaceID: testData.WorkspaceID, StatusID: &statusID}
		updateData := map[string]interface{}{
			"status_id": nil, // Clear status ID
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.StatusID != nil {
			t.Error("Status ID should be nil")
		}
	})

	t.Run("ValidateDueDate", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"due_date": "2025-12-31",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.DueDate == nil {
			t.Error("Due date should be set")
		}

		expected := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		if !item.DueDate.Equal(expected) {
			t.Errorf("Expected due date %v, got %v", expected, *item.DueDate)
		}
	})

	t.Run("ValidateInvalidDueDate", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"due_date": "invalid-date",
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err == nil {
			t.Error("Expected validation error for invalid due date format")
		}
	})

	t.Run("ValidateProjectInheritance", func(t *testing.T) {
		projectID := testData.ProjectID
		item := &models.Item{WorkspaceID: testData.WorkspaceID, ProjectID: &projectID}

		// Setting inherit_project should clear project_id
		updateData := map[string]interface{}{
			"inherit_project": true,
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !item.InheritProject {
			t.Error("InheritProject should be true")
		}
		if item.ProjectID != nil {
			t.Error("ProjectID should be nil when inheriting")
		}
	})

	t.Run("ValidateProjectIDClearsInherit", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID, InheritProject: true}
		updateData := map[string]interface{}{
			"project_id": float64(testData.ProjectID),
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.InheritProject {
			t.Error("InheritProject should be false when setting project_id")
		}
		if item.ProjectID == nil || *item.ProjectID != testData.ProjectID {
			t.Error("ProjectID should be set")
		}
	})

	t.Run("ValidateWorkspaceID", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"workspace_id": float64(testData.WorkspaceID),
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("ValidateInvalidWorkspaceID", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"workspace_id": float64(9999),
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err == nil {
			t.Error("Expected validation error for invalid workspace ID")
		}
	})

	t.Run("ValidateCustomFieldValues", func(t *testing.T) {
		item := &models.Item{WorkspaceID: testData.WorkspaceID}
		updateData := map[string]interface{}{
			"custom_field_values": map[string]interface{}{
				"field1": "value1",
				"field2": 42,
			},
		}

		err := validator.ValidateAndApplyUpdates(item, updateData, testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.CustomFieldValues == nil {
			t.Error("Custom field values should be set")
		}
		if len(item.CustomFieldValues) != 2 {
			t.Errorf("Expected 2 custom field values, got %d", len(item.CustomFieldValues))
		}
	})

	t.Run("ValidateCreateRequest", func(t *testing.T) {
		item := &models.Item{
			WorkspaceID: testData.WorkspaceID,
			Title:       "Test Item",
		}

		err := validator.ValidateCreateRequest(item)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("ValidateCreateRequestMissingTitle", func(t *testing.T) {
		item := &models.Item{
			WorkspaceID: testData.WorkspaceID,
			Title:       "",
		}

		err := validator.ValidateCreateRequest(item)
		if err == nil {
			t.Error("Expected validation error for missing title")
		}
	})

	t.Run("ValidateCreateRequestInvalidWorkspace", func(t *testing.T) {
		item := &models.Item{
			WorkspaceID: 9999,
			Title:       "Test Item",
		}

		err := validator.ValidateCreateRequest(item)
		if err == nil {
			t.Error("Expected validation error for invalid workspace")
		}
	})
}

func TestEntityExists(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	validator := NewItemFieldValidator(tdb.GetDatabase())
	testData := setupTestData(t, tdb)

	t.Run("ExistingEntity", func(t *testing.T) {
		exists, err := validator.EntityExists("workspaces", testData.WorkspaceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !exists {
			t.Error("Expected entity to exist")
		}
	})

	t.Run("NonExistentEntity", func(t *testing.T) {
		exists, err := validator.EntityExists("workspaces", 9999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if exists {
			t.Error("Expected entity to not exist")
		}
	})
}

func TestConvertCustomFieldValuesToJSON(t *testing.T) {
	t.Run("ValidConversion", func(t *testing.T) {
		values := map[string]interface{}{
			"field1": "value1",
			"field2": 42,
		}

		result, err := ConvertCustomFieldValuesToJSON(values)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !result.Valid {
			t.Error("Expected valid NullString")
		}
		if result.String == "" {
			t.Error("Expected non-empty JSON string")
		}
	})

	t.Run("NilConversion", func(t *testing.T) {
		result, err := ConvertCustomFieldValuesToJSON(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected invalid NullString for nil input")
		}
	})

	t.Run("EmptyConversion", func(t *testing.T) {
		result, err := ConvertCustomFieldValuesToJSON(make(map[string]interface{}))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Valid {
			t.Error("Expected invalid NullString for empty map")
		}
	})
}

// setupTestData creates minimal test data for validation tests
func setupTestData(t *testing.T, tdb *testutils.TestDB) *TestData {
	now := time.Now()

	// Default data is already loaded, so we just need to query for existing IDs
	// and create additional test-specific data

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

	// Get existing status category ID (default data should exist)
	var statusCategoryID int
	err = tdb.DB.QueryRow("SELECT id FROM status_categories LIMIT 1").Scan(&statusCategoryID)
	if err != nil {
		t.Fatalf("Failed to get status category: %v", err)
	}

	// Get existing status ID or create one
	var statusID int
	err = tdb.DB.QueryRow("SELECT id FROM statuses WHERE category_id = ? LIMIT 1", statusCategoryID).Scan(&statusID)
	if err != nil {
		// Create status if none exists
		statusResult, err := tdb.DB.Exec(`
			INSERT INTO statuses (name, description, category_id, created_at, updated_at)
			VALUES ('Open', 'Open status', ?, ?, ?)
		`, statusCategoryID, now, now)
		if err != nil {
			t.Fatalf("Failed to create status: %v", err)
		}
		statusIDInt64, _ := statusResult.LastInsertId()
		statusID = int(statusIDInt64)
	}

	// Get existing priority ID (default data should exist)
	var priorityID int
	err = tdb.DB.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get priority: %v", err)
	}

	// Get or create user
	var userID int
	err = tdb.DB.QueryRow("SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		// Create user if none exists
		userResult, err := tdb.DB.Exec(`
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
		// Create time project if none exists
		projectResult, err := tdb.DB.Exec(`
			INSERT INTO time_projects (name, description, active, created_at, updated_at)
			VALUES ('Test Project', 'Test project', 1, ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create time project: %v", err)
		}
		projectID64, _ := projectResult.LastInsertId()
		projectID = int(projectID64)
	}

	// Try to get existing milestone or use 0
	var milestoneID int
	err = tdb.DB.QueryRow("SELECT id FROM milestones LIMIT 1").Scan(&milestoneID)
	if err != nil {
		milestoneID = 0 // No milestones exist, tests will skip milestone-related validations
	}

	// Try to get existing iteration or use 0
	var iterationID int
	err = tdb.DB.QueryRow("SELECT id FROM iterations LIMIT 1").Scan(&iterationID)
	if err != nil {
		iterationID = 0 // No iterations exist, tests will skip iteration-related validations
	}

	return &TestData{
		WorkspaceID:  workspaceID,
		StatusID:     statusID,
		PriorityID:   priorityID,
		UserID:       userID,
		ProjectID:    projectID,
		MilestoneID:  milestoneID,
		IterationID:  iterationID,
	}
}
