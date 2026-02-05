package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Client provides methods to interact with the Jira Cloud REST API
type Client interface {
	// Connection
	TestConnection(ctx context.Context) (*JiraInstanceInfo, error)

	// Projects
	ListProjects(ctx context.Context) ([]JiraProject, error)
	GetProject(ctx context.Context, projectKey string) (*JiraProject, error)

	// Issue Types & Fields
	ListIssueTypes(ctx context.Context) ([]JiraIssueType, error)
	GetProjectIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error)
	ListCustomFields(ctx context.Context) ([]JiraCustomField, error)
	GetProjectFields(ctx context.Context, projectIDs []string) ([]JiraCustomField, error)

	// Workflows & Statuses
	ListStatuses(ctx context.Context) ([]JiraStatus, error)
	GetStatusCategories(ctx context.Context) ([]JiraStatusCategory, error)
	GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*JiraWorkflow, error)
	GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]JiraIssueTypeWithStatuses, error)

	// Issues (Legacy - uses deprecated GET /rest/api/3/search)
	SearchIssues(ctx context.Context, opts SearchOptions) (*SearchResult, error)
	GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error)
	GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error)

	// Issues (Enhanced - uses POST /rest/api/3/search/jql)
	SearchIssuesJQL(ctx context.Context, req JQLSearchRequest) (*JQLSearchResponse, error)
	BulkFetchIssues(ctx context.Context, req BulkFetchRequest) (*BulkFetchResponse, error)
	GetAllIssueKeys(ctx context.Context, jql string) ([]string, error)

	// Versions & Sprints
	GetProjectVersions(ctx context.Context, projectKey string) ([]JiraVersion, error)
	ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error)
	GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error)

	// Attachments
	DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error)

	// Users
	GetUserEmail(ctx context.Context, accountID string) (string, error)

	// Jira Assets (Insight) API
	ListObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error)
	GetObjectSchema(ctx context.Context, schemaID string) (*AssetObjectSchema, error)
	ListObjectTypes(ctx context.Context, schemaID string) ([]AssetObjectType, error)
	GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]AssetObjectAttribute, error)
	SearchObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error)
	GetObjectCount(ctx context.Context, schemaID string) (int, error)
}

// Config contains configuration for the Jira client
type Config struct {
	InstanceURL     string         // e.g., https://company.atlassian.net or https://jira.company.com
	Email           string         // User email (Cloud) or username (Data Center) for Basic auth
	APIToken        string         // API token or password
	DeploymentType  DeploymentType // cloud or datacenter (default: cloud)
	RateLimitPerSec int            // Rate limit (default: 10 requests/second)
	Timeout         time.Duration  // HTTP timeout (default: 30 seconds)
}

// cloudClient implements the Client interface for Jira Cloud
type cloudClient struct {
	baseURL    string
	assetsURL  string
	agileURL   string
	authHeader string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// NewClient creates a new Jira API client
// Returns a Cloud or Data Center client based on cfg.DeploymentType
func NewClient(cfg Config) (Client, error) {
	// Validate and normalize the instance URL
	baseURL := strings.TrimSuffix(cfg.InstanceURL, "/")
	if baseURL == "" {
		return nil, ErrInvalidURL
	}

	// Parse URL to validate it
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("%w: must use http or https", ErrInvalidURL)
	}

	// Create Basic auth header (email:token base64 encoded)
	if cfg.Email == "" || cfg.APIToken == "" {
		return nil, ErrInvalidCredentials
	}
	authString := cfg.Email + ":" + cfg.APIToken
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(authString))

	// Set defaults
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	rateLimit := cfg.RateLimitPerSec
	if rateLimit == 0 {
		rateLimit = 10
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	// Return appropriate client based on deployment type
	if cfg.DeploymentType == DeploymentDataCenter {
		return &dataCenterClient{
			baseURL:    baseURL + "/rest/api/2", // Data Center uses API v2
			agileURL:   baseURL + "/rest/agile/1.0",
			authHeader: authHeader,
			httpClient: httpClient,
			limiter:    limiter,
		}, nil
	}

	// Default to Cloud client
	return &cloudClient{
		baseURL:    baseURL + "/rest/api/3",
		assetsURL:  baseURL + "/rest/assets/1.0",
		agileURL:   baseURL + "/rest/agile/1.0",
		authHeader: authHeader,
		httpClient: httpClient,
		limiter:    limiter,
	}, nil
}

