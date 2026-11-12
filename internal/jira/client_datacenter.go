package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
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
	// assetsPage caches the page size the legacy Insight AQL endpoint reports.
	// It is server-chosen and ignores maxResults, so the offset-to-page mapping
	// needs the observed value rather than the requested one.
	assetsPage atomic.Int64
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

func (c *dataCenterClient) doWithHeaders(ctx context.Context, method, reqURL string, body any, headers map[string]string) (*http.Response, error) {
	return doReadOnlyJiraRequestWithHeaders(
		ctx, c.httpClient, c.limiter, c.throttle, c.authHeader, method, reqURL, body, headers,
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
		info.URL = jiraSiteURLFromBase(c.baseURL)
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

// ListServiceDeskOrganizations reads the customer-organization catalog. JSM
// 5.x marks this read experimental and requires an opt-in header; newer
// versions accept it as well.
func (c *dataCenterClient) ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskOrganization, error) {
	return jiraServiceDeskValuesWithHeaders[JiraServiceDeskOrganization](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk/%s/organization?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(serviceDeskID), start)
	}, jiraExperimentalHeaders())
}

// ListServiceDeskOrganizationUsers reads organization members. Like the
// organization list, JSM 5.x requires the experimental-read opt-in.
func (c *dataCenterClient) ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]JiraUser, error) {
	return jiraServiceDeskValuesWithHeaders[JiraUser](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/organization/%s/user?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(organizationID), start)
	}, jiraExperimentalHeaders())
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

// GetProjectWorkflowConfiguration is implemented in workflow_datacenter.go;
// it reads the configured graph from the admin workflow designer API and
// returns ErrWorkflowConfigurationNotAvailable when that source is unusable.

func (c *dataCenterClient) GetProjectScreenConfiguration(
	ctx context.Context,
	_ string,
	projectKey string,
	_ []string,
) (*JiraProjectScreenConfiguration, error) {
	// Prefer the companion plugin: Data Center's public REST API does not expose
	// screen schemes, so without it the importer cannot build create/edit/view
	// screens.
	if caps, ok := c.companionCapabilities(ctx); ok && companionSupports(caps, "screens") {
		if config, err := c.companionScreenConfiguration(ctx, projectKey); err == nil && config != nil && len(config.Screens) > 0 {
			return config, nil
		}
	}
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
	refs, err := c.GetAllIssueRefs(ctx, jql)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.Key)
	}
	return keys, nil
}

// GetAllIssueRefs retrieves the numeric id and key of every issue matching a
// JQL query.
func (c *dataCenterClient) GetAllIssueRefs(ctx context.Context, jql string) ([]JiraIssueRef, error) {
	var refs []JiraIssueRef
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
			return refs, err
		}

		for _, issue := range result.Issues {
			refs = append(refs, JiraIssueRef{ID: issue.ID, Key: issue.Key})
		}

		startAt += len(result.Issues)
		if startAt >= result.Total || len(result.Issues) == 0 {
			break
		}
	}

	return refs, nil
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

