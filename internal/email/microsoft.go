package email

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"windshift/internal/models"
)

// Microsoft 365 IMAP settings
const (
	MicrosoftIMAPHost = "outlook.office365.com"
	MicrosoftIMAPPort = 993
)

// Microsoft OAuth endpoints
const (
	microsoftAuthURLTemplate  = "https://login.microsoftonline.com/%s/oauth2/v2.0/authorize"
	microsoftTokenURLTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token" //nolint:gosec // G101 false positive: OAuth endpoint URL, not a credential
	microsoftUserInfoURL      = "https://graph.microsoft.com/v1.0/me"
)

// MicrosoftDefaultScopes defines the default scopes for Microsoft 365 IMAP access.
var MicrosoftDefaultScopes = []string{
	"https://outlook.office365.com/IMAP.AccessAsUser.All",
	"offline_access", // Required for refresh tokens
	"openid",
	"email",
}

// MicrosoftProvider implements OAuth email provider for Microsoft 365
type MicrosoftProvider struct {
	ClientID     string
	ClientSecret string
	TenantID     string // "common" for multi-tenant, or specific tenant ID
	Scopes       []string
}

// NewMicrosoftProvider creates a new Microsoft 365 email provider
func NewMicrosoftProvider(clientID, clientSecret, tenantID string, scopes []string) *MicrosoftProvider {
	if tenantID == "" {
		tenantID = "common"
	}
	if len(scopes) == 0 {
		scopes = MicrosoftDefaultScopes
	}
	return &MicrosoftProvider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TenantID:     tenantID,
		Scopes:       scopes,
	}
}

// GetType returns the provider type identifier
func (p *MicrosoftProvider) GetType() string {
	return models.EmailProviderTypeMicrosoft
}

// GetIMAPServer returns Microsoft 365 IMAP server details
func (p *MicrosoftProvider) GetIMAPServer(config *models.ChannelConfig) (host string, port int) {
	return MicrosoftIMAPHost, MicrosoftIMAPPort
}

// GetOAuthURL returns the Microsoft authorization URL
func (p *MicrosoftProvider) GetOAuthURL(state, redirectURI string) string {
	authURL := fmt.Sprintf(microsoftAuthURLTemplate, p.TenantID)
	params := url.Values{
		"client_id":     {p.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"response_mode": {"query"},
		"scope":         {strings.Join(p.Scopes, " ")},
		"state":         {state},
	}
	return authURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens
func (p *MicrosoftProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error) {
	tokenURL := fmt.Sprintf(microsoftTokenURLTemplate, p.TenantID)

	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {strings.Join(p.Scopes, " ")},
	}

	return exchangeOAuthToken(ctx, tokenURL, params)
}

// RefreshToken refreshes an expired access token
func (p *MicrosoftProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	tokenURL := fmt.Sprintf(microsoftTokenURLTemplate, p.TenantID)

	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {strings.Join(p.Scopes, " ")},
	}

	tokens, err := exchangeOAuthToken(ctx, tokenURL, params)
	if err != nil {
		return nil, err
	}

	// Microsoft may return a new refresh token
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}

	return tokens, nil
}

// GetUserEmail retrieves the email address of the authenticated user
func (p *MicrosoftProvider) GetUserEmail(ctx context.Context, accessToken string) (string, error) {
	var userInfo struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
		PreferredLanguage string `json:"preferredLanguage"`
	}

	if err := fetchOAuthJSON(ctx, microsoftUserInfoURL, accessToken, &userInfo); err != nil {
		return "", err
	}

	// Prefer mail, fall back to userPrincipalName
	email := userInfo.Mail
	if email == "" {
		email = userInfo.UserPrincipalName
	}

	return email, nil
}

// Connect establishes an IMAP connection using OAuth
func (p *MicrosoftProvider) Connect(ctx context.Context, config *models.ChannelConfig) (*Client, error) {
	if config.EmailOAuthAccessToken == "" {
		return nil, fmt.Errorf("no OAuth access token configured")
	}
	if config.EmailOAuthEmail == "" {
		return nil, fmt.Errorf("no OAuth email address configured")
	}

	client, err := Connect(ConnectOptions{
		Host:       MicrosoftIMAPHost,
		Port:       MicrosoftIMAPPort,
		Encryption: "ssl",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Microsoft IMAP: %w", err)
	}

	if err := client.AuthenticateXOAuth2(config.EmailOAuthEmail, config.EmailOAuthAccessToken); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("XOAUTH2 authentication failed: %w", err)
	}

	return client, nil
}

// TestConnection tests if the IMAP connection can be established
func (p *MicrosoftProvider) TestConnection(ctx context.Context, config *models.ChannelConfig) error {
	client, err := p.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return nil
}
