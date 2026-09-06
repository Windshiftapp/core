package services

import (
	"testing"

	"windshift/internal/models"
)

func TestIsManagedCredentialMetadataRequiresReservedVersionedMarkerAndOwnership(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		managed  bool
	}{
		{name: "provider metadata", metadata: `{"_windshift_managed_credential":"v1","managed_by":"zammad","owner_id":"provider-1"}`, managed: true},
		{name: "managed by only remains generic", metadata: `{"managed_by":"zammad"}`},
		{name: "owner only remains generic", metadata: `{"owner_id":"provider-1"}`},
		{name: "missing marker remains generic", metadata: `{"managed_by":"zammad","owner_id":"provider-1"}`},
		{name: "unknown marker version remains generic", metadata: `{"_windshift_managed_credential":"v2","managed_by":"zammad","owner_id":"provider-1"}`},
		{name: "blank ownership remains generic", metadata: `{"_windshift_managed_credential":"v1","managed_by":" ","owner_id":"provider-1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManagedCredentialMetadata(tt.metadata); got != tt.managed {
				t.Fatalf("isManagedCredentialMetadata(%s) = %v, want %v", tt.metadata, got, tt.managed)
			}
		})
	}
}

func TestCredentialMatchesPurposeRequiresManagedMetadata(t *testing.T) {
	credential := &models.ActionCredential{
		SecretMetadata: `{"_windshift_managed_credential":"v1","managed_by":"zammad","owner_id":"provider-1"}`,
	}
	if !credentialMatchesPurpose(credential, "zammad", "provider-1") {
		t.Fatal("expected matching provider-managed metadata")
	}

	credential.SecretMetadata = `{"managed_by":"zammad","owner_id":"provider-1"}`
	if credentialMatchesPurpose(credential, "zammad", "provider-1") {
		t.Fatal("generic metadata must not match a managed credential purpose")
	}
}
