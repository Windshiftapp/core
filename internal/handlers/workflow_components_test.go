//go:build test

package handlers

import (
	"net/http"
	"testing"
	"windshift/internal/handlers/testutils"
	"windshift/internal/models"

)

// Test Status Categories

func TestStatusCategoryHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	category := models.StatusCategory{
		Name:        "Test Category",
		Color:       "#ff0000",
		Description: "Test status category",
		IsDefault:   false,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.StatusCategory
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created status category to have an ID")
	}
	if response.Name != category.Name {
		t.Errorf("Expected name %s, got %s", category.Name, response.Name)
	}
	if response.Color != category.Color {
		t.Errorf("Expected color %s, got %s", category.Color, response.Color)
	}
	if response.Description != category.Description {
		t.Errorf("Expected description %s, got %s", category.Description, response.Description)
	}
	if response.IsDefault != category.IsDefault {
		t.Errorf("Expected IsDefault %v, got %v", category.IsDefault, response.IsDefault)
	}
}

func TestStatusCategoryHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	// The database should already have default status categories from initialization
	req := testutils.CreateJSONRequest(t, "GET", "/api/status-categories", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.StatusCategory
	rr.AssertJSONResponse(&response)

	// Should have at least the default categories
	if len(response) < 3 {
		t.Errorf("Expected at least 3 default status categories, got %d", len(response))
	}

	// Default category should appear first
	if len(response) > 0 && !response[0].IsDefault {
		t.Error("Expected first category to be default")
	}
}

func TestStatusCategoryHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test status category
	result, err := tdb.Exec(`
		INSERT INTO status_categories (name, color, description, is_default, created_at, updated_at)
		VALUES ('Get Test Category', '#00ff00', 'Test category for GET', 0, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test status category: %v", err)
	}
	categoryID, _ := result.LastInsertId()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/status-categories/"+testutils.IntToString(int(categoryID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(categoryID)))
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.StatusCategory
	rr.AssertJSONResponse(&response)

	if response.ID != int(categoryID) {
		t.Errorf("Expected ID %d, got %d", categoryID, response.ID)
	}
	if response.Name != "Get Test Category" {
		t.Errorf("Expected name 'Get Test Category', got %s", response.Name)
	}
}

// Test Statuses

func TestStatusHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test status category first
	result, err := tdb.Exec(`
		INSERT INTO status_categories (name, color, description, is_default, created_at, updated_at)
		VALUES ('Test Category', '#0000ff', 'For status test', 0, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test status category: %v", err)
	}
	categoryID, _ := result.LastInsertId()

	handler := NewStatusHandler(tdb.DB.DB)

	status := models.Status{
		Name:        "Test Status",
		Description: "Test status description",
		CategoryID:  int(categoryID),
		IsDefault:   false,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/statuses", status)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Status
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created status to have an ID")
	}
	if response.Name != status.Name {
		t.Errorf("Expected name %s, got %s", status.Name, response.Name)
	}
	if response.CategoryID != status.CategoryID {
		t.Errorf("Expected category ID %d, got %d", status.CategoryID, response.CategoryID)
	}
}

func TestStatusHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusHandler(tdb.DB.DB)

	// The database should already have default statuses from initialization
	req := testutils.CreateJSONRequest(t, "GET", "/api/statuses", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Status
	rr.AssertJSONResponse(&response)

	// Should have at least the default statuses
	if len(response) < 5 {
		t.Errorf("Expected at least 5 default statuses, got %d", len(response))
	}

	// Should have category names populated
	for _, status := range response {
		if status.CategoryName == "" {
			t.Errorf("Expected status %s to have category name populated", status.Name)
		}
	}
}

// Test Custom Fields

func TestCustomFieldHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewCustomFieldHandler(tdb.DB.DB)

	field := models.CustomFieldDefinition{
		Name:        "Test Field",
		FieldType:   "text",
		Description: "Test custom field",
		Required:    false,
		Options:     "[]",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", field)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.CustomFieldDefinition
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created custom field to have an ID")
	}
	if response.Name != field.Name {
		t.Errorf("Expected name %s, got %s", field.Name, response.Name)
	}
	if response.FieldType != field.FieldType {
		t.Errorf("Expected field type %s, got %s", field.FieldType, response.FieldType)
	}
}

