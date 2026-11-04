package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/time/rate"
)

// jiraTransport is the shared request pipeline both deployment clients
// expose: rate-limited, retrying, read-only HTTP execution. Cloud and Data
// Center keep their own URL construction, auth, and response types; this
// interface is the kernel every call flows through.
type jiraTransport interface {
	do(ctx context.Context, method, reqURL string, body any) (*http.Response, error)
}

// summarizeJiraErrorBody makes an upstream error response useful in the UI
// and logs. Enterprise reverse proxies commonly return an HTML error page
// whose actual diagnostic follows a large script or style block. Strip that
// boilerplate before applying a generous safety bound so the useful message
// is not lost merely because it appeared late in the document.
func summarizeJiraErrorBody(b []byte) string {
	const maxRunes = 16 * 1024
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(empty response body)"
	}

	lower := strings.ToLower(s)
	if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
		title := ""
		if match := jiraHTMLTitlePattern.FindStringSubmatch(s); len(match) == 2 {
			title = bluemonday.StrictPolicy().Sanitize(match[1])
		}
		s = strings.TrimSpace(title + " " + bluemonday.StrictPolicy().Sanitize(s))
		s = html.UnescapeString(s)
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			return "(empty HTML response body)"
		}
	}

	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…(truncated)"
	}
	return s
}

var jiraHTMLTitlePattern = regexp.MustCompile(`(?is)<title(?:\s[^>]*)?>(.*?)</title\s*>`)

// maxJiraErrorBodyBytes bounds how much of an error response body is read
// into memory. Reverse proxies occasionally answer with multi-megabyte HTML
// pages; the diagnostic value lies in the first part of the document, so the
// read is capped well above any legitimate JSON error payload.
const maxJiraErrorBodyBytes = 1 << 20

// jiraErrorFromResponse maps a non-2xx Jira response to the package's error
// contract. Jira's response body is preserved on every branch — operators
// debugging "my token should work" need the upstream message (deprecated auth
// scheme, SSO required, account locked, captcha lockouts, reverse proxies
// stripping auth headers) rather than a bare sentinel error.
func jiraErrorFromResponse(resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxJiraErrorBodyBytes))
	if readErr != nil {
		return fmt.Errorf("failed to read Jira error response body: %w", readErr)
	}
	snippet := summarizeJiraErrorBody(body)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (jira said: %s)", ErrInvalidCredentials, snippet)
	case http.StatusForbidden:
		if strings.Contains(string(body), "rate limit") {
			return ErrRateLimited
		}
		return fmt.Errorf("%w (jira said: %s)", ErrForbidden, snippet)
	case http.StatusNotFound:
		return fmt.Errorf("%w (jira said: %s)", ErrNotFound, snippet)
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: status %d - %s", ErrAPIError, resp.StatusCode, snippet)
	}
}

