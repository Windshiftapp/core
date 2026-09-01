package scm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/models"
)

// Compile-time check that GiteaProvider implements the optional
// interfaces it claims. See GitHubProvider for the same pattern.
var (
	_ Provider                     = (*GiteaProvider)(nil)
	_ ReleaseProvider              = (*GiteaProvider)(nil)
	_ CommitProvider               = (*GiteaProvider)(nil)
	_ RefProvider                  = (*GiteaProvider)(nil)
	_ IssueCommentProvider         = (*GiteaProvider)(nil) // WI-426: drives the "@agent" PR-comment trigger
	_ PullRequestReviewProvider    = (*GiteaProvider)(nil)
	_ RepositoryPermissionProvider = (*GiteaProvider)(nil)
)

// GiteaProvider implements the Provider interface for Gitea/Forgejo
type GiteaProvider struct {
	baseProvider
	baseURL      string
	authMethod   models.SCMAuthMethod
	accessToken  string
	clientID     string
	clientSecret string
}

// NewGiteaProvider creates a new Gitea provider instance
func NewGiteaProvider(cfg ProviderConfig) (*GiteaProvider, error) {
	if cfg.BaseURL == "" {
		return nil, ErrInvalidCredentials // Gitea requires a base URL
	}

	// Normalize base URL - remove trailing slash
	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")

	var accessToken string
	switch cfg.AuthMethod {
	case models.SCMAuthMethodOAuth:
		accessToken = cfg.OAuthAccessToken
	case models.SCMAuthMethodPAT:
		accessToken = cfg.PersonalAccessToken
	}

	provider := &GiteaProvider{
		baseURL:      baseURL,
		authMethod:   cfg.AuthMethod,
		accessToken:  accessToken,
		clientID:     cfg.OAuthClientID,
		clientSecret: cfg.OAuthClientSecret,
	}
	provider.baseProvider = baseProvider{
		httpClient:          newSCMHTTPClient(30 * time.Second),
		setAuthHeader:       provider.setAuthHeader,
		handleErrorResponse: provider.handleErrorResponse,
	}

	return provider, nil
}

// GetType returns the provider type
func (g *GiteaProvider) GetType() models.SCMProviderType {
	return models.SCMProviderTypeGitea
}

// apiURL constructs the full API URL for a given path
func (g *GiteaProvider) apiURL(path string) string {
	return fmt.Sprintf("%s/api/v1%s", g.baseURL, path)
}

// setAuthHeader sets the appropriate authentication header based on auth method
func (g *GiteaProvider) setAuthHeader(req *http.Request) {
	if g.accessToken == "" {
		return
	}

	// Gitea uses different auth header format for PAT vs OAuth
	// PAT: Authorization: token <access_token>
	// OAuth: Authorization: bearer <access_token>
	switch g.authMethod {
	case models.SCMAuthMethodOAuth:
		req.Header.Set("Authorization", "bearer "+g.accessToken)
	default:
		// PAT and default
		req.Header.Set("Authorization", "token "+g.accessToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}

// handleErrorResponse handles non-success HTTP responses
func (g *GiteaProvider) handleErrorResponse(resp *http.Response) error {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("%w: failed to read response body: %v", ErrProviderError, readErr)
	}
	bodyStr := string(body)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrForbidden, bodyStr)
		}
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnprocessableEntity:
		// Check for duplicate PR error
		if strings.Contains(bodyStr, "already exists") || strings.Contains(bodyStr, "pull request already exists") {
			return ErrAlreadyExists
		}
		return fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, bodyStr)
	default:
		return fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, bodyStr)
	}
}

// TestConnection tests if the provider connection is working
func (g *GiteaProvider) TestConnection(ctx context.Context) error {
	return g.doJSON(ctx, http.MethodGet, g.apiURL("/user"), http.NoBody, http.StatusOK, nil)
}

// ListRepositories lists all accessible repositories
func (g *GiteaProvider) ListRepositories(ctx context.Context, opts ListRepositoriesOptions) ([]Repository, error) {
	page := opts.Page
	if page == 0 {
		page = 1
	}
	limit := opts.PerPage
	if limit == 0 {
		limit = 50 // Gitea default
	}

	reqURL := fmt.Sprintf("%s?page=%d&limit=%d", g.apiURL("/user/repos"), page, limit)

	var giteaRepos []giteaRepo
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaRepos); err != nil {
		return nil, err
	}

	repos := make([]Repository, len(giteaRepos))
	for i, r := range giteaRepos {
		repos[i] = r.toRepository()
	}
	return repos, nil
}