// ListFilters returns the caller's favorite saved filters. Data Center does
// not expose Cloud's project filter search, so only favorites are discoverable
// here; board-referenced filters are resolved separately by ID. The result is
// marked partial so the importer reports the boundary.
func (c *dataCenterClient) ListFilters(ctx context.Context, _ string) (*FilterSearchResult, error) {
	var filters []JiraFilter
	if err := jiraGetJSON(ctx, c, c.baseURL+"/filter/favourite?expand=jql,description,owner,viewUrl", &filters); err != nil { //nolint:misspell // Atlassian's Data Center route uses the British spelling
		return nil, err
	}
	return &FilterSearchResult{
		MaxResults:    len(filters),
		StartAt:       0,
		Total:         len(filters),
		IsLast:        true,
		Values:        filters,
		Partial:       true,
		PartialReason: "Data Center only exposes favorite saved filters; non-favorite filters cannot be enumerated through the supported API.",
	}, nil
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

// DownloadAttachment downloads an attachment and returns the reader and content
// type. Data Center prefers the API-returned content URL and falls back to the
// attachment-content endpoint under its base URL.
func (c *dataCenterClient) DownloadAttachment(ctx context.Context, attachmentID, contentURL string) (io.ReadCloser, string, error) {
	downloadURL := contentURL
	if downloadURL == "" && attachmentID != "" {
		downloadURL = c.baseURL + "/attachment/content/" + url.PathEscape(attachmentID)
	}
	if downloadURL == "" {
		return nil, "", fmt.Errorf("%w: attachment has no download URL", ErrAPIError)
	}
	return jiraDownloadAttachment(ctx, c.httpClient, c.limiter, c.throttle, c.authHeader, downloadURL)
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

// assetsInsightBase is the legacy Insight REST base that Data Center exposes
// for Assets. Cloud instead uses a per-workspace gateway base
// (…/jsm/assets/workspace/{id}/v1), which Data Center has no equivalent of.
const assetsInsightBase = "/rest/insight/1.0"

// dataCenterAssetsDefaultPageSize is the page size the legacy Insight AQL
// endpoint applies when the schema has not reported its own. The endpoint
// ignores maxResults and pages by page number with a server-chosen size, so the
// adapter must page by the size the server reports.
const dataCenterAssetsDefaultPageSize = 25

func (c *dataCenterClient) assetsBase() string {
	return jiraSiteURLFromBase(c.baseURL) + assetsInsightBase
}

// assetsGetJSON reads a legacy Insight endpoint. A missing endpoint means the
// instance has no Assets capability, so it maps to ErrAssetsNotAvailable rather
// than a generic failure and the importer records an explicit finding.
func (c *dataCenterClient) assetsGetJSON(ctx context.Context, endpoint string, out any) error {
	resp, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrAssetsNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		return jiraErrorFromResponse(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// assetsPageSize returns the server page size last observed for this client,
// falling back to the documented default before the first response.
func (c *dataCenterClient) assetsPageSize() int {
	if size := int(c.assetsPage.Load()); size > 0 {
		return size
	}
	return dataCenterAssetsDefaultPageSize
}

func (c *dataCenterClient) ListObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error) {
	var payload struct {
		ObjectSchemas []AssetObjectSchema `json:"objectschemas"`
	}
	if err := c.assetsGetJSON(ctx, c.assetsBase()+"/objectschema/list", &payload); err != nil {
		return nil, err
	}
	return payload.ObjectSchemas, nil
}

func (c *dataCenterClient) GetObjectSchema(ctx context.Context, schemaID string) (*AssetObjectSchema, error) {
	var schema AssetObjectSchema
	if err := c.assetsGetJSON(ctx, c.assetsBase()+"/objectschema/"+url.PathEscape(schemaID), &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func (c *dataCenterClient) ListObjectTypes(ctx context.Context, schemaID string) ([]AssetObjectType, error) {
	var types []AssetObjectType
	if err := c.assetsGetJSON(ctx, c.assetsBase()+"/objectschema/"+url.PathEscape(schemaID)+"/objecttypes/flat", &types); err != nil {
		return nil, err
	}
	return types, nil
}

func (c *dataCenterClient) GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]AssetObjectAttribute, error) {
	var attrs []AssetObjectAttribute
	if err := c.assetsGetJSON(ctx, c.assetsBase()+"/objecttype/"+url.PathEscape(objectTypeID)+"/attributes", &attrs); err != nil {
		return nil, err
	}
	normalizeAssetDefaultTypes(attrs)
	return attrs, nil
}

// SearchObjects reads one window of Assets objects from the legacy Insight AQL
// endpoint. The endpoint paginates by page number with a server-chosen page
// size and ignores maxResults, so the adapter maps the requested offset onto
// the page containing it and slices to the exact offset. That keeps the
// offset contract the importer's walk relies on even when the server returns
// fewer rows than requested.
func (c *dataCenterClient) SearchObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error) {
	startAt := opts.StartAt
	if opts.Page > 1 && startAt == 0 {
		pageSize := opts.PageSize
		if pageSize < 1 {
			pageSize = c.assetsPageSize()
		}
		startAt = (opts.Page - 1) * pageSize
	}
	if startAt < 0 {
		startAt = 0
	}

	clauses := []string{"objectSchemaId = " + quoteAssetsAQLID(opts.ObjectSchemaID)}
	if opts.ObjectTypeID != "" {
		clauses = append(clauses, "objectTypeId = "+quoteAssetsAQLID(opts.ObjectTypeID))
	}
	if strings.TrimSpace(opts.IQL) != "" {
		clauses = append(clauses, "("+strings.TrimSpace(opts.IQL)+")")
	}

	serverPageSize := c.assetsPageSize()
	page := startAt/serverPageSize + 1
	offsetInPage := startAt % serverPageSize

	query := url.Values{}
	query.Set("qlQuery", strings.Join(clauses, " AND "))
	query.Set("page", strconv.Itoa(page))
	if opts.IncludeAttributes {
		query.Set("includeAttributes", "true")
	}

	var current struct {
		ObjectEntries        []AssetObject          `json:"objectEntries"`
		ObjectTypeAttributes []AssetObjectAttribute `json:"objectTypeAttributes"`
		TotalFilterCount     int                    `json:"totalFilterCount"`
		PageObjectSize       int                    `json:"pageObjectSize"`
	}
	if err := c.assetsGetJSON(ctx, c.assetsBase()+"/aql/objects?"+query.Encode(), &current); err != nil {
		return nil, err
	}
	if current.PageObjectSize > 0 {
		c.assetsPage.Store(int64(current.PageObjectSize))
	}
	normalizeAssetDefaultTypes(current.ObjectTypeAttributes)

	entries := current.ObjectEntries
	if offsetInPage > 0 {
		if offsetInPage >= len(entries) {
			entries = nil
		} else {
			entries = entries[offsetInPage:]
		}
	}

	// The legacy response has no isLast and reports an unreliable toIndex, so
	// exhaustion is derived from the offset plus the rows actually returned.
	isLast := len(entries) == 0 || (current.TotalFilterCount > 0 && startAt+len(entries) >= current.TotalFilterCount)

	return &ObjectSearchResult{
		ObjectEntries:        entries,
		ObjectTypeAttributes: current.ObjectTypeAttributes,
		PageNumber:           page,
		PageSize:             serverPageSize,
		TotalFilterCount:     current.TotalFilterCount,
		StartIndex:           startAt,
		ToIndex:              startAt + len(entries),
		IsLast:               isLast,
	}, nil
}

func (c *dataCenterClient) GetObjectCount(ctx context.Context, schemaID string) (int, error) {
	schema, err := c.GetObjectSchema(ctx, schemaID)
	if err != nil {
		return 0, err
	}
	return schema.ObjectCount, nil
}
