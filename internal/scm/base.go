package scm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// baseProvider holds shared HTTP plumbing for SCM providers.
type baseProvider struct {
	httpClient          *http.Client
	setAuthHeader       func(req *http.Request)
	handleErrorResponse func(resp *http.Response) error
}

// doJSON performs an authenticated HTTP request and decodes the JSON response into result.
// It handles request creation, auth headers, status checking, and response body closing.
// expectedStatus is the HTTP status code that indicates success (e.g., http.StatusOK).
func (b *baseProvider) doJSON(ctx context.Context, method, reqURL string,
	body io.Reader, expectedStatus int, result interface{}) error {

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
