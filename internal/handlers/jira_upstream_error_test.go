package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"windshift/internal/jira"
)

// TestRespondJiraUpstreamError pins the code/status mapping the wizard
// frontend branches on. JIRA_AUTH_FAILED in particular must be a 502 (not
// 401), because the frontend's fetchAPI auto-clears auth on any 401 — turning
// a revoked Jira token into a Windshift logout, which is the booby-trap this
// helper was added to avoid.
func TestRespondJiraUpstreamError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{
			name:     "invalid credentials maps to JIRA_AUTH_FAILED",
			err:      fmt.Errorf("wrap: %w", jira.ErrInvalidCredentials),
			wantCode: "JIRA_AUTH_FAILED",
		},
		{
			name:     "forbidden maps to JIRA_FORBIDDEN",
			err:      jira.ErrForbidden,
			wantCode: "JIRA_FORBIDDEN",
		},
		{
			name:     "rate limited maps to JIRA_RATE_LIMITED",
			err:      jira.ErrRateLimited,
			wantCode: "JIRA_RATE_LIMITED",
		},
		{
			name:     "generic Jira error maps to JIRA_UPSTREAM_ERROR",
			err:      errors.New("some other Jira-side problem"),
			wantCode: "JIRA_UPSTREAM_ERROR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/admin/jira-import/projects", nil)
			respondJiraUpstreamError(rec, r, tc.err)

			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadGateway)
			}

			var body struct {
				Code    string `json:"code"`
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
			}
			if body.Code != tc.wantCode {
				t.Fatalf("code: got %q, want %q (body=%s)", body.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}
