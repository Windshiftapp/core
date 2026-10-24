// Package scheduler — see cfv_cleanup_scheduler.go for the async cleanup
// path used after a custom field is deleted.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// Job types for deferred custom-field maintenance; empty legacy values scrub fields.
const (
	jobFieldScrub    = "field_scrub"    // strip a deleted field's key from cfv JSON
	jobOptionRemoval = "option_removal" // strip removed select/multiselect option ids
	jobIndexBuild    = "index_build"    // build a Postgres cf index CONCURRENTLY
)

// CFVCleanupScheduler batches deferred field scrubs, option removal, and
// Postgres index builds. It reclaims stale running jobs after a process crash.
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
	s.ticker = time.NewTicker(s.checkInterval)
	s.stopChan = make(chan struct{})
	s.running = true
	slog.Info("starting cfv cleanup scheduler", "interval", s.checkInterval, "batch_size", s.batchSize)
	go s.loop(s.ticker, s.stopChan)
}

// Stop halts the scheduler. Safe to call multiple times.
func (s *CFVCleanupScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	close(s.stopChan)
	slog.Info("cfv cleanup scheduler stopped")
}

func (s *CFVCleanupScheduler) loop(ticker *time.Ticker, stopChan <-chan struct{}) {
	// Process immediately on startup so a queued job from a previous
	// process generation doesn't wait the full interval.
	s.tick()
	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-stopChan:
			return
		}
	}
}

// tick drains as many pending jobs as exist; bounded by claimMaxJobsPerTick
// to keep a stuck queue from monopolizing scheduler resources.
const claimMaxJobsPerTick = 20

const maxCleanupJobAttempts = 3

func (s *CFVCleanupScheduler) tick() {
	start := time.Now()
	totalItems := 0
	var runErr error
	defer recordSchedulerRun(s.runRepo, schedulerName, start, &totalItems, &runErr, time.Now)

	// First: rehabilitate stale 'running' rows so a crashed process doesn't
	// strand jobs indefinitely.
	if err := s.requeueStaleRunning(); err != nil {
		slog.Warn("cfv_cleanup: requeue stale failed", "error", err)
		// don't return — we can still try to drain fresh jobs
	}

	for i := 0; i < claimMaxJobsPerTick; i++ {
		job, claimed, err := s.claimNextJob()
		if err != nil {
			runErr = err
			return
		}
		if !claimed {
			return
		}
		processed, err := s.processClaimedJob(job)
		if err != nil {
			permanent := s.recordJobFailure(job, err.Error())
			if permanent {
				runErr = fmt.Errorf("custom field maintenance job %d failed permanently after %d attempts: %w", job.id, maxCleanupJobAttempts, err)
			} else {
				runErr = fmt.Errorf("custom field maintenance job %d attempt %d failed: %w", job.id, job.attemptCount+1, err)
			}
			continue
		}
		s.markDone(job.id, processed)
		totalItems += processed
	}
}

// claimedJob is one row claimed from the queue, ready to dispatch.
type claimedJob struct {
	id           int
	fieldID      int
	jobType      string
	payload      string
	attemptCount int
}

