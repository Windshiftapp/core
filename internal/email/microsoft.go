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

// Microsoft 365 IMAP settings
const (
	MicrosoftIMAPHost = "outlook.office365.com"
	MicrosoftIMAPPort = 993
)

// Microsoft OAuth endpoints
const (
	microsoftAuthURLTemplate  = "https://login.microsoftonline.com/%s/oauth2/v2.0/authorize"
	microsoftTokenURLTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	microsoftUserInfoURL      = "https://graph.microsoft.com/v1.0/me"
)

// Default scopes for Microsoft 365 IMAP access
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
func (p *MicrosoftProvider) GetIMAPServer(config *models.ChannelConfig) (string, int) {
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

	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {strings.Join(p.Scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
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
		json.Unmarshal(body, &errResp)
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
func (p *MicrosoftProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	tokenURL := fmt.Sprintf(microsoftTokenURLTemplate, p.TenantID)

	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {strings.Join(p.Scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
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
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("token refresh failed: %s - %s", errResp.Error, errResp.ErrorDescription)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Microsoft may return a new refresh token
	if tokenResp.RefreshToken == "" {
		tokenResp.RefreshToken = refreshToken
	}

	return &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    &expiresAt,
		Scope:        tokenResp.Scope,
	}, nil
}

// GetUserEmail retrieves the email address of the authenticated user
func (p *MicrosoftProvider) GetUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", microsoftUserInfoURL, nil)
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
		Mail                string `json:"mail"`
		UserPrincipalName   string `json:"userPrincipalName"`
		PreferredLanguage   string `json:"preferredLanguage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", fmt.Errorf("failed to parse user info: %w", err)
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
		client.Close()
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
	defer client.Close()
	return nil
}
