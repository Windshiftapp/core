package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantOK  bool
	}{
		{"bare cloud host", "https://acme.atlassian.net", "https://acme.atlassian.net", true},
		{"trailing slash stripped", "https://acme.atlassian.net/", "https://acme.atlassian.net", true},
		{"path stripped", "https://acme.atlassian.net/jira/projects", "https://acme.atlassian.net", true},
		{"query and fragment stripped", "https://acme.atlassian.net/jira?x=1#y", "https://acme.atlassian.net", true},
		{"default https port dropped", "https://acme.atlassian.net:443", "https://acme.atlassian.net", true},
		{"default http port dropped", "http://jira.local:80", "http://jira.local", true},
		{"non-default port kept", "http://jira.local:8080", "http://jira.local:8080", true},
		{"datacenter https with port", "https://jira.example.com:8443/jira", "https://jira.example.com:8443", true},
		{"scheme lowercased", "HTTPS://Acme.Atlassian.NET", "https://acme.atlassian.net", true},
		{"whitespace trimmed", "  https://acme.atlassian.net  ", "https://acme.atlassian.net", true},
		{"empty rejected", "", "", false},
		{"malformed rejected", "::::not a url", "", false},
		{"ftp scheme rejected", "ftp://acme.atlassian.net", "", false},
		{"scheme-only rejected", "https://", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeOrigin(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("normalizeOrigin(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("normalizeOrigin(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCreateSecurityHeaders_ImgSrcIncludesJiraOrigins(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	origins := func() []string {
		return []string{"https://acme.atlassian.net", "https://jira.example.com:8443"}
	}
	handler := createSecurityHeaders(false, false, nil, origins)(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	want := "img-src 'self' data: blob: https://images.unsplash.com https://acme.atlassian.net https://jira.example.com:8443;"
	if !strings.Contains(csp, want) {
		t.Errorf("CSP missing expected img-src directive.\n got: %q\nwant substring: %q", csp, want)
	}
}

func TestCreateSecurityHeaders_NilProviderKeepsBaseImgSrc(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := createSecurityHeaders(false, false, nil, nil)(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	want := "img-src 'self' data: blob: https://images.unsplash.com;"
	if !strings.Contains(csp, want) {
		t.Errorf("CSP missing base img-src directive.\n got: %q\nwant substring: %q", csp, want)
	}
}

func TestCreateSecurityHeaders_EmptyProviderKeepsBaseImgSrc(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := createSecurityHeaders(false, false, nil, func() []string { return nil })(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	want := "img-src 'self' data: blob: https://images.unsplash.com;"
	if !strings.Contains(csp, want) {
		t.Errorf("CSP missing base img-src directive.\n got: %q\nwant substring: %q", csp, want)
	}
}