// processClaimedJob dispatches a claimed row to its job-type handler. An empty
// job_type is treated as field_scrub for rows enqueued before the queue grew
// a job_type column.
func (s *CFVCleanupScheduler) processClaimedJob(job claimedJob) (int, error) {
	switch job.jobType {
	case "", jobFieldScrub:
		return s.processJob(job.fieldID)
	case jobOptionRemoval:
		return s.processOptionRemoval(job.payload)
	case jobIndexBuild:
		return s.processIndexBuild(job.payload)
	default:
		return 0, fmt.Errorf("unknown job type %q", job.jobType)
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

// claimNextJob claims the oldest pending row. Its status-guarded update makes
// SQLite and Postgres claims safe without row locks.
func (s *CFVCleanupScheduler) claimNextJob() (job claimedJob, claimed bool, err error) {
	var jobType sql.NullString
	var payload sql.NullString
	row := s.db.QueryRow(
		`SELECT id, field_id, job_type, payload, attempt_count FROM pending_custom_field_cleanups
		  WHERE status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		  ORDER BY created_at ASC
		  LIMIT 1`,
		time.Now(),
	)
	if err = row.Scan(&job.id, &job.fieldID, &jobType, &payload, &job.attemptCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Empty queue — caller exits the drain loop normally.
			return claimedJob{}, false, nil
		}
		return claimedJob{}, false, err
	}
	job.jobType = jobType.String
	job.payload = payload.String

	now := time.Now()
	res, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'running', started_at = ?, next_attempt_at = NULL
		  WHERE id = ? AND status = 'pending'`,
		now, job.id,
	)
	if err != nil {
		return claimedJob{}, false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Someone else claimed it between our SELECT and UPDATE — try the
		// next call.
		return claimedJob{}, false, nil
	}
	return job, true, nil
}

// processJob scrubs the deleted field from item, asset, and legacy portal
// storage in bounded keyset-paginated batches.
func (s *CFVCleanupScheduler) processJob(fieldID int) (int, error) {
	fieldKey := strconv.Itoa(fieldID)
	cfRepo := repository.NewCustomFieldRepository(s.db)
	total := 0
	for _, table := range []string{"items", "assets"} {
		processed, err := s.scrubTableField(cfRepo, table, fieldKey)
		total += processed
		if err != nil {
			return total, err
		}
	}
	processed, err := s.scrubPortalField(cfRepo, fieldID)
	total += processed
	if err != nil {
		slog.Warn("cfv_cleanup: portal field scrub skipped", "field_id", fieldID, "error", err)
	}
	return total, nil
}

func (s *CFVCleanupScheduler) scrubTableField(cfRepo *repository.CustomFieldRepository, table, fieldKey string) (int, error) {
	processed := 0
	lastID := 0
	for {
		batch, err := cfRepo.ListRowsWithCustomFieldsPageByKey(table, lastID, fieldKey, s.batchSize)
		if err != nil {
			return processed, err
		}
		if len(batch) == 0 {
			return processed, nil
		}
		for _, row := range batch {
			lastID = row.ID
			changed, malformed, err := applyTableCFVCleanup(cfRepo, table, row.ID, row.Value, func(current string) (string, bool, error) {
				return stripCFVKey(current, fieldKey)
			})
			if err != nil {
				if malformed {
					slog.Warn("cfv_cleanup: skip malformed cfv", "table", table, "id", row.ID, "error", err)
					continue
				}
				return processed, err
			}
			if !changed {
				continue
			}
			processed++
			if table == "items" {
				s.enqueueSLARecalc(row.ID)
			}
		}
		if len(batch) < s.batchSize {
			return processed, nil
		}
	}
}

func (s *CFVCleanupScheduler) scrubPortalField(cfRepo *repository.CustomFieldRepository, fieldID int) (int, error) {
	processed := 0
	lastID := 0
	for {
		batch, err := cfRepo.ListPortalCFVsPageByField(fieldID, lastID, s.batchSize)
		if err != nil {
			return processed, err
		}
		if len(batch) == 0 {
			return processed, nil
		}
		for _, row := range batch {
			lastID = row.ID
			if err := cfRepo.DeletePortalCFV(row.ID); err != nil {
				return processed, err
			}
			processed++
		}
		if len(batch) < s.batchSize {
			return processed, nil
		}
	}
}

// stripCFVKey removes one key from a cfv JSON object. Returns the new
// JSON string, whether the key was actually present, and any parse error.
// If the resulting object would be empty, returns "" (the items.cfv
// column treats empty/NULL identically).
func stripCFVKey(cfvJSON, key string) (newJSON string, changed bool, err error) {
	var m map[string]any
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

// optionRemovalPayload is the JSON stored in pending_custom_field_cleanups.payload
// for an option_removal job. The removed ids are captured at request time (the
// field's stored options no longer contain them once the job runs, so the worker
// cannot recompute the diff).
type optionRemovalPayload struct {
	FieldID    int    `json:"field_id"`
	FieldType  string `json:"field_type"` // "select" or "multiselect"
	RemovedIDs []int  `json:"removed_ids"`
}

// processOptionRemoval strips deleted options in idempotent, keyset-paginated batches.
func (s *CFVCleanupScheduler) processOptionRemoval(payload string) (int, error) {
	var p optionRemovalPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return 0, fmt.Errorf("decode option_removal payload: %w", err)
	}
	if p.FieldID == 0 || len(p.RemovedIDs) == 0 {
		return 0, nil
	}
	if p.FieldType != "select" && p.FieldType != "multiselect" &&
		p.FieldType != "milestone" && p.FieldType != "multi_milestone" {
		return 0, fmt.Errorf("option_removal: unsupported field type %q", p.FieldType)
	}

	removed := make(map[int]bool, len(p.RemovedIDs))
	for _, id := range p.RemovedIDs {
		removed[id] = true
	}
	fieldKey := strconv.Itoa(p.FieldID)
	cfRepo := repository.NewCustomFieldRepository(s.db)
	total := 0

	for _, table := range []string{"items", "assets"} {
		n, err := s.scrubTableOptions(cfRepo, table, fieldKey, p.FieldType, removed)
		total += n
		if err != nil {
			return total, err
		}
	}

	n, err := s.scrubPortalOptions(cfRepo, p.FieldID, p.FieldType, removed)
	total += n
	if err != nil {
		// The portal custom_field_values table may not exist on every
		// deployment; treat a query failure as "nothing to clean" rather
		// than failing the whole job, matching the inline handler's behavior.
		slog.Warn("cfv_cleanup: portal option scrub skipped", "field_id", p.FieldID, "error", err)
	}
	return total, nil
}

// scrubTableOptions removes deleted option ids from items/assets cfv JSON in
// keyset-paginated batches.
func (s *CFVCleanupScheduler) scrubTableOptions(cfRepo *repository.CustomFieldRepository, table, fieldKey, fieldType string, removed map[int]bool) (int, error) {
	processed := 0
	lastID := 0
	for {
		batch, err := cfRepo.ListRowsWithCustomFieldsPageByKey(table, lastID, fieldKey, s.batchSize)
		if err != nil {
			return processed, err
		}
		if len(batch) == 0 {
			return processed, nil
		}
		for _, row := range batch {
			lastID = row.ID
			changed, malformed, err := applyTableCFVCleanup(cfRepo, table, row.ID, row.Value, func(current string) (string, bool, error) {
				return stripCFVOptionIDs(current, fieldKey, fieldType, removed)
			})
			if err != nil {
				if malformed {
					slog.Warn("cfv_cleanup: skip malformed cfv", "table", table, "id", row.ID, "error", err)
					continue
				}
				return processed, err
			}
			if !changed {
				continue
			}
			processed++
			if table == "items" {
				s.enqueueSLARecalc(row.ID)
			}
		}
		if len(batch) < s.batchSize {
			return processed, nil
		}
	}
}

// enqueueSLARecalc schedules an SLA recalculation for an item whose custom
// field values changed outside the recorder. It only enqueues when the item's
// workspace has SLA configuration; the due-work loop notices the job within its
// safety interval.
func (s *CFVCleanupScheduler) enqueueSLARecalc(itemID int) {
	repo := repository.NewSLARepository(s.db)
	ctx := context.Background()
	var workspaceID int
	if err := s.db.QueryRowContext(ctx, `SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID); err != nil {
		return
	}
	generation, err := repo.WorkspaceGeneration(ctx, workspaceID)
	if err != nil || generation == 0 {
		return
	}
	item := itemID
	err = database.WithTx(s.db, func(tx database.Tx) error {
		return repo.UpsertJob(ctx, tx, &models.SLAJob{Kind: models.SLAJobRecalcItem, ItemID: &item, DueAt: time.Now().UTC()})
	})
	if err != nil {
		slog.Warn("cfv_cleanup: enqueue SLA recalc failed", "item_id", itemID, "error", err)
	}
}

