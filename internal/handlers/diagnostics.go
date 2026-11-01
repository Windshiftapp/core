package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/cacheutil"
	"windshift/internal/config"
	domainevents "windshift/internal/events"
	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/webhook"
)

// DiagnosticsHandler exposes admin-only system diagnostics endpoints.
//
// Each endpoint reuses existing instrumentation (action_execution_logs,
// webhook_deliveries, scheduler_runs) and is read-only except for the manual
// purge endpoints, which delete old rows on demand.
type DiagnosticsHandler struct {
	sessionManager      *auth.SessionManager
	databaseDiagRepo    *repository.DatabaseDiagnosticsRepository
	actionRepo          *repository.ActionRepository
	deliveryRepo        *repository.WebhookDeliveryRepository
	schedulerRunRepo    *repository.SchedulerRunRepository
	fracIndexRepo       *repository.FracIndexRepository
	aiRepo              *repository.AIRepository
	llmManager          *llm.ConnectionManager
	llmCache            *llm.ModelCache
	auditor             *logger.Auditor
	runnerRepo          *repository.RunnerRepository
	agentRunRepo        *repository.AgentRunRepository
	webhookSender       *webhook.WebhookSender
	transitionMatrix    *services.TransitionMatrixService
	bulkOperations      *services.BulkOperationMetrics
	recurrenceRepo      *repository.RecurrenceRepository
	settingsRepo        *repository.SystemSettingRepository
	globalRankScheduler *scheduler.GlobalRankMigrationScheduler
	memoryBudget        config.MemoryBudget
	eventStore          *domainevents.Store
	scmHealthRepo       *repository.SCMHealthRepository
	channelRepo         *repository.ChannelRepository
}

// NewDiagnosticsHandler creates a new diagnostics handler.
func NewDiagnosticsHandler(
	sessionManager *auth.SessionManager,
	databaseDiagRepo *repository.DatabaseDiagnosticsRepository,
	actionRepo *repository.ActionRepository,
	deliveryRepo *repository.WebhookDeliveryRepository,
	schedulerRunRepo *repository.SchedulerRunRepository,
	fracIndexRepo *repository.FracIndexRepository,
	aiRepo *repository.AIRepository,
	llmManager *llm.ConnectionManager,
	llmCache *llm.ModelCache,
	auditor *logger.Auditor,
	runnerRepo *repository.RunnerRepository,
	agentRunRepo *repository.AgentRunRepository,
	webhookSender *webhook.WebhookSender,
	transitionMatrix *services.TransitionMatrixService,
	bulkOperations *services.BulkOperationMetrics,
	recurrenceRepo *repository.RecurrenceRepository,
	settingsRepo *repository.SystemSettingRepository,
	globalRankScheduler *scheduler.GlobalRankMigrationScheduler,
	memoryBudget config.MemoryBudget,
	eventStore *domainevents.Store,
	scmHealthRepo *repository.SCMHealthRepository,
	channelRepo *repository.ChannelRepository,
) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		sessionManager:      sessionManager,
		databaseDiagRepo:    databaseDiagRepo,
		actionRepo:          actionRepo,
		deliveryRepo:        deliveryRepo,
		schedulerRunRepo:    schedulerRunRepo,
		fracIndexRepo:       fracIndexRepo,
		aiRepo:              aiRepo,
		llmManager:          llmManager,
		llmCache:            llmCache,
		auditor:             auditor,
		runnerRepo:          runnerRepo,
		agentRunRepo:        agentRunRepo,
		webhookSender:       webhookSender,
		transitionMatrix:    transitionMatrix,
		bulkOperations:      bulkOperations,
		recurrenceRepo:      recurrenceRepo,
		settingsRepo:        settingsRepo,
		globalRankScheduler: globalRankScheduler,
		memoryBudget:        memoryBudget,
		eventStore:          eventStore,
		scmHealthRepo:       scmHealthRepo,
		channelRepo:         channelRepo,
	}
}

// GetSCMConnections returns durable health for scheduled SCM operations.
//
// GET /api/admin/diagnostics/scm-connections
func (h *DiagnosticsHandler) GetSCMConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := h.scmHealthRepo.ListConnectionDiagnostics(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if connections == nil {
		connections = []repository.SCMConnectionDiagnostic{}
	}
	respondJSONOK(w, connections)
}

// GetEmailIntakeDiagnostics returns machine-generated inbound mail that intake
// deliberately did not turn into a ticket (bounces and auto-replies), plus
// per-category counts for the window. These messages are intentionally absent
// from the work item list; this view is the audit trail that keeps them
// observable.
//
// Query parameters:
//   - since: duration string (default "24h")
//   - limit: max rows (default 25, capped at 200)
//
// GET /api/admin/diagnostics/email-intake
func (h *DiagnosticsHandler) GetEmailIntakeDiagnostics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	since, err := parseSinceDuration(q.Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	limit := 25
	if v := q.Get("limit"); v != "" {
		if parsed, convErr := strconv.Atoi(v); convErr == nil && parsed > 0 {
			limit = parsed
			if limit > 200 {
				limit = 200
			}
		}
	}

	windowStart := time.Now().Add(-since)
	counts, err := h.channelRepo.CountSystemMailByCategory(r.Context(), windowStart)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	messages, err := h.channelRepo.ListSystemMail(r.Context(), windowStart, limit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{
		"since":    windowStart,
		"counts":   counts,
		"messages": messages,
	})
}

