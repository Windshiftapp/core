//go:build test

package handlers

import (
	"net/http"
	"strconv"
	"testing"
	"windshift/internal/models"
	"windshift/internal/testutils"
	"windshift/internal/testutils/mocks"

	"golang.org/x/crypto/bcrypt"
)

func TestSetupHandler_GetSetupStatus(t *testing.T) {
	tests := []struct {
		name           string
		setupCompleted bool
		adminExists    bool
		timeEnabled    bool
		testEnabled    bool
	}{
		{
			name:           "Fresh database - setup not completed",
			setupCompleted: false,
			adminExists:    false,
			timeEnabled:    true,
			testEnabled:    true,
		},
		{
			name:           "Setup completed with custom modules",
			setupCompleted: true,
			adminExists:    true,
			timeEnabled:    false,
			testEnabled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test database
			tdb := testutils.CreateTestDB(t, true)
			defer tdb.Close()

			// Set up test data based on test case
			if tt.setupCompleted {
				tdb.Exec("UPDATE system_settings SET value = ? WHERE key = ?", 
					strconv.FormatBool(tt.setupCompleted), "setup_completed")
			}
			if tt.adminExists {
				tdb.Exec("UPDATE system_settings SET value = ? WHERE key = ?", 
					strconv.FormatBool(tt.adminExists), "admin_user_created")
			}
			tdb.Exec("UPDATE system_settings SET value = ? WHERE key = ?", 
				strconv.FormatBool(tt.timeEnabled), "time_tracking_enabled")
			tdb.Exec("UPDATE system_settings SET value = ? WHERE key = ?", 
				strconv.FormatBool(tt.testEnabled), "test_management_enabled")

			// Create handler
			mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

			// Create request
			req := testutils.CreateJSONRequest(t, "GET", "/api/setup/status", nil)

			// Execute request
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetSetupStatus, req, nil)

			// Verify response
			rr.AssertStatusCode(http.StatusOK).
				AssertContentType("application/json")

			var response models.SetupStatus
			rr.AssertJSONResponse(&response)

			// Verify status values
			if response.SetupCompleted != tt.setupCompleted {
				t.Errorf("Expected SetupCompleted %v, got %v", tt.setupCompleted, response.SetupCompleted)
			}
			if response.AdminUserCreated != tt.adminExists {
				t.Errorf("Expected AdminUserCreated %v, got %v", tt.adminExists, response.AdminUserCreated)
			}
			if response.TimeTrackingEnabled != tt.timeEnabled {
				t.Errorf("Expected TimeTrackingEnabled %v, got %v", tt.timeEnabled, response.TimeTrackingEnabled)
			}
			if response.TestManagementEnabled != tt.testEnabled {
				t.Errorf("Expected TestManagementEnabled %v, got %v", tt.testEnabled, response.TestManagementEnabled)
			}
		})
	}
}

