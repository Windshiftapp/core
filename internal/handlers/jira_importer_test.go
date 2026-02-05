//go:build test

package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/jira"
	"windshift/internal/testutils"
)

// ================================================================
// Mock Jira Client
// ================================================================

// mockJiraClient implements jira.Client for testing.
// Populate IssueKeysByJQL, Issues, and Emails before calling import.
type mockJiraClient struct {
	IssueKeysByJQL           map[string][]string                         // JQL -> issue keys
	Issues                   map[string]jira.JiraIssue                   // key -> issue
	Emails                   map[string]string                           // accountID -> email
	ProjectIssueTypeStatuses map[string][]jira.JiraIssueTypeWithStatuses // projectKey -> issue type statuses
}

func newMockJiraClient() *mockJiraClient {
	return &mockJiraClient{
		IssueKeysByJQL:           make(map[string][]string),
		Issues:                   make(map[string]jira.JiraIssue),
		Emails:                   make(map[string]string),
		ProjectIssueTypeStatuses: make(map[string][]jira.JiraIssueTypeWithStatuses),
	}
}

// --- Import methods (real behavior) ---

func (m *mockJiraClient) GetAllIssueKeys(_ context.Context, jql string) ([]string, error) {
	return m.IssueKeysByJQL[jql], nil
}

func (m *mockJiraClient) BulkFetchIssues(_ context.Context, req jira.BulkFetchRequest) (*jira.BulkFetchResponse, error) {
	var issues []jira.JiraIssue
	for _, key := range req.IssueIdsOrKeys {
		if issue, ok := m.Issues[key]; ok {
			issues = append(issues, issue)
		}
	}
	return &jira.BulkFetchResponse{Issues: issues}, nil
}

func (m *mockJiraClient) GetUserEmail(_ context.Context, accountID string) (string, error) {
	return m.Emails[accountID], nil
}

// --- Stubs (not used during import) ---

