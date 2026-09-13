package scm

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"windshift/internal/models"
)

// Compile-time check that GitHubProvider implements the optional
// interfaces it claims. Dropping a method on any of these here surfaces
// as a build error rather than a runtime type-assertion miss.
var (
	_ Provider                     = (*GitHubProvider)(nil)
	_ ReleaseProvider              = (*GitHubProvider)(nil)
	_ CommitProvider               = (*GitHubProvider)(nil)
	_ RefProvider                  = (*GitHubProvider)(nil)
	_ IssueProvider                = (*GitHubProvider)(nil)
	_ PaginatedIssueProvider       = (*GitHubProvider)(nil)
	_ PullRequestReviewProvider    = (*GitHubProvider)(nil)
	_ RepositoryPermissionProvider = (*GitHubProvider)(nil)
)

func githubRepoPath(owner, repo string) string {
	return url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	baseProvider
	baseURL      string
	clientID     string
	clientSecret string
	accessToken  string
	// GitHub App specific fields
	appID                   string
	appPrivateKey           *rsa.PrivateKey
	installationID          int64
	installationToken       string
	installationTokenExpiry *time.Time
}

// NewGitHubProvider creates a new GitHub provider instance
func NewGitHubProvider(cfg ProviderConfig) (*GitHubProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = GitHubAPIURL
	}

	provider := &GitHubProvider{
		baseURL:      baseURL,
		clientID:     cfg.OAuthClientID,
		clientSecret: cfg.OAuthClientSecret,
		appID:        cfg.GitHubAppID,
	}
	provider.baseProvider = baseProvider{
		httpClient:          newSCMHTTPClient(30 * time.Second),
		setAuthHeader:       provider.setAuthHeader,
		handleErrorResponse: provider.handleErrorResponse,
	}

	// Get the access token based on auth method
	switch cfg.AuthMethod {
	case models.SCMAuthMethodOAuth:
		provider.accessToken = cfg.OAuthAccessToken
	case models.SCMAuthMethodPAT:
		provider.accessToken = cfg.PersonalAccessToken
	case models.SCMAuthMethodGitHubApp:
		// Parse the private key for GitHub App
		if cfg.GitHubAppPrivateKey != "" {
			privateKey, err := parseRSAPrivateKey(cfg.GitHubAppPrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to parse GitHub App private key: %w", err)
			}
			provider.appPrivateKey = privateKey
		}
		if cfg.GitHubAppInstallationID != "" {
			id, err := strconv.ParseInt(cfg.GitHubAppInstallationID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid installation ID: %w", err)
			}
			provider.installationID = id
		}
	}

	return provider, nil
}

// parseRSAPrivateKey parses a PEM-encoded RSA private key
func parseRSAPrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	// Try PKCS1 first
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return key, nil
	}

	// Try PKCS8
	keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := keyInterface.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return rsaKey, nil
}

// generateAppJWT generates a JWT for GitHub App authentication
func (g *GitHubProvider) generateAppJWT() (string, error) {
	if g.appPrivateKey == nil {
		return "", fmt.Errorf("GitHub App private key not configured")
	}
	if g.appID == "" {
		return "", fmt.Errorf("GitHub App ID not configured")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), // Issued 60 seconds in the past
		"exp": now.Add(10 * time.Minute).Unix(),  // Expires in 10 minutes (max allowed)
		"iss": g.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(g.appPrivateKey)
}

