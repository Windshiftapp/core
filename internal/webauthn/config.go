// Package webauthn provides WebAuthn configuration and passkey authentication support.
package webauthn

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Config holds the WebAuthn configuration
type Config struct {
	RPID          string   // Relying Party ID (domain)
	RPName        string   // Display name
	RPOrigins     []string // Allowed origins
	Debug         bool     // Debug mode
	webAuthn      *webauthn.WebAuthn
	isDevelopment bool
}

// InvalidRPIDError identifies a relying-party ID that cannot be used by the
// WebAuthn implementation. Keeping this error typed lets the server disable
// only the optional passkey surface for local installations with a hostname
// such as "windshift", while still failing fast for unrelated configuration
// errors.
type InvalidRPIDError struct {
	RPID string
	Err  error
}

func (e *InvalidRPIDError) Error() string {
	return fmt.Sprintf("WebAuthn RP ID %q is invalid: %v", e.RPID, e.Err)
}

func (e *InvalidRPIDError) Unwrap() error { return e.Err }

// MissingOriginsError reports that no browser-visible origin could be derived,
// which leaves the relying party unable to verify a ceremony. Like an invalid
// RP ID this only disables the optional passkey surface, because an
// installation that never uses passkeys should still start.
type MissingOriginsError struct{}

func (e *MissingOriginsError) Error() string {
	return "no WebAuthn origin could be derived from the base URL or allowed hosts"
}

// Options carries the settings NewConfig needs to build a relying party.
type Options struct {
	RPID   string
	RPName string
	// Origins overrides origin inference entirely when set.
	Origins []string
	// BaseURL is the browser-visible URL of the installation and the primary
	// source for origin inference.
	BaseURL string
	// AllowedHosts is the comma-separated CSRF host list. Entries may be bare
	// hostnames or full origins.
	AllowedHosts string
	// Port is the port the server listens on, used for origins that are not
	// served through a proxy on a standard port.
	Port          string
	IsDevelopment bool
	EnableHTTPS   bool
	UseProxy      bool
}

// NewConfig creates a new WebAuthn configuration
func NewConfig(opts Options) (*Config, error) {
	c := &Config{
		RPID:          opts.RPID,
		RPName:        opts.RPName,
		RPOrigins:     opts.Origins,
		isDevelopment: opts.IsDevelopment,
		Debug:         opts.IsDevelopment,
	}

	// Dev-mode RPID override: production-mode RPID/RPName are pre-resolved by
	// config.Load (env → hostname fallback for RPID, default "Windshift" for
	// RPName), so this package no longer reads env vars itself.
	if c.RPID == "" && c.isDevelopment {
		c.RPID = "localhost"
	}
	if c.RPID == "" {
		return nil, fmt.Errorf("no RP ID provided and not in development mode (config.Load should have resolved this)")
	}
	if c.RPName == "" {
		c.RPName = "Windshift"
	}
	if err := protocol.ValidateRPID(c.RPID); err != nil {
		return nil, &InvalidRPIDError{RPID: c.RPID, Err: err}
	}

	// If no origins provided, derive from configuration
	if len(c.RPOrigins) == 0 {
		port := opts.Port
		if c.isDevelopment {
			// Development mode: Allow both http and https with common ports.
			// Also include the actual port the server is bound to so e2e
			// runners (which pick a random free port) work without extra
			// configuration.
			c.RPOrigins = []string{
				fmt.Sprintf("http://%s", c.RPID),
				fmt.Sprintf("http://%s:8080", c.RPID),
				fmt.Sprintf("http://%s:3000", c.RPID),
				fmt.Sprintf("http://%s:5555", c.RPID), // Vite dev server
				fmt.Sprintf("http://%s:5173", c.RPID), // Vite alternate port
				fmt.Sprintf("https://%s", c.RPID),
				"http://localhost",
				"http://localhost:8080",
				"http://localhost:3000",
				"http://localhost:5555", // Vite dev server
				"http://localhost:5173", // Vite alternate port
				"https://localhost",
			}
			if port != "" {
				c.RPOrigins = append(c.RPOrigins,
					fmt.Sprintf("http://%s:%s", c.RPID, port),
					fmt.Sprintf("http://localhost:%s", port),
				)
			}
		} else {
			origins, err := productionOrigins(opts)
			if err != nil {
				return nil, err
			}
			c.RPOrigins = origins
		}
	}

	// Validate origins
	for _, origin := range c.RPOrigins {
		if _, err := url.Parse(origin); err != nil {
			return nil, fmt.Errorf("invalid origin %s: %w", origin, err)
		}
	}

	// Create WebAuthn config
	wconfig := &webauthn.Config{
		RPDisplayName: c.RPName,
		RPID:          c.RPID,
		RPOrigins:     c.RPOrigins,
		// Set reasonable defaults
		AttestationPreference: protocol.PreferNoAttestation, // Don't require attestation by default
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: &[]bool{false}[0],                        // Don't require resident key
			ResidentKey:        protocol.ResidentKeyRequirementPreferred, // Prefer resident keys for passkeys
			UserVerification:   protocol.VerificationPreferred,           // Prefer user verification
		},
		Debug: c.Debug,
	}

	// Create WebAuthn instance
	wa, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn instance: %w", err)
	}

	c.webAuthn = wa
	return c, nil
}

// productionOrigins collects every browser-visible origin a ceremony may come
// from. The base URL comes first because it is the address operators actually
// hand to browsers, and it is the only one that works behind a TLS-terminating
// proxy where the listen port never reaches the client. Allowed hosts are added
// on top so installations reachable under several names keep working.
func productionOrigins(opts Options) ([]string, error) {
	origins := make([]string, 0, 4)
	if origin, ok := originFromBaseURL(opts.BaseURL); ok {
		origins = append(origins, origin)
	}

	scheme := "http"
	if opts.EnableHTTPS || opts.UseProxy {
		scheme = "https"
	}
	standardPort := "80"
	if scheme == "https" {
		standardPort = "443"
	}

	for _, host := range strings.Split(opts.AllowedHosts, ",") {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if isHTTPURL(host) {
			origin, err := normalizeOrigin(host)
			if err != nil {
				return nil, fmt.Errorf("invalid allowed host origin %q: %w", host, err)
			}
			origins = append(origins, origin)
			continue
		}

		if opts.Port != "" && opts.Port != standardPort {
			origins = append(origins, fmt.Sprintf("%s://%s:%s", scheme, host, opts.Port))
		}
		origins = append(origins, fmt.Sprintf("%s://%s:%s", scheme, host, standardPort))
	}

	origins = dedupeOrigins(origins)
	if len(origins) == 0 {
		return nil, &MissingOriginsError{}
	}
	return origins, nil
}

// originFromBaseURL reduces a base URL to its origin, dropping any context path
// so that an installation served under a subpath still matches what the browser
// reports.
func originFromBaseURL(baseURL string) (string, bool) {
	baseURL = strings.TrimSpace(baseURL)
	if !isHTTPURL(baseURL) {
		return "", false
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

func dedupeOrigins(origins []string) []string {
	seen := make(map[string]struct{}, len(origins))
	unique := make([]string, 0, len(origins))
	for _, origin := range origins {
		key := strings.ToLower(origin)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, origin)
	}
	return unique
}

func isHTTPURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func normalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("must be an http(s) URL without credentials")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("must contain only a scheme, host, and optional port")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// WebAuthn returns the underlying WebAuthn instance
func (c *Config) WebAuthn() *webauthn.WebAuthn {
	return c.webAuthn
}