func (m *mockJiraClient) TestConnection(context.Context) (*jira.JiraInstanceInfo, error) {
	return &jira.JiraInstanceInfo{}, nil
}
func (m *mockJiraClient) ListProjects(context.Context) ([]jira.JiraProject, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProject(context.Context, string) (*jira.JiraProject, error) {
	return nil, nil
}
func (m *mockJiraClient) ListIssueTypes(context.Context) ([]jira.JiraIssueType, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProjectIssueTypes(context.Context, string) ([]jira.JiraIssueType, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProjectIssueTypeStatuses(_ context.Context, projectKey string) ([]jira.JiraIssueTypeWithStatuses, error) {
	return m.ProjectIssueTypeStatuses[projectKey], nil
}
func (m *mockJiraClient) ListCustomFields(context.Context) ([]jira.JiraCustomField, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProjectFields(context.Context, []string) ([]jira.JiraCustomField, error) {
	return nil, nil
}
func (m *mockJiraClient) ListStatuses(context.Context) ([]jira.JiraStatus, error) {
	return nil, nil
}
func (m *mockJiraClient) GetStatusCategories(context.Context) ([]jira.JiraStatusCategory, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProjectWorkflowScheme(context.Context, string) (*jira.JiraWorkflow, error) {
	return nil, nil
}
func (m *mockJiraClient) SearchIssues(context.Context, jira.SearchOptions) (*jira.SearchResult, error) {
	return nil, nil
}
func (m *mockJiraClient) GetIssue(context.Context, string, []string) (*jira.JiraIssue, error) {
	return nil, nil
}
func (m *mockJiraClient) GetIssueCount(context.Context, string, bool) (int, error) {
	return 0, nil
}
func (m *mockJiraClient) SearchIssuesJQL(context.Context, jira.JQLSearchRequest) (*jira.JQLSearchResponse, error) {
	return nil, nil
}
func (m *mockJiraClient) GetProjectVersions(context.Context, string) ([]jira.JiraVersion, error) {
	return nil, nil
}
func (m *mockJiraClient) ListBoards(context.Context, string) (*jira.BoardListResult, error) {
	return nil, nil
}
func (m *mockJiraClient) GetBoardSprints(context.Context, int) (*jira.SprintListResult, error) {
	return nil, nil
}
func (m *mockJiraClient) DownloadAttachment(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader("mock attachment content")), "application/octet-stream", nil
}
func (m *mockJiraClient) ListObjectSchemas(context.Context) ([]jira.AssetObjectSchema, error) {
	return nil, nil
}
func (m *mockJiraClient) GetObjectSchema(context.Context, string) (*jira.AssetObjectSchema, error) {
	return nil, nil
}
func (m *mockJiraClient) ListObjectTypes(context.Context, string) ([]jira.AssetObjectType, error) {
	return nil, nil
}
func (m *mockJiraClient) GetObjectTypeAttributes(context.Context, string) ([]jira.AssetObjectAttribute, error) {
	return nil, nil
}
func (m *mockJiraClient) SearchObjects(context.Context, jira.ObjectSearchOptions) (*jira.ObjectSearchResult, error) {
	return nil, nil
}
func (m *mockJiraClient) GetObjectCount(context.Context, string) (int, error) {
	return 0, nil
}

// ================================================================
// Test Helpers
// ================================================================

func newTestJiraImportHandler(db database.Database) *JiraImportHandler {
	return &JiraImportHandler{db: db}
}

func setupTestJob(t *testing.T, tdb *testutils.TestDB, jobID string) {
	t.Helper()

	// Insert a fake connection row
	_, err := tdb.Exec(`
		INSERT INTO jira_import_connections (id, instance_url, email, encrypted_credentials, instance_name)
		VALUES ('conn-1', 'https://test.atlassian.net', 'test@example.com', 'encrypted', 'Test Instance')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test connection: %v", err)
	}

	// Insert the job row
	_, err = tdb.Exec(`
		INSERT INTO jira_import_jobs (id, connection_id, status, scope, config_json)
		VALUES (?, 'conn-1', 'running', 'work_items', '{}')
	`, jobID)
	if err != nil {
		t.Fatalf("Failed to insert test job: %v", err)
	}
}

// ================================================================
// Tests: ensureItemTypes
// ================================================================

func TestEnsureItemTypes_MultipleJiraIDs(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-itemtypes"
	setupTestJob(t, tdb, jobID)

	mappings := []IssueTypeMapping{
		{
			JiraIDs:        []string{"10001", "10002"},
			JiraName:       "Story",
			CreateNew:      true,
			HierarchyLevel: 0,
		},
	}

	result, err := handler.ensureItemTypes(context.Background(), jobID, mappings)
	if err != nil {
		t.Fatalf("ensureItemTypes failed: %v", err)
	}

	// Both Jira IDs should map to the same Windshift item type
	id1, ok1 := result["10001"]
	id2, ok2 := result["10002"]
	if !ok1 || !ok2 {
		t.Fatalf("Expected both IDs to be mapped, got ok1=%v ok2=%v", ok1, ok2)
	}
	if id1 != id2 {
		t.Errorf("Expected same Windshift ID for both Jira IDs, got %d and %d", id1, id2)
	}
	if id1 == 0 {
		t.Error("Expected non-zero Windshift item type ID")
	}
}

func TestEnsureItemTypes_MapToExisting(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-itemtypes-existing"
	setupTestJob(t, tdb, jobID)

	// Use an existing item type created by DB initialization
	var existingID int
	err := tdb.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&existingID)
	if err != nil {
		t.Fatalf("Failed to get existing item type: %v", err)
	}

	mappings := []IssueTypeMapping{
		{
			JiraIDs:     []string{"10010", "10011"},
			JiraName:    "Bug",
			CreateNew:   false,
			WindshiftID: &existingID,
		},
	}

	result, err := handler.ensureItemTypes(context.Background(), jobID, mappings)
	if err != nil {
		t.Fatalf("ensureItemTypes failed: %v", err)
	}

	for _, jiraID := range []string{"10010", "10011"} {
		if got, ok := result[jiraID]; !ok {
			t.Errorf("Jira ID %s not mapped", jiraID)
		} else if got != existingID {
			t.Errorf("Jira ID %s: expected Windshift ID %d, got %d", jiraID, existingID, got)
		}
	}
}

// ================================================================
// Tests: ensureStatuses
// ================================================================

func TestEnsureStatuses_MultipleJiraIDs(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-statuses"
	setupTestJob(t, tdb, jobID)

	mappings := []StatusMapping{
		{
			JiraIDs:     []string{"1", "4"},
			JiraName:    "Open",
			CategoryKey: "new",
			CreateNew:   true,
		},
	}

	result, err := handler.ensureStatuses(context.Background(), jobID, mappings)
	if err != nil {
		t.Fatalf("ensureStatuses failed: %v", err)
	}

	id1, ok1 := result["1"]
	id4, ok4 := result["4"]
	if !ok1 || !ok4 {
		t.Fatalf("Expected both IDs to be mapped, got ok1=%v ok4=%v", ok1, ok4)
	}
	if id1 != id4 {
		t.Errorf("Expected same Windshift ID for both Jira IDs, got %d and %d", id1, id4)
	}
	if id1 == 0 {
		t.Error("Expected non-zero Windshift status ID")
	}
}

func TestEnsureStatuses_MapToExisting(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-statuses-existing"
	setupTestJob(t, tdb, jobID)

	// Get default status ID (created by DB init)
	var existingID int
	err := tdb.QueryRow(`SELECT id FROM statuses WHERE is_default = true LIMIT 1`).Scan(&existingID)
	if err != nil {
		t.Fatalf("Failed to get default status: %v", err)
	}

	mappings := []StatusMapping{
		{
			JiraIDs:     []string{"100", "200"},
			JiraName:    "Whatever",
			CategoryKey: "new",
			CreateNew:   false,
			WindshiftID: &existingID,
		},
	}

	result, err := handler.ensureStatuses(context.Background(), jobID, mappings)
	if err != nil {
		t.Fatalf("ensureStatuses failed: %v", err)
	}

	for _, jiraID := range []string{"100", "200"} {
		if got, ok := result[jiraID]; !ok {
			t.Errorf("Jira ID %s not mapped", jiraID)
		} else if got != existingID {
			t.Errorf("Jira ID %s: expected Windshift ID %d, got %d", jiraID, existingID, got)
		}
	}
}

// ================================================================
// Tests: importIssue (status and type mapping)
// ================================================================

func TestImportIssue_StatusAndTypeMapping(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-issue"
	setupTestJob(t, tdb, jobID)

	// Create status and item type mappings
	statusMap := map[string]int{"10001": 1}
	itemTypeMap := map[string]int{"10100": 1}
	userMap := map[string]int{}
	versionMap := map[string]int{}

	// Get actual IDs from the database for a valid item type
	var itemTypeID int
	err := tdb.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&itemTypeID)
	if err != nil {
		// Create one if none exist
		var res sql.Result
		res, err = tdb.Exec(`INSERT INTO item_types (name, icon, color, hierarchy_level) VALUES ('Task', 'Task', '#000', 0)`)
		if err != nil {
			t.Fatalf("Failed to create item type: %v", err)
		}
		id, _ := res.LastInsertId()
		itemTypeID = int(id)
	}

	var statusID int
	err = tdb.QueryRow(`SELECT id FROM statuses LIMIT 1`).Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	statusMap["10001"] = statusID
	itemTypeMap["10100"] = itemTypeID

	issue := jira.JiraIssue{
		ID:  "12345",
		Key: "TEST-1",
		Fields: jira.JiraIssueFields{
			Summary: "Test issue",
			Status: &jira.JiraStatus{
				ID:   "10001",
				Name: "Open",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10100",
				Name: "Task",
			},
		},
	}

	mockClient := newMockJiraClient()
	progress := &ImportProgress{}
	err = handler.importIssue(context.Background(), jobID, 1, &issue, statusMap, itemTypeMap, userMap, versionMap, nil, mockClient, progress)
	if err != nil {
		t.Fatalf("importIssue failed: %v", err)
	}

	// Verify the item was created with correct mappings
	var title string
	var dbStatusID, dbItemTypeID *int
	err = tdb.QueryRow(`
		SELECT title, status_id, item_type_id FROM items WHERE workspace_id = 1 ORDER BY id DESC LIMIT 1
	`).Scan(&title, &dbStatusID, &dbItemTypeID)
	if err != nil {
		t.Fatalf("Failed to query created item: %v", err)
	}

	if title != "Test issue" {
		t.Errorf("Expected title 'Test issue', got '%s'", title)
	}
	if dbStatusID == nil || *dbStatusID != statusID {
		t.Errorf("Expected status_id %d, got %v", statusID, dbStatusID)
	}
	if dbItemTypeID == nil || *dbItemTypeID != itemTypeID {
		t.Errorf("Expected item_type_id %d, got %v", itemTypeID, dbItemTypeID)
	}
}

// ================================================================
// Tests: executeImportWithClient (end-to-end)
// ================================================================

func TestExecuteImportWithClient_EndToEnd(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-e2e"
	setupTestJob(t, tdb, jobID)

	// Set up mock client with test data
	client := newMockJiraClient()
	client.IssueKeysByJQL["project = PROJ ORDER BY created ASC"] = []string{"PROJ-1", "PROJ-2"}
	client.Issues["PROJ-1"] = jira.JiraIssue{
		ID:  "1001",
		Key: "PROJ-1",
		Fields: jira.JiraIssueFields{
			Summary: "First issue",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10000",
				Name: "Task",
			},
		},
	}
	client.Issues["PROJ-2"] = jira.JiraIssue{
		ID:  "1002",
		Key: "PROJ-2",
		Fields: jira.JiraIssueFields{
			Summary: "Second issue",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10000",
				Name: "Task",
			},
		},
	}

	// Use existing workspace from SeedTestData
	existingWSID := 1
	req := StartImportRequest{
		ConnectionID: "conn-1",
		ProjectKeys:  []string{"PROJ"},
		Mappings: ImportMappings{
			Workspaces: []WorkspaceMapping{
				{
					JiraKey:     "PROJ",
					JiraName:    "Test Project",
					IssueCount:  2,
					CreateNew:   false,
					WindshiftID: &existingWSID,
				},
			},
			Statuses: []StatusMapping{
				{
					JiraIDs:     []string{"1"},
					JiraName:    "To Do",
					CategoryKey: "new",
					CreateNew:   true,
				},
			},
			IssueTypes: []IssueTypeMapping{
				{
					JiraIDs:        []string{"10000"},
					JiraName:       "Task",
					CreateNew:      true,
					HierarchyLevel: 0,
				},
			},
		},
	}

	handler.executeImportWithClient(jobID, req, client)

	// Verify job completed
	var status string
	err := tdb.QueryRow(`SELECT status FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query job status: %v", err)
	}
	if status != "completed" {
		t.Errorf("Expected job status 'completed', got '%s'", status)
	}

	// Verify items were created
	var itemCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = 1`).Scan(&itemCount)
	if err != nil {
		t.Fatalf("Failed to count items: %v", err)
	}
	if itemCount != 2 {
		t.Errorf("Expected 2 items created, got %d", itemCount)
	}

	// Verify item mappings were recorded
	var mappingCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'item'`, jobID).Scan(&mappingCount)
	if err != nil {
		t.Fatalf("Failed to count mappings: %v", err)
	}
	if mappingCount != 2 {
		t.Errorf("Expected 2 item mappings, got %d", mappingCount)
	}
}

// ================================================================
// Tests: executeImportWithClient from fixtures
// ================================================================

func TestExecuteImportWithClient_FromFixtures(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "jira_import")

	reqPath := filepath.Join(fixtureDir, "import_request.json")
	respPath := filepath.Join(fixtureDir, "jira_responses.json")

	// Skip if fixture files don't exist
	if _, err := os.Stat(reqPath); os.IsNotExist(err) {
		t.Skipf("Fixture file not found: %s (run with JIRA_CAPTURE_PAYLOADS to generate)", reqPath)
	}
	if _, err := os.Stat(respPath); os.IsNotExist(err) {
		t.Skipf("Fixture file not found: %s", respPath)
	}

	// Load request
	reqData, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("Failed to read request fixture: %v", err)
	}
	var req StartImportRequest
	if err = json.Unmarshal(reqData, &req); err != nil {
		t.Fatalf("Failed to parse request fixture: %v", err)
	}

	// Load responses
	respData, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("Failed to read response fixture: %v", err)
	}
	var payloads CapturedPayloads
	if err = json.Unmarshal(respData, &payloads); err != nil {
		t.Fatalf("Failed to parse response fixture: %v", err)
	}

	// Build mock client from captured payloads
	client := newMockJiraClient()
	client.IssueKeysByJQL = payloads.IssueKeys
	client.Emails = payloads.UserEmails
	for _, bulk := range payloads.BulkFetch {
		for _, issue := range bulk.Issues {
			client.Issues[issue.Key] = issue
		}
	}

	// Set up DB and handler
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-fixtures"
	setupTestJob(t, tdb, jobID)

	// For fixture-based tests, map workspaces to use existing workspace
	existingWSID := 1
	for i := range req.Mappings.Workspaces {
		req.Mappings.Workspaces[i].CreateNew = false
		req.Mappings.Workspaces[i].WindshiftID = &existingWSID
	}

	handler.executeImportWithClient(jobID, req, client)

	// Verify job completed
	var status string
	err = tdb.QueryRow(`SELECT status FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query job status: %v", err)
	}
	if status != "completed" {
		t.Errorf("Expected job status 'completed', got '%s'", status)
	}

	// Count imported items
	var itemCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = ?`, existingWSID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("Failed to count items: %v", err)
	}
	t.Logf("Imported %d items from fixtures", itemCount)

	// Count total issue keys across all JQL queries
	totalKeys := 0
	for _, keys := range payloads.IssueKeys {
		totalKeys += len(keys)
	}
	if itemCount == 0 && totalKeys > 0 {
		t.Errorf("Expected items to be imported from %d issue keys, got 0", totalKeys)
	}
}

