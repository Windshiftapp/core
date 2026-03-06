// Package tests provides integration test helpers and utilities.
package tests

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"windshift/internal/database"
	"windshift/internal/server"
	"windshift/internal/services"
)

// testHTTPClient is a shared HTTP client with timeout for all test requests
var testHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// TestServer represents a running test server instance
type TestServer struct {
	Port        int
	BaseURL     string
	APIBase     string
	DBPath      string
	DBType      string
	BearerToken string
	server      *server.Server // in-process server reference
}

// GetDBType returns the database type to use for integration tests.
// It reads from TEST_DB_TYPE env var, defaulting to "sqlite".
func GetDBType() string {
	if dt := os.Getenv("TEST_DB_TYPE"); dt != "" {
		return dt
	}
	return "sqlite"
}

// StartTestServer starts a windshift server with an isolated database
// and returns a TestServer instance with cleanup function.
// This uses an in-process server for faster, more reliable tests.
func StartTestServer(t *testing.T, dbType string) (ts *TestServer, cleanup func()) {
	t.Helper()

	// Generate unique database name
	timestamp := time.Now().UnixNano()
	pid := os.Getpid()

	var dbPath string
	var pgBaseDSN string // used for postgres cleanup

	switch dbType {
	case "sqlite":
		// Use temp directory to avoid polluting project root
		tempDir := filepath.Join(os.TempDir(), "windshift-tests")
		if err := os.MkdirAll(tempDir, 0o750); err != nil {
			t.Fatalf("Failed to create test temp dir: %v", err)
		}
		dbPath = filepath.Join(tempDir, fmt.Sprintf("test_%d_%d.db", timestamp, pid))
	case "postgres":
		pgBaseDSN = os.Getenv("TEST_POSTGRES_DSN")
		if pgBaseDSN == "" {
			pgBaseDSN = "postgresql://windshift_test:windshift_test_password@localhost:15432/postgres?sslmode=disable" //nolint:gosec // G101: test-only fallback DSN for local Postgres
		}

		// Create a unique test database for isolation
		dbName := fmt.Sprintf("windshift_test_%d_%d", timestamp, pid)

		// Connect to default "postgres" DB to create the test database
		adminDB, err := sql.Open("postgres", pgBaseDSN)
		if err != nil {
			t.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer adminDB.Close()

		_, err = adminDB.Exec("CREATE DATABASE " + dbName)
		if err != nil {
			t.Fatalf("Failed to create test database %s: %v", dbName, err)
		}

		// Build connection string pointing to the new test database
		dbPath = strings.Replace(pgBaseDSN, "/postgres?", "/"+dbName+"?", 1)
	default:
		t.Fatalf("Unknown database type: %s", dbType)
	}

	// Set required environment variables for testing
	_ = os.Setenv("SESSION_SECRET", "test-session-secret-for-integration-tests")

	// Create server configuration for testing
	cfg := server.Config{
		Port:          "0",                             // Use port 0 for OS-assigned free port
		DisableCSRF:   true,                            // Disable CSRF for testing
		SilentMode:    os.Getenv("TEST_VERBOSE") == "", // Suppress logs unless TEST_VERBOSE is set
		MaxReadConns:  10,
		MaxWriteConns: 1,
	}

	switch dbType {
	case "sqlite":
		cfg.DBPath = dbPath
	case "postgres":
		cfg.PostgresConn = dbPath
	}

	// Create the in-process server
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}

	// Start the server
	if err := srv.Start(); err != nil {
		_ = srv.Shutdown(context.Background())
		t.Fatalf("Failed to start test server: %v", err)
	}

	port := srv.Port()
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	apiBase := baseURL + "/api"

	// Invalidate the frac-index cache so each test gets a clean state
	services.InvalidateFracIndexCache()

	ts = &TestServer{
		Port:    port,
		BaseURL: baseURL,
		APIBase: apiBase,
		DBPath:  dbPath,
		DBType:  dbType,
		server:  srv,
	}

	// Cleanup function with graceful shutdown
	cleanup = func() {
		// Ensure we always clean up database, even if server cleanup fails
		defer func() {
			switch dbType {
			case "sqlite":
				if dbPath != "" {
					// Remove all SQLite database files
					if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
						t.Logf("Warning: Failed to remove database file %s: %v", dbPath, err)
					}
					// Also remove WAL files (ignore errors if they don't exist)
					_ = os.Remove(dbPath + "-shm")
					_ = os.Remove(dbPath + "-wal")
					_ = os.Remove(dbPath + "-journal")
				}
			case "postgres":
				if pgBaseDSN != "" {
					// Extract database name from the test connection string
					dbName := dbPath[strings.LastIndex(dbPath, "/")+1:]
					if idx := strings.Index(dbName, "?"); idx != -1 {
						dbName = dbName[:idx]
					}
					adminDB, err := sql.Open("postgres", pgBaseDSN)
					if err != nil {
						t.Logf("Warning: Failed to connect for cleanup: %v", err)
						return
					}
					defer adminDB.Close()
					// Terminate existing connections before dropping
					_, _ = adminDB.Exec(fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()", dbName))
					if _, err := adminDB.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
						t.Logf("Warning: Failed to drop test database %s: %v", dbName, err)
					}
				}
			}
		}()

		// Graceful shutdown with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			t.Logf("Warning: Server shutdown error: %v", err)
		}
	}

	// Register cleanup with testing framework
	t.Cleanup(cleanup)

	return ts, cleanup
}

