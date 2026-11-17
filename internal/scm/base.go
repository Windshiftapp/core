package scm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"windshift/internal/utils"
)

// baseProvider holds shared HTTP plumbing for SCM providers.
type baseProvider struct {
	httpClient          *http.Client
	setAuthHeader       func(req *http.Request)
	handleErrorResponse func(resp *http.Response) error
}

// HTTPClientPool owns credential-free HTTP plumbing shared by short-lived
// providers. The owning service closes idle connections after draining its work.
type HTTPClientPool struct {
	client *http.Client
}

func NewHTTPClientPool() *HTTPClientPool {
	return &HTTPClientPool{client: newSCMHTTPClient(30 * time.Second)}
}

func (p *HTTPClientPool) CloseIdleConnections() {
	p.client.CloseIdleConnections()
}

func clientPoolOrNew(pools []*HTTPClientPool) *HTTPClientPool {
	if len(pools) > 0 && pools[0] != nil {
		return pools[0]
	}
	return NewHTTPClientPool()
}

func (cfg ProviderConfig) httpClient() *http.Client {
	if cfg.HTTPClientPool != nil {
		return cfg.HTTPClientPool.client
	}
	return newSCMHTTPClient(30 * time.Second)
}

// newSCMHTTPClient preserves default redirect handling and the shared TLS,
// proxy and post-resolution SSRF policy for every new connection.
func newSCMHTTPClient(timeout time.Duration) *http.Client {
	transport := utils.NewSSRFSafeTransport(timeout)
	transport.MaxConnsPerHost = 16
	transport.MaxIdleConnsPerHost = 8
	transport.MaxIdleConns = 100
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// doJSON performs an authenticated HTTP request and decodes the JSON response into result.
// It handles request creation, auth headers, status checking, and response body closing.
// expectedStatus is the HTTP status code that indicates success (e.g., http.StatusOK).
func (b *baseProvider) doJSON(ctx context.Context, method, reqURL string,
	body io.Reader, expectedStatus int, result any) error {

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return err
	}
	b.setAuthHeader(req)

	if body != nil && body != http.NoBody {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != expectedStatus {
		return b.handleErrorResponse(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return err
		}
	}
	return nil
}
