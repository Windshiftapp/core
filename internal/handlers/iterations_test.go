//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/testutils"
)

// createIterationTestServices creates the permission service needed for iteration handler tests
// and grants the test user global iteration management permissions.
// It also ensures the test user exists by seeding basic test data if not already seeded.
func createIterationTestServices(t *testing.T, db testutils.TestDB) *services.PermissionService {
	t.Helper()

	// Check if test user already exists, if not seed the test data
	var userCount int
	err := db.GetDatabase().QueryRow("SELECT COUNT(*) FROM users WHERE id = 1").Scan(&userCount)
	if err != nil || userCount == 0 {
		// Seed basic test data to ensure user ID 1 exists
		db.SeedTestData(t)
	}

	// Create permission service with test-friendly config
	permConfig := services.DefaultPermissionCacheConfig()
	permConfig.WarmupOnStartup = false // Don't warm up during tests
	permConfig.TTL = 1 * time.Minute   // Short TTL for tests

	permService, err := services.NewPermissionService(db.GetDatabase(), permConfig)
	if err != nil {
		t.Fatalf("Failed to create permission service: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		permService.Close()
	})

	// Grant the test user (ID 1) global iteration.manage permission
	// First get the permission ID for iteration.manage
	var permID int
	err = db.GetDatabase().QueryRow("SELECT id FROM permissions WHERE permission_key = ?", models.PermissionIterationManage).Scan(&permID)
	if err != nil {
		t.Fatalf("Failed to get iteration.manage permission ID: %v", err)
	}

	// Grant global permission to user ID 1 (the default test user)
	_, err = db.GetDatabase().ExecWrite(`
		INSERT OR IGNORE INTO user_global_permissions (user_id, permission_id, granted_at)
		VALUES (1, ?, datetime('now'))
	`, permID)
	if err != nil {
		t.Fatalf("Failed to grant global iteration permission: %v", err)
	}

	// Invalidate the permission cache for user 1
	permService.InvalidateUserCache(1)

	return permService
}

func TestIterationHandler_Create_Success_Global(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	iteration := models.Iteration{
		Name:        "Sprint 1",
		Description: "First sprint iteration",
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-14",
		Status:      "planned",
		IsGlobal:    true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Iteration
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created iteration to have an ID")
	}
	if response.Name != iteration.Name {
		t.Errorf("Expected name %s, got %s", iteration.Name, response.Name)
	}
	if response.Status != iteration.Status {
		t.Errorf("Expected status %s, got %s", iteration.Status, response.Status)
	}
	if !response.IsGlobal {
		t.Error("Expected iteration to be global")
	}
	if response.WorkspaceID != nil {
		t.Error("Expected global iteration to have no workspace ID")
	}
}

