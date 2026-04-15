// Package email provides email integration including IMAP, OAuth, and provider-specific implementations.
package email

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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
	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	return exchangeOAuthToken(ctx, googleTokenURL, params)
}

// RefreshToken refreshes an expired access token
func (p *GoogleProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	tokens, err := exchangeOAuthToken(ctx, googleTokenURL, params)
	if err != nil {
		return nil, err
	}

	// Google doesn't return a new refresh token
	tokens.RefreshToken = refreshToken

	return tokens, nil
}

// GetUserEmail retrieves the email address of the authenticated user
func (p *GoogleProvider) GetUserEmail(ctx context.Context, accessToken string) (string, error) {
	var userInfo struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := fetchOAuthJSON(ctx, googleUserInfoURL, accessToken, &userInfo); err != nil {
		return "", err
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
