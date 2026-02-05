package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type TestSummaryHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewTestSummaryHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestSummaryHandler {
	return &TestSummaryHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

func (h *TestSummaryHandler) GetMarkdownSummary(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "run ID")
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Get run details
	var runName, setName string
	var startedAt, endedAt sql.NullTime
	err = db.QueryRow(`
		SELECT tr.name, tr.started_at, tr.ended_at, ts.name
		FROM test_runs tr
		JOIN test_sets ts ON tr.set_id = ts.id
		WHERE tr.id = ?
	`, runID).Scan(&runName, &startedAt, &endedAt, &setName)

	if err != nil {
		respondNotFound(w, r, "test_run")
		return
	}

	// Get test results
	rows, err := db.Query(`
		SELECT tc.title, tr.status, tr.actual_result, tr.notes
		FROM test_results tr
		JOIN test_cases tc ON tr.test_case_id = tc.id
		WHERE tr.run_id = ?
		ORDER BY tc.id
	`, runID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type TestSummary struct {
		Title        string
		Status       string
		ActualResult string
		Notes        string
	}

	var results []TestSummary
	var stats = map[string]int{
		"total":   0,
		"passed":  0,
		"failed":  0,
		"blocked": 0,
		"skipped": 0,
		"not_run": 0,
	}

	for rows.Next() {
		var ts TestSummary
		var actualResult, notes sql.NullString
		err := rows.Scan(&ts.Title, &ts.Status, &actualResult, &notes)
		if err != nil {
			continue
		}

		// Handle NULL values
		if actualResult.Valid {
			ts.ActualResult = actualResult.String
		}
		if notes.Valid {
			ts.Notes = notes.String
		}

		results = append(results, ts)
		stats["total"]++
		stats[ts.Status]++
	}

	// Build markdown
	var markdown strings.Builder

	markdown.WriteString(fmt.Sprintf("# Test Run Summary: %s\n\n", runName))
	markdown.WriteString(fmt.Sprintf("**Test Set:** %s\n\n", setName))
	if startedAt.Valid {
		markdown.WriteString(fmt.Sprintf("**Started:** %s\n\n", startedAt.Time.Format("2006-01-02 15:04:05")))
	}
	if endedAt.Valid {
		markdown.WriteString(fmt.Sprintf("**Ended:** %s\n\n", endedAt.Time.Format("2006-01-02 15:04:05")))
		if startedAt.Valid {
			duration := endedAt.Time.Sub(startedAt.Time)
			markdown.WriteString(fmt.Sprintf("**Duration:** %s\n\n", duration.Round(time.Second)))
		}
	}

	markdown.WriteString("## Statistics\n\n")
	markdown.WriteString("| Status | Count | Percentage |\n")
	markdown.WriteString("|--------|-------|------------|\n")

	if stats["total"] > 0 {
		markdown.WriteString(fmt.Sprintf("| ✅ Passed | %d | %.1f%% |\n", stats["passed"], float64(stats["passed"])/float64(stats["total"])*100))
		markdown.WriteString(fmt.Sprintf("| ❌ Failed | %d | %.1f%% |\n", stats["failed"], float64(stats["failed"])/float64(stats["total"])*100))
		markdown.WriteString(fmt.Sprintf("| ⚠️ Blocked | %d | %.1f%% |\n", stats["blocked"], float64(stats["blocked"])/float64(stats["total"])*100))
		markdown.WriteString(fmt.Sprintf("| ⏭️ Skipped | %d | %.1f%% |\n", stats["skipped"], float64(stats["skipped"])/float64(stats["total"])*100))
		markdown.WriteString(fmt.Sprintf("| ⏸️ Not Run | %d | %.1f%% |\n", stats["not_run"], float64(stats["not_run"])/float64(stats["total"])*100))
		markdown.WriteString(fmt.Sprintf("| **Total** | **%d** | **100%%** |\n\n", stats["total"]))

		passRate := float64(stats["passed"]) / float64(stats["total"]) * 100
		markdown.WriteString(fmt.Sprintf("**Overall Pass Rate:** %.1f%%\n\n", passRate))
	}

	// Failed tests details
	if stats["failed"] > 0 {
		markdown.WriteString("## Failed Tests\n\n")
		for _, result := range results {
			if result.Status == "failed" {
				markdown.WriteString(fmt.Sprintf("### ❌ %s\n\n", result.Title))
				if result.ActualResult != "" {
					markdown.WriteString(fmt.Sprintf("**Actual Result:**\n%s\n\n", result.ActualResult))
				}
				if result.Notes != "" {
					markdown.WriteString(fmt.Sprintf("**Notes:**\n%s\n\n", result.Notes))
				}
				markdown.WriteString("---\n\n")
			}
		}
	}

	// Blocked tests
	if stats["blocked"] > 0 {
		markdown.WriteString("## Blocked Tests\n\n")
		for _, result := range results {
			if result.Status == "blocked" {
				markdown.WriteString(fmt.Sprintf("### ⚠️ %s\n", result.Title))
				if result.Notes != "" {
					markdown.WriteString(fmt.Sprintf("**Reason:** %s\n", result.Notes))
				}
				markdown.WriteString("\n")
			}
		}
	}

	// All test results table
	markdown.WriteString("## All Test Results\n\n")
	markdown.WriteString("| Test Case | Status | Notes |\n")
	markdown.WriteString("|-----------|--------|-------|\n")

	for _, result := range results {
		statusIcon := ""
		switch result.Status {
		case "passed":
			statusIcon = "✅"
		case "failed":
			statusIcon = "❌"
		case "blocked":
			statusIcon = "⚠️"
		case "skipped":
			statusIcon = "⏭️"
		default:
			statusIcon = "⏸️"
		}

		notes := result.Notes
		if notes == "" {
			notes = "-"
		}
		// Escape pipe characters in notes for markdown table
		notes = strings.ReplaceAll(notes, "|", "\\|")

		markdown.WriteString(fmt.Sprintf("| %s | %s %s | %s |\n",
			result.Title,
			statusIcon,
			cases.Title(language.English).String(result.Status),
			notes))
	}

	response := map[string]string{
		"markdown": markdown.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// GetReportsSummary returns aggregate test reports for a workspace
// Supports optional milestone_id and days query parameters
func (h *TestSummaryHandler) GetReportsSummary(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspace ID")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	// Parse optional query parameters
	milestoneIDStr := r.URL.Query().Get("milestone_id")
	daysStr := r.URL.Query().Get("days")

	var milestoneID *int
	if milestoneIDStr != "" {
		var mid int
		mid, err = strconv.Atoi(milestoneIDStr)
		if err != nil {
			respondInvalidID(w, r, "milestone_id")
			return
		}
		milestoneID = &mid
	}

	days := 30 // default
	if daysStr != "" {
		var d int
		d, err = strconv.Atoi(daysStr)
		if err != nil || d < 1 || d > 365 {
			respondValidationError(w, r, "Invalid days parameter (must be 1-365)")
			return
		}
		days = d
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	// Build the base query parts - separate FROM/JOIN from WHERE for proper SQL structure
	baseFrom := `
		FROM test_runs tr
		JOIN test_sets ts ON tr.set_id = ts.id
	`
	baseWhere := `
		WHERE tr.workspace_id = ?
		AND tr.started_at >= ?
	`
	baseArgs := []interface{}{workspaceID, startDate}

	if milestoneID != nil {
		baseWhere += " AND ts.milestone_id = ?"
		baseArgs = append(baseArgs, *milestoneID)
	}

	// Get overall stats
	statsQuery := `
		SELECT
			COUNT(DISTINCT tr.id) as total_runs,
			COUNT(tres.id) as total_tests,
			SUM(CASE WHEN tres.status = 'passed' THEN 1 ELSE 0 END) as passed,
			SUM(CASE WHEN tres.status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN tres.status = 'blocked' THEN 1 ELSE 0 END) as blocked,
			SUM(CASE WHEN tres.status = 'skipped' THEN 1 ELSE 0 END) as skipped,
			SUM(CASE WHEN tres.status = 'not_run' THEN 1 ELSE 0 END) as not_run
		` + baseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		` + baseWhere

	var totalRuns, totalTests int
	var passed, failed, blocked, skipped, notRun sql.NullInt64
	err = db.QueryRow(statsQuery, baseArgs...).Scan(
		&totalRuns, &totalTests, &passed, &failed, &blocked, &skipped, &notRun,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var passRate float64
	if totalTests > 0 {
		passRate = float64(passed.Int64) / float64(totalTests) * 100
	}

	// Get trend data (daily pass rates)
	//nolint:gosec // G202: table name from whitelist, parameters are bound
	trendQuery := `
		SELECT
			DATE(tr.started_at) as date,
			COUNT(tres.id) as total,
			SUM(CASE WHEN tres.status = 'passed' THEN 1 ELSE 0 END) as passed
		` + baseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		` + baseWhere + `
		GROUP BY DATE(tr.started_at)
		ORDER BY date
	`

	trendRows, err := db.Query(trendQuery, baseArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = trendRows.Close() }()

	type TrendPoint struct {
		Date     string  `json:"date"`
		PassRate float64 `json:"pass_rate"`
		Total    int     `json:"total"`
	}

	trend := make([]TrendPoint, 0)
	for trendRows.Next() {
		var date string
		var total int
		var passedCount sql.NullInt64
		if err = trendRows.Scan(&date, &total, &passedCount); err != nil {
			continue
		}

		var rate float64
		if total > 0 {
			rate = float64(passedCount.Int64) / float64(total) * 100
		}

		trend = append(trend, TrendPoint{
			Date:     date,
			PassRate: rate,
			Total:    total,
		})
	}

	// Get recent failures
	//nolint:gosec // G202: table name from whitelist, parameters are bound
	failuresQuery := `
		SELECT
			tc.id as test_case_id,
			tc.title as test_case_title,
			tr.id as run_id,
			tr.name as run_name,
			tres.executed_at as failed_at
		` + baseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		LEFT JOIN test_cases tc ON tres.test_case_id = tc.id
		` + baseWhere + `
		AND tres.status = 'failed'
		ORDER BY tres.executed_at DESC
		LIMIT 20
	`

	failureRows, err := db.Query(failuresQuery, baseArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = failureRows.Close() }()

	type RecentFailure struct {
		TestCaseID    int        `json:"test_case_id"`
		TestCaseTitle string     `json:"test_case_title"`
		RunID         int        `json:"run_id"`
		RunName       string     `json:"run_name"`
		FailedAt      *time.Time `json:"failed_at"`
	}

	failures := make([]RecentFailure, 0)
	for failureRows.Next() {
		var f RecentFailure
		var failedAt sql.NullTime
		if err = failureRows.Scan(&f.TestCaseID, &f.TestCaseTitle, &f.RunID, &f.RunName, &failedAt); err != nil {
			continue
		}
		if failedAt.Valid {
			f.FailedAt = &failedAt.Time
		}
		failures = append(failures, f)
	}

	// Get recent blocked tests with reasons
	//nolint:gosec // G202: table name from whitelist, parameters are bound
	blockedQuery := `
		SELECT
			tc.id as test_case_id,
			tc.title as test_case_title,
			tr.id as run_id,
			tr.name as run_name,
			tres.notes as reason,
			tres.executed_at as blocked_at
		` + baseFrom + `
		LEFT JOIN test_results tres ON tr.id = tres.run_id
		LEFT JOIN test_cases tc ON tres.test_case_id = tc.id
		` + baseWhere + `
		AND tres.status = 'blocked'
		ORDER BY tres.executed_at DESC
		LIMIT 20
	`

	blockedRows, err := db.Query(blockedQuery, baseArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = blockedRows.Close() }()

	type RecentBlocked struct {
		TestCaseID    int        `json:"test_case_id"`
		TestCaseTitle string     `json:"test_case_title"`
		RunID         int        `json:"run_id"`
		RunName       string     `json:"run_name"`
		Reason        string     `json:"reason"`
		BlockedAt     *time.Time `json:"blocked_at"`
	}

	blockedTests := make([]RecentBlocked, 0)
	for blockedRows.Next() {
		var b RecentBlocked
		var reason sql.NullString
		var blockedAt sql.NullTime
		if err := blockedRows.Scan(&b.TestCaseID, &b.TestCaseTitle, &b.RunID, &b.RunName, &reason, &blockedAt); err != nil {
			continue
		}
		if reason.Valid {
			b.Reason = reason.String
		}
		if blockedAt.Valid {
			b.BlockedAt = &blockedAt.Time
		}
		blockedTests = append(blockedTests, b)
	}

	// Build response
	response := map[string]interface{}{
		"overall": map[string]interface{}{
			"total_runs":  totalRuns,
			"total_tests": totalTests,
			"passed":      passed.Int64,
			"failed":      failed.Int64,
			"blocked":     blocked.Int64,
			"skipped":     skipped.Int64,
			"not_run":     notRun.Int64,
			"pass_rate":   passRate,
		},
		"trend":           trend,
		"recent_failures": failures,
		"recent_blocked":  blockedTests,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
