package auth_test

import (
	"testing"

	"windshift/internal/auth"
	"windshift/internal/models"
)

func TestCheckTokenPermissions(t *testing.T) {
	tm := &auth.TokenManager{}

	tests := []struct {
		name        string
		permissions string
		required    []string
		want        bool
	}{
		{"exact granular match", `["items:write"]`, []string{"items:write"}, true},
		{"missing scope rejected", `["items:read"]`, []string{"items:write"}, false},
		{"write satisfies read for same resource", `["items:write"]`, []string{"items:read"}, true},
		{"read does not satisfy write", `["items:read"]`, []string{"items:write"}, false},
		{"multi-scope all required present", `["items:read","workspaces:read"]`, []string{"items:read", "workspaces:read"}, true},
		{"multi-scope one missing fails", `["items:read"]`, []string{"items:read", "workspaces:read"}, false},
		{"legacy read covers items:read", `["read"]`, []string{"items:read"}, true},
		{"legacy read does not cover items:write", `["read"]`, []string{"items:write"}, false},
		{"legacy write covers items:write", `["write"]`, []string{"items:write"}, true},
		{"legacy write does not cover admin scope", `["write"]`, []string{"admin:users:write"}, false},
		{"legacy admin covers admin scope", `["admin"]`, []string{"admin:users:write"}, true},
		{"legacy admin covers granular non-admin scope", `["admin"]`, []string{"items:write"}, true},
		{"malformed JSON returns false", `not-json`, []string{"items:read"}, false},
		{"empty required returns true", `["items:read"]`, []string{}, true},
		{"empty token scopes rejects any required", `[]`, []string{"items:read"}, false},
		{"mcp:access granular match", `["mcp:access"]`, []string{"mcp:access"}, true},
		{"legacy write covers mcp:access", `["write"]`, []string{"mcp:access"}, true},
		{"legacy read does NOT cover mcp:access", `["read"]`, []string{"mcp:access"}, false},
		{"legacy admin covers mcp:access", `["admin"]`, []string{"mcp:access"}, true},
		{"items:write does not satisfy mcp:access", `["items:write"]`, []string{"mcp:access"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &models.APIToken{Permissions: tt.permissions}
			got := tm.CheckTokenPermissions(token, tt.required)
			if got != tt.want {
				t.Errorf("CheckTokenPermissions(scopes=%q, required=%v) = %v, want %v",
					tt.permissions, tt.required, got, tt.want)
			}
		})
	}
}