// CreateBearerToken completes the full authentication flow and returns a bearer token.
// Tests run with DisableCSRF: true, so no CSRF headers are needed.
func CreateBearerToken(t *testing.T, testServer *TestServer) string {
	t.Helper()

	// Step 1: Complete initial setup
	setupData := map[string]interface{}{
		"admin_user": map[string]interface{}{
			"email":         "admin@test.com",
			"username":      "admin",
			"password_hash": "testpass123", // Will be hashed server-side
			"first_name":    "Test",
			"last_name":     "Admin",
		},
		"module_settings": map[string]interface{}{
			"time_tracking_enabled":   true,
			"test_management_enabled": true,
		},
	}

	setupResp := makeRequest(t, http.MethodPost, testServer.APIBase+"/setup/complete", "", setupData, nil)

	if setupResp.StatusCode != http.StatusOK && setupResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(setupResp.Body)
		t.Fatalf("Setup failed: %d - %s", setupResp.StatusCode, string(body))
	}
	setupResp.Body.Close()

	// Step 2: Login to get session cookie
	loginData := map[string]string{
		"email_or_username": "admin",
		"password":          "testpass123",
	}

	loginResp := makeRequest(t, http.MethodPost, testServer.APIBase+"/auth/login", "", loginData, nil)

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Login failed: %d - %s", loginResp.StatusCode, string(body))
	}

	// Extract session cookie
	cookies := loginResp.Cookies()
	var sessionCookie string
	for _, cookie := range cookies {
		if cookie.Name == "session" || cookie.Name == "windshift_session" {
			sessionCookie = cookie.String()
			break
		}
	}

	if sessionCookie == "" {
		t.Fatal("No session cookie received from login")
	}
	loginResp.Body.Close()

	// Step 3: Create API bearer token
	tokenData := map[string]interface{}{
		"name":        "Test API Token",
		"permissions": []string{"read", "write", "admin"},
	}

	tokenResp := makeRequest(t, http.MethodPost, testServer.APIBase+"/api-tokens", "", tokenData, map[string]string{
		"Cookie": sessionCookie,
	})

	if tokenResp.StatusCode != http.StatusOK && tokenResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(tokenResp.Body)
		t.Fatalf("Token creation failed: %d - %s", tokenResp.StatusCode, string(body))
	}

	var tokenResult struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		t.Fatalf("Failed to decode token response: %v", err)
	}
	tokenResp.Body.Close()

	if tokenResult.Token == "" {
		t.Fatal("Empty bearer token received")
	}

	testServer.BearerToken = tokenResult.Token
	return tokenResult.Token
}

// makeRequest is a helper for making HTTP requests with optional auth
func makeRequest(t *testing.T, method, url, bearerToken string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add bearer token if provided
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// MakeAuthRequest makes an authenticated request using the server's bearer token
func MakeAuthRequest(t *testing.T, testServer *TestServer, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := testServer.APIBase + endpoint
	return makeRequest(t, method, url, testServer.BearerToken, body, nil)
}

// MakeAuthRequestRaw makes an authenticated request with a raw string body (for testing invalid JSON)
func MakeAuthRequestRaw(t *testing.T, testServer *TestServer, method, endpoint, rawBody string) *http.Response {
	t.Helper()

	url := testServer.APIBase + endpoint

	req, err := http.NewRequest(method, url, strings.NewReader(rawBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testServer.BearerToken)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// AssertStatusCode checks that the response has the expected status code
func AssertStatusCode(t *testing.T, resp *http.Response, expected int) {
	t.Helper()

	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(body))
	}
}

// DecodeJSON decodes a JSON response into the provided interface
func DecodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, v); err != nil {
		t.Fatalf("Failed to decode JSON response: %v\nResponse body: %s", err, string(bodyBytes))
	}
}

// AssertJSONField checks that a JSON response contains a field with expected value
func AssertJSONField(t *testing.T, data map[string]interface{}, field string, expected interface{}) {
	t.Helper()

	actual, ok := data[field]
	if !ok {
		t.Errorf("Field %s not found in response", field)
		return
	}

	if actual != expected {
		t.Errorf("Field %s: expected %v, got %v", field, expected, actual)
	}
}

