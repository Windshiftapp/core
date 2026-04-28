package webhook

import (
	"strings"
	"testing"
)

func TestValidateWebhookURL(t *testing.T) {
	// Use literal public IPs for the success cases so the test doesn't need
	// network DNS resolution to pass.
	cases := []struct {
		url     string
		wantErr string // substring; empty means must succeed
	}{
		{"https://1.1.1.1/webhook", ""},
		{"http://8.8.8.8/webhook", ""},
		{"ftp://hooks.example.com/x", "scheme"},
		{"https://localhost/x", "localhost"},
		{"https://127.0.0.1/x", "private IP"},
		{"https://10.0.0.1/x", "private IP"},
		{"https://192.168.1.5/x", "private IP"},
		{"https://169.254.169.254/latest/meta-data/", "private IP"},
		{"://broken", "invalid"},
		{"https:///nohost", "host"},
	}

	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			err := ValidateWebhookURL(tc.url)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateWebhookURL(%q) = %v, want nil", tc.url, err)
				}
				return
			}
			if err == nil {
				t.Errorf("ValidateWebhookURL(%q) = nil, want error containing %q", tc.url, tc.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ValidateWebhookURL(%q) = %v, want substring %q", tc.url, err, tc.wantErr)
			}
		})
	}
}
