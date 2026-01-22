//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestMilestoneHandler_Create_Success_Global(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	targetDate := "2025-12-31"
	milestone := models.Milestone{
		Name:        "Q4 Release",
		Description: "Fourth quarter release milestone",
		TargetDate:  &targetDate,
		Status:      "planning",
		IsGlobal:    true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Milestone
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created milestone to have an ID")
	}
	if response.Name != milestone.Name {
		t.Errorf("Expected name %s, got %s", milestone.Name, response.Name)
	}
	if response.Status != milestone.Status {
		t.Errorf("Expected status %s, got %s", milestone.Status, response.Status)
	}
	if !response.IsGlobal {
		t.Error("Expected milestone to be global")
	}
	if response.WorkspaceID != nil {
		t.Error("Expected global milestone to have no workspace ID")
	}
}

func TestMilestoneHandler_Create_Success_Local(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewMilestoneHandler(tdb.GetDatabase())

	workspaceID := data.WorkspaceID
	milestone := models.Milestone{
		Name:        "Sprint 1",
		Description: "First sprint milestone",
		Status:      "in-progress",
		IsGlobal:    false,
		WorkspaceID: &workspaceID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Milestone
	rr.AssertJSONResponse(&response)

	if response.IsGlobal {
		t.Error("Expected milestone to be local")
	}
	if response.WorkspaceID == nil || *response.WorkspaceID != workspaceID {
		t.Errorf("Expected workspace ID %d, got %v", workspaceID, response.WorkspaceID)
	}
}

func TestMilestoneHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewMilestoneHandler(tdb.GetDatabase())

	workspaceID := data.WorkspaceID

	tests := []struct {
		name        string
		milestone   models.Milestone
		expectedErr string
	}{
		{
			name:        "Missing name",
			milestone:   models.Milestone{Status: "planning", IsGlobal: true},
			expectedErr: "Milestone name is required",
		},
		{
			name:        "Empty name",
			milestone:   models.Milestone{Name: "   ", Status: "planning", IsGlobal: true},
			expectedErr: "Milestone name is required",
		},
		{
			name:        "Global with workspace ID",
			milestone:   models.Milestone{Name: "Test", Status: "planning", IsGlobal: true, WorkspaceID: &workspaceID},
			expectedErr: "Global milestones cannot have a workspace_id",
		},
		{
			name:        "Local without workspace ID",
			milestone:   models.Milestone{Name: "Test", Status: "planning", IsGlobal: false},
			expectedErr: "Local milestones must have a workspace_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", tt.milestone)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestMilestoneHandler_Create_DefaultStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	milestone := models.Milestone{
		Name:     "No Status Specified",
		Status:   "invalid-status",
		IsGlobal: true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Milestone
	rr.AssertJSONResponse(&response)

	if response.Status != "planning" {
		t.Errorf("Expected default status 'planning', got %s", response.Status)
	}
}

func TestMilestoneHandler_Create_InvalidWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	invalidWorkspace := 99999
	milestone := models.Milestone{
		Name:        "Test",
		Status:      "planning",
		IsGlobal:    false,
		WorkspaceID: &invalidWorkspace,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Invalid workspace ID") {
		t.Errorf("Expected 'Invalid workspace ID' error, got %s", rr.Body.String())
	}
}

func TestMilestoneHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create global milestones
	milestones := []models.Milestone{
		{Name: "Milestone 1", Status: "planning", IsGlobal: true},
		{Name: "Milestone 2", Status: "in-progress", IsGlobal: true},
	}

	for _, m := range milestones {
		req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", m)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Milestone
	rr.AssertJSONResponse(&response)

	if len(response) < 2 {
		t.Errorf("Expected at least 2 milestones, got %d", len(response))
	}
}

func TestMilestoneHandler_GetAll_FilterByWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewMilestoneHandler(tdb.GetDatabase())

	workspaceID := data.WorkspaceID

	// Create a local milestone for the workspace
	localMilestone := models.Milestone{
		Name:        "Local Milestone",
		Status:      "planning",
		IsGlobal:    false,
		WorkspaceID: &workspaceID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", localMilestone)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	// Create a global milestone
	globalMilestone := models.Milestone{
		Name:     "Global Milestone",
		Status:   "planning",
		IsGlobal: true,
	}
	createReq2 := testutils.CreateJSONRequest(t, "POST", "/api/milestones", globalMilestone)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq2, nil)

	// Get milestones for workspace (should include both local and global)
	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones?workspace_id="+testutils.IntToString(workspaceID), nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Milestone
	rr.AssertJSONResponse(&response)

	foundLocal := false
	foundGlobal := false
	for _, m := range response {
		if m.Name == "Local Milestone" {
			foundLocal = true
		}
		if m.Name == "Global Milestone" {
			foundGlobal = true
		}
	}

	if !foundLocal {
		t.Error("Expected to find local milestone in workspace results")
	}
	if !foundGlobal {
		t.Error("Expected to find global milestone in workspace results")
	}
}

