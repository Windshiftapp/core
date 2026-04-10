package scm

import (
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
		httpClient:          &http.Client{Timeout: 30 * time.Second},
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
	body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
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

// ListBranches lists branches for a repository
func (g *GiteaProvider) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	reqURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/branches", url.PathEscape(owner), url.PathEscape(repo)))

	var giteaBranches []giteaBranch
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &giteaBranches); err != nil {
		return nil, err
	}

	branches := make([]Branch, len(giteaBranches))
	for i, b := range giteaBranches {
		branches[i] = b.toBranch()
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

	reqURL := fmt.Sprintf("%s?state=%s&page=%d&limit=%d",
		g.apiURL(fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo))),
		state, page, limit)

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

	body := map[string]interface{}{
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

// CreateRelease creates a new release in a repository
func (g *GiteaProvider) CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error) {
	createURL := g.apiURL(fmt.Sprintf("/repos/%s/%s/releases", url.PathEscape(owner), url.PathEscape(repo)))

	body := map[string]interface{}{
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

	body := map[string]interface{}{
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
	Ref string `json:"ref"`
	SHA string `json:"sha"`
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
	return Release{
		ID:           fmt.Sprintf("%d", r.ID),
		TagName:      r.TagName,
		Name:         r.Name,
		Body:         r.Body,
		URL:          r.HTMLURL,
		IsDraft:      r.Draft,
		IsPrerelease: r.Prerelease,
		CreatedAt:    r.CreatedAt,
		PublishedAt:  r.PublishedAt,
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
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
