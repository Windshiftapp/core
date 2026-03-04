package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/time/rate"
)

// dataCenterClient implements the Client interface for Jira Data Center/Server
type dataCenterClient struct {
	baseURL    string // Uses /rest/api/2 for Data Center
	agileURL   string
	authHeader string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// do performs an HTTP request with rate limiting
//
//nolint:unparam // method is always "GET" currently but kept for future flexibility
func (c *dataCenterClient) do(ctx context.Context, method, reqURL string, body interface{}) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// setHeaders sets common headers for Jira API requests
func (c *dataCenterClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
}

// handleErrorResponse handles non-2xx responses
func (c *dataCenterClient) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		if strings.Contains(string(body), "rate limit") {
			return ErrRateLimited
		}
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: status %d - %s", ErrAPIError, resp.StatusCode, string(body))
	}
}

// ================================================================
// Connection Methods
// ================================================================

// TestConnection tests if the credentials are valid
func (c *dataCenterClient) TestConnection(ctx context.Context) (*JiraInstanceInfo, error) {
	// Use /myself endpoint to verify credentials
	resp, err := c.do(ctx, "GET", c.baseURL+"/myself", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var user JiraUser
	if err = json.NewDecoder(resp.Body).Decode(&user); err != nil { //nolint:gocritic // intentionally reusing err to avoid shadowing
		return nil, err
	}

	// Get server info for additional details
	serverResp, err := c.do(ctx, "GET", c.baseURL+"/serverInfo", nil)
	if err != nil {
		return &JiraInstanceInfo{
			DisplayName: user.DisplayName,
			URL:         c.baseURL,
		}, nil
	}
	defer func() { _ = serverResp.Body.Close() }()

	var serverInfo struct {
		BaseURL        string `json:"baseUrl"`
		Version        string `json:"version"`
		DeploymentType string `json:"deploymentType"`
		ServerTitle    string `json:"serverTitle"`
	}
	if err := json.NewDecoder(serverResp.Body).Decode(&serverInfo); err != nil {
		return &JiraInstanceInfo{
			DisplayName: user.DisplayName,
			URL:         c.baseURL,
		}, nil
	}

	return &JiraInstanceInfo{
		DisplayName: serverInfo.ServerTitle,
		URL:         serverInfo.BaseURL,
	}, nil
}

// ================================================================
// Project Methods
// ================================================================

// ListProjects lists all projects accessible to the user
func (c *dataCenterClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project?expand=description", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var projects []JiraProject
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject gets details about a specific project
func (c *dataCenterClient) GetProject(ctx context.Context, projectKey string) (*JiraProject, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var project JiraProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ================================================================
// Issue Type Methods
// ================================================================

// ListIssueTypes lists all issue types in the instance
func (c *dataCenterClient) ListIssueTypes(ctx context.Context) ([]JiraIssueType, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/issuetype", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var issueTypes []JiraIssueType
	if err := json.NewDecoder(resp.Body).Decode(&issueTypes); err != nil {
		return nil, err
	}
	return issueTypes, nil
}

// GetProjectIssueTypes gets issue types available in a project
func (c *dataCenterClient) GetProjectIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var issueTypeStatuses []struct {
		ID       string       `json:"id"`
		Name     string       `json:"name"`
		Subtask  bool         `json:"subtask"`
		Statuses []JiraStatus `json:"statuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&issueTypeStatuses); err != nil {
		return nil, err
	}

	issueTypes := make([]JiraIssueType, len(issueTypeStatuses))
	for i, its := range issueTypeStatuses {
		issueTypes[i] = JiraIssueType{
			ID:      its.ID,
			Name:    its.Name,
			Subtask: its.Subtask,
		}
	}
	return issueTypes, nil
}

// ================================================================
// Custom Field Methods
// ================================================================

// ListCustomFields lists all custom field definitions
func (c *dataCenterClient) ListCustomFields(ctx context.Context) ([]JiraCustomField, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/field", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var fields []JiraCustomField
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, err
	}

	// Filter to only custom fields
	customFields := make([]JiraCustomField, 0)
	for _, f := range fields {
		if f.Custom {
			customFields = append(customFields, f)
		}
	}
	return customFields, nil
}

// GetProjectFields returns custom fields - Data Center uses same approach as ListCustomFields
// since the /field/search endpoint with projectIds filter is not available
func (c *dataCenterClient) GetProjectFields(ctx context.Context, projectIDs []string) ([]JiraCustomField, error) {
	// Data Center doesn't have the field/search endpoint with project filtering
	// Return all custom fields instead
	return c.ListCustomFields(ctx)
}

// ================================================================
// Status Methods
// ================================================================

// ListStatuses lists all statuses in the instance
func (c *dataCenterClient) ListStatuses(ctx context.Context) ([]JiraStatus, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var statuses []JiraStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// GetStatusCategories gets all status categories
func (c *dataCenterClient) GetStatusCategories(ctx context.Context) ([]JiraStatusCategory, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/statuscategory", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var categories []JiraStatusCategory
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetProjectWorkflowScheme gets the workflow scheme for a project
func (c *dataCenterClient) GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*JiraWorkflow, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var issueTypeStatuses []struct {
		ID       string       `json:"id"`
		Name     string       `json:"name"`
		Statuses []JiraStatus `json:"statuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&issueTypeStatuses); err != nil {
		return nil, err
	}

	statusMap := make(map[string]JiraStatus)
	for _, its := range issueTypeStatuses {
		for _, s := range its.Statuses {
			statusMap[s.ID] = s
		}
	}

	statuses := make([]JiraStatus, 0, len(statusMap))
	for _, s := range statusMap {
		statuses = append(statuses, s)
	}

	return &JiraWorkflow{
		Name:     projectKey + " Workflow",
		Statuses: statuses,
	}, nil
}

// GetProjectIssueTypeStatuses gets issue types with their available statuses for a project
func (c *dataCenterClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]JiraIssueTypeWithStatuses, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result []JiraIssueTypeWithStatuses
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// ================================================================
// Issue Methods - Data Center uses GET /search with startAt/maxResults
// ================================================================

// SearchIssues searches for issues using JQL (legacy GET endpoint)
func (c *dataCenterClient) SearchIssues(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	params := url.Values{}
	if opts.JQL != "" {
		params.Set("jql", opts.JQL)
	}
	params.Set("startAt", fmt.Sprintf("%d", opts.StartAt))
	if opts.MaxResults > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", opts.MaxResults))
	} else {
		params.Set("maxResults", "50")
	}
	if len(opts.Fields) > 0 {
		params.Set("fields", strings.Join(opts.Fields, ","))
	}
	if len(opts.Expand) > 0 {
		params.Set("expand", strings.Join(opts.Expand, ","))
	}

	resp, err := c.do(ctx, "GET", c.baseURL+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetIssue gets a single issue by key
func (c *dataCenterClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error) {
	params := url.Values{}
	if len(expand) > 0 {
		params.Set("expand", strings.Join(expand, ","))
	}

	urlStr := c.baseURL + "/issue/" + url.PathEscape(issueKey)
	if len(params) > 0 {
		urlStr += "?" + params.Encode()
	}

	resp, err := c.do(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var issue JiraIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// GetIssueCount gets the total number of issues in a project
func (c *dataCenterClient) GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error) {
	jql := fmt.Sprintf("project = %s", projectKey)
	if openOnly {
		jql += " AND statusCategory != Done"
	}

	// Use search with maxResults=0 to get just the total count
	result, err := c.SearchIssues(ctx, SearchOptions{
		JQL:        jql,
		MaxResults: 1,
		Fields:     []string{"key"},
	})
	if err != nil {
		return 0, err
	}

	return result.Total, nil
}

// SearchIssuesJQL searches for issues using JQL
// Data Center uses GET /search with startAt/maxResults pagination
func (c *dataCenterClient) SearchIssuesJQL(ctx context.Context, req JQLSearchRequest) (*JQLSearchResponse, error) {
	params := url.Values{}
	params.Set("jql", req.JQL)
	if req.MaxResults > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", req.MaxResults))
	} else {
		params.Set("maxResults", "50")
	}
	if len(req.Fields) > 0 {
		params.Set("fields", strings.Join(req.Fields, ","))
	}
	if len(req.Expand) > 0 {
		params.Set("expand", strings.Join(req.Expand, ","))
	}

	// Data Center doesn't support nextPageToken, parse it as startAt if provided
	startAt := 0
	if req.NextPageToken != "" {
		_, _ = fmt.Sscanf(req.NextPageToken, "%d", &startAt)
	}
	params.Set("startAt", fmt.Sprintf("%d", startAt))

	resp, err := c.do(ctx, "GET", c.baseURL+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var searchResult SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}

	// Convert to JQLSearchResponse format
	response := &JQLSearchResponse{
		Issues: searchResult.Issues,
		Total:  searchResult.Total,
	}

	// Calculate next page token (startAt for next page)
	nextStartAt := startAt + len(searchResult.Issues)
	if nextStartAt < searchResult.Total {
		response.NextPageToken = fmt.Sprintf("%d", nextStartAt)
	}

	return response, nil
}

// BulkFetchIssues fetches multiple issues by their IDs or keys
// Data Center doesn't have /issue/bulkfetch, so we use search with key in (...) JQL
func (c *dataCenterClient) BulkFetchIssues(ctx context.Context, req BulkFetchRequest) (*BulkFetchResponse, error) {
	if len(req.IssueIdsOrKeys) == 0 {
		return &BulkFetchResponse{Issues: []JiraIssue{}}, nil
	}

	// Build JQL with key in (...)
	// Escape any special characters in keys
	quotedKeys := make([]string, len(req.IssueIdsOrKeys))
	for i, key := range req.IssueIdsOrKeys {
		quotedKeys[i] = "\"" + key + "\""
	}
	jql := "key in (" + strings.Join(quotedKeys, ",") + ")"

	// Fetch in a single request if possible (Data Center typically allows large JQL)
	result, err := c.SearchIssues(ctx, SearchOptions{
		JQL:        jql,
		MaxResults: len(req.IssueIdsOrKeys),
		Fields:     req.Fields,
		Expand:     req.Expand,
	})
	if err != nil {
		return nil, err
	}

	return &BulkFetchResponse{
		Issues: result.Issues,
	}, nil
}

// GetAllIssueKeys retrieves all issue keys matching a JQL query
func (c *dataCenterClient) GetAllIssueKeys(ctx context.Context, jql string) ([]string, error) {
	var keys []string
	startAt := 0
	maxResults := 100

	for {
		result, err := c.SearchIssues(ctx, SearchOptions{
			JQL:        jql,
			StartAt:    startAt,
			MaxResults: maxResults,
			Fields:     []string{"key"},
		})
		if err != nil {
			return keys, err
		}

		for _, issue := range result.Issues {
			keys = append(keys, issue.Key)
		}

		startAt += len(result.Issues)
		if startAt >= result.Total || len(result.Issues) == 0 {
			break
		}
	}

	return keys, nil
}

// ================================================================
// Version & Sprint Methods
// ================================================================

// GetProjectVersions gets all versions for a project
func (c *dataCenterClient) GetProjectVersions(ctx context.Context, projectKey string) ([]JiraVersion, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey)+"/versions", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var versions []JiraVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// ListBoards lists all Agile boards for a project
func (c *dataCenterClient) ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error) {
	params := url.Values{}
	if projectKey != "" {
		params.Set("projectKeyOrId", projectKey)
	}

	resp, err := c.do(ctx, "GET", c.agileURL+"/board?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result BoardListResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetBoardSprints gets all sprints for a board
func (c *dataCenterClient) GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("%s/board/%d/sprint", c.agileURL, boardID), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result SprintListResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ================================================================
// Attachment Methods
// ================================================================

// DownloadAttachment downloads an attachment and returns the reader and content type
func (c *dataCenterClient) DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", attachmentURL, http.NoBody)
	if err != nil {
		return nil, "", err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, "", c.handleErrorResponse(resp)
	}

	contentType := resp.Header.Get("Content-Type")
	return resp.Body, contentType, nil
}

// ================================================================
// User Methods
// ================================================================

// GetUserEmail fetches a user's email address
// Data Center typically includes email in standard user responses, but if called
// we can try to fetch the user directly. If email was already in the issue response,
// the caller should use that instead.
func (c *dataCenterClient) GetUserEmail(ctx context.Context, accountID string) (string, error) {
	if accountID == "" {
		return "", nil
	}

	// Try to get user by username/key (accountID in DC is actually the username or key)
	resp, err := c.do(ctx, "GET", c.baseURL+"/user?username="+url.QueryEscape(accountID), nil)
	if err != nil {
		return "", nil // Return empty, not error - email fetch is best effort
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil // Return empty, not error
	}

	var user JiraUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", nil
	}
	return user.EmailAddress, nil
}

// ================================================================
// Jira Assets (Insight) Methods - Limited support on Data Center
// ================================================================

// ListObjectSchemas lists all object schemas in Assets
// Note: Insight/Assets API on Data Center is different and may not be available
func (c *dataCenterClient) ListObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error) {
	// Try the Insight API path for Data Center
	resp, err := c.do(ctx, "GET", strings.TrimSuffix(c.baseURL, "/rest/api/2")+"/rest/insight/1.0/objectschema/list", nil)
	if err != nil {
		return nil, ErrAssetsNotAvailable
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAssetsNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrAssetsNotAvailable
	}

	var result struct {
		ObjectSchemas []AssetObjectSchema `json:"objectschemas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, ErrAssetsNotAvailable
	}
	return result.ObjectSchemas, nil
}

// GetObjectSchema gets a single object schema by ID
func (c *dataCenterClient) GetObjectSchema(ctx context.Context, schemaID string) (*AssetObjectSchema, error) {
	return nil, ErrAssetsNotAvailable
}

// ListObjectTypes lists all object types in a schema
func (c *dataCenterClient) ListObjectTypes(ctx context.Context, schemaID string) ([]AssetObjectType, error) {
	return nil, ErrAssetsNotAvailable
}

// GetObjectTypeAttributes gets all attributes for an object type
func (c *dataCenterClient) GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]AssetObjectAttribute, error) {
	return nil, ErrAssetsNotAvailable
}

// SearchObjects searches for objects in a schema
func (c *dataCenterClient) SearchObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error) {
	return nil, ErrAssetsNotAvailable
}

// GetObjectCount gets the total number of objects in a schema
func (c *dataCenterClient) GetObjectCount(ctx context.Context, schemaID string) (int, error) {
	return 0, ErrAssetsNotAvailable
}
