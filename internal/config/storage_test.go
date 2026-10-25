package config

import (
	"strings"
	"testing"
)

func TestResolveStorageConfig(t *testing.T) {
	tests := []struct {
		name      string
		env       StorageEnv
		wantBack  string
		wantErr   string
		wantCheck func(*testing.T, StorageConfig)
	}{
		{
			name:     "default is local",
			env:      StorageEnv{},
			wantBack: StorageBackendLocal,
		},
		{
			name:     "explicit local",
			env:      StorageEnv{Backend: "local"},
			wantBack: StorageBackendLocal,
		},
		{
			name:    "unknown backend",
			env:     StorageEnv{Backend: "nfs"},
			wantErr: "STORAGE_BACKEND must be",
		},
		{
			name:    "s3 without bucket",
			env:     StorageEnv{Backend: "s3"},
			wantErr: "requires S3_BUCKET",
		},
		{
			name:    "half credentials",
			env:     StorageEnv{Backend: "s3", S3Bucket: "b", S3AccessKeyID: "id"},
			wantErr: "must be set together",
		},
		{
			name:     "s3 defaults region and trims prefix",
			env:      StorageEnv{Backend: "s3", S3Bucket: "b", S3Prefix: "/attachments/"},
			wantBack: StorageBackendS3,
			wantCheck: func(t *testing.T, cfg StorageConfig) {
				t.Helper()
				if cfg.S3.Region != "us-east-1" {
					t.Fatalf("region = %q, want us-east-1", cfg.S3.Region)
				}
				if cfg.S3.Prefix != "attachments" {
					t.Fatalf("prefix = %q, want attachments", cfg.S3.Prefix)
				}
			},
		},
		{
			name:     "s3 with credentials",
			env:      StorageEnv{Backend: "s3", S3Bucket: "b", S3Region: "eu-west-1", S3AccessKeyID: "id", S3SecretKey: "secret", S3ForcePathStyle: true},
			wantBack: StorageBackendS3,
			wantCheck: func(t *testing.T, cfg StorageConfig) {
				t.Helper()
				if cfg.S3.Region != "eu-west-1" || !cfg.S3.ForcePathStyle {
					t.Fatalf("unexpected s3 config: %+v", cfg.S3)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStorageConfig("/data/attachments", tc.env)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveStorageConfig: %v", err)
			}
			if got.Backend != tc.wantBack {
				t.Fatalf("backend = %q, want %q", got.Backend, tc.wantBack)
			}
			if got.LocalPath != "/data/attachments" {
				t.Fatalf("local path = %q, want /data/attachments", got.LocalPath)
			}
			if tc.wantCheck != nil {
				tc.wantCheck(t, got)
			}
		})
	}
}
