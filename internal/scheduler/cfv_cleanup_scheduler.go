// Package scheduler — see cfv_cleanup_scheduler.go for the async cleanup
// path used after a custom field is deleted.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// CFVCleanupScheduler drains the pending_custom_field_cleanups queue.
//
// Each row in the queue represents a custom field that was deleted while
// some items still carried the field's key in their custom_field_values
// JSON. Scrubbing those references inline on the Delete request would
// block the user for as long as the workspace has items — potentially
// millions of rows — so the Delete handler enqueues a job here and this
// scheduler processes it in batches.
//
// The scheduler:
//   - Ticks every minute (cheap query when the queue is empty).
//   - Picks the oldest pending job, marks it 'running'.
//   - Iterates items that mention the deleted field's key in batches of
//     batchSize, removes the key from the JSON, writes the row back.
//   - Marks the job 'done' (or 'failed' on error) with row counts.
//
// Best-effort semantics: a crashed process leaves the job in 'running';
// the next tick re-claims any row stuck in 'running' for longer than
// staleThreshold so cleanup eventually completes.
type CFVCleanupScheduler struct {
	db      database.Database
	runRepo *repository.SchedulerRunRepository

	ticker   *time.Ticker
	stopChan chan struct{}
	mu       sync.RWMutex
	running  bool

	// Configuration
	checkInterval  time.Duration
	batchSize      int
	staleThreshold time.Duration // running rows older than this are re-claimed
}

const schedulerName = "cfv_cleanup"

// NewCFVCleanupScheduler builds a scheduler with sensible defaults. The
// caller wires Start/Stop into the same lifecycle as the other in-process
// schedulers (server.go).
func NewCFVCleanupScheduler(db database.Database) *CFVCleanupScheduler {
	return &CFVCleanupScheduler{
		db:             db,
		runRepo:        repository.NewSchedulerRunRepository(db),
		checkInterval:  1 * time.Minute,
		batchSize:      500,
		staleThreshold: 30 * time.Minute,
		stopChan:       make(chan struct{}),
	}
}

// Start begins the cleanup loop. Safe to call multiple times — second
// call is a no-op.
func (s *CFVCleanupScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	s.ticker = time.NewTicker(s.checkInterval)
	slog.Info("starting cfv cleanup scheduler", "interval", s.checkInterval, "batch_size", s.batchSize)
	go s.loop()
}

// Stop halts the scheduler. Safe to call multiple times.
func (s *CFVCleanupScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	s.ticker.Stop()
	close(s.stopChan)
	slog.Info("cfv cleanup scheduler stopped")
}

func (s *CFVCleanupScheduler) loop() {
	// Process immediately on startup so a queued job from a previous
	// process generation doesn't wait the full interval.
	s.tick()
	for {
		select {
		case <-s.ticker.C:
			s.tick()
		case <-s.stopChan:
			return
		}
	}
}

// tick drains as many pending jobs as exist; bounded by claimMaxJobsPerTick
// to keep a stuck queue from monopolizing scheduler resources.
const claimMaxJobsPerTick = 20

func (s *CFVCleanupScheduler) tick() {
	start := time.Now()
	totalItems := 0
	var runErr error
	defer recordSchedulerRun(s.runRepo, schedulerName, start, &totalItems, &runErr)

	// First: rehabilitate stale 'running' rows so a crashed process doesn't
	// strand jobs indefinitely.
	if err := s.requeueStaleRunning(); err != nil {
		slog.Warn("cfv_cleanup: requeue stale failed", "error", err)
		// don't return — we can still try to drain fresh jobs
	}

	for i := 0; i < claimMaxJobsPerTick; i++ {
		jobID, fieldID, claimed, err := s.claimNextJob()
		if err != nil {
			runErr = err
			return
		}
		if !claimed {
			return
		}
		processed, err := s.processJob(fieldID)
		if err != nil {
			s.markFailed(jobID, err.Error())
			runErr = err
			continue
		}
		s.markDone(jobID, processed)
		totalItems += processed
	}
}

func (s *CFVCleanupScheduler) requeueStaleRunning() error {
	cutoff := time.Now().Add(-s.staleThreshold)
	_, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'pending', started_at = NULL
		  WHERE status = 'running' AND started_at < ?`,
		cutoff,
	)
	return err
}

// claimNextJob picks the oldest pending row and flips it to 'running'.
// The (status, created_at) index makes this a constant-time lookup.
// Returns claimed=false when the queue is empty.
//
// On SQLite there are no row locks, so a separate process generation could
// theoretically claim the same row. We use a UPDATE ... WHERE status=
// 'pending' guard so only one transition succeeds; second caller sees
// 'no row updated' and tries the next one.
func (s *CFVCleanupScheduler) claimNextJob() (jobID, fieldID int, claimed bool, err error) {
	row := s.db.QueryRow(
		`SELECT id, field_id FROM pending_custom_field_cleanups
		  WHERE status = 'pending'
		  ORDER BY created_at ASC
		  LIMIT 1`,
	)
	if err = row.Scan(&jobID, &fieldID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Empty queue — caller exits the drain loop normally.
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}

	now := time.Now()
	res, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'running', started_at = ?
		  WHERE id = ? AND status = 'pending'`,
		now, jobID,
	)
	if err != nil {
		return 0, 0, false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Someone else claimed it between our SELECT and UPDATE — try the
		// next call.
		return 0, 0, false, nil
	}
	return jobID, fieldID, true, nil
}