// ================================================================
// Tests: recording client
// ================================================================

func TestRecordingClient_CapturesResponses(t *testing.T) {
	inner := newMockJiraClient()
	inner.IssueKeysByJQL["project = TEST"] = []string{"TEST-1"}
	inner.Issues["TEST-1"] = jira.JiraIssue{
		ID:  "1",
		Key: "TEST-1",
		Fields: jira.JiraIssueFields{
			Summary: "Test",
		},
	}
	inner.Emails["user-1"] = "user@example.com"

	rc := newRecordingClient(inner)

	ctx := context.Background()

	// Exercise all 3 recorded methods
	keys, _ := rc.GetAllIssueKeys(ctx, "project = TEST")
	if len(keys) != 1 || keys[0] != "TEST-1" {
		t.Errorf("Unexpected keys: %v", keys)
	}

	resp, _ := rc.BulkFetchIssues(ctx, jira.BulkFetchRequest{IssueIdsOrKeys: []string{"TEST-1"}})
	if len(resp.Issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(resp.Issues))
	}

	email, _ := rc.GetUserEmail(ctx, "user-1")
	if email != "user@example.com" {
		t.Errorf("Expected email 'user@example.com', got '%s'", email)
	}

	// Verify captured payloads
	if len(rc.payloads.IssueKeys) != 1 {
		t.Errorf("Expected 1 IssueKeys entry, got %d", len(rc.payloads.IssueKeys))
	}
	if len(rc.payloads.BulkFetch) != 1 {
		t.Errorf("Expected 1 BulkFetch entry, got %d", len(rc.payloads.BulkFetch))
	}
	if rc.payloads.UserEmails["user-1"] != "user@example.com" {
		t.Errorf("Expected captured email, got '%s'", rc.payloads.UserEmails["user-1"])
	}

	// Test saveToFile
	dir := t.TempDir()
	if err := rc.saveToFile(dir); err != nil {
		t.Fatalf("saveToFile failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "jira_responses.json"))
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	if !strings.Contains(string(data), "TEST-1") {
		t.Error("Expected saved file to contain TEST-1")
	}
}

// ================================================================
// Tests: Custom field extraction via UnmarshalJSON
// ================================================================

func TestCustomFieldExtraction(t *testing.T) {
	jsonData := `{
		"id": "10001",
		"key": "TEST-1",
		"fields": {
			"summary": "Test issue",
			"customfield_10001": "custom value",
			"customfield_10002": 42,
			"customfield_10003": null,
			"customfield_10004": {"accountId": "abc123", "displayName": "John"}
		}
	}`

	var issue jira.JiraIssue
	if err := json.Unmarshal([]byte(jsonData), &issue); err != nil {
		t.Fatalf("Failed to unmarshal issue: %v", err)
	}

	if issue.Fields.Summary != "Test issue" {
		t.Errorf("Expected summary 'Test issue', got '%s'", issue.Fields.Summary)
	}

	if len(issue.Fields.CustomFields) == 0 {
		t.Fatal("Expected CustomFields to be populated")
	}

	// Check string custom field
	if v, ok := issue.Fields.CustomFields["customfield_10001"].(string); !ok || v != "custom value" {
		t.Errorf("Expected customfield_10001 to be 'custom value', got %v", issue.Fields.CustomFields["customfield_10001"])
	}

	// Check numeric custom field
	if v, ok := issue.Fields.CustomFields["customfield_10002"].(float64); !ok || v != 42 {
		t.Errorf("Expected customfield_10002 to be 42, got %v", issue.Fields.CustomFields["customfield_10002"])
	}

	// Null custom fields should be captured as nil
	if _, exists := issue.Fields.CustomFields["customfield_10003"]; !exists {
		t.Error("Expected customfield_10003 to exist in CustomFields map (even as nil)")
	}

	// Object custom field
	if v, ok := issue.Fields.CustomFields["customfield_10004"].(map[string]interface{}); !ok {
		t.Errorf("Expected customfield_10004 to be a map, got %T", issue.Fields.CustomFields["customfield_10004"])
	} else if v["accountId"] != "abc123" {
		t.Errorf("Expected customfield_10004.accountId to be 'abc123', got '%v'", v["accountId"])
	}

	// Non-custom fields should NOT be in CustomFields
	if _, exists := issue.Fields.CustomFields["summary"]; exists {
		t.Error("summary should not be in CustomFields")
	}
}

// ================================================================
// Tests: Priority and Due Date import
// ================================================================

func TestImportIssue_PriorityAndDueDate(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-priority"
	setupTestJob(t, tdb, jobID)

	var itemTypeID int
	err := tdb.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&itemTypeID)
	if err != nil {
		var res sql.Result
		res, err = tdb.Exec(`INSERT INTO item_types (name, icon, color, hierarchy_level) VALUES ('Task', 'Task', '#000', 0)`)
		if err != nil {
			t.Fatalf("Failed to create item type: %v", err)
		}
		id, _ := res.LastInsertId()
		itemTypeID = int(id)
	}

	var statusID int
	err = tdb.QueryRow(`SELECT id FROM statuses LIMIT 1`).Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	statusMap := map[string]int{"10001": statusID}
	itemTypeMap := map[string]int{"10100": itemTypeID}
	userMap := map[string]int{}
	versionMap := map[string]int{}

	issue := jira.JiraIssue{
		ID:  "12346",
		Key: "TEST-2",
		Fields: jira.JiraIssueFields{
			Summary: "Issue with priority and due date",
			Status: &jira.JiraStatus{
				ID:   "10001",
				Name: "Open",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10100",
				Name: "Task",
			},
			Priority: &jira.JiraPriority{
				ID:   "3",
				Name: "High",
			},
			DueDate: "2025-06-15",
		},
	}

	mockClient := newMockJiraClient()
	progress := &ImportProgress{}
	err = handler.importIssue(context.Background(), jobID, 1, &issue, statusMap, itemTypeMap, userMap, versionMap, nil, mockClient, progress)
	if err != nil {
		t.Fatalf("importIssue failed: %v", err)
	}

	// Verify priority was set
	var priorityID *int
	var dueDate *string
	err = tdb.QueryRow(`
		SELECT priority_id, due_date FROM items WHERE workspace_id = 1 ORDER BY id DESC LIMIT 1
	`).Scan(&priorityID, &dueDate)
	if err != nil {
		t.Fatalf("Failed to query created item: %v", err)
	}

	if priorityID == nil {
		t.Error("Expected priority_id to be set")
	} else if *priorityID != 3 {
		t.Errorf("Expected priority_id 3 (High), got %d", *priorityID)
	}

	if dueDate == nil {
		t.Error("Expected due_date to be set")
	} else if !strings.Contains(*dueDate, "2025-06-15") {
		t.Errorf("Expected due_date to contain '2025-06-15', got '%s'", *dueDate)
	}
}

