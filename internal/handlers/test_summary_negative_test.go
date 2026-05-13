package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// Regression tests for docs/bughunt10.md:
//   - F1 (High): GET /workspaces/{ws}/test-runs/{id}/summary previously ignored
//     {ws} entirely. The repository now filters by workspace_id and the handler
//     parses the workspaceId path param, so a cross-workspace request must 404.
//   - F4 (Low):  The "All Test Results" markdown table previously interpolated
//     test-case titles unescaped. Titles with `|` or newlines could break the
//     table layout. The handler now routes every cell through
//     escapeMarkdownTableCell.

func newTestSummaryHandler(t *testing.T, db database.Database) *TestSummaryHandler {
	t.Helper()
	return NewTestSummaryHandlerWithPool(repository.NewTestSummaryRepository(db))
}

func markdownSummaryRequest(t *testing.T, workspaceID, runID, userID int) *http.Request {
	t.Helper()
	req := authedRequest(http.MethodGet,
		fmt.Sprintf("/workspaces/%d/test-runs/%d/summary", workspaceID, runID),
		userID, nil)
	req.SetPathValue("workspaceId", strconv.Itoa(workspaceID))
	req.SetPathValue("id", strconv.Itoa(runID))
	return req
}

// F1: requesting ws1's URL with a run that lives in ws2 must 404.
func TestTestSummaryHandler_GetMarkdownSummary_RejectsRunFromDifferentWorkspace(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	d := seedTwoTestWorkspaces(t, db)

	// seedTwoTestWorkspaces only seeds runs in ws1; add one in ws2 so we can
	// exercise the cross-workspace path.
	if _, err := db.Exec(`
		INSERT INTO test_runs (id, workspace_id, template_id, set_id, name)
		VALUES (403, ?, NULL, ?, 'Run in W2')
	`, d.ws2, d.set2); err != nil {
		t.Fatalf("seed ws2 run: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO test_results (id, run_id, test_case_id, status)
		VALUES (503, 403, ?, 'not_run')
	`, d.testCase2); err != nil {
		t.Fatalf("seed ws2 result: %v", err)
	}

	handler := newTestSummaryHandler(t, db)

	// Cross-workspace: ws1 caller asking for ws2's run → 404.
	rr := httptest.NewRecorder()
	handler.GetMarkdownSummary(rr, markdownSummaryRequest(t, d.ws1, 403, userID))
	if rr.Code == http.StatusOK {
		t.Fatalf("GetMarkdownSummary returned 200 for cross-workspace run; pre-fix bug. body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404. body=%s", rr.Code, rr.Body.String())
	}

	// Sanity: the same run via its own workspace returns 200.
	rr = httptest.NewRecorder()
	handler.GetMarkdownSummary(rr, markdownSummaryRequest(t, d.ws2, 403, userID))
	if rr.Code != http.StatusOK {
		t.Fatalf("same-workspace request unexpectedly failed with %d. body=%s", rr.Code, rr.Body.String())
	}
}

// F4: a test-case title containing `|` and newlines must be escaped/normalized
// in the "All Test Results" markdown table cell.
func TestTestSummaryHandler_GetMarkdownSummary_EscapesPipesAndNewlines(t *testing.T) {
	const userID = 1
	db := newNegativeTestDB(t)
	seedNegativeTestUser(t, db, userID)
	d := seedTwoTestWorkspaces(t, db)

	const badTitle = "Foo | Bar\nBaz"
	if _, err := db.Exec(`UPDATE test_cases SET title = ? WHERE id = ?`, badTitle, d.testCase1); err != nil {
		t.Fatalf("override test case title: %v", err)
	}

	handler := newTestSummaryHandler(t, db)
	rr := httptest.NewRecorder()
	handler.GetMarkdownSummary(rr, markdownSummaryRequest(t, d.ws1, d.run1, userID))
	if rr.Code != http.StatusOK {
		t.Fatalf("GetMarkdownSummary failed: status=%d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Markdown string `json:"markdown"`
	}
	decodeJSONBody(t, rr, &payload)

	// The unescaped raw title must not appear in any table row — that would
	// mean either an unescaped pipe (extra column) or a literal newline (row break).
	if strings.Contains(payload.Markdown, badTitle) {
		t.Errorf("markdown contains the raw unescaped title; escaping was not applied:\n%s", payload.Markdown)
	}

	// Locate the "All Test Results" section and the row that follows the
	// header separator. That row must contain exactly three logical cells
	// (four `|` delimiters when bracketed).
	idx := strings.Index(payload.Markdown, "## All Test Results")
	if idx < 0 {
		t.Fatalf("markdown is missing the All Test Results section:\n%s", payload.Markdown)
	}
	section := payload.Markdown[idx:]
	lines := strings.Split(section, "\n")
	var dataRow string
	for i, line := range lines {
		if strings.HasPrefix(line, "|-") && i+1 < len(lines) {
			dataRow = lines[i+1]
			break
		}
	}
	if dataRow == "" {
		t.Fatalf("could not locate the data row in the All Test Results table:\n%s", section)
	}
	// 4 unescaped pipes = 3-column row. An unescaped pipe inside the title
	// would produce 5 (or more) delimiters and split the row into extra cells.
	if got, want := strings.Count(dataRow, "|")-strings.Count(dataRow, `\|`), 4; got != want {
		t.Errorf("data row unescaped pipe count = %d, want %d (line=%q)", got, want, dataRow)
	}
	if !strings.Contains(dataRow, `Foo \| Bar Baz`) {
		t.Errorf("data row does not contain the escaped/normalized title; got: %q", dataRow)
	}
}
