package server

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
)

// DiagnosticLevel indicates the severity of a security diagnostic.
type DiagnosticLevel int

const (
	DiagInfo DiagnosticLevel = iota
	DiagWarn
	DiagError
)

// SecurityDiagnostic represents a single diagnostic finding from security config resolution.
type SecurityDiagnostic struct {
	Level   DiagnosticLevel
	Code    string
	Message string
	Hint    string
}

// ResolvedSecurityConfig holds the computed security configuration derived from Config.
type ResolvedSecurityConfig struct {
	EnableHTTPS       bool
	UseProxy          bool
	ProxyAutoDetected bool
	AllowedHosts      string
	AllowedPort       string
	Scheme            string
	BaseURL           string
	AdditionalProxies []net.IP
	Diagnostics       []SecurityDiagnostic
}

// ResolveSecurityConfig analyzes the provided Config and returns a fully resolved
// security configuration with diagnostics. Errors in Diagnostics with level DiagError
// cause the function to return an error (server should not start).
func ResolveSecurityConfig(cfg Config) (*ResolvedSecurityConfig, error) {
	r := &ResolvedSecurityConfig{
		AllowedHosts: cfg.AllowedHosts,
		AllowedPort:  cfg.AllowedPort,
		BaseURL:      cfg.BaseURL,
	}

	// 1. TLS configuration
	r.EnableHTTPS = cfg.TLSCertPath != "" && cfg.TLSKeyPath != ""

	// Validate TLS pair completeness
	if (cfg.TLSCertPath != "") != (cfg.TLSKeyPath != "") {
		r.addDiag(DiagError, "TLS_INCOMPLETE",
			"TLS configuration is incomplete: both --tls-cert and --tls-key must be provided together.",
			"Provide both TLS certificate and key, or remove both to disable direct TLS.")
		return r, fmt.Errorf("TLS configuration incomplete: provide both --tls-cert and --tls-key, or neither")
	}

	// 2. Parse BASE_URL
	var parsedURL *url.URL
	if cfg.BaseURL != "" {
		var err error
		parsedURL, err = url.Parse(cfg.BaseURL)
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			r.addDiag(DiagError, "INVALID_BASE_URL",
				fmt.Sprintf("BASE_URL %q could not be parsed as a valid URL.", cfg.BaseURL),
				"Use a full URL like https://myapp.example.com or http://localhost:8080")
			return r, fmt.Errorf("invalid BASE_URL %q: must be a valid URL with scheme and host", cfg.BaseURL)
		}
		r.Scheme = parsedURL.Scheme
	}

	// 3. Derive AllowedHosts from BASE_URL if not explicitly set
	hostsExplicit := cfg.AllowedHosts != ""
	if !hostsExplicit && parsedURL != nil {
		r.AllowedHosts = parsedURL.Hostname()
	}
	if hostsExplicit && parsedURL != nil {
		r.addDiag(DiagInfo, "HOSTS_OVERRIDE",
			fmt.Sprintf("ALLOWED_HOSTS is set explicitly (%s), overriding value derived from BASE_URL.", cfg.AllowedHosts),
			"Remove ALLOWED_HOSTS to let it be derived from BASE_URL automatically.")
	}

	// 4. Derive AllowedPort from BASE_URL if not explicitly set
	if r.AllowedPort == "" && parsedURL != nil {
		switch {
		case parsedURL.Port() != "":
			r.AllowedPort = parsedURL.Port()
		case parsedURL.Scheme == "https":
			r.AllowedPort = "443"
		default:
			r.AllowedPort = "80"
		}
	}

	// 5. Auto-detect proxy mode
	r.UseProxy = cfg.UseProxy
	if cfg.UseProxyExplicit {
		r.addDiag(DiagInfo, "PROXY_EXPLICIT",
			"Proxy mode explicitly enabled via USE_PROXY.",
			"Ensure this server is NOT directly accessible from the internet.")
	} else if parsedURL != nil && parsedURL.Scheme == "https" && !r.EnableHTTPS {
		// BASE_URL is https but no TLS cert → auto-detect proxy
		r.UseProxy = true
		r.ProxyAutoDetected = true
		r.addDiag(DiagWarn, "PROXY_AUTO_DETECTED",
			"Proxy mode auto-detected: BASE_URL uses https:// but no TLS certificate is configured. Assuming a reverse proxy handles TLS termination.",
			"Ensure this server is NOT directly accessible from the internet — it must only be reachable through the reverse proxy. Set USE_PROXY=true to silence this warning.")
	}

	// 6. No BASE_URL and no AllowedHosts
	if cfg.BaseURL == "" && cfg.AllowedHosts == "" {
		r.addDiag(DiagWarn, "NO_BASE_URL",
			"No BASE_URL or ALLOWED_HOSTS configured. CORS will reject all cross-origin requests.",
			"Set BASE_URL to your server's public URL (e.g. BASE_URL=https://myapp.example.com).")
	}

	// 7. Scheme mismatch warnings
	if parsedURL != nil && parsedURL.Scheme == "http" && r.EnableHTTPS {
		r.addDiag(DiagWarn, "SCHEME_MISMATCH",
			"BASE_URL uses http:// but TLS certificates are configured. CORS origins will use http:// which may not match actual requests.",
			"Update BASE_URL to https:// to match your TLS configuration.")
	}

	// 8. No HTTPS at all
	if !r.EnableHTTPS && !r.UseProxy {
		r.addDiag(DiagWarn, "NO_HTTPS",
			"Running without HTTPS — credentials will be transmitted in plaintext.",
			"For production, either configure TLS (--tls-cert, --tls-key) or run behind a reverse proxy (USE_PROXY=true) with TLS termination.")
	}

	// 9. Additional proxies without proxy mode
	if cfg.AdditionalProxies != "" && !r.UseProxy {
		r.addDiag(DiagWarn, "PROXIES_IGNORED",
			"ADDITIONAL_PROXIES is set but proxy mode is not enabled. These IPs will be ignored.",
			"Enable proxy mode with USE_PROXY=true or set BASE_URL to an https:// URL.")
	}

	// 10. Parse additional proxies
	if cfg.AdditionalProxies != "" && r.UseProxy {
		parts := strings.Split(cfg.AdditionalProxies, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			ip := net.ParseIP(part)
			if ip == nil {
				r.addDiag(DiagWarn, "INVALID_PROXY_IP",
					fmt.Sprintf("Could not parse additional proxy IP %q — it will be ignored.", part),
					"Use valid IPv4 or IPv6 addresses (e.g. 10.0.0.1, ::1).")
				continue
			}
			r.AdditionalProxies = append(r.AdditionalProxies, ip)
		}
	}

	// Check for any error-level diagnostics
	for _, d := range r.Diagnostics {
		if d.Level == DiagError {
			return r, fmt.Errorf("security configuration error: %s", d.Message)
		}
	}

	return r, nil
}

// LogDiagnostics logs all diagnostics at appropriate slog levels.
func (r *ResolvedSecurityConfig) LogDiagnostics() {
	for _, d := range r.Diagnostics {
		attrs := []any{
			"code", d.Code,
			"hint", d.Hint,
		}
		switch d.Level {
		case DiagError:
			slog.Error(d.Message, attrs...)
		case DiagWarn:
			slog.Warn(d.Message, attrs...)
		case DiagInfo:
			slog.Info(d.Message, attrs...)
		}
	}
}

func (r *ResolvedSecurityConfig) addDiag(level DiagnosticLevel, code, message, hint string) {
	r.Diagnostics = append(r.Diagnostics, SecurityDiagnostic{
		Level:   level,
		Code:    code,
		Message: message,
		Hint:    hint,
	})
}