func TestSetupHandler_CompleteInitialSetup_Success(t *testing.T) {
	// Create fresh database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create valid setup request
	setupReq := models.SetupRequest{
		AdminUser: models.SetupUser{
			Email:        "admin@example.com",
			Username:     "admin",
			FirstName:    "Admin",
			LastName:     "User",
			PasswordHash: "password123",
		},
		ModuleSettings: models.ModuleSettings{
			TimeTrackingEnabled:   false,
			TestManagementEnabled: true,
		},
	}

	// Create request
	req := testutils.CreateJSONRequest(t, "POST", "/api/setup/complete", setupReq)

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CompleteInitialSetup, req, nil)

	// Verify response
	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response map[string]interface{}
	rr.AssertJSONResponse(&response)

	// Verify response structure
	if response["success"] != true {
		t.Errorf("Expected success true, got %v", response["success"])
	}
	if response["message"] == nil {
		t.Error("Expected message in response")
	}

	// Verify database state
	// Check admin user was created with system.admin permission
	var userCount int
	err := tdb.QueryRow(`
		SELECT COUNT(DISTINCT ugp.user_id) FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE p.permission_key = 'system.admin'
	`).Scan(&userCount)
	if err != nil {
		t.Fatalf("Failed to query users with system.admin permission: %v", err)
	}
	if userCount != 1 {
		t.Errorf("Expected 1 user with system.admin permission, got %d", userCount)
	}

	// Verify password was hashed
	var storedHash string
	err = tdb.QueryRow("SELECT password_hash FROM users WHERE email = ?", setupReq.AdminUser.Email).Scan(&storedHash)
	if err != nil {
		t.Fatalf("Failed to query user password: %v", err)
	}
	
	// Verify password hash is bcrypt hash and not plain text
	if storedHash == setupReq.AdminUser.PasswordHash {
		t.Error("Password was not hashed")
	}
	
	// Verify password can be verified
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(setupReq.AdminUser.PasswordHash)); err != nil {
		t.Errorf("Password hash verification failed: %v", err)
	}

	// Check system settings were updated
	var setupCompleted string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'setup_completed'").Scan(&setupCompleted)
	if err != nil {
		t.Fatalf("Failed to query setup_completed: %v", err)
	}
	if setupCompleted != "true" {
		t.Errorf("Expected setup_completed to be 'true', got '%s'", setupCompleted)
	}

	// Check module settings
	// Note: time_tracking is always enabled by the handler regardless of request
	var timeEnabled string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'time_tracking_enabled'").Scan(&timeEnabled)
	if err != nil {
		t.Fatalf("Failed to query time_tracking_enabled: %v", err)
	}
	if timeEnabled != "true" {
		t.Errorf("Expected time_tracking_enabled to be 'true' (always enabled), got '%s'", timeEnabled)
	}

	var testEnabled string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'test_management_enabled'").Scan(&testEnabled)
	if err != nil {
		t.Fatalf("Failed to query test_management_enabled: %v", err)
	}
	if testEnabled != "true" {
		t.Errorf("Expected test_management_enabled to be 'true', got '%s'", testEnabled)
	}
}

func TestSetupHandler_CompleteInitialSetup_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		setupReq    models.SetupRequest
		expectedErr string
	}{
		{
			name: "Missing email",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Username:     "admin",
					FirstName:    "Admin",
					LastName:     "User",
					PasswordHash: "password123",
				},
			},
			expectedErr: "admin email is required",
		},
		{
			name: "Invalid email format",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Email:        "invalid-email",
					Username:     "admin",
					FirstName:    "Admin",
					LastName:     "User",
					PasswordHash: "password123",
				},
			},
			expectedErr: "invalid email format",
		},
		{
			name: "Missing username",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Email:        "admin@example.com",
					FirstName:    "Admin",
					LastName:     "User",
					PasswordHash: "password123",
				},
			},
			expectedErr: "admin username is required",
		},
		{
			name: "Missing first name",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Email:        "admin@example.com",
					Username:     "admin",
					LastName:     "User",
					PasswordHash: "password123",
				},
			},
			expectedErr: "admin first name is required",
		},
		{
			name: "Missing last name",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Email:        "admin@example.com",
					Username:     "admin",
					FirstName:    "Admin",
					PasswordHash: "password123",
				},
			},
			expectedErr: "admin last name is required",
		},
		{
			name: "Missing password",
			setupReq: models.SetupRequest{
				AdminUser: models.SetupUser{
					Email:     "admin@example.com",
					Username:  "admin",
					FirstName: "Admin",
					LastName:  "User",
				},
			},
			expectedErr: "admin password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test database
			tdb := testutils.CreateTestDB(t, true)
			defer tdb.Close()

			// Create handler
			mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

			// Create request
			req := testutils.CreateJSONRequest(t, "POST", "/api/setup/complete", tt.setupReq)

			// Execute request
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.CompleteInitialSetup, req, nil)

			// Verify error response
			testutils.AssertValidationError(t, rr, tt.expectedErr)
		})
	}
}

func TestSetupHandler_CompleteInitialSetup_AlreadyCompleted(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Mark setup as already completed
	_, err := tdb.Exec("UPDATE system_settings SET value = 'true' WHERE key = 'setup_completed'")
	if err != nil {
		t.Fatalf("Failed to mark setup as completed: %v", err)
	}

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create setup request
	setupReq := models.SetupRequest{
		AdminUser: models.SetupUser{
			Email:        "admin@example.com",
			Username:     "admin",
			FirstName:    "Admin",
			LastName:     "User",
			PasswordHash: "password123",
		},
	}

	// Create request
	req := testutils.CreateJSONRequest(t, "POST", "/api/setup/complete", setupReq)

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CompleteInitialSetup, req, nil)

	// Verify error response
	testutils.AssertValidationError(t, rr, "Setup has already been completed")
}