// GetRepository gets details about a specific repository
func (g *GiteaProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo)))

	var giteaRepoResp giteaRepo
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaRepoResp); err != nil {
		return nil, err
	}

	result := giteaRepoResp.toRepository()
	return &result, nil
}

// ListBranches lists branches for a repository, paginated up to maxBranches.
func (g *GiteaProvider) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	const perPage = 50
	const maxBranches = 1000

	var branches []Branch
	for page := 1; ; page++ {
		reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/branches?page=%d&limit=%d",
			url.PathEscape(owner), url.PathEscape(repo), page, perPage))

		var giteaBranches []giteaBranch
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaBranches); err != nil {
			return nil, err
		}
		for _, b := range giteaBranches {
			branches = append(branches, b.toBranch())
			if len(branches) >= maxBranches {
				return branches, nil
			}
		}
		if len(giteaBranches) < perPage {
			break
		}
	}
	return branches, nil
}

// ListPullRequests lists pull requests for a repository
func (g *GiteaProvider) ListPullRequests(ctx context.Context, owner, repo string, opts ListPROptions) ([]PullRequest, error) {
	page := opts.Page
	if page == 0 {
		page = 1
	}
	limit := opts.PerPage
	if limit == 0 {
		limit = 50
	}

	state := opts.State
	if state == "" {
		state = "open"
	}

	// Gitea and GitHub use different sort tokens for the same concept
	// (e.g. "updated" on GitHub is "recentupdate" on Gitea). Translate the
	// provider-agnostic value the caller passes into the token this forge
	// understands; an unknown value is forwarded verbatim so the server
	// can reject it rather than silently defaulting to "oldest".
	sortParam := opts.Sort
	if sortParam == "updated" {
		sortParam = "recentupdate"
	}

	reqURL := fmt.Sprintf("%s?state=%s&page=%d&limit=%d",
		g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo))),
		state, page, limit)
	if sortParam != "" {
		reqURL += "&sort=" + sortParam
	}
	if opts.Direction != "" {
		reqURL += "&direction=" + opts.Direction
	}

	var giteaPRs []giteaPullRequest
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaPRs); err != nil {
		return nil, err
	}

	prs := make([]PullRequest, len(giteaPRs))
	for i, pr := range giteaPRs {
		prs[i] = pr.toPullRequest()
	}
	return prs, nil
}

// GetPullRequest gets details about a specific pull request
func (g *GiteaProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number))

	var giteaPR giteaPullRequest
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaPR); err != nil {
		return nil, err
	}

	pr := giteaPR.toPullRequest()
	return &pr, nil
}

// ListPullRequestCommits lists commits in a PR. Paginated up to maxPRCommits.
func (g *GiteaProvider) ListPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]Commit, error) {
	const perPage = 50
	const maxPRCommits = 250

	var all []Commit
	for page := 1; ; page++ {
		reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?page=%d&limit=%d",
			url.PathEscape(owner), url.PathEscape(repo), number, page, perPage))
		var giteaCommits []giteaCommit
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaCommits); err != nil {
			return nil, err
		}
		for _, c := range giteaCommits {
			all = append(all, c.toCommit())
			if len(all) >= maxPRCommits {
				return all, nil
			}
		}
		if len(giteaCommits) < perPage {
			break
		}
	}
	return all, nil
}

// GetCommit gets details about a specific commit
func (g *GiteaProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/git/commits/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha)))

	var giteaCommitResp giteaCommit
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaCommitResp); err != nil {
		return nil, err
	}

	commit := giteaCommitResp.toCommit()
	return &commit, nil
}

// ListCommits lists commits from a repository branch/tag, newest first.
func (g *GiteaProvider) ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) ([]Commit, error) {
	page := opts.Page
	if page == 0 {
		page = 1
	}
	limit := opts.PerPage
	if limit == 0 {
		limit = 50
	}

	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("limit", fmt.Sprintf("%d", limit))
	if opts.Sha != "" {
		q.Set("sha", opts.Sha)
	}
	if opts.Since != nil && !opts.Since.IsZero() {
		q.Set("since", opts.Since.Format(time.RFC3339))
	}

	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/commits?%s", url.PathEscape(owner), url.PathEscape(repo), q.Encode()))
	var giteaCommits []giteaCommit
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaCommits); err != nil {
		return nil, err
	}

	commits := make([]Commit, len(giteaCommits))
	for i, c := range giteaCommits {
		commits[i] = c.toCommit()
	}
	return commits, nil
}

