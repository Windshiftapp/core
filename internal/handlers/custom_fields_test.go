//go:build test

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func createCustomFieldHandler(t *testing.T, tdb *testutils.TestDB) *CustomFieldHandler {
	t.Helper()
	return NewCustomFieldHandler(tdb.GetDatabase())
}

// createField is a test helper that creates a custom field and returns it
func createField(t *testing.T, handler *CustomFieldHandler, name, fieldType string) models.CustomFieldDefinition {
	t.Helper()
	body := map[string]interface{}{
		"name":       name,
		"field_type": fieldType,
	}
	req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	rr.AssertStatusCode(http.StatusCreated)

	var cf models.CustomFieldDefinition
	rr.AssertJSONResponse(&cf)
	return cf
}

// assertIndexExists checks that a database index exists in sqlite_master
func assertIndexExists(t *testing.T, tdb *testutils.TestDB, indexName string) {
	t.Helper()
	var count int
	err := tdb.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, indexName).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check index existence: %v", err)
	}
	if count == 0 {
		t.Errorf("Expected index %q to exist, but it does not", indexName)
	}
}

// assertIndexNotExists checks that a database index does not exist
func assertIndexNotExists(t *testing.T, tdb *testutils.TestDB, indexName string) {
	t.Helper()
	var count int
	err := tdb.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, indexName).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check index existence: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected index %q to NOT exist, but it does", indexName)
	}
}

// assertIndexRecordExists checks the junction table has a row
func assertIndexRecordExists(t *testing.T, tdb *testutils.TestDB, fieldID int, targetTable string) {
	t.Helper()
	var count int
	err := tdb.QueryRow(`SELECT COUNT(*) FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`, fieldID, targetTable).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check index record: %v", err)
	}
	if count == 0 {
		t.Errorf("Expected index record for field %d on %s to exist", fieldID, targetTable)
	}
}

// assertIndexRecordNotExists checks the junction table has no row
func assertIndexRecordNotExists(t *testing.T, tdb *testutils.TestDB, fieldID int, targetTable string) {
	t.Helper()
	var count int
	err := tdb.QueryRow(`SELECT COUNT(*) FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`, fieldID, targetTable).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check index record: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected index record for field %d on %s to NOT exist", fieldID, targetTable)
	}
}

// enableIndex sends an update request with indexing enabled for the given table
func enableIndex(t *testing.T, handler *CustomFieldHandler, fieldID int, fieldType string, items, assets bool) *testutils.ResponseRecorder {
	t.Helper()
	body := map[string]interface{}{
		"name":       "IndexTest",
		"field_type": fieldType,
		"indexed": map[string]bool{
			"items":  items,
			"assets": assets,
		},
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/custom-fields/1", body)
	req.SetPathValue("id", testutils.IntToString(fieldID))
	return testutils.ExecuteAuthenticatedRequest(t, handler.Update, req, nil)
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

	var resp customFieldsResponse
	rr.AssertJSONResponse(&resp)

	if len(resp.Data) != 0 {
		t.Errorf("Expected 0 fields, got %d", len(resp.Data))
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
		createField(t, handler, name, "text")
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var resp customFieldsResponse
	rr.AssertJSONResponse(&resp)

	if len(resp.Data) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(resp.Data))
	}
}

// --- Get ---

func TestCustomFieldHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	created := createField(t, handler, "Get Test Field", "number")

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
	created := createField(t, handler, "Original Name", "text")

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
	created := createField(t, handler, "Delete Me", "text")

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

// --- Indexing ---

func TestCustomFieldHandler_EnableIndex_Number(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Cost", "number")

	rr := enableIndex(t, handler, cf.ID, "number", true, false)
	rr.AssertStatusCode(http.StatusOK)

	indexName := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assertIndexExists(t, tdb, indexName)
	assertIndexRecordExists(t, tdb, cf.ID, "items")
}

func TestCustomFieldHandler_EnableIndex_Text(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Serial", "text")

	rr := enableIndex(t, handler, cf.ID, "text", true, false)
	rr.AssertStatusCode(http.StatusOK)

	indexName := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assertIndexExists(t, tdb, indexName)
	assertIndexRecordExists(t, tdb, cf.ID, "items")
}