// processJob scrubs every item whose cfv JSON contains the deleted
// field's key. Iterates in batches keyed by item id; bounded memory.
func (s *CFVCleanupScheduler) processJob(fieldID int) (int, error) {
	fieldKey := strconv.Itoa(fieldID)
	totalProcessed := 0
	lastID := 0

	for {
		rows, err := s.db.Query(
			`SELECT id, custom_field_values
			   FROM items
			  WHERE id > ?
			    AND custom_field_values IS NOT NULL
			    AND custom_field_values != ''
			    AND custom_field_values LIKE ?
			  ORDER BY id ASC
			  LIMIT ?`,
			lastID, "%\""+fieldKey+"\"%", s.batchSize,
		)
		if err != nil {
			return totalProcessed, err
		}

		type itemRow struct {
			id  int
			cfv string
		}
		var batch []itemRow
		for rows.Next() {
			var ir itemRow
			if err := rows.Scan(&ir.id, &ir.cfv); err != nil {
				_ = rows.Close()
				return totalProcessed, err
			}
			batch = append(batch, ir)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return totalProcessed, err
		}
		_ = rows.Close()
		if len(batch) == 0 {
			return totalProcessed, nil
		}

		for _, ir := range batch {
			lastID = ir.id
			cleaned, changed, err := stripCFVKey(ir.cfv, fieldKey)
			if err != nil {
				// Malformed JSON in cfv — log and skip the row rather
				// than failing the whole job.
				slog.Warn("cfv_cleanup: skip malformed cfv", "item_id", ir.id, "error", err)
				continue
			}
			if !changed {
				continue
			}
			if _, err := s.db.ExecWrite(
				`UPDATE items SET custom_field_values = ? WHERE id = ?`,
				cleaned, ir.id,
			); err != nil {
				return totalProcessed, err
			}
			totalProcessed++
		}

		// If we read fewer than batchSize rows, we're done.
		if len(batch) < s.batchSize {
			return totalProcessed, nil
		}
	}
}

// stripCFVKey removes one key from a cfv JSON object. Returns the new
// JSON string, whether the key was actually present, and any parse error.
// If the resulting object would be empty, returns "" (the items.cfv
// column treats empty/NULL identically).
func stripCFVKey(cfvJSON, key string) (newJSON string, changed bool, err error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(cfvJSON), &m); err != nil {
		return "", false, err
	}
	if _, ok := m[key]; !ok {
		return cfvJSON, false, nil
	}
	delete(m, key)
	if len(m) == 0 {
		return "", true, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func (s *CFVCleanupScheduler) markDone(jobID, processed int) {
	now := time.Now()
	if _, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'done', completed_at = ?, items_processed = ?
		  WHERE id = ?`,
		now, processed, jobID,
	); err != nil {
		slog.Warn("cfv_cleanup: failed to mark job done", "job_id", jobID, "error", err)
	}
}

func (s *CFVCleanupScheduler) markFailed(jobID int, msg string) {
	now := time.Now()
	if _, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'failed', completed_at = ?, error_message = ?
		  WHERE id = ?`,
		now, msg, jobID,
	); err != nil {
		slog.Warn("cfv_cleanup: failed to mark job failed", "job_id", jobID, "error", err)
	}
}

// EnqueueFieldCleanup inserts a pending job for the given deleted field.
// Called by handlers/custom_fields.go Delete instead of doing inline
// cleanup. Idempotent: if a pending or running job already exists for
// the field, this is a no-op.
//
// Implementation note: lives in the scheduler package (not in handlers)
// so the table schema and the producer/consumer stay close.
func EnqueueFieldCleanup(db database.Database, fieldID int) error {
	// Skip duplicates: if a pending or running job already exists, don't
	// add another one.
	var existing int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pending_custom_field_cleanups
		  WHERE field_id = ? AND status IN ('pending', 'running')`,
		fieldID,
	).Scan(&existing)
	if err == nil && existing > 0 {
		return nil
	}

	now := time.Now()
	_, err = db.ExecWrite(
		`INSERT INTO pending_custom_field_cleanups (field_id, status, created_at)
		 VALUES (?, 'pending', ?)`,
		fieldID, now,
	)
	return err
}

// RunOnceForTests drives a single tick without starting the loop. Used
// by integration tests so they can assert deterministic post-conditions
// instead of sleeping waiting for the ticker.
func (s *CFVCleanupScheduler) RunOnceForTests() {
	// Reset stopChan in case Stop was called before RunOnceForTests.
	s.mu.Lock()
	if s.stopChan == nil {
		s.stopChan = make(chan struct{})
	}
	s.mu.Unlock()

	// Use a noop context just to ensure we have a current-time anchor.
	_ = context.Background()
	s.tick()
}