func TestSetupHandler_GetModuleSettings(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Set custom module settings
	tdb.Exec("UPDATE system_settings SET value = 'false' WHERE key = 'time_tracking_enabled'")
	tdb.Exec("UPDATE system_settings SET value = 'true' WHERE key = 'test_management_enabled'")

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create request
	req := testutils.CreateJSONRequest(t, "GET", "/api/setup/modules", nil)

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetModuleSettings, req, nil)

	// Verify response
	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.ModuleSettings
	rr.AssertJSONResponse(&response)

	// Verify settings
	if response.TimeTrackingEnabled != false {
		t.Errorf("Expected TimeTrackingEnabled false, got %v", response.TimeTrackingEnabled)
	}
	if response.TestManagementEnabled != true {
		t.Errorf("Expected TestManagementEnabled true, got %v", response.TestManagementEnabled)
	}
}

func TestSetupHandler_UpdateModuleSettings(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create update request
	updateReq := models.ModuleSettings{
		TimeTrackingEnabled:   true,
		TestManagementEnabled: false,
	}

	// Create request
	req := testutils.CreateJSONRequest(t, "PUT", "/api/setup/modules", updateReq)

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateModuleSettings, req, nil)

	// Verify response
	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response map[string]interface{}
	rr.AssertJSONResponse(&response)

	// Verify response structure
	if response["success"] != true {
		t.Errorf("Expected success true, got %v", response["success"])
	}

	// Verify database was updated
	var timeEnabled string
	err := tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'time_tracking_enabled'").Scan(&timeEnabled)
	if err != nil {
		t.Fatalf("Failed to query time_tracking_enabled: %v", err)
	}
	if timeEnabled != "true" {
		t.Errorf("Expected time_tracking_enabled to be 'true', got '%s'", timeEnabled)
	}

	var testEnabled string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'test_management_enabled'").Scan(&testEnabled)
	if err != nil {
		t.Fatalf("Failed to query test_management_enabled: %v", err)
	}
	if testEnabled != "false" {
		t.Errorf("Expected test_management_enabled to be 'false', got '%s'", testEnabled)
	}
}

func TestSetupHandler_TransactionRollback(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create a setup request that will cause a database error after user creation
	// We'll simulate this by creating a user with a duplicate email first
	_, err := tdb.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active)
		VALUES ('admin@example.com', 'existing', 'Existing', 'User', 'hash', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to create existing user: %v", err)
	}

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create setup request with duplicate email
	setupReq := models.SetupRequest{
		AdminUser: models.SetupUser{
			Email:        "admin@example.com", // This will cause a conflict
			Username:     "admin",
			FirstName:    "Admin",
			LastName:     "User",
			PasswordHash: "password123",
		},
		ModuleSettings: models.ModuleSettings{
			TimeTrackingEnabled:   false,
			TestManagementEnabled: true,
		},
	}

	// Create request
	req := testutils.CreateJSONRequest(t, "POST", "/api/setup/complete", setupReq)

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CompleteInitialSetup, req, nil)

	// Should get an internal server error due to constraint violation
	testutils.AssertInternalServerError(t, rr)

	// Verify setup_completed is still false (transaction rolled back)
	var setupCompleted string
	err = tdb.QueryRow("SELECT value FROM system_settings WHERE key = 'setup_completed'").Scan(&setupCompleted)
	if err != nil {
		t.Fatalf("Failed to query setup_completed: %v", err)
	}
	if setupCompleted != "false" {
		t.Errorf("Expected setup_completed to remain 'false' after rollback, got '%s'", setupCompleted)
	}

	// Verify only the original user exists (not the failed admin user)
	var userCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		t.Fatalf("Failed to count users: %v", err)
	}
	if userCount != 1 {
		t.Errorf("Expected 1 user (original), got %d", userCount)
	}
}

func TestSetupHandler_InvalidJSON(t *testing.T) {
	// Create test database
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create handler
	mockAuthMiddleware := mocks.CreateMockAuthMiddleware()
	handler := NewSetupHandler(tdb.GetDatabase(), mocks.CreateMockSessionManager(), mockAuthMiddleware)

	// Create request with invalid JSON
	req, _ := testutils.MockHTTPRequest("POST", "/api/setup/complete", nil)
	req.Body = http.NoBody

	// Execute request
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CompleteInitialSetup, req, nil)

	// Should get bad request error
	testutils.AssertValidationError(t, rr, "Invalid request body")
}