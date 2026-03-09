//go:build test

package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

// insertTestConfigSet creates a configuration set and returns its ID
func insertTestConfigSet(t *testing.T, tdb *testutils.TestDB, name string) int {
	var id int
	err := tdb.GetDatabase().QueryRow(
		`INSERT INTO configuration_sets (name, description, is_default, created_at, updated_at)
		 VALUES (?, 'Test config set', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert test config set: %v", err)
	}
	return id
}

// insertTestNotificationSetting creates a notification setting and returns its ID.
// Uses the seeded user ID (1) as created_by to satisfy the users JOIN in GetAvailableNotificationSettings.
func insertTestNotificationSetting(t *testing.T, tdb *testutils.TestDB, name string, isActive bool) int {
	// Use NULL for created_by since the GetAvailableNotificationSettings query does a LEFT JOIN on users
	var id int
	err := tdb.GetDatabase().QueryRow(
		`INSERT INTO notification_settings (name, description, is_active, created_by, created_at, updated_at)
		 VALUES (?, 'Test notification setting', ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		name, isActive,
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert test notification setting: %v", err)
	}
	return id
}

// --- GetConfigurationSetNotifications ---

func TestConfigSetNotificationHandler_GetNotifications_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	configSetID := insertTestConfigSet(t, tdb, "Test Config Set")
	settingID := insertTestNotificationSetting(t, tdb, "Test Setting", true)

	// Assign the setting
	_, err := tdb.GetDatabase().Exec(
		`INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)`,
		configSetID, settingID,
	)
	if err != nil {
		t.Fatalf("Failed to assign setting: %v", err)
	}

	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), nil)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetConfigurationSetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var assignments []models.ConfigurationSetNotificationSetting
	rr.AssertJSONResponse(&assignments)

	if len(assignments) != 1 {
		t.Errorf("Expected 1 assignment, got %d", len(assignments))
	}
	if len(assignments) > 0 && assignments[0].NotificationSettingName != "Test Setting" {
		t.Errorf("Expected setting name 'Test Setting', got %q", assignments[0].NotificationSettingName)
	}
}

func TestConfigSetNotificationHandler_GetNotifications_Empty(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Empty Config Set")

	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), nil)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetConfigurationSetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	body := rr.Body.String()
	if body != "null\n" && body != "[]\n" {
		// Handler returns null for empty slice
		var assignments []models.ConfigurationSetNotificationSetting
		rr.AssertJSONResponse(&assignments)
		if len(assignments) != 0 {
			t.Errorf("Expected 0 assignments, got %d", len(assignments))
		}
	}
}

func TestConfigSetNotificationHandler_GetNotifications_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/abc/notifications", nil)
	req.SetPathValue("config_set_id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetConfigurationSetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// --- AssignNotificationToConfigurationSet ---

func TestConfigSetNotificationHandler_AssignNotification_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	configSetID := insertTestConfigSet(t, tdb, "Assign Config Set")
	settingID := insertTestNotificationSetting(t, tdb, "Active Setting", true)

	body := map[string]int{"notification_setting_id": settingID}
	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var assignment models.ConfigurationSetNotificationSetting
	rr.AssertJSONResponse(&assignment)

	if assignment.ID == 0 {
		t.Error("Expected assignment to have an ID")
	}
	if assignment.ConfigurationSetID != configSetID {
		t.Errorf("Expected config set ID %d, got %d", configSetID, assignment.ConfigurationSetID)
	}
	if assignment.NotificationSettingID != settingID {
		t.Errorf("Expected setting ID %d, got %d", settingID, assignment.NotificationSettingID)
	}
	if assignment.ConfigurationSetName != "Assign Config Set" {
		t.Errorf("Expected config set name 'Assign Config Set', got %q", assignment.ConfigurationSetName)
	}
	if assignment.NotificationSettingName != "Active Setting" {
		t.Errorf("Expected setting name 'Active Setting', got %q", assignment.NotificationSettingName)
	}
}

func TestConfigSetNotificationHandler_AssignNotification_ConfigSetNotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	settingID := insertTestNotificationSetting(t, tdb, "Orphan Setting", true)

	body := map[string]int{"notification_setting_id": settingID}
	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets/99999/notifications", body)
	req.SetPathValue("config_set_id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestConfigSetNotificationHandler_AssignNotification_SettingNotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "No Setting Config")

	body := map[string]int{"notification_setting_id": 99999}
	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestConfigSetNotificationHandler_AssignNotification_InactiveSetting(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Inactive Test Config")
	settingID := insertTestNotificationSetting(t, tdb, "Inactive Setting", false)

	body := map[string]int{"notification_setting_id": settingID}
	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	rr.AssertBodyContains("inactive")
}

func TestConfigSetNotificationHandler_AssignNotification_Duplicate(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Dup Config Set")
	settingID := insertTestNotificationSetting(t, tdb, "Dup Setting", true)

	body := map[string]int{"notification_setting_id": settingID}

	// First assignment succeeds
	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)
	rr.AssertStatusCode(http.StatusCreated)

	// Duplicate assignment returns conflict
	req2 := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req2.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req2, nil)
	rr2.AssertStatusCode(http.StatusConflict)
}

func TestConfigSetNotificationHandler_AssignNotification_MissingSettingID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Missing ID Config")

	// Send body with missing/zero notification_setting_id
	body := map[string]int{"notification_setting_id": 0}
	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	rr.AssertBodyContains("notification_setting_id is required")
}

