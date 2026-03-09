//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestNotificationSettingsHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	setting := models.NotificationSetting{
		Name:        "Dev Team Notifications",
		Description: "Standard notifications for dev workspaces",
		IsActive:    true,
		CreatedBy:   data.UserID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.NotificationSetting
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected notification setting to have an ID")
	}
	if response.Name != setting.Name {
		t.Errorf("Expected name %q, got %q", setting.Name, response.Name)
	}
	if response.Description != setting.Description {
		t.Errorf("Expected description %q, got %q", setting.Description, response.Description)
	}
	if !response.IsActive {
		t.Error("Expected is_active to be true")
	}
}

func TestNotificationSettingsHandler_Create_WithEventRules(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	setting := models.NotificationSetting{
		Name:      "Notifications with Rules",
		IsActive:  true,
		CreatedBy: data.UserID,
		EventRules: []models.NotificationEventRule{
			{
				EventType:      "item.created",
				IsEnabled:      true,
				NotifyAssignee: true,
				NotifyCreator:  false,
				NotifyWatchers: true,
			},
			{
				EventType:             "comment.created",
				IsEnabled:             true,
				NotifyAssignee:        true,
				NotifyCreator:         true,
				NotifyWorkspaceAdmins: true,
			},
		},
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.NotificationSetting
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected notification setting to have an ID")
	}

	// Verify event rules were created by fetching the setting
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/"+testutils.IntToString(response.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(response.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSetting, getReq, nil)

	getRR.AssertStatusCode(http.StatusOK)

	var fetched models.NotificationSetting
	getRR.AssertJSONResponse(&fetched)

	if len(fetched.EventRules) != 2 {
		t.Errorf("Expected 2 event rules, got %d", len(fetched.EventRules))
	}
}

func TestNotificationSettingsHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	tests := []struct {
		name        string
		setting     models.NotificationSetting
		expectedErr string
	}{
		{
			name:        "Missing name",
			setting:     models.NotificationSetting{CreatedBy: 1},
			expectedErr: "Name is required",
		},
		{
			name:        "Missing created_by",
			setting:     models.NotificationSetting{Name: "Test"},
			expectedErr: "CreatedBy is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", tt.setting)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestNotificationSettingsHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Count pre-existing settings from DB initialization
	baseReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings", nil)
	baseRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSettings, baseReq, nil)
	var baseSettings []models.NotificationSetting
	baseRR.AssertJSONResponse(&baseSettings)
	baseCount := len(baseSettings)

	// Create multiple settings
	for i := 0; i < 3; i++ {
		setting := models.NotificationSetting{
			Name:      "Custom Setting " + testutils.IntToString(i+1),
			IsActive:  true,
			CreatedBy: data.UserID,
		}
		req := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSettings, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var settings []models.NotificationSetting
	rr.AssertJSONResponse(&settings)

	expected := baseCount + 3
	if len(settings) != expected {
		t.Errorf("Expected %d settings, got %d", expected, len(settings))
	}
}

func TestNotificationSettingsHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Create a setting
	setting := models.NotificationSetting{
		Name:        "Test Setting",
		Description: "A test notification setting",
		IsActive:    true,
		CreatedBy:   data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, createReq, nil)

	var created models.NotificationSetting
	createRR.AssertJSONResponse(&created)

	// Get the setting
	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSetting, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.NotificationSetting
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != setting.Name {
		t.Errorf("Expected name %q, got %q", setting.Name, response.Name)
	}
}

func TestNotificationSettingsHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSetting, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationSettingsHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Create a setting
	setting := models.NotificationSetting{
		Name:      "Original Name",
		IsActive:  true,
		CreatedBy: data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, createReq, nil)

	var created models.NotificationSetting
	createRR.AssertJSONResponse(&created)

	// Update the setting
	updated := models.NotificationSetting{
		Name:        "Updated Name",
		Description: "Now with description",
		IsActive:    false,
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/notification-settings/"+testutils.IntToString(created.ID), updated)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateNotificationSetting, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.NotificationSetting
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %q", response.Name)
	}
}

func TestNotificationSettingsHandler_Update_ReplacesEventRules(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Create setting with one rule
	setting := models.NotificationSetting{
		Name:      "Setting with Rules",
		IsActive:  true,
		CreatedBy: data.UserID,
		EventRules: []models.NotificationEventRule{
			{EventType: "item.created", IsEnabled: true, NotifyAssignee: true},
		},
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, createReq, nil)

	var created models.NotificationSetting
	createRR.AssertJSONResponse(&created)

	// Update with different rules (should replace, not append)
	updated := models.NotificationSetting{
		Name:     "Setting with Rules",
		IsActive: true,
		EventRules: []models.NotificationEventRule{
			{EventType: "comment.created", IsEnabled: true, NotifyCreator: true},
			{EventType: "item.assigned", IsEnabled: true, NotifyAssignee: true},
		},
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/notification-settings/"+testutils.IntToString(created.ID), updated)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	testutils.ExecuteAuthenticatedRequest(t, handler.UpdateNotificationSetting, updateReq, nil)

	// Fetch and verify rules were replaced
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSetting, getReq, nil)

	var fetched models.NotificationSetting
	getRR.AssertJSONResponse(&fetched)

	if len(fetched.EventRules) != 2 {
		t.Errorf("Expected 2 event rules after update, got %d", len(fetched.EventRules))
	}
}

func TestNotificationSettingsHandler_Update_ValidationError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Create a setting
	setting := models.NotificationSetting{
		Name:      "Test",
		IsActive:  true,
		CreatedBy: data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, createReq, nil)

	var created models.NotificationSetting
	createRR.AssertJSONResponse(&created)

	// Update with empty name
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/notification-settings/"+testutils.IntToString(created.ID),
		models.NotificationSetting{Name: ""})
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateNotificationSetting, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Name is required") {
		t.Errorf("Expected 'Name is required', got %q", rr.Body.String())
	}
}

func TestNotificationSettingsHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	// Create a setting
	setting := models.NotificationSetting{
		Name:      "To Delete",
		IsActive:  true,
		CreatedBy: data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-settings", setting)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotificationSetting, createReq, nil)

	var created models.NotificationSetting
	createRR.AssertJSONResponse(&created)

	// Delete the setting
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/notification-settings/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteNotificationSetting, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify it's gone
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotificationSetting, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationSettingsHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/notification-settings/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteNotificationSetting, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationSettingsHandler_GetAvailableEvents(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-settings/available-events", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAvailableEvents, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	// Should return a non-empty list
	body := strings.TrimSpace(rr.Body.String())
	if body == "null" || body == "[]" {
		t.Error("Expected non-empty list of available events")
	}
}

func TestNotificationSettingsHandler_InvalidID_Scenarios(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationSettingsHandler(tdb.GetDatabase())

	tests := []struct {
		name    string
		method  string
		handler testutils.TestHandler
	}{
		{"Get invalid ID", "GET", handler.GetNotificationSetting},
		{"Update invalid ID", "PUT", handler.UpdateNotificationSetting},
		{"Delete invalid ID", "DELETE", handler.DeleteNotificationSetting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, tt.method, "/api/notification-settings/abc",
				models.NotificationSetting{Name: "test", CreatedBy: 1})
			req.SetPathValue("id", "abc")
			rr := testutils.ExecuteAuthenticatedRequest(t, tt.handler, req, nil)
			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}
