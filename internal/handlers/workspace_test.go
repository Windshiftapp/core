//go:build test

package handlers

import (
	"net/http"
	"testing"
	"windshift/internal/handlers/testutils"
	"windshift/internal/models"

)

func TestWorkspaceHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	workspace := models.Workspace{
		Name:        "Test Workspace",
		Key:         "TEST",
		Description: "Test workspace for unit testing",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/workspaces", workspace)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Workspace
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created workspace to have an ID")
	}
	if response.Name != workspace.Name {
		t.Errorf("Expected name %s, got %s", workspace.Name, response.Name)
	}
	if response.Key != workspace.Key {
		t.Errorf("Expected key %s, got %s", workspace.Key, response.Key)
	}
	if response.Description != workspace.Description {
		t.Errorf("Expected description %s, got %s", workspace.Description, response.Description)
	}
	if response.Active != workspace.Active {
		t.Errorf("Expected active %v, got %v", workspace.Active, response.Active)
	}
	if response.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if response.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}

	// Verify workspace was actually inserted into database
	var count int
	err := tdb.QueryRow("SELECT COUNT(*) FROM workspaces WHERE name = ?", workspace.Name).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace creation: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 workspace in database, got %d", count)
	}
}

func TestWorkspaceHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	tests := []struct {
		name        string
		workspace   models.Workspace
		expectedErr string
	}{
		{
			name:        "Missing name",
			workspace:   models.Workspace{Key: "TEST", Description: "Test"},
			expectedErr: "Workspace name is required",
		},
		{
			name:        "Empty name",
			workspace:   models.Workspace{Name: "   ", Key: "TEST", Description: "Test"},
			expectedErr: "Workspace name is required",
		},
		{
			name:        "Missing key",
			workspace:   models.Workspace{Name: "Test", Description: "Test"},
			expectedErr: "Workspace key is required",
		},
		{
			name:        "Empty key",
			workspace:   models.Workspace{Name: "Test", Key: "   ", Description: "Test"},
			expectedErr: "Workspace key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/workspaces", tt.workspace)
			rr := testutils.ExecuteRequest(t, handler.Create, req)

			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

func TestWorkspaceHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test workspace
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES ('Test Workspace', 'TEST', 'Test workspace', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	workspaceID, _ := result.LastInsertId()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/workspaces/"+testutils.IntToString(int(workspaceID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(workspaceID)))
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Workspace
	rr.AssertJSONResponse(&response)

	if response.ID != int(workspaceID) {
		t.Errorf("Expected ID %d, got %d", workspaceID, response.ID)
	}
	if response.Name != "Test Workspace" {
		t.Errorf("Expected name 'Test Workspace', got %s", response.Name)
	}
	if response.Key != "TEST" {
		t.Errorf("Expected key 'TEST', got %s", response.Key)
	}
	if response.Description != "Test workspace" {
		t.Errorf("Expected description 'Test workspace', got %s", response.Description)
	}
	if !response.Active {
		t.Error("Expected workspace to be active")
	}
}

func TestWorkspaceHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/workspaces/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestWorkspaceHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create multiple test workspaces
	workspaces := []struct {
		name string
		key  string
		desc string
	}{
		{"Workspace A", "WSA", "First workspace"},
		{"Workspace B", "WSB", "Second workspace"},
		{"Workspace C", "WSC", "Third workspace"},
	}

	for _, ws := range workspaces {
		_, err := tdb.Exec(`
			INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
			VALUES (?, ?, ?, 1, datetime('now'), datetime('now'))
		`, ws.name, ws.key, ws.desc)
		if err != nil {
			t.Fatalf("Failed to create workspace %s: %v", ws.name, err)
		}
	}

	handler := NewWorkspaceHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/workspaces", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Workspace
	rr.AssertJSONResponse(&response)

	if len(response) != len(workspaces) {
		t.Errorf("Expected %d workspaces, got %d", len(workspaces), len(response))
	}

	// Verify workspaces are ordered by name
	expectedOrder := []string{"Workspace A", "Workspace B", "Workspace C"}
	for i, ws := range response {
		if ws.Name != expectedOrder[i] {
			t.Errorf("Expected workspace at position %d to be %s, got %s", i, expectedOrder[i], ws.Name)
		}
	}
}

func TestWorkspaceHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test workspace
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES ('Original Name', 'ORIG', 'Original description', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	workspaceID, _ := result.LastInsertId()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	updatedWorkspace := models.Workspace{
		Name:        "Updated Name",
		Description: "Updated description",
		Active:      false,
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/workspaces/"+testutils.IntToString(int(workspaceID)), updatedWorkspace)
	req.SetPathValue("id", testutils.IntToString(int(workspaceID)))
	rr := testutils.ExecuteRequest(t, handler.Update, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Workspace
	rr.AssertJSONResponse(&response)

	if response.Name != updatedWorkspace.Name {
		t.Errorf("Expected name %s, got %s", updatedWorkspace.Name, response.Name)
	}
	if response.Description != updatedWorkspace.Description {
		t.Errorf("Expected description %s, got %s", updatedWorkspace.Description, response.Description)
	}
	if response.Active != updatedWorkspace.Active {
		t.Errorf("Expected active %v, got %v", updatedWorkspace.Active, response.Active)
	}
	if response.Key != "ORIG" {
		t.Errorf("Expected key to remain unchanged as 'ORIG', got %s", response.Key)
	}

	// Verify database was updated
	var name, description string
	var active bool
	err = tdb.QueryRow("SELECT name, description, active FROM workspaces WHERE id = ?", workspaceID).Scan(&name, &description, &active)
	if err != nil {
		t.Fatalf("Failed to verify workspace update: %v", err)
	}
	if name != updatedWorkspace.Name || description != updatedWorkspace.Description || active != updatedWorkspace.Active {
		t.Error("Database was not updated correctly")
	}
}

func TestWorkspaceHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test workspace
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES ('Delete Me', 'DEL', 'To be deleted', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	workspaceID, _ := result.LastInsertId()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/workspaces/"+testutils.IntToString(int(workspaceID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(workspaceID)))
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify workspace was deleted
	var count int
	err = tdb.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", workspaceID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify workspace deletion: %v", err)
	}
	if count != 0 {
		t.Error("Workspace was not deleted from database")
	}
}

func TestWorkspaceHandler_GetProjects_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test workspace
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES ('Test Workspace', 'TEST', 'Test workspace', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	workspaceID, _ := result.LastInsertId()

	// Create test projects
	projects := []string{"Project A", "Project B"}
	for _, projectName := range projects {
		_, err := tdb.Exec(`
			INSERT INTO projects (workspace_id, name, description, active, created_at, updated_at)
			VALUES (?, ?, 'Test project', 1, datetime('now'), datetime('now'))
		`, workspaceID, projectName)
		if err != nil {
			t.Fatalf("Failed to create test project %s: %v", projectName, err)
		}
	}

	handler := NewWorkspaceHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/workspaces/"+testutils.IntToString(int(workspaceID))+"/projects", nil)
	req.SetPathValue("id", testutils.IntToString(int(workspaceID)))
	rr := testutils.ExecuteRequest(t, handler.GetProjects, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Project
	rr.AssertJSONResponse(&response)

	if len(response) != len(projects) {
		t.Errorf("Expected %d projects, got %d", len(projects), len(response))
	}

	for i, project := range response {
		if project.WorkspaceID == nil || *project.WorkspaceID != int(workspaceID) {
			t.Errorf("Expected project workspace_id %d, got %v", workspaceID, project.WorkspaceID)
		}
		if project.WorkspaceName != "Test Workspace" {
			t.Errorf("Expected project workspace_name 'Test Workspace', got %s", project.WorkspaceName)
		}
		expectedName := projects[i]
		if project.Name != expectedName {
			t.Errorf("Expected project name %s, got %s", expectedName, project.Name)
		}
	}
}

func TestWorkspaceHandler_InvalidID_Scenarios(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkspaceHandler(tdb.DB.DB)

	tests := []struct {
		name     string
		endpoint string
		method   string
	}{
		{"Get invalid ID", "/api/workspaces/invalid", "GET"},
		{"Update invalid ID", "/api/workspaces/invalid", "PUT"},
		{"Delete invalid ID", "/api/workspaces/invalid", "DELETE"},
		{"GetProjects invalid ID", "/api/workspaces/invalid/projects", "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			switch tt.method {
			case "GET":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, nil)
			case "PUT":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, models.Workspace{Name: "Test"})
			case "DELETE":
				req = testutils.CreateJSONRequest(t, tt.method, tt.endpoint, nil)
			}

			// Set invalid ID in mux vars
			req.SetPathValue("id", "invalid")

			var rr *testutils.ResponseRecorder
			switch {
			case tt.endpoint == "/api/workspaces/invalid":
				switch tt.method {
				case "GET":
					rr = testutils.ExecuteRequest(t, handler.Get, req)
				case "PUT":
					rr = testutils.ExecuteRequest(t, handler.Update, req)
				case "DELETE":
					rr = testutils.ExecuteRequest(t, handler.Delete, req)
				}
			case tt.endpoint == "/api/workspaces/invalid/projects":
				rr = testutils.ExecuteRequest(t, handler.GetProjects, req)
			}

			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}

func TestWorkspaceHandler_DuplicateKey_Error(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create first workspace
	_, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active, created_at, updated_at)
		VALUES ('First Workspace', 'DUPLICATE', 'First workspace', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create first workspace: %v", err)
	}

	handler := NewWorkspaceHandler(tdb.DB.DB)

	// Try to create workspace with duplicate key
	duplicateWorkspace := models.Workspace{
		Name:        "Second Workspace",
		Key:         "DUPLICATE",
		Description: "Should fail due to duplicate key",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/workspaces", duplicateWorkspace)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusInternalServerError)
}