func TestConfigSetNotificationHandler_AssignNotification_InvalidJSON(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Bad JSON Config")

	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID),
		strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	req = testutils.WithAuthContext(req, nil)

	rr := testutils.ExecuteRequest(t, handler.AssignNotificationToConfigurationSet, req)
	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestConfigSetNotificationHandler_AssignNotification_InvalidConfigSetID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	body := map[string]int{"notification_setting_id": 1}
	req := testutils.CreateJSONRequest(t, "POST", "/api/configuration-sets/abc/notifications", body)
	req.SetPathValue("config_set_id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// --- UnassignNotificationFromConfigurationSet ---

func TestConfigSetNotificationHandler_UnassignNotification_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Unassign Config")
	settingID := insertTestNotificationSetting(t, tdb, "Unassign Setting", true)

	// Assign first
	body := map[string]int{"notification_setting_id": settingID}
	assignReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), body)
	assignReq.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	assignRR := testutils.ExecuteAuthenticatedRequest(t, handler.AssignNotificationToConfigurationSet, assignReq, nil)

	var assignment models.ConfigurationSetNotificationSetting
	assignRR.AssertJSONResponse(&assignment)

	// Unassign
	unassignReq := testutils.CreateJSONRequest(t, "DELETE",
		fmt.Sprintf("/api/configuration-sets/%d/notifications/%d", configSetID, assignment.ID), nil)
	unassignReq.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	unassignReq.SetPathValue("assignment_id", testutils.IntToString(assignment.ID))
	unassignRR := testutils.ExecuteAuthenticatedRequest(t, handler.UnassignNotificationFromConfigurationSet, unassignReq, nil)

	unassignRR.AssertStatusCode(http.StatusNoContent)

	// Verify assignment is gone
	getReq := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/configuration-sets/%d/notifications", configSetID), nil)
	getReq.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetConfigurationSetNotifications, getReq, nil)

	getRR.AssertStatusCode(http.StatusOK)
	body2 := getRR.Body.String()
	if body2 != "null\n" {
		var remaining []models.ConfigurationSetNotificationSetting
		getRR.AssertJSONResponse(&remaining)
		if len(remaining) != 0 {
			t.Errorf("Expected 0 assignments after unassign, got %d", len(remaining))
		}
	}
}

func TestConfigSetNotificationHandler_UnassignNotification_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "NotFound Config")

	req := testutils.CreateJSONRequest(t, "DELETE",
		fmt.Sprintf("/api/configuration-sets/%d/notifications/99999", configSetID), nil)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	req.SetPathValue("assignment_id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UnassignNotificationFromConfigurationSet, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestConfigSetNotificationHandler_UnassignNotification_InvalidIDs(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	// Invalid config_set_id
	req := testutils.CreateJSONRequest(t, "DELETE", "/api/configuration-sets/abc/notifications/1", nil)
	req.SetPathValue("config_set_id", "abc")
	req.SetPathValue("assignment_id", "1")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UnassignNotificationFromConfigurationSet, req, nil)
	rr.AssertStatusCode(http.StatusBadRequest)

	// Invalid assignment_id
	req2 := testutils.CreateJSONRequest(t, "DELETE", "/api/configuration-sets/1/notifications/xyz", nil)
	req2.SetPathValue("config_set_id", "1")
	req2.SetPathValue("assignment_id", "xyz")
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.UnassignNotificationFromConfigurationSet, req2, nil)
	rr2.AssertStatusCode(http.StatusBadRequest)
}

// --- GetAvailableNotificationSettings ---

func TestConfigSetNotificationHandler_GetAvailableSettings_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Available Config")

	// Create two active settings
	setting1ID := insertTestNotificationSetting(t, tdb, "Available Setting 1", true)
	insertTestNotificationSetting(t, tdb, "Available Setting 2", true)

	// Assign one of them
	_, err := tdb.GetDatabase().Exec(
		`INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)`,
		configSetID, setting1ID,
	)
	if err != nil {
		t.Fatalf("Failed to assign setting: %v", err)
	}

	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/configuration-sets/%d/notifications/available", configSetID), nil)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAvailableNotificationSettings, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var settings []models.NotificationSetting
	rr.AssertJSONResponse(&settings)

	// Should contain "Available Setting 2" but not "Available Setting 1" (already assigned)
	// Also may include the default seed setting if not already assigned to this config set
	foundSetting2 := false
	for _, s := range settings {
		if s.Name == "Available Setting 1" {
			t.Error("Available settings should not include already-assigned setting")
		}
		if s.Name == "Available Setting 2" {
			foundSetting2 = true
		}
	}
	if !foundSetting2 {
		t.Error("Expected 'Available Setting 2' in available settings")
	}
}

func TestConfigSetNotificationHandler_GetAvailableSettings_ExcludesInactive(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())
	configSetID := insertTestConfigSet(t, tdb, "Inactive Filter Config")

	// Create one inactive setting
	insertTestNotificationSetting(t, tdb, "Inactive Setting", false)
	// Create one active setting
	insertTestNotificationSetting(t, tdb, "Active Available", true)

	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/configuration-sets/%d/notifications/available", configSetID), nil)
	req.SetPathValue("config_set_id", testutils.IntToString(configSetID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAvailableNotificationSettings, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var settings []models.NotificationSetting
	rr.AssertJSONResponse(&settings)

	for _, s := range settings {
		if s.Name == "Inactive Setting" {
			t.Error("Inactive settings should not appear in available settings")
		}
	}
}

func TestConfigSetNotificationHandler_GetAvailableSettings_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := NewConfigurationSetNotificationHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/configuration-sets/abc/notifications/available", nil)
	req.SetPathValue("config_set_id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAvailableNotificationSettings, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}
