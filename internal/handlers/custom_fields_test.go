//go:build test

package handlers

import (
	"net/http"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func createCustomFieldHandler(t *testing.T, tdb *testutils.TestDB) *CustomFieldHandler {
	t.Helper()
	return NewCustomFieldHandler(tdb.GetDatabase())
}

// --- Create ---

func TestCustomFieldHandler_Create_TextField(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	body := map[string]interface{}{
		"name":       "Priority Level",
		"field_type": "text",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var cf models.CustomFieldDefinition
	rr.AssertJSONResponse(&cf)

	if cf.ID == 0 {
		t.Error("Expected custom field to have an ID")
	}
	if cf.Name != "Priority Level" {
		t.Errorf("Expected name 'Priority Level', got %q", cf.Name)
	}
	if cf.FieldType != "text" {
		t.Errorf("Expected field_type 'text', got %q", cf.FieldType)
	}
}

func TestCustomFieldHandler_Create_SelectWithOptions(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	body := map[string]interface{}{
		"name":       "Environment",
		"field_type": "select",
		"options":    `["Production","Staging","Development"]`,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var cf models.CustomFieldDefinition
	rr.AssertJSONResponse(&cf)

	if cf.ID == 0 {
		t.Error("Expected custom field to have an ID")
	}
	if cf.FieldType != "select" {
		t.Errorf("Expected field_type 'select', got %q", cf.FieldType)
	}
}

func TestCustomFieldHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{
			name: "Empty name",
			body: map[string]interface{}{
				"name":       "",
				"field_type": "text",
			},
		},
		{
			name: "Invalid field type",
			body: map[string]interface{}{
				"name":       "Test",
				"field_type": "invalid_type",
			},
		},
		{
			name: "Select with empty options",
			body: map[string]interface{}{
				"name":       "Test",
				"field_type": "select",
				"options":    `[]`,
			},
		},
		{
			name: "Asset missing asset_set_id",
			body: map[string]interface{}{
				"name":       "Test",
				"field_type": "asset",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", tt.body)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}

func TestCustomFieldHandler_Create_InvalidBody(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	req, _ := testutils.MockHTTPRequest("POST", "/api/custom-fields", nil)
	req.Body = http.NoBody
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// --- GetAll ---

func TestCustomFieldHandler_GetAll_Empty(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Delete any system defaults that may exist
	_, _ = tdb.Exec("DELETE FROM custom_field_definitions")

	req := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var fields []models.CustomFieldDefinition
	rr.AssertJSONResponse(&fields)

	if len(fields) != 0 {
		t.Errorf("Expected 0 fields, got %d", len(fields))
	}
}

func TestCustomFieldHandler_GetAll_WithFields(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Delete any system defaults
	_, _ = tdb.Exec("DELETE FROM custom_field_definitions")

	// Create 2 fields
	for _, name := range []string{"Field A", "Field B"} {
		body := map[string]interface{}{
			"name":       name,
			"field_type": "text",
		}
		req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
		rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
		rr.AssertStatusCode(http.StatusCreated)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var fields []models.CustomFieldDefinition
	rr.AssertJSONResponse(&fields)

	if len(fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(fields))
	}
}

// --- Get ---

func TestCustomFieldHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Create a field
	body := map[string]interface{}{
		"name":       "Get Test Field",
		"field_type": "number",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	var created models.CustomFieldDefinition
	createRR.AssertJSONResponse(&created)

	// Get it
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields/1", nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var fetched models.CustomFieldDefinition
	rr.AssertJSONResponse(&fetched)

	if fetched.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, fetched.ID)
	}
	if fetched.Name != "Get Test Field" {
		t.Errorf("Expected name 'Get Test Field', got %q", fetched.Name)
	}
}

func TestCustomFieldHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	req := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

// --- Update ---

func TestCustomFieldHandler_Update_Name(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Create a field
	createBody := map[string]interface{}{
		"name":       "Original Name",
		"field_type": "text",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", createBody)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	var created models.CustomFieldDefinition
	createRR.AssertJSONResponse(&created)

	// Update name
	updateBody := map[string]interface{}{
		"name":       "Updated Name",
		"field_type": "text",
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/custom-fields/1", updateBody)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var updated models.CustomFieldDefinition
	rr.AssertJSONResponse(&updated)

	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %q", updated.Name)
	}
}

func TestCustomFieldHandler_Update_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	body := map[string]interface{}{
		"name":       "Test",
		"field_type": "text",
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/custom-fields/99999", body)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

// --- Delete ---

func TestCustomFieldHandler_Delete_AndVerifyGone(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Create a field
	createBody := map[string]interface{}{
		"name":       "Delete Me",
		"field_type": "text",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", createBody)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	var created models.CustomFieldDefinition
	createRR.AssertJSONResponse(&created)

	// Delete
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/custom-fields/1", nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify it's gone
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields/1", nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestCustomFieldHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/custom-fields/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCustomFieldHandler_Delete_SystemDefault(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Insert a system_default field directly via SQL
	var id int
	err := tdb.QueryRow(`
		INSERT INTO custom_field_definitions (name, field_type, required, display_order, system_default, created_at, updated_at)
		VALUES ('System Field', 'text', 0, 1, 1, datetime('now'), datetime('now')) RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert system default field: %v", err)
	}

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/custom-fields/1", nil)
	req.SetPathValue("id", testutils.IntToString(id))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusForbidden)
}