func TestIterationHandler_Create_Success_Local(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	workspaceID := data.WorkspaceID
	iteration := models.Iteration{
		Name:        "Local Sprint",
		StartDate:   "2025-02-01",
		EndDate:     "2025-02-14",
		Status:      "active",
		IsGlobal:    false,
		WorkspaceID: &workspaceID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Iteration
	rr.AssertJSONResponse(&response)

	if response.IsGlobal {
		t.Error("Expected iteration to be local")
	}
	if response.WorkspaceID == nil || *response.WorkspaceID != workspaceID {
		t.Errorf("Expected workspace ID %d, got %v", workspaceID, response.WorkspaceID)
	}
}

func TestIterationHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	workspaceID := data.WorkspaceID

	tests := []struct {
		name        string
		iteration   models.Iteration
		expectedErr string
	}{
		{
			name:        "Missing name",
			iteration:   models.Iteration{StartDate: "2025-01-01", EndDate: "2025-01-14", Status: "planned", IsGlobal: true},
			expectedErr: "Iteration name is required",
		},
		{
			name:        "Empty name",
			iteration:   models.Iteration{Name: "   ", StartDate: "2025-01-01", EndDate: "2025-01-14", Status: "planned", IsGlobal: true},
			expectedErr: "Iteration name is required",
		},
		{
			name:        "Missing start date",
			iteration:   models.Iteration{Name: "Test", EndDate: "2025-01-14", Status: "planned", IsGlobal: true},
			expectedErr: "Start date is required",
		},
		{
			name:        "Missing end date",
			iteration:   models.Iteration{Name: "Test", StartDate: "2025-01-01", Status: "planned", IsGlobal: true},
			expectedErr: "End date is required",
		},
		{
			name:        "Global with workspace ID",
			iteration:   models.Iteration{Name: "Test", StartDate: "2025-01-01", EndDate: "2025-01-14", Status: "planned", IsGlobal: true, WorkspaceID: &workspaceID},
			expectedErr: "Global iterations cannot have a workspace_id",
		},
		{
			name:        "Local without workspace ID",
			iteration:   models.Iteration{Name: "Test", StartDate: "2025-01-01", EndDate: "2025-01-14", Status: "planned", IsGlobal: false},
			expectedErr: "Local iterations must have a workspace_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", tt.iteration)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestIterationHandler_Create_DefaultStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	iteration := models.Iteration{
		Name:      "Default Status Test",
		StartDate: "2025-01-01",
		EndDate:   "2025-01-14",
		Status:    "invalid-status",
		IsGlobal:  true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Iteration
	rr.AssertJSONResponse(&response)

	if response.Status != "planned" {
		t.Errorf("Expected default status 'planned', got %s", response.Status)
	}
}

func TestIterationHandler_Create_InvalidWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	invalidWorkspace := 99999
	iteration := models.Iteration{
		Name:        "Test",
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-14",
		Status:      "planned",
		IsGlobal:    false,
		WorkspaceID: &invalidWorkspace,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	// With permission checks, we now return 403 Forbidden for workspaces the user doesn't have access to
	// This is more secure as it doesn't reveal whether the workspace exists or not
	rr.AssertStatusCode(http.StatusForbidden)
}

func TestIterationHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create global iterations
	iterations := []models.Iteration{
		{Name: "Sprint 1", StartDate: "2025-01-01", EndDate: "2025-01-14", Status: "completed", IsGlobal: true},
		{Name: "Sprint 2", StartDate: "2025-01-15", EndDate: "2025-01-28", Status: "active", IsGlobal: true},
	}

	for _, iter := range iterations {
		req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iter)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Iteration
	rr.AssertJSONResponse(&response)

	if len(response) < 2 {
		t.Errorf("Expected at least 2 iterations, got %d", len(response))
	}
}

func TestIterationHandler_GetAll_FilterByWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	workspaceID := data.WorkspaceID

	// Create a local iteration for the workspace
	localIteration := models.Iteration{
		Name:        "Local Iteration",
		StartDate:   "2025-03-01",
		EndDate:     "2025-03-14",
		Status:      "planned",
		IsGlobal:    false,
		WorkspaceID: &workspaceID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", localIteration)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	// Create a global iteration
	globalIteration := models.Iteration{
		Name:      "Global Iteration",
		StartDate: "2025-04-01",
		EndDate:   "2025-04-14",
		Status:    "planned",
		IsGlobal:  true,
	}
	createReq2 := testutils.CreateJSONRequest(t, "POST", "/api/iterations", globalIteration)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq2, nil)

	// Get iterations for workspace (should include both local and global)
	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations?workspace_id="+testutils.IntToString(workspaceID), nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Iteration
	rr.AssertJSONResponse(&response)

	foundLocal := false
	foundGlobal := false
	for _, iter := range response {
		if iter.Name == "Local Iteration" {
			foundLocal = true
		}
		if iter.Name == "Global Iteration" {
			foundGlobal = true
		}
	}

	if !foundLocal {
		t.Error("Expected to find local iteration in workspace results")
	}
	if !foundGlobal {
		t.Error("Expected to find global iteration in workspace results")
	}
}

