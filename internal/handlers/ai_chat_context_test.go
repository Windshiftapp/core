package handlers

import (
	"strings"
	"testing"
)

// buildChatContextHint is pure: it deterministically maps an optional
// ChatContext into the per-request system-prompt suffix. We test each of
// the three branches the chat handler relies on, plus the defensive cases
// (nil pointer, mismatched view, missing workspace_id).
func TestBuildChatContextHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		ctx         *ChatContext
		wantEmpty   bool
		wantSubstrs []string
	}{
		{
			name:      "nil context — no hint",
			ctx:       nil,
			wantEmpty: true,
		},
		{
			name:      "unknown view — no hint",
			ctx:       &ChatContext{View: "homepage", WorkspaceID: 7},
			wantEmpty: true,
		},
		{
			name:      "workspace-actions view without workspace_id — no hint",
			ctx:       &ChatContext{View: "workspace-actions"},
			wantEmpty: true,
		},
		{
			name: "workspace-actions list — list-page hint",
			ctx:  &ChatContext{View: "workspace-actions", WorkspaceID: 7},
			wantSubstrs: []string{
				"action settings page",
				"workspace 7",
				"describe_action_catalog",
				"create_action",
			},
		},
		{
			name: "workspace-actions editor — editor hint",
			ctx:  &ChatContext{View: "workspace-actions", WorkspaceID: 7, ActionID: 42},
			wantSubstrs: []string{
				"editing action 42",
				"workspace 7",
				"update_action",
				"live-reloads",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildChatContextHint(tc.ctx)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty hint, got %q", got)
				}
				return
			}
			for _, sub := range tc.wantSubstrs {
				if !strings.Contains(got, sub) {
					t.Errorf("expected hint to contain %q, got: %s", sub, got)
				}
			}
		})
	}
}

// Editor hint must not be emitted when ActionID is set but View is wrong —
// guards against an LLM-supplied context fooling the prompt builder.
func TestBuildChatContextHintIgnoresActionIDOnWrongView(t *testing.T) {
	t.Parallel()
	got := buildChatContextHint(&ChatContext{View: "item-detail", WorkspaceID: 7, ActionID: 42})
	if got != "" {
		t.Fatalf("expected no hint for non-actions view, got: %s", got)
	}
}
