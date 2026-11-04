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
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/utils"

	"golang.org/x/time/rate"
)

// Client provides methods to interact with the Jira Cloud REST API
type Client interface {
	// Connection
	TestConnection(ctx context.Context) (*JiraInstanceInfo, error)

	// Projects
	ListProjects(ctx context.Context) ([]JiraProject, error)
	GetProject(ctx context.Context, projectKey string) (*JiraProject, error)

	// Jira Service Management
	ListServiceDesks(ctx context.Context) ([]JiraServiceDesk, error)
	ListServiceDeskRequestTypes(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskRequestType, error)
	ListServiceDeskRequestComments(ctx context.Context, issueKey string) ([]JiraServiceDeskComment, error)
	ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskOrganization, error)
	ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]JiraUser, error)

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

	// Issues. SearchIssuesJQL is the only issue search on the Client:
	// Cloud serves it via POST /rest/api/3/search/jql (the legacy GET /search
	// endpoint was removed by Atlassian), Data Center adapts it to its
	// supported GET /rest/api/2/search.
	SearchIssuesJQL(ctx context.Context, req JQLSearchRequest) (*JQLSearchResponse, error)
	GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error)
	GetIssueComments(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraCommentContainer, error)
	GetIssueWorklogs(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraWorklogContainer, error)
	GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error)

	// Issues (Bulk - uses POST /rest/api/3/issue/bulkfetch)
	BulkFetchIssues(ctx context.Context, req BulkFetchRequest) (*BulkFetchResponse, error)
	GetAllIssueKeys(ctx context.Context, jql string) ([]string, error)

	// Versions & Sprints
	GetProjectVersions(ctx context.Context, projectKey string) ([]JiraVersion, error)
	ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error)
	GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error)
	GetBoardConfiguration(ctx context.Context, boardID int) (*JiraBoardConfiguration, error)

	// Filters
	ListFilters(ctx context.Context, projectKey string) (*FilterSearchResult, error)
	GetFilter(ctx context.Context, filterID string) (*JiraFilter, error)

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
	Email           string         // User email for Jira Cloud Basic authentication
	APIToken        string         // Cloud API token or Data Center personal access token
	DeploymentType  DeploymentType // cloud or datacenter (default: cloud)
	RateLimitPerSec int            // Rate limit (default: 10 requests/second)
	Timeout         time.Duration  // HTTP timeout (default: 30 seconds)
}

// cloudClient implements the Client interface for Jira Cloud
type cloudClient struct {
	baseURL        string
	assetsURL      string
	agileURL       string
	serviceDeskURL string
	authHeader     string
	httpClient     *http.Client
	limiter        *rate.Limiter
	retryWait      func(context.Context, time.Duration) error
	retryAttempts  int
}

// NewClient creates a new Jira API client.
// Returns a Cloud or Data Center client based on cfg.DeploymentType.
//
// For Cloud, NewClient runs a one-time auto-probe against the operator's
// instance URL to detect whether the supplied API token is a **scoped**
// Atlassian token or a **legacy unscoped** one, and picks the appropriate
// base URL. See cloudRoutingProbe for the algorithm and Atlassian's
// rationale.
func NewClient(cfg Config) (Client, error) {
	baseURL := strings.TrimSuffix(cfg.InstanceURL, "/")
	if baseURL == "" {
		return nil, ErrInvalidURL
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("%w: must use http or https", ErrInvalidURL)
	}

	authHeader, err := jiraAuthHeader(cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	rateLimit := cfg.RateLimitPerSec
	if rateLimit == 0 {
		rateLimit = 10
	}

	// The instance URL is operator-supplied and used as the base for every
	// request (plus the pre-auth tenant_info probe and attachment downloads).
	// Dial through the SSRF-safe dialer so a base URL — or a redirect — that
	// resolves to a private/internal host cannot receive the Jira
	// credential. Redirect-following is preserved; each hop is re-checked.
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: utils.ConfigureHTTPTransport(&http.Transport{DialContext: utils.SafeNetDialer(timeout).DialContext}),
	}
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	if cfg.DeploymentType == DeploymentDataCenter {
		return &dataCenterClient{
			baseURL:        baseURL + "/rest/api/2", // Data Center uses API v2
			agileURL:       baseURL + "/rest/agile/1.0",
			serviceDeskURL: baseURL + "/rest/servicedeskapi",
			xrayURL:        baseURL + "/rest/raven/2.0/api",
			authHeader:     authHeader,
			httpClient:     httpClient,
			limiter:        limiter,
		}, nil
	}

	// Cloud routing is selected by the authentication probe.
	routing := cloudRoutingProbe(baseURL, authHeader, httpClient)
	return &cloudClient{
		baseURL:        routing.platformBase, // /rest/api/3 already appended by probe
		assetsURL:      routing.assetsBase,
		agileURL:       routing.agileBase,
		serviceDeskURL: routing.serviceDeskBase,
		authHeader:     authHeader,
		httpClient:     httpClient,
		limiter:        limiter,
	}, nil
}

// jiraAuthHeader returns the authentication contract for each Jira
// deployment. Data Center is PAT-only; Cloud uses the account email and API
// token as Basic credentials.
func jiraAuthHeader(cfg Config) (string, error) {
	if cfg.APIToken == "" {
		return "", ErrInvalidCredentials
	}
	if cfg.DeploymentType == DeploymentDataCenter {
		return "Bearer " + cfg.APIToken, nil
	}
	if cfg.Email == "" {
		return "", ErrInvalidCredentials
	}
	authString := cfg.Email + ":" + cfg.APIToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(authString)), nil
}

// defaultCloudGatewayBase is the Atlassian gateway host prefix. The discovered
// cloud ID is appended to form the per-tenant gateway base.
const defaultCloudGatewayBase = "https://api.atlassian.com/ex/jira/"

// cloudRouting holds the resolved base URLs for a Cloud client. All three
// already include their respective REST path prefix so the caller can
// concatenate sub-paths directly.
type cloudRouting struct {
	platformBase    string // .../rest/api/3
	agileBase       string // .../rest/agile/1.0
	assetsBase      string // .../jsm/assets/workspace/{id}/v1; empty when unavailable
	serviceDeskBase string // .../rest/servicedeskapi
	viaGateway      bool   // chosen routing, for logging only
}

// cloudRoutingProbe selects the gateway for scoped tokens and the site URL for
// legacy tokens. Scoped tokens sent to a site URL appear anonymous, so it probes
// gateway /myself after discovering the cloud ID. Any discovery or probe failure
// falls back to the site URL, preserving legacy and private-network behavior.
func cloudRoutingProbe(siteURL, authHeader string, httpClient *http.Client) cloudRouting {
	return cloudRoutingProbeWithGateway(siteURL, authHeader, httpClient, defaultCloudGatewayBase)
}

