package scm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
)

const gitLabDefaultURL = "https://gitlab.com"

var (
	_ Provider                     = (*GitLabProvider)(nil)
	_ OAuthProvider                = (*GitLabProvider)(nil)
	_ TokenRevoker                 = (*GitLabProvider)(nil)
	_ ReleaseProvider              = (*GitLabProvider)(nil)
	_ CommitProvider               = (*GitLabProvider)(nil)
	_ RefProvider                  = (*GitLabProvider)(nil)
	_ IssueCommentProvider         = (*GitLabProvider)(nil)
	_ PullRequestReviewProvider    = (*GitLabProvider)(nil)
	_ RepositoryPermissionProvider = (*GitLabProvider)(nil)
)

// GitLabProvider implements GitLab REST API v4 for GitLab.com and
// self-managed installations.
type GitLabProvider struct {
	baseProvider
	baseURL      string
	apiBaseURL   string
	authMethod   models.SCMAuthMethod
	accessToken  string
	clientID     string
	clientSecret string
}

func NewGitLabProvider(cfg ProviderConfig) (*GitLabProvider, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = gitLabDefaultURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v4")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid GitLab base URL: %w", err)
	}

	accessToken := cfg.PersonalAccessToken
	if cfg.AuthMethod == models.SCMAuthMethodOAuth {
		accessToken = cfg.OAuthAccessToken
	}
	p := &GitLabProvider{
		baseURL:      baseURL,
		apiBaseURL:   baseURL + "/api/v4",
		authMethod:   cfg.AuthMethod,
		accessToken:  accessToken,
		clientID:     cfg.OAuthClientID,
		clientSecret: cfg.OAuthClientSecret,
	}
	p.baseProvider = baseProvider{
		httpClient:          newSCMHTTPClient(30 * time.Second),
		setAuthHeader:       p.setAuthHeader,
		handleErrorResponse: p.handleErrorResponse,
	}
	return p, nil
}

func (g *GitLabProvider) GetType() models.SCMProviderType { return models.SCMProviderTypeGitLab }

func (g *GitLabProvider) apiURL(path string) string { return g.apiBaseURL + path }

func (g *GitLabProvider) projectPath(owner, repo string) string {
	return url.PathEscape(strings.Trim(owner, "/") + "/" + strings.Trim(repo, "/"))
}

func (g *GitLabProvider) projectWebURL(owner, repo string) string {
	return g.baseURL + "/" + strings.Trim(owner, "/") + "/" + strings.Trim(repo, "/")
}

func (g *GitLabProvider) projectAPIURL(owner, repo, suffix string) string {
	return g.apiURL("/projects/" + g.projectPath(owner, repo) + suffix)
}

func (g *GitLabProvider) setAuthHeader(req *http.Request) {
	if g.accessToken == "" {
		return
	}
	if g.authMethod == models.SCMAuthMethodPAT {
		req.Header.Set("PRIVATE-TOKEN", g.accessToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+g.accessToken)
	}
	req.Header.Set("Accept", "application/json")
}

func (g *GitLabProvider) handleErrorResponse(resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("%w: read GitLab response: %v", ErrProviderError, readErr)
	}
	message := strings.TrimSpace(string(body))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidCredentials
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrForbidden, message)
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusConflict:
		return ErrAlreadyExists
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		if strings.Contains(strings.ToLower(message), "already exists") || strings.Contains(strings.ToLower(message), "another open merge request") {
			return ErrAlreadyExists
		}
	}
	return fmt.Errorf("%w: GitLab status %d: %s", ErrProviderError, resp.StatusCode, message)
}

func (g *GitLabProvider) TestConnection(ctx context.Context) error {
	return g.doJSON(ctx, http.MethodGet, g.apiURL("/user"), http.NoBody, http.StatusOK, nil)
}