// ================================================================
// Tests: Comment Import
// ================================================================

func TestImportIssue_WithComments(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-comments"
	setupTestJob(t, tdb, jobID)

	var itemTypeID int
	err := tdb.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&itemTypeID)
	if err != nil {
		t.Fatalf("Failed to get item type: %v", err)
	}

	var statusID int
	err = tdb.QueryRow(`SELECT id FROM statuses LIMIT 1`).Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	statusMap := map[string]int{"10001": statusID}
	itemTypeMap := map[string]int{"10100": itemTypeID}
	userMap := map[string]int{}
	versionMap := map[string]int{}

	issue := jira.JiraIssue{
		ID:  "12347",
		Key: "TEST-3",
		Fields: jira.JiraIssueFields{
			Summary: "Issue with comments",
			Status: &jira.JiraStatus{
				ID:   "10001",
				Name: "Open",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10100",
				Name: "Task",
			},
			Comment: &jira.JiraCommentContainer{
				Comments: []jira.JiraComment{
					{
						ID:      "100",
						Body:    "This is a plain text comment",
						Created: "2025-01-15T10:30:00.000+0000",
					},
					{
						ID:      "101",
						Body:    "Another comment",
						Created: "2025-01-16T14:00:00.000+0000",
					},
				},
				Total: 2,
			},
		},
	}

	mockClient := newMockJiraClient()
	progress := &ImportProgress{}
	err = handler.importIssue(context.Background(), jobID, 1, &issue, statusMap, itemTypeMap, userMap, versionMap, nil, mockClient, progress)
	if err != nil {
		t.Fatalf("importIssue failed: %v", err)
	}

	// Verify comments were created
	var commentCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&commentCount)
	if err != nil {
		t.Fatalf("Failed to count comments: %v", err)
	}
	if commentCount != 2 {
		t.Errorf("Expected 2 comments, got %d", commentCount)
	}

	// Verify comment content
	var content string
	err = tdb.QueryRow(`SELECT content FROM comments ORDER BY id ASC LIMIT 1`).Scan(&content)
	if err != nil {
		t.Fatalf("Failed to query comment: %v", err)
	}
	if content != "This is a plain text comment" {
		t.Errorf("Expected comment content 'This is a plain text comment', got '%s'", content)
	}

	// Verify comment mappings were recorded
	var mappingCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'comment'`, jobID).Scan(&mappingCount)
	if err != nil {
		t.Fatalf("Failed to count comment mappings: %v", err)
	}
	if mappingCount != 2 {
		t.Errorf("Expected 2 comment mappings, got %d", mappingCount)
	}
}

// ================================================================
// Tests: Attachment Import
// ================================================================

func TestImportIssue_WithAttachments(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-attachments"
	setupTestJob(t, tdb, jobID)

	var itemTypeID int
	err := tdb.QueryRow(`SELECT id FROM item_types LIMIT 1`).Scan(&itemTypeID)
	if err != nil {
		t.Fatalf("Failed to get item type: %v", err)
	}

	var statusID int
	err = tdb.QueryRow(`SELECT id FROM statuses LIMIT 1`).Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	// Seed attachment_settings with a temp directory
	attachmentDir := t.TempDir()
	_, err = tdb.Exec(`INSERT INTO attachment_settings (enabled, attachment_path) VALUES (true, ?)`, attachmentDir)
	if err != nil {
		t.Fatalf("Failed to seed attachment_settings: %v", err)
	}

	statusMap := map[string]int{"10001": statusID}
	itemTypeMap := map[string]int{"10100": itemTypeID}
	userMap := map[string]int{}
	versionMap := map[string]int{}

	issue := jira.JiraIssue{
		ID:  "12348",
		Key: "TEST-4",
		Fields: jira.JiraIssueFields{
			Summary: "Issue with attachments",
			Status: &jira.JiraStatus{
				ID:   "10001",
				Name: "Open",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10100",
				Name: "Task",
			},
			Attachment: []jira.JiraAttachment{
				{
					ID:       "att-1",
					Filename: "screenshot.png",
					Content:  "https://jira.example.com/attachments/screenshot.png",
					MimeType: "image/png",
					Size:     1024,
				},
				{
					ID:       "att-2",
					Filename: "report.pdf",
					Content:  "https://jira.example.com/attachments/report.pdf",
					MimeType: "application/pdf",
					Size:     2048,
				},
			},
		},
	}

	mockClient := newMockJiraClient()
	progress := &ImportProgress{}
	err = handler.importIssue(context.Background(), jobID, 1, &issue, statusMap, itemTypeMap, userMap, versionMap, nil, mockClient, progress)
	if err != nil {
		t.Fatalf("importIssue failed: %v", err)
	}

	// Verify attachment DB records were created
	var attachmentCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM attachments`).Scan(&attachmentCount)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if attachmentCount != 2 {
		t.Errorf("Expected 2 attachments, got %d", attachmentCount)
	}

	// Verify file on disk and original filename for each attachment
	rows, err := tdb.Query(`SELECT file_path, original_filename FROM attachments ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("Failed to query attachments: %v", err)
	}
	defer func() { _ = rows.Close() }()

	expectedFilenames := []string{"screenshot.png", "report.pdf"}
	i := 0
	for rows.Next() {
		var filePath, origFilename string
		if err = rows.Scan(&filePath, &origFilename); err != nil {
			t.Fatalf("Failed to scan attachment row: %v", err)
		}

		// Verify original filename preserved
		if i < len(expectedFilenames) && origFilename != expectedFilenames[i] {
			t.Errorf("Attachment %d: expected original_filename '%s', got '%s'", i, expectedFilenames[i], origFilename)
		}

		// Verify file exists on disk with correct content
		var content []byte
		content, err = os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Attachment %d: failed to read file at '%s': %v", i, filePath, err)
		} else if string(content) != "mock attachment content" {
			t.Errorf("Attachment %d: expected content 'mock attachment content', got '%s'", i, string(content))
		}

		i++
	}

	// Verify ID mappings were recorded
	var mappingCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'attachment'`, jobID).Scan(&mappingCount)
	if err != nil {
		t.Fatalf("Failed to count attachment mappings: %v", err)
	}
	if mappingCount != 2 {
		t.Errorf("Expected 2 attachment mappings, got %d", mappingCount)
	}

	// Verify progress counter
	if progress.ImportedAttachments != 2 {
		t.Errorf("Expected ImportedAttachments = 2, got %d", progress.ImportedAttachments)
	}
}

