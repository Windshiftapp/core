// Package docmost is the OAuth client and search client for a self-hosted
// Docmost instance. Docmost is a public OAuth client (no secret, PKCE S256)
// and issues rotating refresh tokens.
package docmost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/utils"
)

// Stable error categories so callers can distinguish "reconnect required"
// from "provider unavailable".
var (
	ErrUnauthorized  = errors.New("docmost: unauthorized")
	ErrUnavailable   = errors.New("docmost: unavailable")
	ErrInvalidURL    = errors.New("docmost: invalid base URL")
	ErrOAuthDisabled = errors.New("docmost: oauth is not enabled on the instance")
)

// maxResponseBytes caps a proxied response. Docmost search responses are JSON
// snippets; anything larger is misconfiguration or a memory-exhaustion vector.
const maxResponseBytes = 2 << 20

// SearchResult is one normalized search hit.
type SearchResult struct {
	ID        string  `json:"id"`
	SlugID    string  `json:"slug_id"`
	Title     string  `json:"title"`
	SpaceSlug string  `json:"space_slug"`
	SpaceName string  `json:"space_name"`
	Highlight string  `json:"highlight"`
	Rank      float64 `json:"rank"`
}

// Token is the result of an authorization-code or refresh exchange.
type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    time.Time
}

// Client is the HTTP client for one Docmost instance.
type Client struct {
	http *http.Client
}

// NewClient builds a client over the SSRF-safe transport (no redirects,
// DNS-rebinding-safe dialing) with a 10 s timeout.
func NewClient() *Client {
	return &Client{http: utils.NewSSRFSafeHTTPClient(10 * time.Second)}
}

// NormalizeBaseURL trims and validates an admin-configured base URL. It is
// configuration supplied by an admin, so it may be plain HTTP on a private
// host; SSRF protection is enforced by the dialer, whose IP policy is governed
// by ALLOW_LOCAL_CONNECTIONS.
func NormalizeBaseURL(raw string) (string, error) {
	normalized := strings.TrimRight(strings.TrimSpace(raw), "/")
	if normalized == "" {
		return "", fmt.Errorf("%w: URL must not be empty", ErrInvalidURL)
	}
	if err := utils.ValidateHTTPBaseURL(normalized); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	return normalized, nil
}

// PageURL builds the browser URL for a search hit:
// {base}/s/{space.slug}/p/{slugId}.
func PageURL(baseURL, spaceSlug, slugID string) string {
	return strings.TrimRight(baseURL, "/") + "/s/" + spaceSlug + "/p/" + slugID
}

// Verify probes the instance with the given token.
func (c *Client) Verify(ctx context.Context, baseURL, token string) error {
	_, err := c.Search(ctx, baseURL, token, "", 1)
	return err
}

// Search runs an authenticated wiki search as the token's user. Docmost applies
// its own permissions, so the result set is exactly what that user may read.
func (c *Client) Search(ctx context.Context, baseURL, token, query string, limit int) ([]SearchResult, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrUnauthorized
	}
	if limit <= 0 {
		limit = 25
	}

	requestBody, err := json.Marshal(map[string]any{"query": query, "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("%w: encode request", ErrUnavailable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalized+"/api/search", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	body, status, err := c.do(req)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, status)
	}

	var parsed struct {
		Data struct {
			Items []struct {
				ID        string  `json:"id"`
				SlugID    string  `json:"slugId"`
				Title     string  `json:"title"`
				Highlight string  `json:"highlight"`
				Rank      float64 `json:"rank"`
				Space     struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"space"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode response", ErrUnavailable)
	}

	results := make([]SearchResult, 0, len(parsed.Data.Items))
	for _, item := range parsed.Data.Items {
		results = append(results, SearchResult{
			ID:        item.ID,
			SlugID:    item.SlugID,
			Title:     item.Title,
			SpaceSlug: item.Space.Slug,
			SpaceName: item.Space.Name,
			Highlight: item.Highlight,
			Rank:      item.Rank,
		})
	}
	return results, nil
}

// RegisterClient registers a public OAuth client (RFC 7591) and returns its id.
// Docmost exposes this only when an EE license is active and MCP is enabled;
// otherwise the endpoint 403s.
func (c *Client) RegisterClient(ctx context.Context, baseURL, clientName, redirectURI string) (string, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	body, status, err := c.postJSON(ctx, normalized+"/api/oauth/register", map[string]any{
		"client_name":   clientName,
		"redirect_uris": []string{redirectURI},
	})
	if err != nil {
		return "", err
	}
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return "", ErrOAuthDisabled
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return "", fmt.Errorf("%w: register status %d", ErrUnavailable, status)
	}
	var parsed struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ClientID == "" {
		return "", fmt.Errorf("%w: register response", ErrUnavailable)
	}
	return parsed.ClientID, nil
}

// ExchangeCode exchanges an authorization code for tokens using a public
// client and PKCE.
func (c *Client) ExchangeCode(ctx context.Context, baseURL, clientID, redirectURI, code, codeVerifier string) (Token, error) {
	return c.tokenRequest(ctx, baseURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	})
}

// RefreshToken exchanges a refresh token for a new token pair.
func (c *Client) RefreshToken(ctx context.Context, baseURL, clientID, refreshToken string) (Token, error) {
	return c.tokenRequest(ctx, baseURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
}

// RevokeToken revokes an access or refresh token. Best-effort.
func (c *Client) RevokeToken(ctx context.Context, baseURL, clientID, token string) error {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return err
	}
	_, _, err = c.postForm(ctx, normalized+"/api/oauth/revoke", url.Values{
		"token":     {token},
		"client_id": {clientID},
	})
	return err
}

func (c *Client) tokenRequest(ctx context.Context, baseURL string, form url.Values) (Token, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return Token{}, err
	}
	body, status, err := c.postForm(ctx, normalized+"/api/oauth/token", form)
	if err != nil {
		return Token{}, err
	}
	if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Token{}, ErrUnauthorized
	}
	if status != http.StatusOK {
		return Token{}, fmt.Errorf("%w: token status %d", ErrUnavailable, status)
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return Token{}, fmt.Errorf("%w: token response", ErrUnavailable)
	}
	token := Token{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		TokenType:    parsed.TokenType,
		Scope:        parsed.Scope,
	}
	if parsed.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	}
	return token, nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any) (body []byte, status int, err error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: encode request", ErrUnavailable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) (body []byte, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) do(req *http.Request) (body []byte, status int, err error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: read response", ErrUnavailable)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, 0, fmt.Errorf("%w: response exceeded %d bytes", ErrUnavailable, maxResponseBytes)
	}
	return body, resp.StatusCode, nil
}
