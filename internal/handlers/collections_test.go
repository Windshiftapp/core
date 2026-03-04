//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestCollectionHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	collection := models.Collection{
		Name:        "My Collection",
		Description: "Test collection description",
		QLQuery:     "status = 'open'",
		IsPublic:    false,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Collection
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created collection to have an ID")
	}
	if response.Name != collection.Name {
		t.Errorf("Expected name %s, got %s", collection.Name, response.Name)
	}
	if response.Description != collection.Description {
		t.Errorf("Expected description %s, got %s", collection.Description, response.Description)
	}
	if response.CreatedBy == nil {
		t.Error("Expected created_by to be set")
	}
}

func TestCollectionHandler_Create_WithWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	workspaceID := data.WorkspaceID
	collection := models.Collection{
		Name:        "Workspace Collection",
		WorkspaceID: &workspaceID,
		IsPublic:    true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Collection
	rr.AssertJSONResponse(&response)

	if response.WorkspaceID == nil || *response.WorkspaceID != workspaceID {
		t.Errorf("Expected workspace ID %d, got %v", workspaceID, response.WorkspaceID)
	}
}

func TestCollectionHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	tests := []struct {
		name        string
		collection  models.Collection
		expectedErr string
	}{
		{
			name:        "Missing name",
			collection:  models.Collection{Description: "Description only"},
			expectedErr: "Name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/collections", tt.collection)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestCollectionHandler_Create_InvalidWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	invalidWorkspace := 99999
	collection := models.Collection{
		Name:        "Test Collection",
		WorkspaceID: &invalidWorkspace,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Workspace not found") {
		t.Errorf("Expected 'Workspace not found' error, got %s", rr.Body.String())
	}
}

func TestCollectionHandler_Create_Unauthenticated(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	collection := models.Collection{
		Name: "Test Collection",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestCollectionHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create test collections
	collections := []models.Collection{
		{Name: "Collection 1", IsPublic: true},
		{Name: "Collection 2", IsPublic: false},
	}

	for _, c := range collections {
		req := testutils.CreateJSONRequest(t, "POST", "/api/collections", c)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/collections", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Collection
	rr.AssertJSONResponse(&response)

	if len(response) < 2 {
		t.Errorf("Expected at least 2 collections, got %d", len(response))
	}
}

func TestCollectionHandler_GetAll_FilterByWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	workspaceID := data.WorkspaceID

	// Create collection for workspace
	workspaceCollection := models.Collection{
		Name:        "Workspace Collection",
		WorkspaceID: &workspaceID,
		IsPublic:    true,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", workspaceCollection)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	// Create global collection
	globalCollection := models.Collection{
		Name:     "Global Collection",
		IsPublic: true,
	}
	createReq2 := testutils.CreateJSONRequest(t, "POST", "/api/collections", globalCollection)
	testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq2, nil)

	// Get collections filtered by workspace
	req := testutils.CreateJSONRequest(t, "GET", "/api/collections?workspace_id="+testutils.IntToString(workspaceID), nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var response []models.Collection
	rr.AssertJSONResponse(&response)

	// Should only include workspace collection
	for _, c := range response {
		if c.WorkspaceID == nil || *c.WorkspaceID != workspaceID {
			t.Errorf("Expected only workspace %d collections, got workspace %v", workspaceID, c.WorkspaceID)
		}
	}
}

func TestCollectionHandler_GetAll_Unauthenticated(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/collections", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestCollectionHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create test collection
	collection := models.Collection{
		Name:        "Test Get Collection",
		Description: "Test description",
		QLQuery:     "priority = 'high'",
		IsPublic:    true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Get the collection
	req := testutils.CreateJSONRequest(t, "GET", "/api/collections/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Collection
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != collection.Name {
		t.Errorf("Expected name %s, got %s", collection.Name, response.Name)
	}
}

func TestCollectionHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	req := testutils.CreateJSONRequest(t, "GET", "/api/collections/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "collection not found") {
		t.Errorf("Expected 'collection not found', got %s", rr.Body.String())
	}
}

func TestCollectionHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create test collection
	collection := models.Collection{
		Name:        "Original Name",
		Description: "Original description",
		IsPublic:    false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Update the collection
	updateData := models.Collection{
		Name:        "Updated Name",
		Description: "Updated description",
		QLQuery:     "status = 'closed'",
		IsPublic:    true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/collections/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	// Verify the update by getting the collection
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/collections/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	var response models.Collection
	getRR.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", response.Name)
	}
}

func TestCollectionHandler_Update_ValidationError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create test collection
	collection := models.Collection{
		Name: "Test Collection",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Try to update with empty name
	updateData := models.Collection{
		Name: "",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/collections/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestCollectionHandler_Update_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	updateData := models.Collection{
		Name: "Updated Name",
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/collections/99999", updateData)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCollectionHandler_Update_PermissionDenied(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create collection as default user
	collection := models.Collection{
		Name:     "Test Collection",
		IsPublic: true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Try to update as different user
	differentUser := testutils.TestUserWithID(99)
	updateData := models.Collection{
		Name: "Updated Name",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/collections/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, differentUser)

	rr.AssertStatusCode(http.StatusForbidden)
	if !strings.Contains(rr.Body.String(), "Insufficient permissions") {
		t.Errorf("Expected 'Insufficient permissions', got %s", rr.Body.String())
	}
}

func TestCollectionHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create test collection
	collection := models.Collection{
		Name: "To Delete",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Delete the collection
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/collections/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusOK)

	// Verify collection is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/collections/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestCollectionHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/collections/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCollectionHandler_Delete_PermissionDenied(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	permService, _, _ := createTestServices(t, *tdb)
	handler := NewCollectionHandler(tdb.GetDatabase(), permService)

	// Create collection as default user
	collection := models.Collection{
		Name:     "Test Collection",
		IsPublic: true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/collections", collection)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.Collection
	createRR.AssertJSONResponse(&created)

	// Try to delete as different user
	differentUser := testutils.TestUserWithID(99)

	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/collections/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, differentUser)

	rr.AssertStatusCode(http.StatusForbidden)
	if !strings.Contains(rr.Body.String(), "Insufficient permissions") {
		t.Errorf("Expected 'Insufficient permissions', got %s", rr.Body.String())
	}
}