// ExtractIDFromResponse safely extracts an ID from a JSON response
func ExtractIDFromResponse(t *testing.T, result map[string]interface{}) int {
	t.Helper()

	if id, ok := result["id"].(float64); ok {
		return int(id)
	}
	t.Fatal("ID not found in response")
	return 0
}

// CreateTestWorkspace creates a test workspace and returns its ID and key
func CreateTestWorkspace(t *testing.T, testServer *TestServer, name, key string) (workspaceID int, workspaceKey string) {
	t.Helper()

	// Generate short key if not already present
	if key == "" {
		key = shortKey("TEST")
	}

	workspaceData := map[string]interface{}{
		"name":        name,
		"key":         key,
		"description": fmt.Sprintf("Test workspace: %s", name),
		"active":      true,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/workspaces", workspaceData)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	workspaceID = ExtractIDFromResponse(t, result)
	workspaceKey, _ = result["key"].(string)

	return workspaceID, workspaceKey
}

// CreateTestCustomField creates a custom field and returns its ID
func CreateTestCustomField(t *testing.T, testServer *TestServer, name, fieldType, options string) int {
	t.Helper()

	fieldData := map[string]interface{}{
		"name":        name,
		"field_type":  fieldType,
		"description": fmt.Sprintf("Test field: %s", name),
		"required":    false,
	}

	if options != "" {
		fieldData["options"] = options
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/admin/custom-fields", fieldData)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	return ExtractIDFromResponse(t, result)
}

// CreateTestStatusCategories creates 3 standard status categories and returns their IDs
func CreateTestStatusCategories(t *testing.T, testServer *TestServer, prefix string) []int {
	t.Helper()

	timestamp := time.Now().Unix()
	categories := []map[string]interface{}{
		{
			"name":         fmt.Sprintf("%s To Do %d", prefix, timestamp),
			"color":        "#6b7280",
			"description":  "Pending items",
			"is_default":   false,
			"is_completed": false,
		},
		{
			"name":         fmt.Sprintf("%s In Progress %d", prefix, timestamp),
			"color":        "#3b82f6",
			"description":  "Active items",
			"is_default":   false,
			"is_completed": false,
		},
		{
			"name":         fmt.Sprintf("%s Done %d", prefix, timestamp),
			"color":        "#10b981",
			"description":  "Completed items",
			"is_default":   false,
			"is_completed": true,
		},
	}

	categoryIDs := make([]int, 0, len(categories))
	for _, catData := range categories {
		resp := MakeAuthRequest(t, testServer, http.MethodPost, "/status-categories", catData)

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		categoryIDs = append(categoryIDs, ExtractIDFromResponse(t, result))
		resp.Body.Close()
	}

	return categoryIDs
}

// CreateTestStatuses creates 6 standard statuses across 3 categories and returns their IDs
func CreateTestStatuses(t *testing.T, testServer *TestServer, prefix string, categoryIDs []int) []int {
	t.Helper()

	if len(categoryIDs) != 3 {
		t.Fatalf("CreateTestStatuses requires exactly 3 category IDs, got %d", len(categoryIDs))
	}

	timestamp := time.Now().Unix()
	statuses := []map[string]interface{}{
		{
			"name":        fmt.Sprintf("%s Open %d", prefix, timestamp),
			"description": "New items",
			"category_id": categoryIDs[0],
			"is_default":  false,
		},
		{
			"name":        fmt.Sprintf("%s To Do %d", prefix, timestamp),
			"description": "Ready to start",
			"category_id": categoryIDs[0],
			"is_default":  false,
		},
		{
			"name":        fmt.Sprintf("%s In Progress %d", prefix, timestamp),
			"description": "Being worked on",
			"category_id": categoryIDs[1],
			"is_default":  false,
		},
		{
			"name":        fmt.Sprintf("%s In Review %d", prefix, timestamp),
			"description": "Under review",
			"category_id": categoryIDs[1],
			"is_default":  false,
		},
		{
			"name":        fmt.Sprintf("%s Completed %d", prefix, timestamp),
			"description": "Finished",
			"category_id": categoryIDs[2],
			"is_default":  false,
		},
		{
			"name":        fmt.Sprintf("%s Canceled %d", prefix, timestamp),
			"description": "Canceled",
			"category_id": categoryIDs[2],
			"is_default":  false,
		},
	}

	statusIDs := make([]int, 0, len(statuses))
	for _, statusData := range statuses {
		resp := MakeAuthRequest(t, testServer, http.MethodPost, "/statuses", statusData)

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		statusIDs = append(statusIDs, ExtractIDFromResponse(t, result))
		resp.Body.Close()
	}

	return statusIDs
}

// GetDefaultConfigurationSet retrieves the default configuration set ID
func GetDefaultConfigurationSet(t *testing.T, testServer *TestServer) int {
	t.Helper()

	resp := MakeAuthRequest(t, testServer, http.MethodGet, "/configuration-sets", nil)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusOK)

	// Handle paginated response format: {"configuration_sets": [...], "pagination": {...}}
	var result struct {
		ConfigurationSets []map[string]interface{} `json:"configuration_sets"`
	}
	DecodeJSON(t, resp, &result)

	configSets := result.ConfigurationSets

	// Find the default configuration set
	for _, cs := range configSets {
		if isDefault, ok := cs["is_default"].(bool); ok && isDefault {
			return ExtractIDFromResponse(t, cs)
		}
	}

	// If no default found, use the first one
	if len(configSets) > 0 {
		return ExtractIDFromResponse(t, configSets[0])
	}

	t.Fatal("No configuration set found")
	return 0
}

// GetItemTypes retrieves all item types for a configuration set as a map of name->ID
func GetItemTypes(t *testing.T, testServer *TestServer, configSetID int) map[string]int {
	t.Helper()

	// First try with config set filter
	endpoint := fmt.Sprintf("/item-types?configuration_set_id=%d", configSetID)
	resp := MakeAuthRequest(t, testServer, http.MethodGet, endpoint, nil)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusOK)

	bodyBytes, _ := io.ReadAll(resp.Body)

	var itemTypes []map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &itemTypes); err != nil {
		t.Fatalf("Failed to decode item types: %v\nResponse: %s", err, string(bodyBytes))
	}

	// If no item types found for config set, fall back to all item types
	// This handles the case where item types aren't yet associated with configuration sets
	if len(itemTypes) == 0 {
		allResp := MakeAuthRequest(t, testServer, http.MethodGet, "/item-types", nil)
		allBodyBytes, _ := io.ReadAll(allResp.Body)
		allResp.Body.Close()
		if err := json.Unmarshal(allBodyBytes, &itemTypes); err != nil {
			t.Fatalf("Failed to decode all item types: %v\nResponse: %s", err, string(allBodyBytes))
		}
	}

	itemTypeMap := make(map[string]int)
	for _, it := range itemTypes {
		if name, ok := it["name"].(string); ok {
			if id, ok := it["id"].(float64); ok {
				itemTypeMap[name] = int(id)
			}
		}
	}

	return itemTypeMap
}

