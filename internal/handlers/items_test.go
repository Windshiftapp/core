//go:build test

package handlers

import (
	"net/http"
	"testing"
	"windshift/internal/handlers/testutils"
	"windshift/internal/models"

)

func TestItemHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Test Item",
		Description: "Test item description",
		Status:      "open",
		Priority:    "medium",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Item
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created item to have an ID")
	}
	if response.Title != item.Title {
		t.Errorf("Expected title %s, got %s", item.Title, response.Title)
	}
	if response.WorkspaceID != item.WorkspaceID {
		t.Errorf("Expected workspace ID %d, got %d", item.WorkspaceID, response.WorkspaceID)
	}
	if response.Level != 0 {
		t.Errorf("Expected level 0 for root item, got %d", response.Level)
	}
	if response.Path == "" {
		t.Error("Expected item to have a path")
	}
}

func TestItemHandler_Create_WithParent(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create parent item first
	parentItem := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Parent Item",
		Description: "Parent item description",
		Status:      "open",
		Priority:    "medium",
	}

	parentReq := testutils.CreateJSONRequest(t, "POST", "/api/items", parentItem)
	parentRR := testutils.ExecuteRequest(t, handler.Create, parentReq)

	var parentResponse models.Item
	parentRR.AssertJSONResponse(&parentResponse)

	// Create child item
	childItem := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Child Item",
		Description: "Child item description",
		Status:      "open",
		Priority:    "medium",
		ParentID:    &parentResponse.ID,
	}

	childReq := testutils.CreateJSONRequest(t, "POST", "/api/items", childItem)
	childRR := testutils.ExecuteRequest(t, handler.Create, childReq)

	childRR.AssertStatusCode(http.StatusCreated)

	var childResponse models.Item
	childRR.AssertJSONResponse(&childResponse)

	if childResponse.Level != 1 {
		t.Errorf("Expected child item level 1, got %d", childResponse.Level)
	}
	if childResponse.ParentID == nil || *childResponse.ParentID != parentResponse.ID {
		t.Errorf("Expected parent ID %d, got %v", parentResponse.ID, childResponse.ParentID)
	}
}

func TestItemHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	tests := []struct {
		name        string
		item        models.Item
		expectedErr string
	}{
		{
			name:        "Missing title",
			item:        models.Item{WorkspaceID: data.WorkspaceID, Title: ""},
			expectedErr: "Title is required",
		},
		{
			name:        "Invalid workspace",
			item:        models.Item{WorkspaceID: 99999, Title: "Test Item"},
			expectedErr: "Workspace not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/items", tt.item)
			rr := testutils.ExecuteRequest(t, handler.Create, req)

			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

func TestItemHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test items
	for i := 0; i < 3; i++ {
		item := models.Item{
			WorkspaceID: data.WorkspaceID,
			Title:       "Test Item " + testutils.IntToString(i+1),
			Description: "Test description",
			Status:      "open",
			Priority:    "medium",
		}

		req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
		testutils.ExecuteRequest(t, handler.Create, req)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/items?workspace_id="+testutils.IntToString(data.WorkspaceID), nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.PaginatedItemsResponse
	rr.AssertJSONResponse(&response)

	if len(response.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(response.Items))
	}

	// Check pagination metadata
	if response.Pagination.Total != 3 {
		t.Errorf("Expected total count 3, got %d", response.Pagination.Total)
	}
}

func TestItemHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test item
	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Get Test Item",
		Description: "Test description",
		Status:      "open",
		Priority:    "high",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdItem models.Item
	createRR.AssertJSONResponse(&createdItem)

	// Get the item
	req := testutils.CreateJSONRequest(t, "GET", "/api/items/"+testutils.IntToString(createdItem.ID), nil)
	req.SetPathValue("id", testutils.IntToString(createdItem.ID))
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Item
	rr.AssertJSONResponse(&response)

	if response.ID != createdItem.ID {
		t.Errorf("Expected ID %d, got %d", createdItem.ID, response.ID)
	}
	if response.Title != item.Title {
		t.Errorf("Expected title %s, got %s", item.Title, response.Title)
	}
	if response.Priority != item.Priority {
		t.Errorf("Expected priority %s, got %s", item.Priority, response.Priority)
	}
}

func TestItemHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test item
	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Original Title",
		Description: "Original description",
		Status:      "open",
		Priority:    "medium",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdItem models.Item
	createRR.AssertJSONResponse(&createdItem)

	// Update the item
	updateData := map[string]interface{}{
		"title":       "Updated Title",
		"description": "Updated description",
		"priority":    "high",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/items/"+testutils.IntToString(createdItem.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(createdItem.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Item
	rr.AssertJSONResponse(&response)

	if response.Title != "Updated Title" {
		t.Errorf("Expected updated title 'Updated Title', got %s", response.Title)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected updated description 'Updated description', got %s", response.Description)
	}
	if response.Priority != "high" {
		t.Errorf("Expected updated priority 'high', got %s", response.Priority)
	}
}

