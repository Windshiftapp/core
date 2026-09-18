package models

import "testing"

func TestFormatRequirementKeyUsesLiveWorkspacePrefix(t *testing.T) {
	if got := FormatRequirementKey("CRM", 42); got != "CRM-DOC-42" {
		t.Fatalf("got %q", got)
	}
	if got := FormatRequirementKey("SALES", 42); got != "SALES-DOC-42" {
		t.Fatalf("rename should follow live workspace key, got %q", got)
	}
}