// ============================================================================
// Key Generation Helpers
// ============================================================================

// shortKey generates a short workspace key (max 10 chars) with a prefix and random suffix.
func shortKey(prefix string) string {
	// Ensure we have room for at least 4 random digits
	maxPrefixLen := 6
	if len(prefix) > maxPrefixLen {
		prefix = prefix[:maxPrefixLen]
	}
	return fmt.Sprintf("%s%d", prefix, mathrand.Intn(10000)) //nolint:gosec // G404: test helper, crypto random not needed
}

// ============================================================================
// Permission Testing Helpers
// ============================================================================

// CreateTestUserWithCredentials creates a user via the API and returns userID, username, and password.
// Requires admin token to be set on the server.
func CreateTestUserWithCredentials(t *testing.T, testServer *TestServer, username, email string) (userID int, uname, password string) {
	t.Helper()

	password = "testpass123"

	userData := map[string]interface{}{
		"email":      email,
		"username":   username,
		"first_name": "Test",
		"last_name":  "User",
		"is_active":  true,
		"password":   password,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/users", userData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create user %s: %d - %s", username, resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	userID = ExtractIDFromResponse(t, result)
	return userID, username, password
}

// CreateBearerTokenForUser logs in as the specified user and creates a bearer token.
// Tests run with DisableCSRF: true, so no CSRF headers are needed.
func CreateBearerTokenForUser(t *testing.T, testServer *TestServer, username, password string) string {
	t.Helper()

	// Login to get session cookie
	loginData := map[string]string{
		"email_or_username": username,
		"password":          password,
	}

	loginResp := makeRequest(t, http.MethodPost, testServer.APIBase+"/auth/login", "", loginData, nil)

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Login failed for user %s: %d - %s", username, loginResp.StatusCode, string(body))
	}

	// Extract session cookie
	cookies := loginResp.Cookies()
	var sessionCookie string
	for _, cookie := range cookies {
		if cookie.Name == "session" || cookie.Name == "windshift_session" {
			sessionCookie = cookie.String()
			break
		}
	}

	if sessionCookie == "" {
		t.Fatalf("No session cookie received for user %s", username)
	}
	loginResp.Body.Close()

	// Create API bearer token
	tokenData := map[string]interface{}{
		"name":        fmt.Sprintf("Test Token for %s", username),
		"permissions": []string{"read", "write", "admin"},
	}

	tokenResp := makeRequest(t, http.MethodPost, testServer.APIBase+"/api-tokens", "", tokenData, map[string]string{
		"Cookie": sessionCookie,
	})

	if tokenResp.StatusCode != http.StatusOK && tokenResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(tokenResp.Body)
		t.Fatalf("Token creation failed for user %s: %d - %s", username, tokenResp.StatusCode, string(body))
	}

	var tokenResult struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		t.Fatalf("Failed to decode token response: %v", err)
	}
	tokenResp.Body.Close()

	if tokenResult.Token == "" {
		t.Fatalf("Empty bearer token received for user %s", username)
	}

	return tokenResult.Token
}

