package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFProtection(t *testing.T) {
	allowedOrigins := []string{"https://example.com", "http://localhost:7776"}

	tests := []struct {
		name           string
		method         string
		secFetchSite   string // "" means header is absent; use "ABSENT" sentinel handled below
		origin         string
		referer        string
		csrfExempt     bool
		allowedOrigins []string
		wantStatus     int
	}{
		// Safe methods always pass
		{
			name:       "GET always passes",
			method:     "GET",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HEAD always passes",
			method:     "HEAD",
			wantStatus: http.StatusOK,
		},
		{
			name:       "OPTIONS always passes",
			method:     "OPTIONS",
			wantStatus: http.StatusOK,
		},

		// CSRF-exempt requests pass
		{
			name:       "exempt request passes",
			method:     "POST",
			csrfExempt: true,
			wantStatus: http.StatusOK,
		},

		// Valid Sec-Fetch-Site
		{
			name:         "same-origin passes",
			method:       "POST",
			secFetchSite: "same-origin",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "none passes",
			method:       "POST",
			secFetchSite: "none",
			wantStatus:   http.StatusOK,
		},

		// Invalid Sec-Fetch-Site blocks even with valid Origin
		{
			name:         "cross-site blocks even with valid Origin",
			method:       "POST",
			secFetchSite: "cross-site",
			origin:       "https://example.com",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "same-site blocks",
			method:       "POST",
			secFetchSite: "same-site",
			wantStatus:   http.StatusForbidden,
		},

		// Missing Sec-Fetch-Site — Origin fallback
		{
			name:       "missing header + valid Origin passes",
			method:     "POST",
			origin:     "https://example.com",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing header + invalid Origin blocks",
			method:     "POST",
			origin:     "https://evil.com",
			wantStatus: http.StatusForbidden,
		},

		// Missing Sec-Fetch-Site — Referer fallback
		{
			name:       "missing header + no Origin + valid Referer passes",
			method:     "POST",
			referer:    "https://example.com/some/page",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing header + no Origin + invalid Referer blocks",
			method:     "POST",
			referer:    "https://evil.com/some/page",
			wantStatus: http.StatusForbidden,
		},

		// All headers missing
		{
			name:       "all headers missing blocks",
			method:     "POST",
			wantStatus: http.StatusForbidden,
		},

		// Case insensitivity
		{
			name:       "case insensitive Origin comparison",
			method:     "POST",
			origin:     "HTTPS://EXAMPLE.COM",
			wantStatus: http.StatusOK,
		},

		// Empty allowed origins
		{
			name:           "empty allowed origins blocks",
			method:         "POST",
			origin:         "https://example.com",
			allowedOrigins: []string{},
			wantStatus:     http.StatusForbidden,
		},

		// Malformed Referer
		{
			name:       "malformed Referer blocks",
			method:     "POST",
			referer:    "://not-a-url",
			wantStatus: http.StatusForbidden,
		},

		// Origin: null
		{
			name:       "Origin null blocks",
			method:     "POST",
			origin:     "null",
			wantStatus: http.StatusForbidden,
		},

		// Port-specific origin
		{
			name:       "port-specific origin passes",
			method:     "POST",
			origin:     "http://localhost:7776",
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong port blocks",
			method:     "POST",
			origin:     "http://localhost:9999",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := allowedOrigins
			if tt.allowedOrigins != nil {
				origins = tt.allowedOrigins
			}

			handler := CSRFProtection(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/api/test", nil)

			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			if tt.csrfExempt {
				ctx := context.WithValue(req.Context(), ContextKeyCSRFExempt, true)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