// CreateBranch creates a new branch
// Note: Gitea has a direct branch creation API, unlike GitHub which uses git refs
func (g *GiteaProvider) CreateBranch(ctx context.Context, owner, repo, branchName, baseBranch string) error {
	createURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/branches", url.PathEscape(owner), url.PathEscape(repo)))

	body := map[string]string{
		"new_branch_name": branchName,
		"old_branch_name": baseBranch,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	return g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, nil)
}

// CreatePullRequest creates a new pull request
func (g *GiteaProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error) {
	createURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo)))

	body := map[string]any{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  opts.HeadBranch,
		"base":  opts.BaseBranch,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var giteaPR giteaPullRequest
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &giteaPR); err != nil {
		return nil, err
	}

	pr := giteaPR.toPullRequest()
	return &pr, nil
}

// giteaComment is the Gitea issue/PR comment payload. In Gitea, pull-request
// comments live on the issue-comments endpoint (a PR shares its issue index),
// so the same shape covers both.
type giteaComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      giteaUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c giteaComment) toIssueComment() IssueComment {
	return IssueComment{
		ID:        c.ID,
		Kind:      "issue_comment",
		Body:      c.Body,
		User:      c.User.toUser(),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// ListIssueComments lists all comments on an issue or pull request. Gitea routes
// PR comments through the issue-comments endpoint (the PR's index doubles as its
// issue index), so this serves both. Paginated to gather every comment — the
// "@agent" PR-comment poller (WI-426) relies on seeing the full list.
func (g *GiteaProvider) ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	const perPage = 50
	var comments []IssueComment
	for page := 1; ; page++ {
		reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/issues/%d/comments?page=%d&limit=%d",
			url.PathEscape(owner), url.PathEscape(repo), number, page, perPage))

		var giteaComments []giteaComment
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaComments); err != nil {
			return nil, err
		}
		for _, c := range giteaComments {
			comments = append(comments, c.toIssueComment())
		}
		if len(giteaComments) < perPage {
			break
		}
	}
	return comments, nil
}

// ListPullRequestReviewEvents normalizes Gitea review bodies and their inline
// comments. Inline comments are nested under a review in the Gitea API.
func (g *GiteaProvider) ListPullRequestReviewEvents(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	const perPage = 50
	var events []IssueComment
	for page := 1; ; page++ {
		reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?page=%d&limit=%d", url.PathEscape(owner), url.PathEscape(repo), number, page, perPage))
		var reviews []struct {
			ID          int64     `json:"id"`
			Body        string    `json:"body"`
			User        giteaUser `json:"user"`
			SubmittedAt time.Time `json:"submitted_at"`
		}
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &reviews); err != nil {
			return nil, err
		}
		for _, review := range reviews {
			if strings.TrimSpace(review.Body) != "" {
				events = append(events, IssueComment{ID: review.ID, Kind: "review", Body: review.Body, User: review.User.toUser(), CreatedAt: review.SubmittedAt, UpdatedAt: review.SubmittedAt})
			}
			commentsURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews/%d/comments", url.PathEscape(owner), url.PathEscape(repo), number, review.ID))
			var comments []struct {
				ID          int64     `json:"id"`
				Body        string    `json:"body"`
				User        giteaUser `json:"user"`
				Path        string    `json:"path"`
				NewPosition int       `json:"new_position"`
				CreatedAt   time.Time `json:"created_at"`
				UpdatedAt   time.Time `json:"updated_at"`
			}
			if err := g.doJSON(ctx, "GET", commentsURL, http.NoBody, http.StatusOK, &comments); err != nil {
				return nil, err
			}
			for _, c := range comments {
				events = append(events, IssueComment{ID: c.ID, Kind: "review_comment", Body: c.Body, User: c.User.toUser(), Path: c.Path, Line: c.NewPosition, ThreadID: fmt.Sprintf("%d", review.ID), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
			}
		}
		if len(reviews) < perPage {
			break
		}
	}
	return events, nil
}

// CanUserWriteRepository checks Gitea's effective collaborator permission.
func (g *GiteaProvider) CanUserWriteRepository(ctx context.Context, owner, repo, username string) (bool, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(username)))
	var out struct {
		Permission string `json:"permission"`
	}
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &out); err != nil {
		return false, err
	}
	switch strings.ToLower(out.Permission) {
	case "admin", "owner", "write", "push", "triage":
		return true, nil
	default:
		return false, nil
	}
}