// ================================================================
// Tests: Parent linking
// ================================================================

func TestLinkParents(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-parents"
	setupTestJob(t, tdb, jobID)

	client := newMockJiraClient()
	client.IssueKeysByJQL["project = PROJ ORDER BY created ASC"] = []string{"PROJ-1", "PROJ-2"}

	// PROJ-1 is an epic (parent), PROJ-2 is a story (child of PROJ-1)
	client.Issues["PROJ-1"] = jira.JiraIssue{
		ID:  "2001",
		Key: "PROJ-1",
		Fields: jira.JiraIssueFields{
			Summary: "Epic issue",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:             "10000",
				Name:           "Epic",
				HierarchyLevel: 1,
			},
		},
	}
	client.Issues["PROJ-2"] = jira.JiraIssue{
		ID:  "2002",
		Key: "PROJ-2",
		Fields: jira.JiraIssueFields{
			Summary: "Child story",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:             "10001",
				Name:           "Story",
				HierarchyLevel: 0,
			},
			Parent: &jira.JiraIssue{
				Key: "PROJ-1",
			},
		},
	}

	existingWSID := 1
	req := StartImportRequest{
		ConnectionID: "conn-1",
		ProjectKeys:  []string{"PROJ"},
		Mappings: ImportMappings{
			Workspaces: []WorkspaceMapping{
				{
					JiraKey:     "PROJ",
					JiraName:    "Test Project",
					IssueCount:  2,
					CreateNew:   false,
					WindshiftID: &existingWSID,
				},
			},
			Statuses: []StatusMapping{
				{
					JiraIDs:     []string{"1"},
					JiraName:    "To Do",
					CategoryKey: "new",
					CreateNew:   true,
				},
			},
			IssueTypes: []IssueTypeMapping{
				{
					JiraIDs:        []string{"10000"},
					JiraName:       "Epic",
					CreateNew:      true,
					HierarchyLevel: 1,
				},
				{
					JiraIDs:        []string{"10001"},
					JiraName:       "Story",
					CreateNew:      true,
					HierarchyLevel: 0,
				},
			},
		},
	}

	handler.executeImportWithClient(jobID, req, client)

	// Verify the child item has parent_id set
	var childParentID *int
	err := tdb.QueryRow(`
		SELECT i.parent_id
		FROM items i
		JOIN jira_import_id_mappings m ON m.windshift_id = i.id
		WHERE m.job_id = ? AND m.entity_type = 'item' AND m.jira_key = 'PROJ-2'
	`, jobID).Scan(&childParentID)
	if err != nil {
		t.Fatalf("Failed to query child item: %v", err)
	}

	if childParentID == nil {
		t.Error("Expected child item to have parent_id set")
	} else {
		// Verify parent_id points to PROJ-1's Windshift ID
		var parentWindshiftID int
		err := tdb.QueryRow(`
			SELECT windshift_id FROM jira_import_id_mappings
			WHERE job_id = ? AND entity_type = 'item' AND jira_key = 'PROJ-1'
		`, jobID).Scan(&parentWindshiftID)
		if err != nil {
			t.Fatalf("Failed to get parent mapping: %v", err)
		}
		if *childParentID != parentWindshiftID {
			t.Errorf("Expected parent_id %d, got %d", parentWindshiftID, *childParentID)
		}
	}
}

