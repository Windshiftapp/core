package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	ErrGitHubVerifiedEmail    = errors.New("GitHub account must have a verified email address")
	ErrGitHubOrganization     = errors.New("GitHub organization membership could not be verified")
	githubOrganizationPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)
)

// ValidateGitHubOrganization accepts a login name, not a URL or API path.
func ValidateGitHubOrganization(organization string) bool {
	return organization == "" || (githubOrganizationPattern.MatchString(organization) && !strings.Contains(organization, "--"))
}

// GitHubScopes never grants repository access for login.
func GitHubScopes(organization string) []string {
	scopes := []string{"read:user", "user:email"}
	if organization != "" {
		scopes = append(scopes, "read:org")
	}
	return scopes
}

func GitHubOAuthConfig(provider *SSOProvider, secret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: provider.ClientID, ClientSecret: secret, RedirectURL: redirectURI,
		Scopes: GitHubScopes(provider.GitHubOrganization),
		Endpoint: oauth2.Endpoint{ // #nosec G101 -- Public OAuth endpoints, not credentials.
			AuthURL:   "https://github.com/login/oauth/authorize",
			TokenURL:  "https://github.com/login/oauth/access_token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// GitHubService exchanges short-lived login credentials without storing SCM tokens.
type GitHubService struct{ client *http.Client }

func NewGitHubService() *GitHubService {
	return &GitHubService{client: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *GitHubService) Authenticate(ctx context.Context, provider *SSOProvider, secret, redirectURI, code, verifier string) (*OIDCClaims, error) {
	config := GitHubOAuthConfig(provider, secret, redirectURI)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("GitHub token exchange failed: %w", err)
	}
	if token.AccessToken == "" {
		return nil, errors.New("GitHub returned an empty access token")
	}
	return s.fetchIdentity(ctx, token.AccessToken, provider.GitHubOrganization)
}

func (s *GitHubService) fetchIdentity(ctx context.Context, token, organization string) (*OIDCClaims, error) {
	var profile struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := s.get(ctx, token, "/user", &profile); err != nil {
		return nil, err
	}
	if profile.ID <= 0 || profile.Login == "" {
		return nil, errors.New("GitHub returned an invalid identity")
	}

	// Membership is checked before any account lookup, including linked identities.
	if organization != "" {
		if !ValidateGitHubOrganization(organization) {
			return nil, ErrGitHubOrganization
		}
		var membership struct {
			State        string `json:"state"`
			Organization struct {
				Login string `json:"login"`
			} `json:"organization"`
		}
		if err := s.get(ctx, token, "/user/memberships/orgs/"+url.PathEscape(organization), &membership); err != nil {
			return nil, ErrGitHubOrganization
		}
		if membership.State != "active" || !strings.EqualFold(membership.Organization.Login, organization) {
			return nil, ErrGitHubOrganization
		}
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	// Ask for up to 100 addresses; never trust the public profile's email field.
	if err := s.get(ctx, token, "/user/emails?per_page=100", &emails); err != nil {
		return nil, err
	}
	email := ""
	for _, candidate := range emails {
		if !candidate.Verified || strings.TrimSpace(candidate.Email) == "" {
			continue
		}
		if email == "" || candidate.Primary {
			email = candidate.Email
		}
		if candidate.Primary {
			break
		}
	}
	if email == "" {
		return nil, ErrGitHubVerifiedEmail
	}
	return &OIDCClaims{
		Subject: strconv.FormatInt(profile.ID, 10), Username: profile.Login,
		Email: email, EmailVerified: true, EmailVerifiedProvided: true,
		Name: profile.Name, Picture: profile.AvatarURL,
	}, nil
}

func (s *GitHubService) get(ctx context.Context, token, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Windshift")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("invalid GitHub response: %w", err)
	}
	return nil
}
