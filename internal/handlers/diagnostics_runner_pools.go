package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
)

// runnerPoolDiag is one pool's health snapshot for the admin Diagnostics
// panel: who could claim work (runner liveness) versus what is waiting
// (queued/running runs). Healthy=false is the "assigned a ticket and nothing
// happens" signature — queued work with nobody live to claim it.
type runnerPoolDiag struct {
	ID                  int        `json:"id"`
	Name                string     `json:"name"`
	Enabled             bool       `json:"enabled"`
	MaxConcurrentRuns   int        `json:"max_concurrent_runs"` // 0 = unlimited
	LiveRunners         int        `json:"live_runners"`
	StaleRunners        int        `json:"stale_runners"` // active rows past the liveness window (reaper will revoke)
	RevokedRunners      int        `json:"revoked_runners"`
	LastHeartbeatAt     *time.Time `json:"last_heartbeat_at,omitempty"` // newest across the pool
	QueuedRuns          int        `json:"queued_runs"`
	RunningRuns         int        `json:"running_runs"`
	OldestQueuedSeconds int        `json:"oldest_queued_seconds,omitempty"`
	Healthy             bool       `json:"healthy"`
}

// GetRunnerPools returns a per-pool health snapshot for every runner_pool
// capability. Read-only; mirrors the lease reaper's liveness window so this
// panel and the reaper never disagree about what counts as alive.
//
// GET /api/admin/diagnostics/runner-pools
func (h *DiagnosticsHandler) GetRunnerPools(w http.ResponseWriter, r *http.Request) {
	if err := h.controls.Check(operatorcontrols.RuntimeDiagnostics); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if h.runnerRepo == nil || h.agentRunRepo == nil {
		respondJSONOK(w, []runnerPoolDiag{})
		return
	}
	caps, err := h.actionRepo.ListCapabilities()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	now := time.Now().UTC()
	freshSince := now.Add(-models.RunnerLivenessWindow)

	// One query for every queued remote run (olderThan=now matches all),
	// bucketed per pool below — cheaper than a per-pool query and it also
	// yields the oldest-queued age for free (results are oldest-first).
	queued, err := h.agentRunRepo.ListStaleQueuedPoolRuns(r.Context(), now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	queuedCount := map[int]int{}
	oldestQueued := map[int]time.Time{}
	for _, run := range queued {
		queuedCount[run.PoolID]++
		if _, ok := oldestQueued[run.PoolID]; !ok {
			oldestQueued[run.PoolID] = run.QueuedAt
		}
	}

	out := []runnerPoolDiag{}
	for _, capRow := range caps {
		if capRow.CapabilityType != models.CapabilityRunnerPool {
			continue
		}
		var cfg models.RunnerPoolConfig
		_ = json.Unmarshal([]byte(capRow.Config), &cfg)
		diag := runnerPoolDiag{
			ID:                capRow.ID,
			Name:              capRow.Name,
			Enabled:           capRow.IsEnabled,
			MaxConcurrentRuns: cfg.MaxConcurrentRuns,
			QueuedRuns:        queuedCount[capRow.ID],
		}

		instances, err := h.runnerRepo.ListInstancesForPool(r.Context(), capRow.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for _, inst := range instances {
			if inst.Status != models.RunnerInstanceStatusActive {
				diag.RevokedRunners++
				continue
			}
			lastSeen := inst.RegisteredAt
			if inst.LastHeartbeatAt != nil {
				lastSeen = *inst.LastHeartbeatAt
			}
			if lastSeen.After(freshSince) || lastSeen.Equal(freshSince) {
				diag.LiveRunners++
			} else {
				diag.StaleRunners++
			}
			if inst.LastHeartbeatAt != nil && (diag.LastHeartbeatAt == nil || inst.LastHeartbeatAt.After(*diag.LastHeartbeatAt)) {
				diag.LastHeartbeatAt = inst.LastHeartbeatAt
			}
		}

		if diag.RunningRuns, err = h.agentRunRepo.CountRunningForPool(r.Context(), capRow.ID); err != nil {
			respondInternalError(w, r, err)
			return
		}
		if at, ok := oldestQueued[capRow.ID]; ok {
			diag.OldestQueuedSeconds = int(now.Sub(at).Seconds())
		}
		diag.Healthy = diag.QueuedRuns == 0 || diag.LiveRunners > 0
		out = append(out, diag)
	}
	respondJSONOK(w, out)
}