// MakeAuthRequestWithToken makes an authenticated request using a specific bearer token.
func MakeAuthRequestWithToken(t *testing.T, testServer *TestServer, token, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := testServer.APIBase + endpoint
	return makeRequest(t, method, url, token, body, nil)
}

// GetWorkspaceRoles retrieves all workspace roles and returns a map of name -> ID.
func GetWorkspaceRoles(t *testing.T, testServer *TestServer) map[string]int {
	t.Helper()

	resp := MakeAuthRequest(t, testServer, http.MethodGet, "/workspace-roles", nil)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusOK)

	var roles []map[string]interface{}
	DecodeJSON(t, resp, &roles)

	roleMap := make(map[string]int)
	for _, role := range roles {
		if name, ok := role["name"].(string); ok {
			if id, ok := role["id"].(float64); ok {
				roleMap[name] = int(id)
			}
		}
	}

	return roleMap
}

// GetPermissions retrieves all permissions and returns a map of permission_key -> ID.
// Note: This requires system admin permissions.
func GetPermissions(t *testing.T, testServer *TestServer) map[string]int {
	t.Helper()

	resp := MakeAuthRequest(t, testServer, http.MethodGet, "/permissions", nil)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusOK)

	var permissions []map[string]interface{}
	DecodeJSON(t, resp, &permissions)

	permMap := make(map[string]int)
	for _, perm := range permissions {
		if key, ok := perm["permission_key"].(string); ok {
			if id, ok := perm["id"].(float64); ok {
				permMap[key] = int(id)
			}
		}
	}

	return permMap
}

// AssignWorkspaceRole assigns a role to a user in a workspace.
// roleName should be "Viewer", "Editor", or "Administrator".
func AssignWorkspaceRole(t *testing.T, testServer *TestServer, userID, workspaceID int, roleName string) {
	t.Helper()

	// Get role ID from name
	roles := GetWorkspaceRoles(t, testServer)
	roleID, ok := roles[roleName]
	if !ok {
		t.Fatalf("Role %s not found. Available roles: %v", roleName, roles)
	}

	assignData := map[string]interface{}{
		"user_id":      userID,
		"workspace_id": workspaceID,
		"role_id":      roleID,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/workspace-roles/assign", assignData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to assign role %s to user %d in workspace %d: %d - %s",
			roleName, userID, workspaceID, resp.StatusCode, string(body))
	}
}

// RevokeWorkspaceRole removes a user's role assignment in a workspace.
func RevokeWorkspaceRole(t *testing.T, testServer *TestServer, userID, workspaceID, roleID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/users/%d/workspaces/%d/roles/%d", userID, workspaceID, roleID)
	resp := MakeAuthRequest(t, testServer, http.MethodDelete, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to revoke role %d from user %d in workspace %d: %d - %s",
			roleID, userID, workspaceID, resp.StatusCode, string(body))
	}
}

// GrantGlobalPermission grants a global permission to a user.
func GrantGlobalPermission(t *testing.T, testServer *TestServer, userID int, permissionKey string) {
	t.Helper()

	// Get permission ID from key
	permissions := GetPermissions(t, testServer)
	permissionID, ok := permissions[permissionKey]
	if !ok {
		t.Fatalf("Permission %s not found. Available permissions: %v", permissionKey, permissions)
	}

	grantData := map[string]interface{}{
		"user_id":       userID,
		"permission_id": permissionID,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/permissions/global/grant", grantData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to grant permission %s to user %d: %d - %s",
			permissionKey, userID, resp.StatusCode, string(body))
	}
}

// RevokeGlobalPermission removes a global permission from a user.
func RevokeGlobalPermission(t *testing.T, testServer *TestServer, userID, permissionID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/users/%d/permissions/global/%d", userID, permissionID)
	resp := MakeAuthRequest(t, testServer, http.MethodDelete, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to revoke permission %d from user %d: %d - %s",
			permissionID, userID, resp.StatusCode, string(body))
	}
}

