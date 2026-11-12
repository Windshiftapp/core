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

// newSCMHTTPClient builds the HTTP client used by every SCM provider. The
// provider base URL is operator-configured and all requests — including the
// OAuth token exchange that carries the client secret — go to it, so the
// client dials through the SSRF-safe dialer: a base URL (or a redirect) that
// resolves to a loopback/RFC1918/link-local/CGNAT/metadata address is refused
// before the handshake. Redirect-following is preserved and re-checked per hop.
func newSCMHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: utils.NewSSRFSafeTransport(timeout),
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