// do performs an HTTP request with rate limiting
func (c *cloudClient) do(ctx context.Context, method, reqURL string, body interface{}) (*http.Response, error) {
	// Wait for rate limiter
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
func (c *cloudClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
}

// handleErrorResponse handles non-2xx responses
func (c *cloudClient) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		// Check for rate limiting
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
func (c *cloudClient) TestConnection(ctx context.Context) (*JiraInstanceInfo, error) {
	// Use /myself endpoint to verify credentials
	resp, err := c.do(ctx, "GET", c.baseURL+"/myself", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
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
		// If we can't get server info, just return basic info
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
func (c *cloudClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
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
func (c *cloudClient) GetProject(ctx context.Context, projectKey string) (*JiraProject, error) {
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
func (c *cloudClient) ListIssueTypes(ctx context.Context) ([]JiraIssueType, error) {
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
func (c *cloudClient) GetProjectIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
	resp, err := c.do(ctx, "GET", c.baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	// The response is an array of issue types with their statuses
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
func (c *cloudClient) ListCustomFields(ctx context.Context) ([]JiraCustomField, error) {
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

// GetProjectFields returns only custom fields used by specific projects
// Uses the stable GET /rest/api/3/field/search endpoint with projectIds filter
func (c *cloudClient) GetProjectFields(ctx context.Context, projectIDs []string) ([]JiraCustomField, error) {
	if len(projectIDs) == 0 {
		return nil, fmt.Errorf("at least one project ID is required")
	}

	// Build URL with project IDs and type=custom filter
	endpoint := c.baseURL + "/field/search?projectIds=" + strings.Join(projectIDs, ",") + "&type=custom"

	slog.Debug("GetProjectFields request", slog.String("component", "jira"), slog.String("url", endpoint))

	var allFields []JiraCustomField
	startAt := 0
	maxResults := 50

	for {
		paginatedEndpoint := fmt.Sprintf("%s&startAt=%d&maxResults=%d", endpoint, startAt, maxResults)

		resp, err := c.do(ctx, "GET", paginatedEndpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		slog.Debug("GetProjectFields response", slog.String("component", "jira"), slog.Int("status", resp.StatusCode), slog.Int("body_length", len(body)))

		if resp.StatusCode != http.StatusOK {
			bodyPreview := string(body)
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			slog.Debug("GetProjectFields error response", slog.String("component", "jira"), slog.String("body", bodyPreview))
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, bodyPreview)
		}

		// Parse paginated response
		var result struct {
			Values     []JiraCustomField `json:"values"`
			StartAt    int               `json:"startAt"`
			MaxResults int               `json:"maxResults"`
			Total      int               `json:"total"`
			IsLast     bool              `json:"isLast"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		allFields = append(allFields, result.Values...)

		// Check if we've fetched all fields
		if result.IsLast || len(result.Values) == 0 {
			break
		}
		startAt += len(result.Values)
	}

	return allFields, nil
}

// ================================================================
// Status Methods
// ================================================================

// ListStatuses lists all statuses in the instance
func (c *cloudClient) ListStatuses(ctx context.Context) ([]JiraStatus, error) {
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
func (c *cloudClient) GetStatusCategories(ctx context.Context) ([]JiraStatusCategory, error) {
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
func (c *cloudClient) GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*JiraWorkflow, error) {
	// Get project statuses which includes workflow information
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

	// Collect unique statuses across all issue types
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
func (c *cloudClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]JiraIssueTypeWithStatuses, error) {
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
// Issue Methods
// ================================================================

// SearchIssues searches for issues using JQL
func (c *cloudClient) SearchIssues(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	// Build URL with query parameters
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
func (c *cloudClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error) {
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

// GetIssueCount gets the total number of issues in a project using the new JQL search endpoint
func (c *cloudClient) GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error) {
	jql := fmt.Sprintf("project = %s", projectKey)
	if openOnly {
		jql += " AND statusCategory != Done"
	}

	// Use the new POST /rest/api/3/search/jql endpoint
	// Request only the key field to minimize response size
	result, err := c.SearchIssuesJQL(ctx, JQLSearchRequest{
		JQL:        jql,
		MaxResults: 1, // We only need the total count
		Fields:     []string{"key"},
	})
	if err != nil {
		return 0, err
	}

	// If Total is returned, use it
	if result.Total > 0 {
		return result.Total, nil
	}

	// If Total is not returned (some Jira instances), we need to paginate and count
	// This is a fallback for when total is not available
	return c.countAllIssues(ctx, jql)
}

// countAllIssues counts issues by paginating through all results
// This is a fallback when the total field is not available
func (c *cloudClient) countAllIssues(ctx context.Context, jql string) (int, error) {
	count := 0
	nextPageToken := ""

	for {
		result, err := c.SearchIssuesJQL(ctx, JQLSearchRequest{
			JQL:           jql,
			MaxResults:    100, // Larger batches to count faster
			Fields:        []string{"key"},
			NextPageToken: nextPageToken,
		})
		if err != nil {
			return count, err
		}

		count += len(result.Issues)

		if result.NextPageToken == "" {
			break
		}
		nextPageToken = result.NextPageToken
	}

	return count, nil
}

// SearchIssuesJQL searches for issues using the new POST /rest/api/3/search/jql endpoint
func (c *cloudClient) SearchIssuesJQL(ctx context.Context, req JQLSearchRequest) (*JQLSearchResponse, error) {
	resp, err := c.do(ctx, "POST", c.baseURL+"/search/jql", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result JQLSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BulkFetchIssues fetches multiple issues by their IDs or keys
// Uses POST /rest/api/3/issue/bulkfetch
func (c *cloudClient) BulkFetchIssues(ctx context.Context, req BulkFetchRequest) (*BulkFetchResponse, error) {
	resp, err := c.do(ctx, "POST", c.baseURL+"/issue/bulkfetch", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result BulkFetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAllIssueKeys retrieves all issue keys matching a JQL query
// Paginates through all results using nextPageToken
func (c *cloudClient) GetAllIssueKeys(ctx context.Context, jql string) ([]string, error) {
	var keys []string
	nextPageToken := ""

	for {
		result, err := c.SearchIssuesJQL(ctx, JQLSearchRequest{
			JQL:           jql,
			MaxResults:    100, // Fetch 100 at a time
			Fields:        []string{"key"},
			NextPageToken: nextPageToken,
		})
		if err != nil {
			return keys, err
		}

		for _, issue := range result.Issues {
			keys = append(keys, issue.Key)
		}

		if result.NextPageToken == "" {
			break
		}
		nextPageToken = result.NextPageToken
	}

	return keys, nil
}

// ================================================================
// Version & Sprint Methods
// ================================================================

// GetProjectVersions gets all versions for a project
func (c *cloudClient) GetProjectVersions(ctx context.Context, projectKey string) ([]JiraVersion, error) {
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
func (c *cloudClient) ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error) {
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
func (c *cloudClient) GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error) {
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
func (c *cloudClient) DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error) {
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

// GetUserEmail fetches a user's email address by account ID
// This is needed because Jira Cloud omits email addresses from standard API responses
// Reference: https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-users/#api-rest-api-3-user-email-get
func (c *cloudClient) GetUserEmail(ctx context.Context, accountID string) (string, error) {
	if accountID == "" {
		return "", nil
	}

	resp, err := c.do(ctx, "GET", c.baseURL+"/user/email?accountId="+url.QueryEscape(accountID), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// 404 means user not found or email not available - return empty string, not error
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	// 403 means the user doesn't have permission to view emails
	if resp.StatusCode == http.StatusForbidden {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", c.handleErrorResponse(resp)
	}

	var result UserEmailResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Email, nil
}

// ================================================================
// Jira Assets (Insight) Methods
// ================================================================

// ListObjectSchemas lists all object schemas in Assets
func (c *cloudClient) ListObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error) {
	resp, err := c.do(ctx, "GET", c.assetsURL+"/objectschema/list", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAssetsNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result struct {
		ObjectSchemas []AssetObjectSchema `json:"objectschemas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.ObjectSchemas, nil
}

// GetObjectSchema gets a single object schema by ID
func (c *cloudClient) GetObjectSchema(ctx context.Context, schemaID string) (*AssetObjectSchema, error) {
	resp, err := c.do(ctx, "GET", c.assetsURL+"/objectschema/"+url.PathEscape(schemaID), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var schema AssetObjectSchema
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// ListObjectTypes lists all object types in a schema
func (c *cloudClient) ListObjectTypes(ctx context.Context, schemaID string) ([]AssetObjectType, error) {
	resp, err := c.do(ctx, "GET", c.assetsURL+"/objectschema/"+url.PathEscape(schemaID)+"/objecttypes/flat", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var types []AssetObjectType
	if err := json.NewDecoder(resp.Body).Decode(&types); err != nil {
		return nil, err
	}
	return types, nil
}

// GetObjectTypeAttributes gets all attributes for an object type
func (c *cloudClient) GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]AssetObjectAttribute, error) {
	resp, err := c.do(ctx, "GET", c.assetsURL+"/objecttype/"+url.PathEscape(objectTypeID)+"/attributes", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var attrs []AssetObjectAttribute
	if err := json.NewDecoder(resp.Body).Decode(&attrs); err != nil {
		return nil, err
	}
	return attrs, nil
}

// SearchObjects searches for objects in a schema
func (c *cloudClient) SearchObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error) {
	// Build the request body for object search
	reqBody := map[string]interface{}{
		"objectSchemaId":    opts.ObjectSchemaID,
		"page":              opts.Page,
		"resultsPerPage":    opts.PageSize,
		"includeAttributes": opts.IncludeAttributes,
	}
	if opts.ObjectTypeID != "" {
		reqBody["objectTypeId"] = opts.ObjectTypeID
	}
	if opts.IQL != "" {
		reqBody["iql"] = opts.IQL
	}

	resp, err := c.do(ctx, "POST", c.assetsURL+"/object/navlist/aql", reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result ObjectSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetObjectCount gets the total number of objects in a schema
func (c *cloudClient) GetObjectCount(ctx context.Context, schemaID string) (int, error) {
	schema, err := c.GetObjectSchema(ctx, schemaID)
	if err != nil {
		return 0, err
	}
	return schema.ObjectCount, nil
}
