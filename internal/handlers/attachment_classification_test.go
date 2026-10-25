package handlers

import (
	"path/filepath"
	"testing"

	"windshift/internal/blobstore"
)

func TestClassifyAttachment(t *testing.T) {
	tests := []struct {
		name            string
		entityType      string
		category        string
		wantEntity      string
		wantCollection  string
		wantScoped      bool
		wantDomain      bool
		wantImageOnly   bool
		wantRequiresID  bool
		wantPublicAsset bool
		wantURLKey      string
		wantErr         bool
	}{
		{name: "empty defaults to item", wantEntity: "item", wantCollection: "items", wantScoped: true, wantDomain: true, wantRequiresID: true},
		{name: "category alias avatar", category: "avatar", wantEntity: "avatar", wantCollection: "avatars", wantImageOnly: true, wantURLKey: "avatar_url"},
		{name: "category alias workspace avatar", category: "workspace_avatar", wantEntity: "workspace_avatar", wantCollection: "workspace_avatars", wantImageOnly: true, wantRequiresID: true, wantURLKey: "avatar_url"},
		{name: "category alias portal logo", category: "portal_logo", wantEntity: "portal_logo", wantCollection: "portal_logos", wantImageOnly: true, wantRequiresID: true, wantPublicAsset: true, wantURLKey: "logo_url"},
		{name: "unknown category defaults to item", category: "not-a-type", wantEntity: "item", wantCollection: "items", wantScoped: true, wantDomain: true, wantRequiresID: true},
		{name: "item", entityType: "item", wantEntity: "item", wantCollection: "items", wantScoped: true, wantDomain: true, wantRequiresID: true},
		{name: "page", entityType: "page", wantEntity: "page", wantCollection: "pages", wantScoped: true, wantDomain: true, wantRequiresID: true},
		{name: "test case", entityType: "test_case", wantEntity: "test_case", wantCollection: "test_cases", wantScoped: true, wantRequiresID: true},
		{name: "test result", entityType: "test_result", wantEntity: "test_result", wantCollection: "test_results", wantScoped: true, wantRequiresID: true},
		{name: "team avatar", entityType: "team_avatar", wantEntity: "team_avatar", wantCollection: "team_avatars", wantImageOnly: true, wantRequiresID: true, wantURLKey: "avatar_url"},
		{name: "customer avatar", entityType: "customer_avatar", wantEntity: "customer_avatar", wantCollection: "customer_avatars", wantImageOnly: true, wantRequiresID: true, wantURLKey: "avatar_url"},
		{name: "workspace background", entityType: "workspace_background", wantEntity: "workspace_background", wantCollection: "workspace_backgrounds", wantImageOnly: true, wantRequiresID: true, wantURLKey: "background_url"},
		{name: "portal background", entityType: "portal_background", wantEntity: "portal_background", wantCollection: "portal_backgrounds", wantImageOnly: true, wantRequiresID: true, wantPublicAsset: true, wantURLKey: "background_url"},
		{name: "hub logo", entityType: "hub_logo", wantEntity: "hub_logo", wantCollection: "hub_logos", wantImageOnly: true, wantPublicAsset: true, wantURLKey: "logo_url"},
		{name: "theme logo", entityType: "theme_logo", wantEntity: "theme_logo", wantCollection: "theme_logos", wantImageOnly: true, wantPublicAsset: true, wantURLKey: "logo_url"},
		{name: "unknown entity type", entityType: "not-a-type", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, err := classifyAttachment(tc.entityType, tc.category)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error for an unknown entity type")
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyAttachment: %v", err)
			}
			if kind.entityType != tc.wantEntity || kind.collection != tc.wantCollection {
				t.Fatalf("kind = %+v, want entity=%q collection=%q", kind, tc.wantEntity, tc.wantCollection)
			}
			if kind.scopedByEntity != tc.wantScoped || kind.domain != tc.wantDomain ||
				kind.imageOnly != tc.wantImageOnly || kind.requiresEntityID != tc.wantRequiresID ||
				kind.publicAsset != tc.wantPublicAsset || kind.urlKey != tc.wantURLKey {
				t.Fatalf("kind = %+v, want scoped=%v domain=%v image=%v requiresID=%v public=%v urlKey=%q",
					kind, tc.wantScoped, tc.wantDomain, tc.wantImageOnly, tc.wantRequiresID, tc.wantPublicAsset, tc.wantURLKey)
			}
		})
	}
}

func TestKindPath(t *testing.T) {
	root := t.TempDir()
	h := &AttachmentHandler{storage: blobstore.NewLocal(root)}

	avatar, _ := classifyAttachment("avatar", "")
	if got, want := h.kindPath(avatar, 0, "a.png"), filepath.Join(root, "avatars", "a.png"); got != want {
		t.Fatalf("avatar path = %q, want %q", got, want)
	}

	testCase, _ := classifyAttachment("test_case", "")
	if got, want := h.kindPath(testCase, 7, "b.txt"), filepath.Join(root, "test_cases", "7", "b.txt"); got != want {
		t.Fatalf("test_case path = %q, want %q", got, want)
	}

	item, _ := classifyAttachment("", "")
	if got, want := h.kindPath(item, 9, "c.txt"), filepath.Join(root, "items", "9", "c.txt"); got != want {
		t.Fatalf("item path = %q, want %q", got, want)
	}
}
