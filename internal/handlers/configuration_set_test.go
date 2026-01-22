//go:build test

package handlers

import (
	"net/http"
	"testing"
	"windshift/internal/models"
	"windshift/internal/testutils"
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

	mockNotificationService := testutils.CreateMockNotificationService()
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
	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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

	mockNotificationService := testutils.CreateMockNotificationService()
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
	mockNotificationService := testutils.CreateMockNotificationService()
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