func (g *GitLabProvider) ListRepositories(ctx context.Context, opts ListRepositoriesOptions) ([]Repository, error) {
	page, perPage := opts.Page, opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	q := url.Values{
		"membership": {"true"}, "simple": {"true"}, "page": {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(perPage)}, "order_by": {"last_activity_at"}, "sort": {"desc"},
	}
	if opts.Visibility != "" && opts.Visibility != "all" {
		q.Set("visibility", opts.Visibility)
	}
	var raw []gitLabProject
	if err := g.doJSON(ctx, http.MethodGet, g.apiURL("/projects?"+q.Encode()), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	out := make([]Repository, 0, len(raw))
	for _, project := range raw {
		out = append(out, project.toRepository())
	}
	return out, nil
}

func (g *GitLabProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	var raw gitLabProject
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, ""), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	result := raw.toRepository()
	return &result, nil
}

func (g *GitLabProvider) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	const perPage, maxBranches = 100, 1000
	var out []Branch
	for page := 1; len(out) < maxBranches; page++ {
		var raw []gitLabBranch
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/repository/branches?page=%d&per_page=%d", page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, branch := range raw {
			out = append(out, branch.toBranch())
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) ListPullRequests(ctx context.Context, owner, repo string, opts ListPROptions) ([]PullRequest, error) {
	page, perPage := opts.Page, opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	state := opts.State
	if state == "" || state == "open" {
		state = "opened"
	}
	q := url.Values{"scope": {"all"}, "state": {state}, "page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
	if opts.Sort != "" {
		orderBy := opts.Sort
		if orderBy == "updated" || orderBy == "created" {
			orderBy += "_at"
		}
		q.Set("order_by", orderBy)
	}
	if opts.Direction != "" {
		q.Set("sort", opts.Direction)
	}
	var raw []gitLabMergeRequest
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, "/merge_requests?"+q.Encode()), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	out := make([]PullRequest, 0, len(raw))
	for _, mr := range raw {
		out = append(out, mr.toPullRequest(owner+"/"+repo))
	}
	return out, nil
}

func (g *GitLabProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	var raw gitLabMergeRequest
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, fmt.Sprintf("/merge_requests/%d", number)), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	result := raw.toPullRequest(owner + "/" + repo)
	return &result, nil
}

func (g *GitLabProvider) ListPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]Commit, error) {
	const perPage, maxCommits = 100, 500
	var out []Commit
	for page := 1; len(out) < maxCommits; page++ {
		var raw []gitLabCommit
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/merge_requests/%d/commits?page=%d&per_page=%d", number, page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, commit := range raw {
			out = append(out, commit.toCommit())
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) CreateBranch(ctx context.Context, owner, repo, branchName, baseBranch string) error {
	body, err := json.Marshal(map[string]string{"branch": branchName, "ref": baseBranch})
	if err != nil {
		return err
	}
	return g.doJSON(ctx, http.MethodPost, g.projectAPIURL(owner, repo, "/repository/branches"), bytes.NewReader(body), http.StatusCreated, nil)
}

func (g *GitLabProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error) {
	title := opts.Title
	if opts.Draft && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "draft:") {
		title = "Draft: " + title
	}
	body, err := json.Marshal(map[string]any{
		"title": title, "description": opts.Body, "source_branch": opts.HeadBranch,
		"target_branch": opts.BaseBranch,
	})
	if err != nil {
		return nil, err
	}
	var raw gitLabMergeRequest
	if err := g.doJSON(ctx, http.MethodPost, g.projectAPIURL(owner, repo, "/merge_requests"), bytes.NewReader(body), http.StatusCreated, &raw); err != nil {
		return nil, err
	}
	result := raw.toPullRequest(owner + "/" + repo)
	return &result, nil
}

func (g *GitLabProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error) {
	var raw gitLabCommit
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, "/repository/commits/"+url.PathEscape(sha)), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	result := raw.toCommit()
	return &result, nil
}

func (g *GitLabProvider) ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) ([]Commit, error) {
	page, perPage := opts.Page, opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
	if opts.Sha != "" {
		q.Set("ref_name", opts.Sha)
	}
	if opts.Since != nil && !opts.Since.IsZero() {
		q.Set("since", opts.Since.Format(time.RFC3339))
	}
	var raw []gitLabCommit
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, "/repository/commits?"+q.Encode()), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw))
	for _, commit := range raw {
		out = append(out, commit.toCommit())
	}
	return out, nil
}

