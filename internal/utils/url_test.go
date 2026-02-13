package utils

import (
	"testing"
)

func TestValidateExternalURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "ValidPublicHTTPS",
			url:     "https://google.com/share/abc123",
			wantErr: false,
		},
		{
			name:    "EmptyURL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "HTTPSchemeRejected",
			url:     "http://docs.example.com/share/abc123",
			wantErr: true,
		},
		{
			name:    "FTPSchemeRejected",
			url:     "ftp://docs.example.com/file",
			wantErr: true,
		},
		{
			name:    "NoScheme",
			url:     "docs.example.com/share/abc123",
			wantErr: true,
		},
		{
			name:    "LoopbackIPv4",
			url:     "https://127.0.0.1/share/abc123",
			wantErr: true,
		},
		{
			name:    "Localhost",
			url:     "https://localhost/share/abc123",
			wantErr: true,
		},
		{
			name:    "PrivateIP10",
			url:     "https://10.0.0.1/share/abc123",
			wantErr: true,
		},
		{
			name:    "PrivateIP172",
			url:     "https://172.16.0.1/share/abc123",
			wantErr: true,
		},
		{
			name:    "PrivateIP192",
			url:     "https://192.168.1.1/share/abc123",
			wantErr: true,
		},
		{
			name:    "AWSMetadata",
			url:     "https://169.254.169.254/latest/meta-data/",
			wantErr: true,
		},
		{
			name:    "IPv6Loopback",
			url:     "https://[::1]/share/abc123",
			wantErr: true,
		},
		{
			name:    "MalformedURL",
			url:     "://not-a-url",
			wantErr: true,
		},
		{
			name:    "NoHostname",
			url:     "https:///path",
			wantErr: true,
		},
		{
			name:    "UnresolvableHost",
			url:     "https://this-host-definitely-does-not-exist-xyz123.example/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExternalURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExternalURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