// cloudRoutingProbeWithGateway is cloudRoutingProbe with the gateway prefix
// injected so tests can point the probe at a local server.
func cloudRoutingProbeWithGateway(siteURL, authHeader string, httpClient *http.Client, gatewayPrefix string) cloudRouting {
	cloudID, err := discoverCloudID(siteURL, httpClient)
	if err != nil || cloudID == "" {
		slog.Debug("Jira cloud routing: tenant_info lookup failed, using site URL",
			slog.String("component", "jira"),
			slog.String("site_url", siteURL),
			slog.Any("error", err),
		)
		return resolveSiteRouting(siteURL, authHeader, httpClient)
	}

	gatewayBase := gatewayPrefix + cloudID
	if !gatewayAuthProbe(gatewayBase, authHeader, httpClient) {
		slog.Info("Jira cloud routing: gateway probe declined, using site URL",
			slog.String("component", "jira"),
			slog.String("cloud_id", cloudID),
		)
		return resolveSiteRouting(siteURL, authHeader, httpClient)
	}

	slog.Info("Jira cloud routing: using api.atlassian.com gateway (scoped token detected)",
		slog.String("component", "jira"),
		slog.String("cloud_id", cloudID),
	)
	assetsBase, assetsErr := discoverAssetsWorkspaceBase(gatewayBase, authHeader, httpClient)
	if assetsErr != nil {
		slog.Info("Jira cloud routing: Assets workspace is unavailable",
			slog.String("component", "jira"),
			slog.Any("error", assetsErr),
		)
	}
	return cloudRouting{
		platformBase:    gatewayBase + "/rest/api/3",
		agileBase:       gatewayBase + "/rest/agile/1.0",
		assetsBase:      assetsBase,
		serviceDeskBase: gatewayBase + "/rest/servicedeskapi",
		viaGateway:      true,
	}
}

// resolveSiteRouting builds the site-URL routing and resolves the supported
// Assets workspace API. The legacy /rest/assets/1.0 navlist API is deprecated,
// so an unresolved workspace reports Assets as unavailable (empty assetsBase)
// instead of falling back to it.
func resolveSiteRouting(siteURL, authHeader string, httpClient *http.Client) cloudRouting {
	assetsBase, assetsErr := discoverAssetsWorkspaceBase(siteURL, authHeader, httpClient)
	if assetsErr != nil {
		slog.Info("Jira cloud routing: Assets workspace is unavailable",
			slog.String("component", "jira"),
			slog.String("base_url", siteURL),
			slog.Any("error", assetsErr),
		)
	}
	return cloudRouting{
		platformBase:    siteURL + "/rest/api/3",
		agileBase:       siteURL + "/rest/agile/1.0",
		assetsBase:      assetsBase,
		serviceDeskBase: siteURL + "/rest/servicedeskapi",
		viaGateway:      false,
	}
}