// CreateIssueComment posts a comment on an issue or pull request and returns the
// new comment's id. Used for the agent's reply-back, which carries
// models.AgentCommentMarker so the poller never re-triggers on it.
func (g *GiteaProvider) CreateIssueComment(ctx context.Context, owner, repo string, number int, commentBody string) (int64, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/issues/%d/comments",
		url.PathEscape(owner), url.PathEscape(repo), number))

	bodyJSON, err := json.Marshal(map[string]string{"body": commentBody})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var created giteaComment
	if err := g.doJSON(ctx, "POST", reqURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// UpdateIssueComment edits an existing issue/PR comment by its id.
func (g *GiteaProvider) UpdateIssueComment(ctx context.Context, owner, repo string, _ int, commentID int64, commentBody string) error {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/issues/comments/%d",
		url.PathEscape(owner), url.PathEscape(repo), commentID))

	bodyJSON, err := json.Marshal(map[string]string{"body": commentBody})
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}
	return g.doJSON(ctx, "PATCH", reqURL, strings.NewReader(string(bodyJSON)), http.StatusOK, nil)
}

// CreateRelease creates a new release in a repository
func (g *GiteaProvider) CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error) {
	createURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/releases", url.PathEscape(owner), url.PathEscape(repo)))

	body := map[string]any{
		"tag_name":   opts.TagName,
		"name":       opts.Name,
		"body":       opts.Body,
		"draft":      opts.IsDraft,
		"prerelease": opts.IsPrerelease,
	}
	if opts.TargetCommitish != "" {
		body["target_commitish"] = opts.TargetCommitish
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var giteaRel giteaRelease
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &giteaRel); err != nil {
		return nil, err
	}

	release := giteaRel.toRelease()
	return &release, nil
}

// ListReleases lists releases for a repository
func (g *GiteaProvider) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/releases", url.PathEscape(owner), url.PathEscape(repo)))

	var giteaReleases []giteaRelease
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaReleases); err != nil {
		return nil, err
	}

	releases := make([]Release, 0, len(giteaReleases))
	for _, r := range giteaReleases {
		releases = append(releases, r.toRelease())
	}
	return releases, nil
}

// ListTags lists tags for a repository, paginated up to maxTags.
// Filters by `since` against each tag's commit date.
func (g *GiteaProvider) ListTags(ctx context.Context, owner, repo string, since time.Time) ([]Tag, error) {
	const perPage = 50
	const maxTags = 500

	type giteaTag struct {
		Name   string `json:"name"`
		Commit struct {
			SHA     string    `json:"sha"`
			Created time.Time `json:"created"`
		} `json:"commit"`
	}

	var tags []Tag
	for page := 1; ; page++ {
		reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/tags?page=%d&limit=%d",
			url.PathEscape(owner), url.PathEscape(repo), page, perPage))

		var raw []giteaTag
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, t := range raw {
			created := t.Commit.Created
			if created.IsZero() {
				if c, err := g.GetCommit(ctx, owner, repo, t.Commit.SHA); err == nil {
					created = c.CreatedAt
				}
			}
			if !since.IsZero() && created.Before(since) {
				continue
			}
			tags = append(tags, Tag{Name: t.Name, SHA: t.Commit.SHA, CreatedAt: created})
			if len(tags) >= maxTags {
				return tags, nil
			}
		}
		if len(raw) < perPage {
			break
		}
	}
	return tags, nil
}

// CompareCommits returns commits reachable from `head` but not `base`,
// using Gitea's /compare endpoint. Capped at maxCompareCommits.
func (g *GiteaProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]Commit, error) {
	const maxCompareCommits = 500

	type giteaCompare struct {
		Commits []giteaCommit `json:"commits"`
	}

	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/compare/%s...%s",
		url.PathEscape(owner), url.PathEscape(repo),
		url.PathEscape(base), url.PathEscape(head)))

	var resp giteaCompare
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &resp); err != nil {
		return nil, err
	}

	out := make([]Commit, 0, len(resp.Commits))
	for _, c := range resp.Commits {
		out = append(out, c.toCommit())
		if len(out) >= maxCompareCommits {
			break
		}
	}
	return out, nil
}