// jiraRequestJSON executes one request, maps non-200 responses through the
// shared error contract, and decodes the JSON body into result.
func jiraRequestJSON(ctx context.Context, t jiraTransport, method, reqURL string, body, result any) error {
	resp, err := t.do(ctx, method, reqURL, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return jiraErrorFromResponse(resp)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// jiraGetJSON is the GET specialization of jiraRequestJSON.
func jiraGetJSON(ctx context.Context, t jiraTransport, reqURL string, result any) error {
	return jiraRequestJSON(ctx, t, http.MethodGet, reqURL, nil, result)
}

// jiraServiceDeskValues accumulates every page of a Jira Service Management
// start/limit endpoint. Callers supply the URL for one page; the envelope is
// always JiraServiceDeskPage regardless of deployment.
func jiraServiceDeskValues[T any](ctx context.Context, t jiraTransport, pageURL func(start int) string) ([]T, error) {
	var result []T
	for start := 0; ; {
		var page JiraServiceDeskPage[T]
		if err := jiraGetJSON(ctx, t, pageURL(start), &page); err != nil {
			return nil, err
		}
		result = append(result, page.Values...)
		if page.IsLastPage || len(page.Values) == 0 {
			return result, nil
		}
		start += len(page.Values)
	}
}

// jiraStartAtPage is the shared shape of Jira's startAt/maxResults paginated
// responses (Agile boards, sprints, saved-filter search) on both deployments.
type jiraStartAtPage[T any] struct {
	MaxResults int  `json:"maxResults"`
	StartAt    int  `json:"startAt"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast"`
	Values     []T  `json:"values"`
}

// jiraAccumulateStartAtValues walks a Jira startAt/maxResults endpoint to
// completion and merges every page into one aggregate. It is the single
// tested implementation of the pagination-boundary contract shared by the
// Agile and filter-search endpoints on both deployments: stop on isLast, an
// empty page, or when the aggregate reaches the server-reported total, and
// otherwise advance past the union of the echoed and requested offsets.
func jiraAccumulateStartAtValues[T any](
	ctx context.Context,
	t jiraTransport,
	defaultMaxResults int,
	pageURL func(startAt int) string,
) (*jiraStartAtPage[T], error) {
	aggregate := &jiraStartAtPage[T]{MaxResults: defaultMaxResults, IsLast: true}
	for startAt := 0; ; {
		var page jiraStartAtPage[T]
		if err := jiraGetJSON(ctx, t, pageURL(startAt), &page); err != nil {
			return nil, err
		}
		aggregate.Values = append(aggregate.Values, page.Values...)
		aggregate.Total = page.Total
		aggregate.IsLast = page.IsLast
		if page.MaxResults > 0 {
			aggregate.MaxResults = page.MaxResults
		}
		if page.IsLast || len(page.Values) == 0 || (page.Total > 0 && len(aggregate.Values) >= page.Total) {
			return aggregate, nil
		}
		if page.StartAt+len(page.Values) > startAt {
			startAt = page.StartAt + len(page.Values)
		} else {
			startAt += defaultMaxResults
		}
	}
}

// jiraProjectIssueTypeStatus is the shared shape of the
// /project/{key}/statuses response element on both deployments.
type jiraProjectIssueTypeStatus struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Subtask  bool         `json:"subtask"`
	Statuses []JiraStatus `json:"statuses"`
}

// jiraFetchProjectIssueTypeStatuses reads the issue-type/status membership a
// project exposes. Both deployments serve the same payload here.
func jiraFetchProjectIssueTypeStatuses(ctx context.Context, t jiraTransport, baseURL, projectKey string) ([]jiraProjectIssueTypeStatus, error) {
	var issueTypeStatuses []jiraProjectIssueTypeStatus
	if err := jiraGetJSON(ctx, t, baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", &issueTypeStatuses); err != nil {
		return nil, err
	}
	return issueTypeStatuses, nil
}

// jiraProjectWorkflowFromStatuses derives the importer-facing workflow from
// the project's issue-type status membership: the union of statuses across
// issue types, keyed by status ID.
func jiraProjectWorkflowFromStatuses(projectKey string, issueTypeStatuses []jiraProjectIssueTypeStatus) *JiraWorkflow {
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
	}
}

// jiraStartAtParams builds the shared startAt/maxResults query used by the
// issue comment and worklog endpoints.
func jiraStartAtParams(startAt, maxResults int) url.Values {
	if maxResults <= 0 {
		maxResults = 100
	}
	params := url.Values{}
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	return params
}

// jiraSearchParams builds the shared legacy GET /search query used by both
// deployments.
func jiraSearchParams(opts SearchOptions) url.Values {
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
	return params
}

// jiraBoardPageURL builds a board-list page URL. The project filter is part
// of the caller-visible contract and stays with the clients; this helper only
// encodes the paging parameters both deployments share.
func jiraStartAtPageURL(base string, startAt, maxResults int, extra url.Values) string {
	params := url.Values{}
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	for key, values := range extra {
		for _, value := range values {
			params.Set(key, value)
		}
	}
	return base + "?" + params.Encode()
}