// ListAppInstallations lists all installations for the GitHub App
func (g *GitHubProvider) ListAppInstallations(ctx context.Context) ([]GitHubAppInstallation, error) {
	jwtToken, err := g.generateAppJWT()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", g.baseURL+"/app/installations", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("%w: status %d; failed to read response body: %v", ErrProviderError, resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, string(body))
	}

	var ghInstallations []struct {
		ID      int64 `json:"id"`
		Account struct {
			Login     string `json:"login"`
			Type      string `json:"type"`
			ID        int64  `json:"id"`
			AvatarURL string `json:"avatar_url"`
		} `json:"account"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghInstallations); err != nil {
		return nil, err
	}

	installations := make([]GitHubAppInstallation, len(ghInstallations))
	for i, inst := range ghInstallations {
		installations[i] = GitHubAppInstallation{
			ID:               inst.ID,
			AccountLogin:     inst.Account.Login,
			AccountType:      inst.Account.Type,
			AccountID:        inst.Account.ID,
			AccountAvatarURL: inst.Account.AvatarURL,
		}
	}

	return installations, nil
}

// GetInstallationAccessToken gets an access token for a specific installation
func (g *GitHubProvider) GetInstallationAccessToken(ctx context.Context, installationID int64) (string, *time.Time, error) {
	jwtToken, err := g.generateAppJWT()
	if err != nil {
		return "", nil, err
	}

	installationURL := fmt.Sprintf("%s/app/installations/%d/access_tokens", g.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", installationURL, http.NoBody)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", nil, fmt.Errorf("%w: status %d; failed to read response body: %v", ErrProviderError, resp.StatusCode, readErr)
		}
		return "", nil, fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", nil, err
	}

	return tokenResp.Token, &tokenResp.ExpiresAt, nil
}

// ensureInstallationToken ensures we have a valid installation access token
func (g *GitHubProvider) ensureInstallationToken(ctx context.Context) error {
	if g.appPrivateKey == nil || g.installationID == 0 {
		return nil // Not using GitHub App auth
	}

	// Check if current token is still valid (with 5 minute buffer)
	if g.installationToken != "" && g.installationTokenExpiry != nil {
		if time.Until(*g.installationTokenExpiry) > 5*time.Minute {
			return nil
		}
	}

	// Get new installation access token
	token, expiresAt, err := g.GetInstallationAccessToken(ctx, g.installationID)
	if err != nil {
		return err
	}

	g.installationToken = token
	g.installationTokenExpiry = expiresAt
	g.accessToken = token // Use installation token as access token

	return nil
}

// GetType returns the provider type
func (g *GitHubProvider) GetType() models.SCMProviderType {
	return models.SCMProviderTypeGitHub
}

// TestConnection tests if the provider connection is working
func (g *GitHubProvider) TestConnection(ctx context.Context) error {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	// GitHub App installation tokens can't access /user, use /installation/repositories instead
	var testURL string
	if g.appPrivateKey != nil && g.installationID != 0 {
		testURL = g.baseURL + "/installation/repositories?per_page=1"
	} else {
		testURL = g.baseURL + "/user"
	}

	return g.doJSON(ctx, "GET", testURL, http.NoBody, http.StatusOK, nil)
}

// ListRepositories lists all accessible repositories
func (g *GitHubProvider) ListRepositories(ctx context.Context, opts ListRepositoriesOptions) ([]Repository, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 30
	}

	// GitHub App installation tokens use /installation/repositories
	isApp := g.appPrivateKey != nil && g.installationID != 0
	var reqURL string
	if isApp {
		reqURL = fmt.Sprintf("%s/installation/repositories?page=%d&per_page=%d", g.baseURL, page, perPage)
	} else {
		reqURL = fmt.Sprintf("%s/user/repos?page=%d&per_page=%d", g.baseURL, page, perPage)
		if opts.Visibility != "" {
			reqURL += "&visibility=" + opts.Visibility
		}
		if opts.Sort != "" {
			reqURL += "&sort=" + opts.Sort
		}
	}

	// Use json.RawMessage so we can decode conditionally based on auth method
	var raw json.RawMessage
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}

	// GitHub App returns a different response structure
	if isApp {
		var installationRepos struct {
			TotalCount   int          `json:"total_count"`
			Repositories []githubRepo `json:"repositories"`
		}
		if err := json.Unmarshal(raw, &installationRepos); err != nil {
			return nil, err
		}
		repos := make([]Repository, len(installationRepos.Repositories))
		for i, r := range installationRepos.Repositories {
			repos[i] = r.toRepository()
		}
		return repos, nil
	}

	var ghRepos []githubRepo
	if err := json.Unmarshal(raw, &ghRepos); err != nil {
		return nil, err
	}

	repos := make([]Repository, len(ghRepos))
	for i, r := range ghRepos {
		repos[i] = r.toRepository()
	}
	return repos, nil
}

// GetRepository gets details about a specific repository
func (g *GitHubProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s", g.baseURL, githubRepoPath(owner, repo))

	var ghRepo githubRepo
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghRepo); err != nil {
		return nil, err
	}

	result := ghRepo.toRepository()
	return &result, nil
}

// ListPullRequests lists pull requests for a repository
func (g *GitHubProvider) ListPullRequests(ctx context.Context, owner, repo string, opts ListPROptions) ([]PullRequest, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 30
	}

	state := opts.State
	if state == "" {
		state = "open"
	}

	reqURL := fmt.Sprintf("%s/repos/%s/pulls?state=%s&page=%d&per_page=%d",
		g.baseURL, githubRepoPath(owner, repo), state, page, perPage)
	if opts.Sort != "" {
		reqURL += "&sort=" + opts.Sort
	}
	if opts.Direction != "" {
		reqURL += "&direction=" + opts.Direction
	}

	var ghPRs []githubPullRequest
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghPRs); err != nil {
		return nil, err
	}

	prs := make([]PullRequest, len(ghPRs))
	for i, pr := range ghPRs {
		prs[i] = pr.toPullRequest()
	}
	return prs, nil
}

// GetPullRequest gets details about a specific pull request
func (g *GitHubProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/pulls/%d", g.baseURL, githubRepoPath(owner, repo), number)

	var ghPR githubPullRequest
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghPR); err != nil {
		return nil, err
	}

	pr := ghPR.toPullRequest()
	return &pr, nil
}

// ListPullRequestCommits lists commits in a PR. Paginated up to maxPRCommits.
func (g *GitHubProvider) ListPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]Commit, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	const perPage = 100
	const maxPRCommits = 250

	var all []Commit
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/pulls/%d/commits?page=%d&per_page=%d",
			g.baseURL, githubRepoPath(owner, repo), number, page, perPage)
		var ghCommits []githubCommit
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghCommits); err != nil {
			return nil, err
		}
		for _, c := range ghCommits {
			all = append(all, c.toCommit())
			if len(all) >= maxPRCommits {
				return all, nil
			}
		}
		if len(ghCommits) < perPage {
			break
		}
	}
	return all, nil
}

// CreateBranch creates a new branch
func (g *GitHubProvider) CreateBranch(ctx context.Context, owner, repo, branchName, baseBranch string) error {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	// First, get the SHA of the base branch
	refURL := fmt.Sprintf("%s/repos/%s/git/refs/heads/%s", g.baseURL, githubRepoPath(owner, repo), baseBranch)
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := g.doJSON(ctx, "GET", refURL, http.NoBody, http.StatusOK, &ref); err != nil {
		return err
	}

	// Create the new branch
	createURL := fmt.Sprintf("%s/repos/%s/git/refs", g.baseURL, githubRepoPath(owner, repo))
	body := map[string]string{
		"ref": "refs/heads/" + branchName,
		"sha": ref.Object.SHA,
	}
	bodyJSON, _ := json.Marshal(body)

	return g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, nil)
}

// CreatePullRequest creates a new pull request
func (g *GitHubProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	createURL := fmt.Sprintf("%s/repos/%s/pulls", g.baseURL, githubRepoPath(owner, repo))

	body := map[string]any{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  opts.HeadBranch,
		"base":  opts.BaseBranch,
		"draft": opts.Draft,
	}
	bodyJSON, _ := json.Marshal(body)

	var ghPR githubPullRequest
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &ghPR); err != nil {
		return nil, err
	}

	pr := ghPR.toPullRequest()
	return &pr, nil
}

// CreateRelease creates a new release in a repository
func (g *GitHubProvider) CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	createURL := fmt.Sprintf("%s/repos/%s/releases", g.baseURL, githubRepoPath(owner, repo))

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
	bodyJSON, _ := json.Marshal(body)

	var ghRelease githubRelease
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &ghRelease); err != nil {
		return nil, err
	}

	release := ghRelease.toRelease()
	return &release, nil
}

// ListReleases lists releases for a repository
func (g *GitHubProvider) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/releases", g.baseURL, githubRepoPath(owner, repo))

	var ghReleases []githubRelease
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghReleases); err != nil {
		return nil, err
	}

	releases := make([]Release, 0, len(ghReleases))
	for _, r := range ghReleases {
		releases = append(releases, r.toRelease())
	}
	return releases, nil
}

// GetCommit gets details about a specific commit
func (g *GitHubProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/commits/%s", g.baseURL, githubRepoPath(owner, repo), sha)

	var ghCommit githubCommit
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghCommit); err != nil {
		return nil, err
	}

	commit := ghCommit.toCommit()
	return &commit, nil
}

// ListCommits lists commits from a repository branch/tag, newest first.
func (g *GitHubProvider) ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) ([]Commit, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 100
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	if opts.Sha != "" {
		q.Set("sha", opts.Sha)
	}
	if opts.Since != nil && !opts.Since.IsZero() {
		q.Set("since", opts.Since.Format(time.RFC3339))
	}

	reqURL := fmt.Sprintf("%s/repos/%s/commits?%s", g.baseURL, githubRepoPath(owner, repo), q.Encode())
	var ghCommits []githubCommit
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghCommits); err != nil {
		return nil, err
	}

	commits := make([]Commit, len(ghCommits))
	for i, c := range ghCommits {
		commits[i] = c.toCommit()
	}
	return commits, nil
}

// ListBranches lists branches for a repository, paginated up to maxBranches.
func (g *GitHubProvider) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	const perPage = 100
	const maxBranches = 1000

	type ghBranch struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}

	var branches []Branch
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/branches?page=%d&per_page=%d",
			g.baseURL, githubRepoPath(owner, repo), page, perPage)

		var ghBranches []ghBranch
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghBranches); err != nil {
			return nil, err
		}
		for _, b := range ghBranches {
			branches = append(branches, Branch{
				Name:      b.Name,
				SHA:       b.Commit.SHA,
				Protected: b.Protected,
			})
			if len(branches) >= maxBranches {
				return branches, nil
			}
		}
		if len(ghBranches) < perPage {
			break
		}
	}
	return branches, nil
}

// ListTags lists tags for a repository, paginated. Returns tags whose
// target commit's committer date is at or after `since`. GitHub's
// /tags endpoint returns lightweight summaries; we fetch each tag's
// commit to obtain the date, capped at maxTags to keep the work bounded.
func (g *GitHubProvider) ListTags(ctx context.Context, owner, repo string, since time.Time) ([]Tag, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	const perPage = 100
	const maxTags = 500

	type ghTag struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}

	var tags []Tag
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/tags?page=%d&per_page=%d",
			g.baseURL, githubRepoPath(owner, repo), page, perPage)

		var ghTags []ghTag
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghTags); err != nil {
			return nil, err
		}
		for _, t := range ghTags {
			created, err := g.tagCommitDate(ctx, owner, repo, t.Commit.SHA)
			if err != nil {
				return nil, err
			}
			if !since.IsZero() && created.Before(since) {
				continue
			}
			tags = append(tags, Tag{Name: t.Name, SHA: t.Commit.SHA, CreatedAt: created})
			if len(tags) >= maxTags {
				return tags, nil
			}
		}
		if len(ghTags) < perPage {
			break
		}
	}
	return tags, nil
}

func (g *GitHubProvider) tagCommitDate(ctx context.Context, owner, repo, sha string) (time.Time, error) {
	c, err := g.GetCommit(ctx, owner, repo, sha)
	if err != nil {
		return time.Time{}, err
	}
	return c.CreatedAt, nil
}

// CompareCommits returns the commits reachable from `head` but not from
// `base`. Paginates GitHub's /compare endpoint via per_page; capped at
// maxCompareCommits to bound work.
func (g *GitHubProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]Commit, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	const perPage = 100
	const maxCompareCommits = 500

	type ghCompare struct {
		Commits []githubCommit `json:"commits"`
	}

	var out []Commit
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/compare/%s...%s?page=%d&per_page=%d",
			g.baseURL, githubRepoPath(owner, repo), base, head, page, perPage)

		var resp ghCompare
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &resp); err != nil {
			return nil, err
		}
		for _, c := range resp.Commits {
			out = append(out, c.toCommit())
			if len(out) >= maxCompareCommits {
				return out, nil
			}
		}
		if len(resp.Commits) < perPage {
			break
		}
	}
	return out, nil
}

// RegisterWebhook registers a webhook for repository events
func (g *GitHubProvider) RegisterWebhook(ctx context.Context, owner, repo string, opts WebhookOptions) (*WebhookRegistration, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	createURL := fmt.Sprintf("%s/repos/%s/hooks", g.baseURL, githubRepoPath(owner, repo))

	contentType := opts.ContentType
	if contentType == "" {
		contentType = "json"
	}

	body := map[string]any{
		"name":   "web",
		"active": true,
		"events": opts.Events,
		"config": map[string]string{
			"url":          opts.URL,
			"content_type": contentType,
			"secret":       opts.Secret,
		},
	}
	bodyJSON, _ := json.Marshal(body)

	var hook struct {
		ID        int       `json:"id"`
		URL       string    `json:"url"`
		Events    []string  `json:"events"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := g.doJSON(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &hook); err != nil {
		return nil, err
	}

	return &WebhookRegistration{
		ID:        fmt.Sprintf("%d", hook.ID),
		URL:       hook.URL,
		Events:    hook.Events,
		IsActive:  hook.Active,
		CreatedAt: hook.CreatedAt,
	}, nil
}