func (g *GitLabProvider) ListTags(ctx context.Context, owner, repo string, since time.Time) ([]Tag, error) {
	const perPage, maxTags = 100, 500
	var out []Tag
	webURL := g.projectWebURL(owner, repo)
	for page := 1; len(out) < maxTags; page++ {
		var raw []gitLabTag
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/repository/tags?page=%d&per_page=%d", page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, tag := range raw {
			created := tag.Commit.CommittedDate
			if !since.IsZero() && created.Before(since) {
				continue
			}
			out = append(out, Tag{Name: tag.Name, SHA: tag.Target, URL: webURL + "/-/tags/" + url.PathEscape(tag.Name), CreatedAt: created})
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]Commit, error) {
	q := url.Values{"from": {base}, "to": {head}, "straight": {"true"}}
	var raw struct {
		Commits []gitLabCommit `json:"commits"`
	}
	if err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, "/repository/compare?"+q.Encode()), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw.Commits))
	for i, commit := range raw.Commits {
		if i >= 500 {
			break
		}
		out = append(out, commit.toCommit())
	}
	return out, nil
}

func (g *GitLabProvider) CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error) {
	bodyMap := map[string]any{"tag_name": opts.TagName, "name": opts.Name, "description": opts.Body}
	if opts.TargetCommitish != "" {
		bodyMap["ref"] = opts.TargetCommitish
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	var raw gitLabRelease
	if err := g.doJSON(ctx, http.MethodPost, g.projectAPIURL(owner, repo, "/releases"), bytes.NewReader(body), http.StatusCreated, &raw); err != nil {
		return nil, err
	}
	result := raw.toRelease(g.projectWebURL(owner, repo))
	return &result, nil
}

func (g *GitLabProvider) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	const perPage = 100
	var out []Release
	webURL := g.projectWebURL(owner, repo)
	for page := 1; ; page++ {
		var raw []gitLabRelease
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/releases?page=%d&per_page=%d", page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, release := range raw {
			out = append(out, release.toRelease(webURL))
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	const perPage = 100
	var out []IssueComment
	for page := 1; ; page++ {
		var raw []gitLabNote
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/merge_requests/%d/notes?sort=asc&page=%d&per_page=%d", number, page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, note := range raw {
			if note.System || note.Type != "" {
				continue
			}
			out = append(out, note.toIssueComment("issue_comment", ""))
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) ListPullRequestReviewEvents(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	const perPage = 100
	var out []IssueComment
	for page := 1; ; page++ {
		var raw []gitLabDiscussion
		reqURL := g.projectAPIURL(owner, repo, fmt.Sprintf("/merge_requests/%d/discussions?page=%d&per_page=%d", number, page, perPage))
		if err := g.doJSON(ctx, http.MethodGet, reqURL, http.NoBody, http.StatusOK, &raw); err != nil {
			return nil, err
		}
		for _, discussion := range raw {
			if discussion.IndividualNote {
				continue
			}
			for _, note := range discussion.Notes {
				if note.System {
					continue
				}
				kind := "review"
				if note.Position.NewPath != "" {
					kind = "review_comment"
				}
				out = append(out, note.toIssueComment(kind, discussion.ID))
			}
		}
		if len(raw) < perPage {
			break
		}
	}
	return out, nil
}

func (g *GitLabProvider) CreateIssueComment(ctx context.Context, owner, repo string, number int, commentBody string) (int64, error) {
	body, err := json.Marshal(map[string]string{"body": commentBody})
	if err != nil {
		return 0, err
	}
	var raw gitLabNote
	if err := g.doJSON(ctx, http.MethodPost, g.projectAPIURL(owner, repo, fmt.Sprintf("/merge_requests/%d/notes", number)), bytes.NewReader(body), http.StatusCreated, &raw); err != nil {
		return 0, err
	}
	return raw.ID, nil
}

func (g *GitLabProvider) UpdateIssueComment(ctx context.Context, owner, repo string, number int, commentID int64, commentBody string) error {
	body, err := json.Marshal(map[string]string{"body": commentBody})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/merge_requests/%d/notes/%d", number, commentID)
	return g.doJSON(ctx, http.MethodPut, g.projectAPIURL(owner, repo, path), bytes.NewReader(body), http.StatusOK, nil)
}

func (g *GitLabProvider) CanUserWriteRepository(ctx context.Context, owner, repo, username string) (bool, error) {
	q := url.Values{"username": {username}}
	var users []gitLabUser
	if err := g.doJSON(ctx, http.MethodGet, g.apiURL("/users?"+q.Encode()), http.NoBody, http.StatusOK, &users); err != nil {
		return false, err
	}
	var userID int64
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			userID = user.ID
			break
		}
	}
	if userID == 0 {
		return false, nil
	}
	var member struct {
		AccessLevel int `json:"access_level"`
	}
	err := g.doJSON(ctx, http.MethodGet, g.projectAPIURL(owner, repo, "/members/all/"+strconv.FormatInt(userID, 10)), http.NoBody, http.StatusOK, &member)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return member.AccessLevel >= 30, nil
}