// RegisterWebhook registers a webhook for repository events
func (g *GiteaProvider) RegisterWebhook(ctx context.Context, owner, repo string, opts WebhookOptions) (*WebhookRegistration, error) {
	createURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(owner), url.PathEscape(repo)))

	contentType := opts.ContentType
	if contentType == "" {
		contentType = "json"
	}

	// Map events to Gitea event names if needed
	events := opts.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request"}
	}

	body := map[string]any{
		"type":   "gitea", // Gitea webhook type
		"active": true,
		"events": events,
		"config": map[string]string{
			"url":          opts.URL,
			"content_type": contentType,
			"secret":       opts.Secret,
		},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var hook giteaWebhook
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &hook); err != nil {
		return nil, err
	}

	return &WebhookRegistration{
		ID:        fmt.Sprintf("%d", hook.ID),
		URL:       hook.Config.URL,
		Events:    hook.Events,
		IsActive:  hook.Active,
		CreatedAt: hook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a registered webhook
func (g *GiteaProvider) DeleteWebhook(ctx context.Context, owner, repo, webhookID string) error {
	deleteURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/hooks/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(webhookID)))
	return g.doJSON(ctx, "DELETE", deleteURL, http.NoBody, http.StatusNoContent, nil)
}

// =============================================================================
// Gitea API response types
// =============================================================================

type giteaRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	HTMLURL       string    `json:"html_url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	Archived      bool      `json:"archived"`
	Owner         giteaUser `json:"owner"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (r giteaRepo) toRepository() Repository {
	return Repository{
		ID:            fmt.Sprintf("%d", r.ID),
		Name:          r.Name,
		FullName:      r.FullName,
		Description:   r.Description,
		URL:           r.HTMLURL,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		IsPrivate:     r.Private,
		IsArchived:    r.Archived,
		Owner:         r.Owner.Username,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

type giteaBranch struct {
	Name      string            `json:"name"`
	Protected bool              `json:"protected"`
	Commit    giteaBranchCommit `json:"commit"`
}

type giteaBranchCommit struct {
	ID string `json:"id"` // SHA
}

func (b giteaBranch) toBranch() Branch {
	return Branch{
		Name:      b.Name,
		SHA:       b.Commit.ID,
		Protected: b.Protected,
	}
}

type giteaPullRequest struct {
	ID        int64         `json:"id"`
	Number    int64         `json:"number"` // Gitea uses "index" in some contexts but "number" in API responses
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     string        `json:"state"` // open, closed
	HTMLURL   string        `json:"html_url"`
	Merged    bool          `json:"merged"`
	Head      giteaPRBranch `json:"head"`
	Base      giteaPRBranch `json:"base"`
	User      giteaUser     `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	MergedAt  *time.Time    `json:"merged_at"`
	ClosedAt  *time.Time    `json:"closed_at"`
}

type giteaPRBranch struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}

func (pr giteaPullRequest) toPullRequest() PullRequest {
	state := pr.State
	if pr.Merged {
		state = "merged"
	}

	return PullRequest{
		ID:         int(pr.ID),
		Number:     int(pr.Number),
		Title:      pr.Title,
		Body:       pr.Body,
		State:      state,
		URL:        pr.HTMLURL,
		HeadBranch: pr.Head.Ref,
		HeadRepo:   pr.Head.Repo.FullName,
		HeadSHA:    pr.Head.SHA,
		BaseBranch: pr.Base.Ref,
		IsMerged:   pr.Merged,
		IsDraft:    false, // Gitea PRs don't have draft state in the same way
		Author:     pr.User.toUser(),
		CreatedAt:  pr.CreatedAt,
		UpdatedAt:  pr.UpdatedAt,
		MergedAt:   pr.MergedAt,
		ClosedAt:   pr.ClosedAt,
	}
}

type giteaCommit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
		Committer struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	Author    *giteaUser `json:"author"`
	Committer *giteaUser `json:"committer"`
}

func (c giteaCommit) toCommit() Commit {
	author := User{
		Name:  c.Commit.Author.Name,
		Email: c.Commit.Author.Email,
	}
	if c.Author != nil {
		author = c.Author.toUser()
	}

	committer := User{
		Name:  c.Commit.Committer.Name,
		Email: c.Commit.Committer.Email,
	}
	if c.Committer != nil {
		committer = c.Committer.toUser()
	}

	return Commit{
		SHA:       c.SHA,
		Message:   c.Commit.Message,
		URL:       c.HTMLURL,
		Author:    author,
		Committer: committer,
		CreatedAt: c.Commit.Author.Date,
	}
}

type giteaUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"login"` // Gitea uses "login" like GitHub
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func (u giteaUser) toUser() User {
	name := u.FullName
	if name == "" {
		name = u.Username
	}
	return User{
		ID:        fmt.Sprintf("%d", u.ID),
		Username:  u.Username,
		Name:      name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
	}
}

type giteaRelease struct {
	ID          int64      `json:"id"`
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	HTMLURL     string     `json:"html_url"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
}

func (r giteaRelease) toRelease() Release {
	status := "released"
	if r.Draft {
		status = "draft"
	} else if r.Prerelease {
		status = "prerelease"
	}
	return Release{
		ID:           fmt.Sprintf("%d", r.ID),
		TagName:      r.TagName,
		Name:         r.Name,
		Body:         r.Body,
		URL:          r.HTMLURL,
		Status:       status,
		IsDraft:      r.Draft,
		IsPrerelease: r.Prerelease,
		CreatedAt:    r.CreatedAt,
		PublishedAt:  r.PublishedAt,
		ReleasedAt:   r.PublishedAt,
	}
}

type giteaWebhook struct {
	ID        int64              `json:"id"`
	Type      string             `json:"type"`
	Events    []string           `json:"events"`
	Active    bool               `json:"active"`
	Config    giteaWebhookConfig `json:"config"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type giteaWebhookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
}

// =============================================================================
// OAuth methods
// =============================================================================

// performTokenRequest performs an OAuth token exchange HTTP request against the Gitea token endpoint
// and parses the response. It handles both authorization code exchange and refresh token flows.
//
// A 400 response (or a 200 body) with `error: invalid_grant` maps to
// ErrRefreshTokenInvalid so the caller can mark the stored credential as
// dead. Gitea/Forgejo also return invalid_grant when a rotated refresh
// token has already been consumed by a concurrent refresh — this is a
// terminal condition either way.
func (g *GiteaProvider) performTokenRequest(ctx context.Context, params url.Values) (*OAuthTokens, error) {
	tokenURL := fmt.Sprintf("%s/login/oauth/access_token", g.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req) //nolint:gosec // URL from admin-configured Gitea baseURL
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("%w: failed to read response body: %v", ErrProviderError, readErr)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusBadRequest && bytes.Contains(body, []byte(`"invalid_grant"`)) {
			return nil, fmt.Errorf("%w: %s", ErrRefreshTokenInvalid, string(body))
		}
		return nil, fmt.Errorf("%w: %s", ErrProviderError, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token,omitempty"`
		ExpiresIn    int    `json:"expires_in,omitempty"`
		Scope        string `json:"scope,omitempty"`
		Error        string `json:"error,omitempty"`
		ErrorDesc    string `json:"error_description,omitempty"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.Error == "invalid_grant" {
		return nil, fmt.Errorf("%w: %s", ErrRefreshTokenInvalid, tokenResp.ErrorDesc)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%w: %s - %s", ErrProviderError, tokenResp.Error, tokenResp.ErrorDesc)
	}

	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		Scope:        tokenResp.Scope,
	}

	if tokenResp.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		tokens.ExpiresAt = &expiresAt
	}

	return tokens, nil
}

// ExchangeCode exchanges an OAuth authorization code for access tokens
func (g *GiteaProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error) {
	params := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	return g.performTokenRequest(ctx, params)
}

// RefreshToken refreshes an expired access token using a refresh token
func (g *GiteaProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	params := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	return g.performTokenRequest(ctx, params)
}

// GetCurrentUser returns the authenticated user's info from Gitea
func (g *GiteaProvider) GetCurrentUser(ctx context.Context) (*User, error) {
	var giteaUserResp giteaUser
	if err := g.doJSON(ctx, "GET", g.apiURL("/user"), http.NoBody, http.StatusOK, &giteaUserResp); err != nil {
		return nil, err
	}

	user := giteaUserResp.toUser()
	return &user, nil
}