// LockDownWorkspace restricts a workspace so that only explicitly assigned
// users have access. It does this by assigning the Viewer role to the admin
// user (the bearer-token holder), which triggers the "has explicit Viewer
// assignments" condition and blocks implicit everyone access.
func LockDownWorkspace(t *testing.T, testServer *TestServer, workspaceID int) {
	t.Helper()

	// Get the admin user's ID via GET /users (bearer-token compatible)
	resp := MakeAuthRequest(t, testServer, http.MethodGet, "/users", nil)
	defer resp.Body.Close()
	AssertStatusCode(t, resp, http.StatusOK)

	var users []map[string]interface{}
	DecodeJSON(t, resp, &users)
	if len(users) == 0 {
		t.Fatal("No users found")
	}

	// Find admin user (username "admin" from setup)
	var adminID int
	for _, u := range users {
		if u["username"] == "admin" {
			adminID = int(u["id"].(float64))
			break
		}
	}
	if adminID == 0 {
		// Fallback to first user
		adminID = int(users[0]["id"].(float64))
	}

	AssignWorkspaceRole(t, testServer, adminID, workspaceID, "Viewer")
}

// CreateTestItem creates a work item in a workspace and returns its ID.
func CreateTestItem(t *testing.T, testServer *TestServer, workspaceID int, title string) int {
	t.Helper()

	// Get default configuration set and item type
	configSetID := GetDefaultConfigurationSet(t, testServer)
	itemTypes := GetItemTypes(t, testServer, configSetID)

	// Find the first item type (usually "Task" or similar)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	if itemTypeID == 0 {
		t.Fatal("No item types found")
	}

	itemData := map[string]interface{}{
		"title":        title,
		"workspace_id": workspaceID,
		"item_type_id": itemTypeID,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/items", itemData)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	return ExtractIDFromResponse(t, result)
}

// ============================================================================
// SCIM Testing Helpers
// ============================================================================