func (g *GitLabProvider) RegisterWebhook(ctx context.Context, owner, repo string, opts WebhookOptions) (*WebhookRegistration, error) {
	events := map[string]bool{}
	if len(opts.Events) == 0 {
		events["push_events"] = true
		events["merge_requests_events"] = true
	}
	for _, event := range opts.Events {
		switch event {
		case "push":
			events["push_events"] = true
		case "tag_push":
			events["tag_push_events"] = true
		case "pull_request", "merge_request":
			events["merge_requests_events"] = true
		case "note":
			events["note_events"] = true
		case "release":
			events["releases_events"] = true
		}
	}
	bodyMap := map[string]any{"url": opts.URL, "token": opts.Secret, "enable_ssl_verification": true}
	for key, enabled := range events {
		bodyMap[key] = enabled
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	var raw gitLabHook
	if err := g.doJSON(ctx, http.MethodPost, g.projectAPIURL(owner, repo, "/hooks"), bytes.NewReader(body), http.StatusCreated, &raw); err != nil {
		return nil, err
	}
	return &WebhookRegistration{ID: strconv.FormatInt(raw.ID, 10), URL: raw.URL, Events: opts.Events, IsActive: true, CreatedAt: raw.CreatedAt}, nil
}

func (g *GitLabProvider) DeleteWebhook(ctx context.Context, owner, repo, webhookID string) error {
	return g.doJSON(ctx, http.MethodDelete, g.projectAPIURL(owner, repo, "/hooks/"+url.PathEscape(webhookID)), http.NoBody, http.StatusNoContent, nil)
}

func (g *GitLabProvider) GetOAuthURL(state, redirectURI string) string {
	q := url.Values{"client_id": {g.clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "state": {state}, "scope": {"api"}}
	return g.baseURL + "/oauth/authorize?" + q.Encode()
}

func (g *GitLabProvider) performTokenRequest(ctx context.Context, params url.Values) (*OAuthTokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/oauth/token", strings.NewReader(params.Encode()))
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusBadRequest && bytes.Contains(body, []byte("invalid_grant")) {
			return nil, fmt.Errorf("%w: %s", ErrRefreshTokenInvalid, body)
		}
		return nil, fmt.Errorf("%w: %s", ErrProviderError, body)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		CreatedAt    int64  `json:"created_at"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	result := &OAuthTokens{AccessToken: raw.AccessToken, TokenType: raw.TokenType, RefreshToken: raw.RefreshToken, Scope: raw.Scope}
	if raw.ExpiresIn > 0 {
		base := time.Now()
		if raw.CreatedAt > 0 {
			base = time.Unix(raw.CreatedAt, 0)
		}
		expires := base.Add(time.Duration(raw.ExpiresIn) * time.Second)
		result.ExpiresAt = &expires
	}
	return result, nil
}

func (g *GitLabProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error) {
	return g.performTokenRequest(ctx, url.Values{"client_id": {g.clientID}, "client_secret": {g.clientSecret}, "code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {redirectURI}})
}

func (g *GitLabProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	return g.performTokenRequest(ctx, url.Values{"client_id": {g.clientID}, "client_secret": {g.clientSecret}, "refresh_token": {refreshToken}, "grant_type": {"refresh_token"}})
}

func (g *GitLabProvider) RevokeToken(ctx context.Context, accessToken string) error {
	params := url.Values{"client_id": {g.clientID}, "client_secret": {g.clientSecret}, "token": {accessToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/oauth/revoke", strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return g.handleErrorResponse(resp)
	}
	return nil
}

func (g *GitLabProvider) GetCurrentUser(ctx context.Context) (*User, error) {
	var raw gitLabUser
	if err := g.doJSON(ctx, http.MethodGet, g.apiURL("/user"), http.NoBody, http.StatusOK, &raw); err != nil {
		return nil, err
	}
	result := raw.toUser()
	return &result, nil
}

type gitLabProject struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	PathWithNamespace string    `json:"path_with_namespace"`
	Description       string    `json:"description"`
	WebURL            string    `json:"web_url"`
	HTTPURL           string    `json:"http_url_to_repo"`
	SSHURL            string    `json:"ssh_url_to_repo"`
	DefaultBranch     string    `json:"default_branch"`
	Visibility        string    `json:"visibility"`
	Archived          bool      `json:"archived"`
	CreatedAt         time.Time `json:"created_at"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	Namespace         struct {
		FullPath string `json:"full_path"`
	} `json:"namespace"`
}

func (p gitLabProject) toRepository() Repository {
	return Repository{ID: strconv.FormatInt(p.ID, 10), Name: p.Name, FullName: p.PathWithNamespace, Description: p.Description,
		URL: p.WebURL, CloneURL: p.HTTPURL, SSHURL: p.SSHURL, DefaultBranch: p.DefaultBranch, IsPrivate: p.Visibility == "private",
		IsArchived: p.Archived, Owner: p.Namespace.FullPath, CreatedAt: p.CreatedAt, UpdatedAt: p.LastActivityAt}
}

type gitLabUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	PublicEmail string `json:"public_email"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

func (u gitLabUser) toUser() User {
	email := u.Email
	if email == "" {
		email = u.PublicEmail
	}
	return User{ID: strconv.FormatInt(u.ID, 10), Username: u.Username, Name: u.Name, Email: email, AvatarURL: u.AvatarURL}
}

type gitLabCommit struct {
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	WebURL         string    `json:"web_url"`
	AuthorName     string    `json:"author_name"`
	AuthorEmail    string    `json:"author_email"`
	AuthoredDate   time.Time `json:"authored_date"`
	CommitterName  string    `json:"committer_name"`
	CommitterEmail string    `json:"committer_email"`
	CommittedDate  time.Time `json:"committed_date"`
}

func (c gitLabCommit) toCommit() Commit {
	created := c.AuthoredDate
	if created.IsZero() {
		created = c.CommittedDate
	}
	return Commit{SHA: c.ID, Message: c.Message, URL: c.WebURL, Author: User{Name: c.AuthorName, Email: c.AuthorEmail}, Committer: User{Name: c.CommitterName, Email: c.CommitterEmail}, CreatedAt: created}
}

type gitLabBranch struct {
	Name      string       `json:"name"`
	Protected bool         `json:"protected"`
	Default   bool         `json:"default"`
	Commit    gitLabCommit `json:"commit"`
}

func (b gitLabBranch) toBranch() Branch {
	return Branch{Name: b.Name, SHA: b.Commit.ID, Protected: b.Protected, IsDefault: b.Default}
}

type gitLabMergeRequest struct {
	ID             int64      `json:"id"`
	IID            int        `json:"iid"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	State          string     `json:"state"`
	WebURL         string     `json:"web_url"`
	SourceBranch   string     `json:"source_branch"`
	TargetBranch   string     `json:"target_branch"`
	SHA            string     `json:"sha"`
	Draft          bool       `json:"draft"`
	WorkInProgress bool       `json:"work_in_progress"`
	Author         gitLabUser `json:"author"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at"`
	ClosedAt       *time.Time `json:"closed_at"`
}

func (mr gitLabMergeRequest) toPullRequest(repo string) PullRequest {
	state := mr.State
	if state == "opened" {
		state = "open"
	}
	merged := state == "merged" || mr.MergedAt != nil
	if merged {
		state = "merged"
	}
	return PullRequest{ID: int(mr.ID), Number: mr.IID, Title: mr.Title, Body: mr.Description, State: state, URL: mr.WebURL, HeadBranch: mr.SourceBranch, HeadRepo: repo, HeadSHA: mr.SHA, BaseBranch: mr.TargetBranch, IsMerged: merged, IsDraft: mr.Draft || mr.WorkInProgress, Author: mr.Author.toUser(), CreatedAt: mr.CreatedAt, UpdatedAt: mr.UpdatedAt, MergedAt: mr.MergedAt, ClosedAt: mr.ClosedAt}
}

type gitLabTag struct {
	Name   string       `json:"name"`
	Target string       `json:"target"`
	Commit gitLabCommit `json:"commit"`
}

type gitLabRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	ReleasedAt  time.Time `json:"released_at"`
	Upcoming    bool      `json:"upcoming_release"`
	Historical  bool      `json:"historical_release"`
	Links       struct {
		Self string `json:"self"`
	} `json:"_links"`
	Assets struct {
		Sources []struct {
			Format string `json:"format"`
			URL    string `json:"url"`
		} `json:"sources"`
		Links []struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			DirectURL string `json:"direct_asset_url"`
			LinkType  string `json:"link_type"`
		} `json:"links"`
	} `json:"assets"`
}

