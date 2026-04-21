package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCreateCORSMiddleware_Origins(t *testing.T) {
	dummy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name         string
		allowedHosts string
		serverPort   string
		scheme       string
		disableCSRF  bool
		useProxy     bool
		origin       string
		wantAllowed  bool
	}{
		{
			name:         "http scheme with non-default port",
			allowedHosts: "localhost",
			serverPort:   "7776",
			scheme:       "http",
			origin:       "http://localhost:7776",
			wantAllowed:  true,
		},
		{
			name:         "https with default port omitted",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			origin:       "https://example.com",
			wantAllowed:  true,
		},
		{
			name:         "http with default port omitted",
			allowedHosts: "localhost",
			serverPort:   "80",
			scheme:       "http",
			origin:       "http://localhost",
			wantAllowed:  true,
		},
		{
			name:         "empty scheme defaults to https",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "",
			origin:       "https://example.com",
			wantAllowed:  true,
		},
		{
			name:         "https with non-default port",
			allowedHosts: "example.com",
			serverPort:   "8443",
			scheme:       "https",
			origin:       "https://example.com:8443",
			wantAllowed:  true,
		},
		{
			name:         "wrong origin rejected",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			origin:       "https://evil.com",
			wantAllowed:  false,
		},
		{
			name:         "full URL in allowedHosts passed through",
			allowedHosts: "http://localhost:3000",
			serverPort:   "8080",
			scheme:       "http",
			origin:       "http://localhost:3000",
			wantAllowed:  true,
		},
		{
			name:         "useProxy accepts http when configured https",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			useProxy:     true,
			origin:       "http://example.com",
			wantAllowed:  true,
		},
		{
			name:         "useProxy still accepts configured https",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			useProxy:     true,
			origin:       "https://example.com",
			wantAllowed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := createCORSMiddleware(tt.allowedHosts, tt.serverPort, tt.scheme, tt.disableCSRF, tt.useProxy)
			handler := mw(dummy)

			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			acao := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantAllowed && acao == "" {
				t.Errorf("expected origin %q to be allowed, but Access-Control-Allow-Origin is empty", tt.origin)
			}
			if !tt.wantAllowed && acao != "" {
				t.Errorf("expected origin %q to be rejected, but got Access-Control-Allow-Origin=%q", tt.origin, acao)
			}
		})
	}
}

func TestBuildAllowedOrigins(t *testing.T) {
	tests := []struct {
		name         string
		allowedHosts string
		serverPort   string
		scheme       string
		useProxy     bool
		want         []string
	}{
		{
			name:         "empty hosts returns nil",
			allowedHosts: "",
			serverPort:   "443",
			scheme:       "https",
			want:         nil,
		},
		{
			name:         "single host with https default port",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			want:         []string{"https://example.com"},
		},
		{
			name:         "single host with non-default port",
			allowedHosts: "localhost",
			serverPort:   "7776",
			scheme:       "http",
			want:         []string{"http://localhost:7776"},
		},
		{
			name:         "multiple hosts",
			allowedHosts: "example.com,other.com",
			serverPort:   "443",
			scheme:       "https",
			want:         []string{"https://example.com", "https://other.com"},
		},
		{
			name:         "full URL passed through",
			allowedHosts: "http://localhost:3000",
			serverPort:   "8080",
			scheme:       "http",
			want:         []string{"http://localhost:3000"},
		},
		{
			name:         "empty scheme defaults to https",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "",
			want:         []string{"https://example.com"},
		},
		{
			name:         "empty entries skipped",
			allowedHosts: "example.com,,other.com",
			serverPort:   "443",
			scheme:       "https",
			want:         []string{"https://example.com", "https://other.com"},
		},
		{
			name:         "whitespace trimmed",
			allowedHosts: " example.com , other.com ",
			serverPort:   "443",
			scheme:       "https",
			want:         []string{"https://example.com", "https://other.com"},
		},
		{
			name:         "useProxy expands https to include http variant",
			allowedHosts: "example.com",
			serverPort:   "443",
			scheme:       "https",
			useProxy:     true,
			want:         []string{"https://example.com", "http://example.com"},
		},
		{
			name:         "useProxy expands http to include https variant",
			allowedHosts: "example.com",
			serverPort:   "80",
			scheme:       "http",
			useProxy:     true,
			want:         []string{"http://example.com", "https://example.com"},
		},
		{
			name:         "useProxy expands multiple hosts",
			allowedHosts: "example.com,other.com",
			serverPort:   "443",
			scheme:       "https",
			useProxy:     true,
			want: []string{
				"https://example.com", "http://example.com",
				"https://other.com", "http://other.com",
			},
		},
		{
			name:         "useProxy does not expand explicit-scheme host",
			allowedHosts: "http://localhost:3000",
			serverPort:   "8080",
			scheme:       "http",
			useProxy:     true,
			want:         []string{"http://localhost:3000"},
		},
		{
			name:         "useProxy with non-default port drops port in opposite variant",
			allowedHosts: "example.com",
			serverPort:   "8443",
			scheme:       "https",
			useProxy:     true,
			want:         []string{"https://example.com:8443", "http://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAllowedOrigins(tt.allowedHosts, tt.serverPort, tt.scheme, tt.useProxy)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildAllowedOrigins(%q, %q, %q, %v) = %v, want %v",
					tt.allowedHosts, tt.serverPort, tt.scheme, tt.useProxy, got, tt.want)
			}
		})
	}
}

func TestIsDefaultPort(t *testing.T) {
	tests := []struct {
		scheme string
		port   string
		want   bool
	}{
		{"https", "443", true},
		{"http", "80", true},
		{"https", "8443", false},
		{"http", "8080", false},
		{"https", "80", false},
		{"http", "443", false},
	}
	for _, tt := range tests {
		t.Run(tt.scheme+":"+tt.port, func(t *testing.T) {
			if got := isDefaultPort(tt.scheme, tt.port); got != tt.want {
				t.Errorf("isDefaultPort(%q, %q) = %v, want %v", tt.scheme, tt.port, got, tt.want)
			}
		})
	}
}