// DeleteWebhook removes a registered webhook
func (g *GitHubProvider) DeleteWebhook(ctx context.Context, owner, repo, webhookID string) error {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	deleteURL := fmt.Sprintf("%s/repos/%s/hooks/%s", g.baseURL, githubRepoPath(owner, repo), webhookID)
	return g.doJSON(ctx, "DELETE", deleteURL, http.NoBody, http.StatusNoContent, nil)
}

// ListIssues lists issues for a repository, excluding pull requests
func (g *GitHubProvider) ListIssues(ctx context.Context, owner, repo string, opts ListIssueOptions) ([]Issue, error) {
	issues, _, err := g.ListIssuesPage(ctx, owner, repo, opts)
	return issues, err
}

// ListIssuesPage lists one raw GitHub issue page, filters pull requests from
// the returned entities, and separately reports whether the raw page was full.
// GitHub's issues endpoint includes pull requests, so len(filteredIssues) is
// not a valid pagination signal.
func (g *GitHubProvider) ListIssuesPage(ctx context.Context, owner, repo string, opts ListIssueOptions) ([]Issue, bool, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, false, err
	}

	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 100
	}
	state := opts.State
	if state == "" {
		state = "all"
	}

	reqURL := fmt.Sprintf("%s/repos/%s/issues?state=%s&page=%d&per_page=%d&direction=asc",
		g.baseURL, githubRepoPath(owner, repo), state, page, perPage)

	if opts.Since != nil {
		reqURL += "&since=" + url.QueryEscape(opts.Since.Format(time.RFC3339))
	}
	if len(opts.Labels) > 0 {
		reqURL += "&labels=" + url.QueryEscape(strings.Join(opts.Labels, ","))
	}

	var ghIssues []githubIssue
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghIssues); err != nil {
		return nil, false, err
	}

	issues, hasNext := filterGitHubIssuePage(ghIssues, perPage)
	return issues, hasNext, nil
}