func TestCustomFieldHandler_EnableIndex_Date(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Deadline", "date")

	rr := enableIndex(t, handler, cf.ID, "date", true, false)
	rr.AssertStatusCode(http.StatusOK)

	indexName := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assertIndexExists(t, tdb, indexName)
	assertIndexRecordExists(t, tdb, cf.ID, "items")
}

func TestCustomFieldHandler_EnableIndex_NonIndexableType(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Create a select field (not indexable)
	body := map[string]interface{}{
		"name":       "Category",
		"field_type": "select",
		"options":    `["A","B","C"]`,
	}
	req := testutils.CreateJSONRequest(t, "POST", "/api/custom-fields", body)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	var cf models.CustomFieldDefinition
	createRR.AssertJSONResponse(&cf)

	// Try to enable indexing - should fail with 400
	rr := enableIndex(t, handler, cf.ID, "select", true, false)
	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestCustomFieldHandler_DisableIndex(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Cost", "number")

	// Enable
	rr := enableIndex(t, handler, cf.ID, "number", true, false)
	rr.AssertStatusCode(http.StatusOK)

	indexName := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assertIndexExists(t, tdb, indexName)

	// Disable
	rr = enableIndex(t, handler, cf.ID, "number", false, false)
	rr.AssertStatusCode(http.StatusOK)

	assertIndexNotExists(t, tdb, indexName)
	assertIndexRecordNotExists(t, tdb, cf.ID, "items")
}

func TestCustomFieldHandler_IndexLimit(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Set limit to 2 for testing
	_, err := tdb.Exec(`UPDATE system_settings SET value = '2' WHERE key = 'max_custom_field_indexes_per_table'`)
	if err != nil {
		t.Fatalf("Failed to update setting: %v", err)
	}

	// Create 3 number fields
	fields := make([]models.CustomFieldDefinition, 3)
	for i := 0; i < 3; i++ {
		fields[i] = createField(t, handler, fmt.Sprintf("Field %d", i), "number")
	}

	// Enable index on first two - should succeed
	for i := 0; i < 2; i++ {
		rr := enableIndex(t, handler, fields[i].ID, "number", true, false)
		rr.AssertStatusCode(http.StatusOK)
	}

	// Third should fail with 400
	rr := enableIndex(t, handler, fields[2].ID, "number", true, false)
	rr.AssertStatusCode(http.StatusBadRequest)

	// Verify error message contains count info
	var errResp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err == nil {
		msg, _ := errResp["error"].(string)
		if msg == "" {
			t.Error("Expected error message about index limit")
		}
	}
}

func TestCustomFieldHandler_DeleteIndexedField(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Indexed Cost", "number")

	// Enable index
	rr := enableIndex(t, handler, cf.ID, "number", true, false)
	rr.AssertStatusCode(http.StatusOK)

	indexName := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assertIndexExists(t, tdb, indexName)

	// Delete the field
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/custom-fields/1", nil)
	deleteReq.SetPathValue("id", testutils.IntToString(cf.ID))
	deleteRR := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)
	deleteRR.AssertStatusCode(http.StatusNoContent)

	// Verify DB index is gone
	assertIndexNotExists(t, tdb, indexName)
}

func TestCustomFieldHandler_IndexOnMultipleTables(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)
	cf := createField(t, handler, "Multi Index", "number")

	// Enable on both tables
	rr := enableIndex(t, handler, cf.ID, "number", true, true)
	rr.AssertStatusCode(http.StatusOK)

	itemsIndex := fmt.Sprintf("idx_cf_items_%d", cf.ID)
	assetsIndex := fmt.Sprintf("idx_cf_assets_%d", cf.ID)

	assertIndexExists(t, tdb, itemsIndex)
	assertIndexExists(t, tdb, assetsIndex)
	assertIndexRecordExists(t, tdb, cf.ID, "items")
	assertIndexRecordExists(t, tdb, cf.ID, "assets")

	// Disable items only
	rr = enableIndex(t, handler, cf.ID, "number", false, true)
	rr.AssertStatusCode(http.StatusOK)

	assertIndexNotExists(t, tdb, itemsIndex)
	assertIndexExists(t, tdb, assetsIndex)
	assertIndexRecordNotExists(t, tdb, cf.ID, "items")
	assertIndexRecordExists(t, tdb, cf.ID, "assets")
}

