package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type TestSummaryHandler struct {
	*BaseHandler
}

func NewTestSummaryHandlerWithPool(db database.Database) *TestSummaryHandler {
	return &TestSummaryHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

func (h *TestSummaryHandler) GetMarkdownSummary(w http.ResponseWriter, r *http.Request) {
	runID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	repo := repository.NewTestSummaryRepository(db)

	header, err := repo.FindMarkdownRunHeader(runID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_run")
		return
	}
	if err != nil {
		respondNotFound(w, r, "test_run")
		return
	}

	results, err := repo.FindMarkdownResults(runID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	stats := map[string]int{
		"total":   0,
		"passed":  0,
		"failed":  0,
		"blocked": 0,
		"skipped": 0,
		"not_run": 0,
	}
	for _, res := range results {
		stats["total"]++
		stats[res.Status]++
	}

	var markdown strings.Builder

	fmt.Fprintf(&markdown, "# Test Run Summary: %s\n\n", header.RunName)
	fmt.Fprintf(&markdown, "**Test Set:** %s\n\n", header.SetName)
	if header.StartedAt.Valid {
		fmt.Fprintf(&markdown, "**Started:** %s\n\n", header.StartedAt.Time.Format("2006-01-02 15:04:05"))
	}
	if header.EndedAt.Valid {
		fmt.Fprintf(&markdown, "**Ended:** %s\n\n", header.EndedAt.Time.Format("2006-01-02 15:04:05"))
		if header.StartedAt.Valid {
			duration := header.EndedAt.Time.Sub(header.StartedAt.Time)
			fmt.Fprintf(&markdown, "**Duration:** %s\n\n", duration.Round(time.Second))
		}
	}

	markdown.WriteString("## Statistics\n\n")
	markdown.WriteString("| Status | Count | Percentage |\n")
	markdown.WriteString("|--------|-------|------------|\n")

	if stats["total"] > 0 {
		fmt.Fprintf(&markdown, "| ✅ Passed | %d | %.1f%% |\n", stats["passed"], float64(stats["passed"])/float64(stats["total"])*100)
		fmt.Fprintf(&markdown, "| ❌ Failed | %d | %.1f%% |\n", stats["failed"], float64(stats["failed"])/float64(stats["total"])*100)
		fmt.Fprintf(&markdown, "| ⚠️ Blocked | %d | %.1f%% |\n", stats["blocked"], float64(stats["blocked"])/float64(stats["total"])*100)
		fmt.Fprintf(&markdown, "| ⏭️ Skipped | %d | %.1f%% |\n", stats["skipped"], float64(stats["skipped"])/float64(stats["total"])*100)
		fmt.Fprintf(&markdown, "| ⏸️ Not Run | %d | %.1f%% |\n", stats["not_run"], float64(stats["not_run"])/float64(stats["total"])*100)
		fmt.Fprintf(&markdown, "| **Total** | **%d** | **100%%** |\n\n", stats["total"])

		passRate := float64(stats["passed"]) / float64(stats["total"]) * 100
		fmt.Fprintf(&markdown, "**Overall Pass Rate:** %.1f%%\n\n", passRate)
	}

	if stats["failed"] > 0 {
		markdown.WriteString("## Failed Tests\n\n")
		for _, result := range results {
			if result.Status == "failed" {
				fmt.Fprintf(&markdown, "### ❌ %s\n\n", result.Title)
				if result.ActualResult != "" {
					fmt.Fprintf(&markdown, "**Actual Result:**\n%s\n\n", result.ActualResult)
				}
				if result.Notes != "" {
					fmt.Fprintf(&markdown, "**Notes:**\n%s\n\n", result.Notes)
				}
				markdown.WriteString("---\n\n")
			}
		}
	}

	if stats["blocked"] > 0 {
		markdown.WriteString("## Blocked Tests\n\n")
		for _, result := range results {
			if result.Status == "blocked" {
				fmt.Fprintf(&markdown, "### ⚠️ %s\n", result.Title)
				if result.Notes != "" {
					fmt.Fprintf(&markdown, "**Reason:** %s\n", result.Notes)
				}
				markdown.WriteString("\n")
			}
		}
	}

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
		notes = strings.ReplaceAll(notes, "|", "\\|")

		fmt.Fprintf(&markdown, "| %s | %s %s | %s |\n",
			result.Title,
			statusIcon,
			cases.Title(language.English).String(result.Status),
			notes)
	}

	respondJSONOK(w, map[string]string{"markdown": markdown.String()})
}

// GetReportsSummary returns aggregate test reports for a workspace
// Supports optional milestone_id and days query parameters
func (h *TestSummaryHandler) GetReportsSummary(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	milestoneIDStr := r.URL.Query().Get("milestone_id")
	daysStr := r.URL.Query().Get("days")

	var milestoneID *int
	if milestoneIDStr != "" {
		mid, err := strconv.Atoi(milestoneIDStr)
		if err != nil {
			respondInvalidID(w, r, "milestone_id")
			return
		}
		milestoneID = &mid
	}

	days := 30
	if daysStr != "" {
		d, err := strconv.Atoi(daysStr)
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

	filter := repository.ReportFilter{
		WorkspaceID: workspaceID,
		MilestoneID: milestoneID,
		StartDate:   time.Now().AddDate(0, 0, -days),
	}

	repo := repository.NewTestSummaryRepository(db)

	stats, err := repo.GetOverallStats(filter)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	trend, err := repo.GetTrend(filter)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	failures, err := repo.GetRecentFailures(filter, 20)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	blocked, err := repo.GetRecentBlocked(filter, 20)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"overall": map[string]interface{}{
			"total_runs":  stats.TotalRuns,
			"total_tests": stats.TotalTests,
			"passed":      stats.Passed,
			"failed":      stats.Failed,
			"blocked":     stats.Blocked,
			"skipped":     stats.Skipped,
			"not_run":     stats.NotRun,
			"pass_rate":   stats.PassRate(),
		},
		"trend":           trend,
		"recent_failures": failures,
		"recent_blocked":  blocked,
	})
}
