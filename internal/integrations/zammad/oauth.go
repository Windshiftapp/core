package zammad

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidGrant = errors.New("zammad OAuth authorization is no longer valid")

type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// ExchangeOAuthCode and RefreshOAuthToken keep OAuth responses out of logs and
// return only a generic upstream error for malformed remote replies.
func ExchangeOAuthCode(ctx context.Context, transport Transport, tokenURL, clientID, clientSecret, code, redirectURI string) (*OAuthTokens, error) {
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "client_secret": {clientSecret}, "code": {code}, "redirect_uri": {redirectURI}}
	return requestOAuthTokens(ctx, transport, tokenURL, values)
}

func RefreshOAuthToken(ctx context.Context, transport Transport, tokenURL, clientID, clientSecret, refreshToken string) (*OAuthTokens, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "client_secret": {clientSecret}, "refresh_token": {refreshToken}}
	return requestOAuthTokens(ctx, transport, tokenURL, values)
}

func requestOAuthTokens(ctx context.Context, transport Transport, tokenURL string, values url.Values) (*OAuthTokens, error) {
	response, err := transport.Do(ctx, http.MethodPost, tokenURL, []byte(values.Encode()), map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return nil, &UpstreamError{Cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(response.Body, &problem)
		if problem.Error == "invalid_grant" {
			return nil, ErrInvalidGrant
		}
		return nil, &APIError{StatusCode: response.StatusCode}
	}
	var payload struct {
		AccessToken  string          `json:"access_token"`
		RefreshToken string          `json:"refresh_token"`
		ExpiresIn    json.RawMessage `json:"expires_in"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil || strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" {
		return nil, &UpstreamError{Cause: errors.New("invalid OAuth token response")}
	}
	seconds, err := strconv.ParseInt(strings.Trim(string(payload.ExpiresIn), `"`), 10, 64)
	if err != nil || seconds <= 0 {
		return nil, &UpstreamError{Cause: errors.New("invalid OAuth token expiry")}
	}
	return &OAuthTokens{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresIn: time.Duration(seconds) * time.Second}, nil
}