// GetCacheMemory returns the configured process budget and live process-local
// cache allocation/traffic counters for this replica.
//
// GET /api/admin/diagnostics/cache-memory
func (h *DiagnosticsHandler) GetCacheMemory(w http.ResponseWriter, _ *http.Request) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	caches := cacheutil.Snapshots()
	var allocatedBytes int64
	var maximumBytes int64
	var evictions uint64
	for _, cache := range caches {
		allocatedBytes += cache.AllocatedCapacityBytes
		maximumBytes += cache.MaximumCapacityBytes
		evictions += cache.NoSpaceEvictions
	}
	heapUtilization := float64(0)
	if h.memoryBudget.GoLimitBytes > 0 {
		heapUtilization = float64(memory.HeapAlloc) / float64(h.memoryBudget.GoLimitBytes) * 100
	}
	respondJSONOK(w, map[string]any{
		"budget":                 h.memoryBudget,
		"heap_alloc_bytes":       memory.HeapAlloc,
		"heap_in_use_bytes":      memory.HeapInuse,
		"runtime_system_bytes":   memory.Sys,
		"process_rss_bytes":      processResidentBytes(),
		"cgroup_memory":          readCgroupMemory(),
		"next_gc_bytes":          memory.NextGC,
		"gc_count":               memory.NumGC,
		"gc_cpu_fraction":        memory.GCCPUFraction,
		"gc_pause_total_ns":      memory.PauseTotalNs,
		"gc_pause_max_ns":        maximumGCPause(memory),
		"heap_limit_utilization": heapUtilization,
		"allocated_cache_bytes":  allocatedBytes,
		"maximum_cache_bytes":    maximumBytes,
		"no_space_evictions":     evictions,
		"caches":                 caches,
		"healthy":                heapUtilization < 90,
		"sampled_at":             time.Now().UTC(),
	})
}

func maximumGCPause(memory runtime.MemStats) uint64 {
	count := min(uint32(len(memory.PauseNs)), memory.NumGC)
	var maximum uint64
	for index := range count {
		pause := memory.PauseNs[index]
		if pause > maximum {
			maximum = pause
		}
	}
	return maximum
}

type cgroupMemorySnapshot struct {
	Available     bool   `json:"available"`
	CurrentBytes  uint64 `json:"current_bytes"`
	PeakBytes     uint64 `json:"peak_bytes"`
	LimitBytes    uint64 `json:"limit_bytes"`
	OOMEvents     uint64 `json:"oom_events"`
	OOMKillEvents uint64 `json:"oom_kill_events"`
}

func processResidentBytes() uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	// The page size is a positive, platform-provided value; residentPages is
	// parsed as uint64 above so the multiplication cannot use a signed value.
	return residentPages * uint64(os.Getpagesize()) //nolint:gosec // page size is platform-provided and positive
}

func readCgroupMemory() cgroupMemorySnapshot {
	const root = "/sys/fs/cgroup/"
	current, currentOK := readCgroupUint(root + "memory.current")
	peak, _ := readCgroupUint(root + "memory.peak")
	limit, _ := readCgroupUint(root + "memory.max")
	snapshot := cgroupMemorySnapshot{
		Available:    currentOK,
		CurrentBytes: current,
		PeakBytes:    peak,
		LimitBytes:   limit,
	}
	events, err := os.ReadFile(root + "memory.events")
	if err != nil {
		return snapshot
	}
	for _, line := range strings.Split(string(events), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "oom":
			snapshot.OOMEvents = value
		case "oom_kill":
			snapshot.OOMKillEvents = value
		}
	}
	return snapshot
}

