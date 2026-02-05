//go:build test

package handlers

import (
	"net/http"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
	"windshift/internal/testutils/mocks"
)

func createTestWorkspace(t *testing.T, tdb *testutils.TestDB, name, key string) int {
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES (?, ?, 'Test workspace', 1, datetime('now'), datetime('now'))
	`, name, key)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func createTestWorkflow(t *testing.T, tdb *testutils.TestDB, name string) int {
	result, err := tdb.Exec(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES (?, 'Test workflow', 0, datetime('now'), datetime('now'))
	`, name)
	if err != nil {
		t.Fatalf("Failed to create test workflow: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func createTestScreen(t *testing.T, tdb *testutils.TestDB, name string) int {
	result, err := tdb.Exec(`
		INSERT INTO screens (name, description, created_at, updated_at)
		VALUES (?, 'Test screen', datetime('now'), datetime('now'))
	`, name)
	if err != nil {
		t.Fatalf("Failed to create test screen: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func TestConfigurationSetHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID1 := createTestWorkspace(t, tdb, "Workspace 1", "WS1")
	workspaceID2 := createTestWorkspace(t, tdb, "Workspace 2", "WS2")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")
	createScreenID := createTestScreen(t, tdb, "Create Screen")
	editScreenID := createTestScreen(t, tdb, "Edit Screen")

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	configSet := models.ConfigurationSet{
		Name:           "Test Configuration Set",
		Description:    "Test configuration set for unit testing",
		IsDefault:      false,
		WorkflowID:     &workflowID,
		WorkspaceIDs:   []int{workspaceID1, workspaceID2},
		CreateScreenID: &createScreenID,
		EditScreenID:   &editScreenID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", configSet)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created configuration set to have an ID")
	}
	if response.Name != configSet.Name {
		t.Errorf("Expected name %s, got %s", configSet.Name, response.Name)
	}
	if response.Description != configSet.Description {
		t.Errorf("Expected description %s, got %s", configSet.Description, response.Description)
	}
	if response.IsDefault != configSet.IsDefault {
		t.Errorf("Expected IsDefault %v, got %v", configSet.IsDefault, response.IsDefault)
	}
	if response.WorkflowID == nil || *response.WorkflowID != workflowID {
		t.Errorf("Expected workflow ID %d, got %v", workflowID, response.WorkflowID)
	}
	if len(response.WorkspaceIDs) != 2 {
		t.Errorf("Expected 2 workspace IDs, got %d", len(response.WorkspaceIDs))
	}
	if response.CreateScreenID == nil || *response.CreateScreenID != createScreenID {
		t.Errorf("Expected create screen ID %d, got %v", createScreenID, response.CreateScreenID)
	}
	if response.EditScreenID == nil || *response.EditScreenID != editScreenID {
		t.Errorf("Expected edit screen ID %d, got %v", editScreenID, response.EditScreenID)
	}

	// Verify configuration set was inserted in database
	var count int
	err := tdb.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE name = ?", configSet.Name).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify configuration set creation: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 configuration set in database, got %d", count)
	}

	// Verify workspace assignments were created
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspace_configuration_sets WHERE configuration_set_id = ?", response.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace assignments: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 workspace assignments, got %d", count)
	}

	// Verify screen assignments were created
	err = tdb.QueryRow("SELECT COUNT(*) FROM configuration_set_screens WHERE configuration_set_id = ?", response.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify screen assignments: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 screen assignments, got %d", count)
	}
}

func TestConfigurationSetHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	tests := []struct {
		name        string
		configSet   models.ConfigurationSet
		expectedErr string
	}{
		{
			name:        "Missing name",
			configSet:   models.ConfigurationSet{WorkspaceIDs: []int{workspaceID}},
			expectedErr: "Configuration set name is required",
		},
		{
			name:        "Empty name",
			configSet:   models.ConfigurationSet{Name: "   ", WorkspaceIDs: []int{workspaceID}},
			expectedErr: "Configuration set name is required",
		},
		{
			name:        "Invalid workspace ID",
			configSet:   models.ConfigurationSet{Name: "Test Config", WorkspaceIDs: []int{99999}},
			expectedErr: "One or more workspaces not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", tt.configSet)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

func TestConfigurationSetHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")
	screenID := createTestScreen(t, tdb, "Test Screen")

	// Create configuration set
	result, err := tdb.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, workflow_id, created_at, updated_at)
		VALUES ('Test Config Set', 'Test configuration', 0, ?, datetime('now'), datetime('now'))
	`, workflowID)
	if err != nil {
		t.Fatalf("Failed to create configuration set: %v", err)
	}
	configSetID, _ := result.LastInsertId()

	// Create workspace assignment
	_, err = tdb.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, datetime('now'))
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("Failed to create workspace assignment: %v", err)
	}

	// Create screen assignment
	_, err = tdb.Exec(`
		INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context, created_at)
		VALUES (?, ?, 'create', datetime('now'))
	`, configSetID, screenID)
	if err != nil {
		t.Fatalf("Failed to create screen assignment: %v", err)
	}

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/"+testutils.IntToString(int(configSetID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(configSetID)))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	if response.ID != int(configSetID) {
		t.Errorf("Expected ID %d, got %d", configSetID, response.ID)
	}
	if response.Name != "Test Config Set" {
		t.Errorf("Expected name 'Test Config Set', got %s", response.Name)
	}
	if response.WorkflowName != "Test Workflow" {
		t.Errorf("Expected workflow name 'Test Workflow', got %s", response.WorkflowName)
	}
	if len(response.WorkspaceIDs) != 1 || response.WorkspaceIDs[0] != workspaceID {
		t.Errorf("Expected workspace ID %d, got %v", workspaceID, response.WorkspaceIDs)
	}
	if len(response.Workspaces) != 1 || response.Workspaces[0] != "Test Workspace" {
		t.Errorf("Expected workspace name 'Test Workspace', got %v", response.Workspaces)
	}
	if response.CreateScreenID == nil || *response.CreateScreenID != screenID {
		t.Errorf("Expected create screen ID %d, got %v", screenID, response.CreateScreenID)
	}
	if response.CreateScreenName != "Test Screen" {
		t.Errorf("Expected create screen name 'Test Screen', got %s", response.CreateScreenName)
	}
}

func TestConfigurationSetHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestConfigurationSetHandler_GetAll_Success(t *testing.T) {
	t.Skip("TODO: Fix GetAll method database connection issue")
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")

	// Create multiple configuration sets
	configSets := []struct {
		name      string
		isDefault bool
	}{
		{"Config Set A", false},
		{"Config Set B", true}, // Default should appear first
		{"Config Set C", false},
	}

	for _, cs := range configSets {
		result, err := tdb.Exec(`
			INSERT INTO configuration_sets (name, description, is_default, workflow_id, created_at, updated_at)
			VALUES (?, 'Test configuration', ?, ?, datetime('now'), datetime('now'))
		`, cs.name, cs.isDefault, workflowID)
		if err != nil {
			t.Fatalf("Failed to create configuration set %s: %v", cs.name, err)
		}
		configSetID, _ := result.LastInsertId()

		// Assign to workspace
		_, err = tdb.Exec(`
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, datetime('now'))
		`, workspaceID, configSetID)
		if err != nil {
			t.Fatalf("Failed to create workspace assignment for %s: %v", cs.name, err)
		}
	}

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	if len(response) != len(configSets) {
		t.Errorf("Expected %d configuration sets, got %d", len(configSets), len(response))
	}

	// Verify default appears first, then alphabetical order
	expectedOrder := []string{"Config Set B", "Config Set A", "Config Set C"}
	for i, cs := range response {
		if cs.Name != expectedOrder[i] {
			t.Errorf("Expected configuration set at position %d to be %s, got %s", i, expectedOrder[i], cs.Name)
		}
		if len(cs.WorkspaceIDs) != 1 || cs.WorkspaceIDs[0] != workspaceID {
			t.Errorf("Expected workspace assignments for %s", cs.Name)
		}
	}
}

func TestConfigurationSetHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID1 := createTestWorkspace(t, tdb, "Workspace 1", "WS1")
	workspaceID2 := createTestWorkspace(t, tdb, "Workspace 2", "WS2")
	workspaceID3 := createTestWorkspace(t, tdb, "Workspace 3", "WS3")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")
	screenID := createTestScreen(t, tdb, "Test Screen")

	// Create configuration set
	result, err := tdb.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, workflow_id, created_at, updated_at)
		VALUES ('Original Name', 'Original description', 0, ?, datetime('now'), datetime('now'))
	`, workflowID)
	if err != nil {
		t.Fatalf("Failed to create configuration set: %v", err)
	}
	configSetID, _ := result.LastInsertId()

	// Create initial workspace assignments
	_, err = tdb.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, datetime('now'))
	`, workspaceID1, configSetID)
	if err != nil {
		t.Fatalf("Failed to create initial workspace assignment: %v", err)
	}

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	updatedConfigSet := models.ConfigurationSet{
		Name:         "Updated Name",
		Description:  "Updated description",
		IsDefault:    true,
		WorkflowID:   &workflowID,
		WorkspaceIDs: []int{workspaceID2, workspaceID3}, // Different workspaces
		ViewScreenID: &screenID,
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/configuration-sets/"+testutils.IntToString(int(configSetID)), updatedConfigSet)
	req.SetPathValue("id", testutils.IntToString(int(configSetID)))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	if response.Name != updatedConfigSet.Name {
		t.Errorf("Expected name %s, got %s", updatedConfigSet.Name, response.Name)
	}
	if response.Description != updatedConfigSet.Description {
		t.Errorf("Expected description %s, got %s", updatedConfigSet.Description, response.Description)
	}
	if response.IsDefault != updatedConfigSet.IsDefault {
		t.Errorf("Expected IsDefault %v, got %v", updatedConfigSet.IsDefault, response.IsDefault)
	}
	if len(response.WorkspaceIDs) != 2 {
		t.Errorf("Expected 2 workspace IDs, got %d", len(response.WorkspaceIDs))
	}

	// Verify database was updated
	var name, description string
	var isDefault bool
	err = tdb.QueryRow("SELECT name, description, is_default FROM configuration_sets WHERE id = ?", configSetID).Scan(&name, &description, &isDefault)
	if err != nil {
		t.Fatalf("Failed to verify configuration set update: %v", err)
	}
	if name != updatedConfigSet.Name || description != updatedConfigSet.Description || isDefault != updatedConfigSet.IsDefault {
		t.Error("Database was not updated correctly")
	}

	// Verify workspace assignments were updated
	var count int
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspace_configuration_sets WHERE configuration_set_id = ?", configSetID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace assignments: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 workspace assignments after update, got %d", count)
	}
}

func TestConfigurationSetHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")

	// Create configuration set
	result, err := tdb.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, created_at, updated_at)
		VALUES ('Delete Me', 'To be deleted', 0, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create configuration set: %v", err)
	}
	configSetID, _ := result.LastInsertId()

	// Create workspace assignment (should be cascade deleted)
	_, err = tdb.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, datetime('now'))
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("Failed to create workspace assignment: %v", err)
	}

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/configuration-sets/"+testutils.IntToString(int(configSetID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(configSetID)))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify configuration set was deleted
	var count int
	err = tdb.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE id = ?", configSetID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify configuration set deletion: %v", err)
	}
	if count != 0 {
		t.Error("Configuration set was not deleted from database")
	}

	// Verify workspace assignments were cascade deleted
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspace_configuration_sets WHERE configuration_set_id = ?", configSetID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace assignment cascade deletion: %v", err)
	}
	if count != 0 {
		t.Error("Workspace assignments were not cascade deleted")
	}
}

func TestConfigurationSetHandler_InvalidID_Scenarios(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	tests := []struct {
		name     string
		endpoint string
		method   string
	}{
		{"Get invalid ID", "/api/configuration-sets/invalid", "GET"},
		{"Update invalid ID", "/api/configuration-sets/invalid", "PUT"},
		{"Delete invalid ID", "/api/configuration-sets/invalid", "DELETE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			switch tt.method {
			case "GET":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, nil)
			case "PUT":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, models.ConfigurationSet{Name: "Test"})
			case "DELETE":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, nil)
			}

			// Set invalid ID in mux vars
			req.SetPathValue("id", "invalid")

			var rr *testutils.ResponseRecorder
			switch tt.method {
			case "GET":
				rr = testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)
			case "PUT":
				rr = testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)
			case "DELETE":
				rr = testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)
			}

			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}

func TestConfigurationSetHandler_TransactionRollback(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")

	// Create a configuration set that will cause constraint violation during screen assignment
	// by referencing a non-existent screen
	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	invalidScreenID := 99999
	configSet := models.ConfigurationSet{
		Name:           "Test Config Set",
		Description:    "Should fail due to invalid screen",
		WorkspaceIDs:   []int{workspaceID},
		CreateScreenID: &invalidScreenID, // Non-existent screen
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", configSet)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	// Should get an internal server error due to constraint violation
	testutils.AssertInternalServerError(t, rr)

	// Verify no configuration set was created (transaction rolled back)
	var count int
	err := tdb.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE name = ?", configSet.Name).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify rollback: %v", err)
	}
	if count != 0 {
		t.Error("Expected configuration set creation to be rolled back")
	}

	// Verify no workspace assignments were created
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspace_configuration_sets WHERE workspace_id = ?", workspaceID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace assignment rollback: %v", err)
	}
	if count != 0 {
		t.Error("Expected workspace assignments to be rolled back")
	}
}

// Helper function to create a custom field definition for testing
func createTestCustomField(t *testing.T, tdb *testutils.TestDB, name, fieldType string) int {
	result, err := tdb.Exec(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, created_at, updated_at)
		VALUES (?, ?, 'Test field', 0, datetime('now'), datetime('now'))
	`, name, fieldType)
	if err != nil {
		t.Fatalf("Failed to create test custom field: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

// Helper function to add a field to a screen with specific configuration
func addFieldToScreen(t *testing.T, tdb *testutils.TestDB, screenID, fieldID int, order int, required bool, width string) {
	_, err := tdb.Exec(`
		INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
		VALUES (?, 'custom', ?, ?, ?, ?)
	`, screenID, testutils.IntToString(fieldID), order, required, width)
	if err != nil {
		t.Fatalf("Failed to add field to screen: %v", err)
	}
}

func TestConfigurationSetHandler_DistinctScreens(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")

	// Create 3 different screens for different contexts
	createScreenID := createTestScreen(t, tdb, "Create Screen")
	editScreenID := createTestScreen(t, tdb, "Edit Screen")
	viewScreenID := createTestScreen(t, tdb, "View Screen")

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	configSet := models.ConfigurationSet{
		Name:           "Test Config With Distinct Screens",
		Description:    "Configuration set with distinct screens for each context",
		IsDefault:      false,
		WorkflowID:     &workflowID,
		WorkspaceIDs:   []int{workspaceID},
		CreateScreenID: &createScreenID,
		EditScreenID:   &editScreenID,
		ViewScreenID:   &viewScreenID,
	}

	// Create the configuration set
	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", configSet)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	// Verify all screen IDs are distinct
	if response.CreateScreenID == nil || response.EditScreenID == nil || response.ViewScreenID == nil {
		t.Fatal("Expected all screen IDs to be set")
	}
	if *response.CreateScreenID == *response.EditScreenID ||
		*response.EditScreenID == *response.ViewScreenID ||
		*response.CreateScreenID == *response.ViewScreenID {
		t.Error("Expected all screen IDs to be different")
	}

	// Verify correct screen IDs
	if *response.CreateScreenID != createScreenID {
		t.Errorf("Expected create screen ID %d, got %d", createScreenID, *response.CreateScreenID)
	}
	if *response.EditScreenID != editScreenID {
		t.Errorf("Expected edit screen ID %d, got %d", editScreenID, *response.EditScreenID)
	}
	if *response.ViewScreenID != viewScreenID {
		t.Errorf("Expected view screen ID %d, got %d", viewScreenID, *response.ViewScreenID)
	}

	// Fetch the configuration set and verify screens are correctly returned
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/"+testutils.IntToString(response.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(response.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusOK)

	var fetchedResponse models.ConfigurationSet
	getRR.AssertJSONResponse(&fetchedResponse)

	// Verify screen names are correctly returned
	if fetchedResponse.CreateScreenName != "Create Screen" {
		t.Errorf("Expected create screen name 'Create Screen', got %s", fetchedResponse.CreateScreenName)
	}
	if fetchedResponse.EditScreenName != "Edit Screen" {
		t.Errorf("Expected edit screen name 'Edit Screen', got %s", fetchedResponse.EditScreenName)
	}
	if fetchedResponse.ViewScreenName != "View Screen" {
		t.Errorf("Expected view screen name 'View Screen', got %s", fetchedResponse.ViewScreenName)
	}

	// Verify updating one screen doesn't affect others
	newViewScreenID := createTestScreen(t, tdb, "New View Screen")
	updateConfigSet := models.ConfigurationSet{
		Name:           "Test Config With Distinct Screens",
		Description:    "Updated description",
		WorkflowID:     &workflowID,
		WorkspaceIDs:   []int{workspaceID},
		CreateScreenID: &createScreenID,
		EditScreenID:   &editScreenID,
		ViewScreenID:   &newViewScreenID,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/configuration-sets/"+testutils.IntToString(response.ID), updateConfigSet)
	updateReq.SetPathValue("id", testutils.IntToString(response.ID))
	updateRR := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	updateRR.AssertStatusCode(http.StatusOK)

	var updatedResponse models.ConfigurationSet
	updateRR.AssertJSONResponse(&updatedResponse)

	// Verify only view screen changed, others remain the same
	if *updatedResponse.CreateScreenID != createScreenID {
		t.Errorf("Create screen ID should not change, expected %d, got %d", createScreenID, *updatedResponse.CreateScreenID)
	}
	if *updatedResponse.EditScreenID != editScreenID {
		t.Errorf("Edit screen ID should not change, expected %d, got %d", editScreenID, *updatedResponse.EditScreenID)
	}
	if *updatedResponse.ViewScreenID != newViewScreenID {
		t.Errorf("View screen ID should be updated to %d, got %d", newViewScreenID, *updatedResponse.ViewScreenID)
	}
}

func TestConfigurationSetHandler_SameScreenAllContexts(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")

	// Create a single screen to be used for all contexts
	sharedScreenID := createTestScreen(t, tdb, "Shared Screen")

	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	configSet := models.ConfigurationSet{
		Name:           "Test Config With Same Screen",
		Description:    "Configuration set with same screen for all contexts",
		IsDefault:      false,
		WorkflowID:     &workflowID,
		WorkspaceIDs:   []int{workspaceID},
		CreateScreenID: &sharedScreenID,
		EditScreenID:   &sharedScreenID,
		ViewScreenID:   &sharedScreenID,
	}

	// Create the configuration set
	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", configSet)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	// Verify all screen IDs are the same
	if response.CreateScreenID == nil || response.EditScreenID == nil || response.ViewScreenID == nil {
		t.Fatal("Expected all screen IDs to be set")
	}
	if *response.CreateScreenID != sharedScreenID {
		t.Errorf("Expected create screen ID %d, got %d", sharedScreenID, *response.CreateScreenID)
	}
	if *response.EditScreenID != sharedScreenID {
		t.Errorf("Expected edit screen ID %d, got %d", sharedScreenID, *response.EditScreenID)
	}
	if *response.ViewScreenID != sharedScreenID {
		t.Errorf("Expected view screen ID %d, got %d", sharedScreenID, *response.ViewScreenID)
	}

	// Fetch and verify screen names are consistent
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/"+testutils.IntToString(response.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(response.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusOK)

	var fetchedResponse models.ConfigurationSet
	getRR.AssertJSONResponse(&fetchedResponse)

	if fetchedResponse.CreateScreenName != "Shared Screen" {
		t.Errorf("Expected create screen name 'Shared Screen', got %s", fetchedResponse.CreateScreenName)
	}
	if fetchedResponse.EditScreenName != "Shared Screen" {
		t.Errorf("Expected edit screen name 'Shared Screen', got %s", fetchedResponse.EditScreenName)
	}
	if fetchedResponse.ViewScreenName != "Shared Screen" {
		t.Errorf("Expected view screen name 'Shared Screen', got %s", fetchedResponse.ViewScreenName)
	}

	// Verify database has 3 entries in configuration_set_screens but all point to same screen
	var count int
	err := tdb.QueryRow(`
		SELECT COUNT(*) FROM configuration_set_screens
		WHERE configuration_set_id = ?
	`, response.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query configuration_set_screens: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 screen assignments (one per context), got %d", count)
	}

	// Verify all screen assignments point to the same screen
	var distinctScreens int
	err = tdb.QueryRow(`
		SELECT COUNT(DISTINCT screen_id) FROM configuration_set_screens
		WHERE configuration_set_id = ?
	`, response.ID).Scan(&distinctScreens)
	if err != nil {
		t.Fatalf("Failed to query distinct screens: %v", err)
	}
	if distinctScreens != 1 {
		t.Errorf("Expected 1 distinct screen, got %d", distinctScreens)
	}
}

func TestScreenHandler_CustomFieldsPerScreen(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create 2 screens
	screenA := createTestScreen(t, tdb, "Screen A")
	screenB := createTestScreen(t, tdb, "Screen B")

	// Create 2 custom fields
	field1ID := createTestCustomField(t, tdb, "Field One", "text")
	field2ID := createTestCustomField(t, tdb, "Field Two", "number")

	// Assign field_1 to screen_a only
	addFieldToScreen(t, tdb, screenA, field1ID, 1, false, "full")

	// Assign field_2 to screen_b only
	addFieldToScreen(t, tdb, screenB, field2ID, 1, false, "full")

	screenHandler := NewScreenHandler(tdb.GetDatabase())

	// Get screen_a and verify it only has field_1
	reqA := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(screenA), nil)
	reqA.SetPathValue("id", testutils.IntToString(screenA))
	rrA := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, reqA, nil)

	rrA.AssertStatusCode(http.StatusOK)

	var screenAResponse models.Screen
	rrA.AssertJSONResponse(&screenAResponse)

	// Count custom fields (excluding system fields like title, status)
	customFieldsA := 0
	for _, field := range screenAResponse.Fields {
		if field.FieldType == "custom" {
			customFieldsA++
		}
	}
	if customFieldsA != 1 {
		t.Errorf("Expected screen A to have 1 custom field, got %d", customFieldsA)
	}

	// Verify field_1 is on screen_a
	foundField1 := false
	for _, field := range screenAResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(field1ID) {
			foundField1 = true
			break
		}
	}
	if !foundField1 {
		t.Error("Expected field_1 to be on screen_a")
	}

	// Get screen_b and verify it only has field_2
	reqB := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(screenB), nil)
	reqB.SetPathValue("id", testutils.IntToString(screenB))
	rrB := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, reqB, nil)

	rrB.AssertStatusCode(http.StatusOK)

	var screenBResponse models.Screen
	rrB.AssertJSONResponse(&screenBResponse)

	// Count custom fields
	customFieldsB := 0
	for _, field := range screenBResponse.Fields {
		if field.FieldType == "custom" {
			customFieldsB++
		}
	}
	if customFieldsB != 1 {
		t.Errorf("Expected screen B to have 1 custom field, got %d", customFieldsB)
	}

	// Verify field_2 is on screen_b
	foundField2 := false
	for _, field := range screenBResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(field2ID) {
			foundField2 = true
			break
		}
	}
	if !foundField2 {
		t.Error("Expected field_2 to be on screen_b")
	}

	// Verify fields don't leak between screens
	for _, field := range screenAResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(field2ID) {
			t.Error("Field 2 should not appear on screen A")
		}
	}
	for _, field := range screenBResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(field1ID) {
			t.Error("Field 1 should not appear on screen B")
		}
	}
}

