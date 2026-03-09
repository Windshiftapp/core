//go:build test

package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	"windshift/internal/testutils"
)

// createRecurrenceTestServices returns a RecurrenceScheduler and PermissionService for tests.
func createRecurrenceTestServices(t *testing.T, tdb testutils.TestDB) (*scheduler.RecurrenceScheduler, *services.PermissionService) {
	t.Helper()

	permConfig := services.DefaultPermissionCacheConfig()
	permConfig.WarmupOnStartup = false
	permConfig.TTL = 1 * time.Minute

	permService, err := services.NewPermissionService(tdb.GetDatabase(), permConfig)
	if err != nil {
		t.Fatalf("Failed to create permission service: %v", err)
	}
	t.Cleanup(func() { permService.Close() })

	sched := scheduler.NewRecurrenceScheduler(tdb.GetDatabase())

	return sched, permService
}

// createRecurrenceHandler creates a RecurrenceHandler with test services.
func createRecurrenceHandler(t *testing.T, tdb *testutils.TestDB) *RecurrenceHandler {
	t.Helper()
	sched, permService := createRecurrenceTestServices(t, *tdb)
	return NewRecurrenceHandler(tdb.GetDatabase(), sched, permService)
}

// createTestItemForRecurrence creates a minimal item in the test workspace and returns its ID.
func createTestItemForRecurrence(t *testing.T, tdb *testutils.TestDB, data testutils.TestDataSet) int {
	t.Helper()
	permService, actTracker, notifService := createTestServices(t, *tdb)
	itemHandler := NewItemHandler(tdb.GetDatabase(), permService, actTracker, notifService)

	statusID := data.StatusID
	priorityID := data.PriorityID
	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Recurrence Template Item",
		StatusID:    &statusID,
		PriorityID:  &priorityID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	rr := testutils.ExecuteAuthenticatedRequest(t, itemHandler.Create, req, nil)
	rr.AssertStatusCode(http.StatusCreated)

	var created models.Item
	rr.AssertJSONResponse(&created)
	return created.ID
}

// --- PreviewRRule ---

func TestRecurrenceHandler_PreviewRRule_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "FREQ=WEEKLY;COUNT=5",
		"dtstart": "2025-01-01",
		"count":   5,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/recurrence/preview", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.PreviewRRule, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var resp map[string]interface{}
	rr.AssertJSONResponse(&resp)

	occurrences, ok := resp["occurrences"].([]interface{})
	if !ok {
		t.Fatal("Expected occurrences array in response")
	}
	if len(occurrences) != 5 {
		t.Errorf("Expected 5 occurrences, got %d", len(occurrences))
	}
}

func TestRecurrenceHandler_PreviewRRule_MissingRRule(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "",
		"dtstart": "2025-01-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/recurrence/preview", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.PreviewRRule, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestRecurrenceHandler_PreviewRRule_InvalidRRule(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "NOT_A_VALID_RRULE",
		"dtstart": "2025-01-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/recurrence/preview", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.PreviewRRule, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestRecurrenceHandler_PreviewRRule_InvalidDtStart(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=3",
		"dtstart": "not-a-date",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/recurrence/preview", body)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.PreviewRRule, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// --- CreateRecurrence ---

func TestRecurrenceHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	body := map[string]interface{}{
		"rrule":   "FREQ=WEEKLY;COUNT=10",
		"dtstart": "2025-06-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), body)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var rule models.RecurrenceRule
	rr.AssertJSONResponse(&rule)

	if rule.ID == 0 {
		t.Error("Expected recurrence rule to have an ID")
	}
	if rule.TemplateItemID != itemID {
		t.Errorf("Expected template_item_id %d, got %d", itemID, rule.TemplateItemID)
	}
}

func TestRecurrenceHandler_Create_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=3",
		"dtstart": "2025-01-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items/abc/recurrence", body)
	req.SetPathValue("id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestRecurrenceHandler_Create_ItemNotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	body := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=3",
		"dtstart": "2025-01-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items/99999/recurrence", body)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestRecurrenceHandler_Create_MissingRRule(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	body := map[string]interface{}{
		"rrule":   "",
		"dtstart": "2025-01-01",
	}

	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), body)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestRecurrenceHandler_Create_Conflict(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	body := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=5",
		"dtstart": "2025-01-01",
	}

	// First creation should succeed
	req1 := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), body)
	req1.SetPathValue("id", testutils.IntToString(itemID))
	rr1 := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req1, nil)
	rr1.AssertStatusCode(http.StatusCreated)

	// Second creation should conflict
	req2 := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), body)
	req2.SetPathValue("id", testutils.IntToString(itemID))
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, req2, nil)

	rr2.AssertStatusCode(http.StatusConflict)
}