// CreateSCIMToken creates a SCIM token via the admin API and returns the raw token string.
func CreateSCIMToken(t *testing.T, testServer *TestServer, name string) string {
	t.Helper()

	tokenData := map[string]interface{}{
		"name": name,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/admin/scim-tokens", tokenData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create SCIM token: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	DecodeJSON(t, resp, &result)

	if result.Token == "" {
		t.Fatal("Empty SCIM token received")
	}

	return result.Token
}

// MakeSCIMRequest makes a request to a SCIM endpoint with SCIM token authentication.
// The endpoint should start with /scim/v2/ (e.g., "/scim/v2/Users")
func MakeSCIMRequest(t *testing.T, testServer *TestServer, scimToken, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := testServer.BaseURL + endpoint
	return makeRequest(t, method, url, scimToken, body, map[string]string{
		"Content-Type": "application/scim+json",
	})
}

// MakeSCIMRequestNoAuth makes a request to a SCIM endpoint without authentication.
// Used for testing public endpoints like ServiceProviderConfig.
func MakeSCIMRequestNoAuth(t *testing.T, testServer *TestServer, method, endpoint string) *http.Response {
	t.Helper()

	url := testServer.BaseURL + endpoint

	req, err := http.NewRequest(method, url, http.NoBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// CreateTestItemWithToken creates a work item using a specific bearer token.
func CreateTestItemWithToken(t *testing.T, testServer *TestServer, token string, workspaceID int, title string) (resp *http.Response, itemID int) {
	t.Helper()

	// Get default configuration set and item type (using admin token)
	configSetID := GetDefaultConfigurationSet(t, testServer)
	itemTypes := GetItemTypes(t, testServer, configSetID)

	// Find the first item type
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	if itemTypeID == 0 {
		t.Fatal("No item types found")
	}

	itemData := map[string]interface{}{
		"title":        title,
		"workspace_id": workspaceID,
		"item_type_id": itemTypeID,
	}

	resp = MakeAuthRequestWithToken(t, testServer, token, http.MethodPost, "/items", itemData)

	if resp.StatusCode == http.StatusCreated {
		var result map[string]interface{}
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		_ = json.Unmarshal(bodyBytes, &result)
		if id, ok := result["id"].(float64); ok {
			itemID = int(id)
		}
		// Recreate response for caller to check
		resp = &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		return resp, itemID
	}

	return resp, 0
}

// EmailChannelConfig contains configuration for creating an email channel
type EmailChannelConfig struct {
	Name              string
	WorkspaceID       int
	ItemTypeID        int
	EmailProviderID   int
	IMAPHost          string
	IMAPPort          int
	Username          string
	Password          string
	Encryption        string // "ssl", "tls", "starttls", "none"
	DefaultPriorityID *int
}

// CreateEmailProvider creates an email provider for testing
func CreateEmailProvider(t *testing.T, testServer *TestServer, name, providerType string) int {
	t.Helper()

	// Generate a slug from the name
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	data := map[string]interface{}{
		"name":       name,
		"slug":       slug,
		"type":       providerType,
		"is_enabled": true,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/admin/email-providers", data)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create email provider: %d - %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse email provider response: %v", err)
	}

	if id, ok := result["id"].(float64); ok {
		return int(id)
	}
	t.Fatal("No ID returned for email provider")
	return 0
}

// CreateInboundEmailChannel creates an inbound email channel for testing
func CreateInboundEmailChannel(t *testing.T, testServer *TestServer, config EmailChannelConfig) int {
	t.Helper()

	encryption := config.Encryption
	if encryption == "" {
		encryption = "none" // Plain for testing with mock server
	}

	channelConfig := map[string]interface{}{
		"email_provider_id":  config.EmailProviderID,
		"email_workspace_id": config.WorkspaceID,
		"email_item_type_id": config.ItemTypeID,
		"email_auth_method":  "basic",
		"email_mailbox":      "INBOX",
		"email_mark_as_read": true,
		// IMAP settings for generic provider
		"imap_host":       config.IMAPHost,
		"imap_port":       config.IMAPPort,
		"imap_username":   config.Username,
		"imap_password":   config.Password,
		"imap_encryption": encryption,
	}

	if config.DefaultPriorityID != nil {
		channelConfig["email_default_priority_id"] = *config.DefaultPriorityID
	}

	// Marshal the config to JSON string since Channel.Config is a string
	configJSON, err := json.Marshal(channelConfig)
	if err != nil {
		t.Fatalf("Failed to marshal channel config: %v", err)
	}

	data := map[string]interface{}{
		"name":        config.Name,
		"type":        "email",
		"direction":   "inbound",
		"description": "Test inbound email channel",
		"status":      "enabled",
		"config":      string(configJSON),
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/channels", data)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create email channel: %d - %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse email channel response: %v", err)
	}

	if id, ok := result["id"].(float64); ok {
		t.Logf("Created email channel ID: %d", int(id))
		return int(id)
	}
	t.Fatal("No ID returned for email channel")
	return 0
}

// TriggerEmailProcessing triggers immediate email processing for a channel
func TriggerEmailProcessing(t *testing.T, testServer *TestServer, channelID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/channels/%d/process-emails", channelID)
	resp := MakeAuthRequest(t, testServer, http.MethodPost, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Email processing trigger response: %d - %s", resp.StatusCode, string(body))
		// Don't fail - the channel might not process any emails and that's OK for some tests
	} else {
		t.Log("Email processing triggered successfully")
	}
}

// GetItemsByWorkspace returns items in a workspace
func GetItemsByWorkspace(t *testing.T, testServer *TestServer, workspaceID int) []map[string]interface{} {
	t.Helper()

	endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
	resp := MakeAuthRequest(t, testServer, http.MethodGet, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to get items: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse items response: %v", err)
	}

	return result.Items
}

// AssociateWorkspaceWithConfigSet associates a workspace with a configuration set
func AssociateWorkspaceWithConfigSet(t *testing.T, testServer *TestServer, workspaceID, configSetID int) {
	t.Helper()

	// First, get the current configuration set
	getEndpoint := fmt.Sprintf("/configuration-sets/%d", configSetID)
	getResp := MakeAuthRequest(t, testServer, http.MethodGet, getEndpoint, nil)
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("Failed to get config set %d: %d - %s", configSetID, getResp.StatusCode, string(body))
	}

	var configSet map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&configSet); err != nil {
		t.Fatalf("Failed to decode config set: %v", err)
	}

	// Add workspace to WorkspaceIDs if not already present
	workspaceIDs := []int{}
	if ids, ok := configSet["workspace_ids"].([]interface{}); ok {
		for _, id := range ids {
			if idFloat, ok := id.(float64); ok {
				workspaceIDs = append(workspaceIDs, int(idFloat))
			}
		}
	}

	// Check if already associated
	for _, id := range workspaceIDs {
		if id == workspaceID {
			t.Logf("Workspace %d already associated with configuration set %d", workspaceID, configSetID)
			return
		}
	}
	workspaceIDs = append(workspaceIDs, workspaceID)
	configSet["workspace_ids"] = workspaceIDs

	// Update the configuration set with skip_migration_check
	updateEndpoint := fmt.Sprintf("/configuration-sets/%d?skip_migration_check=true", configSetID)
	updateResp := MakeAuthRequest(t, testServer, http.MethodPut, updateEndpoint, configSet)
	defer updateResp.Body.Close()

	if updateResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(updateResp.Body)
		t.Logf("Failed to associate workspace with config set: %d - %s", updateResp.StatusCode, string(body))
	} else {
		t.Logf("Associated workspace %d with configuration set %d", workspaceID, configSetID)
	}
}

// DB returns the underlying database for direct DB operations in tests
func (ts *TestServer) DB() database.Database {
	return ts.server.DB()
}