func TestScreenHandler_SharedCustomFieldsDifferentConfig(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create 2 screens
	screenA := createTestScreen(t, tdb, "Screen A")
	screenB := createTestScreen(t, tdb, "Screen B")

	// Create 1 custom field
	sharedFieldID := createTestCustomField(t, tdb, "Shared Field", "text")

	// Add field to both screens with different configurations
	// Screen A: order 1, required, full width
	addFieldToScreen(t, tdb, screenA, sharedFieldID, 1, true, "full")
	// Screen B: order 5, not required, half width
	addFieldToScreen(t, tdb, screenB, sharedFieldID, 5, false, "half")

	screenHandler := NewScreenHandler(tdb.GetDatabase())

	// Get screen_a and verify field config
	reqA := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(screenA), nil)
	reqA.SetPathValue("id", testutils.IntToString(screenA))
	rrA := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, reqA, nil)

	rrA.AssertStatusCode(http.StatusOK)

	var screenAResponse models.Screen
	rrA.AssertJSONResponse(&screenAResponse)

	// Find the shared field on screen A
	var fieldOnA *models.ScreenField
	for i, field := range screenAResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(sharedFieldID) {
			fieldOnA = &screenAResponse.Fields[i]
			break
		}
	}
	if fieldOnA == nil {
		t.Fatal("Shared field not found on screen A")
	}

	// Verify screen A configuration
	if fieldOnA.DisplayOrder != 1 {
		t.Errorf("Expected display order 1 on screen A, got %d", fieldOnA.DisplayOrder)
	}
	if !fieldOnA.IsRequired {
		t.Error("Expected field to be required on screen A")
	}
	if fieldOnA.FieldWidth != "full" {
		t.Errorf("Expected field width 'full' on screen A, got %s", fieldOnA.FieldWidth)
	}

	// Get screen_b and verify field config
	reqB := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(screenB), nil)
	reqB.SetPathValue("id", testutils.IntToString(screenB))
	rrB := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, reqB, nil)

	rrB.AssertStatusCode(http.StatusOK)

	var screenBResponse models.Screen
	rrB.AssertJSONResponse(&screenBResponse)

	// Find the shared field on screen B
	var fieldOnB *models.ScreenField
	for i, field := range screenBResponse.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(sharedFieldID) {
			fieldOnB = &screenBResponse.Fields[i]
			break
		}
	}
	if fieldOnB == nil {
		t.Fatal("Shared field not found on screen B")
	}

	// Verify screen B configuration is different
	if fieldOnB.DisplayOrder != 5 {
		t.Errorf("Expected display order 5 on screen B, got %d", fieldOnB.DisplayOrder)
	}
	if fieldOnB.IsRequired {
		t.Error("Expected field to not be required on screen B")
	}
	if fieldOnB.FieldWidth != "half" {
		t.Errorf("Expected field width 'half' on screen B, got %s", fieldOnB.FieldWidth)
	}

	// Update field on screen A and verify it doesn't affect screen B
	updatedFields := []models.ScreenField{
		{
			ScreenID:        screenA,
			FieldType:       "system",
			FieldIdentifier: "title",
			DisplayOrder:    0,
			IsRequired:      true,
			FieldWidth:      "full",
		},
		{
			ScreenID:        screenA,
			FieldType:       "custom",
			FieldIdentifier: testutils.IntToString(sharedFieldID),
			DisplayOrder:    10,     // Changed from 1
			IsRequired:      false,  // Changed from true
			FieldWidth:      "half", // Changed from full
		},
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/screens/"+testutils.IntToString(screenA)+"/fields", updatedFields)
	updateReq.SetPathValue("id", testutils.IntToString(screenA))
	updateRR := testutils.ExecuteAuthenticatedRequest(t, screenHandler.UpdateFields, updateReq, nil)

	updateRR.AssertStatusCode(http.StatusOK)

	// Re-fetch screen B and verify its configuration is unchanged
	reqB2 := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(screenB), nil)
	reqB2.SetPathValue("id", testutils.IntToString(screenB))
	rrB2 := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, reqB2, nil)

	rrB2.AssertStatusCode(http.StatusOK)

	var screenBResponse2 models.Screen
	rrB2.AssertJSONResponse(&screenBResponse2)

	var fieldOnBAfterUpdate *models.ScreenField
	for i, field := range screenBResponse2.Fields {
		if field.FieldType == "custom" && field.FieldIdentifier == testutils.IntToString(sharedFieldID) {
			fieldOnBAfterUpdate = &screenBResponse2.Fields[i]
			break
		}
	}
	if fieldOnBAfterUpdate == nil {
		t.Fatal("Shared field not found on screen B after update")
	}

	// Screen B should still have its original configuration
	if fieldOnBAfterUpdate.DisplayOrder != 5 {
		t.Errorf("Screen B display order should not change, expected 5, got %d", fieldOnBAfterUpdate.DisplayOrder)
	}
	if fieldOnBAfterUpdate.IsRequired {
		t.Error("Screen B is_required should not change, expected false")
	}
	if fieldOnBAfterUpdate.FieldWidth != "half" {
		t.Errorf("Screen B field_width should not change, expected 'half', got %s", fieldOnBAfterUpdate.FieldWidth)
	}
}

