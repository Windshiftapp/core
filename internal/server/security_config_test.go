package server

import (
	"testing"
)

func TestResolveSecurityConfig(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		wantErr       bool
		wantProxy     bool
		wantAutoProxy bool
		wantHTTPS     bool
		wantHosts     string
		wantPort      string
		wantDiagCodes []string // expected diagnostic codes (order-independent)
		noDiagCodes   []string // codes that must NOT appear
	}{
		{
			name: "https BASE_URL only - auto-detect proxy",
			cfg: Config{
				BaseURL: "https://example.com",
			},
			wantProxy:     true,
			wantAutoProxy: true,
			wantHosts:     "example.com",
			wantPort:      "443",
			wantDiagCodes: []string{"PROXY_AUTO_DETECTED"},
			noDiagCodes:   []string{"NO_HTTPS"},
		},
		{
			name: "http BASE_URL only - warn no HTTPS",
			cfg: Config{
				BaseURL: "http://localhost:5111",
			},
			wantHosts:     "localhost",
			wantPort:      "5111",
			wantDiagCodes: []string{"NO_HTTPS"},
			noDiagCodes:   []string{"PROXY_AUTO_DETECTED"},
		},
		{
			name: "https BASE_URL with TLS cert - direct HTTPS, no proxy",
			cfg: Config{
				BaseURL:     "https://example.com",
				TLSCertPath: "/path/to/cert.pem",
				TLSKeyPath:  "/path/to/key.pem",
			},
			wantHTTPS:   true,
			wantHosts:   "example.com",
			wantPort:    "443",
			noDiagCodes: []string{"PROXY_AUTO_DETECTED", "NO_HTTPS"},
		},
		{
			name: "explicit USE_PROXY with https BASE_URL - no auto-detect",
			cfg: Config{
				BaseURL:          "https://example.com",
				UseProxy:         true,
				UseProxyExplicit: true,
			},
			wantProxy:     true,
			wantHosts:     "example.com",
			wantPort:      "443",
			wantDiagCodes: []string{"PROXY_EXPLICIT"},
			noDiagCodes:   []string{"PROXY_AUTO_DETECTED", "NO_HTTPS"},
		},
		{
			name:          "no BASE_URL, no AllowedHosts - warning",
			cfg:           Config{},
			wantDiagCodes: []string{"NO_BASE_URL", "NO_HTTPS"},
		},
		{
			name: "ALLOWED_HOSTS override BASE_URL",
			cfg: Config{
				BaseURL:      "https://example.com",
				AllowedHosts: "custom.host.com",
			},
			wantProxy:     true,
			wantAutoProxy: true,
			wantHosts:     "custom.host.com",
			wantPort:      "443",
			wantDiagCodes: []string{"HOSTS_OVERRIDE", "PROXY_AUTO_DETECTED"},
		},
		{
			name: "invalid BASE_URL - error",
			cfg: Config{
				BaseURL: "://broken",
			},
			wantErr: true,
		},
		{
			name:    "BASE_URL without scheme - error",
			cfg:     Config{BaseURL: "//example.com"},
			wantErr: true,
		},
		{
			name: "scheme mismatch - http BASE_URL with TLS",
			cfg: Config{
				BaseURL:     "http://example.com",
				TLSCertPath: "/path/to/cert.pem",
				TLSKeyPath:  "/path/to/key.pem",
			},
			wantHTTPS:     true,
			wantHosts:     "example.com",
			wantPort:      "80",
			wantDiagCodes: []string{"SCHEME_MISMATCH"},
		},
		{
			name: "TLS cert without key - error",
			cfg: Config{
				BaseURL:     "https://example.com",
				TLSCertPath: "/path/to/cert.pem",
			},
			wantErr: true,
		},
		{
			name: "TLS key without cert - error",
			cfg: Config{
				BaseURL:    "https://example.com",
				TLSKeyPath: "/path/to/key.pem",
			},
			wantErr: true,
		},
		{
			name: "additional proxies without proxy mode",
			cfg: Config{
				BaseURL:           "http://localhost:8080",
				AdditionalProxies: "10.0.0.1",
			},
			wantHosts:     "localhost",
			wantPort:      "8080",
			wantDiagCodes: []string{"PROXIES_IGNORED", "NO_HTTPS"},
		},
		{
			name: "BASE_URL with explicit port",
			cfg: Config{
				BaseURL: "https://example.com:8443",
			},
			wantProxy:     true,
			wantAutoProxy: true,
			wantHosts:     "example.com",
			wantPort:      "8443",
		},
		{
			name: "http default port derived",
			cfg: Config{
				BaseURL: "http://example.com",
			},
			wantHosts:     "example.com",
			wantPort:      "80",
			wantDiagCodes: []string{"NO_HTTPS"},
		},
		{
			name: "valid additional proxies with proxy mode",
			cfg: Config{
				BaseURL:           "https://example.com",
				UseProxy:          true,
				UseProxyExplicit:  true,
				AdditionalProxies: "10.0.0.1,192.168.1.1",
			},
			wantProxy: true,
			wantHosts: "example.com",
			wantPort:  "443",
		},
		{
			name: "invalid additional proxy IP",
			cfg: Config{
				BaseURL:           "https://example.com",
				UseProxy:          true,
				UseProxyExplicit:  true,
				AdditionalProxies: "not-an-ip",
			},
			wantProxy:     true,
			wantHosts:     "example.com",
			wantPort:      "443",
			wantDiagCodes: []string{"INVALID_PROXY_IP"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := ResolveSecurityConfig(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resolved.UseProxy != tt.wantProxy {
				t.Errorf("UseProxy = %v, want %v", resolved.UseProxy, tt.wantProxy)
			}
			if resolved.ProxyAutoDetected != tt.wantAutoProxy {
				t.Errorf("ProxyAutoDetected = %v, want %v", resolved.ProxyAutoDetected, tt.wantAutoProxy)
			}
			if resolved.EnableHTTPS != tt.wantHTTPS {
				t.Errorf("EnableHTTPS = %v, want %v", resolved.EnableHTTPS, tt.wantHTTPS)
			}
			if tt.wantHosts != "" && resolved.AllowedHosts != tt.wantHosts {
				t.Errorf("AllowedHosts = %q, want %q", resolved.AllowedHosts, tt.wantHosts)
			}
			if tt.wantPort != "" && resolved.AllowedPort != tt.wantPort {
				t.Errorf("AllowedPort = %q, want %q", resolved.AllowedPort, tt.wantPort)
			}

			diagCodes := make(map[string]bool)
			for _, d := range resolved.Diagnostics {
				diagCodes[d.Code] = true
			}

			for _, code := range tt.wantDiagCodes {
				if !diagCodes[code] {
					t.Errorf("expected diagnostic code %q not found (got: %v)", code, diagCodes)
				}
			}
			for _, code := range tt.noDiagCodes {
				if diagCodes[code] {
					t.Errorf("unexpected diagnostic code %q found", code)
				}
			}
		})
	}
}