func TestCustomFieldHandler_GetAll_IncludesIndexInfo(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Delete any system defaults
	_, _ = tdb.Exec("DELETE FROM custom_field_definitions")

	// Create a number field and index it
	cf := createField(t, handler, "Indexed Number", "number")
	rr := enableIndex(t, handler, cf.ID, "number", true, false)
	rr.AssertStatusCode(http.StatusOK)

	// GetAll and verify index info
	getAllReq := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields", nil)
	getAllRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, getAllReq, nil)
	getAllRR.AssertStatusCode(http.StatusOK)

	var resp customFieldsResponse
	getAllRR.AssertJSONResponse(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("Expected 1 field, got %d", len(resp.Data))
	}

	field := resp.Data[0]
	if field.Indexed == nil {
		t.Fatal("Expected indexed info to be present")
	}
	if !field.Indexed.Items {
		t.Error("Expected items index to be true")
	}
	if field.Indexed.Assets {
		t.Error("Expected assets index to be false")
	}

	// Verify index counts
	if resp.IndexCounts["items"].Current != 1 {
		t.Errorf("Expected items index count 1, got %d", resp.IndexCounts["items"].Current)
	}
	if resp.IndexCounts["assets"].Current != 0 {
		t.Errorf("Expected assets index count 0, got %d", resp.IndexCounts["assets"].Current)
	}
	if resp.IndexCounts["items"].Max != 20 {
		t.Errorf("Expected items max 20, got %d", resp.IndexCounts["items"].Max)
	}
}

// --- UpdateSettings ---

func TestCustomFieldHandler_UpdateSettings(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Update limit to 10
	body := map[string]interface{}{
		"max_indexes_per_table": 10,
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/admin/custom-fields/settings", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateSettings, req, nil)
	rr.AssertStatusCode(http.StatusOK)

	var settings customFieldSettings
	rr.AssertJSONResponse(&settings)
	if settings.MaxIndexesPerTable != 10 {
		t.Errorf("Expected max_indexes_per_table 10, got %d", settings.MaxIndexesPerTable)
	}

	// Verify GetAll returns new max
	getAllReq := testutils.CreateJSONRequest(t, "GET", "/api/custom-fields", nil)
	getAllRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, getAllReq, nil)
	getAllRR.AssertStatusCode(http.StatusOK)

	var resp customFieldsResponse
	getAllRR.AssertJSONResponse(&resp)

	if resp.IndexCounts["items"].Max != 10 {
		t.Errorf("Expected items max 10, got %d", resp.IndexCounts["items"].Max)
	}
	if resp.IndexCounts["assets"].Max != 10 {
		t.Errorf("Expected assets max 10, got %d", resp.IndexCounts["assets"].Max)
	}
}

func TestCustomFieldHandler_UpdateSettings_BelowUsage(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	// Create 3 number fields and index them
	for i := 0; i < 3; i++ {
		cf := createField(t, handler, fmt.Sprintf("Field %d", i), "number")
		rr := enableIndex(t, handler, cf.ID, "number", true, false)
		rr.AssertStatusCode(http.StatusOK)
	}

	// Try to set limit to 2 - should fail
	body := map[string]interface{}{
		"max_indexes_per_table": 2,
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/admin/custom-fields/settings", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateSettings, req, nil)
	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestCustomFieldHandler_UpdateSettings_InvalidValue(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := createCustomFieldHandler(t, tdb)

	tests := []struct {
		name  string
		value int
	}{
		{"Zero", 0},
		{"Negative", -5},
		{"Over max", 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{
				"max_indexes_per_table": tt.value,
			}
			req := testutils.CreateJSONRequest(t, "PUT", "/api/admin/custom-fields/settings", body)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateSettings, req, nil)
			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}