func TestIterationHandler_GetAll_FilterByStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create iterations with different statuses
	iterations := []models.Iteration{
		{Name: "Planned Iteration", StartDate: "2025-05-01", EndDate: "2025-05-14", Status: "planned", IsGlobal: true},
		{Name: "Active Iteration", StartDate: "2025-06-01", EndDate: "2025-06-14", Status: "active", IsGlobal: true},
		{Name: "Completed Iteration", StartDate: "2025-07-01", EndDate: "2025-07-14", Status: "completed", IsGlobal: true},
	}

	for _, iter := range iterations {
		req := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iter)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	// Get only active iterations
	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations?status=active", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Iteration
	rr.AssertJSONResponse(&response)

	for _, iter := range response {
		if iter.Status != "active" {
			t.Errorf("Expected only active iterations, got status %s", iter.Status)
		}
	}
}

func TestIterationHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	iteration := models.Iteration{
		Name:        "Test Get Iteration",
		Description: "Test description",
		StartDate:   "2025-08-01",
		EndDate:     "2025-08-14",
		Status:      "active",
		IsGlobal:    true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Iteration
	createRR.AssertJSONResponse(&created)

	// Get the iteration
	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Iteration
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != iteration.Name {
		t.Errorf("Expected name %s, got %s", iteration.Name, response.Name)
	}
	if response.Description != iteration.Description {
		t.Errorf("Expected description %s, got %s", iteration.Description, response.Description)
	}
}

func TestIterationHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "iteration not found") {
		t.Errorf("Expected 'iteration not found', got %s", rr.Body.String())
	}
}

func TestIterationHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create an iteration
	iteration := models.Iteration{
		Name:        "Original Name",
		Description: "Original description",
		StartDate:   "2025-09-01",
		EndDate:     "2025-09-14",
		Status:      "planned",
		IsGlobal:    true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Iteration
	createRR.AssertJSONResponse(&created)

	// Update the iteration
	updateData := models.Iteration{
		Name:        "Updated Name",
		Description: "Updated description",
		StartDate:   "2025-09-05",
		EndDate:     "2025-09-20",
		Status:      "active",
		IsGlobal:    true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/iterations/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Iteration
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", response.Name)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", response.Description)
	}
	if response.Status != "active" {
		t.Errorf("Expected status 'active', got %s", response.Status)
	}
}

func TestIterationHandler_Update_InvalidStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create an iteration
	iteration := models.Iteration{
		Name:      "Test",
		StartDate: "2025-10-01",
		EndDate:   "2025-10-14",
		Status:    "planned",
		IsGlobal:  true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Iteration
	createRR.AssertJSONResponse(&created)

	// Try to update with invalid status
	updateData := models.Iteration{
		Name:      "Test",
		StartDate: "2025-10-01",
		EndDate:   "2025-10-14",
		Status:    "invalid-status",
		IsGlobal:  true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/iterations/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Invalid status") {
		t.Errorf("Expected 'Invalid status' error, got %s", rr.Body.String())
	}
}

func TestIterationHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create an iteration
	iteration := models.Iteration{
		Name:      "To Delete",
		StartDate: "2025-11-01",
		EndDate:   "2025-11-14",
		Status:    "planned",
		IsGlobal:  true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Iteration
	createRR.AssertJSONResponse(&created)

	// Delete the iteration
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/iterations/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify iteration is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/iterations/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestIterationHandler_GetProgress_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	// Create an iteration
	iteration := models.Iteration{
		Name:      "Progress Test",
		StartDate: "2025-12-01",
		EndDate:   "2025-12-14",
		Status:    "active",
		IsGlobal:  true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/iterations", iteration)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Iteration
	createRR.AssertJSONResponse(&created)

	// Get progress report
	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations/"+testutils.IntToString(created.ID)+"/progress", nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetProgress, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response services.IterationProgressReport
	rr.AssertJSONResponse(&response)

	if response.IterationID != created.ID {
		t.Errorf("Expected iteration ID %d, got %d", created.ID, response.IterationID)
	}
	if response.IterationName != iteration.Name {
		t.Errorf("Expected iteration name %s, got %s", iteration.Name, response.IterationName)
	}
}

func TestIterationHandler_GetProgress_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService := createIterationTestServices(t, *tdb)
	handler := NewIterationHandler(tdb.GetDatabase(), permService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/iterations/99999/progress", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetProgress, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}