// ================================================================
// Tests: Issue Link Import
// ================================================================

func TestImportIssueLinks(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-links"
	setupTestJob(t, tdb, jobID)

	client := newMockJiraClient()
	client.IssueKeysByJQL["project = PROJ ORDER BY created ASC"] = []string{"PROJ-1", "PROJ-2"}

	client.Issues["PROJ-1"] = jira.JiraIssue{
		ID:  "3001",
		Key: "PROJ-1",
		Fields: jira.JiraIssueFields{
			Summary: "Blocking issue",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10000",
				Name: "Task",
			},
			IssueLinks: []jira.JiraIssueLink{
				{
					ID: "link-1",
					Type: &jira.JiraLinkType{
						ID:      "10000",
						Name:    "Blocks",
						Inward:  "is blocked by",
						Outward: "blocks",
					},
					OutwardIssue: &jira.JiraIssue{
						Key: "PROJ-2",
					},
				},
			},
		},
	}
	client.Issues["PROJ-2"] = jira.JiraIssue{
		ID:  "3002",
		Key: "PROJ-2",
		Fields: jira.JiraIssueFields{
			Summary: "Blocked issue",
			Status: &jira.JiraStatus{
				ID:   "1",
				Name: "To Do",
			},
			IssueType: &jira.JiraIssueType{
				ID:   "10000",
				Name: "Task",
			},
			IssueLinks: []jira.JiraIssueLink{
				{
					ID: "link-1",
					Type: &jira.JiraLinkType{
						ID:      "10000",
						Name:    "Blocks",
						Inward:  "is blocked by",
						Outward: "blocks",
					},
					InwardIssue: &jira.JiraIssue{
						Key: "PROJ-1",
					},
				},
			},
		},
	}

	existingWSID := 1
	req := StartImportRequest{
		ConnectionID: "conn-1",
		ProjectKeys:  []string{"PROJ"},
		Mappings: ImportMappings{
			Workspaces: []WorkspaceMapping{
				{
					JiraKey:     "PROJ",
					JiraName:    "Test Project",
					IssueCount:  2,
					CreateNew:   false,
					WindshiftID: &existingWSID,
				},
			},
			Statuses: []StatusMapping{
				{
					JiraIDs:     []string{"1"},
					JiraName:    "To Do",
					CategoryKey: "new",
					CreateNew:   true,
				},
			},
			IssueTypes: []IssueTypeMapping{
				{
					JiraIDs:        []string{"10000"},
					JiraName:       "Task",
					CreateNew:      true,
					HierarchyLevel: 0,
				},
			},
		},
	}

	handler.executeImportWithClient(jobID, req, client)

	// Verify link type was created
	var linkTypeCount int
	err := tdb.QueryRow(`SELECT COUNT(*) FROM link_types WHERE name = 'Blocks'`).Scan(&linkTypeCount)
	if err != nil {
		t.Fatalf("Failed to count link types: %v", err)
	}
	if linkTypeCount != 1 {
		t.Errorf("Expected 1 'Blocks' link type, got %d", linkTypeCount)
	}

	// Verify link labels
	var forwardLabel, reverseLabel string
	err = tdb.QueryRow(`SELECT forward_label, reverse_label FROM link_types WHERE name = 'Blocks'`).Scan(&forwardLabel, &reverseLabel)
	if err != nil {
		t.Fatalf("Failed to query link type: %v", err)
	}
	if forwardLabel != "blocks" {
		t.Errorf("Expected forward_label 'blocks', got '%s'", forwardLabel)
	}
	if reverseLabel != "is blocked by" {
		t.Errorf("Expected reverse_label 'is blocked by', got '%s'", reverseLabel)
	}

	// Verify item link was created (only one, since we only process outward links)
	var itemLinkCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM item_links`).Scan(&itemLinkCount)
	if err != nil {
		t.Fatalf("Failed to count item links: %v", err)
	}
	if itemLinkCount != 1 {
		t.Errorf("Expected 1 item link, got %d", itemLinkCount)
	}

	// Verify link mapping was recorded
	var linkMappingCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'link'`, jobID).Scan(&linkMappingCount)
	if err != nil {
		t.Fatalf("Failed to count link mappings: %v", err)
	}
	if linkMappingCount != 1 {
		t.Errorf("Expected 1 link mapping, got %d", linkMappingCount)
	}
}

// ================================================================
// Tests: ensureWorkflowsAndConfigSet
// ================================================================