func filterGitHubIssuePage(ghIssues []githubIssue, perPage int) ([]Issue, bool) {
	// Filter out pull requests (GitHub Issues API returns PRs too)
	issues := make([]Issue, 0, len(ghIssues))
	for _, gi := range ghIssues {
		if gi.PullRequest != nil {
			continue
		}
		issues = append(issues, gi.toIssue())
	}
	return issues, len(ghIssues) == perPage
}

// GetIssue gets details about a specific issue
func (g *GitHubProvider) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/issues/%d", g.baseURL, githubRepoPath(owner, repo), number)

	var gi githubIssue
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &gi); err != nil {
		return nil, err
	}

	issue := gi.toIssue()
	return &issue, nil
}

// UpdateIssue updates an issue's state, title, body, labels, assignees, or milestone
func (g *GitHubProvider) UpdateIssue(ctx context.Context, owner, repo string, number int, opts UpdateIssueOptions) (*Issue, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/issues/%d", g.baseURL, githubRepoPath(owner, repo), number)

	body := make(map[string]any)
	if opts.State != nil {
		body["state"] = *opts.State
	}
	if opts.Title != nil {
		body["title"] = *opts.Title
	}
	if opts.Body != nil {
		body["body"] = *opts.Body
	}
	if opts.Labels != nil {
		body["labels"] = opts.Labels
	}
	if opts.Assignees != nil {
		body["assignees"] = opts.Assignees
	}
	if opts.Milestone != nil {
		if *opts.Milestone == 0 {
			body["milestone"] = nil
		} else {
			body["milestone"] = *opts.Milestone
		}
	}

	bodyJSON, _ := json.Marshal(body)

	var gi githubIssue
	if err := g.doJSON(ctx, "PATCH", reqURL, strings.NewReader(string(bodyJSON)), http.StatusOK, &gi); err != nil {
		return nil, err
	}

	issue := gi.toIssue()
	return &issue, nil
}