// jiraDownloadAttachment streams an attachment from a URL previously returned
// by the Jira API. It shares the client's rate limit, throttle cooldown, and
// retry/backoff contract with the JSON endpoints. The body stays open; the
// caller owns closing it.
func jiraDownloadAttachment(
	ctx context.Context,
	httpClient *http.Client,
	limiter *rate.Limiter,
	throttle *jiraThrottle,
	authHeader string,
	attachmentURL string,
) (io.ReadCloser, string, error) {
	resp, err := jiraRetryRequest(ctx, httpClient, limiter, throttle, 0, nil, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, attachmentURL, http.NoBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		downloadErr := jiraErrorFromResponse(resp)
		_ = resp.Body.Close()
		return nil, "", downloadErr
	}

	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// ================================================================
// Typed startAt/maxResults accumulators shared by both deployments.
// ================================================================

const jiraAgilePageSize = 50

// jiraListBoards fetches every Agile board, optionally scoped to a project.
func jiraListBoards(ctx context.Context, t jiraTransport, agileURL, projectKey string) (*BoardListResult, error) {
	extra := url.Values{}
	if projectKey != "" {
		extra.Set("projectKeyOrId", projectKey)
	}
	agg, err := jiraAccumulateStartAtValues[JiraBoard](ctx, t, jiraAgilePageSize, func(startAt int) string {
		return jiraStartAtPageURL(agileURL+"/board", startAt, jiraAgilePageSize, extra)
	})
	if err != nil {
		return nil, err
	}
	return &BoardListResult{MaxResults: agg.MaxResults, Total: agg.Total, IsLast: agg.IsLast, Values: agg.Values}, nil
}

// jiraGetBoardConfiguration fetches one board's columns and status mappings.
func jiraGetBoardConfiguration(ctx context.Context, t jiraTransport, agileURL string, boardID int) (*JiraBoardConfiguration, error) {
	var config JiraBoardConfiguration
	if err := jiraGetJSON(ctx, t, fmt.Sprintf("%s/board/%d/configuration", agileURL, boardID), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// jiraListBoardSprints fetches every sprint on a board.
func jiraListBoardSprints(ctx context.Context, t jiraTransport, agileURL string, boardID int) (*SprintListResult, error) {
	agg, err := jiraAccumulateStartAtValues[JiraSprint](ctx, t, jiraAgilePageSize, func(startAt int) string {
		return jiraStartAtPageURL(fmt.Sprintf("%s/board/%d/sprint", agileURL, boardID), startAt, jiraAgilePageSize, nil)
	})
	if err != nil {
		return nil, err
	}
	return &SprintListResult{MaxResults: agg.MaxResults, Total: agg.Total, IsLast: agg.IsLast, Values: agg.Values}, nil
}

// jiraGetProject fetches one project. The /project/{key} endpoint is
// identical on Cloud and Data Center; only the API-versioned base URL differs.
func jiraGetProject(ctx context.Context, t jiraTransport, baseURL, projectKey string) (*JiraProject, error) {
	var project JiraProject
	if err := jiraGetJSON(ctx, t, baseURL+"/project/"+url.PathEscape(projectKey), &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// jiraListFilters fetches every saved filter associated with a project.
func jiraListFilters(ctx context.Context, t jiraTransport, baseURL, projectKey string) (*FilterSearchResult, error) {
	project, err := jiraGetProject(ctx, t, baseURL, projectKey)
	if err != nil {
		return nil, err
	}
	extra := url.Values{}
	extra.Set("expand", "jql,description,owner,viewUrl")
	if project != nil && strings.TrimSpace(project.ID) != "" {
		extra.Set("projectId", project.ID)
	}
	agg, err := jiraAccumulateStartAtValues[JiraFilter](ctx, t, jiraAgilePageSize, func(startAt int) string {
		return jiraStartAtPageURL(baseURL+"/filter/search", startAt, jiraAgilePageSize, extra)
	})
	if err != nil {
		return nil, err
	}
	return &FilterSearchResult{MaxResults: agg.MaxResults, Total: agg.Total, IsLast: agg.IsLast, Values: agg.Values}, nil
}

// jiraGetFilter fetches one saved filter with expanded JQL where available.
func jiraGetFilter(ctx context.Context, t jiraTransport, baseURL, filterID string) (*JiraFilter, error) {
	params := url.Values{}
	params.Set("expand", "jql,description,owner,viewUrl")
	var filter JiraFilter
	if err := jiraGetJSON(ctx, t, baseURL+"/filter/"+url.PathEscape(filterID)+"?"+params.Encode(), &filter); err != nil {
		return nil, err
	}
	return &filter, nil
}

// ================================================================
// Shared issue endpoint helpers. These paths are identical on Cloud and
// Data Center; only the API-versioned base URL differs.
// ================================================================

// jiraGetIssue fetches one issue by key, with optional expand.
func jiraGetIssue(ctx context.Context, t jiraTransport, baseURL, issueKey string, expand []string) (*JiraIssue, error) {
	params := url.Values{}
	if len(expand) > 0 {
		params.Set("expand", strings.Join(expand, ","))
	}

	reqURL := baseURL + "/issue/" + url.PathEscape(issueKey)
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	var issue JiraIssue
	if err := jiraGetJSON(ctx, t, reqURL, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// jiraGetIssueWatchers fetches the identities behind an issue's watcher count.
func jiraGetIssueWatchers(ctx context.Context, t jiraTransport, baseURL, issueKey string) (*JiraIssueWatchers, error) {
	var result JiraIssueWatchers
	if err := jiraGetJSON(ctx, t, baseURL+"/issue/"+url.PathEscape(issueKey)+"/watchers", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// jiraGetIssueComments fetches one page of issue comments.
func jiraGetIssueComments(ctx context.Context, t jiraTransport, baseURL, issueKey string, startAt, maxResults int) (*JiraCommentContainer, error) {
	reqURL := baseURL + "/issue/" + url.PathEscape(issueKey) + "/comment?" + jiraStartAtParams(startAt, maxResults).Encode()
	var result JiraCommentContainer
	if err := jiraGetJSON(ctx, t, reqURL, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// jiraGetIssueWorklogs fetches one page of issue worklogs.
func jiraGetIssueWorklogs(ctx context.Context, t jiraTransport, baseURL, issueKey string, startAt, maxResults int) (*JiraWorklogContainer, error) {
	reqURL := baseURL + "/issue/" + url.PathEscape(issueKey) + "/worklog?" + jiraStartAtParams(startAt, maxResults).Encode()
	var result JiraWorklogContainer
	if err := jiraGetJSON(ctx, t, reqURL, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// jiraGetProjectVersions fetches all versions for a project.
func jiraGetProjectVersions(ctx context.Context, t jiraTransport, baseURL, projectKey string) ([]JiraVersion, error) {
	var versions []JiraVersion
	if err := jiraGetJSON(ctx, t, baseURL+"/project/"+url.PathEscape(projectKey)+"/versions", &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// jiraSearchIssuesLegacy runs the legacy GET /search endpoint shared by both
// deployments.
func jiraSearchIssuesLegacy(ctx context.Context, t jiraTransport, baseURL string, opts SearchOptions) (*SearchResult, error) {
	var result SearchResult
	if err := jiraGetJSON(ctx, t, baseURL+"/search?"+jiraSearchParams(opts).Encode(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// jiraGetProjectIssueTypeStatuses fetches issue types with their available
// statuses for a project.
func jiraGetProjectIssueTypeStatuses(ctx context.Context, t jiraTransport, baseURL, projectKey string) ([]JiraIssueTypeWithStatuses, error) {
	var result []JiraIssueTypeWithStatuses
	if err := jiraGetJSON(ctx, t, baseURL+"/project/"+url.PathEscape(projectKey)+"/statuses", &result); err != nil {
		return nil, err
	}
	return result, nil
}