// setupWorkflowTestData creates statuses and item types needed for workflow tests.
// Returns statusMap (Jira status ID -> Windshift status ID) and itemTypeMap (Jira issue type ID -> Windshift item type ID).
func setupWorkflowTestData(t *testing.T, tdb *testutils.TestDB, handler *JiraImportHandler, jobID string) (map[string]int, map[string]int) {
	t.Helper()

	statusMap, err := handler.ensureStatuses(context.Background(), jobID, []StatusMapping{
		{
			JiraIDs:     []string{"1"},
			JiraName:    "WF Open",
			CategoryKey: "new",
			CreateNew:   true,
		},
		{
			JiraIDs:     []string{"2"},
			JiraName:    "WF In Progress",
			CategoryKey: "indeterminate",
			CreateNew:   true,
		},
		{
			JiraIDs:     []string{"3"},
			JiraName:    "WF Done",
			CategoryKey: "done",
			CreateNew:   true,
		},
		{
			JiraIDs:     []string{"4"},
			JiraName:    "WF Review",
			CategoryKey: "indeterminate",
			CreateNew:   true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to ensure statuses: %v", err)
	}

	itemTypeMap, err := handler.ensureItemTypes(context.Background(), jobID, []IssueTypeMapping{
		{
			JiraIDs:        []string{"100"},
			JiraName:       "WF Task",
			CreateNew:      true,
			HierarchyLevel: 0,
		},
		{
			JiraIDs:        []string{"101"},
			JiraName:       "WF Bug",
			CreateNew:      true,
			HierarchyLevel: 0,
		},
		{
			JiraIDs:        []string{"102"},
			JiraName:       "WF Epic",
			CreateNew:      true,
			HierarchyLevel: 1,
		},
	})
	if err != nil {
		t.Fatalf("Failed to ensure item types: %v", err)
	}

	return statusMap, itemTypeMap
}

func TestEnsureWorkflowsAndConfigSet_SingleWorkflow(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-wf-single"
	setupTestJob(t, tdb, jobID)

	statusMap, itemTypeMap := setupWorkflowTestData(t, tdb, handler, jobID)

	// All issue types share the same statuses
	client := newMockJiraClient()
	client.ProjectIssueTypeStatuses["PROJ"] = []jira.JiraIssueTypeWithStatuses{
		{
			ID:   "100",
			Name: "Task",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open", StatusCategory: &jira.JiraStatusCategory{ID: 1, Key: "new"}},
				{ID: "2", Name: "In Progress", StatusCategory: &jira.JiraStatusCategory{ID: 2, Key: "indeterminate"}},
				{ID: "3", Name: "Done", StatusCategory: &jira.JiraStatusCategory{ID: 3, Key: "done"}},
			},
		},
		{
			ID:   "101",
			Name: "Bug",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open", StatusCategory: &jira.JiraStatusCategory{ID: 1, Key: "new"}},
				{ID: "2", Name: "In Progress", StatusCategory: &jira.JiraStatusCategory{ID: 2, Key: "indeterminate"}},
				{ID: "3", Name: "Done", StatusCategory: &jira.JiraStatusCategory{ID: 3, Key: "done"}},
			},
		},
	}

	err := handler.ensureWorkflowsAndConfigSet(context.Background(), jobID, "PROJ", 1, statusMap, itemTypeMap, client)
	if err != nil {
		t.Fatalf("ensureWorkflowsAndConfigSet failed: %v", err)
	}

	// Verify: 1 workflow created
	var workflowCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'workflow'`, jobID).Scan(&workflowCount)
	if err != nil {
		t.Fatalf("Failed to count workflows: %v", err)
	}
	if workflowCount != 1 {
		t.Errorf("Expected 1 workflow, got %d", workflowCount)
	}

	// Verify: config set created with differentiate_by_item_type = false
	var csCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'configuration_set'`, jobID).Scan(&csCount)
	if err != nil {
		t.Fatalf("Failed to count config sets: %v", err)
	}
	if csCount != 1 {
		t.Errorf("Expected 1 configuration set, got %d", csCount)
	}

	// Verify differentiate_by_item_type is false
	var differentiate bool
	err = tdb.QueryRow(`
		SELECT cs.differentiate_by_item_type
		FROM configuration_sets cs
		JOIN jira_import_id_mappings m ON m.windshift_id = cs.id
		WHERE m.job_id = ? AND m.entity_type = 'configuration_set'
	`, jobID).Scan(&differentiate)
	if err != nil {
		t.Fatalf("Failed to query config set: %v", err)
	}
	if differentiate {
		t.Error("Expected differentiate_by_item_type to be false for single workflow")
	}

	// Verify workspace is assigned
	var wsAssignCount int
	err = tdb.QueryRow(`
		SELECT COUNT(*) FROM workspace_configuration_sets wcs
		JOIN jira_import_id_mappings m ON m.windshift_id = wcs.configuration_set_id
		WHERE m.job_id = ? AND m.entity_type = 'configuration_set' AND wcs.workspace_id = 1
	`, jobID).Scan(&wsAssignCount)
	if err != nil {
		t.Fatalf("Failed to count workspace assignments: %v", err)
	}
	if wsAssignCount != 1 {
		t.Errorf("Expected 1 workspace assignment, got %d", wsAssignCount)
	}
}

