package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// dataCenterClient implements the Client interface for Jira Data Center/Server
type dataCenterClient struct {
	baseURL        string // Uses /rest/api/2 for Data Center
	agileURL       string
	serviceDeskURL string
	xrayURL        string
	authHeader     string
	httpClient     *http.Client
	limiter        *rate.Limiter
	throttle       *jiraThrottle
	retryWait      func(context.Context, time.Duration) error
	retryAttempts  int
}

// do performs an HTTP request with rate limiting
//
//nolint:unparam // method is always "GET" currently but kept for future flexibility
func (c *dataCenterClient) do(ctx context.Context, method, reqURL string, body any) (*http.Response, error) {
	return doReadOnlyJiraRequest(
		ctx, c.httpClient, c.limiter, c.throttle, c.authHeader, method, reqURL, body,
		c.retryAttempts, c.retryWait,
	)
}

// ================================================================
// Connection Methods
// ================================================================

// TestConnection tests if the credentials are valid.
//
// Jira Data Center's /serverInfo endpoint may be available anonymously, so it
// cannot prove that a PAT is valid. Require /myself first, then use
// /serverInfo for instance metadata.
func (c *dataCenterClient) TestConnection(ctx context.Context) (*JiraInstanceInfo, error) {
	userResp, err := c.do(ctx, "GET", c.baseURL+"/myself", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	defer func() { _ = userResp.Body.Close() }()

	if userResp.StatusCode != http.StatusOK {
		return nil, jiraErrorFromResponse(userResp)
	}

	var user JiraUser
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode myself: %w", err)
	}

	serverResp, err := c.do(ctx, "GET", c.baseURL+"/serverInfo", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	defer func() { _ = serverResp.Body.Close() }()

	if serverResp.StatusCode != http.StatusOK {
		return nil, jiraErrorFromResponse(serverResp)
	}

	var serverInfo struct {
		BaseURL        string `json:"baseUrl"`
		Version        string `json:"version"`
		DeploymentType string `json:"deploymentType"`
		ServerTitle    string `json:"serverTitle"`
	}
	if err := json.NewDecoder(serverResp.Body).Decode(&serverInfo); err != nil {
		return nil, fmt.Errorf("decode serverInfo: %w", err)
	}

	info := &JiraInstanceInfo{
		DisplayName: serverInfo.ServerTitle,
		URL:         serverInfo.BaseURL,
	}
	if info.URL == "" {
		info.URL = c.baseURL
	}
	if user.DisplayName != "" {
		info.DisplayName = user.DisplayName
	}

	if info.DisplayName == "" {
		info.DisplayName = info.URL
	}
	return info, nil
}

// ================================================================
// Project Methods
// ================================================================

// ListProjects lists all projects accessible to the user
func (c *dataCenterClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
	var projects []JiraProject
	if err := jiraGetJSON(ctx, c, c.baseURL+"/project?expand=description", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject gets details about a specific project
func (c *dataCenterClient) GetProject(ctx context.Context, projectKey string) (*JiraProject, error) {
	return jiraGetProject(ctx, c, c.baseURL, projectKey)
}

func (c *dataCenterClient) ListServiceDesks(ctx context.Context) ([]JiraServiceDesk, error) {
	return jiraServiceDeskValues[JiraServiceDesk](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk?start=%d&limit=100", c.serviceDeskURL, start)
	})
}

func (c *dataCenterClient) ListServiceDeskRequestTypes(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskRequestType, error) {
	return jiraServiceDeskValues[JiraServiceDeskRequestType](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk/%s/requesttype?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(serviceDeskID), start)
	})
}

func (c *dataCenterClient) ListServiceDeskRequestComments(ctx context.Context, issueKey string) ([]JiraServiceDeskComment, error) {
	return jiraServiceDeskValues[JiraServiceDeskComment](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/request/%s/comment?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(issueKey), start)
	})
}

func (c *dataCenterClient) ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskOrganization, error) {
	return jiraServiceDeskValues[JiraServiceDeskOrganization](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk/%s/organization?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(serviceDeskID), start)
	})
}