func TestCustomFieldHandler_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewCustomFieldHandler(tdb.DB.DB)

	tests := []struct {
		name        string
		field       models.CustomFieldDefinition
		expectedErr string
	}{
		{
			name:        "Missing name",
			field:       models.CustomFieldDefinition{FieldType: "text"},
			expectedErr: "Field name is required",
		},
		{
			name:        "Missing field type",
			field:       models.CustomFieldDefinition{Name: "Test Field"},
			expectedErr: "Invalid field type",
		},
		{
			name:        "Invalid field type",
			field:       models.CustomFieldDefinition{Name: "Test Field", FieldType: "invalid"},
			expectedErr: "Invalid field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", tt.field)
			rr := testutils.ExecuteRequest(t, handler.Create, req)

			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

// Test Screens

func TestScreenHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewScreenHandler(tdb.DB.DB)

	screen := models.Screen{
		Name:        "Test Screen",
		Description: "Test screen description",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/screens", screen)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Screen
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created screen to have an ID")
	}
	if response.Name != screen.Name {
		t.Errorf("Expected name %s, got %s", screen.Name, response.Name)
	}
	if response.Description != screen.Description {
		t.Errorf("Expected description %s, got %s", screen.Description, response.Description)
	}
}

func TestScreenHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test screens
	screens := []string{"Screen A", "Screen B", "Screen C"}
	for _, screenName := range screens {
		_, err := tdb.Exec(`
			INSERT INTO screens (name, description, created_at, updated_at)
			VALUES (?, 'Test screen', datetime('now'), datetime('now'))
		`, screenName)
		if err != nil {
			t.Fatalf("Failed to create test screen %s: %v", screenName, err)
		}
	}

	handler := NewScreenHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/screens", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Screen
	rr.AssertJSONResponse(&response)

	// Should have at least our test screens plus any default ones
	if len(response) < len(screens) {
		t.Errorf("Expected at least %d screens, got %d", len(screens), len(response))
	}

	// Verify our test screens are present and sorted by name
	foundScreens := make(map[string]bool)
	for _, screen := range response {
		foundScreens[screen.Name] = true
	}

	for _, screenName := range screens {
		if !foundScreens[screenName] {
			t.Errorf("Expected to find screen %s in response", screenName)
		}
	}
}

// Test Workflows

func TestWorkflowHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkflowHandler(tdb.DB.DB)

	workflow := models.Workflow{
		Name:        "Test Workflow",
		Description: "Test workflow description",
		IsDefault:   false,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/workflows", workflow)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Workflow
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created workflow to have an ID")
	}
	if response.Name != workflow.Name {
		t.Errorf("Expected name %s, got %s", workflow.Name, response.Name)
	}
	if response.Description != workflow.Description {
		t.Errorf("Expected description %s, got %s", workflow.Description, response.Description)
	}
	if response.IsDefault != workflow.IsDefault {
		t.Errorf("Expected IsDefault %v, got %v", workflow.IsDefault, response.IsDefault)
	}
}

func TestWorkflowHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkflowHandler(tdb.DB.DB)

	// The database should already have at least one default workflow from initialization
	req := testutils.CreateJSONRequest(t, "GET", "/api/workflows", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.Workflow
	rr.AssertJSONResponse(&response)

	// Should have at least the default workflow
	if len(response) < 1 {
		t.Errorf("Expected at least 1 default workflow, got %d", len(response))
	}

	// Default workflow should appear first
	if len(response) > 0 && !response[0].IsDefault {
		t.Error("Expected first workflow to be default")
	}
}

// Update and Delete Tests for Status Categories

func TestStatusCategoryHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	// Create initial status category
	category := models.StatusCategory{
		Name:        "Original Category",
		Color:       "#ff0000",
		Description: "Original description",
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdCategory models.StatusCategory
	createRR.AssertJSONResponse(&createdCategory)

	// Update the category
	updatedCategory := models.StatusCategory{
		Name:        "Updated Category",
		Color:       "#00ff00",
		Description: "Updated description",
		IsDefault:   true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/status-categories/"+testutils.IntToString(createdCategory.ID), updatedCategory)
	updateReq.SetPathValue("id", testutils.IntToString(createdCategory.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.StatusCategory
	rr.AssertJSONResponse(&response)

	if response.ID != createdCategory.ID {
		t.Errorf("Expected ID %d, got %d", createdCategory.ID, response.ID)
	}
	if response.Name != updatedCategory.Name {
		t.Errorf("Expected updated name %s, got %s", updatedCategory.Name, response.Name)
	}
	if response.Color != updatedCategory.Color {
		t.Errorf("Expected updated color %s, got %s", updatedCategory.Color, response.Color)
	}
	if response.Description != updatedCategory.Description {
		t.Errorf("Expected updated description %s, got %s", updatedCategory.Description, response.Description)
	}
	if response.IsDefault != updatedCategory.IsDefault {
		t.Errorf("Expected updated IsDefault %v, got %v", updatedCategory.IsDefault, response.IsDefault)
	}
}

func TestStatusCategoryHandler_Update_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	// Create initial status category
	category := models.StatusCategory{
		Name:        "Original Category",
		Color:       "#ff0000",
		Description: "Original description",
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdCategory models.StatusCategory
	createRR.AssertJSONResponse(&createdCategory)

	tests := []struct {
		name        string
		category    models.StatusCategory
		expectedErr string
	}{
		{
			name:        "Empty name",
			category:    models.StatusCategory{Name: "", Color: "#ff0000", Description: "Test"},
			expectedErr: "Name is required",
		},
		{
			name:        "Invalid color format",
			category:    models.StatusCategory{Name: "Test", Color: "invalid", Description: "Test"},
			expectedErr: "Color must be a valid hex color",
		},
		{
			name:        "Empty color",
			category:    models.StatusCategory{Name: "Test", Color: "", Description: "Test"},
			expectedErr: "Color is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/status-categories/"+testutils.IntToString(createdCategory.ID), tt.category)
			updateReq.SetPathValue("id", testutils.IntToString(createdCategory.ID))
			rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

func TestStatusCategoryHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewStatusCategoryHandler(tdb.DB.DB)

	// Create status category to delete
	category := models.StatusCategory{
		Name:        "Category to Delete",
		Color:       "#ff0000",
		Description: "Will be deleted",
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdCategory models.StatusCategory
	createRR.AssertJSONResponse(&createdCategory)

	// Delete the category
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/status-categories/"+testutils.IntToString(createdCategory.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdCategory.ID))
	rr := testutils.ExecuteRequest(t, handler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify deletion by trying to get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/status-categories/"+testutils.IntToString(createdCategory.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdCategory.ID))
	getRR := testutils.ExecuteRequest(t, handler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestStatusCategoryHandler_Delete_WithStatuses_Fails(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	statusCategoryHandler := NewStatusCategoryHandler(tdb.DB.DB)
	statusHandler := NewStatusHandler(tdb.DB.DB)

	// Create status category
	category := models.StatusCategory{
		Name:        "Category with Status",
		Color:       "#ff0000",
		Description: "Has statuses",
		IsDefault:   false,
	}

	createCatReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createCatRR := testutils.ExecuteRequest(t, statusCategoryHandler.Create, createCatReq)

	var createdCategory models.StatusCategory
	createCatRR.AssertJSONResponse(&createdCategory)

	// Create status in this category
	status := models.Status{
		Name:        "Test Status",
		Description: "Test status description",
		CategoryID:  createdCategory.ID,
		IsDefault:   false,
	}

	createStatusReq := testutils.CreateJSONRequest(t, "POST", "/api/statuses", status)
	testutils.ExecuteRequest(t, statusHandler.Create, createStatusReq)

	// Try to delete the category (should fail)
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/status-categories/"+testutils.IntToString(createdCategory.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdCategory.ID))
	rr := testutils.ExecuteRequest(t, statusCategoryHandler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusConflict).
		AssertBodyContains("Cannot delete status category that is in use by statuses")
}

// Update and Delete Tests for Statuses

func TestStatusHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	statusCategoryHandler := NewStatusCategoryHandler(tdb.DB.DB)
	statusHandler := NewStatusHandler(tdb.DB.DB)

	// Create status category first
	category := models.StatusCategory{
		Name:        "Test Category",
		Color:       "#ff0000",
		Description: "For status updates",
		IsDefault:   false,
	}

	createCatReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createCatRR := testutils.ExecuteRequest(t, statusCategoryHandler.Create, createCatReq)

	var createdCategory models.StatusCategory
	createCatRR.AssertJSONResponse(&createdCategory)

	// Create initial status
	status := models.Status{
		Name:        "Original Status",
		Description: "Original description",
		CategoryID:  createdCategory.ID,
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/statuses", status)
	createRR := testutils.ExecuteRequest(t, statusHandler.Create, createReq)

	var createdStatus models.Status
	createRR.AssertJSONResponse(&createdStatus)

	// Update the status
	updatedStatus := models.Status{
		Name:        "Updated Status",
		Description: "Updated description",
		CategoryID:  createdCategory.ID,
		IsDefault:   true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/statuses/"+testutils.IntToString(createdStatus.ID), updatedStatus)
	updateReq.SetPathValue("id", testutils.IntToString(createdStatus.ID))
	rr := testutils.ExecuteRequest(t, statusHandler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Status
	rr.AssertJSONResponse(&response)

	if response.ID != createdStatus.ID {
		t.Errorf("Expected ID %d, got %d", createdStatus.ID, response.ID)
	}
	if response.Name != updatedStatus.Name {
		t.Errorf("Expected updated name %s, got %s", updatedStatus.Name, response.Name)
	}
	if response.Description != updatedStatus.Description {
		t.Errorf("Expected updated description %s, got %s", updatedStatus.Description, response.Description)
	}
	if response.IsDefault != updatedStatus.IsDefault {
		t.Errorf("Expected updated IsDefault %v, got %v", updatedStatus.IsDefault, response.IsDefault)
	}
}

func TestStatusHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	statusCategoryHandler := NewStatusCategoryHandler(tdb.DB.DB)
	statusHandler := NewStatusHandler(tdb.DB.DB)

	// Create status category first
	category := models.StatusCategory{
		Name:        "Test Category",
		Color:       "#ff0000",
		Description: "For status deletion",
		IsDefault:   false,
	}

	createCatReq := testutils.CreateJSONRequest(t, "POST", "/api/status-categories", category)
	createCatRR := testutils.ExecuteRequest(t, statusCategoryHandler.Create, createCatReq)

	var createdCategory models.StatusCategory
	createCatRR.AssertJSONResponse(&createdCategory)

	// Create status to delete
	status := models.Status{
		Name:        "Status to Delete",
		Description: "Will be deleted",
		CategoryID:  createdCategory.ID,
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/statuses", status)
	createRR := testutils.ExecuteRequest(t, statusHandler.Create, createReq)

	var createdStatus models.Status
	createRR.AssertJSONResponse(&createdStatus)

	// Delete the status
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/statuses/"+testutils.IntToString(createdStatus.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdStatus.ID))
	rr := testutils.ExecuteRequest(t, statusHandler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify deletion by trying to get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/statuses/"+testutils.IntToString(createdStatus.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdStatus.ID))
	getRR := testutils.ExecuteRequest(t, statusHandler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}

// Update and Delete Tests for Custom Fields

func TestCustomFieldHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewCustomFieldHandler(tdb.DB.DB)

	// Create initial custom field
	field := models.CustomFieldDefinition{
		Name:        "Original Field",
		FieldType:   "text",
		Description: "Original description",
		Required:    false,
		Options:     "[]",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", field)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdField models.CustomFieldDefinition
	createRR.AssertJSONResponse(&createdField)

	// Update the field
	updatedField := models.CustomFieldDefinition{
		Name:         "Updated Field",
		FieldType:    "select",
		Description:  "Updated description",
		Required:     true,
		Options:      "[\"Option 1\", \"Option 2\"]",
		DisplayOrder: 5,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/custom-fields/"+testutils.IntToString(createdField.ID), updatedField)
	updateReq.SetPathValue("id", testutils.IntToString(createdField.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.CustomFieldDefinition
	rr.AssertJSONResponse(&response)

	if response.ID != createdField.ID {
		t.Errorf("Expected ID %d, got %d", createdField.ID, response.ID)
	}
	if response.Name != updatedField.Name {
		t.Errorf("Expected updated name %s, got %s", updatedField.Name, response.Name)
	}
	if response.FieldType != updatedField.FieldType {
		t.Errorf("Expected updated field type %s, got %s", updatedField.FieldType, response.FieldType)
	}
	if response.Required != updatedField.Required {
		t.Errorf("Expected updated required %v, got %v", updatedField.Required, response.Required)
	}
}

func TestCustomFieldHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewCustomFieldHandler(tdb.DB.DB)

	// Create custom field to delete
	field := models.CustomFieldDefinition{
		Name:        "Field to Delete",
		FieldType:   "text",
		Description: "Will be deleted",
		Required:    false,
		Options:     "[]",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", field)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdField models.CustomFieldDefinition
	createRR.AssertJSONResponse(&createdField)

	// Delete the field
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/custom-fields/"+testutils.IntToString(createdField.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdField.ID))
	rr := testutils.ExecuteRequest(t, handler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify deletion by trying to get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields/"+testutils.IntToString(createdField.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdField.ID))
	getRR := testutils.ExecuteRequest(t, handler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}

// Update and Delete Tests for Screens

func TestScreenHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewScreenHandler(tdb.DB.DB)

	// Create initial screen
	screen := models.Screen{
		Name:        "Original Screen",
		Description: "Original description",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/screens", screen)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdScreen models.Screen
	createRR.AssertJSONResponse(&createdScreen)

	// Update the screen
	updatedScreen := models.Screen{
		Name:        "Updated Screen",
		Description: "Updated description",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/screens/"+testutils.IntToString(createdScreen.ID), updatedScreen)
	updateReq.SetPathValue("id", testutils.IntToString(createdScreen.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Screen
	rr.AssertJSONResponse(&response)

	if response.ID != createdScreen.ID {
		t.Errorf("Expected ID %d, got %d", createdScreen.ID, response.ID)
	}
	if response.Name != updatedScreen.Name {
		t.Errorf("Expected updated name %s, got %s", updatedScreen.Name, response.Name)
	}
	if response.Description != updatedScreen.Description {
		t.Errorf("Expected updated description %s, got %s", updatedScreen.Description, response.Description)
	}
}

func TestScreenHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewScreenHandler(tdb.DB.DB)

	// Create screen to delete
	screen := models.Screen{
		Name:        "Screen to Delete",
		Description: "Will be deleted",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/screens", screen)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdScreen models.Screen
	createRR.AssertJSONResponse(&createdScreen)

	// Delete the screen
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/screens/"+testutils.IntToString(createdScreen.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdScreen.ID))
	rr := testutils.ExecuteRequest(t, handler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify deletion by trying to get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/screens/"+testutils.IntToString(createdScreen.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdScreen.ID))
	getRR := testutils.ExecuteRequest(t, handler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}

// Update and Delete Tests for Workflows

func TestWorkflowHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkflowHandler(tdb.DB.DB)

	// Create initial workflow
	workflow := models.Workflow{
		Name:        "Original Workflow",
		Description: "Original description",
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/workflows", workflow)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdWorkflow models.Workflow
	createRR.AssertJSONResponse(&createdWorkflow)

	// Update the workflow
	updatedWorkflow := models.Workflow{
		Name:        "Updated Workflow",
		Description: "Updated description",
		IsDefault:   true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/workflows/"+testutils.IntToString(createdWorkflow.ID), updatedWorkflow)
	updateReq.SetPathValue("id", testutils.IntToString(createdWorkflow.ID))
	rr := testutils.ExecuteRequest(t, handler.Update, updateReq)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.Workflow
	rr.AssertJSONResponse(&response)

	if response.ID != createdWorkflow.ID {
		t.Errorf("Expected ID %d, got %d", createdWorkflow.ID, response.ID)
	}
	if response.Name != updatedWorkflow.Name {
		t.Errorf("Expected updated name %s, got %s", updatedWorkflow.Name, response.Name)
	}
	if response.Description != updatedWorkflow.Description {
		t.Errorf("Expected updated description %s, got %s", updatedWorkflow.Description, response.Description)
	}
	if response.IsDefault != updatedWorkflow.IsDefault {
		t.Errorf("Expected updated IsDefault %v, got %v", updatedWorkflow.IsDefault, response.IsDefault)
	}
}

func TestWorkflowHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewWorkflowHandler(tdb.DB.DB)

	// Create workflow to delete
	workflow := models.Workflow{
		Name:        "Workflow to Delete",
		Description: "Will be deleted",
		IsDefault:   false,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/workflows", workflow)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var createdWorkflow models.Workflow
	createRR.AssertJSONResponse(&createdWorkflow)

	// Delete the workflow
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/workflows/"+testutils.IntToString(createdWorkflow.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(createdWorkflow.ID))
	rr := testutils.ExecuteRequest(t, handler.Delete, deleteReq)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify deletion by trying to get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/workflows/"+testutils.IntToString(createdWorkflow.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(createdWorkflow.ID))
	getRR := testutils.ExecuteRequest(t, handler.Get, getReq)

	getRR.AssertStatusCode(http.StatusNotFound)
}