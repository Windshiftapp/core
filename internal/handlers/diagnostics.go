package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
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
	fracIndexRepo    *repository.FracIndexRepository
	auditor          *logger.Auditor
}

// NewDiagnosticsHandler creates a new diagnostics handler.
func NewDiagnosticsHandler(
	actionRepo *repository.ActionRepository,
	deliveryRepo *repository.WebhookDeliveryRepository,
	schedulerRunRepo *repository.SchedulerRunRepository,
	fracIndexRepo *repository.FracIndexRepository,
	auditor *logger.Auditor,
) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		actionRepo:       actionRepo,
		deliveryRepo:     deliveryRepo,
		schedulerRunRepo: schedulerRunRepo,
		fracIndexRepo:    fracIndexRepo,
		auditor:          auditor,
	}
}

// GetFracIndexState returns a snapshot of in-memory cache state and DB state
// for the items.frac_index column, scoped to the admin diagnostics panel.
// "healthy" is false when the column collation diverges from byte ordering
// or when the cache's predicted-next key already exists in the table.
//
// GET /api/admin/diagnostics/frac-index
func (h *DiagnosticsHandler) GetFracIndexState(w http.ResponseWriter, r *http.Request) {
	cache := services.GetFracIndexCacheStats()
	dbState, err := h.fracIndexRepo.GetDBStats(cache.NextWouldBe)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	healthy := !dbState.CollationMismatch && dbState.PredictedCollision == nil
	respondJSONOK(w, map[string]any{
		"cache":   cache,
		"db":      dbState,
		"healthy": healthy,
	})
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

	cutoff := time.Now().Add(-dur)
	rows, err := h.deliveryRepo.Purge(r.Context(), cutoff)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditPurge(r, logger.ActionDiagnosticsWebhookDeliveriesPurge, req.OlderThan, cutoff, rows)
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

	cutoff := time.Now().Add(-dur)
	rows, err := h.schedulerRunRepo.Purge(r.Context(), cutoff)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditPurge(r, logger.ActionDiagnosticsSchedulerRunsPurge, req.OlderThan, cutoff, rows)
	respondJSONOK(w, map[string]int64{"deleted": rows})
}

func (h *DiagnosticsHandler) auditPurge(r *http.Request, action, olderThan string, cutoff time.Time, rows int64) {
	if h.auditor == nil {
		return
	}
	user := utils.GetCurrentUser(r)
	if user == nil {
		return
	}
	h.auditor.LogWithDetails(r, user, action, logger.ResourceDiagnostics, nil, "", map[string]interface{}{
		"older_than": olderThan,
		"cutoff":     cutoff.Format(time.RFC3339),
		"deleted":    rows,
	})
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
