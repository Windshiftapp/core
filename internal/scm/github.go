package scm

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"windshift/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	baseURL      string
	clientID     string
	clientSecret string
	accessToken  string
	httpClient   *http.Client
	// GitHub App specific fields
	appID            string
	appPrivateKey    *rsa.PrivateKey
	installationID   int64
	installationToken string
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
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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

	req, err := http.NewRequestWithContext(ctx, "GET", g.baseURL+"/app/installations", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", g.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
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
	// Ensure we have a valid token for GitHub App auth
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	// Use different endpoints based on auth method
	// GitHub App installation tokens can't access /user, use /installation/repositories instead
	var testURL string
	if g.appPrivateKey != nil && g.installationID != 0 {
		testURL = g.baseURL + "/installation/repositories?per_page=1"
	} else {
		testURL = g.baseURL + "/user"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: unexpected status %d", ErrProviderError, resp.StatusCode)
	}

	return nil
}

// ListRepositories lists all accessible repositories
func (g *GitHubProvider) ListRepositories(ctx context.Context, opts ListRepositoriesOptions) ([]Repository, error) {
	// Ensure we have a valid token for GitHub App auth
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

	// Use different endpoints based on auth method
	// GitHub App installation tokens use /installation/repositories
	var url string
	if g.appPrivateKey != nil && g.installationID != 0 {
		url = fmt.Sprintf("%s/installation/repositories?page=%d&per_page=%d", g.baseURL, page, perPage)
	} else {
		url = fmt.Sprintf("%s/user/repos?page=%d&per_page=%d", g.baseURL, page, perPage)
		if opts.Visibility != "" {
			url += "&visibility=" + opts.Visibility
		}
		if opts.Sort != "" {
			url += "&sort=" + opts.Sort
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	// GitHub App returns a different response structure
	if g.appPrivateKey != nil && g.installationID != 0 {
		var installationRepos struct {
			TotalCount   int          `json:"total_count"`
			Repositories []githubRepo `json:"repositories"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&installationRepos); err != nil {
			return nil, err
		}
		repos := make([]Repository, len(installationRepos.Repositories))
		for i, r := range installationRepos.Repositories {
			repos[i] = r.toRepository()
		}
		return repos, nil
	}

	var ghRepos []githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&ghRepos); err != nil {
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
	url := fmt.Sprintf("%s/repos/%s/%s", g.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghRepo githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&ghRepo); err != nil {
		return nil, err
	}

	result := ghRepo.toRepository()
	return &result, nil
}

// ListPullRequests lists pull requests for a repository
func (g *GitHubProvider) ListPullRequests(ctx context.Context, owner, repo string, opts ListPROptions) ([]PullRequest, error) {
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

	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=%s&page=%d&per_page=%d",
		g.baseURL, owner, repo, state, page, perPage)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghPRs []githubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&ghPRs); err != nil {
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
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", g.baseURL, owner, repo, number)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghPR githubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err != nil {
		return nil, err
	}

	pr := ghPR.toPullRequest()
	return &pr, nil
}

// CreateBranch creates a new branch
func (g *GitHubProvider) CreateBranch(ctx context.Context, owner, repo, branchName, baseBranch string) error {
	// Ensure we have a valid installation token for GitHub App auth
	if err := g.ensureInstallationToken(ctx); err != nil {
		return err
	}

	// First, get the SHA of the base branch
	refURL := fmt.Sprintf("%s/repos/%s/%s/git/refs/heads/%s", g.baseURL, owner, repo, baseBranch)
	req, err := http.NewRequestWithContext(ctx, "GET", refURL, nil)
	if err != nil {
		return err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return g.handleErrorResponse(resp)
	}

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return err
	}

	// Create the new branch
	createURL := fmt.Sprintf("%s/repos/%s/%s/git/refs", g.baseURL, owner, repo)
	body := map[string]string{
		"ref": "refs/heads/" + branchName,
		"sha": ref.Object.SHA,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err = http.NewRequestWithContext(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return err
	}
	g.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err = g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return g.handleErrorResponse(resp)
	}

	return nil
}

// CreatePullRequest creates a new pull request
func (g *GitHubProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error) {
	// Ensure we have a valid installation token for GitHub App auth
	if err := g.ensureInstallationToken(ctx); err != nil {
		return nil, err
	}

	createURL := fmt.Sprintf("%s/repos/%s/%s/pulls", g.baseURL, owner, repo)

	body := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  opts.HeadBranch,
		"base":  opts.BaseBranch,
		"draft": opts.Draft,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, g.handleErrorResponse(resp)
	}

	var ghPR githubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err != nil {
		return nil, err
	}

	pr := ghPR.toPullRequest()
	return &pr, nil
}

// GetCommit gets details about a specific commit
func (g *GitHubProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", g.baseURL, owner, repo, sha)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghCommit githubCommit
	if err := json.NewDecoder(resp.Body).Decode(&ghCommit); err != nil {
		return nil, err
	}

	commit := ghCommit.toCommit()
	return &commit, nil
}

// ListBranches lists branches for a repository
func (g *GitHubProvider) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/branches", g.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghBranches []struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghBranches); err != nil {
		return nil, err
	}

	branches := make([]Branch, len(ghBranches))
	for i, b := range ghBranches {
		branches[i] = Branch{
			Name:      b.Name,
			SHA:       b.Commit.SHA,
			Protected: b.Protected,
		}
	}
	return branches, nil
}

// RegisterWebhook registers a webhook for repository events
func (g *GitHubProvider) RegisterWebhook(ctx context.Context, owner, repo string, opts WebhookOptions) (*WebhookRegistration, error) {
	createURL := fmt.Sprintf("%s/repos/%s/%s/hooks", g.baseURL, owner, repo)

	contentType := opts.ContentType
	if contentType == "" {
		contentType = "json"
	}

	body := map[string]interface{}{
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

	req, err := http.NewRequestWithContext(ctx, "POST", createURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, g.handleErrorResponse(resp)
	}

	var hook struct {
		ID        int       `json:"id"`
		URL       string    `json:"url"`
		Events    []string  `json:"events"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
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
	deleteURL := fmt.Sprintf("%s/repos/%s/%s/hooks/%s", g.baseURL, owner, repo, webhookID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusNoContent {
		return g.handleErrorResponse(resp)
	}

	return nil
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
	tokenURL := "https://github.com/login/oauth/access_token"

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: %s", ErrProviderError, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		Error        string `json:"error,omitempty"`
		ErrorDesc    string `json:"error_description,omitempty"`
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

// GetCurrentUser returns the authenticated user's info from GitHub
func (g *GitHubProvider) GetCurrentUser(ctx context.Context) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", g.baseURL+"/user", nil)
	if err != nil {
		return nil, err
	}
	g.setAuthHeader(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var ghUser struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
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
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		if strings.Contains(bodyStr, "rate limit") {
			return ErrRateLimited
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
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	HTMLURL       string    `json:"html_url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	Archived      bool      `json:"archived"`
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
	ID        int    `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	Draft     bool   `json:"draft"`
	Merged    bool   `json:"merged"`
	Head      struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
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
