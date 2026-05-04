package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// DiagnosticsHandler exposes admin-only system diagnostics endpoints.
//
// Each endpoint reuses existing instrumentation (action_execution_logs,
// webhook_deliveries, scheduler_runs) and is read-only except for the manual
// purge endpoints, which delete old rows on demand.
type DiagnosticsHandler struct {
	actionRepo       *repository.ActionRepository
	deliveryRepo     *repository.WebhookDeliveryRepository
	schedulerRunRepo *repository.SchedulerRunRepository
}

// NewDiagnosticsHandler creates a new diagnostics handler.
func NewDiagnosticsHandler(db database.Database) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		actionRepo:       repository.NewActionRepository(db),
		deliveryRepo:     repository.NewWebhookDeliveryRepository(db),
		schedulerRunRepo: repository.NewSchedulerRunRepository(db),
	}
}

// GetActionLogs returns recent cross-workspace action execution logs.
//
// Query params:
//   - mode:  "failed" (default — recent failures) or "slowest" (longest-running completed runs)
//   - since: duration string like "24h", "1h", "15m" — defaults to 24h
//   - limit: max rows (default 25, capped at 200)
//
// GET /api/admin/diagnostics/action-logs
func (h *DiagnosticsHandler) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "failed"
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		sinceStr = "24h"
	}
	dur, err := time.ParseDuration(sinceStr)
	if err != nil {
		respondValidationError(w, r, "invalid 'since' duration")
		return
	}

	limit := 25
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	opts := repository.RecentExecutionLogsOpts{
		Since: time.Now().Add(-dur),
		Limit: limit,
	}
	switch mode {
	case "failed":
		opts.Status = "failed"
	case "slowest":
		opts.SortBy = "duration"
	default:
		respondValidationError(w, r, "mode must be 'failed' or 'slowest'")
		return
	}

	logs, err := h.actionRepo.GetRecentExecutionLogs(opts)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}
	respondJSONOK(w, logs)
}

// GetWebhookDeliveries returns recent outbound webhook delivery rows.
//
// Query params:
//   - status:     "" (any), "failed", or "success"
//   - channel_id: optional integer to scope to one channel
//   - since:      duration string (default "24h")
//   - limit:      max rows (default 25, capped at 200)
//
// GET /api/admin/diagnostics/webhook-deliveries
func (h *DiagnosticsHandler) GetWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	since, err := parseSinceDuration(q.Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	limit := 25
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	channelID := 0
	if v := q.Get("channel_id"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			channelID = parsed
		}
	}

	opts := repository.RecentDeliveriesOpts{
		Status:    q.Get("status"),
		ChannelID: channelID,
		Since:     time.Now().Add(-since),
		Limit:     limit,
	}
	if opts.Status != "" && opts.Status != "failed" && opts.Status != "success" {
		respondValidationError(w, r, "status must be 'failed' or 'success'")
		return
	}

	rows, err := h.deliveryRepo.GetRecent(opts)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if rows == nil {
		rows = []*models.WebhookDelivery{}
	}
	respondJSONOK(w, rows)
}

// GetWebhookStats returns per-channel delivery aggregates for a time window.
//
// Query params:
//   - since: duration string (default "24h")
//
// GET /api/admin/diagnostics/webhook-stats
func (h *DiagnosticsHandler) GetWebhookStats(w http.ResponseWriter, r *http.Request) {
	since, err := parseSinceDuration(r.URL.Query().Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	stats, err := h.deliveryRepo.Stats(time.Now().Add(-since))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if stats == nil {
		stats = []*repository.ChannelDeliveryStats{}
	}
	respondJSONOK(w, stats)
}

// PurgeWebhookDeliveriesRequest is the body for the manual purge endpoint.
type PurgeWebhookDeliveriesRequest struct {
	OlderThan string `json:"older_than"` // duration string, e.g. "30d", "168h"
}

// PurgeWebhookDeliveries deletes delivery rows older than the requested cutoff.
//
// Body: { "older_than": "30d" }  (or any Go-style duration; "d" = 24h here)
//
// POST /api/admin/diagnostics/webhook-deliveries/purge
func (h *DiagnosticsHandler) PurgeWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[PurgeWebhookDeliveriesRequest](w, r)
	if !ok {
		return
	}
	dur, err := parseExtendedDuration(req.OlderThan)
	if err != nil {
		respondValidationError(w, r, "invalid 'older_than' duration: "+err.Error())
		return
	}
	if dur < time.Hour {
		respondValidationError(w, r, "'older_than' must be at least 1h to avoid wiping live data")
		return
	}

	rows, err := h.deliveryRepo.Purge(r.Context(), time.Now().Add(-dur))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]int64{"deleted": rows})
}

// GetSchedulerRuns returns recent in-process scheduler tick history.
//
// Query params:
//   - scheduler: "" (any), "briefing", "email", "recurrence", "notification"
//   - status:    "" (any), "success", or "failed"
//   - since:     duration string (default "24h")
//   - limit:     max rows (default 25, capped at 200)
//
// GET /api/admin/diagnostics/scheduler-runs
func (h *DiagnosticsHandler) GetSchedulerRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	since, err := parseSinceDuration(q.Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	limit := 25
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	opts := repository.RecentSchedulerRunsOpts{
		Scheduler: q.Get("scheduler"),
		Status:    q.Get("status"),
		Since:     time.Now().Add(-since),
		Limit:     limit,
	}
	if opts.Status != "" && opts.Status != "success" && opts.Status != "failed" {
		respondValidationError(w, r, "status must be 'success' or 'failed'")
		return
	}

	runs, err := h.schedulerRunRepo.GetRecent(opts)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if runs == nil {
		runs = []*models.SchedulerRun{}
	}
	respondJSONOK(w, runs)
}

// GetSchedulerStats returns per-scheduler aggregates for a time window.
//
// Query params:
//   - since: duration string (default "24h")
//
// GET /api/admin/diagnostics/scheduler-stats
func (h *DiagnosticsHandler) GetSchedulerStats(w http.ResponseWriter, r *http.Request) {
	since, err := parseSinceDuration(r.URL.Query().Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	stats, err := h.schedulerRunRepo.Stats(time.Now().Add(-since))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if stats == nil {
		stats = []*repository.SchedulerStats{}
	}
	respondJSONOK(w, stats)
}

// PurgeSchedulerRuns deletes scheduler run rows older than the requested cutoff.
//
// Body: { "older_than": "30d" }
//
// POST /api/admin/diagnostics/scheduler-runs/purge
func (h *DiagnosticsHandler) PurgeSchedulerRuns(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[PurgeWebhookDeliveriesRequest](w, r)
	if !ok {
		return
	}
	dur, err := parseExtendedDuration(req.OlderThan)
	if err != nil {
		respondValidationError(w, r, "invalid 'older_than' duration: "+err.Error())
		return
	}
	if dur < time.Hour {
		respondValidationError(w, r, "'older_than' must be at least 1h to avoid wiping live data")
		return
	}

	rows, err := h.schedulerRunRepo.Purge(r.Context(), time.Now().Add(-dur))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]int64{"deleted": rows})
}

// parseSinceDuration parses a duration string with a default fallback.
//
//nolint:unparam // def kept on signature for callers that may pass non-default windows in the future
func parseSinceDuration(s string, def time.Duration) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	d, err := parseExtendedDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid 'since' duration: %w", err)
	}
	return d, nil
}

// parseExtendedDuration parses Go duration strings, additionally treating a
// "d" suffix as days (e.g. "30d" → 30 * 24h). Standard time.ParseDuration does
// not accept "d", but humans expect it for retention windows.
func parseExtendedDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
