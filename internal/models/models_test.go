//go:build test

package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSystemSetting_JSONSerialization(t *testing.T) {
	setting := SystemSetting{
		ID:          1,
		Key:         "test_setting",
		Value:       "test_value",
		ValueType:   "string",
		Description: "Test system setting",
		Category:    "test",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(setting)
	if err != nil {
		t.Fatalf("Failed to marshal SystemSetting: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SystemSetting
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SystemSetting: %v", err)
	}

	// Verify fields
	if unmarshaled.Key != setting.Key {
		t.Errorf("Expected Key %s, got %s", setting.Key, unmarshaled.Key)
	}
	if unmarshaled.Value != setting.Value {
		t.Errorf("Expected Value %s, got %s", setting.Value, unmarshaled.Value)
	}
	if unmarshaled.ValueType != setting.ValueType {
		t.Errorf("Expected ValueType %s, got %s", setting.ValueType, unmarshaled.ValueType)
	}
}

func TestSetupStatus_JSONSerialization(t *testing.T) {
	status := SetupStatus{
		SetupCompleted:        true,
		AdminUserCreated:      true,
		TimeTrackingEnabled:   false,
		TestManagementEnabled: true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal SetupStatus: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SetupStatus
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SetupStatus: %v", err)
	}

	// Verify all boolean fields
	if unmarshaled.SetupCompleted != status.SetupCompleted {
		t.Errorf("Expected SetupCompleted %v, got %v", status.SetupCompleted, unmarshaled.SetupCompleted)
	}
	if unmarshaled.AdminUserCreated != status.AdminUserCreated {
		t.Errorf("Expected AdminUserCreated %v, got %v", status.AdminUserCreated, unmarshaled.AdminUserCreated)
	}
	if unmarshaled.TimeTrackingEnabled != status.TimeTrackingEnabled {
		t.Errorf("Expected TimeTrackingEnabled %v, got %v", status.TimeTrackingEnabled, unmarshaled.TimeTrackingEnabled)
	}
	if unmarshaled.TestManagementEnabled != status.TestManagementEnabled {
		t.Errorf("Expected TestManagementEnabled %v, got %v", status.TestManagementEnabled, unmarshaled.TestManagementEnabled)
	}
}

func TestSetupRequest_JSONSerialization(t *testing.T) {
	setupReq := SetupRequest{
		AdminUser: SetupUser{
			Email:        "admin@example.com",
			Username:     "admin",
			FirstName:    "Admin",
			LastName:     "User",
			PasswordHash: "password123",
		},
		ModuleSettings: ModuleSettings{
			TimeTrackingEnabled:   true,
			TestManagementEnabled: false,
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(setupReq)
	if err != nil {
		t.Fatalf("Failed to marshal SetupRequest: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SetupRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SetupRequest: %v", err)
	}

	// Verify admin user fields
	if unmarshaled.AdminUser.Email != setupReq.AdminUser.Email {
		t.Errorf("Expected Email %s, got %s", setupReq.AdminUser.Email, unmarshaled.AdminUser.Email)
	}
	if unmarshaled.AdminUser.Username != setupReq.AdminUser.Username {
		t.Errorf("Expected Username %s, got %s", setupReq.AdminUser.Username, unmarshaled.AdminUser.Username)
	}
	if unmarshaled.AdminUser.PasswordHash != setupReq.AdminUser.PasswordHash {
		t.Errorf("Expected PasswordHash %s, got %s", setupReq.AdminUser.PasswordHash, unmarshaled.AdminUser.PasswordHash)
	}

	// Verify module settings
	if unmarshaled.ModuleSettings.TimeTrackingEnabled != setupReq.ModuleSettings.TimeTrackingEnabled {
		t.Errorf("Expected TimeTrackingEnabled %v, got %v",
			setupReq.ModuleSettings.TimeTrackingEnabled, unmarshaled.ModuleSettings.TimeTrackingEnabled)
	}
	if unmarshaled.ModuleSettings.TestManagementEnabled != setupReq.ModuleSettings.TestManagementEnabled {
		t.Errorf("Expected TestManagementEnabled %v, got %v",
			setupReq.ModuleSettings.TestManagementEnabled, unmarshaled.ModuleSettings.TestManagementEnabled)
	}
}

func TestSetupUser_JSONSerialization(t *testing.T) {
	setupUser := SetupUser{
		Email:        "test@example.com",
		Username:     "testuser",
		FirstName:    "Test",
		LastName:     "User",
		PasswordHash: "hashedpassword",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(setupUser)
	if err != nil {
		t.Fatalf("Failed to marshal SetupUser: %v", err)
	}

	// Verify PasswordHash is included in JSON (unlike regular User model)
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonMap); err != nil {
		t.Fatalf("Failed to unmarshal JSON to map: %v", err)
	}

	if _, exists := jsonMap["password_hash"]; !exists {
		t.Error("Expected password_hash to be included in JSON")
	}

	// Test JSON unmarshaling
	var unmarshaled SetupUser
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SetupUser: %v", err)
	}

	// Verify all fields
	if unmarshaled.Email != setupUser.Email {
		t.Errorf("Expected Email %s, got %s", setupUser.Email, unmarshaled.Email)
	}
	if unmarshaled.PasswordHash != setupUser.PasswordHash {
		t.Errorf("Expected PasswordHash %s, got %s", setupUser.PasswordHash, unmarshaled.PasswordHash)
	}
}

func TestModuleSettings_JSONSerialization(t *testing.T) {
	settings := ModuleSettings{
		TimeTrackingEnabled:   true,
		TestManagementEnabled: false,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("Failed to marshal ModuleSettings: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled ModuleSettings
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ModuleSettings: %v", err)
	}

	// Verify fields
	if unmarshaled.TimeTrackingEnabled != settings.TimeTrackingEnabled {
		t.Errorf("Expected TimeTrackingEnabled %v, got %v",
			settings.TimeTrackingEnabled, unmarshaled.TimeTrackingEnabled)
	}
	if unmarshaled.TestManagementEnabled != settings.TestManagementEnabled {
		t.Errorf("Expected TestManagementEnabled %v, got %v",
			settings.TestManagementEnabled, unmarshaled.TestManagementEnabled)
	}
}

func TestUser_PasswordHashHidden(t *testing.T) {
	user := User{
		ID:           1,
		Email:        "test@example.com",
		Username:     "testuser",
		FirstName:    "Test",
		LastName:     "User",
		IsActive:     true,
		PasswordHash: "secret-hash",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Failed to marshal User: %v", err)
	}

	// Verify PasswordHash is NOT included in JSON
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonMap); err != nil {
		t.Fatalf("Failed to unmarshal JSON to map: %v", err)
	}

	if _, exists := jsonMap["password_hash"]; exists {
		t.Error("PasswordHash should not be included in JSON for User model")
	}

	// Verify other fields are included
	if jsonMap["email"] != user.Email {
		t.Errorf("Expected email %s in JSON, got %v", user.Email, jsonMap["email"])
	}
	if jsonMap["username"] != user.Username {
		t.Errorf("Expected username %s in JSON, got %v", user.Username, jsonMap["username"])
	}
}

func TestSetupRequest_ValidationScenarios(t *testing.T) {
	tests := []struct {
		name    string
		req     SetupRequest
		isValid bool
	}{
		{
			name: "Valid complete request",
			req: SetupRequest{
				AdminUser: SetupUser{
					Email:        "admin@example.com",
					Username:     "admin",
					FirstName:    "Admin",
					LastName:     "User",
					PasswordHash: "password123",
				},
				ModuleSettings: ModuleSettings{
					TimeTrackingEnabled:   true,
					TestManagementEnabled: true,
				},
			},
			isValid: true,
		},
		{
			name: "Missing admin user fields",
			req: SetupRequest{
				AdminUser: SetupUser{
					Email: "admin@example.com",
					// Missing other required fields
				},
				ModuleSettings: ModuleSettings{
					TimeTrackingEnabled:   true,
					TestManagementEnabled: false,
				},
			},
			isValid: false,
		},
		{
			name: "Module settings only",
			req: SetupRequest{
				ModuleSettings: ModuleSettings{
					TimeTrackingEnabled:   false,
					TestManagementEnabled: true,
				},
				// Missing admin user
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON serialization/deserialization
			jsonData, err := json.Marshal(tt.req)
			if err != nil {
				if tt.isValid {
					t.Fatalf("Failed to marshal valid request: %v", err)
				}
				return
			}

			var unmarshaled SetupRequest
			if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
				if tt.isValid {
					t.Fatalf("Failed to unmarshal valid request: %v", err)
				}
				return
			}

			// Basic validation - check if required fields are present
			if tt.isValid {
				if unmarshaled.AdminUser.Email == "" {
					t.Error("Expected valid admin user email")
				}
				// Add more validation as needed
			}
		})
	}
}

func TestSetupStatus_DefaultValues(t *testing.T) {
	// Test zero values
	var status SetupStatus

	if status.SetupCompleted != false {
		t.Errorf("Expected default SetupCompleted false, got %v", status.SetupCompleted)
	}
	if status.AdminUserCreated != false {
		t.Errorf("Expected default AdminUserCreated false, got %v", status.AdminUserCreated)
	}
	if status.TimeTrackingEnabled != false {
		t.Errorf("Expected default TimeTrackingEnabled false, got %v", status.TimeTrackingEnabled)
	}
	if status.TestManagementEnabled != false {
		t.Errorf("Expected default TestManagementEnabled false, got %v", status.TestManagementEnabled)
	}
}

func TestModuleSettings_DefaultValues(t *testing.T) {
	// Test zero values
	var settings ModuleSettings

	if settings.TimeTrackingEnabled != false {
		t.Errorf("Expected default TimeTrackingEnabled false, got %v", settings.TimeTrackingEnabled)
	}
	if settings.TestManagementEnabled != false {
		t.Errorf("Expected default TestManagementEnabled false, got %v", settings.TestManagementEnabled)
	}
}

func TestJSONFieldNames(t *testing.T) {
	// Test that JSON field names match expected API structure
	setupReq := SetupRequest{
		AdminUser: SetupUser{
			Email:        "admin@example.com",
			Username:     "admin",
			FirstName:    "Admin",
			LastName:     "User",
			PasswordHash: "password123",
		},
		ModuleSettings: ModuleSettings{
			TimeTrackingEnabled:   true,
			TestManagementEnabled: false,
		},
	}

	jsonData, err := json.Marshal(setupReq)
	if err != nil {
		t.Fatalf("Failed to marshal SetupRequest: %v", err)
	}

	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonMap); err != nil {
		t.Fatalf("Failed to unmarshal JSON to map: %v", err)
	}

	// Verify top-level structure
	if _, exists := jsonMap["admin_user"]; !exists {
		t.Error("Expected 'admin_user' field in JSON")
	}
	if _, exists := jsonMap["module_settings"]; !exists {
		t.Error("Expected 'module_settings' field in JSON")
	}

	// Verify admin_user structure
	adminUserMap := jsonMap["admin_user"].(map[string]interface{})
	expectedAdminFields := []string{"email", "username", "first_name", "last_name", "password_hash"}
	for _, field := range expectedAdminFields {
		if _, exists := adminUserMap[field]; !exists {
			t.Errorf("Expected '%s' field in admin_user JSON", field)
		}
	}

	// Verify module_settings structure
	moduleSettingsMap := jsonMap["module_settings"].(map[string]interface{})
	expectedModuleFields := []string{"time_tracking_enabled", "test_management_enabled"}
	for _, field := range expectedModuleFields {
		if _, exists := moduleSettingsMap[field]; !exists {
			t.Errorf("Expected '%s' field in module_settings JSON", field)
		}
	}
}
