package scm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"windshift/internal/models"
)

// fakeGiteaTokenServer returns an httptest.Server whose only handler is
// /login/oauth/access_token. The provided handler decides what to write.
func fakeGiteaTokenServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/login/oauth/access_token", handler)
	return httptest.NewServer(mux)
}

func TestGiteaRefreshToken_InvalidGrantMapsToTypedError(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "400 with invalid_grant body",
			statusCode: http.StatusBadRequest,
			body:       `{"error":"invalid_grant","error_description":"refresh token has been used or revoked"}`,
		},
		{
			name:       "200 with invalid_grant in body (some Gitea versions)",
			statusCode: http.StatusOK,
			body:       `{"error":"invalid_grant","error_description":"refresh token has been used"}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := fakeGiteaTokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.statusCode)
				_, _ = w.Write([]byte(c.body))
			})
			defer srv.Close()

			provider, err := NewGiteaProvider(ProviderConfig{
				ProviderType:      models.SCMProviderTypeGitea,
				AuthMethod:        models.SCMAuthMethodOAuth,
				BaseURL:           srv.URL,
				OAuthClientID:     "id",
				OAuthClientSecret: "secret",
			})
			if err != nil {
				t.Fatalf("NewGiteaProvider: %v", err)
			}

			_, err = provider.RefreshToken(context.Background(), "stale-refresh-token")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrRefreshTokenInvalid) {
				t.Fatalf("expected ErrRefreshTokenInvalid, got %v", err)
			}
		})
	}
}

func TestGiteaRefreshToken_TransientErrorIsNotInvalidGrant(t *testing.T) {
	srv := fakeGiteaTokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
	})
	defer srv.Close()

	provider, err := NewGiteaProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitea,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           srv.URL,
		OAuthClientID:     "id",
		OAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewGiteaProvider: %v", err)
	}

	_, err = provider.RefreshToken(context.Background(), "rt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("transient 5xx should not map to ErrRefreshTokenInvalid; got %v", err)
	}
	if !errors.Is(err, ErrProviderError) {
		t.Fatalf("expected ErrProviderError on transient failure, got %v", err)
	}
}

func TestGiteaRefreshToken_HappyPath(t *testing.T) {
	srv := fakeGiteaTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a refresh-token grant.
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.PostForm.Get("refresh_token"); got != "old-rt" {
			t.Errorf("refresh_token = %q, want old-rt", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","expires_in":3600,"token_type":"Bearer"}`))
	})
	defer srv.Close()

	provider, err := NewGiteaProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitea,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           srv.URL,
		OAuthClientID:     "id",
		OAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewGiteaProvider: %v", err)
	}

	tokens, err := provider.RefreshToken(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tokens.AccessToken != "new-at" {
		t.Errorf("AccessToken = %q, want new-at", tokens.AccessToken)
	}
	if tokens.RefreshToken != "new-rt" {
		t.Errorf("RefreshToken = %q, want new-rt", tokens.RefreshToken)
	}
	if tokens.ExpiresAt == nil {
		t.Error("ExpiresAt = nil, want non-nil for expires_in=3600")
	}
}

func TestRefreshLockKey_DistinguishesPrincipals(t *testing.T) {
	cases := []struct {
		name string
		a    *ProviderCredentials
		b    *ProviderCredentials
		conn int
		same bool
	}{
		{
			name: "same user same connection",
			a:    &ProviderCredentials{AuthSource: "user", UserID: 7},
			b:    &ProviderCredentials{AuthSource: "user", UserID: 7},
			conn: 12,
			same: true,
		},
		{
			name: "different users same connection",
			a:    &ProviderCredentials{AuthSource: "user", UserID: 7},
			b:    &ProviderCredentials{AuthSource: "user", UserID: 8},
			conn: 12,
			same: false,
		},
		{
			name: "user vs workspace are distinct even with overlapping IDs",
			a:    &ProviderCredentials{AuthSource: "user", UserID: 12},
			b:    &ProviderCredentials{AuthSource: "workspace"},
			conn: 12,
			same: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ka := refreshLockKeyFor(c.a, c.conn)
			kb := refreshLockKeyFor(c.b, c.conn)
			eq := ka == kb
			if eq != c.same {
				t.Fatalf("keys equal=%v, want %v (a=%+v b=%+v)", eq, c.same, ka, kb)
			}
		})
	}
}

func TestGitHubRevokeToken_Success(t *testing.T) {
	var capturedAuth, capturedBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/applications/test-client-id/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		capturedAuth = r.Header.Get("Authorization")
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		capturedBody = string(buf[:n])
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider, err := NewGitHubProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitHub,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           srv.URL,
		OAuthClientID:     "test-client-id",
		OAuthClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	if err := provider.RevokeToken(context.Background(), "the-access-token"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	// Authorization: Basic base64("test-client-id:test-client-secret")
	if !strings.HasPrefix(capturedAuth, "Basic ") {
		t.Errorf("Authorization header = %q, want Basic ...", capturedAuth)
	}
	if !strings.Contains(capturedBody, `"access_token":"the-access-token"`) {
		t.Errorf("body = %q, want access_token in body", capturedBody)
	}
}

func TestGitHubRevokeToken_404IsTreatedAsAlreadyRevoked(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/applications/cid/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	provider, err := NewGitHubProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitHub,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           srv.URL,
		OAuthClientID:     "cid",
		OAuthClientSecret: "csec",
	})
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	if err := provider.RevokeToken(context.Background(), "tok"); err != nil {
		t.Fatalf("404 should be treated as already-revoked, got err: %v", err)
	}
}

func TestGitHubRevokeToken_RequiresClientCreds(t *testing.T) {
	provider, err := NewGitHubProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitHub,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           "http://example.invalid",
		OAuthClientID:     "",
		OAuthClientSecret: "",
	})
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}
	if err := provider.RevokeToken(context.Background(), "tok"); err == nil {
		t.Fatal("expected error when client_id/secret missing")
	}
}

func TestGiteaProvider_DoesNotImplementTokenRevoker(t *testing.T) {
	provider, err := NewGiteaProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitea,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           "https://gitea.example",
		OAuthClientID:     "id",
		OAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewGiteaProvider: %v", err)
	}
	if _, ok := interface{}(provider).(TokenRevoker); ok {
		t.Fatal("Gitea provider must not implement TokenRevoker — there's no standardized revoke endpoint")
	}
}

func TestGitHubProvider_ImplementsTokenRevoker(t *testing.T) {
	provider, err := NewGitHubProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitHub,
		AuthMethod:        models.SCMAuthMethodOAuth,
		OAuthClientID:     "id",
		OAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}
	if _, ok := interface{}(provider).(TokenRevoker); !ok {
		t.Fatal("GitHub provider should implement TokenRevoker")
	}
}

// TestGiteaProvider_BaseURLNoTrailingSlash sanity-checks that token URL
// composition tolerates the user supplying a base URL with or without a
// trailing slash. Hits the same code path as the rest of these tests so
// the assertion is just that the request actually arrives.
func TestGiteaProvider_BaseURLNoTrailingSlash(t *testing.T) {
	hit := false
	srv := fakeGiteaTokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer","expires_in":60}`))
	})
	defer srv.Close()

	provider, err := NewGiteaProvider(ProviderConfig{
		ProviderType:      models.SCMProviderTypeGitea,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           strings.TrimRight(srv.URL, "/") + "/",
		OAuthClientID:     "id",
		OAuthClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewGiteaProvider: %v", err)
	}
	if _, err := provider.RefreshToken(context.Background(), "rt"); err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if !hit {
		t.Fatal("token endpoint was not hit; URL composition broken")
	}
}