func (r gitLabRelease) toRelease(projectURL string) Release {
	status := "released"
	if r.Upcoming {
		status = "upcoming"
	} else if r.Historical {
		status = "historical"
	}
	assets := make([]models.SCMReleaseAsset, 0, len(r.Assets.Sources)+len(r.Assets.Links))
	for _, source := range r.Assets.Sources {
		assets = append(assets, models.SCMReleaseAsset{Kind: "source", Name: source.Format, URL: source.URL, Format: source.Format})
	}
	for _, link := range r.Assets.Links {
		assets = append(assets, models.SCMReleaseAsset{Kind: "link", Name: link.Name, URL: link.URL, DirectURL: link.DirectURL, LinkType: link.LinkType})
	}
	var released *time.Time
	if !r.ReleasedAt.IsZero() {
		releasedAt := r.ReleasedAt
		released = &releasedAt
	}
	return Release{ID: r.TagName, TagName: r.TagName, TagURL: projectURL + "/-/tags/" + url.PathEscape(r.TagName), Name: r.Name, Body: r.Description, URL: r.Links.Self, Status: status, Assets: assets, CreatedAt: r.CreatedAt, PublishedAt: released, ReleasedAt: released}
}

type gitLabNote struct {
	ID        int64      `json:"id"`
	Type      string     `json:"type"`
	Body      string     `json:"body"`
	Author    gitLabUser `json:"author"`
	System    bool       `json:"system"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Position  struct {
		NewPath      string `json:"new_path"`
		NewLine      int    `json:"new_line"`
		OldPath      string `json:"old_path"`
		OldLine      int    `json:"old_line"`
		PositionType string `json:"position_type"`
	} `json:"position"`
}

func (n gitLabNote) toIssueComment(kind, threadID string) IssueComment {
	path, line, side := n.Position.NewPath, n.Position.NewLine, "RIGHT"
	if path == "" {
		path, line, side = n.Position.OldPath, n.Position.OldLine, "LEFT"
	}
	return IssueComment{ID: n.ID, Kind: kind, Body: n.Body, User: n.Author.toUser(), Path: path, Line: line, Side: side, ThreadID: threadID, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

type gitLabDiscussion struct {
	ID             string       `json:"id"`
	IndividualNote bool         `json:"individual_note"`
	Notes          []gitLabNote `json:"notes"`
}
type gitLabHook struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}
