package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/integrations/zammad"
	"windshift/internal/utils"
)

var errZammadRedirect = errors.New("zammad redirects are not allowed")

const zammadHTTPTimeout = 30 * time.Second

// newZammadSafeTransport is deliberately separate from the generic action
// transport. It keeps the exact configured Zammad origin and endpoint prefix,
// while its dialer honors the operator's ALLOW_LOCAL_CONNECTIONS policy.
func newZammadSafeTransport(baseURL, allowedPathPrefix string) zammad.Transport {
	base, err := url.Parse(baseURL)
	return zammad.TransportFunc(func(ctx context.Context, method, targetURL string, body []byte, headers map[string]string) (*zammad.Response, error) {
		if err != nil || base == nil || base.Scheme == "" || base.Host == "" {
			return nil, errors.New("invalid Zammad base URL")
		}
		target, err := url.Parse(targetURL)
		if err != nil || target == nil || target.Scheme == "" || target.Host == "" {
			return nil, errors.New("invalid Zammad request URL")
		}
		allowedPath := strings.TrimRight(base.EscapedPath(), "/") + allowedPathPrefix
		pathAllowed := target.EscapedPath() == allowedPath
		if strings.HasSuffix(allowedPath, "/") {
			pathAllowed = strings.HasPrefix(target.EscapedPath(), allowedPath)
		}
		if target.User != nil || target.Fragment != "" || target.Scheme != base.Scheme || !strings.EqualFold(target.Host, base.Host) || !pathAllowed {
			return nil, errors.New("zammad request URL is outside the allowed origin or path")
		}
		request, err := http.NewRequestWithContext(ctx, method, target.String(), strings.NewReader(string(body)))
		if err != nil {
			return nil, errors.New("could not create Zammad request")
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		transport := utils.ConfigureHTTPTransport(&http.Transport{DialContext: utils.SafeNetDialer(10 * time.Second).DialContext, DisableKeepAlives: true})
		client := &http.Client{Timeout: zammadHTTPTimeout, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errZammadRedirect }}
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("zammad request failed: %w", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			return nil, errZammadRedirect
		}
		const maxResponseBytes = 1 << 20
		responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		if err != nil {
			return nil, errors.New("could not read Zammad response")
		}
		if len(responseBody) > maxResponseBytes {
			return nil, errors.New("zammad response exceeds 1 MiB")
		}
		return &zammad.Response{StatusCode: response.StatusCode, Body: responseBody}, nil
	})
}
