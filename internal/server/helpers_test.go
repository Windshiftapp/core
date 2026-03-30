package server

import (
	"net/http"
	"net/http/httptest"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := createCORSMiddleware(tt.allowedHosts, tt.serverPort, tt.scheme, tt.disableCSRF)
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
