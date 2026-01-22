//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestProjectHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID
	project := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Test Project",
		Description: "Test project description",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Project
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created project to have an ID")
	}
	if response.Name != project.Name {
		t.Errorf("Expected name %s, got %s", project.Name, response.Name)
	}
	if response.Description != project.Description {
		t.Errorf("Expected description %s, got %s", project.Description, response.Description)
	}
	if !response.Active {
		t.Error("Expected project to be active")
	}
}

func TestProjectHandler_Create_WithoutWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	// Create a project without a workspace
	project := models.Project{
		Name:        "Global Project",
		Description: "A project without workspace",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Project
	rr.AssertJSONResponse(&response)

	if response.WorkspaceID != nil {
		t.Error("Expected project without workspace ID")
	}
}

func TestProjectHandler_Create_InvalidWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	invalidWorkspace := 99999
	project := models.Project{
		WorkspaceID: &invalidWorkspace,
		Name:        "Test Project",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Workspace not found") {
		t.Errorf("Expected 'Workspace not found' error, got %s", rr.Body.String())
	}
}

func TestProjectHandler_Create_Unauthenticated(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	project := models.Project{
		Name:   "Test Project",
		Active: true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestProjectHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID

	// Create test projects
	projects := []models.Project{
		{WorkspaceID: &workspaceID, Name: "Project 1", Active: true},
		{WorkspaceID: &workspaceID, Name: "Project 2", Active: true},
	}

	for _, p := range projects {
		req := testutils.CreateJSONRequest(t, "POST", "/api/projects", p)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/projects", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Project
	rr.AssertJSONResponse(&response)

	if len(response) < 2 {
		t.Errorf("Expected at least 2 projects, got %d", len(response))
	}
}

func TestProjectHandler_GetAll_FilterByWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID

	// Create project for workspace
	project := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Workspace Project",
		Active:      true,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	// Get projects filtered by workspace
	req := testutils.CreateJSONRequest(t, "GET", "/api/projects?workspace_id="+testutils.IntToString(workspaceID), nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Project
	rr.AssertJSONResponse(&response)

	for _, p := range response {
		if p.WorkspaceID == nil || *p.WorkspaceID != workspaceID {
			t.Errorf("Expected only workspace %d projects, got workspace %v", workspaceID, p.WorkspaceID)
		}
	}
}

func TestProjectHandler_GetAll_Unauthenticated(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	req := testutils.CreateJSONRequest(t, "GET", "/api/projects", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestProjectHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID
	project := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Test Get Project",
		Description: "Test description",
		Active:      true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Project
	createRR.AssertJSONResponse(&created)

	// Get the project
	req := testutils.CreateJSONRequest(t, "GET", "/api/projects/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Project
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != project.Name {
		t.Errorf("Expected name %s, got %s", project.Name, response.Name)
	}
}

func TestProjectHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	req := testutils.CreateJSONRequest(t, "GET", "/api/projects/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "Project not found") {
		t.Errorf("Expected 'Project not found', got %s", rr.Body.String())
	}
}

func TestProjectHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)

	// Create the project_milestone_categories table if it doesn't exist (handler requires it)
	_, err := tdb.Exec(`
		CREATE TABLE IF NOT EXISTS project_milestone_categories (
			project_id INTEGER NOT NULL,
			category_id INTEGER NOT NULL,
			PRIMARY KEY (project_id, category_id)
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create project_milestone_categories table: %v", err)
	}

	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID
	project := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Original Name",
		Description: "Original description",
		Active:      true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Project
	createRR.AssertJSONResponse(&created)

	// Update the project
	updateData := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Updated Name",
		Description: "Updated description",
		Active:      false,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/projects/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Project
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", response.Name)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", response.Description)
	}
	if response.Active {
		t.Error("Expected project to be inactive")
	}
}

func TestProjectHandler_Update_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID
	updateData := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "Updated Name",
		Active:      true,
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/projects/99999", updateData)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestProjectHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	workspaceID := data.WorkspaceID
	project := models.Project{
		WorkspaceID: &workspaceID,
		Name:        "To Delete",
		Active:      true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/projects", project)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Project
	createRR.AssertJSONResponse(&created)

	// Delete the project
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/projects/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify project is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/projects/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestProjectHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/projects/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestProjectHandler_Delete_Unauthenticated(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Pass nil for permission service to bypass permission checks in tests
	handler := NewProjectHandler(tdb.GetDatabase(), nil)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/projects/1", nil)
	req.SetPathValue("id", "1")
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}
