// Package email provides email integration including IMAP, OAuth, and provider-specific implementations.
package email

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

// Google IMAP settings
const (
	GoogleIMAPHost = "imap.gmail.com"
	GoogleIMAPPort = 993
)

// Google OAuth endpoints
const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token" //nolint:gosec // G101 false positive: OAuth endpoint URL, not a credential
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// GoogleDefaultScopes defines the default scopes for Gmail IMAP access.
var GoogleDefaultScopes = []string{
	"https://mail.google.com/", // Full Gmail access (required for IMAP)
	"https://www.googleapis.com/auth/userinfo.email",
}

// GoogleProvider implements OAuth email provider for Gmail
type GoogleProvider struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// NewGoogleProvider creates a new Google email provider
func NewGoogleProvider(clientID, clientSecret string, scopes []string) *GoogleProvider {
	if len(scopes) == 0 {
		scopes = GoogleDefaultScopes
	}
	return &GoogleProvider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
	}
}

// GetType returns the provider type identifier
func (p *GoogleProvider) GetType() string {
	return models.EmailProviderTypeGoogle
}

// GetIMAPServer returns Gmail IMAP server details
func (p *GoogleProvider) GetIMAPServer(config *models.ChannelConfig) (string, int) { //nolint:gocritic // unnamedResult
	return GoogleIMAPHost, GoogleIMAPPort
}

// GetOAuthURL returns the Google authorization URL
func (p *GoogleProvider) GetOAuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":     {p.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {strings.Join(p.Scopes, " ")},
		"state":         {state},
		"access_type":   {"offline"}, // Required for refresh token
		"prompt":        {"consent"}, // Force consent to get refresh token
	}
	return googleAuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens
func (p *GoogleProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error) {
	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("token exchange failed: %s - %s", errResp.Error, errResp.ErrorDescription)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    &expiresAt,
		Scope:        tokenResp.Scope,
	}, nil
}

// RefreshToken refreshes an expired access token
func (p *GoogleProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("token refresh failed: %s - %s", errResp.Error, errResp.ErrorDescription)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Google doesn't return a new refresh token
	return &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken, // Keep the original
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    &expiresAt,
		Scope:        tokenResp.Scope,
	}, nil
}

// GetUserEmail retrieves the email address of the authenticated user
func (p *GoogleProvider) GetUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", googleUserInfoURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", fmt.Errorf("failed to parse user info: %w", err)
	}

	return userInfo.Email, nil
}

// Connect establishes an IMAP connection using OAuth
func (p *GoogleProvider) Connect(ctx context.Context, config *models.ChannelConfig) (*Client, error) {
	if config.EmailOAuthAccessToken == "" {
		return nil, fmt.Errorf("no OAuth access token configured")
	}
	if config.EmailOAuthEmail == "" {
		return nil, fmt.Errorf("no OAuth email address configured")
	}

	client, err := Connect(ConnectOptions{
		Host:       GoogleIMAPHost,
		Port:       GoogleIMAPPort,
		Encryption: "ssl",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Gmail IMAP: %w", err)
	}

	if err := client.AuthenticateXOAuth2(config.EmailOAuthEmail, config.EmailOAuthAccessToken); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("XOAUTH2 authentication failed: %w", err)
	}

	return client, nil
}

// TestConnection tests if the IMAP connection can be established
func (p *GoogleProvider) TestConnection(ctx context.Context, config *models.ChannelConfig) error {
	client, err := p.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return nil
}
