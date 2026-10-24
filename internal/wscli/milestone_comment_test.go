package wscli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordedRequest struct {
	Method      string
	Path        string
	Query       string
	ContentType string
	Auth        string
	Body        string
}

// milestoneCommentServer fakes the v2 milestone comment routes and records
// every request it receives.
func milestoneCommentServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, func() []recordedRequest) {
	t.Helper()
	var (
		mu       sync.Mutex
		requests []recordedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			ContentType: r.Header.Get("Content-Type"),
			Auth:        r.Header.Get("Authorization"),
			Body:        string(body),
		})
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), requests...)
	}
}

func runWS(t *testing.T, serverURL string, stdinBody string, args ...string) (code int, stdoutText, stderrText string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, strings.NewReader(stdinBody), &out, &errOut, map[string]string{
		"WS_URL":   serverURL,
		"WS_TOKEN": "test-token",
	})
	return code, out.String(), errOut.String()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func milestoneCommentJSON(id int, content string) map[string]any {
	return map[string]any{
		"id":           id,
		"milestone_id": 5,
		"author_id":    1,
		"author_name":  "Korneel",
		"content":      content,
		"content_html": "<p>" + content + "</p>",
		"created_at":   fmt.Sprintf("2026-10-0%dT10:00:00Z", id),
		"updated_at":   fmt.Sprintf("2026-10-0%dT10:00:00Z", id),
	}
}

func TestMilestoneCommentListFollowsCursorNewestFirst(t *testing.T) {
	server, requests := milestoneCommentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
				"comments":    []any{milestoneCommentJSON(3, "third"), milestoneCommentJSON(2, "second")},
				"next_cursor": "older-than-2",
				"has_more":    true,
				"total":       3,
			}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"comments": []any{milestoneCommentJSON(1, "first")},
			"has_more": false,
		}})
	})

	code, out, errOut := runWS(t, server.URL, "", "milestone", "comment", "list", "5")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut)
	}
	var comments []MilestoneComment
	if err := json.Unmarshal([]byte(out), &comments); err != nil {
		t.Fatalf("output is not a JSON comment list: %v\n%s", err, out)
	}
	if len(comments) != 3 || comments[0].ID != 3 || comments[2].ID != 1 || comments[2].Content != "first" {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	got := requests()
	if len(got) != 2 {
		t.Fatalf("expected 2 page requests, got %d", len(got))
	}
	for i, req := range got {
		if req.Method != http.MethodGet || req.Path != "/rest/api/v2/milestones/5/comments" {
			t.Fatalf("request %d: %s %s", i, req.Method, req.Path)
		}
		if req.Auth != "Bearer test-token" {
			t.Fatalf("request %d: missing bearer token", i)
		}
		if !strings.Contains(req.Query, "page_size=100") {
			t.Fatalf("request %d: unexpected query %q", i, req.Query)
		}
	}
	if strings.Contains(got[0].Query, "cursor=") || !strings.Contains(got[1].Query, "cursor=older-than-2") {
		t.Fatalf("expected the second request to pass next_cursor, got %q then %q", got[0].Query, got[1].Query)
	}
}

func TestMilestoneCommentListTableOutput(t *testing.T) {
	server, _ := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"comments": []any{milestoneCommentJSON(1, "line one\nline two")},
			"has_more": false,
		}})
	})

	code, out, errOut := runWS(t, server.URL, "", "milestone", "comment", "list", "5", "-o", "table")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "AUTHOR") || !strings.Contains(out, "Korneel") || !strings.Contains(out, "line one line two") {
		t.Fatalf("unexpected table output:\n%s", out)
	}
}

func TestMilestoneCommentAddSendsMessage(t *testing.T) {
	server, requests := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]any{"data": milestoneCommentJSON(7, "hello")})
	})

	code, out, errOut := runWS(t, server.URL, "", "milestone", "comment", "add", "5", "-m", `hello\nworld`)
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut)
	}
	var comment MilestoneComment
	if err := json.Unmarshal([]byte(out), &comment); err != nil || comment.ID != 7 {
		t.Fatalf("expected the created comment as JSON, got %q (%v)", out, err)
	}
	got := requests()
	if len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != "/rest/api/v2/milestones/5/comments" {
		t.Fatalf("unexpected requests: %+v", got)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(got[0].Body), &body); err != nil || body["content"] != "hello\nworld" {
		t.Fatalf("unexpected request body %q", got[0].Body)
	}
}

func TestMilestoneCommentAddRejectsMissingBody(t *testing.T) {
	server, requests := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if code, _, errOut := runWS(t, server.URL, "", "milestone", "comment", "add", "5"); code == 0 || !strings.Contains(errOut, "message is required") {
		t.Fatalf("missing body: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := runWS(t, server.URL, "", "milestone", "comment", "add", "5", "-m", "   "); code == 0 || !strings.Contains(errOut, "message is required") {
		t.Fatalf("blank body: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := runWS(t, server.URL, "", "milestone", "comment", "add", "PROJ", "-m", "x"); code == 0 || !strings.Contains(errOut, "invalid milestone ID") {
		t.Fatalf("non-numeric milestone: exit %d, stderr %q", code, errOut)
	}
	if got := requests(); len(got) != 0 {
		t.Fatalf("expected no requests, got %+v", got)
	}
}

func TestMilestoneCommentEditUsesMergePatch(t *testing.T) {
	server, requests := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": milestoneCommentJSON(9, "edited")})
	})

	code, out, errOut := runWS(t, server.URL, "", "milestone", "comment", "edit", "5", "9", "-m", "edited")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, `"content": "edited"`) {
		t.Fatalf("expected the edited comment as JSON, got %s", out)
	}
	got := requests()
	if len(got) != 1 || got[0].Method != http.MethodPatch || got[0].Path != "/rest/api/v2/milestones/5/comments/9" {
		t.Fatalf("unexpected requests: %+v", got)
	}
	if got[0].ContentType != "application/merge-patch+json" {
		t.Fatalf("unexpected content type %q", got[0].ContentType)
	}
}

func TestMilestoneCommentEditReportsServerError(t *testing.T) {
	server, _ := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{
			"code": "not_found", "message": "Planning object was not found",
		}})
	})

	code, _, errOut := runWS(t, server.URL, "", "milestone", "comment", "edit", "5", "9", "-m", "edited")
	if code == 0 || !strings.Contains(errOut, "Planning object was not found") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestMilestoneCommentDelete(t *testing.T) {
	server, requests := milestoneCommentServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errOut := runWS(t, server.URL, "", "milestone", "comment", "delete", "5", "9")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil || result["deleted"] != true || result["comment_id"] != float64(9) || result["milestone_id"] != float64(5) {
		t.Fatalf("unexpected output %q (%v)", out, err)
	}
	got := requests()
	if len(got) != 1 || got[0].Method != http.MethodDelete || got[0].Path != "/rest/api/v2/milestones/5/comments/9" {
		t.Fatalf("unexpected requests: %+v", got)
	}
}