const cleanupCompareAndSwapAttempts = 3

type tableCFVCleanup func(current string) (cleaned string, changed bool, err error)

func applyTableCFVCleanup(cfRepo *repository.CustomFieldRepository, table string, rowID int, initial string, cleanup tableCFVCleanup) (changed, malformed bool, err error) {
	current := initial
	for range cleanupCompareAndSwapAttempts {
		cleaned, changed, err := cleanup(current)
		if err != nil {
			return false, true, err
		}
		if !changed {
			return false, false, nil
		}
		swapped, err := cfRepo.CompareAndSwapRowCustomFields(table, rowID, current, cleaned)
		if err != nil {
			return false, false, err
		}
		if swapped {
			return true, false, nil
		}
		latest, found, err := cfRepo.FindRowCustomFields(table, rowID)
		if err != nil || !found || latest == "" {
			return false, false, err
		}
		current = latest
	}
	return false, false, nil
}

// scrubPortalOptions removes deleted option ids from the portal
// custom_field_values table in keyset-paginated batches. Portal values are
// stored as a bare option id (select) or a JSON array of ids (multiselect),
// not as a cfv map, so they are handled separately from items/assets.
func (s *CFVCleanupScheduler) scrubPortalOptions(cfRepo *repository.CustomFieldRepository, fieldID int, fieldType string, removed map[int]bool) (int, error) {
	processed := 0
	lastID := 0
	for {
		batch, err := cfRepo.ListPortalCFVsPageByField(fieldID, lastID, s.batchSize)
		if err != nil {
			return processed, err
		}
		if len(batch) == 0 {
			return processed, nil
		}
		for _, row := range batch {
			lastID = row.ID
			switch fieldType {
			case "select", "milestone":
				if numVal, err := strconv.Atoi(row.Value); err == nil && removed[numVal] {
					if err := cfRepo.DeletePortalCFV(row.ID); err != nil {
						return processed, err
					}
					processed++
				}
			case "multiselect", "multi_milestone":
				var ids []int
				if err := json.Unmarshal([]byte(row.Value), &ids); err != nil {
					continue
				}
				changed := false
				filtered := make([]int, 0, len(ids))
				for _, optID := range ids {
					if removed[optID] {
						changed = true
						continue
					}
					filtered = append(filtered, optID)
				}
				if !changed {
					continue
				}
				if len(filtered) == 0 {
					if err := cfRepo.DeletePortalCFV(row.ID); err != nil {
						return processed, err
					}
				} else {
					b, err := json.Marshal(filtered)
					if err != nil {
						continue
					}
					if err := cfRepo.UpdatePortalCFV(row.ID, string(b)); err != nil {
						return processed, err
					}
				}
				processed++
			}
		}
		if len(batch) < s.batchSize {
			return processed, nil
		}
	}
}

