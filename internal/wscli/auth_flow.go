package wscli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scopes requested by the automatic onboarding flow. Kept minimal — enough for
// every currently-shipped `ws` command but none of the admin namespace.
var defaultCLIScopes = []string{
	"items:read",
	"items:write",
	"workspaces:read",
	"workspaces:write",
	"users:read",
	"item-types:read",
	"workflows:read",
}

type cliCapabilities struct {
	AutoOnboardingEnabled bool   `json:"auto_onboarding_enabled"`
	ManualTokensEnabled   bool   `json:"manual_tokens_enabled"`
	AgentsEnabled         bool   `json:"agents_enabled"`
	TokenPolicy           string `json:"token_policy"`
}

type cliAuthResult struct {
	Token       string
	Agent       string
	InstanceURL string
}

type cliExchangeResponse struct {
	Token string `json:"token"`
	Agent struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"agent"`
	Scopes   []string `json:"scopes"`
	Hostname string   `json:"hostname"`
}

// fetchCLICapabilities probes the unauthenticated capabilities endpoint so
// the CLI can decide between the automatic flow and the manual fallback
// before holding any credentials.
func fetchCLICapabilities(instanceURL string) (*cliCapabilities, error) {
	endpoint := strings.TrimSuffix(instanceURL, "/") + "/api/cli/capabilities"
	req, err := http.NewRequest(http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G107: endpoint composed from user-configured URL, not arbitrary input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		// Older server that doesn't know about CLI onboarding — treat as
		// "auto disabled", manual still works.
		return &cliCapabilities{AutoOnboardingEnabled: false, ManualTokensEnabled: true}, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("capabilities probe failed: status %d", resp.StatusCode)
	}
	var c cliCapabilities
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// runCLIAuthFlow orchestrates the browser-based consent flow. It binds a
// loopback listener, opens the consent page, waits for the browser to POST
// back with a one-time code, then exchanges the code for a token. The
// caller owns the returned token and must persist it.
func runCLIAuthFlow(instanceURL, agentName, hostname string, scopes []string) (*cliAuthResult, error) {
	if hostname == "" {
		hostname = hostnameForAgent()
	}
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to bind callback listener: %w", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("listener returned non-TCP address %T", ln.Addr())
	}
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", tcpAddr.Port)

	type callback struct {
		code, state, result string
	}
	done := make(chan callback, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		cb := callback{
			code:   q.Get("code"),
			state:  q.Get("state"),
			result: q.Get("result"),
		}
		select {
		case done <- cb:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if cb.result == "ok" {
			_, _ = io.WriteString(w, successHTML)
		} else {
			_, _ = io.WriteString(w, deniedHTML)
		}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	authorizeURL, err := buildAuthorizeURL(instanceURL, state, callbackURL, agentName, hostname, scopes)
	if err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintln(stdout, "Opening browser to authorize the CLI…")
	// #nosec G705 -- writing to a CLI terminal, not HTML; G705 is checking for an XSS sink that doesn't exist here
	_, _ = fmt.Fprintf(stdout, "  %s\n", authorizeURL)
	if err := openBrowser(authorizeURL); err != nil {
		_, _ = fmt.Fprintf(stdout, "Could not open a browser automatically (%s).\n", err)
		_, _ = fmt.Fprintln(stdout, "Open the URL above manually to continue.")
	}
	_, _ = fmt.Fprintln(stdout, "Waiting for browser approval (press Ctrl-C to cancel)…")

	select {
	case cb := <-done:
		if cb.state != state {
			return nil, fmt.Errorf("auth state mismatch — aborting")
		}
		switch cb.result {
		case "ok":
			if cb.code == "" {
				return nil, fmt.Errorf("browser callback returned no code")
			}
			return exchangeCode(instanceURL, cb.code, state)
		case "denied":
			return nil, fmt.Errorf("authorization denied in the browser")
		default:
			return nil, fmt.Errorf("unexpected browser callback result %q", cb.result)
		}
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("timed out waiting for browser approval")
	}
}

func exchangeCode(instanceURL, code, state string) (*cliAuthResult, error) {
	body, _ := json.Marshal(map[string]string{"code": code, "state": state})
	endpoint := strings.TrimSuffix(instanceURL, "/") + "/api/cli/auth/exchange"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G107: endpoint composed from user-configured URL
	if err != nil {
		return nil, fmt.Errorf("exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("exchange failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var r cliExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &cliAuthResult{Token: r.Token, Agent: r.Agent.Username, InstanceURL: instanceURL}, nil
}

// buildAuthorizeURL composes the consent-page URL. It strips a trailing /api
// from the configured server URL the same way buildItemURL does, so the
// browser lands on the web UI even when the API base is namespaced.
func buildAuthorizeURL(instanceURL, state, callback, agentName, hostname string, scopes []string) (string, error) {
	u, err := url.Parse(strings.TrimSuffix(instanceURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid instance URL: %s", instanceURL)
	}
	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, "/api")
	u.Path = path + "/cli/authorize"
	q := u.Query()
	q.Set("state", state)
	q.Set("callback", callback)
	q.Set("hostname", hostname)
	q.Set("agent_name", agentName)
	q.Set("scope", strings.Join(scopes, ","))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hostnameForAgent returns a sanitized hostname that is safe to use as part
// of a generated agent username.
func hostnameForAgent() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "local"
	}
	h = strings.ToLower(h)
	h = strings.TrimSuffix(h, ".local")
	h = strings.TrimSuffix(h, ".lan")
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '.', r == '_', r == ' ':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "local"
	}
	if len(s) > 20 {
		s = s[:20]
	}
	return s
}

func defaultGlobalAgentName() string {
	return "ws-cli-" + hostnameForAgent()
}

func projectSlug() string {
	wd, err := os.Getwd()
	if err != nil {
		return "project"
	}
	base := filepath.Base(wd)
	base = strings.ToLower(base)
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '.', r == '_', r == ' ':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "project"
	}
	if len(s) > 16 {
		s = s[:16]
	}
	return s
}

const successHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Windshift CLI connected</title></head>
<body style="font-family: system-ui, sans-serif; padding: 40px; color: #1f2937;">
  <h2 style="margin-top: 0;">Windshift CLI connected</h2>
  <p>You can close this window and return to your terminal.</p>
</body>
</html>
`

const deniedHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Authorization denied</title></head>
<body style="font-family: system-ui, sans-serif; padding: 40px; color: #1f2937;">
  <h2 style="margin-top: 0;">Authorization denied</h2>
  <p>You can close this window.</p>
</body>
</html>
`
