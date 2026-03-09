//go:build test

package handlers

import "testing"

func TestIsValidRedirectURI(t *testing.T) {
	tests := []struct {
		name  string
		uri   string
		valid bool
	}{
		// Valid cases
		{"root path", "/", true},
		{"dashboard", "/dashboard", true},
		{"nested path", "/settings/profile", true},
		{"path with query", "/path?q=foo", true},
		{"path with fragment", "/path#section", true},
		{"item path", "/items/123", true},

		// Invalid cases
		{"empty string", "", false},
		{"protocol-relative URL", "//evil.com", false},
		{"absolute URL https", "https://evil.com", false},
		{"backslash bypass", `/\evil.com`, false},
		{"newline injection", "/path\ninjection", false},
		{"tab injection", "/path\tinjection", false},
		{"carriage return injection", "/path\rinjection", false},
		{"userinfo redirect", "/@evil.com", false},
		{"absolute URL http", "http://x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidRedirectURI(tt.uri)
			if got != tt.valid {
				t.Errorf("isValidRedirectURI(%q) = %v, want %v", tt.uri, got, tt.valid)
			}
		})
	}
}
