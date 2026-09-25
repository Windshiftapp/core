package services

import (
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// IncidentEscalationSweeperConfig configures the background ticker that drives
// time-based escalation for triggered incidents.
type IncidentEscalationSweeperConfig struct {
	TickInterval time.Duration // how often to scan for due incidents; default 30s
	BatchSize    int           // max incidents to advance per tick; default 50
}

// DefaultIncidentEscalationSweeperConfig returns sensible defaults.
func DefaultIncidentEscalationSweeperConfig() IncidentEscalationSweeperConfig {
	return IncidentEscalationSweeperConfig{
		TickInterval: 30 * time.Second,
		BatchSize:    50,
	}
}

// IncidentEscalationSweeper periodically advances incidents whose
// next_escalation_at has passed. It shares IncidentService.AdvanceDue with the
// inline step-0 path so the cursor transition is identical.
type IncidentEscalationSweeper struct {
	repo     *repository.OnCallRepository
	incident *IncidentService
	config   IncidentEscalationSweeperConfig
	stopChan chan struct{}
	wg       sync.WaitGroup

	ticksProcessed    int64
	stepsAdvanced     int64
	notificationsSent int64
	errors            int64
}

// NewIncidentEscalationSweeper constructs the sweeper. Call Start() to begin.
func NewIncidentEscalationSweeper(db database.Database, incident *IncidentService, config IncidentEscalationSweeperConfig) *IncidentEscalationSweeper {
	if config.TickInterval == 0 {
		config.TickInterval = 30 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 50
	}
	return &IncidentEscalationSweeper{
		repo:     repository.NewOnCallRepository(db),
		incident: incident,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start launches the background worker. Idempotent — calling Start twice is a no-op.
func (s *IncidentEscalationSweeper) Start() {
	s.wg.Add(1)
	go s.run()
	slog.Debug("incident escalation sweeper started",
		slog.String("component", "oncall"),
		slog.Duration("tick_interval", s.config.TickInterval),
		slog.Int("batch_size", s.config.BatchSize),
	)
}

// Stop signals shutdown and waits for the worker to drain.
func (s *IncidentEscalationSweeper) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

func (s *IncidentEscalationSweeper) run() {
	defer s.wg.Done()
	t := time.NewTicker(s.config.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-t.C:
			s.tick()
		}
	}
}

// tick runs a single sweep pass. It advances due escalation steps and then
// delivers due delayed/repeated notifications. Per-row failures are logged and
// skipped.
func (s *IncidentEscalationSweeper) tick() {
	s.ticksProcessed++
	now := time.Now()

	dueIDs, err := s.repo.FindDueIncidentIDs(now, s.config.BatchSize)
	if err != nil {
		s.errors++
		slog.Warn("incident sweeper: failed to query due incidents",
			slog.String("component", "oncall"), slog.Any("error", err))
	} else {
		for _, id := range dueIDs {
			if err := s.incident.AdvanceDue(id); err != nil {
				s.errors++
				slog.Warn("incident sweeper: escalation failed",
					slog.String("component", "oncall"),
					slog.Int("incident_id", id),
					slog.Any("error", err),
				)
				continue
			}
			s.stepsAdvanced++
		}
	}

	stateIDs, err := s.repo.FindDueNotificationStateIDs(now, s.config.BatchSize)
	if err != nil {
		s.errors++
		slog.Warn("incident sweeper: failed to query scheduled notifications",
			slog.String("component", "oncall"), slog.Any("error", err))
		return
	}
	for _, id := range stateIDs {
		if err := s.incident.DispatchDueNotification(id); err != nil {
			s.errors++
			slog.Warn("incident sweeper: scheduled notification failed",
				slog.String("component", "oncall"),
				slog.Int("notification_state_id", id),
				slog.Any("error", err),
			)
			continue
		}
		s.notificationsSent++
	}
}