// CreateIssueComment creates a comment on an issue and returns the GitHub comment ID
func (g *GitHubProvider) CreateIssueComment(ctx context.Context, owner, repo string, number int, commentBody string) (int64, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return 0, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/issues/%d/comments", g.baseURL, githubRepoPath(owner, repo), number)

	body := map[string]string{"body": commentBody}
	bodyJSON, _ := json.Marshal(body)

	var created struct {
		ID int64 `json:"id"`
	}
	if err := g.doJSON(ctx, "POST", reqURL, strings.NewReader(string(bodyJSON)), http.StatusCreated, &created); err != nil {
		return 0, err
	}

	return created.ID, nil
}

// ListIssueComments lists all comments on an issue
func (g *GitHubProvider) ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	var allComments []IssueComment
	page := 1
	for {
		reqURL := fmt.Sprintf("%s/repos/%s/issues/%d/comments?per_page=100&page=%d", g.baseURL, githubRepoPath(owner, repo), number, page)

		var ghComments []struct {
			ID                int64      `json:"id"`
			Body              string     `json:"body"`
			User              githubUser `json:"user"`
			AuthorAssociation string     `json:"author_association"`
			CreatedAt         time.Time  `json:"created_at"`
			UpdatedAt         time.Time  `json:"updated_at"`
		}
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghComments); err != nil {
			return nil, err
		}

		for _, c := range ghComments {
			allComments = append(allComments, IssueComment{
				ID: c.ID, Kind: "issue_comment", Body: c.Body,
				User: c.User.toUser(), AuthorAssociation: c.AuthorAssociation,
				CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
			})
		}

		if len(ghComments) < 100 {
			break
		}
		page++
	}
	return allComments, nil
}