// --- GetRecurrence ---

func TestRecurrenceHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Create recurrence first
	createBody := map[string]interface{}{
		"rrule":   "FREQ=MONTHLY;COUNT=12",
		"dtstart": "2025-01-01",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	// Get it
	getReq := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/recurrence", itemID), nil)
	getReq.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetRecurrence, getReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var rule models.RecurrenceRule
	rr.AssertJSONResponse(&rule)

	if rule.TemplateItemID != itemID {
		t.Errorf("Expected template_item_id %d, got %d", itemID, rule.TemplateItemID)
	}
}

func TestRecurrenceHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Item exists but no recurrence
	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/recurrence", itemID), nil)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestRecurrenceHandler_Get_ItemNotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)

	req := testutils.CreateJSONRequest(t, "GET", "/api/items/99999/recurrence", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

// --- UpdateRecurrence ---

func TestRecurrenceHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Create recurrence
	createBody := map[string]interface{}{
		"rrule":   "FREQ=WEEKLY;COUNT=10",
		"dtstart": "2025-01-01",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	// Update lead_time_days
	leadTime := 7
	updateBody := map[string]interface{}{
		"lead_time_days": leadTime,
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", fmt.Sprintf("/api/items/%d/recurrence", itemID), updateBody)
	updateReq.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateRecurrence, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var rule models.RecurrenceRule
	rr.AssertJSONResponse(&rule)

	if rule.LeadTimeDays != leadTime {
		t.Errorf("Expected lead_time_days %d, got %d", leadTime, rule.LeadTimeDays)
	}
}

func TestRecurrenceHandler_Update_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Item exists but no recurrence
	body := map[string]interface{}{
		"lead_time_days": 7,
	}
	req := testutils.CreateJSONRequest(t, "PUT", fmt.Sprintf("/api/items/%d/recurrence", itemID), body)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

// --- DeleteRecurrence ---

func TestRecurrenceHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Create recurrence
	createBody := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=5",
		"dtstart": "2025-01-01",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	// Delete
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", fmt.Sprintf("/api/items/%d/recurrence", itemID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteRecurrence, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify it's gone
	getReq := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/recurrence", itemID), nil)
	getReq.SetPathValue("id", testutils.IntToString(itemID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetRecurrence, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestRecurrenceHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Item exists but no recurrence
	req := testutils.CreateJSONRequest(t, "DELETE", fmt.Sprintf("/api/items/%d/recurrence", itemID), nil)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteRecurrence, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

// --- ListInstances ---

func TestRecurrenceHandler_ListInstances_Empty(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createRecurrenceHandler(t, tdb)
	itemID := createTestItemForRecurrence(t, tdb, data)

	// Create recurrence
	createBody := map[string]interface{}{
		"rrule":   "FREQ=DAILY;COUNT=5",
		"dtstart": "2025-06-01",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/recurrence", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateRecurrence, createReq, nil)
	createRR.AssertStatusCode(http.StatusCreated)

	// List instances (should be empty - no generated instances yet)
	listReq := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/recurrence/instances", itemID), nil)
	listReq.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.ListInstances, listReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var resp map[string]interface{}
	rr.AssertJSONResponse(&resp)

	pagination, ok := resp["pagination"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected pagination in response")
	}
	total, ok := pagination["total"].(float64)
	if !ok {
		t.Fatal("Expected total in pagination")
	}
	if int(total) != 0 {
		t.Errorf("Expected 0 instances, got %d", int(total))
	}
}