func readCgroupUint(path string) (uint64, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // callers pass fixed cgroup metric paths
	if err != nil {
		return 0, false
	}
	value := strings.TrimSpace(string(data))
	if value == "max" {
		return 0, true
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return parsed, err == nil
}

const (
	recurrenceVolumeDiagnosticEnabledKey = "recurrence_volume_diagnostic_enabled"
	recurrenceVolumeWarningThresholdKey  = "recurrence_volume_warning_threshold"
	recurrenceVolumeDefaultThreshold     = 80
)

type recurrenceVolumeWorkspace struct {
	repository.RecurrenceWorkspaceVolume
	Warning    bool `json:"warning"`
	AtCapacity bool `json:"at_capacity"`
}

type recurrenceVolumeSnapshot struct {
	DiagnosticEnabled bool                        `json:"diagnostic_enabled"`
	WarningThreshold  int                         `json:"warning_threshold"`
	HardLimit         int                         `json:"hard_limit"`
	SchedulerBatch    int                         `json:"scheduler_batch_size"`
	TotalRules        int                         `json:"total_rules"`
	ActiveRules       int                         `json:"active_rules"`
	DueRules          int                         `json:"due_rules"`
	BatchBacklogged   bool                        `json:"batch_backlogged"`
	Healthy           bool                        `json:"healthy"`
	Workspaces        []recurrenceVolumeWorkspace `json:"workspaces"`
}

type recurrenceVolumeSettingsRequest struct {
	DiagnosticEnabled bool `json:"diagnostic_enabled"`
	WarningThreshold  int  `json:"warning_threshold"`
}

func (h *DiagnosticsHandler) recurrenceVolumeSettings() (enabled bool, threshold int, err error) {
	enabled = true
	threshold = recurrenceVolumeDefaultThreshold
	if value, ok, err := h.settingsRepo.GetValue(recurrenceVolumeDiagnosticEnabledKey); err != nil {
		return false, 0, err
	} else if ok {
		enabled = strings.EqualFold(value, "true")
	}
	if value, ok, err := h.settingsRepo.GetValue(recurrenceVolumeWarningThresholdKey); err != nil {
		return false, 0, err
	} else if ok {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil &&
			parsed >= 1 && parsed <= services.MaxRecurrenceRulesPerWorkspace {
			threshold = parsed
		}
	}
	return enabled, threshold, nil
}

// GetRecurrenceVolume returns persisted recurrence cardinality and the
// administrator-controlled warning state.
//
// GET /api/admin/diagnostics/recurrence-volume
func (h *DiagnosticsHandler) GetRecurrenceVolume(w http.ResponseWriter, r *http.Request) {
	enabled, threshold, err := h.recurrenceVolumeSettings()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	volumes, err := h.recurrenceRepo.ListWorkspaceVolumes()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	dueRules, err := h.recurrenceRepo.CountRulesDueForGeneration()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	snapshot := recurrenceVolumeSnapshot{
		DiagnosticEnabled: enabled,
		WarningThreshold:  threshold,
		HardLimit:         services.MaxRecurrenceRulesPerWorkspace,
		SchedulerBatch:    scheduler.DefaultRecurrenceBatchSize,
		DueRules:          dueRules,
		BatchBacklogged:   dueRules > scheduler.DefaultRecurrenceBatchSize,
		Healthy:           true,
		Workspaces:        make([]recurrenceVolumeWorkspace, 0, len(volumes)),
	}
	for _, volume := range volumes {
		workspace := recurrenceVolumeWorkspace{
			RecurrenceWorkspaceVolume: volume,
			Warning:                   enabled && volume.RuleCount >= threshold,
			AtCapacity:                volume.RuleCount >= services.MaxRecurrenceRulesPerWorkspace,
		}
		snapshot.TotalRules += volume.RuleCount
		snapshot.ActiveRules += volume.ActiveCount
		if workspace.Warning {
			snapshot.Healthy = false
		}
		snapshot.Workspaces = append(snapshot.Workspaces, workspace)
	}
	if enabled && snapshot.BatchBacklogged {
		snapshot.Healthy = false
	}
	respondJSONOK(w, snapshot)
}

// UpdateRecurrenceVolumeSettings updates the diagnostic only; the hard quota
// is intentionally not administrator-configurable.
//
// PUT /api/admin/diagnostics/recurrence-volume
func (h *DiagnosticsHandler) UpdateRecurrenceVolumeSettings(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[recurrenceVolumeSettingsRequest](w, r)
	if !ok {
		return
	}
	if req.WarningThreshold < 1 || req.WarningThreshold > services.MaxRecurrenceRulesPerWorkspace {
		respondValidationError(w, r, fmt.Sprintf(
			"warning_threshold must be between 1 and %d",
			services.MaxRecurrenceRulesPerWorkspace,
		))
		return
	}
	if err := h.settingsRepo.Upsert(
		recurrenceVolumeDiagnosticEnabledKey,
		strconv.FormatBool(req.DiagnosticEnabled),
		"boolean",
		"Enable recurrence rule volume warnings in system diagnostics",
		"diagnostics",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := h.settingsRepo.Upsert(
		recurrenceVolumeWarningThresholdKey,
		strconv.Itoa(req.WarningThreshold),
		"integer",
		"Recurrence rules per workspace that trigger an administrator warning",
		"diagnostics",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		h.auditor.LogWithDetails(
			r,
			user,
			logger.ActionDiagnosticsRecurrenceVolumeUpdate,
			logger.ResourceDiagnostics,
			nil,
			"recurrence_volume",
			map[string]any{
				"diagnostic_enabled": req.DiagnosticEnabled,
				"warning_threshold":  req.WarningThreshold,
			},
		)
	}
	respondJSONOK(w, req)
}

// GetBulkOperations returns bounded bulk-edit and iteration-completion
// cardinality, SQL, latency, pool, failure, and side-effect counters.
//
// GET /api/admin/diagnostics/bulk-operations
func (h *DiagnosticsHandler) GetBulkOperations(w http.ResponseWriter, _ *http.Request) {
	if h.bulkOperations == nil {
		respondJSONOK(w, services.BulkOperationStats{Operations: map[string]services.BulkOperationKindStats{}})
		return
	}
	respondJSONOK(w, h.bulkOperations.Stats())
}

// GetWebhookDispatch returns process-local bounded-pipeline pressure and
// lifetime counters. Delivery history remains on the persisted endpoints.
//
// GET /api/admin/diagnostics/webhook-dispatch
func (h *DiagnosticsHandler) GetWebhookDispatch(w http.ResponseWriter, _ *http.Request) {
	if h.webhookSender == nil {
		respondJSONOK(w, webhook.DispatchStats{})
		return
	}
	respondJSONOK(w, h.webhookSender.Stats())
}

// GetTransitionMatrix returns process-local matrix cardinality, bounded-query,
// coalescing, latency, and response-size counters.
//
// GET /api/admin/diagnostics/transition-matrix
func (h *DiagnosticsHandler) GetTransitionMatrix(w http.ResponseWriter, _ *http.Request) {
	if h.transitionMatrix == nil {
		respondJSONOK(w, services.TransitionMatrixStats{})
		return
	}
	respondJSONOK(w, h.transitionMatrix.Stats())
}

// GetSessionValidationCache returns local, identifier-free validation cache
// counters. In a multi-instance deployment each replica reports its own view.
//
// GET /api/admin/diagnostics/session-validation-cache
func (h *DiagnosticsHandler) GetSessionValidationCache(w http.ResponseWriter, _ *http.Request) {
	stats := h.sessionManager.SessionValidationCacheStats()
	respondJSONOK(w, map[string]any{
		"enabled":               stats.Enabled,
		"ttl_ms":                stats.TTL.Milliseconds(),
		"entries":               stats.Entries,
		"hits":                  stats.Hits,
		"misses":                stats.Misses,
		"database_loads":        stats.DatabaseLoads,
		"coalesced_waiters":     stats.CoalescedWaiters,
		"invalidated_entries":   stats.InvalidatedEntries,
		"evicted_entries":       stats.EvictedEntries,
		"stale_rejections":      stats.StaleRejections,
		"cache_decode_failures": stats.CacheDecodeFailures,
	})
}

// DatabasePoolSnapshot is the HTTP representation of database pool state.
type DatabasePoolSnapshot struct {
	Name                   string  `json:"name"`
	Driver                 string  `json:"driver"`
	MaxOpenConnections     int     `json:"max_open_connections"`
	OpenConnections        int     `json:"open_connections"`
	InUse                  int     `json:"in_use"`
	Idle                   int     `json:"idle"`
	WaitCount              int64   `json:"wait_count"`
	WaitDurationMillis     int64   `json:"wait_duration_ms"`
	MaxIdleClosed          int64   `json:"max_idle_closed"`
	MaxIdleTimeClosed      int64   `json:"max_idle_time_closed"`
	MaxLifetimeClosed      int64   `json:"max_lifetime_closed"`
	UtilizationPercent     float64 `json:"utilization_percent"`
	Saturated              bool    `json:"saturated"`
	SaturationThresholdPct int     `json:"saturation_threshold_percent"`
}

type DatabaseCapacityBudgetSnapshot struct {
	ServerMaxConnections           int     `json:"server_max_connections"`
	MainConnectionsPerReplica      int     `json:"main_connections_per_replica"`
	AuxiliaryConnectionsPerReplica int     `json:"auxiliary_connections_per_replica"`
	ConnectionsPerReplica          int     `json:"connections_per_replica"`
	ReplicaCount                   int     `json:"replica_count"`
	HeadroomConnections            int     `json:"headroom_connections"`
	RequiredConnections            int     `json:"required_connections"`
	RemainingConnections           int     `json:"remaining_connections"`
	UtilizationPercent             float64 `json:"utilization_percent"`
	Safe                           bool    `json:"safe"`
}

type DatabaseProcessSnapshot struct {
	Goroutines     int    `json:"goroutines"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapInUseBytes uint64 `json:"heap_in_use_bytes"`
	SystemBytes    uint64 `json:"system_bytes"`
}

const databasePoolSaturationThresholdPercent = 90

// GetDatabasePool returns the live database/sql pool state so operators can
// distinguish connection starvation from query execution latency.
//
// GET /api/admin/diagnostics/database-pool
func (h *DiagnosticsHandler) GetDatabasePool(w http.ResponseWriter, _ *http.Request) {
	stats := h.databaseDiagRepo.PoolStats()
	pools := make([]DatabasePoolSnapshot, 0, len(stats))
	healthy := true
	var mainPool *DatabasePoolSnapshot
	for _, poolStats := range stats {
		utilization := 0.0
		if poolStats.MaxOpenConnections > 0 {
			utilization = float64(poolStats.InUse) / float64(poolStats.MaxOpenConnections) * 100
		}
		snapshot := DatabasePoolSnapshot{
			Name:                   poolStats.Name,
			Driver:                 poolStats.Driver,
			MaxOpenConnections:     poolStats.MaxOpenConnections,
			OpenConnections:        poolStats.OpenConnections,
			InUse:                  poolStats.InUse,
			Idle:                   poolStats.Idle,
			WaitCount:              poolStats.WaitCount,
			WaitDurationMillis:     poolStats.WaitDurationMillis,
			MaxIdleClosed:          poolStats.MaxIdleClosed,
			MaxIdleTimeClosed:      poolStats.MaxIdleTimeClosed,
			MaxLifetimeClosed:      poolStats.MaxLifetimeClosed,
			UtilizationPercent:     utilization,
			Saturated:              utilization >= databasePoolSaturationThresholdPercent,
			SaturationThresholdPct: databasePoolSaturationThresholdPercent,
		}
		if snapshot.Saturated {
			healthy = false
		}
		pools = append(pools, snapshot)
		if snapshot.Name == "main" {
			copyOfSnapshot := snapshot
			mainPool = &copyOfSnapshot
		}
	}

	var capacity *DatabaseCapacityBudgetSnapshot
	if budget := h.databaseDiagRepo.CapacityBudget(); budget != nil {
		capacity = &DatabaseCapacityBudgetSnapshot{
			ServerMaxConnections:           budget.ServerMaxConnections,
			MainConnectionsPerReplica:      budget.MainConnectionsPerReplica,
			AuxiliaryConnectionsPerReplica: budget.AuxiliaryConnectionsPerReplica,
			ConnectionsPerReplica:          budget.ConnectionsPerReplica,
			ReplicaCount:                   budget.ReplicaCount,
			HeadroomConnections:            budget.HeadroomConnections,
			RequiredConnections:            budget.RequiredConnections,
			RemainingConnections:           budget.RemainingConnections,
			UtilizationPercent:             budget.UtilizationPercent,
			Safe:                           budget.Safe,
		}
		if !capacity.Safe {
			healthy = false
		}
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	respondJSONOK(w, map[string]any{
		// pool is retained for existing load-test clients; new consumers should
		// use pools so auxiliary process-local pools are visible too.
		"pool":     mainPool,
		"pools":    pools,
		"capacity": capacity,
		"process": DatabaseProcessSnapshot{
			Goroutines:     runtime.NumGoroutine(),
			HeapAllocBytes: memory.HeapAlloc,
			HeapInUseBytes: memory.HeapInuse,
			SystemBytes:    memory.Sys,
		},
		"request_query_outcomes": h.databaseDiagRepo.RequestQueryStats(),
		"instance":               hostname,
		"sampled_at":             time.Now().UTC(),
		"healthy":                healthy,
	})
}

// GetFracIndexState returns a snapshot of persisted items.frac_index state
// for the admin diagnostics panel. "healthy" is false when the column
// collation diverges from byte ordering or when the next key the generator
// would produce already exists in the table.
//
// GET /api/admin/diagnostics/frac-index
func (h *DiagnosticsHandler) GetFracIndexState(w http.ResponseWriter, r *http.Request) {
	dbState, err := h.fracIndexRepo.GetDBStats()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Compute what the next append would produce so the panel can flag
	// in-flight collisions even though the in-process generator no longer
	// caches a predicted key.
	predicted, perr := nextAppendKey(dbState.ByteMax)
	if perr != nil {
		respondInternalError(w, r, perr)
		return
	}
	if predicted != "" {
		p := predicted
		dbState.PredictedNext = &p
		collision, cerr := h.fracIndexRepo.ProbePredictedKey(predicted)
		if cerr != nil {
			respondInternalError(w, r, cerr)
			return
		}
		dbState.PredictedCollision = collision
	}

	globalState, err := h.fracIndexRepo.GetGlobalRankState()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	integrity, err := h.fracIndexRepo.GetGlobalRankIntegrity(globalState, time.Now().UTC())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lengthHealthy := dbState.OverlongRankCount == 0 ||
		globalState.Phase == repository.GlobalRankPhaseMigrating ||
		globalState.Phase == repository.GlobalRankPhasePaused
	healthy := !dbState.CollationMismatch && dbState.PredictedCollision == nil && lengthHealthy && integrity.Healthy
	respondJSONOK(w, map[string]any{
		"db":        dbState,
		"migration": globalState,
		"integrity": integrity,
		"healthy":   healthy,
	})
}

type GlobalRankMigrationControlRequest struct {
	Action string `json:"action"`
}

// ControlGlobalRankMigration starts, pauses, resumes, or resets the bounded
// online migration. The route is system-admin-only and every successful action
// is audited.
//
// POST /api/admin/diagnostics/frac-index/migration
func (h *DiagnosticsHandler) ControlGlobalRankMigration(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[GlobalRankMigrationControlRequest](w, r)
	if !ok {
		return
	}
	action := repository.GlobalRankMigrationAction(strings.ToLower(strings.TrimSpace(req.Action)))
	switch action {
	case repository.GlobalRankMigrationStart, repository.GlobalRankMigrationPause, repository.GlobalRankMigrationResume, repository.GlobalRankMigrationReset:
	default:
		respondValidationError(w, r, "action must be 'start', 'pause', 'resume', or 'reset'")
		return
	}

	state, err := h.fracIndexRepo.ControlGlobalRankMigration(r.Context(), action)
	if errors.Is(err, repository.ErrGlobalRankMigrationConflict) {
		respondConflict(w, r, err.Error())
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if h.globalRankScheduler != nil && (action == repository.GlobalRankMigrationStart || action == repository.GlobalRankMigrationResume) {
		h.globalRankScheduler.Wake()
	}
	if h.auditor != nil {
		if user := utils.GetCurrentUser(r); user != nil {
			h.auditor.LogWithDetails(r, user, logger.ActionDiagnosticsGlobalRankMigrationControl, logger.ResourceDiagnostics, nil, "global rank migration", map[string]any{
				"action":        action,
				"active_bucket": state.ActiveBucket,
				"target_bucket": state.TargetBucket,
				"phase":         state.Phase,
			})
		}
	}
	respondJSONOK(w, map[string]any{"migration": state})
}

// nextAppendKey mirrors the deterministic base of GenerateFracIndexForNewItem's
// "append after current max" step: it runs KeyBetween over the supplied
// byte-wise max. The real generator appends a random jitter suffix to this base
// (so concurrent appends don't collide), meaning the exact base is essentially
// never inserted verbatim — so a positive ProbePredictedKey hit on it would
// signal a genuine ordering anomaly rather than a normal append. Empty when
// there are no rows (the generator would seed with KeyBetween("", "")).
func nextAppendKey(byteMax *string) (string, error) {
	last := ""
	if byteMax == nil {
		return repository.KeyBetween(last, "")
	}

	last = *byteMax
	rank, err := repository.ParseGlobalRank(last)
	if err != nil {
		// Legacy databases are still observable while the checkpoint converter
		// is running, so retain the pre-checkpoint calculation for bare keys.
		return repository.KeyBetween(last, "")
	}

	nextFraction, err := repository.KeyBetween(rank.Fraction, "")
	if err != nil {
		return "", err
	}
	return repository.EncodeGlobalRank(rank.Bucket, nextFraction)
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
//   - scheduler: "" (any), "briefing", "email", "email_outbox", "recurrence", "notification"
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
	h.auditor.LogWithDetails(r, user, action, logger.ResourceDiagnostics, nil, "", map[string]any{
		"older_than": olderThan,
		"cutoff":     cutoff.Format(time.RFC3339),
		"deleted":    rows,
	})
}

// LLMProviderConnectionStatus pairs an enabled connection with whether its
// configured model is still present in the provider's cached catalog.
//
// ModelStillInCatalog is nil when the catalog hasn't been refreshed yet
// (the UI then shows "unknown — refresh to check") and false when the
// model has dropped — the drift signal that surfaced the Gemini deprecation
// in the first place.
type LLMProviderConnectionStatus struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	Model               string `json:"model"`
	ModelStillInCatalog *bool  `json:"model_still_in_catalog,omitempty"`
}

// LLMProviderStatus is one row in the diagnostics widget: provider metadata,
// the cache state, and the list of enabled connections (with drift flags).
type LLMProviderStatus struct {
	Type              llm.ProviderType              `json:"type"`
	Name              string                        `json:"name"`
	HasDynamicModels  bool                          `json:"has_dynamic_models"`
	LastRefreshedAt   *time.Time                    `json:"last_refreshed_at,omitempty"`
	LastError         string                        `json:"last_error,omitempty"`
	ModelsCachedCount int                           `json:"models_cached_count"`
	Connections       []LLMProviderConnectionStatus `json:"connections"`
}

// GetLLMProviderStatus returns per-provider catalog cache state plus an
// enabled-connection drift check (configured model present in the cached
// catalog?). This is the System Diagnostics counterpart to the per-provider
// rows already shown in Settings → LLM Connections — same data, but explicitly
// surfaces drift instead of expecting admins to spot it.
//
// GET /api/admin/diagnostics/llm-providers
func (h *DiagnosticsHandler) GetLLMProviderStatus(w http.ResponseWriter, r *http.Request) {
	if h.llmManager == nil || h.llmCache == nil {
		respondInternalError(w, r, fmt.Errorf("llm dependencies not configured"))
		return
	}

	connections, err := h.llmManager.ListConnections()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	byProvider := make(map[llm.ProviderType][]llm.ConnectionInfo, len(connections))
	for _, c := range connections {
		if !c.IsEnabled {
			continue
		}
		byProvider[c.ProviderType] = append(byProvider[c.ProviderType], c)
	}

	providers := llm.KnownProviders()
	out := make([]LLMProviderStatus, 0, len(providers))
	for _, p := range providers {
		entry := LLMProviderStatus{
			Type:             p.Type,
			Name:             p.Name,
			HasDynamicModels: p.HasDynamicModels(),
			Connections:      []LLMProviderConnectionStatus{},
		}
		var cachedIDs map[string]struct{}
		if p.HasDynamicModels() {
			cached, cerr := h.llmCache.Get(p.Type)
			if cerr != nil {
				slog.Warn("read model cache", slog.String("provider", string(p.Type)), slog.Any("error", cerr))
			} else {
				entry.LastRefreshedAt = cached.LastRefreshedAt
				entry.LastError = cached.LastError
				entry.ModelsCachedCount = len(cached.Models)
				if cached.LastRefreshedAt != nil {
					cachedIDs = make(map[string]struct{}, len(cached.Models))
					for _, m := range cached.Models {
						cachedIDs[m.ID] = struct{}{}
					}
				}
			}
		}
		for _, c := range byProvider[p.Type] {
			cs := LLMProviderConnectionStatus{ID: c.ID, Name: c.Name, Model: c.Model}
			if cachedIDs != nil {
				_, ok := cachedIDs[c.Model]
				cs.ModelStillInCatalog = &ok
			}
			entry.Connections = append(entry.Connections, cs)
		}
		out = append(out, entry)
	}
	respondJSONOK(w, out)
}

// BriefingFailureBucket counts failed briefings under one error class.
type BriefingFailureBucket struct {
	Class         string `json:"class"`
	Count         int    `json:"count"`
	LatestMessage string `json:"latest_message,omitempty"`
}

// BriefingFailureRow is one row in the recent-failures table, paired with its
// classifier verdict so the frontend can render badges without re-classifying.
type BriefingFailureRow struct {
	ID           int    `json:"id"`
	UserID       int    `json:"user_id"`
	Date         string `json:"date"`
	Error        string `json:"error"`
	ClassifiedAs string `json:"classified_as"`
	CreatedAt    string `json:"created_at"`
}

// GetBriefingFailures returns recent failed daily_briefings rows bucketed by
// classifier verdict. The user reported a Gemini-deprecation 404 buried in
// scheduler logs — this surfaces those buckets in the admin UI.
//
// Query params:
//   - since: duration string (default "24h")
//
// GET /api/admin/diagnostics/briefing-failures
func (h *DiagnosticsHandler) GetBriefingFailures(w http.ResponseWriter, r *http.Request) {
	if h.aiRepo == nil {
		respondInternalError(w, r, fmt.Errorf("ai repository not configured"))
		return
	}
	since, err := parseSinceDuration(r.URL.Query().Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	rows, err := h.aiRepo.ListFailedBriefings(time.Now().Add(-since), 100)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Stable bucket order so the frontend can render a fixed grid even when
	// some classes have zero hits.
	bucketOrder := []llm.ErrorClass{
		llm.ErrorClassModelNotFound,
		llm.ErrorClassAuthFailed,
		llm.ErrorClassRateLimited,
		llm.ErrorClassServerError,
		llm.ErrorClassConnectionFailed,
		llm.ErrorClassOther,
	}
	buckets := make(map[llm.ErrorClass]*BriefingFailureBucket, len(bucketOrder))
	for _, c := range bucketOrder {
		buckets[c] = &BriefingFailureBucket{Class: string(c)}
	}

	recent := make([]BriefingFailureRow, 0, len(rows))
	for i, row := range rows {
		cls := llm.ClassifyError(row.Error)
		b := buckets[cls]
		b.Count++
		if b.LatestMessage == "" {
			b.LatestMessage = row.Error
		}
		if i < 25 {
			recent = append(recent, BriefingFailureRow{
				ID:           row.ID,
				UserID:       row.UserID,
				Date:         row.Date,
				Error:        row.Error,
				ClassifiedAs: string(cls),
				CreatedAt:    row.CreatedAt,
			})
		}
	}

	bucketList := make([]BriefingFailureBucket, 0, len(bucketOrder))
	for _, c := range bucketOrder {
		bucketList = append(bucketList, *buckets[c])
	}

	respondJSONOK(w, map[string]any{
		"since":   since.String(),
		"buckets": bucketList,
		"recent":  recent,
	})
}

// BriefingFeatureStatus describes how the daily_briefing AI feature resolves.
// A disabled feature or a pinned connection that no longer exists makes the
// scheduler skip every user without writing a row, so the failure-only view
// cannot see it.
type BriefingFeatureStatus struct {
	Mode         string `json:"mode"`
	Schedule     string `json:"schedule"`
	ConnectionID int    `json:"connection_id"`
	Resolved     bool   `json:"resolved"`
	Available    bool   `json:"available"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	UsedFallback bool   `json:"used_fallback"`
	Error        string `json:"error,omitempty"`
}

// BriefingCoverage counts the latest briefing state per active user inside the
// diagnostic window. Missing is active users with no row at all.
type BriefingCoverage struct {
	UsersTotal        int `json:"users_total"`
	Generated         int `json:"generated"`
	Empty             int `json:"empty"`
	Failed            int `json:"failed"`
	Stuck             int `json:"stuck"`
	WithoutProvenance int `json:"without_provenance"`
	Missing           int `json:"missing"`
}

// BriefingHealthRow is one user's latest briefing state for the diagnostic UI.
type BriefingHealthRow struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Date          string `json:"date"`
	Status        string `json:"status"`
	ContentLength int    `json:"content_length"`
	Error         string `json:"error,omitempty"`
	HasProvenance bool   `json:"has_provenance"`
	LockHeld      bool   `json:"lock_held"`
	UpdatedAt     string `json:"updated_at"`
}

// classifyBriefingStatus reduces a stored briefing row to the state an admin
// cares about. Order matters: an explicit error wins, then real content, then
// an in-flight lease, and an empty released row is the "no context" case.
func classifyBriefingStatus(contentLength int, errMsg string, lockHeld bool) string {
	switch {
	case errMsg != "":
		return "failed"
	case contentLength > 0:
		return "generated"
	case lockHeld:
		return "stuck"
	default:
		return "empty"
	}
}

// GetBriefingHealth returns the daily-briefing generation state: feature
// resolution, the last scheduler tick, per-user coverage, and the latest row
// per user. The failure-only view answers "which LLM errors happened"; this
// answers "is the scheduler producing briefings at all, and why not".
//
// Query params:
//   - since: duration string (default "24h")
//   - limit: max per-user rows (default 100, capped at 500)
//
// GET /api/admin/diagnostics/briefing-health
func (h *DiagnosticsHandler) GetBriefingHealth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := parseSinceDuration(q.Get("since"), 24*time.Hour)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}

	// Coverage is computed over every user with a row, so a large instance is
	// not undercounted by the display limit; only the recent table is capped.
	rows, err := h.aiRepo.ListLatestBriefingHealthRows(time.Now().Add(-since), time.Now(), 5000)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	usersTotal, err := h.aiRepo.CountActiveUsers()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	coverage := BriefingCoverage{UsersTotal: usersTotal}
	recent := make([]BriefingHealthRow, 0, len(rows))
	for _, row := range rows {
		status := classifyBriefingStatus(row.ContentLength, row.Error, row.LockHeld)
		switch status {
		case "generated":
			coverage.Generated++
			if !row.HasProvenance {
				coverage.WithoutProvenance++
			}
		case "failed":
			coverage.Failed++
		case "stuck":
			coverage.Stuck++
		default:
			coverage.Empty++
		}
		recent = append(recent, BriefingHealthRow{
			ID:            row.ID,
			UserID:        row.UserID,
			Date:          row.Date,
			Status:        status,
			ContentLength: row.ContentLength,
			Error:         row.Error,
			HasProvenance: row.HasProvenance,
			LockHeld:      row.LockHeld,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	coverage.Missing = usersTotal - len(rows)
	if coverage.Missing < 0 {
		coverage.Missing = 0
	}
	if len(recent) > limit {
		recent = recent[:limit]
	}

	respondJSONOK(w, map[string]any{
		"since":    since.String(),
		"feature":  h.briefingFeatureStatus(),
		"last_run": h.lastBriefingRun(),
		"coverage": coverage,
		"recent":   recent,
	})
}

// briefingFeatureStatus reports the configured daily_briefing policy and the
// connection the scheduler would actually use.
func (h *DiagnosticsHandler) briefingFeatureStatus() BriefingFeatureStatus {
	status := BriefingFeatureStatus{Mode: string(models.AIFeatureModeDefault), Schedule: "daily"}
	if h.settingsRepo != nil {
		if raw, ok, err := h.settingsRepo.GetValue("ai_feature_config"); err == nil && ok {
			var cfg models.AIFeaturesConfig
			if json.Unmarshal([]byte(raw), &cfg) == nil {
				fc := cfg["daily_briefing"]
				if fc.Mode != "" {
					status.Mode = string(fc.Mode)
				}
				if fc.Schedule != "" {
					status.Schedule = fc.Schedule
				}
				status.ConnectionID = fc.ConnectionID
			}
		}
	}
	client, descriptor, err := h.llmManager.ResolveForFeatureWithIdentity("daily_briefing", 0)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Resolved = true
	status.Available = client != nil && client.Available()
	status.Provider = string(descriptor.ProviderType)
	status.Model = descriptor.Model
	status.UsedFallback = descriptor.UsedFallback
	return status
}

// lastBriefingRun returns the most recent briefing scheduler tick, or nil when
// the scheduler has not recorded one in the retained window.
func (h *DiagnosticsHandler) lastBriefingRun() *models.SchedulerRun {
	runs, err := h.schedulerRunRepo.GetRecent(repository.RecentSchedulerRunsOpts{Scheduler: "briefing", Limit: 1})
	if err != nil || len(runs) == 0 {
		return nil
	}
	return runs[0]
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
		const maxDays = int64(1<<63-1) / int64(24*time.Hour)
		if int64(n) > maxDays || int64(n) < -maxDays {
			return 0, fmt.Errorf("duration out of range: %s", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
