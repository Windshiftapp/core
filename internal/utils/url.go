package utils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ValidateExternalURL checks that a URL is safe for server-side requests.
// It enforces HTTPS scheme and rejects URLs that resolve to private/loopback/link-local IPs.
func ValidateExternalURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL must not be empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("URL must use HTTPS scheme")
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL must have a valid hostname")
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("could not resolve hostname")
	}

	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return fmt.Errorf("URL must not resolve to a private or internal address")
		}
	}

	return nil
}

// NewSSRFSafeHTTPClient returns an *http.Client that blocks redirects and
// validates resolved IPs against private ranges before connecting (DNS rebinding defense).
func NewSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("connection refused")
			}

			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, fmt.Errorf("connection refused")
			}

			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return nil, fmt.Errorf("connection refused")
				}
			}

			// Connect to the first valid resolved IP
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