func TestItemHandler_Update_CustomFields(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test item with custom fields
	customFields := map[string]interface{}{
		"field1": "value1",
		"field2": 123,
	}

	item := models.Item{
		WorkspaceID:        data.WorkspaceID,
		Title:              "Item with Custom Fields",
		Description:        "Test description",
		Status:             "open",
		Priority:           "medium",
		CustomFieldValues: customFields,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdItem models.Item
	createRR.AssertJSONResponse(&createdItem)

	// Update custom fields
	updatedCustomFields := map[string]interface{}{
		"field1": "updated_value1",
		"field3": "new_field",
	}

	updateData := map[string]interface{}{
		"custom_field_values": updatedCustomFields,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/items/"+testutils.IntToString(createdItem.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(createdItem.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK)

	var response models.Item
	rr.AssertJSONResponse(&response)

	if response.CustomFieldValues["field1"] != "updated_value1" {
		t.Errorf("Expected custom field1 'updated_value1', got %v", response.CustomFieldValues["field1"])
	}
	if response.CustomFieldValues["field3"] != "new_field" {
		t.Errorf("Expected custom field3 'new_field', got %v", response.CustomFieldValues["field3"])
	}
	// field2 should be gone since it wasn't included in the update
	if _, exists := response.CustomFieldValues["field2"]; exists {
		t.Error("Expected field2 to be removed, but it still exists")
	}
}

func TestItemHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test item
	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Item to Delete",
		Description: "This item will be deleted",
		Status:      "open",
		Priority:    "medium",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdItem models.Item
	createRR.AssertJSONResponse(&createdItem)

	// Delete the item
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/items/"+testutils.IntToString(createdItem.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdItem.ID))
	rr := testutils.ExecuteRequest(t, handler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify item is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/items/"+testutils.IntToString(createdItem.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdItem.ID))
	getRR := testutils.ExecuteRequest(t, handler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestItemHandler_GetChildren_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create parent item
	parentItem := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Parent Item",
		Description: "Parent item description",
		Status:      "open",
		Priority:    "medium",
	}

	parentReq := testutils.CreateJSONRequest(t, "POST", "/api/items", parentItem)
	parentRR := testutils.ExecuteRequest(t, handler.Create, parentReq)

	var parent models.Item
	parentRR.AssertJSONResponse(&parent)

	// Create child items
	for i := 0; i < 3; i++ {
		childItem := models.Item{
			WorkspaceID: data.WorkspaceID,
			Title:       "Child Item " + testutils.IntToString(i+1),
			Description: "Child description",
			Status:      "open",
			Priority:    "medium",
			ParentID:    &parent.ID,
		}

		childReq := testutils.CreateJSONRequest(t, "POST", "/api/items", childItem)
		testutils.ExecuteRequest(t, handler.Create, childReq)
	}

	// Get children
	req := testutils.CreateJSONRequest(t, "GET", "/api/items/"+testutils.IntToString(parent.ID)+"/children", nil)
	req.SetPathValue("id", testutils.IntToString(parent.ID))
	rr := testutils.ExecuteRequest(t, handler.GetChildren, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var children []models.Item
	rr.AssertJSONResponse(&children)

	if len(children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(children))
	}

	// Verify all children have correct parent
	for _, child := range children {
		if child.ParentID == nil || *child.ParentID != parent.ID {
			t.Errorf("Expected child to have parent ID %d, got %v", parent.ID, child.ParentID)
		}
		if child.Level != 1 {
			t.Errorf("Expected child level 1, got %d", child.Level)
		}
	}
}

func TestItemHandler_Search_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test items with different content
	items := []models.Item{
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "Bug in login system",
			Description: "Users cannot log in",
			Status:      "open",
			Priority:    "high",
		},
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "Feature request for dashboard",
			Description: "Add new widgets to dashboard",
			Status:      "to_do",
			Priority:    "medium",
		},
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "Documentation update",
			Description: "Update API documentation",
			Status:      "completed",
			Priority:    "low",
		},
	}

	for _, item := range items {
		req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
		testutils.ExecuteRequest(t, handler.Create, req)
	}

	// Search for items containing "dashboard"
	searchReq := testutils.CreateJSONRequest(t, "GET", "/api/items/search?q=dashboard", nil)
	rr := testutils.ExecuteRequest(t, handler.Search, searchReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var results []models.Item
	rr.AssertJSONResponse(&results)

	if len(results) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(results))
	}

	if len(results) > 0 {
		if results[0].Title != "Feature request for dashboard" {
			t.Errorf("Expected to find dashboard item, got %s", results[0].Title)
		}
	}
}

func TestItemHandler_Search_WithFilters(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewItemHandler(tdb.DB.DB)

	// Create test items with different statuses and priorities
	items := []models.Item{
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "High priority bug",
			Description: "Critical issue",
			Status:      "open",
			Priority:    "high",
		},
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "Medium priority task",
			Description: "Regular task",
			Status:      "in_progress",
			Priority:    "medium",
		},
		{
			WorkspaceID: data.WorkspaceID,
			Title:       "Completed high priority",
			Description: "Finished task",
			Status:      "completed",
			Priority:    "high",
		},
	}

	for _, item := range items {
		req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
		testutils.ExecuteRequest(t, handler.Create, req)
	}

	// Search for high priority items
	searchReq := testutils.CreateJSONRequest(t, "GET", "/api/items/search?priority=high", nil)
	rr := testutils.ExecuteRequest(t, handler.Search, searchReq)

	rr.AssertStatusCode(http.StatusOK)

	var results []models.Item
	rr.AssertJSONResponse(&results)

	if len(results) != 2 {
		t.Errorf("Expected 2 high priority items, got %d", len(results))
	}

	// Search for open status items
	searchReq2 := testutils.CreateJSONRequest(t, "GET", "/api/items/search?status=open", nil)
	rr2 := testutils.ExecuteRequest(t, handler.Search, searchReq2)

	rr2.AssertStatusCode(http.StatusOK)

	var results2 []models.Item
	rr2.AssertJSONResponse(&results2)

	if len(results2) != 1 {
		t.Errorf("Expected 1 open status item, got %d", len(results2))
	}
}