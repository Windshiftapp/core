package services

import (
	"log/slog"
	"sync"
	"time"

	"windshift/internal/repository"
)

// DatabasePoolStatsSource is implemented by the process-local diagnostics
// repository. Keeping this narrow makes the alert state machine independently
// testable without a live SQL driver.
type DatabasePoolStatsSource interface {
	PoolStats() []repository.DatabasePoolStats
}

type DatabasePoolMonitorConfig struct {
	Interval                   time.Duration
	HighUtilizationPercent     float64
	RecoveryUtilizationPercent float64
	HighSamplesBeforeAlert     int
}

func DefaultDatabasePoolMonitorConfig() DatabasePoolMonitorConfig {
	return DatabasePoolMonitorConfig{
		Interval:                   30 * time.Second,
		HighUtilizationPercent:     90,
		RecoveryUtilizationPercent: 75,
		HighSamplesBeforeAlert:     2,
	}
}

type databasePoolAlertState struct {
	previousWaitCount          int64
	previousWaitDurationMillis int64
	highSamples                int
	alerting                   bool
	initialized                bool
}

type databasePoolAlertEvent struct {
	kind                    string
	utilizationPercent      float64
	waitCountDelta          int64
	waitDurationMillisDelta int64
}

// DatabasePoolMonitor emits low-volume, structured transition logs. Actual
// pool waits alert immediately; utilization alone must remain high across
// consecutive samples. Recovery uses a lower threshold to prevent flapping.
type DatabasePoolMonitor struct {
	source DatabasePoolStatsSource
	config DatabasePoolMonitorConfig

	mu       sync.Mutex
	states   map[string]databasePoolAlertState
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewDatabasePoolMonitor(source DatabasePoolStatsSource, config DatabasePoolMonitorConfig) *DatabasePoolMonitor {
	defaults := DefaultDatabasePoolMonitorConfig()
	if config.Interval <= 0 {
		config.Interval = defaults.Interval
	}
	if config.HighUtilizationPercent <= 0 {
		config.HighUtilizationPercent = defaults.HighUtilizationPercent
	}
	if config.RecoveryUtilizationPercent <= 0 || config.RecoveryUtilizationPercent >= config.HighUtilizationPercent {
		config.RecoveryUtilizationPercent = defaults.RecoveryUtilizationPercent
	}
	if config.HighSamplesBeforeAlert <= 0 {
		config.HighSamplesBeforeAlert = defaults.HighSamplesBeforeAlert
	}
	return &DatabasePoolMonitor{
		source: source,
		config: config,
		states: make(map[string]databasePoolAlertState),
		stop:   make(chan struct{}),
	}
}

func (m *DatabasePoolMonitor) Start() {
	if m == nil || m.source == nil {
		return
	}
	m.wg.Add(1)
	go m.run()
}

func (m *DatabasePoolMonitor) run() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()
	m.sample()
	for {
		select {
		case <-ticker.C:
			m.sample()
		case <-m.stop:
			return
		}
	}
}

func (m *DatabasePoolMonitor) sample() {
	for _, stats := range m.source.PoolStats() {
		key := stats.Key()
		m.mu.Lock()
		state := m.states[key]
		next, event := evaluateDatabasePoolSample(state, stats, m.config)
		m.states[key] = next
		m.mu.Unlock()

		slog.Debug("database pool sample",
			"component", "database_pool",
			"pool", stats.Name,
			"role", stats.Role,
			"driver", stats.Driver,
			"open_connections", stats.OpenConnections,
			"in_use", stats.InUse,
			"idle", stats.Idle,
			"max_open_connections", stats.MaxOpenConnections,
			"wait_count", stats.WaitCount,
			"wait_duration_ms", stats.WaitDurationMillis,
			"utilization_percent", event.utilizationPercent,
		)
		switch event.kind {
		case "waiting":
			slog.Warn("database pool wait detected",
				"component", "database_pool",
				"event", "database_pool_waiting",
				"pool", stats.Name,
				"driver", stats.Driver,
				"in_use", stats.InUse,
				"max_open_connections", stats.MaxOpenConnections,
				"utilization_percent", event.utilizationPercent,
				"wait_count_delta", event.waitCountDelta,
				"wait_duration_ms_delta", event.waitDurationMillisDelta,
			)
		case "saturated":
			slog.Warn("database pool utilization is sustained near capacity",
				"component", "database_pool",
				"event", "database_pool_saturated",
				"pool", stats.Name,
				"driver", stats.Driver,
				"in_use", stats.InUse,
				"max_open_connections", stats.MaxOpenConnections,
				"utilization_percent", event.utilizationPercent,
			)
		case "recovered":
			slog.Info("database pool recovered",
				"component", "database_pool",
				"event", "database_pool_recovered",
				"pool", stats.Name,
				"driver", stats.Driver,
				"in_use", stats.InUse,
				"max_open_connections", stats.MaxOpenConnections,
				"utilization_percent", event.utilizationPercent,
			)
		}
	}
}

func evaluateDatabasePoolSample(
	state databasePoolAlertState,
	stats repository.DatabasePoolStats,
	config DatabasePoolMonitorConfig,
) (databasePoolAlertState, databasePoolAlertEvent) {
	event := databasePoolAlertEvent{
		waitCountDelta:          max(int64(0), stats.WaitCount-state.previousWaitCount),
		waitDurationMillisDelta: max(int64(0), stats.WaitDurationMillis-state.previousWaitDurationMillis),
	}
	if !state.initialized {
		event.waitCountDelta = 0
		event.waitDurationMillisDelta = 0
		state.initialized = true
	}
	if stats.MaxOpenConnections > 0 {
		event.utilizationPercent = float64(stats.InUse) / float64(stats.MaxOpenConnections) * 100
	}
	state.previousWaitCount = stats.WaitCount
	state.previousWaitDurationMillis = stats.WaitDurationMillis

	if event.waitCountDelta > 0 {
		event.kind = "waiting"
		state.alerting = true
	}
	if event.utilizationPercent >= config.HighUtilizationPercent {
		state.highSamples++
		if !state.alerting && state.highSamples >= config.HighSamplesBeforeAlert {
			event.kind = "saturated"
			state.alerting = true
		}
	} else {
		state.highSamples = 0
	}
	if state.alerting && event.waitCountDelta == 0 && event.utilizationPercent <= config.RecoveryUtilizationPercent {
		event.kind = "recovered"
		state.alerting = false
	}
	return state, event
}

func (m *DatabasePoolMonitor) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stop) })
	m.wg.Wait()
}