// ListPullRequestReviewEvents returns submitted review bodies and inline
// review comments, which GitHub exposes separately from issue comments.
func (g *GitHubProvider) ListPullRequestReviewEvents(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}
	var events []IssueComment
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews?per_page=100&page=%d", g.baseURL, githubRepoPath(owner, repo), number, page)
		var reviews []struct {
			ID                int64      `json:"id"`
			Body              string     `json:"body"`
			User              githubUser `json:"user"`
			AuthorAssociation string     `json:"author_association"`
			SubmittedAt       time.Time  `json:"submitted_at"`
		}
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &reviews); err != nil {
			return nil, err
		}
		for _, review := range reviews {
			if strings.TrimSpace(review.Body) == "" {
				continue
			}
			events = append(events, IssueComment{
				ID: review.ID, Kind: "review", Body: review.Body, User: review.User.toUser(),
				AuthorAssociation: review.AuthorAssociation, CreatedAt: review.SubmittedAt, UpdatedAt: review.SubmittedAt,
			})
		}
		if len(reviews) < 100 {
			break
		}
	}
	for page := 1; ; page++ {
		reqURL := fmt.Sprintf("%s/repos/%s/pulls/%d/comments?per_page=100&page=%d", g.baseURL, githubRepoPath(owner, repo), number, page)
		var comments []struct {
			ID                int64      `json:"id"`
			Body              string     `json:"body"`
			User              githubUser `json:"user"`
			AuthorAssociation string     `json:"author_association"`
			Path              string     `json:"path"`
			Line              int        `json:"line"`
			OriginalLine      int        `json:"original_line"`
			Side              string     `json:"side"`
			InReplyToID       int64      `json:"in_reply_to_id"`
			CreatedAt         time.Time  `json:"created_at"`
			UpdatedAt         time.Time  `json:"updated_at"`
		}
		if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &comments); err != nil {
			return nil, err
		}
		for _, c := range comments {
			line := c.Line
			if line == 0 {
				line = c.OriginalLine
			}
			events = append(events, IssueComment{
				ID: c.ID, Kind: "review_comment", Body: c.Body, User: c.User.toUser(),
				AuthorAssociation: c.AuthorAssociation, Path: c.Path, Line: line, Side: c.Side,
				ThreadID: strconv.FormatInt(c.InReplyToID, 10), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
			})
		}
		if len(comments) < 100 {
			break
		}
	}
	return events, nil
}

// CanUserWriteRepository checks the commenter's effective collaborator
// permission. Triage is accepted because it is the minimum review role.
func (g *GitHubProvider) CanUserWriteRepository(ctx context.Context, owner, repo, username string) (bool, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return false, err
	}
	reqURL := fmt.Sprintf("%s/repos/%s/collaborators/%s/permission", g.baseURL, githubRepoPath(owner, repo), url.PathEscape(username))
	var out struct {
		Permission string `json:"permission"`
		User       struct {
			Permissions map[string]bool `json:"permissions"`
		} `json:"user"`
	}
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &out); err != nil {
		return false, err
	}
	switch strings.ToLower(out.Permission) {
	case "admin", "maintain", "write", "push", "triage":
		return true, nil
	}
	return out.User.Permissions["admin"] || out.User.Permissions["maintain"] || out.User.Permissions["push"] || out.User.Permissions["triage"], nil
}

