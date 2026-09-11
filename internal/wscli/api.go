package wscli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	apiMethod  string
	apiData    string
	apiInput   string
	apiHeaders []string
	apiInclude bool
)

var apiCmd = &cobra.Command{
	Use:   "api <endpoint>",
	Short: "Call an authenticated Windshift API endpoint",
	Long: `Call an API endpoint using the configured server and bearer token.

Endpoints from the OpenAPI document, such as /items/42/history, are resolved
under /rest/api/v2. A fully prefixed /rest/api/v2 path is also accepted. Other
API versions, browser-session routes, absolute URLs, and redirects are rejected.

The response body is written to stdout without decoding its JSON envelope.
Use --include to print the HTTP status and response headers before the body.
This is useful for reading ETag before a conditional mutation.

Examples:
  ws api /items/42/history
  ws api '/items?workspace_id=7&page_size=100'
  ws api /items/42 -X PATCH -H 'If-Match: "item-4"' -d '{"title":"Updated"}'
  ws api /items/bulk-update -X POST --input payload.json
  cat payload.json | ws api /items/bulk-update -X POST --input -`,
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, err := normalizeAPIEndpoint(args[0])
		if err != nil {
			return err
		}
		headers, err := parseAPIHeaders(apiHeaders)
		if err != nil {
			return err
		}
		body, hasBody, err := apiRequestBody(cmd)
		if err != nil {
			return err
		}
		defer func() { _ = body.Close() }()

		client, err := NewClient()
		if err != nil {
			return err
		}
		resp, err := client.apiRequest(cmd.Context(), apiMethod, endpoint, body, hasBody, headers)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if err := writeAPIResponse(stdout, resp, apiInclude); err != nil {
			return fmt.Errorf("write API response: %w", err)
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("API request failed: %s", resp.Status)
		}
		return nil
	},
}

func normalizeAPIEndpoint(raw string) (string, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return "", fmt.Errorf("API endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid API endpoint: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(endpoint, "//") {
		return "", fmt.Errorf("API endpoint must be a path, not an absolute URL")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("API endpoint must not contain a fragment")
	}
	for segment := range strings.SplitSeq(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("API endpoint must not contain dot path segments")
		}
	}

	if strings.HasPrefix(endpoint, "rest/api/") {
		endpoint = "/" + endpoint
		parsed, err = url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf("invalid API endpoint: %w", err)
		}
	}
	if parsed.Path == "/rest/api/v2" || strings.HasPrefix(parsed.Path, "/rest/api/v2/") {
		return endpoint, nil
	}
	if strings.HasPrefix(parsed.Path, "/rest/api/") || strings.HasPrefix(parsed.Path, "/api/") ||
		strings.HasPrefix(parsed.Path, "api/") {
		return "", fmt.Errorf("ws api supports only /rest/api/v2 endpoints")
	}
	return "/rest/api/v2/" + strings.TrimPrefix(endpoint, "/"), nil
}

func parseAPIHeaders(values []string) (http.Header, error) {
	headers := make(http.Header, len(values))
	for _, value := range values {
		name, headerValue, ok := strings.Cut(value, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid header %q: expected 'Name: value'", value)
		}
		if strings.EqualFold(name, "Authorization") {
			return nil, fmt.Errorf("authorization is managed by ws; use --token to select a token")
		}
		headers.Add(name, strings.TrimSpace(headerValue))
	}
	return headers, nil
}

func apiRequestBody(cmd *cobra.Command) (io.ReadCloser, bool, error) {
	if cmd.Flags().Changed("data") {
		return io.NopCloser(strings.NewReader(apiData)), true, nil
	}
	if apiInput == "" {
		return http.NoBody, false, nil
	}
	if apiInput == "-" {
		return io.NopCloser(stdin), true, nil
	}
	file, err := os.Open(apiInput) //nolint:gosec // the request body path is selected by the CLI caller
	if err != nil {
		return nil, false, fmt.Errorf("open API input %s: %w", apiInput, err)
	}
	return file, true, nil
}

func (c *Client) apiRequest(ctx context.Context, method, endpoint string, body io.Reader, hasBody bool, headers http.Header) (*http.Response, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create API request: %w", err)
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if hasBody && req.Header.Get("Content-Type") == "" {
		contentType := "application/json"
		if method == http.MethodPatch {
			contentType = "application/merge-patch+json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	// Authentication is owned by ws and cannot be replaced through --header.
	req.Header.Set("Authorization", "Bearer "+c.token)

	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] %s %s\n", method, endpoint)
	}
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req) //nolint:gosec // the endpoint is restricted to the configured Windshift server
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] -> status=%d\n", resp.StatusCode)
	}
	return resp, nil
}

func writeAPIResponse(out io.Writer, resp *http.Response, include bool) error {
	if include {
		if _, err := fmt.Fprintf(out, "%s %s\r\n", resp.Proto, resp.Status); err != nil {
			return err
		}
		if err := resp.Header.Write(out); err != nil {
			return err
		}
		if _, err := io.WriteString(out, "\r\n"); err != nil {
			return err
		}
	}
	_, err := io.Copy(out, resp.Body)
	return err
}

func init() {
	rootCmd.AddCommand(apiCmd)

	apiCmd.Flags().StringVarP(&apiMethod, "method", "X", http.MethodGet, "HTTP method")
	apiCmd.Flags().StringVarP(&apiData, "data", "d", "", "inline request body")
	apiCmd.Flags().StringVar(&apiInput, "input", "", "request body file (use - for stdin)")
	apiCmd.Flags().StringArrayVarP(&apiHeaders, "header", "H", nil, "request header in 'Name: value' format (repeatable)")
	apiCmd.Flags().BoolVarP(&apiInclude, "include", "i", false, "include response status and headers")
	apiCmd.MarkFlagsMutuallyExclusive("data", "input")
}