// stripCFVOptionIDs removes the given option ids from one cfv JSON object's
// entry for fieldKey. For select the whole key is dropped when its value is a
// removed id; for multiselect the removed ids are filtered out (and the key
// dropped if the array empties). Returns the new JSON, whether anything
// changed, and any parse error.
func stripCFVOptionIDs(cfvJSON, fieldKey, fieldType string, removed map[int]bool) (newJSON string, changed bool, err error) {
	var cfv map[string]any
	if err := json.Unmarshal([]byte(cfvJSON), &cfv); err != nil {
		return "", false, err
	}
	val, exists := cfv[fieldKey]
	if !exists {
		return cfvJSON, false, nil
	}

	switch fieldType {
	case "select", "milestone":
		if optionID, ok := cleanupOptionID(val); ok && removed[optionID] {
			delete(cfv, fieldKey)
			changed = true
		}
	case "multiselect", "multi_milestone":
		if arr, ok := val.([]any); ok {
			var filtered []any
			for _, item := range arr {
				if optionID, ok := cleanupOptionID(item); ok && removed[optionID] {
					changed = true
					continue
				}
				filtered = append(filtered, item)
			}
			if changed {
				if len(filtered) == 0 {
					delete(cfv, fieldKey)
				} else {
					cfv[fieldKey] = filtered
				}
			}
		}
	}

	if !changed {
		return cfvJSON, false, nil
	}
	b, err := json.Marshal(cfv)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func cleanupOptionID(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case string:
		optionID, err := strconv.Atoi(typed)
		return optionID, err == nil
	default:
		return 0, false
	}
}