func (c *dataCenterClient) ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]JiraUser, error) {
	return jiraServiceDeskValues[JiraUser](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/organization/%s/user?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(organizationID), start)
	})
}

// ================================================================
// Issue Type Methods
// ================================================================

// ListIssueTypes lists all issue types in the instance
func (c *dataCenterClient) ListIssueTypes(ctx context.Context) ([]JiraIssueType, error) {
	var issueTypes []JiraIssueType
	if err := jiraGetJSON(ctx, c, c.baseURL+"/issuetype", &issueTypes); err != nil {
		return nil, err
	}
	return issueTypes, nil
}

// GetProjectIssueTypes gets issue types available in a project
func (c *dataCenterClient) GetProjectIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
	issueTypeStatuses, err := jiraFetchProjectIssueTypeStatuses(ctx, c, c.baseURL, projectKey)
	if err != nil {
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
	var fields []JiraCustomField
	if err := jiraGetJSON(ctx, c, c.baseURL+"/field", &fields); err != nil {
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
	var statuses []JiraStatus
	if err := jiraGetJSON(ctx, c, c.baseURL+"/status", &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// GetStatusCategories gets all status categories
func (c *dataCenterClient) GetStatusCategories(ctx context.Context) ([]JiraStatusCategory, error) {
	var categories []JiraStatusCategory
	if err := jiraGetJSON(ctx, c, c.baseURL+"/statuscategory", &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetProjectWorkflowScheme gets the workflow scheme for a project
func (c *dataCenterClient) GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*JiraWorkflow, error) {
	issueTypeStatuses, err := jiraFetchProjectIssueTypeStatuses(ctx, c, c.baseURL, projectKey)
	if err != nil {
		return nil, err
	}
	return jiraProjectWorkflowFromStatuses(projectKey, issueTypeStatuses), nil
}

// GetProjectWorkflowConfiguration reports that Jira Data Center's standard
// REST API does not expose the configured workflow graph used by this importer.
// Status membership remains available through GetProjectIssueTypeStatuses.
func (c *dataCenterClient) GetProjectWorkflowConfiguration(
	context.Context,
	string,
	[]string,
) (*JiraProjectWorkflowConfiguration, error) {
	return nil, ErrWorkflowConfigurationNotAvailable
}

func (c *dataCenterClient) GetProjectScreenConfiguration(
	context.Context,
	string,
	string,
	[]string,
) (*JiraProjectScreenConfiguration, error) {
	return nil, ErrScreenConfigurationNotAvailable
}

// GetProjectIssueTypeStatuses gets issue types with their available statuses for a project
func (c *dataCenterClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]JiraIssueTypeWithStatuses, error) {
	return jiraGetProjectIssueTypeStatuses(ctx, c, c.baseURL, projectKey)
}

// ================================================================
// Issue Methods - Data Center uses GET /search with startAt/maxResults
// ================================================================

// searchIssues runs JQL against the Data Center legacy GET /search endpoint,
// which remains the supported search API for Data Center deployments.
func (c *dataCenterClient) searchIssues(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	return jiraSearchIssuesLegacy(ctx, c, c.baseURL, opts)
}

// GetIssue gets a single issue by key
func (c *dataCenterClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error) {
	return jiraGetIssue(ctx, c, c.baseURL, issueKey, expand)
}

func (c *dataCenterClient) GetIssueWatchers(ctx context.Context, issueKey string) (*JiraIssueWatchers, error) {
	return jiraGetIssueWatchers(ctx, c, c.baseURL, issueKey)
}

func (c *dataCenterClient) GetIssueComments(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraCommentContainer, error) {
	return jiraGetIssueComments(ctx, c, c.baseURL, issueKey, startAt, maxResults)
}

func (c *dataCenterClient) GetIssueWorklogs(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraWorklogContainer, error) {
	return jiraGetIssueWorklogs(ctx, c, c.baseURL, issueKey, startAt, maxResults)
}

// GetIssueCount gets the total number of issues in a project
func (c *dataCenterClient) GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error) {
	jql := `project = "` + escapeJQLString(projectKey) + `"`
	if openOnly {
		jql += " AND statusCategory != Done"
	}

	// Use search with maxResults=0 to get just the total count
	result, err := c.searchIssues(ctx, SearchOptions{
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

	var searchResult SearchResult
	if err := jiraGetJSON(ctx, c, c.baseURL+"/search?"+params.Encode(), &searchResult); err != nil {
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

// BulkFetchIssues fetches multiple issues by their IDs or keys.
// Data Center doesn't have /issue/bulkfetch, so we use search with key in (...)
// JQL. The legacy search endpoint can cap a page below the requested
// maxResults (jira.search.views.default.max and the server's own limit), so a
// single request would silently drop the rest of the batch. Walk every page
// using the returned startAt/len offsets and let the caller reconcile any key
// the server never returned.
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

	requested := len(req.IssueIdsOrKeys)
	issues := make([]JiraIssue, 0, requested)
	for startAt := 0; ; {
		page, err := c.searchIssues(ctx, SearchOptions{
			JQL:        jql,
			StartAt:    startAt,
			MaxResults: requested,
			Fields:     req.Fields,
			Expand:     req.Expand,
		})
		if err != nil {
			return nil, err
		}
		if page == nil || len(page.Issues) == 0 {
			break
		}
		issues = append(issues, page.Issues...)
		next := page.StartAt + len(page.Issues)
		if next <= startAt {
			// The server did not advance its offset; stop instead of looping.
			break
		}
		if next >= requested || (page.Total > 0 && next >= page.Total) {
			break
		}
		startAt = next
	}

	return &BulkFetchResponse{
		Issues: issues,
	}, nil
}

// GetAllIssueKeys retrieves all issue keys matching a JQL query
func (c *dataCenterClient) GetAllIssueKeys(ctx context.Context, jql string) ([]string, error) {
	var keys []string
	startAt := 0
	maxResults := 100

	for {
		result, err := c.searchIssues(ctx, SearchOptions{
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
	return jiraGetProjectVersions(ctx, c, c.baseURL, projectKey)
}

// ListBoards lists all Agile boards for a project.
//
// Jira's Agile API is paginated. Returning only the first page would silently
// miss boards on larger projects, which in turn drops every sprint that lives on
// later boards from the Windshift iteration import.
func (c *dataCenterClient) ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error) {
	return jiraListBoards(ctx, c, c.agileURL, projectKey)
}

// GetBoardConfiguration gets Agile board columns, status mappings, and backing filter metadata.
func (c *dataCenterClient) GetBoardConfiguration(ctx context.Context, boardID int) (*JiraBoardConfiguration, error) {
	return jiraGetBoardConfiguration(ctx, c, c.agileURL, boardID)
}

// ListFilters lists saved filters associated with a project.
func (c *dataCenterClient) ListFilters(ctx context.Context, projectKey string) (*FilterSearchResult, error) {
	return jiraListFilters(ctx, c, c.baseURL, projectKey)
}

// GetFilter gets a saved filter with expanded JQL where available.
func (c *dataCenterClient) GetFilter(ctx context.Context, filterID string) (*JiraFilter, error) {
	return jiraGetFilter(ctx, c, c.baseURL, filterID)
}

// GetBoardSprints gets all sprints for a board.
//
// Jira embeds sprint results in pages too; importers need the full set so issue
// sprint custom fields can always resolve to a Windshift iteration.
func (c *dataCenterClient) GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error) {
	return jiraListBoardSprints(ctx, c, c.agileURL, boardID)
}

// ================================================================
// Attachment Methods
// ================================================================

// DownloadAttachment downloads an attachment and returns the reader and content type
func (c *dataCenterClient) DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error) {
	return jiraDownloadAttachment(ctx, c.httpClient, c.limiter, c.throttle, c.authHeader, attachmentURL)
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
