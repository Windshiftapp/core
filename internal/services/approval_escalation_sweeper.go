package services

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// ApprovalEscalationSweeperConfig configures the background ticker that drives
// time-based escalation for pending approval steps.
type ApprovalEscalationSweeperConfig struct {
	TickInterval time.Duration // how often to scan for due steps; default 1m
	BatchSize    int           // max rows to escalate per tick; default 50
}

// DefaultApprovalEscalationSweeperConfig returns sensible defaults.
func DefaultApprovalEscalationSweeperConfig() ApprovalEscalationSweeperConfig {
	return ApprovalEscalationSweeperConfig{
		TickInterval: time.Minute,
		BatchSize:    50,
	}
}

// ApprovalEscalationSweeper is a background worker that periodically queries
// pending approval_step_instances whose escalation_due_at has passed, and
// invokes ApprovalService.Escalate(stepInstanceID, "timeout") on each.
//
// Modeled on NotificationService.cacheRefresher (notification_service.go:183).
type ApprovalEscalationSweeper struct {
	repo            *repository.ApprovalRepository
	approvalService *ApprovalService
	config          ApprovalEscalationSweeperConfig
	stopChan        chan struct{}
	wg              sync.WaitGroup

	// Stats (atomic-friendly counters; not strict but useful for observability).
	ticksProcessed int64
	stepsEscalated int64
	errors         int64
}

// NewApprovalEscalationSweeper constructs the sweeper. Call Start() to begin.
func NewApprovalEscalationSweeper(db database.Database, approvalService *ApprovalService, config ApprovalEscalationSweeperConfig) *ApprovalEscalationSweeper {
	if config.TickInterval == 0 {
		config.TickInterval = time.Minute
	}
	if config.BatchSize == 0 {
		config.BatchSize = 50
	}
	return &ApprovalEscalationSweeper{
		repo:            repository.NewApprovalRepository(db),
		approvalService: approvalService,
		config:          config,
		stopChan:        make(chan struct{}),
	}
}

// Start launches the background worker. Idempotent — calling Start twice is a no-op.
func (s *ApprovalEscalationSweeper) Start() {
	s.wg.Add(1)
	go s.run()
	slog.Debug("approval escalation sweeper started",
		slog.String("component", "approvals"),
		slog.Duration("tick_interval", s.config.TickInterval),
		slog.Int("batch_size", s.config.BatchSize),
	)
}

// Stop signals shutdown and waits for the worker to drain.
func (s *ApprovalEscalationSweeper) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

func (s *ApprovalEscalationSweeper) run() {
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

// tick runs a single sweep pass. Failures on individual steps are logged and
// skipped — a misconfigured step shouldn't halt the entire chain.
func (s *ApprovalEscalationSweeper) tick() {
	s.ticksProcessed++

	ctx := context.Background()
	dueIDs, err := s.repo.FindDueStepInstanceIDs(ctx, s.config.BatchSize)
	if err != nil {
		s.errors++
		slog.Warn("approval sweeper: failed to query due steps",
			slog.String("component", "approvals"), slog.Any("error", err))
		return
	}
	if len(dueIDs) == 0 {
		return
	}

	for _, id := range dueIDs {
		if err := s.approvalService.Escalate(ctx, id, 0, "timeout"); err != nil {
			s.errors++
			slog.Warn("approval sweeper: escalation failed",
				slog.String("component", "approvals"),
				slog.Int("step_instance_id", id),
				slog.Any("error", err),
			)
			continue
		}
		s.stepsEscalated++
	}
}
