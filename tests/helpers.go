package tests

import (
	"bytes"
	"context"
	"crypto/rand"
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

	"windshift/internal/database"
	"windshift/internal/server"
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

// StartTestServer starts a windshift server with an isolated database
// and returns a TestServer instance with cleanup function.
// This uses an in-process server for faster, more reliable tests.
func StartTestServer(t *testing.T, dbType string) (*TestServer, func()) {
	t.Helper()

	// Generate unique database name
	timestamp := time.Now().UnixNano()
	pid := os.Getpid()

	var dbPath string

	if dbType == "sqlite" {
		// Use temp directory to avoid polluting project root
		tempDir := filepath.Join(os.TempDir(), "windshift-tests")
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			t.Fatalf("Failed to create test temp dir: %v", err)
		}
		dbPath = filepath.Join(tempDir, fmt.Sprintf("test_%d_%d.db", timestamp, pid))
	} else if dbType == "postgres" {
		// PostgreSQL setup would go here
		t.Skip("PostgreSQL testing not yet implemented")
	} else {
		t.Fatalf("Unknown database type: %s", dbType)
	}

	// Set required environment variables for testing
	os.Setenv("SESSION_SECRET", "test-session-secret-for-integration-tests")

	// Create server configuration for testing
	cfg := server.Config{
		Port:          "0", // Use port 0 for OS-assigned free port
		DBPath:        dbPath,
		DisableCSRF:   true,                              // Disable CSRF for testing
		SilentMode:    os.Getenv("TEST_VERBOSE") == "",   // Suppress logs unless TEST_VERBOSE is set
		MaxReadConns:  10,
		MaxWriteConns: 1,
	}

	// Create the in-process server
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}

	// Start the server
	if err := srv.Start(); err != nil {
		srv.Shutdown(context.Background())
		t.Fatalf("Failed to start test server: %v", err)
	}

	port := srv.Port()
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	apiBase := baseURL + "/api"

	ts := &TestServer{
		Port:    port,
		BaseURL: baseURL,
		APIBase: apiBase,
		DBPath:  dbPath,
		DBType:  dbType,
		server:  srv,
	}

	// Cleanup function with graceful shutdown
	cleanup := func() {
		// Ensure we always clean up database files, even if server cleanup fails
		defer func() {
			if dbPath != "" && dbType == "sqlite" {
				// Remove all SQLite database files
				if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
					t.Logf("Warning: Failed to remove database file %s: %v", dbPath, err)
				}
				// Also remove WAL files (ignore errors if they don't exist)
				os.Remove(dbPath + "-shm")
				os.Remove(dbPath + "-wal")
				os.Remove(dbPath + "-journal")
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

// CreateBearerToken completes the full authentication flow and returns a bearer token
func CreateBearerToken(t *testing.T, server *TestServer) string {
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

	// Get CSRF token for setup
	csrfToken1 := getCSRFToken(t, server.APIBase)

	setupResp := makeRequest(t, http.MethodPost, server.APIBase+"/setup/complete", "", setupData, map[string]string{
		"X-CSRF-Token": csrfToken1,
	})

	if setupResp.StatusCode != http.StatusOK && setupResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(setupResp.Body)
		t.Fatalf("Setup failed: %d - %s", setupResp.StatusCode, string(body))
	}
	setupResp.Body.Close()

	// Step 2: Login to get session cookie
	csrfToken2 := getCSRFToken(t, server.APIBase)

	loginData := map[string]string{
		"email_or_username": "admin",
		"password":          "testpass123",
	}

	loginResp := makeRequest(t, http.MethodPost, server.APIBase+"/auth/login", "", loginData, map[string]string{
		"X-CSRF-Token": csrfToken2,
	})

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
	csrfToken3 := getCSRFTokenWithCookie(t, server.APIBase, sessionCookie)

	tokenData := map[string]interface{}{
		"name":        "Test API Token",
		"permissions": []string{"read", "write", "admin"},
	}

	tokenResp := makeRequest(t, http.MethodPost, server.APIBase+"/api-tokens", "", tokenData, map[string]string{
		"X-CSRF-Token": csrfToken3,
		"Cookie":       sessionCookie,
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

	server.BearerToken = tokenResult.Token
	return tokenResult.Token
}

// getCSRFToken fetches a CSRF token from the API
func getCSRFToken(t *testing.T, apiBase string) string {
	t.Helper()

	resp, err := testHTTPClient.Get(apiBase + "/csrf-token")
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		CSRFToken string `json:"csrf_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode CSRF token: %v", err)
	}

	return result.CSRFToken
}

// getCSRFTokenWithCookie fetches a CSRF token with a session cookie
func getCSRFTokenWithCookie(t *testing.T, apiBase, cookie string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, apiBase+"/csrf-token", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Cookie", cookie)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		CSRFToken string `json:"csrf_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode CSRF token: %v", err)
	}

	return result.CSRFToken
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
func MakeAuthRequest(t *testing.T, server *TestServer, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := server.APIBase + endpoint
	return makeRequest(t, method, url, server.BearerToken, body, nil)
}

// MakeAuthRequestRaw makes an authenticated request with a raw string body (for testing invalid JSON)
func MakeAuthRequestRaw(t *testing.T, server *TestServer, method, endpoint string, rawBody string) *http.Response {
	t.Helper()

	url := server.APIBase + endpoint

	req, err := http.NewRequest(method, url, strings.NewReader(rawBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+server.BearerToken)

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
func CreateTestWorkspace(t *testing.T, server *TestServer, name, key string) (int, string) {
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

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", workspaceData)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	workspaceID := ExtractIDFromResponse(t, result)
	workspaceKey, _ := result["key"].(string)

	return workspaceID, workspaceKey
}

// CreateTestCustomField creates a custom field and returns its ID
func CreateTestCustomField(t *testing.T, server *TestServer, name, fieldType, options string) int {
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

	resp := MakeAuthRequest(t, server, http.MethodPost, "/custom-fields", fieldData)
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusCreated)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	return ExtractIDFromResponse(t, result)
}

// CreateTestStatusCategories creates 3 standard status categories and returns their IDs
func CreateTestStatusCategories(t *testing.T, server *TestServer, prefix string) []int {
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

	var categoryIDs []int
	for _, catData := range categories {
		resp := MakeAuthRequest(t, server, http.MethodPost, "/status-categories", catData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		categoryIDs = append(categoryIDs, ExtractIDFromResponse(t, result))
	}

	return categoryIDs
}

// CreateTestStatuses creates 6 standard statuses across 3 categories and returns their IDs
func CreateTestStatuses(t *testing.T, server *TestServer, prefix string, categoryIDs []int) []int {
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
			"name":        fmt.Sprintf("%s Cancelled %d", prefix, timestamp),
			"description": "Cancelled",
			"category_id": categoryIDs[2],
			"is_default":  false,
		},
	}

	var statusIDs []int
	for _, statusData := range statuses {
		resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", statusData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		statusIDs = append(statusIDs, ExtractIDFromResponse(t, result))
	}

	return statusIDs
}

// GetDefaultConfigurationSet retrieves the default configuration set ID
func GetDefaultConfigurationSet(t *testing.T, server *TestServer) int {
	t.Helper()

	resp := MakeAuthRequest(t, server, http.MethodGet, "/configuration-sets", nil)
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
func GetItemTypes(t *testing.T, server *TestServer, configSetID int) map[string]int {
	t.Helper()

	// First try with config set filter
	endpoint := fmt.Sprintf("/item-types?configuration_set_id=%d", configSetID)
	resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
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
		allResp := MakeAuthRequest(t, server, http.MethodGet, "/item-types", nil)
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
	return fmt.Sprintf("%s%d", prefix, mathrand.Intn(10000))
}

// ============================================================================
// Permission Testing Helpers
// ============================================================================

// CreateTestUserWithCredentials creates a user via the API and returns userID, username, and password.
// Requires admin token to be set on the server.
func CreateTestUserWithCredentials(t *testing.T, server *TestServer, username, email string) (int, string, string) {
	t.Helper()

	password := "testpass123"

	userData := map[string]interface{}{
		"email":      email,
		"username":   username,
		"first_name": "Test",
		"last_name":  "User",
		"is_active":  true,
		"password":   password,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/users", userData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create user %s: %d - %s", username, resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)

	userID := ExtractIDFromResponse(t, result)
	return userID, username, password
}

// CreateBearerTokenForUser logs in as the specified user and creates a bearer token.
func CreateBearerTokenForUser(t *testing.T, server *TestServer, username, password string) string {
	t.Helper()

	// Login to get session cookie
	csrfToken := getCSRFToken(t, server.APIBase)

	loginData := map[string]string{
		"email_or_username": username,
		"password":          password,
	}

	loginResp := makeRequest(t, http.MethodPost, server.APIBase+"/auth/login", "", loginData, map[string]string{
		"X-CSRF-Token": csrfToken,
	})

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
	csrfToken2 := getCSRFTokenWithCookie(t, server.APIBase, sessionCookie)

	tokenData := map[string]interface{}{
		"name":        fmt.Sprintf("Test Token for %s", username),
		"permissions": []string{"read", "write", "admin"},
	}

	tokenResp := makeRequest(t, http.MethodPost, server.APIBase+"/api-tokens", "", tokenData, map[string]string{
		"X-CSRF-Token": csrfToken2,
		"Cookie":       sessionCookie,
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
func MakeAuthRequestWithToken(t *testing.T, server *TestServer, token, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := server.APIBase + endpoint
	return makeRequest(t, method, url, token, body, nil)
}

// GetWorkspaceRoles retrieves all workspace roles and returns a map of name -> ID.
func GetWorkspaceRoles(t *testing.T, server *TestServer) map[string]int {
	t.Helper()

	resp := MakeAuthRequest(t, server, http.MethodGet, "/workspace-roles", nil)
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
func GetPermissions(t *testing.T, server *TestServer) map[string]int {
	t.Helper()

	resp := MakeAuthRequest(t, server, http.MethodGet, "/permissions", nil)
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
func AssignWorkspaceRole(t *testing.T, server *TestServer, userID, workspaceID int, roleName string) {
	t.Helper()

	// Get role ID from name
	roles := GetWorkspaceRoles(t, server)
	roleID, ok := roles[roleName]
	if !ok {
		t.Fatalf("Role %s not found. Available roles: %v", roleName, roles)
	}

	assignData := map[string]interface{}{
		"user_id":      userID,
		"workspace_id": workspaceID,
		"role_id":      roleID,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/workspace-roles/assign", assignData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to assign role %s to user %d in workspace %d: %d - %s",
			roleName, userID, workspaceID, resp.StatusCode, string(body))
	}
}

// RevokeWorkspaceRole removes a user's role assignment in a workspace.
func RevokeWorkspaceRole(t *testing.T, server *TestServer, userID, workspaceID, roleID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/users/%d/workspaces/%d/roles/%d", userID, workspaceID, roleID)
	resp := MakeAuthRequest(t, server, http.MethodDelete, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to revoke role %d from user %d in workspace %d: %d - %s",
			roleID, userID, workspaceID, resp.StatusCode, string(body))
	}
}

// GrantGlobalPermission grants a global permission to a user.
func GrantGlobalPermission(t *testing.T, server *TestServer, userID int, permissionKey string) {
	t.Helper()

	// Get permission ID from key
	permissions := GetPermissions(t, server)
	permissionID, ok := permissions[permissionKey]
	if !ok {
		t.Fatalf("Permission %s not found. Available permissions: %v", permissionKey, permissions)
	}

	grantData := map[string]interface{}{
		"user_id":       userID,
		"permission_id": permissionID,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/permissions/global/grant", grantData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to grant permission %s to user %d: %d - %s",
			permissionKey, userID, resp.StatusCode, string(body))
	}
}

// RevokeGlobalPermission removes a global permission from a user.
func RevokeGlobalPermission(t *testing.T, server *TestServer, userID, permissionID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/users/%d/permissions/global/%d", userID, permissionID)
	resp := MakeAuthRequest(t, server, http.MethodDelete, endpoint, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to revoke permission %d from user %d: %d - %s",
			permissionID, userID, resp.StatusCode, string(body))
	}
}

// SetEveryoneRole sets or removes the Everyone role for a workspace.
// Pass nil to remove Everyone access (lock down the workspace).
func SetEveryoneRole(t *testing.T, server *TestServer, workspaceID int, roleID *int) {
	t.Helper()

	endpoint := fmt.Sprintf("/workspaces/%d/everyone-role", workspaceID)
	data := map[string]interface{}{
		"role_id": roleID,
	}

	resp := MakeAuthRequest(t, server, http.MethodPut, endpoint, data)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to set Everyone role for workspace %d: %d - %s",
			workspaceID, resp.StatusCode, string(body))
	}
}

// LockDownWorkspace removes the Everyone role access from a workspace,
// requiring explicit role assignments for access.
func LockDownWorkspace(t *testing.T, server *TestServer, workspaceID int) {
	t.Helper()
	SetEveryoneRole(t, server, workspaceID, nil)
}

// CreateTestItem creates a work item in a workspace and returns its ID.
func CreateTestItem(t *testing.T, server *TestServer, workspaceID int, title string) int {
	t.Helper()

	// Get default configuration set and item type
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)

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

	resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
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
func CreateSCIMToken(t *testing.T, server *TestServer, name string) string {
	t.Helper()

	tokenData := map[string]interface{}{
		"name": name,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/scim-tokens", tokenData)
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
func MakeSCIMRequest(t *testing.T, server *TestServer, scimToken, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := server.BaseURL + endpoint
	return makeRequest(t, method, url, scimToken, body, map[string]string{
		"Content-Type": "application/scim+json",
	})
}

// MakeSCIMRequestNoAuth makes a request to a SCIM endpoint without authentication.
// Used for testing public endpoints like ServiceProviderConfig.
func MakeSCIMRequestNoAuth(t *testing.T, server *TestServer, method, endpoint string) *http.Response {
	t.Helper()

	url := server.BaseURL + endpoint

	req, err := http.NewRequest(method, url, nil)
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
func CreateTestItemWithToken(t *testing.T, server *TestServer, token string, workspaceID int, title string) (*http.Response, int) {
	t.Helper()

	// Get default configuration set and item type (using admin token)
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)

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

	resp := MakeAuthRequestWithToken(t, server, token, http.MethodPost, "/items", itemData)

	if resp.StatusCode == http.StatusCreated {
		var result map[string]interface{}
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(bodyBytes, &result)
		itemID := 0
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
func CreateEmailProvider(t *testing.T, server *TestServer, name string, providerType string) int {
	t.Helper()

	// Generate a slug from the name
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	data := map[string]interface{}{
		"name":       name,
		"slug":       slug,
		"type":       providerType,
		"is_enabled": true,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/email-providers", data)
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
func CreateInboundEmailChannel(t *testing.T, server *TestServer, config EmailChannelConfig) int {
	t.Helper()

	encryption := config.Encryption
	if encryption == "" {
		encryption = "none" // Plain for testing with mock server
	}

	channelConfig := map[string]interface{}{
		"email_provider_id":    config.EmailProviderID,
		"email_workspace_id":   config.WorkspaceID,
		"email_item_type_id":   config.ItemTypeID,
		"email_auth_method":    "basic",
		"email_mailbox":        "INBOX",
		"email_mark_as_read":   true,
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

	resp := MakeAuthRequest(t, server, http.MethodPost, "/channels", data)
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
func TriggerEmailProcessing(t *testing.T, server *TestServer, channelID int) {
	t.Helper()

	endpoint := fmt.Sprintf("/channels/%d/process-emails", channelID)
	resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, nil)
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
func GetItemsByWorkspace(t *testing.T, server *TestServer, workspaceID int) []map[string]interface{} {
	t.Helper()

	endpoint := fmt.Sprintf("/items?workspace_id=%d", workspaceID)
	resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
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
func AssociateWorkspaceWithConfigSet(t *testing.T, server *TestServer, workspaceID, configSetID int) {
	t.Helper()

	data := map[string]interface{}{
		"configuration_set_id": configSetID,
	}

	endpoint := fmt.Sprintf("/workspaces/%d/configuration-sets", workspaceID)
	resp := MakeAuthRequest(t, server, http.MethodPost, endpoint, data)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Failed to associate workspace with config set: %d - %s", resp.StatusCode, string(body))
		// Don't fail - it might already be associated
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
func CreatePortalCustomerWithSession(t *testing.T, server *TestServer, name, email string) (int, string) {
	t.Helper()

	// Create portal customer via admin API
	customerData := map[string]interface{}{
		"name":  name,
		"email": email,
	}

	resp := MakeAuthRequest(t, server, http.MethodPost, "/portal-customers", customerData)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create portal customer %s: %d - %s", name, resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)
	customerID := ExtractIDFromResponse(t, result)

	// Generate a random session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		t.Fatalf("Failed to generate session token: %v", err)
	}
	sessionToken := fmt.Sprintf("%x", tokenBytes)

	// Insert session directly into the database
	db := server.DB()
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
func MakeUnauthenticatedRequest(t *testing.T, server *TestServer, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := server.APIBase + endpoint
	return makeRequest(t, method, url, "", body, nil)
}

// MakePortalRequest makes a request using a portal session token as Bearer auth
func MakePortalRequest(t *testing.T, server *TestServer, portalToken, method, endpoint string, body interface{}) *http.Response {
	t.Helper()

	url := server.APIBase + endpoint
	return makeRequest(t, method, url, portalToken, body, nil)
}

// SetupPortalChannel creates a portal channel with a slug and a request type.
// Returns the portal slug.
func SetupPortalChannel(t *testing.T, server *TestServer, workspaceID int) string {
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

	resp := MakeAuthRequest(t, server, http.MethodPost, "/channels", channelData)
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

	resp2 := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/channels/%d", channelID), updateData)
	defer resp2.Body.Close()
	AssertStatusCode(t, resp2, http.StatusOK)

	// Create a request type for submissions
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
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

	resp3 := MakeAuthRequest(t, server, http.MethodPost, fmt.Sprintf("/channels/%d/request-types", channelID), requestTypeData)
	defer resp3.Body.Close()
	AssertStatusCode(t, resp3, http.StatusCreated)

	return portalSlug
}

// SubmitPortalRequest submits a request through the portal for a specific portal customer.
// Requires portal authentication token from CreatePortalCustomerWithSession.
// Returns the created item ID.
func SubmitPortalRequest(t *testing.T, server *TestServer, portalSlug, portalToken, title string) int {
	t.Helper()

	submissionData := map[string]interface{}{
		"title":       title,
		"description": "Test portal submission",
	}

	endpoint := fmt.Sprintf("/portal/%s/submit", portalSlug)
	submitResp := MakePortalRequest(t, server, portalToken, http.MethodPost, endpoint, submissionData)
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
func GetItemComments(t *testing.T, server *TestServer, itemID int) []map[string]interface{} {
	t.Helper()

	endpoint := fmt.Sprintf("/items/%d/comments", itemID)
	resp := MakeAuthRequest(t, server, http.MethodGet, endpoint, nil)
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