// indexBuildPayload is the JSON stored in pending_custom_field_cleanups.payload
// for an index_build job (Postgres only).
type indexBuildPayload struct {
	FieldID     int    `json:"field_id"`
	FieldType   string `json:"field_type"`
	TargetTable string `json:"target_table"`
	IndexName   string `json:"index_name"`
}

// processIndexBuild concurrently builds a Postgres index and drops invalid
// leftovers first, making retries self-healing.
func (s *CFVCleanupScheduler) processIndexBuild(payload string) (int, error) {
	var p indexBuildPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return 0, fmt.Errorf("decode index_build payload: %w", err)
	}
	if p.IndexName == "" || p.TargetTable == "" {
		return 0, fmt.Errorf("index_build: incomplete payload")
	}

	cfRepo := repository.NewCustomFieldRepository(s.db)
	recorded, err := cfRepo.IsIndexRecorded(p.FieldID, p.TargetTable)
	if err != nil {
		return 0, fmt.Errorf("check index record: %w", err)
	}
	if !recorded {
		// Field (or its index) was removed after the job was enqueued; the
		// Delete handler already dropped the physical index. Nothing to do.
		return 0, nil
	}

	if err := cfRepo.ExecDDL("DROP INDEX IF EXISTS " + p.IndexName); err != nil {
		return 0, fmt.Errorf("drop stale index before rebuild: %w", err)
	}
	createSQL := database.BuildPostgresCustomFieldIndexSQL(p.FieldID, p.FieldType, p.TargetTable, p.IndexName, true)
	if err := cfRepo.ExecDDL(createSQL); err != nil {
		return 0, fmt.Errorf("create index concurrently: %w", err)
	}
	return 1, nil
}

func (s *CFVCleanupScheduler) markDone(jobID, processed int) {
	now := time.Now()
	if _, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'done', completed_at = ?, items_processed = ?, next_attempt_at = NULL, error_message = NULL
		  WHERE id = ?`,
		now, processed, jobID,
	); err != nil {
		slog.Warn("cfv_cleanup: failed to mark job done", "job_id", jobID, "error", err)
	}
}

func (s *CFVCleanupScheduler) recordJobFailure(job claimedJob, msg string) bool {
	now := time.Now()
	nextAttempt := job.attemptCount + 1
	if nextAttempt < maxCleanupJobAttempts {
		retryAt := now.Add(time.Minute * time.Duration(1<<(nextAttempt-1)))
		if _, err := s.db.ExecWrite(
			`UPDATE pending_custom_field_cleanups
			    SET status = 'pending', started_at = NULL, completed_at = NULL,
			        attempt_count = ?, next_attempt_at = ?, error_message = ?
			  WHERE id = ?`,
			nextAttempt, retryAt, msg, job.id,
		); err != nil {
			slog.Warn("cfv_cleanup: failed to schedule job retry", "job_id", job.id, "error", err)
		}
		return false
	}
	if _, err := s.db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'failed', completed_at = ?, attempt_count = ?, next_attempt_at = NULL, error_message = ?
		  WHERE id = ?`,
		now, nextAttempt, msg, job.id,
	); err != nil {
		slog.Warn("cfv_cleanup: failed to mark job failed", "job_id", job.id, "error", err)
	}
	return true
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
		`INSERT INTO pending_custom_field_cleanups (field_id, job_type, status, created_at)
		 VALUES (?, 'field_scrub', 'pending', ?)`,
		fieldID, now,
	)
	return err
}

