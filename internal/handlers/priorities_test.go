//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestPriorityHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	priority := models.Priority{
		Name:        "Super Critical",
		Description: "Super critical priority items",
		Icon:        "flame",
		Color:       "#ff0000",
		SortOrder:   1,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Priority
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created priority to have an ID")
	}
	if response.Name != priority.Name {
		t.Errorf("Expected name %s, got %s", priority.Name, response.Name)
	}
	if response.Icon != priority.Icon {
		t.Errorf("Expected icon %s, got %s", priority.Icon, response.Icon)
	}
	if response.Color != priority.Color {
		t.Errorf("Expected color %s, got %s", priority.Color, response.Color)
	}
}

func TestPriorityHandler_Create_ValidationError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	tests := []struct {
		name        string
		priority    models.Priority
		expectedErr string
	}{
		{
			name:        "Missing name",
			priority:    models.Priority{Description: "Description only"},
			expectedErr: "Priority name is required",
		},
		{
			name:        "Empty name",
			priority:    models.Priority{Name: "   ", Description: "Description"},
			expectedErr: "Priority name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/priorities", tt.priority)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestPriorityHandler_Create_DuplicateName(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	priority := models.Priority{
		Name:  "Duplicate Test",
		Color: "#ff0000",
	}

	// Create first priority
	req1 := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	rr1 := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req1, nil)
	rr1.AssertStatusCode(http.StatusCreated)

	// Try to create second with same name
	req2 := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req2, nil)

	rr2.AssertStatusCode(http.StatusConflict)
	if !strings.Contains(rr2.Body.String(), "already exists") {
		t.Errorf("Expected 'already exists' error, got %s", rr2.Body.String())
	}
}

func TestPriorityHandler_Create_SetDefault(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create first priority as default
	priority1 := models.Priority{
		Name:      "First Default",
		IsDefault: true,
	}
	req1 := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority1)
	rr1 := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req1, nil)
	rr1.AssertStatusCode(http.StatusCreated)

	var created1 models.Priority
	rr1.AssertJSONResponse(&created1)

	// Create second priority as default
	priority2 := models.Priority{
		Name:      "Second Default",
		IsDefault: true,
	}
	req2 := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority2)
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req2, nil)
	rr2.AssertStatusCode(http.StatusCreated)

	var created2 models.Priority
	rr2.AssertJSONResponse(&created2)

	// Verify the second one is the only default now
	if !created2.IsDefault {
		t.Error("Expected second priority to be default")
	}

	// Verify first one is no longer default
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/priorities/"+testutils.IntToString(created1.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created1.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	var updated1 models.Priority
	getRR.AssertJSONResponse(&updated1)

	if updated1.IsDefault {
		t.Error("Expected first priority to no longer be default")
	}
}

func TestPriorityHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create test priorities
	priorities := []models.Priority{
		{Name: "Low", SortOrder: 3},
		{Name: "Medium", SortOrder: 2},
		{Name: "High", SortOrder: 1},
	}

	for _, p := range priorities {
		req := testutils.CreateJSONRequest(t, "POST", "/api/priorities", p)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/priorities", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Priority
	rr.AssertJSONResponse(&response)

	// Should include at least our 3 created types (may also have default priorities)
	if len(response) < 3 {
		t.Errorf("Expected at least 3 priorities, got %d", len(response))
	}
}

func TestPriorityHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create test priority
	priority := models.Priority{
		Name:        "Test Get Priority",
		Description: "Test description",
		Icon:        "star",
		Color:       "#00ff00",
		SortOrder:   5,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Priority
	createRR.AssertJSONResponse(&created)

	// Get the priority
	req := testutils.CreateJSONRequest(t, "GET", "/api/priorities/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Priority
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != priority.Name {
		t.Errorf("Expected name %s, got %s", priority.Name, response.Name)
	}
	if response.Description != priority.Description {
		t.Errorf("Expected description %s, got %s", priority.Description, response.Description)
	}
}

func TestPriorityHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/priorities/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestPriorityHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create test priority
	priority := models.Priority{
		Name:        "Original Priority",
		Description: "Original description",
		Icon:        "circle",
		Color:       "#111111",
		SortOrder:   1,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Priority
	createRR.AssertJSONResponse(&created)

	// Update the priority
	updateData := models.Priority{
		Name:        "Updated Priority",
		Description: "Updated description",
		Icon:        "flag",
		Color:       "#222222",
		SortOrder:   2,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/priorities/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Priority
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Priority" {
		t.Errorf("Expected name 'Updated Priority', got %s", response.Name)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", response.Description)
	}
	if response.Icon != "flag" {
		t.Errorf("Expected icon 'flag', got %s", response.Icon)
	}
	if response.Color != "#222222" {
		t.Errorf("Expected color '#222222', got %s", response.Color)
	}
}

func TestPriorityHandler_Update_ValidationError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create test priority
	priority := models.Priority{
		Name:  "Test Priority",
		Color: "#333333",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Priority
	createRR.AssertJSONResponse(&created)

	// Try to update with missing name
	updateData := models.Priority{
		Name:  "",
		Color: "#444444",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/priorities/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestPriorityHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create test priority
	priority := models.Priority{
		Name:  "To Delete",
		Color: "#555555",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Priority
	createRR.AssertJSONResponse(&created)

	// Delete the priority
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/priorities/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify priority is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/priorities/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestPriorityHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewPriorityHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/priorities/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestPriorityHandler_Delete_InUse(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewPriorityHandler(tdb.GetDatabase())

	// Create a new priority and use it in an item
	priority := models.Priority{
		Name:  "In Use Priority",
		Color: "#666666",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/priorities", priority)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Priority
	createRR.AssertJSONResponse(&created)

	// Create an item using this priority
	_, err := tdb.Exec(`
		INSERT INTO items (workspace_id, title, status_id, priority_id, created_at, updated_at, workspace_item_number)
		VALUES (?, 'Test Item', ?, ?, datetime('now'), datetime('now'), 1)
	`, data.WorkspaceID, data.StatusID, created.ID)
	if err != nil {
		t.Fatalf("Failed to create test item: %v", err)
	}

	// Try to delete the priority
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/priorities/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusConflict)
	if !strings.Contains(rr.Body.String(), "Cannot delete priority") {
		t.Errorf("Expected 'Cannot delete priority' error, got %s", rr.Body.String())
	}
}
