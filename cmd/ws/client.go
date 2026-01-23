package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client provides methods for calling the Windshift API
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new API client
func NewClient() (*Client, error) {
	if cfg.Server.URL == "" {
		return nil, fmt.Errorf("server URL not configured. Set WS_URL, use --url, or run 'ws config init'")
	}
	if cfg.Server.Token == "" {
		return nil, fmt.Errorf("API token not configured. Set WS_TOKEN, use --token, or run 'ws config init'")
	}

	return &Client{
		baseURL: strings.TrimSuffix(cfg.Server.URL, "/"),
		token:   cfg.Server.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// APIError represents an error response from the API
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// doRequest executes an HTTP request with authentication
func (c *Client) doRequest(method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && (apiErr.Code != "" || apiErr.Message != "") {
			return &apiErr
		}
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// GET performs a GET request
func (c *Client) GET(path string, result interface{}) error {
	return c.doRequest("GET", path, nil, result)
}

// POST performs a POST request
func (c *Client) POST(path string, body interface{}, result interface{}) error {
	return c.doRequest("POST", path, body, result)
}

// PUT performs a PUT request
func (c *Client) PUT(path string, body interface{}, result interface{}) error {
	return c.doRequest("PUT", path, body, result)
}

// DELETE performs a DELETE request
func (c *Client) DELETE(path string) error {
	return c.doRequest("DELETE", path, nil, nil)
}

// ============================================
// REST API v1 Methods
// ============================================

// GetCurrentUser returns the authenticated user
func (c *Client) GetCurrentUser() (*User, error) {
	var user User
	if err := c.GET("/rest/api/v1/users/me", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ListItems lists items with optional filters
func (c *Client) ListItems(filters map[string]string) (*PaginatedResponse[Item], error) {
	path := "/rest/api/v1/items"
	if len(filters) > 0 {
		params := url.Values{}
		for k, v := range filters {
			params.Set(k, v)
		}
		path += "?" + params.Encode()
	}

	var resp PaginatedResponse[Item]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetItem gets a single item by ID with optional expansions
func (c *Client) GetItem(id int, expand string) (*Item, error) {
	path := fmt.Sprintf("/rest/api/v1/items/%d", id)
	if expand != "" {
		path += "?expand=" + url.QueryEscape(expand)
	}

	var item Item
	if err := c.GET(path, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateItem creates a new item
func (c *Client) CreateItem(req ItemCreateRequest) (*Item, error) {
	var item Item
	if err := c.POST("/rest/api/v1/items", req, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateItem updates an item
func (c *Client) UpdateItem(id int, req ItemUpdateRequest) (*Item, error) {
	var item Item
	if err := c.PUT(fmt.Sprintf("/rest/api/v1/items/%d", id), req, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// GetItemTransitions gets valid status transitions for an item
func (c *Client) GetItemTransitions(id int) ([]Transition, error) {
	var transitions []Transition
	if err := c.GET(fmt.Sprintf("/rest/api/v1/items/%d/transitions", id), &transitions); err != nil {
		return nil, err
	}
	return transitions, nil
}

// ListWorkspaces lists all accessible workspaces
func (c *Client) ListWorkspaces() (*PaginatedResponse[Workspace], error) {
	var resp PaginatedResponse[Workspace]
	if err := c.GET("/rest/api/v1/workspaces", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetWorkspace gets a workspace by ID
func (c *Client) GetWorkspace(id int) (*Workspace, error) {
	var ws Workspace
	if err := c.GET(fmt.Sprintf("/rest/api/v1/workspaces/%d", id), &ws); err != nil {
		return nil, err
	}
	return &ws, nil
}

// GetWorkspaceStatuses gets statuses for a workspace
func (c *Client) GetWorkspaceStatuses(workspaceID int) ([]Status, error) {
	var statuses []Status
	if err := c.GET(fmt.Sprintf("/rest/api/v1/workspaces/%d/statuses", workspaceID), &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// ListStatuses lists all statuses
func (c *Client) ListStatuses() ([]Status, error) {
	var statuses []Status
	if err := c.GET("/rest/api/v1/statuses", &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// ListItemTypes lists all item types
func (c *Client) ListItemTypes() ([]ItemType, error) {
	var types []ItemType
	if err := c.GET("/rest/api/v1/item-types", &types); err != nil {
		return nil, err
	}
	return types, nil
}

// ListWorkflows lists all workflows
func (c *Client) ListWorkflows() ([]Workflow, error) {
	var workflows []Workflow
	if err := c.GET("/rest/api/v1/workflows", &workflows); err != nil {
		return nil, err
	}
	return workflows, nil
}

// GetWorkflowTransitions gets transitions for a workflow
func (c *Client) GetWorkflowTransitions(workflowID int) ([]Transition, error) {
	var transitions []Transition
	if err := c.GET(fmt.Sprintf("/rest/api/v1/workflows/%d/transitions", workflowID), &transitions); err != nil {
		return nil, err
	}
	return transitions, nil
}

// ============================================
// Test Management API Methods
// ============================================

// ListTestCases lists test cases in a workspace
func (c *Client) ListTestCases(workspaceID int, folderID string) ([]TestCase, error) {
	path := fmt.Sprintf("/workspaces/%d/test-cases", workspaceID)
	if folderID != "" {
		path += "?folder_id=" + url.QueryEscape(folderID)
	} else {
		path += "?all=true"
	}

	var cases []TestCase
	if err := c.GET(path, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// GetTestCase gets a test case by ID
func (c *Client) GetTestCase(workspaceID, id int) (*TestCase, error) {
	var tc TestCase
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-cases/%d", workspaceID, id), &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

// GetTestSteps gets steps for a test case
func (c *Client) GetTestSteps(workspaceID, testCaseID int) ([]TestStep, error) {
	var steps []TestStep
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-cases/%d/steps", workspaceID, testCaseID), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// ListTestRuns lists test runs in a workspace
func (c *Client) ListTestRuns(workspaceID int, assigneeID string) ([]TestRun, error) {
	path := fmt.Sprintf("/workspaces/%d/test-runs", workspaceID)
	if assigneeID != "" {
		path += "?assignee_id=" + url.QueryEscape(assigneeID)
	}

	var runs []TestRun
	if err := c.GET(path, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// GetTestRun gets a test run by ID
func (c *Client) GetTestRun(workspaceID, id int) (*TestRun, error) {
	var run TestRun
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-runs/%d", workspaceID, id), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// CreateTestRun creates a new test run
func (c *Client) CreateTestRun(workspaceID int, req TestRunCreateRequest) (*TestRun, error) {
	var run TestRun
	if err := c.POST(fmt.Sprintf("/workspaces/%d/test-runs", workspaceID), req, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// EndTestRun ends a test run
func (c *Client) EndTestRun(workspaceID, id int) error {
	return c.POST(fmt.Sprintf("/workspaces/%d/test-runs/%d/end", workspaceID, id), nil, nil)
}

// GetTestRunResults gets results for a test run
func (c *Client) GetTestRunResults(workspaceID, runID int) ([]TestResult, error) {
	var results []TestResult
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-runs/%d/results", workspaceID, runID), &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpdateTestResult updates a test result
func (c *Client) UpdateTestResult(workspaceID, runID, resultID int, req TestResultUpdateRequest) error {
	return c.PUT(fmt.Sprintf("/workspaces/%d/test-runs/%d/results/%d", workspaceID, runID, resultID), req, nil)
}

// ListTestSets lists test sets in a workspace
func (c *Client) ListTestSets(workspaceID int) ([]TestSet, error) {
	var sets []TestSet
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-sets", workspaceID), &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

// GetTestSet gets a test set by ID
func (c *Client) GetTestSet(workspaceID, id int) (*TestSet, error) {
	var set TestSet
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-sets/%d", workspaceID, id), &set); err != nil {
		return nil, err
	}
	return &set, nil
}

// GetTestSetTestCases gets test cases in a test set
func (c *Client) GetTestSetTestCases(workspaceID, setID int) ([]TestCase, error) {
	var cases []TestCase
	if err := c.GET(fmt.Sprintf("/workspaces/%d/test-sets/%d/test-cases", workspaceID, setID), &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// ExecuteRunTemplate executes a test run template
func (c *Client) ExecuteRunTemplate(workspaceID, templateID int) (*TestRun, error) {
	var run TestRun
	if err := c.POST(fmt.Sprintf("/workspaces/%d/test-run-templates/%d/execute", workspaceID, templateID), nil, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// ============================================
// Helper Methods
// ============================================

// ResolveWorkspaceID resolves a workspace key to an ID
func (c *Client) ResolveWorkspaceID(keyOrID string) (int, error) {
	// Try parsing as integer first
	var id int
	if _, err := fmt.Sscanf(keyOrID, "%d", &id); err == nil {
		return id, nil
	}

	// Otherwise, look up by key
	resp, err := c.ListWorkspaces()
	if err != nil {
		return 0, err
	}

	for _, ws := range resp.Data {
		if strings.EqualFold(ws.Key, keyOrID) {
			return ws.ID, nil
		}
	}

	return 0, fmt.Errorf("workspace not found: %s", keyOrID)
}

// ResolveItemID resolves an item key (e.g., PROJ-123) or ID to an item ID
func (c *Client) ResolveItemID(keyOrID string) (int, error) {
	// Try parsing as integer first
	var id int
	if _, err := fmt.Sscanf(keyOrID, "%d", &id); err == nil {
		return id, nil
	}

	// Parse as workspace key + item number (e.g., PROJ-123)
	parts := strings.SplitN(keyOrID, "-", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid item identifier: %s (expected ID or KEY-NUMBER format)", keyOrID)
	}

	wsKey := parts[0]
	var itemNum int
	if _, err := fmt.Sscanf(parts[1], "%d", &itemNum); err != nil {
		return 0, fmt.Errorf("invalid item number in: %s", keyOrID)
	}

	// Find workspace ID
	wsID, err := c.ResolveWorkspaceID(wsKey)
	if err != nil {
		return 0, err
	}

	// Search for item by workspace item number
	filters := map[string]string{
		"workspace_id": fmt.Sprintf("%d", wsID),
	}
	resp, err := c.ListItems(filters)
	if err != nil {
		return 0, err
	}

	for _, item := range resp.Data {
		if item.WorkspaceItemNumber == itemNum {
			return item.ID, nil
		}
	}

	return 0, fmt.Errorf("item not found: %s", keyOrID)
}
