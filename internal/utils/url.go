package utils

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValidateClientRedirectURL checks that a URL is safe to send back to a browser
// for client-side navigation (e.g. window.location.href = url). It blocks
// javascript:, data:, vbscript:, file:, and protocol-relative URLs that would
// otherwise execute attacker JS in the form-submitter's origin.
//
// Allows http(s) absolute URLs only — matches the client-side isValidHttpUrl
// validator in ChannelFormConfig.svelte. Empty input is treated as a no-op
// (callers handle the "redirect_url is optional" case themselves).
func ValidateClientRedirectURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("redirect URL must use http(s) scheme")
	}

	if parsed.Host == "" {
		return fmt.Errorf("redirect URL must have a host")
	}

	return nil
}

// ValidateBrowserNavigationURL checks a URL that will be placed in an anchor
// href. It mirrors the browser-side safeHref allow-list: absolute HTTP(S),
// mailto/tel links, and same-origin paths or fragments are accepted. In
// particular, protocol-relative and executable schemes are rejected.
func ValidateBrowserNavigationURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	if rawURL != strings.TrimSpace(rawURL) || strings.ContainsAny(rawURL, "\t\r\n") {
		return fmt.Errorf("invalid URL")
	}
	if strings.HasPrefix(rawURL, "#") {
		return nil
	}
	if strings.HasPrefix(rawURL, "/") {
		return validateSameOriginPath(rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Hostname() == "" || parsed.User != nil {
			return fmt.Errorf("URL must have an unambiguous host")
		}
	case "mailto", "tel":
		if parsed.Opaque == "" && parsed.Path == "" {
			return fmt.Errorf("URL target must not be empty")
		}
	default:
		return fmt.Errorf("URL scheme is not allowed")
	}
	return nil
}

// ValidateBrowserAssetURL checks a URL rendered as a public image or CSS
// background. Portal branding uploads intentionally return same-origin paths,
// while legacy/custom configurations may contain absolute HTTP(S) URLs.
func ValidateBrowserAssetURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	if rawURL != strings.TrimSpace(rawURL) || strings.ContainsAny(rawURL, "\t\r\n") {
		return fmt.Errorf("invalid URL")
	}
	if strings.HasPrefix(rawURL, "/") {
		return validateSameOriginPath(rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("asset URL must be an HTTP(S) URL or same-origin path")
	}
	return nil
}

func validateSameOriginPath(rawURL string) error {
	if strings.HasPrefix(rawURL, "//") || strings.HasPrefix(rawURL, "/\\") || strings.Contains(rawURL, "\\") {
		return fmt.Errorf("protocol-relative URL is not allowed")
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return fmt.Errorf("invalid same-origin path")
	}
	return nil
}

// ValidateHTTPBaseURL checks that a server-configured base URL is syntactically
// valid for outbound HTTP clients. It intentionally does not resolve DNS or
// reject private hosts: some admin-configured integrations (notably local/custom
// LLM providers such as Ollama) are expected to point at localhost or private
// network addresses. Use ValidateExternalURL instead for untrusted/user-provided
// URLs that need SSRF protection.
func ValidateHTTPBaseURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL must not be empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL must use HTTP(S) scheme")
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("URL must have a valid hostname")
	}

	if parsed.Fragment != "" {
		return fmt.Errorf("URL must not include a fragment")
	}

	return nil
}

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
		if !AllowLocalConnections() && IsPrivateIP(ip) {
			return fmt.Errorf("URL must not resolve to a private or internal address")
		}
	}

	return nil
}

// NewSSRFSafeHTTPClient returns an *http.Client that blocks redirects and
// refuses loopback/private/metadata destinations at connect time (DNS rebinding
// defense), using the shared SSRF-safe transport.
func NewSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewSSRFSafeTransport(timeout),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