// UpdateIssueComment updates an existing comment on an issue
func (g *GitHubProvider) UpdateIssueComment(ctx context.Context, owner, repo string, _ int, commentID int64, commentBody string) error {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", g.baseURL, githubRepoPath(owner, repo), commentID)

	body := map[string]string{"body": commentBody}
	bodyJSON, _ := json.Marshal(body)

	return g.doJSON(ctx, "PATCH", reqURL, strings.NewReader(string(bodyJSON)), http.StatusOK, nil)
}

// ListRepoLabels lists all labels for a repository
func (g *GitHubProvider) ListRepoLabels(ctx context.Context, owner, repo string) ([]IssueLabel, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/labels?per_page=100", g.baseURL, githubRepoPath(owner, repo))

	var ghLabels []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghLabels); err != nil {
		return nil, err
	}

	labels := make([]IssueLabel, len(ghLabels))
	for i, l := range ghLabels {
		labels[i] = IssueLabel{ID: l.ID, Name: l.Name, Color: l.Color}
	}
	return labels, nil
}

// ListRepoMilestones lists all milestones for a repository
func (g *GitHubProvider) ListRepoMilestones(ctx context.Context, owner, repo string) ([]IssueMilestone, error) {
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/repos/%s/milestones?state=all&per_page=100", g.baseURL, githubRepoPath(owner, repo))

	var ghMilestones []struct {
		ID     int64  `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
	}
	if err := g.doJSON(ctx, "GET", reqURL, http.NoBody, http.StatusOK, &ghMilestones); err != nil {
		return nil, err
	}

	milestones := make([]IssueMilestone, len(ghMilestones))
	for i, m := range ghMilestones {
		milestones[i] = IssueMilestone{ID: m.ID, Number: m.Number, Title: m.Title, State: m.State}
	}
	return milestones, nil
}

// GetOAuthURL returns the URL to start the OAuth flow
func (g *GitHubProvider) GetOAuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":    {g.clientID},
		"redirect_uri": {redirectURI},
		"scope":        {"repo read:user user:email"},
		"state":        {state},
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

// ExchangeCode exchanges an OAuth code for tokens
func (g *GitHubProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error) {
	tokenURL := "https://github.com/login/oauth/access_token" //nolint:gosec // G101 false positive: OAuth endpoint URL, not a credential

	params := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("%w: failed to read response body: %v", ErrProviderError, readErr)
		}
		return nil, fmt.Errorf("%w: %s", ErrProviderError, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error,omitempty"`
		ErrorDesc   string `json:"error_description,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%w: %s - %s", ErrProviderError, tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &OAuthTokens{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		Scope:       tokenResp.Scope,
		// GitHub access tokens don't expire by default
	}, nil
}

// RefreshToken refreshes an expired access token
// Note: GitHub OAuth tokens don't expire and can't be refreshed
func (g *GitHubProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	return nil, fmt.Errorf("GitHub OAuth tokens do not support refresh")
}

// RevokeToken asks GitHub to invalidate an OAuth access token issued to
// this OAuth App. Implements scm.TokenRevoker.
//
// Endpoint: DELETE /applications/{client_id}/token
// Auth:     HTTP Basic with the OAuth app's client_id:client_secret
// Body:     {"access_token": "<token>"}
//
// A 404 here is treated as already-revoked (token not recognized) and
// returned as nil so callers can disconnect cleanly. Any other failure
// surfaces to the caller for best-effort logging.
func (g *GitHubProvider) RevokeToken(ctx context.Context, accessToken string) error {
	if g.clientID == "" || g.clientSecret == "" {
		return fmt.Errorf("github revoke: missing OAuth client credentials")
	}
	if accessToken == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"access_token": accessToken})
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/applications/%s/token", g.baseURL, g.clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(g.clientID, g.clientSecret)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// 204: revoked. 404: already gone or never existed — treat as success.
		return nil
	default:
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("%w: github revoke status %d; failed to read response body: %v", ErrProviderError, resp.StatusCode, readErr)
		}
		return fmt.Errorf("%w: github revoke status %d - %s", ErrProviderError, resp.StatusCode, string(respBody))
	}
}

// GetCurrentUser returns the authenticated user's info from GitHub
func (g *GitHubProvider) GetCurrentUser(ctx context.Context) (*User, error) {
	var ghUser struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := g.doJSON(ctx, "GET", g.baseURL+"/user", http.NoBody, http.StatusOK, &ghUser); err != nil {
		return nil, err
	}

	return &User{
		ID:        fmt.Sprintf("%d", ghUser.ID),
		Username:  ghUser.Login,
		Name:      ghUser.Name,
		Email:     ghUser.Email,
		AvatarURL: ghUser.AvatarURL,
	}, nil
}

// Helper methods

func (g *GitHubProvider) setAuthHeader(req *http.Request) {
	if g.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.accessToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}

func (g *GitHubProvider) handleErrorResponse(resp *http.Response) error {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("%w: failed to read response body: %v", ErrProviderError, readErr)
	}
	bodyStr := string(body)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		if strings.Contains(bodyStr, "rate limit") || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			resetAt := resp.Header.Get("X-RateLimit-Reset")
			slog.Warn("GitHub rate limit hit", "reset_at", resetAt)
			return ErrRateLimited
		}
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrForbidden, bodyStr)
		}
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnprocessableEntity:
		// Check for duplicate PR error
		if strings.Contains(bodyStr, "already exists") || strings.Contains(bodyStr, "A pull request already exists") {
			return ErrAlreadyExists
		}
		return fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, bodyStr)
	default:
		return fmt.Errorf("%w: status %d - %s", ErrProviderError, resp.StatusCode, bodyStr)
	}
}

// GitHub API response types

type githubRepo struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r githubRepo) toRepository() Repository {
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
		Owner:         r.Owner.Login,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

type githubPullRequest struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Merged  bool   `json:"merged"`
	Head    struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	User      githubUser `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

func (pr githubPullRequest) toPullRequest() PullRequest {
	return PullRequest{
		ID:         pr.ID,
		Number:     pr.Number,
		Title:      pr.Title,
		Body:       pr.Body,
		State:      pr.State,
		URL:        pr.HTMLURL,
		HeadBranch: pr.Head.Ref,
		HeadRepo:   pr.Head.Repo.FullName,
		HeadSHA:    pr.Head.SHA,
		BaseBranch: pr.Base.Ref,
		IsMerged:   pr.Merged,
		IsDraft:    pr.Draft,
		Author:     pr.User.toUser(),
		CreatedAt:  pr.CreatedAt,
		UpdatedAt:  pr.UpdatedAt,
		MergedAt:   pr.MergedAt,
		ClosedAt:   pr.ClosedAt,
	}
}

type githubRelease struct {
	ID          int        `json:"id"`
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	HTMLURL     string     `json:"html_url"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
}

func (r githubRelease) toRelease() Release {
	status := "released"
	if r.Draft {
		status = "draft"
	} else if r.Prerelease {
		status = "prerelease"
	}
	return Release{
		ID:           strconv.Itoa(r.ID),
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

type githubCommit struct {
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
	Author    *githubUser `json:"author"`
	Committer *githubUser `json:"committer"`
}

func (c githubCommit) toCommit() Commit {
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

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

func (u githubUser) toUser() User {
	return User{
		ID:        fmt.Sprintf("%d", u.ID),
		Username:  u.Login,
		AvatarURL: u.AvatarURL,
	}
}

type githubIssue struct {
	ID      int64      `json:"id"`
	Number  int        `json:"number"`
	Title   string     `json:"title"`
	Body    string     `json:"body"`
	State   string     `json:"state"`
	HTMLURL string     `json:"html_url"`
	User    githubUser `json:"user"`
	Labels  []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	Assignees []githubUser `json:"assignees"`
	Milestone *struct {
		ID     int64  `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
	} `json:"milestone"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

func (gi githubIssue) toIssue() Issue {
	issue := Issue{
		ID:        gi.ID,
		Number:    gi.Number,
		Title:     gi.Title,
		Body:      gi.Body,
		State:     gi.State,
		URL:       gi.HTMLURL,
		Author:    gi.User.toUser(),
		CreatedAt: gi.CreatedAt,
		UpdatedAt: gi.UpdatedAt,
		ClosedAt:  gi.ClosedAt,
	}

	for _, l := range gi.Labels {
		issue.Labels = append(issue.Labels, IssueLabel{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	for _, a := range gi.Assignees {
		issue.Assignees = append(issue.Assignees, a.toUser())
	}
	if gi.Milestone != nil {
		issue.Milestone = &IssueMilestone{
			ID:     gi.Milestone.ID,
			Number: gi.Milestone.Number,
			Title:  gi.Milestone.Title,
			State:  gi.Milestone.State,
		}
	}

	return issue
}