// CreatePortalCustomerWithSession creates a portal customer via the admin API
// and inserts a session directly into the database. Returns customerID and raw session token.
func CreatePortalCustomerWithSession(t *testing.T, testServer *TestServer, name, email string) (customerID int, sessionToken string) {
	t.Helper()

	// Create portal customer via admin API
	customerData := map[string]interface{}{
		"name":  name,
		"email": email,
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/portal-customers", customerData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create portal customer %s: %d - %s", name, resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)
	customerID = ExtractIDFromResponse(t, result)

	// Generate a random session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		t.Fatalf("Failed to generate session token: %v", err)
	}
	sessionToken = fmt.Sprintf("%x", tokenBytes)

	// Insert session directly into the database
	db := testServer.DB()
	_, err := db.ExecWrite(
		`INSERT INTO portal_customer_sessions (portal_customer_id, session_token, expires_at, ip_address, user_agent, is_active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		customerID, sessionToken, time.Now().Add(7*24*time.Hour), "127.0.0.1", "test-agent", true, time.Now(),
	)
	if err != nil {
		t.Fatalf("Failed to insert portal session: %v", err)
	}

	return customerID, sessionToken
}

// MakeUnauthenticatedRequest makes a request with no authentication
func MakeUnauthenticatedRequest(t *testing.T, testServer *TestServer, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := testServer.APIBase + endpoint
	return makeRequest(t, method, url, "", body, nil)
}

// MakePortalRequest makes a request using a portal session token as Bearer auth
func MakePortalRequest(t *testing.T, testServer *TestServer, portalToken, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := testServer.APIBase + endpoint
	return makeRequest(t, method, url, portalToken, body, nil)
}

// SetupPortalChannel creates a portal channel with a slug and a request type.
// Returns the portal slug.
func SetupPortalChannel(t *testing.T, testServer *TestServer, workspaceID int) string {
	t.Helper()

	timestamp := time.Now().UnixNano()
	portalSlug := fmt.Sprintf("test-portal-%d", timestamp)

	// Create the channel
	channelData := map[string]interface{}{
		"name":        fmt.Sprintf("Test Portal %d", timestamp),
		"type":        "portal",
		"direction":   "inbound",
		"description": "Portal for boundary testing",
		"status":      "active",
	}

	resp := MakeAuthRequest(t, testServer, http.MethodPost, "/channels", channelData)
	defer resp.Body.Close()
	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)
	channelID := ExtractIDFromResponse(t, result)

	// Configure the portal with slug and workspace
	configJSON, _ := json.Marshal(map[string]interface{}{
		"portal_slug":          portalSlug,
		"portal_enabled":       true,
		"portal_title":         "Test Portal",
		"portal_description":   "Test portal for boundary tests",
		"portal_workspace_ids": []int{workspaceID},
	})

	updateData := map[string]interface{}{
		"name":   fmt.Sprintf("Test Portal %d", timestamp),
		"type":   "portal",
		"status": "active",
		"config": string(configJSON),
	}

	resp2 := MakeAuthRequest(t, testServer, http.MethodPut, fmt.Sprintf("/channels/%d", channelID), updateData)
	defer resp2.Body.Close()
	AssertStatusCode(t, resp2, http.StatusOK)

	// Create a request type for submissions
	configSetID := GetDefaultConfigurationSet(t, testServer)
	itemTypes := GetItemTypes(t, testServer, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	requestTypeData := map[string]interface{}{
		"name":         "General Request",
		"description":  "General request type",
		"item_type_id": itemTypeID,
		"icon":         "Circle",
		"color":        "#666666",
		"is_active":    true,
	}

	resp3 := MakeAuthRequest(t, testServer, http.MethodPost, fmt.Sprintf("/channels/%d/request-types", channelID), requestTypeData)
	defer resp3.Body.Close()
	AssertStatusCode(t, resp3, http.StatusCreated)

	return portalSlug
}

// SubmitPortalRequest submits a request through the portal for a specific portal customer.
// Requires portal authentication token from CreatePortalCustomerWithSession.
// Returns the created item ID.
func SubmitPortalRequest(t *testing.T, testServer *TestServer, portalSlug, portalToken, title string) int {
	t.Helper()

	submissionData := map[string]interface{}{
		"title":       title,
		"description": "Test portal submission",
	}

	endpoint := fmt.Sprintf("/portal/%s/submit", portalSlug)
	submitResp := MakePortalRequest(t, testServer, portalToken, http.MethodPost, endpoint, submissionData)
	defer submitResp.Body.Close()

	AssertStatusCode(t, submitResp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, submitResp, &result)

	if itemID, ok := result["item_id"].(float64); ok {
		return int(itemID)
	}
	t.Fatal("No item_id in portal submission response")
	return 0
}

// GetItemComments returns comments for an item
func GetItemComments(t *testing.T, testServer *TestServer, itemID int) []map[string]interface{} {
	t.Helper()

	endpoint := fmt.Sprintf("/items/%d/comments", itemID)
	resp := MakeAuthRequest(t, testServer, http.MethodGet, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to get comments: %d - %s", resp.StatusCode, string(body))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse comments response: %v", err)
	}

	return result
}