func TestConfigurationSetHandler_ScreensWithCustomFields(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test dependencies
	workspaceID := createTestWorkspace(t, tdb, "Test Workspace", "TEST")
	workflowID := createTestWorkflow(t, tdb, "Test Workflow")

	// Create 3 screens
	createScreen := createTestScreen(t, tdb, "Create Screen")
	editScreen := createTestScreen(t, tdb, "Edit Screen")
	viewScreen := createTestScreen(t, tdb, "View Screen")

	// Create 4 custom fields
	nameField := createTestCustomField(t, tdb, "Name Field", "text")
	descField := createTestCustomField(t, tdb, "Description Field", "textarea")
	dateField := createTestCustomField(t, tdb, "Date Field", "date")
	statusField := createTestCustomField(t, tdb, "Custom Status Field", "select")

	// Assign different fields to different screens
	// Create screen: name (required), desc
	addFieldToScreen(t, tdb, createScreen, nameField, 1, true, "full")
	addFieldToScreen(t, tdb, createScreen, descField, 2, false, "full")

	// Edit screen: name, desc, date, status (all fields)
	addFieldToScreen(t, tdb, editScreen, nameField, 1, true, "half")
	addFieldToScreen(t, tdb, editScreen, descField, 2, false, "full")
	addFieldToScreen(t, tdb, editScreen, dateField, 3, false, "half")
	addFieldToScreen(t, tdb, editScreen, statusField, 4, false, "half")

	// View screen: only name and date (read-only view)
	addFieldToScreen(t, tdb, viewScreen, nameField, 1, false, "full")
	addFieldToScreen(t, tdb, viewScreen, dateField, 2, false, "full")

	// Create configuration set with these screens
	mockNotificationService := mocks.CreateMockNotificationService()
	handler := NewConfigurationSetHandler(tdb.GetDatabase(), mockNotificationService)

	configSet := models.ConfigurationSet{
		Name:           "Test Config With Custom Fields",
		Description:    "End-to-end test for config set with screens and custom fields",
		IsDefault:      false,
		WorkflowID:     &workflowID,
		WorkspaceIDs:   []int{workspaceID},
		CreateScreenID: &createScreen,
		EditScreenID:   &editScreen,
		ViewScreenID:   &viewScreen,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets", configSet)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.ConfigurationSet
	rr.AssertJSONResponse(&response)

	// Verify config set has correct screen IDs
	if *response.CreateScreenID != createScreen {
		t.Errorf("Expected create screen ID %d, got %d", createScreen, *response.CreateScreenID)
	}
	if *response.EditScreenID != editScreen {
		t.Errorf("Expected edit screen ID %d, got %d", editScreen, *response.EditScreenID)
	}
	if *response.ViewScreenID != viewScreen {
		t.Errorf("Expected view screen ID %d, got %d", viewScreen, *response.ViewScreenID)
	}

	// Verify each screen has its own custom fields
	screenHandler := NewScreenHandler(tdb.GetDatabase())

	// Check create screen (should have 2 custom fields: name, desc)
	createReq := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(createScreen), nil)
	createReq.SetPathValue("id", testutils.IntToString(createScreen))
	createRR := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, createReq, nil)
	createRR.AssertStatusCode(http.StatusOK)

	var createScreenResponse models.Screen
	createRR.AssertJSONResponse(&createScreenResponse)

	createCustomFields := countCustomFields(createScreenResponse.Fields)
	if createCustomFields != 2 {
		t.Errorf("Create screen should have 2 custom fields, got %d", createCustomFields)
	}

	// Check edit screen (should have 4 custom fields)
	editReq := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(editScreen), nil)
	editReq.SetPathValue("id", testutils.IntToString(editScreen))
	editRR := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, editReq, nil)
	editRR.AssertStatusCode(http.StatusOK)

	var editScreenResponse models.Screen
	editRR.AssertJSONResponse(&editScreenResponse)

	editCustomFields := countCustomFields(editScreenResponse.Fields)
	if editCustomFields != 4 {
		t.Errorf("Edit screen should have 4 custom fields, got %d", editCustomFields)
	}

	// Check view screen (should have 2 custom fields: name, date)
	viewReq := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(viewScreen), nil)
	viewReq.SetPathValue("id", testutils.IntToString(viewScreen))
	viewRR := testutils.ExecuteAuthenticatedRequest(t, screenHandler.Get, viewReq, nil)
	viewRR.AssertStatusCode(http.StatusOK)

	var viewScreenResponse models.Screen
	viewRR.AssertJSONResponse(&viewScreenResponse)

	viewCustomFields := countCustomFields(viewScreenResponse.Fields)
	if viewCustomFields != 2 {
		t.Errorf("View screen should have 2 custom fields, got %d", viewCustomFields)
	}

	// Verify fields don't bleed across screen contexts
	// Check that create screen doesn't have date or status fields
	for _, field := range createScreenResponse.Fields {
		if field.FieldType == "custom" {
			if field.FieldIdentifier == testutils.IntToString(dateField) {
				t.Error("Date field should not appear on create screen")
			}
			if field.FieldIdentifier == testutils.IntToString(statusField) {
				t.Error("Status field should not appear on create screen")
			}
		}
	}

	// Check that view screen doesn't have desc or status fields
	for _, field := range viewScreenResponse.Fields {
		if field.FieldType == "custom" {
			if field.FieldIdentifier == testutils.IntToString(descField) {
				t.Error("Description field should not appear on view screen")
			}
			if field.FieldIdentifier == testutils.IntToString(statusField) {
				t.Error("Status field should not appear on view screen")
			}
		}
	}
}

// Helper function to count custom fields in a slice of ScreenFields
func countCustomFields(fields []models.ScreenField) int {
	count := 0
	for _, field := range fields {
		if field.FieldType == "custom" {
			count++
		}
	}
	return count
}