func TestMilestoneHandler_GetAll_FilterByStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create milestones with different statuses
	milestones := []models.Milestone{
		{Name: "Planning Milestone", Status: "planning", IsGlobal: true},
		{Name: "In Progress Milestone", Status: "in-progress", IsGlobal: true},
		{Name: "Completed Milestone", Status: "completed", IsGlobal: true},
	}

	for _, m := range milestones {
		req := testutils.CreateJSONRequest(t, "POST", "/api/milestones", m)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	// Get only in-progress milestones
	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones?status=in-progress", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Milestone
	rr.AssertJSONResponse(&response)

	for _, m := range response {
		if m.Status != "in-progress" {
			t.Errorf("Expected only in-progress milestones, got status %s", m.Status)
		}
	}
}

func TestMilestoneHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	targetDate := "2025-06-30"
	milestone := models.Milestone{
		Name:        "Test Get Milestone",
		Description: "Test description",
		TargetDate:  &targetDate,
		Status:      "in-progress",
		IsGlobal:    true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Milestone
	createRR.AssertJSONResponse(&created)

	// Get the milestone
	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Milestone
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != milestone.Name {
		t.Errorf("Expected name %s, got %s", milestone.Name, response.Name)
	}
	if response.Description != milestone.Description {
		t.Errorf("Expected description %s, got %s", milestone.Description, response.Description)
	}
}

func TestMilestoneHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "Milestone not found") {
		t.Errorf("Expected 'Milestone not found', got %s", rr.Body.String())
	}
}

func TestMilestoneHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create a milestone
	milestone := models.Milestone{
		Name:        "Original Name",
		Description: "Original description",
		Status:      "planning",
		IsGlobal:    true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Milestone
	createRR.AssertJSONResponse(&created)

	// Update the milestone
	targetDate := "2025-09-30"
	updateData := models.Milestone{
		Name:        "Updated Name",
		Description: "Updated description",
		TargetDate:  &targetDate,
		Status:      "in-progress",
		IsGlobal:    true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/milestones/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Milestone
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", response.Name)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", response.Description)
	}
	if response.Status != "in-progress" {
		t.Errorf("Expected status 'in-progress', got %s", response.Status)
	}
}

func TestMilestoneHandler_Update_InvalidStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create a milestone
	milestone := models.Milestone{
		Name:     "Test",
		Status:   "planning",
		IsGlobal: true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Milestone
	createRR.AssertJSONResponse(&created)

	// Try to update with invalid status
	updateData := models.Milestone{
		Name:     "Test",
		Status:   "invalid-status",
		IsGlobal: true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/milestones/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Invalid status") {
		t.Errorf("Expected 'Invalid status' error, got %s", rr.Body.String())
	}
}

func TestMilestoneHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create a milestone
	milestone := models.Milestone{
		Name:     "To Delete",
		Status:   "planning",
		IsGlobal: true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Milestone
	createRR.AssertJSONResponse(&created)

	// Delete the milestone
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/milestones/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify milestone is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/milestones/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestMilestoneHandler_GetProgress_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	// Create a milestone
	milestone := models.Milestone{
		Name:     "Progress Test",
		Status:   "in-progress",
		IsGlobal: true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/milestones", milestone)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Milestone
	createRR.AssertJSONResponse(&created)

	// Get progress report
	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones/"+testutils.IntToString(created.ID)+"/progress", nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetProgress, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response MilestoneProgressReport
	rr.AssertJSONResponse(&response)

	if response.MilestoneID != created.ID {
		t.Errorf("Expected milestone ID %d, got %d", created.ID, response.MilestoneID)
	}
	if response.MilestoneName != milestone.Name {
		t.Errorf("Expected milestone name %s, got %s", milestone.Name, response.MilestoneName)
	}
}

func TestMilestoneHandler_GetProgress_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewMilestoneHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/milestones/99999/progress", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetProgress, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}