// discoverAssetsWorkspaceBase resolves the site-scoped Assets API base used by
// scoped Cloud tokens. Jira Service Management exposes the workspace identifier
// through a read-only discovery endpoint before the Assets API can be called.
func discoverAssetsWorkspaceBase(gatewayBase, authHeader string, httpClient *http.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		gatewayBase+"/rest/servicedeskapi/assets/workspace?start=0&limit=100",
		http.NoBody,
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req) //nolint:gosec // gatewayBase is derived from Atlassian's cloud ID
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("assets workspace discovery HTTP %d", resp.StatusCode)
	}

	var page struct {
		Values []struct {
			WorkspaceID string `json:"workspaceId"`
			ID          string `json:"id"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", err
	}
	if len(page.Values) == 0 {
		return "", ErrAssetsNotAvailable
	}
	workspaceID := page.Values[0].WorkspaceID
	if workspaceID == "" {
		workspaceID = page.Values[0].ID
	}
	if workspaceID == "" {
		return "", fmt.Errorf("%w: workspace response has no identifier", ErrAssetsNotAvailable)
	}
	return gatewayBase + "/jsm/assets/workspace/" + url.PathEscape(workspaceID) + "/v1", nil
}

// discoverCloudID calls the public /_edge/tenant_info well-known endpoint,
// which returns the site's stable cloud identifier. The endpoint is
// unauthenticated.
func discoverCloudID(siteURL string, httpClient *http.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", siteURL+"/_edge/tenant_info", http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req) //nolint:gosec // siteURL is operator-supplied Jira base URL
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tenant_info HTTP %d", resp.StatusCode)
	}
	var body struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.CloudID, nil
}

// gatewayAuthProbe asks "does this token authenticate against the gateway?"
// by hitting /rest/api/3/myself. A 200 means scoped-token routing is in
// effect for this caller; anything else means it isn't (legacy token, or a
// scoped token that the gateway has rejected outright).
//
// /myself is the right probe here: it requires an authenticated identity,
// so the response distinguishes "auth succeeded" (200) from "auth was
// silently dropped to anonymous" (401). Other endpoints like /serverInfo
// return 200 even anonymously and would give false positives.
func gatewayAuthProbe(gatewayBase, authHeader string, httpClient *http.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", gatewayBase+"/rest/api/3/myself", http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req) //nolint:gosec // gatewayBase derived from discovered cloudId
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// do performs an HTTP request with rate limiting
func (c *cloudClient) do(ctx context.Context, method, reqURL string, body any) (*http.Response, error) {
	return doReadOnlyJiraRequest(
		ctx, c.httpClient, c.limiter, c.authHeader, method, reqURL, body,
		c.retryAttempts, c.retryWait,
	)
}

const (
	defaultJiraRetryAttempts = 3
	jiraRetryBaseDelay       = 250 * time.Millisecond
	jiraRetryMaxDelay        = 5 * time.Second
)

func doReadOnlyJiraRequest(
	ctx context.Context,
	httpClient *http.Client,
	limiter *rate.Limiter,
	authHeader string,
	method string,
	reqURL string,
	body any,
	retryAttempts int,
	retryWait func(context.Context, time.Duration) error,
) (*http.Response, error) {
	if err := validateReadOnlyRequest(method, reqURL); err != nil {
		return nil, err
	}
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	if retryAttempts <= 0 {
		retryAttempts = defaultJiraRetryAttempts
	}
	if retryWait == nil {
		retryWait = waitForJiraRetry
	}

	for attempt := 0; ; attempt++ {
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := httpClient.Do(req) //nolint:gosec // G704: URL constructed from configured Jira base URL
		if err != nil {
			return nil, err
		}
		if !jiraResponseIsRetryable(resp.StatusCode) || attempt >= retryAttempts {
			return resp, nil
		}

		delay := jiraRetryDelay(resp.Header.Get("Retry-After"), attempt, time.Now())
		// Make upstream throttling diagnosable: without this a slow import cannot
		// be told apart from a rate-limited one.
		slog.Warn("Jira request throttled; retrying",
			slog.String("component", "jira"),
			slog.Int("status", resp.StatusCode),
			slog.Int("attempt", attempt+1),
			slog.Duration("delay", delay))
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if err := retryWait(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func jiraResponseIsRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

func jiraRetryDelay(retryAfter string, attempt int, now time.Time) time.Duration {
	retryAfter = strings.TrimSpace(retryAfter)
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(retryAfter); err == nil {
		if delay := retryAt.Sub(now); delay > 0 {
			return delay
		}
		return 0
	}
	delay := jiraRetryBaseDelay * time.Duration(1<<min(attempt, 4))
	if delay > jiraRetryMaxDelay {
		return jiraRetryMaxDelay
	}
	return delay
}

func waitForJiraRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// validateReadOnlyRequest prevents the Jira importer from ever mutating the
// source instance. Jira exposes a few query operations as POST endpoints, so
// those exact paths are allowed while every other non-GET method is denied.
func validateReadOnlyRequest(method, reqURL string) error {
	switch method {
	case http.MethodGet, http.MethodHead:
		return nil
	case http.MethodPost:
		parsed, err := url.Parse(reqURL)
		if err != nil {
			return fmt.Errorf("%w: invalid request URL: %v", ErrReadOnlyViolation, err)
		}
		for _, suffix := range []string{
			"/rest/api/3/search/jql",
			"/rest/api/3/issue/bulkfetch",
			"/rest/api/3/workflows",
		} {
			if strings.HasSuffix(parsed.Path, suffix) {
				return nil
			}
		}
		if isAssetsWorkspaceAQLPath(parsed.Path) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s %s", ErrReadOnlyViolation, method, reqURL)
}

func isAssetsWorkspaceAQLPath(path string) bool {
	const marker = "/jsm/assets/workspace/"
	index := strings.Index(path, marker)
	if index < 0 {
		return false
	}
	parts := strings.Split(strings.Trim(path[index+len(marker):], "/"), "/")
	return len(parts) == 4 &&
		parts[0] != "" &&
		parts[1] == "v1" &&
		parts[2] == "object" &&
		parts[3] == "aql"
}

// ================================================================
// Connection Methods
// ================================================================

// TestConnection tests if the credentials are valid.
//
// Probe order is deliberate: /serverInfo first, /myself second. Atlassian's
// scoped API tokens (rolled out 2024) can grant read access to projects /
// fields / issues without granting read:me, so /myself returns 401 even
// though the token is perfectly usable for the importer. Probing with
// /serverInfo confirms credentials reach the instance without depending on
// the account-identity scope. /myself is then a best-effort enrichment for
// the human-readable connection label — failure is logged and ignored.
func (c *cloudClient) TestConnection(ctx context.Context) (*JiraInstanceInfo, error) {
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

	// Best-effort: enrich DisplayName with the authenticating user's name.
	// 401 here is expected for scoped tokens without read:me — do not fail
	// the whole TestConnection over it.
	if userResp, userErr := c.do(ctx, "GET", c.baseURL+"/myself", nil); userErr == nil {
		defer func() { _ = userResp.Body.Close() }()
		if userResp.StatusCode == http.StatusOK {
			var user JiraUser
			if decodeErr := json.NewDecoder(userResp.Body).Decode(&user); decodeErr == nil && user.DisplayName != "" {
				info.DisplayName = user.DisplayName
			}
		}
	}

	if info.DisplayName == "" {
		info.DisplayName = info.URL
	}
	return info, nil
}

// ================================================================
// Project Methods
// ================================================================

// ListProjects lists all projects accessible to the user. Jira Cloud removed
// the legacy unpaginated GET /project endpoint, so walk the supported paginated
// GET /project/search instead. Data Center keeps its deployment-specific call.
func (c *cloudClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
	return cloudPageValues[JiraProject](ctx, c, c.baseURL+"/project/search?expand=description")
}

// GetProject gets details about a specific project
func (c *cloudClient) GetProject(ctx context.Context, projectKey string) (*JiraProject, error) {
	return jiraGetProject(ctx, c, c.baseURL, projectKey)
}

// ListServiceDesks lists every Jira Service Management portal visible to the
// importing account.
func (c *cloudClient) ListServiceDesks(ctx context.Context) ([]JiraServiceDesk, error) {
	return jiraServiceDeskValues[JiraServiceDesk](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk?start=%d&limit=100", c.serviceDeskURL, start)
	})
}

// ListServiceDeskRequestTypes returns the complete customer-facing request
// type catalog for one service desk.
func (c *cloudClient) ListServiceDeskRequestTypes(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskRequestType, error) {
	return jiraServiceDeskValues[JiraServiceDeskRequestType](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk/%s/requesttype?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(serviceDeskID), start)
	})
}

// ListServiceDeskRequestComments returns JSM's public/internal visibility
// metadata for every comment on a customer request.
func (c *cloudClient) ListServiceDeskRequestComments(ctx context.Context, issueKey string) ([]JiraServiceDeskComment, error) {
	return jiraServiceDeskValues[JiraServiceDeskComment](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/request/%s/comment?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(issueKey), start)
	})
}

// ListServiceDeskOrganizations returns organizations associated with one JSM
// service desk.
func (c *cloudClient) ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]JiraServiceDeskOrganization, error) {
	return jiraServiceDeskValues[JiraServiceDeskOrganization](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/servicedesk/%s/organization?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(serviceDeskID), start)
	})
}

// ListServiceDeskOrganizationUsers returns every customer in a JSM
// organization.
func (c *cloudClient) ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]JiraUser, error) {
	return jiraServiceDeskValues[JiraUser](ctx, c, func(start int) string {
		return fmt.Sprintf("%s/organization/%s/user?start=%d&limit=100",
			c.serviceDeskURL, url.PathEscape(organizationID), start)
	})
}

// ================================================================
// Issue Type Methods
// ================================================================

// ListIssueTypes lists all issue types in the instance
func (c *cloudClient) ListIssueTypes(ctx context.Context) ([]JiraIssueType, error) {
	var issueTypes []JiraIssueType
	if err := jiraGetJSON(ctx, c, c.baseURL+"/issuetype", &issueTypes); err != nil {
		return nil, err
	}
	return issueTypes, nil
}

// GetProjectIssueTypes gets issue types available in a project
func (c *cloudClient) GetProjectIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
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
func (c *cloudClient) ListCustomFields(ctx context.Context) ([]JiraCustomField, error) {
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

// GetProjectFields returns only custom fields used by specific projects
// Uses the stable GET /rest/api/3/field/search endpoint with projectIds filter
func (c *cloudClient) GetProjectFields(ctx context.Context, projectIDs []string) ([]JiraCustomField, error) {
	if len(projectIDs) == 0 {
		return nil, fmt.Errorf("at least one project ID is required")
	}

	// Build URL with project IDs and type=custom filter
	endpoint := c.baseURL + "/field/search?projectIds=" + strings.Join(projectIDs, ",") + "&type=custom"

	slog.Debug("GetProjectFields request", slog.String("component", "jira"), slog.String("url", endpoint))

	const pageSize = 50
	agg, err := jiraAccumulateStartAtValues[JiraCustomField](ctx, c, pageSize, func(startAt int) string {
		return fmt.Sprintf("%s&startAt=%d&maxResults=%d", endpoint, startAt, pageSize)
	})
	if err != nil {
		return nil, err
	}
	return agg.Values, nil
}

type cloudFlexibleID string

func (id *cloudFlexibleID) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = cloudFlexibleID(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*id = cloudFlexibleID(number.String())
	return nil
}

type cloudCustomFieldContext struct {
	ID             cloudFlexibleID `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	IsGlobal       bool            `json:"isGlobalContext"`
	IsAnyIssueType bool            `json:"isAnyIssueType"`
}

type cloudCustomFieldProjectMapping struct {
	ContextID       cloudFlexibleID `json:"contextId"`
	ProjectID       string          `json:"projectId"`
	IsGlobalContext bool            `json:"isGlobalContext"`
}

type cloudCustomFieldIssueTypeMapping struct {
	ContextID      cloudFlexibleID `json:"contextId"`
	IssueTypeID    string          `json:"issueTypeId"`
	IsAnyIssueType bool            `json:"isAnyIssueType"`
}

type cloudCustomFieldOption struct {
	ID       cloudFlexibleID `json:"id"`
	Value    string          `json:"value"`
	OptionID cloudFlexibleID `json:"optionId"`
	Disabled bool            `json:"disabled"`
}

type cloudCustomFieldContextDefaults struct {
	ContextID     cloudFlexibleID `json:"contextId"`
	DefaultValues []struct {
		IssueTypeID    string `json:"issueTypeId"`
		IsAnyIssueType bool   `json:"isAnyIssueType"`
		Value          any    `json:"value"`
	} `json:"defaultValues"`
}

func cloudPageValues[T any](ctx context.Context, c *cloudClient, endpoint string) ([]T, error) {
	agg, err := jiraAccumulateStartAtValues[T](ctx, c, 100, func(startAt int) string {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		return fmt.Sprintf("%s%sstartAt=%d&maxResults=100", endpoint, separator, startAt)
	})
	if err != nil {
		return nil, err
	}
	return agg.Values, nil
}

// GetCustomFieldConfiguration reads every context, project/issue-type mapping,
// default, and (for choice fields) option exposed by Jira Cloud. The endpoints
// require Jira configuration permissions, so callers deliberately consume this
// through the optional CustomFieldConfigurationClient capability.
func (c *cloudClient) GetCustomFieldConfiguration(
	ctx context.Context,
	fieldID string,
	includeOptions bool,
) (*JiraCustomFieldConfiguration, error) {
	fieldID = strings.TrimSpace(fieldID)
	if fieldID == "" {
		return nil, fmt.Errorf("%w: field ID is required", ErrCustomFieldConfigurationNotAvailable)
	}
	fieldPath := c.baseURL + "/field/" + url.PathEscape(fieldID) + "/context"
	contexts, err := cloudPageValues[cloudCustomFieldContext](ctx, c, fieldPath)
	if err != nil {
		return nil, fmt.Errorf("%w: contexts for %s: %v", ErrCustomFieldConfigurationNotAvailable, fieldID, err)
	}
	projectMappings, err := cloudPageValues[cloudCustomFieldProjectMapping](ctx, c, fieldPath+"/projectmapping")
	if err != nil {
		return nil, fmt.Errorf("%w: project mappings for %s: %v", ErrCustomFieldConfigurationNotAvailable, fieldID, err)
	}
	issueTypeMappings, err := cloudPageValues[cloudCustomFieldIssueTypeMapping](ctx, c, fieldPath+"/issuetypemapping")
	if err != nil {
		return nil, fmt.Errorf("%w: issue-type mappings for %s: %v", ErrCustomFieldConfigurationNotAvailable, fieldID, err)
	}
	defaultGroups, err := cloudPageValues[cloudCustomFieldContextDefaults](ctx, c, fieldPath+"/defaultValues")
	defaultsUnavailableReason := ""
	if err != nil {
		// Jira rejects this endpoint for several app-owned and picker fields.
		// Context and applicability remain authoritative even when the field's
		// default-value contract cannot be read.
		defaultGroups = nil
		defaultsUnavailableReason = err.Error()
	}

	result := &JiraCustomFieldConfiguration{
		FieldID:                   fieldID,
		Contexts:                  make([]JiraCustomFieldContext, 0, len(contexts)),
		DefaultsUnavailableReason: defaultsUnavailableReason,
	}
	contextIndexes := make(map[string]int, len(contexts))
	for _, source := range contexts {
		contextID := string(source.ID)
		contextIndexes[contextID] = len(result.Contexts)
		result.Contexts = append(result.Contexts, JiraCustomFieldContext{
			ID:             contextID,
			Name:           source.Name,
			Description:    source.Description,
			IsGlobal:       source.IsGlobal,
			IsAnyIssueType: source.IsAnyIssueType,
		})
	}
	for _, mapping := range projectMappings {
		index, ok := contextIndexes[string(mapping.ContextID)]
		if !ok {
			continue
		}
		if mapping.IsGlobalContext {
			result.Contexts[index].IsGlobal = true
		} else if mapping.ProjectID != "" {
			result.Contexts[index].ProjectIDs = append(result.Contexts[index].ProjectIDs, mapping.ProjectID)
		}
	}
	for _, mapping := range issueTypeMappings {
		index, ok := contextIndexes[string(mapping.ContextID)]
		if !ok {
			continue
		}
		if mapping.IsAnyIssueType {
			result.Contexts[index].IsAnyIssueType = true
		} else if mapping.IssueTypeID != "" {
			result.Contexts[index].IssueTypeIDs = append(result.Contexts[index].IssueTypeIDs, mapping.IssueTypeID)
		}
	}
	for _, group := range defaultGroups {
		index, ok := contextIndexes[string(group.ContextID)]
		if !ok {
			continue
		}
		for _, value := range group.DefaultValues {
			result.Contexts[index].Defaults = append(result.Contexts[index].Defaults, JiraCustomFieldDefaultValue{
				IssueTypeID:    value.IssueTypeID,
				IsAnyIssueType: value.IsAnyIssueType,
				Value:          value.Value,
			})
		}
	}
	if includeOptions {
		for index := range result.Contexts {
			contextID := result.Contexts[index].ID
			options, optionErr := cloudPageValues[cloudCustomFieldOption](
				ctx,
				c,
				fieldPath+"/"+url.PathEscape(contextID)+"/option",
			)
			if optionErr != nil {
				// Not every field that Windshift represents as a select is backed
				// by Jira's custom-field option model (group pickers are a common
				// example). Keep the context/default/applicability contract and
				// report only the option slice as unavailable.
				result.Contexts[index].OptionsUnavailableReason = optionErr.Error()
				continue
			}
			for _, option := range options {
				result.Contexts[index].Options = append(result.Contexts[index].Options, JiraCustomFieldContextOption{
					ID:             string(option.ID),
					Value:          option.Value,
					ParentOptionID: string(option.OptionID),
					Disabled:       option.Disabled,
				})
			}
		}
	}
	for index := range result.Contexts {
		sort.Strings(result.Contexts[index].ProjectIDs)
		sort.Strings(result.Contexts[index].IssueTypeIDs)
	}
	sort.SliceStable(result.Contexts, func(i, j int) bool {
		return result.Contexts[i].ID < result.Contexts[j].ID
	})
	return result, nil
}

// ================================================================
// Status Methods
// ================================================================

// ListStatuses lists all statuses in the instance
func (c *cloudClient) ListStatuses(ctx context.Context) ([]JiraStatus, error) {
	var statuses []JiraStatus
	if err := jiraGetJSON(ctx, c, c.baseURL+"/status", &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// GetStatusCategories gets all status categories
func (c *cloudClient) GetStatusCategories(ctx context.Context) ([]JiraStatusCategory, error) {
	var categories []JiraStatusCategory
	if err := jiraGetJSON(ctx, c, c.baseURL+"/statuscategory", &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetProjectWorkflowScheme gets the workflow scheme for a project
func (c *cloudClient) GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*JiraWorkflow, error) {
	issueTypeStatuses, err := jiraFetchProjectIssueTypeStatuses(ctx, c, c.baseURL, projectKey)
	if err != nil {
		return nil, err
	}
	return jiraProjectWorkflowFromStatuses(projectKey, issueTypeStatuses), nil
}

type cloudWorkflowReadRequest struct {
	ProjectAndIssueTypes []cloudWorkflowProjectIssueType `json:"projectAndIssueTypes"`
}

type cloudWorkflowProjectIssueType struct {
	ProjectID   string `json:"projectId"`
	IssueTypeID string `json:"issueTypeId"`
}

type cloudWorkflowReadResponse struct {
	Statuses  []cloudWorkflowStatus `json:"statuses"`
	Workflows []cloudWorkflow       `json:"workflows"`
}

type cloudWorkflowStatus struct {
	ID              string `json:"id"`
	StatusReference string `json:"statusReference"`
}

type cloudWorkflow struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Statuses    []cloudWorkflowStatusRef  `json:"statuses"`
	Transitions []cloudWorkflowTransition `json:"transitions"`
}

type cloudWorkflowStatusRef struct {
	StatusReference string `json:"statusReference"`
}

type cloudWorkflowTransition struct {
	ID                string                        `json:"id"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description"`
	Type              string                        `json:"type"`
	ToStatusReference string                        `json:"toStatusReference"`
	Links             []cloudWorkflowTransitionLink `json:"links"`
	Validators        []json.RawMessage             `json:"validators"`
	Actions           []json.RawMessage             `json:"actions"`
	Triggers          []json.RawMessage             `json:"triggers"`
	Conditions        []json.RawMessage             `json:"conditions"`
	Rules             json.RawMessage               `json:"rules"`
}

type cloudWorkflowTransitionLink struct {
	FromStatusReference string `json:"fromStatusReference"`
}

// GetProjectWorkflowConfiguration reads the workflow selected for every
// project/issue-type pair. Calls are intentionally made one issue type at a
// time: the Jira response identifies workflows but does not echo the pair that
// selected each workflow, so batching several pairs would make that mapping
// ambiguous when a project uses multiple workflows.
func (c *cloudClient) GetProjectWorkflowConfiguration(
	ctx context.Context,
	projectID string,
	issueTypeIDs []string,
) (*JiraProjectWorkflowConfiguration, error) {
	result := &JiraProjectWorkflowConfiguration{
		IssueTypeWorkflowIDs: make(map[string]string, len(issueTypeIDs)),
	}
	workflowsByID := make(map[string]JiraWorkflowConfiguration)

	for _, issueTypeID := range issueTypeIDs {
		req := cloudWorkflowReadRequest{
			ProjectAndIssueTypes: []cloudWorkflowProjectIssueType{{
				ProjectID:   projectID,
				IssueTypeID: issueTypeID,
			}},
		}
		resp, err := c.do(ctx, http.MethodPost, c.baseURL+"/workflows", req)
		if err != nil {
			return nil, err
		}

		var payload cloudWorkflowReadResponse
		if resp.StatusCode != http.StatusOK {
			err = jiraErrorFromResponse(resp)
			_ = resp.Body.Close()
			return nil, err
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode Jira workflow configuration for issue type %s: %w", issueTypeID, err)
		}
		if len(payload.Workflows) != 1 {
			return nil, fmt.Errorf(
				"jira workflow configuration for issue type %s returned %d workflows",
				issueTypeID,
				len(payload.Workflows),
			)
		}

		source := payload.Workflows[0]
		statusIDsByReference := make(map[string]string, len(payload.Statuses))
		for _, status := range payload.Statuses {
			statusIDsByReference[status.StatusReference] = status.ID
		}

		workflow := JiraWorkflowConfiguration{
			ID:          source.ID,
			Name:        source.Name,
			Description: source.Description,
			StatusIDs:   make([]string, 0, len(source.Statuses)),
			Transitions: make([]JiraConfiguredWorkflowTransition, 0, len(source.Transitions)),
		}
		for _, status := range source.Statuses {
			statusID, ok := statusIDsByReference[status.StatusReference]
			if !ok {
				return nil, fmt.Errorf(
					"jira workflow %q references unknown status %q",
					source.Name,
					status.StatusReference,
				)
			}
			workflow.StatusIDs = append(workflow.StatusIDs, statusID)
		}
		for _, transition := range source.Transitions {
			toStatusID, ok := statusIDsByReference[transition.ToStatusReference]
			if !ok {
				return nil, fmt.Errorf(
					"jira workflow %q transition %q references unknown target status %q",
					source.Name,
					transition.Name,
					transition.ToStatusReference,
				)
			}
			configured := JiraConfiguredWorkflowTransition{
				ID:             transition.ID,
				Name:           transition.Name,
				Description:    transition.Description,
				Type:           JiraConfiguredWorkflowTransitionType(transition.Type),
				ToStatusID:     toStatusID,
				ValidatorCount: len(transition.Validators),
				ActionCount:    len(transition.Actions),
				TriggerCount:   len(transition.Triggers),
				ConditionCount: len(transition.Conditions),
			}
			if len(transition.Rules) > 0 && string(transition.Rules) != "null" && string(transition.Rules) != "{}" {
				configured.ConditionCount++
			}
			if configured.Type != JiraWorkflowTransitionInitial {
				for _, link := range transition.Links {
					fromStatusID, exists := statusIDsByReference[link.FromStatusReference]
					if !exists {
						return nil, fmt.Errorf(
							"jira workflow %q transition %q references unknown source status %q",
							source.Name,
							transition.Name,
							link.FromStatusReference,
						)
					}
					configured.FromStatusIDs = append(configured.FromStatusIDs, fromStatusID)
				}
			}
			workflow.Transitions = append(workflow.Transitions, configured)
		}

		result.IssueTypeWorkflowIDs[issueTypeID] = workflow.ID
		workflowsByID[workflow.ID] = workflow
	}

	workflowIDs := make([]string, 0, len(workflowsByID))
	for workflowID := range workflowsByID {
		workflowIDs = append(workflowIDs, workflowID)
	}
	sort.Strings(workflowIDs)
	for _, workflowID := range workflowIDs {
		result.Workflows = append(result.Workflows, workflowsByID[workflowID])
	}
	return result, nil
}

type cloudPage[T any] struct {
	StartAt    int  `json:"startAt"`
	MaxResults int  `json:"maxResults"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast"`
	Values     []T  `json:"values"`
}

type cloudIssueTypeScreenSchemeProject struct {
	IssueTypeScreenScheme struct {
		ID string `json:"id"`
	} `json:"issueTypeScreenScheme"`
	ProjectIDs []string `json:"projectIds"`
}

type cloudIssueTypeScreenSchemeMapping struct {
	IssueTypeID             string `json:"issueTypeId"`
	IssueTypeScreenSchemeID string `json:"issueTypeScreenSchemeId"`
	ScreenSchemeID          string `json:"screenSchemeId"`
}

type cloudScreenScheme struct {
	ID          json.Number      `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Screens     cloudScreenTypes `json:"screens"`
}

type cloudScreenTypes struct {
	Default json.Number `json:"default"`
	Create  json.Number `json:"create"`
	Edit    json.Number `json:"edit"`
	View    json.Number `json:"view"`
}

type cloudScreen struct {
	ID          json.Number `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
}

type cloudScreenTab struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

type cloudScreenField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func jiraNumberString(value json.Number) string {
	if value == "" || value == "0" {
		return ""
	}
	return value.String()
}

// GetProjectScreenConfiguration follows Jira's company-managed configuration
// chain from project to issue-type screen scheme, screen schemes, screens,
// tabs, and fields.
func (c *cloudClient) GetProjectScreenConfiguration(
	ctx context.Context,
	projectID string,
	projectKey string,
	issueTypeIDs []string,
) (*JiraProjectScreenConfiguration, error) {
	projectQuery := url.Values{}
	projectQuery.Add("projectId", projectID)
	projectQuery.Set("maxResults", "100")
	var projectSchemes cloudPage[cloudIssueTypeScreenSchemeProject]
	if err := jiraGetJSON(ctx, c, c.baseURL+"/issuetypescreenscheme/project?"+projectQuery.Encode(),
		&projectSchemes,
	); err != nil {
		return nil, err
	}
	if len(projectSchemes.Values) != 1 || projectSchemes.Values[0].IssueTypeScreenScheme.ID == "" {
		return nil, fmt.Errorf(
			"%w: Jira project %s resolved to %d issue type screen schemes",
			ErrScreenConfigurationNotAvailable,
			projectKey,
			len(projectSchemes.Values),
		)
	}
	issueTypeScreenSchemeID := projectSchemes.Values[0].IssueTypeScreenScheme.ID

	mappingQuery := url.Values{}
	mappingQuery.Add("issueTypeScreenSchemeId", issueTypeScreenSchemeID)
	mappingQuery.Set("maxResults", "100")
	var mappingsPage cloudPage[cloudIssueTypeScreenSchemeMapping]
	if err := jiraGetJSON(ctx, c, c.baseURL+"/issuetypescreenscheme/mapping?"+mappingQuery.Encode(),
		&mappingsPage,
	); err != nil {
		return nil, err
	}
	if !mappingsPage.IsLast && mappingsPage.Total > len(mappingsPage.Values) {
		return nil, fmt.Errorf(
			"%w: issue type screen scheme %s has more than 100 mappings",
			ErrScreenConfigurationNotAvailable,
			issueTypeScreenSchemeID,
		)
	}

	screenSchemeIDByIssueType := make(map[string]string)
	defaultScreenSchemeID := ""
	for _, mapping := range mappingsPage.Values {
		if mapping.IssueTypeScreenSchemeID != issueTypeScreenSchemeID {
			continue
		}
		if mapping.IssueTypeID == "default" {
			defaultScreenSchemeID = mapping.ScreenSchemeID
			continue
		}
		screenSchemeIDByIssueType[mapping.IssueTypeID] = mapping.ScreenSchemeID
	}

	screenSchemeIDs := make(map[string]bool)
	for _, issueTypeID := range issueTypeIDs {
		screenSchemeID := screenSchemeIDByIssueType[issueTypeID]
		if screenSchemeID == "" {
			screenSchemeID = defaultScreenSchemeID
		}
		if screenSchemeID == "" {
			return nil, fmt.Errorf(
				"%w: no screen scheme mapping for Jira issue type %s",
				ErrScreenConfigurationNotAvailable,
				issueTypeID,
			)
		}
		screenSchemeIDByIssueType[issueTypeID] = screenSchemeID
		screenSchemeIDs[screenSchemeID] = true
	}

	screenSchemes := make(map[string]cloudScreenScheme, len(screenSchemeIDs))
	for screenSchemeID := range screenSchemeIDs {
		screenSchemeQuery := url.Values{}
		screenSchemeQuery.Add("id", screenSchemeID)
		screenSchemeQuery.Set("maxResults", "100")
		var page cloudPage[cloudScreenScheme]
		if err := jiraGetJSON(ctx, c, c.baseURL+"/screenscheme?"+screenSchemeQuery.Encode(), &page); err != nil {
			return nil, err
		}
		if len(page.Values) != 1 {
			return nil, fmt.Errorf(
				"%w: Jira screen scheme %s returned %d definitions",
				ErrScreenConfigurationNotAvailable,
				screenSchemeID,
				len(page.Values),
			)
		}
		screenSchemes[screenSchemeID] = page.Values[0]
	}

	result := &JiraProjectScreenConfiguration{
		IssueTypeScreens: make(map[string]JiraIssueTypeScreens, len(issueTypeIDs)),
	}
	screenIDs := make(map[string]bool)
	for _, issueTypeID := range issueTypeIDs {
		scheme := screenSchemes[screenSchemeIDByIssueType[issueTypeID]]
		defaultID := jiraNumberString(scheme.Screens.Default)
		effective := JiraIssueTypeScreens{
			CreateScreenID: jiraNumberString(scheme.Screens.Create),
			EditScreenID:   jiraNumberString(scheme.Screens.Edit),
			ViewScreenID:   jiraNumberString(scheme.Screens.View),
		}
		if effective.CreateScreenID == "" {
			effective.CreateScreenID = defaultID
		}
		if effective.EditScreenID == "" {
			effective.EditScreenID = defaultID
		}
		if effective.ViewScreenID == "" {
			effective.ViewScreenID = defaultID
		}
		if effective.CreateScreenID == "" || effective.EditScreenID == "" || effective.ViewScreenID == "" {
			return nil, fmt.Errorf(
				"%w: Jira screen scheme %s lacks an effective create/edit/view screen",
				ErrScreenConfigurationNotAvailable,
				screenSchemeIDByIssueType[issueTypeID],
			)
		}
		result.IssueTypeScreens[issueTypeID] = effective
		screenIDs[effective.CreateScreenID] = true
		screenIDs[effective.EditScreenID] = true
		screenIDs[effective.ViewScreenID] = true
	}

	sortedScreenIDs := make([]string, 0, len(screenIDs))
	for screenID := range screenIDs {
		sortedScreenIDs = append(sortedScreenIDs, screenID)
	}
	sort.Strings(sortedScreenIDs)
	for _, screenID := range sortedScreenIDs {
		screenQuery := url.Values{}
		screenQuery.Add("id", screenID)
		screenQuery.Set("maxResults", "100")
		var screensPage cloudPage[cloudScreen]
		if err := jiraGetJSON(ctx, c, c.baseURL+"/screens?"+screenQuery.Encode(), &screensPage); err != nil {
			return nil, err
		}
		if len(screensPage.Values) != 1 {
			return nil, fmt.Errorf(
				"%w: Jira screen %s returned %d definitions",
				ErrScreenConfigurationNotAvailable,
				screenID,
				len(screensPage.Values),
			)
		}
		sourceScreen := screensPage.Values[0]
		screen := JiraScreenConfiguration{
			ID:          screenID,
			Name:        sourceScreen.Name,
			Description: sourceScreen.Description,
		}

		tabsQuery := url.Values{}
		tabsQuery.Set("projectKey", projectKey)
		var tabs []cloudScreenTab
		if err := jiraGetJSON(ctx, c, c.baseURL+"/screens/"+url.PathEscape(screenID)+"/tabs?"+tabsQuery.Encode(),
			&tabs,
		); err != nil {
			return nil, err
		}
		screen.TabCount = len(tabs)
		for _, tab := range tabs {
			fieldsQuery := url.Values{}
			fieldsQuery.Set("projectKey", projectKey)
			var fields []cloudScreenField
			if err := jiraGetJSON(ctx, c, c.baseURL+"/screens/"+url.PathEscape(screenID)+"/tabs/"+
				url.PathEscape(jiraNumberString(tab.ID))+"/fields?"+fieldsQuery.Encode(),
				&fields,
			); err != nil {
				return nil, err
			}
			for _, field := range fields {
				screen.Fields = append(screen.Fields, JiraScreenField(field))
			}
		}
		result.Screens = append(result.Screens, screen)
	}
	return result, nil
}

// GetProjectIssueTypeStatuses gets issue types with their available statuses for a project
func (c *cloudClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]JiraIssueTypeWithStatuses, error) {
	return jiraGetProjectIssueTypeStatuses(ctx, c, c.baseURL, projectKey)
}

// ================================================================
// Issue Methods
// ================================================================

// GetIssue gets a single issue by key
func (c *cloudClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*JiraIssue, error) {
	return jiraGetIssue(ctx, c, c.baseURL, issueKey, expand)
}

// GetIssueWatchers returns the identities behind an issue's watcher count.
// Jira can reject this request when the importing principal lacks the
// "View Watchers and Voters" permission; callers preserve that distinction
// instead of treating an unavailable list as an empty list.
func (c *cloudClient) GetIssueWatchers(ctx context.Context, issueKey string) (*JiraIssueWatchers, error) {
	return jiraGetIssueWatchers(ctx, c, c.baseURL, issueKey)
}

func (c *cloudClient) GetIssueComments(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraCommentContainer, error) {
	return jiraGetIssueComments(ctx, c, c.baseURL, issueKey, startAt, maxResults)
}

func (c *cloudClient) GetIssueWorklogs(ctx context.Context, issueKey string, startAt, maxResults int) (*JiraWorklogContainer, error) {
	return jiraGetIssueWorklogs(ctx, c, c.baseURL, issueKey, startAt, maxResults)
}

// GetIssueCount gets the total number of issues in a project using the new JQL search endpoint
func (c *cloudClient) GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error) {
	jql := `project = "` + escapeJQLString(projectKey) + `"`
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
	var result JQLSearchResponse
	if err := jiraRequestJSON(ctx, c, http.MethodPost, c.baseURL+"/search/jql", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BulkFetchIssues fetches multiple issues by their IDs or keys
// Uses POST /rest/api/3/issue/bulkfetch
func (c *cloudClient) BulkFetchIssues(ctx context.Context, req BulkFetchRequest) (*BulkFetchResponse, error) {
	var result BulkFetchResponse
	if err := jiraRequestJSON(ctx, c, http.MethodPost, c.baseURL+"/issue/bulkfetch", req, &result); err != nil {
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
	return jiraGetProjectVersions(ctx, c, c.baseURL, projectKey)
}

// ListBoards lists all Agile boards for a project.
//
// Jira's Agile API is paginated. Returning only the first page would silently
// miss boards on larger projects, which in turn drops every sprint that lives on
// later boards from the Windshift iteration import.
func (c *cloudClient) ListBoards(ctx context.Context, projectKey string) (*BoardListResult, error) {
	return jiraListBoards(ctx, c, c.agileURL, projectKey)
}

// GetBoardConfiguration gets Agile board columns, status mappings, and backing filter metadata.
func (c *cloudClient) GetBoardConfiguration(ctx context.Context, boardID int) (*JiraBoardConfiguration, error) {
	return jiraGetBoardConfiguration(ctx, c, c.agileURL, boardID)
}

// ListFilters lists saved filters associated with a project.
func (c *cloudClient) ListFilters(ctx context.Context, projectKey string) (*FilterSearchResult, error) {
	return jiraListFilters(ctx, c, c.baseURL, projectKey)
}

// GetFilter gets a saved filter with expanded JQL where available.
func (c *cloudClient) GetFilter(ctx context.Context, filterID string) (*JiraFilter, error) {
	return jiraGetFilter(ctx, c, c.baseURL, filterID)
}

// GetBoardSprints gets all sprints for a board.
//
// Jira embeds sprint results in pages too; importers need the full set so issue
// sprint custom fields can always resolve to a Windshift iteration.
func (c *cloudClient) GetBoardSprints(ctx context.Context, boardID int) (*SprintListResult, error) {
	return jiraListBoardSprints(ctx, c, c.agileURL, boardID)
}

// ================================================================
// Attachment Methods
// ================================================================

// DownloadAttachment downloads an attachment and returns the reader and content type
func (c *cloudClient) DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error) {
	return jiraDownloadAttachment(ctx, c.httpClient, c.limiter, c.authHeader, attachmentURL)
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
		return "", jiraErrorFromResponse(resp)
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

// ListObjectSchemas lists all object schemas in Assets. Cloud uses the
// workspace API exclusively; an unresolved workspace reports Assets as
// unavailable rather than falling back to the deprecated legacy endpoint.
func (c *cloudClient) ListObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error) {
	if c.assetsURL == "" {
		return nil, ErrAssetsNotAvailable
	}
	return c.listCurrentObjectSchemas(ctx)
}

func (c *cloudClient) listCurrentObjectSchemas(ctx context.Context) ([]AssetObjectSchema, error) {
	const pageSize = 100
	var schemas []AssetObjectSchema
	for startAt := 0; ; {
		query := url.Values{}
		query.Set("startAt", fmt.Sprintf("%d", startAt))
		query.Set("maxResults", fmt.Sprintf("%d", pageSize))
		query.Set("includeCounts", "true")
		resp, err := c.do(
			ctx,
			http.MethodGet,
			c.assetsURL+"/objectschema/list?"+query.Encode(),
			nil,
		)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, ErrAssetsNotAvailable
		}
		if resp.StatusCode != http.StatusOK {
			responseErr := jiraErrorFromResponse(resp)
			_ = resp.Body.Close()
			return nil, responseErr
		}

		var page struct {
			Values     []AssetObjectSchema `json:"values"`
			StartAt    int                 `json:"startAt"`
			MaxResults int                 `json:"maxResults"`
			Total      int                 `json:"total"`
			IsLast     bool                `json:"isLast"`
			Last       bool                `json:"last"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&page)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		schemas = append(schemas, page.Values...)
		next := page.StartAt + len(page.Values)
		if page.IsLast || page.Last || len(page.Values) == 0 || (page.Total > 0 && next >= page.Total) {
			return schemas, nil
		}
		startAt = next
	}
}

// GetObjectSchema gets a single object schema by ID
func (c *cloudClient) GetObjectSchema(ctx context.Context, schemaID string) (*AssetObjectSchema, error) {
	if c.assetsURL == "" {
		return nil, ErrAssetsNotAvailable
	}
	var schema AssetObjectSchema
	if err := jiraGetJSON(ctx, c, c.assetsURL+"/objectschema/"+url.PathEscape(schemaID), &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// ListObjectTypes lists all object types in a schema
func (c *cloudClient) ListObjectTypes(ctx context.Context, schemaID string) ([]AssetObjectType, error) {
	if c.assetsURL == "" {
		return nil, ErrAssetsNotAvailable
	}
	var types []AssetObjectType
	if err := jiraGetJSON(ctx, c, c.assetsURL+"/objectschema/"+url.PathEscape(schemaID)+"/objecttypes/flat", &types); err != nil {
		return nil, err
	}
	return types, nil
}

// GetObjectTypeAttributes gets all attributes for an object type
func (c *cloudClient) GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]AssetObjectAttribute, error) {
	if c.assetsURL == "" {
		return nil, ErrAssetsNotAvailable
	}
	var attrs []AssetObjectAttribute
	if err := jiraGetJSON(ctx, c, c.assetsURL+"/objecttype/"+url.PathEscape(objectTypeID)+"/attributes", &attrs); err != nil {
		return nil, err
	}
	normalizeAssetDefaultTypes(attrs)
	return attrs, nil
}

// SearchObjects searches for objects in a schema. Cloud uses the workspace
// object/aql API exclusively; an unresolved workspace reports Assets as
// unavailable rather than calling the deprecated legacy navlist endpoint.
func (c *cloudClient) SearchObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error) {
	if c.assetsURL == "" {
		return nil, ErrAssetsNotAvailable
	}
	return c.searchCurrentObjects(ctx, opts)
}

func (c *cloudClient) searchCurrentObjects(ctx context.Context, opts ObjectSearchOptions) (*ObjectSearchResult, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 100
	}
	startAt := (page - 1) * pageSize

	clauses := []string{
		"objectSchemaId = " + quoteAssetsAQLID(opts.ObjectSchemaID),
	}
	if opts.ObjectTypeID != "" {
		clauses = append(clauses, "objectTypeId = "+quoteAssetsAQLID(opts.ObjectTypeID))
	}
	if strings.TrimSpace(opts.IQL) != "" {
		clauses = append(clauses, "("+strings.TrimSpace(opts.IQL)+")")
	}

	query := url.Values{}
	query.Set("startAt", fmt.Sprintf("%d", startAt))
	query.Set("maxResults", fmt.Sprintf("%d", pageSize))
	query.Set("includeAttributes", fmt.Sprintf("%t", opts.IncludeAttributes))
	reqBody := map[string]string{"qlQuery": strings.Join(clauses, " AND ")}

	var current struct {
		Values               []AssetObject          `json:"values"`
		ObjectTypeAttributes []AssetObjectAttribute `json:"objectTypeAttributes"`
		MaxResults           int                    `json:"maxResults"`
		StartAt              int                    `json:"startAt"`
		Total                int                    `json:"total"`
		IsLast               bool                   `json:"isLast"`
		HasMoreResults       *bool                  `json:"hasMoreResults"`
		Last                 bool                   `json:"last"`
	}
	if err := jiraRequestJSON(ctx, c, http.MethodPost, c.assetsURL+"/object/aql?"+query.Encode(), reqBody, &current); err != nil {
		return nil, err
	}
	normalizeAssetDefaultTypes(current.ObjectTypeAttributes)

	// hasMoreResults is optional in the Assets response. Treat an absent flag
	// as unknown rather than "no more": negating a missing field previously
	// forced IsLast and silently truncated imports after the first page.
	isLast := current.IsLast || current.Last
	if current.HasMoreResults != nil && !*current.HasMoreResults {
		isLast = true
	}
	if current.Total > 0 && current.StartAt+len(current.Values) >= current.Total {
		isLast = true
	}

	return &ObjectSearchResult{
		ObjectEntries:        current.Values,
		ObjectTypeAttributes: current.ObjectTypeAttributes,
		PageNumber:           page,
		PageSize:             current.MaxResults,
		TotalFilterCount:     current.Total,
		StartIndex:           current.StartAt,
		ToIndex:              current.StartAt + len(current.Values),
		IsLast:               isLast,
	}, nil
}

func quoteAssetsAQLID(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return r < '0' || r > '9'
	}) == -1 {
		return value
	}
	escaped := strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`)
	return `"` + escaped + `"`
}

func normalizeAssetDefaultTypes(attributes []AssetObjectAttribute) {
	for index := range attributes {
		if attributes[index].DefaultType != nil {
			attributes[index].DefaultTypeID = attributes[index].DefaultType.ID
		}
	}
}

// GetObjectCount gets the total number of objects in a schema
func (c *cloudClient) GetObjectCount(ctx context.Context, schemaID string) (int, error) {
	schema, err := c.GetObjectSchema(ctx, schemaID)
	if err != nil {
		return 0, err
	}
	return schema.ObjectCount, nil
}