// EnqueueOptionRemoval inserts a job to strip the given removed choice ids
// (select/multiselect options or milestone references) from items, assets, and
// portal custom_field_values. Called by handlers/custom_fields.go Update and
// milestone deletion instead of cleaning inline.
//
// Unlike EnqueueFieldCleanup this does NOT dedup: each edit removes a distinct
// set of option ids, and the removed-id set is captured here (the field's
// stored options no longer contain them by the time the job runs). A no-op
// when removedIDs is empty.
func EnqueueOptionRemoval(db database.Database, fieldID int, fieldType string, removedIDs []int) error {
	if len(removedIDs) == 0 {
		return nil
	}
	return database.WithTx(db, func(tx database.Tx) error {
		return EnqueueOptionRemovalTx(tx, fieldID, fieldType, removedIDs)
	})
}

// EnqueueOptionRemovalTx is EnqueueOptionRemoval inside the caller's
// transaction, so a repair that rewrites an option set commits its scrubbing
// job atomically (WI-1529).
func EnqueueOptionRemovalTx(tx database.Tx, fieldID int, fieldType string, removedIDs []int) error {
	if len(removedIDs) == 0 {
		return nil
	}
	payload, err := json.Marshal(optionRemovalPayload{
		FieldID:    fieldID,
		FieldType:  fieldType,
		RemovedIDs: removedIDs,
	})
	if err != nil {
		return fmt.Errorf("marshal option_removal payload: %w", err)
	}
	now := time.Now()
	_, err = tx.ExecContext(context.Background(),
		`INSERT INTO pending_custom_field_cleanups (field_id, job_type, payload, status, created_at)
		 VALUES (?, 'option_removal', ?, 'pending', ?)`,
		fieldID, string(payload), now,
	)
	return err
}

// EnqueueIndexBuild inserts a job to build a Postgres custom-field index
// CONCURRENTLY off the request thread (WI-416). Idempotent: indexName uniquely
// identifies (field, table), so a build already pending/running for the same
// index is a no-op. Callers only invoke this on Postgres — SQLite materializes
// recorded indexes at startup instead.
func EnqueueIndexBuild(db database.Database, fieldID int, fieldType, targetTable, indexName string) error {
	payload, err := json.Marshal(indexBuildPayload{
		FieldID:     fieldID,
		FieldType:   fieldType,
		TargetTable: targetTable,
		IndexName:   indexName,
	})
	if err != nil {
		return fmt.Errorf("marshal index_build payload: %w", err)
	}

	var existing int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pending_custom_field_cleanups
		  WHERE job_type = 'index_build' AND status IN ('pending', 'running') AND payload LIKE ?`,
		`%"index_name":"`+indexName+`"%`,
	).Scan(&existing); err == nil && existing > 0 {
		return nil
	}

	now := time.Now()
	_, err = db.ExecWrite(
		`INSERT INTO pending_custom_field_cleanups (field_id, job_type, payload, status, created_at)
		 VALUES (?, 'index_build', ?, 'pending', ?)`,
		fieldID, string(payload), now,
	)
	return err
}

// CancelPendingIndexBuilds marks any pending/running index_build jobs for the
// field as done so a queued build cannot recreate an index for a field that is
// being deleted. Called from the Delete handler after it drops the field's
// indexes synchronously.
func CancelPendingIndexBuilds(db database.Database, fieldID int) error {
	now := time.Now()
	_, err := db.ExecWrite(
		`UPDATE pending_custom_field_cleanups
		    SET status = 'done', completed_at = ?
		  WHERE field_id = ? AND job_type = 'index_build' AND status IN ('pending', 'running')`,
		now, fieldID,
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