func TestEnsureWorkflowsAndConfigSet_MultipleWorkflows(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-wf-multi"
	setupTestJob(t, tdb, jobID)

	statusMap, itemTypeMap := setupWorkflowTestData(t, tdb, handler, jobID)

	// Different status sets per issue type
	client := newMockJiraClient()
	client.ProjectIssueTypeStatuses["PROJ"] = []jira.JiraIssueTypeWithStatuses{
		{
			ID:   "100",
			Name: "Task",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open"},
				{ID: "2", Name: "In Progress"},
				{ID: "3", Name: "Done"},
			},
		},
		{
			ID:   "101",
			Name: "Bug",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open"},
				{ID: "2", Name: "In Progress"},
				{ID: "3", Name: "Done"},
				{ID: "4", Name: "Review"}, // Bug has an extra status
			},
		},
	}

	err := handler.ensureWorkflowsAndConfigSet(context.Background(), jobID, "PROJ", 1, statusMap, itemTypeMap, client)
	if err != nil {
		t.Fatalf("ensureWorkflowsAndConfigSet failed: %v", err)
	}

	// Verify: 2 workflows created
	var workflowCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'workflow'`, jobID).Scan(&workflowCount)
	if err != nil {
		t.Fatalf("Failed to count workflows: %v", err)
	}
	if workflowCount != 2 {
		t.Errorf("Expected 2 workflows, got %d", workflowCount)
	}

	// Verify: differentiate_by_item_type = true
	var differentiate bool
	err = tdb.QueryRow(`
		SELECT cs.differentiate_by_item_type
		FROM configuration_sets cs
		JOIN jira_import_id_mappings m ON m.windshift_id = cs.id
		WHERE m.job_id = ? AND m.entity_type = 'configuration_set'
	`, jobID).Scan(&differentiate)
	if err != nil {
		t.Fatalf("Failed to query config set: %v", err)
	}
	if !differentiate {
		t.Error("Expected differentiate_by_item_type to be true for multiple workflows")
	}

	// Verify item type configs exist
	var itemTypeConfigCount int
	err = tdb.QueryRow(`
		SELECT COUNT(*) FROM configuration_set_item_types cit
		JOIN jira_import_id_mappings m ON m.windshift_id = cit.configuration_set_id
		WHERE m.job_id = ? AND m.entity_type = 'configuration_set'
	`, jobID).Scan(&itemTypeConfigCount)
	if err != nil {
		t.Fatalf("Failed to count item type configs: %v", err)
	}
	if itemTypeConfigCount != 2 {
		t.Errorf("Expected 2 item type configs, got %d", itemTypeConfigCount)
	}
}

func TestEnsureWorkflowsAndConfigSet_ExistingConfigSet(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-wf-existing"
	setupTestJob(t, tdb, jobID)

	statusMap, itemTypeMap := setupWorkflowTestData(t, tdb, handler, jobID)

	// Pre-create a configuration set assigned to workspace 1
	_, err := tdb.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, differentiate_by_item_type, created_at, updated_at)
		VALUES ('Existing Config', '', false, false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create existing config set: %v", err)
	}
	var existingCSID int
	err = tdb.QueryRow(`SELECT id FROM configuration_sets WHERE name = 'Existing Config'`).Scan(&existingCSID)
	if err != nil {
		t.Fatalf("Failed to get existing config set ID: %v", err)
	}
	_, err = tdb.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
	`, existingCSID)
	if err != nil {
		t.Fatalf("Failed to assign existing config set: %v", err)
	}

	client := newMockJiraClient()
	client.ProjectIssueTypeStatuses["PROJ"] = []jira.JiraIssueTypeWithStatuses{
		{
			ID:   "100",
			Name: "Task",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open"},
				{ID: "2", Name: "In Progress"},
			},
		},
	}

	// Should skip without error
	err = handler.ensureWorkflowsAndConfigSet(context.Background(), jobID, "PROJ", 1, statusMap, itemTypeMap, client)
	if err != nil {
		t.Fatalf("ensureWorkflowsAndConfigSet should not fail: %v", err)
	}

	// Verify: no new workflows or config sets created
	var workflowCount int
	err = tdb.QueryRow(`SELECT COUNT(*) FROM jira_import_id_mappings WHERE job_id = ? AND entity_type = 'workflow'`, jobID).Scan(&workflowCount)
	if err != nil {
		t.Fatalf("Failed to count workflows: %v", err)
	}
	if workflowCount != 0 {
		t.Errorf("Expected 0 workflows (skipped), got %d", workflowCount)
	}
}

func TestEnsureWorkflowsAndConfigSet_Transitions(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	handler := newTestJiraImportHandler(tdb.GetDatabase())
	jobID := "test-job-wf-transitions"
	setupTestJob(t, tdb, jobID)

	statusMap, itemTypeMap := setupWorkflowTestData(t, tdb, handler, jobID)

	// Single workflow with 3 statuses
	client := newMockJiraClient()
	client.ProjectIssueTypeStatuses["PROJ"] = []jira.JiraIssueTypeWithStatuses{
		{
			ID:   "100",
			Name: "Task",
			Statuses: []jira.JiraStatus{
				{ID: "1", Name: "Open", StatusCategory: &jira.JiraStatusCategory{ID: 1, Key: "new"}},
				{ID: "2", Name: "In Progress", StatusCategory: &jira.JiraStatusCategory{ID: 2, Key: "indeterminate"}},
				{ID: "3", Name: "Done", StatusCategory: &jira.JiraStatusCategory{ID: 3, Key: "done"}},
			},
		},
	}

	err := handler.ensureWorkflowsAndConfigSet(context.Background(), jobID, "PROJ", 1, statusMap, itemTypeMap, client)
	if err != nil {
		t.Fatalf("ensureWorkflowsAndConfigSet failed: %v", err)
	}

	// Get the created workflow ID
	var workflowID int
	err = tdb.QueryRow(`
		SELECT windshift_id FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'workflow'
	`, jobID).Scan(&workflowID)
	if err != nil {
		t.Fatalf("Failed to get workflow ID: %v", err)
	}

	// Count initial transitions (from_status_id IS NULL)
	var initialCount int
	err = tdb.QueryRow(`
		SELECT COUNT(*) FROM workflow_transitions
		WHERE workflow_id = ? AND from_status_id IS NULL
	`, workflowID).Scan(&initialCount)
	if err != nil {
		t.Fatalf("Failed to count initial transitions: %v", err)
	}
	// Should have 1 initial transition (only "Open" has category_id=1)
	if initialCount != 1 {
		t.Errorf("Expected 1 initial transition (for 'new' category status), got %d", initialCount)
	}

	// Count all-to-all transitions (from_status_id IS NOT NULL)
	var allToAllCount int
	err = tdb.QueryRow(`
		SELECT COUNT(*) FROM workflow_transitions
		WHERE workflow_id = ? AND from_status_id IS NOT NULL
	`, workflowID).Scan(&allToAllCount)
	if err != nil {
		t.Fatalf("Failed to count all-to-all transitions: %v", err)
	}
	// 3 statuses: 3*(3-1) = 6 all-to-all transitions
	if allToAllCount != 6 {
		t.Errorf("Expected 6 all-to-all transitions (3*2), got %d", allToAllCount)
	}

	// Verify total transition count
	var totalCount int
	err = tdb.QueryRow(`
		SELECT COUNT(*) FROM workflow_transitions WHERE workflow_id = ?
	`, workflowID).Scan(&totalCount)
	if err != nil {
		t.Fatalf("Failed to count total transitions: %v", err)
	}
	expectedTotal := initialCount + allToAllCount
	if totalCount != expectedTotal {
		t.Errorf("Expected %d total transitions, got %d", expectedTotal, totalCount)
	}
}
