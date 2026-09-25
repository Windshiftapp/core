package csvimport

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// LeaseDuration bounds one worker's exclusive write window on a job. Row
// callbacks renew it through the store while they work.
const LeaseDuration = time.Minute

// JobRow is the persisted state of one import job.
type JobRow struct {
	JobID          string
	Kind           string
	ScopeID        int
	Status         sql.NullString
	Phase          sql.NullString
	ProgressJSON   sql.NullString
	ErrorMessage   sql.NullString
	CreatedAt      sql.NullTime
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
	LeaseExpiresAt sql.NullInt64
}

// Store persists import uploads and jobs in the shared generic tables.
type Store struct {
	db database.Database
}

func NewStore(db database.Database) *Store {
	return &Store{db: db}
}

// CreateUpload records a staged upload before a job claims it.
func (s *Store) CreateUpload(kind string, scopeID, userID int, uploadID string, now time.Time) error {
	_, err := s.db.ExecWrite(
		"INSERT INTO import_uploads (id, kind, scope_id, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		uploadID, kind, scopeID, userID, now.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create import upload: %w", err)
	}
	return nil
}

// UploadOwnedBy reports whether the upload belongs to the caller and scope.
func (s *Store) UploadOwnedBy(kind string, scopeID, userID int, uploadID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM import_uploads WHERE id = ? AND kind = ? AND scope_id = ? AND created_by = ?",
		uploadID, kind, scopeID, userID,
	).Scan(&count)
	return count == 1, err
}

// ClaimUpload uses the upload ID as the job ID, so the primary key elects one
// worker. It reports false when the upload was already claimed; a claim under
// different settings yields ErrConfigConflict.
func (s *Store) ClaimUpload(kind string, scopeID, userID int, uploadID, filePath, configJSON string, now time.Time) (bool, error) {
	result, err := s.db.ExecWrite(`
  INSERT INTO import_jobs (id, kind, scope_id, status, phase, file_path, config_json, created_by, created_at, lease_expires_at)
  SELECT ?, ?, ?, 'queued', 'initializing', ?, ?, ?, ?, ? FROM import_uploads
  WHERE id = ? AND kind = ? AND scope_id = ? AND created_by = ?
  ON CONFLICT (id) DO NOTHING`,
		uploadID, kind, scopeID, filePath, configJSON, userID, now, now.Add(LeaseDuration).Unix(),
		uploadID, kind, scopeID, userID)
	if err != nil {
		return false, fmt.Errorf("claim import upload: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 1 {
		return true, nil
	}
	var existing string
	if err := s.db.QueryRow(
		"SELECT config_json FROM import_jobs WHERE id = ? AND kind = ? AND scope_id = ? AND created_by = ?",
		uploadID, kind, scopeID, userID,
	).Scan(&existing); err != nil {
		return false, notFoundOrWrap(err, "find claimed import")
	}
	if existing != configJSON {
		return false, ErrConfigConflict
	}
	return false, nil
}

// GetJob returns one job's persisted state.
func (s *Store) GetJob(kind string, scopeID int, jobID string) (*JobRow, error) {
	row := &JobRow{JobID: jobID, Kind: kind, ScopeID: scopeID}
	err := s.db.QueryRow(`
		SELECT status, phase, progress_json, error_message, created_at, started_at, completed_at, lease_expires_at
		FROM import_jobs WHERE id = ? AND kind = ? AND scope_id = ?
	`, jobID, kind, scopeID).Scan(&row.Status, &row.Phase, &row.ProgressJSON, &row.ErrorMessage,
		&row.CreatedAt, &row.StartedAt, &row.CompletedAt, &row.LeaseExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUploadNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get import job: %w", err)
	}
	return row, nil
}

// ListJobs returns the scope's most recent jobs, newest first.
func (s *Store) ListJobs(kind string, scopeID, limit int) ([]JobRow, error) {
	rows, err := s.db.Query(`
		SELECT id, status, phase, progress_json, error_message, created_at, started_at, completed_at, lease_expires_at
		FROM import_jobs WHERE kind = ? AND scope_id = ? ORDER BY created_at DESC LIMIT ?
	`, kind, scopeID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]JobRow, 0)
	for rows.Next() {
		var job JobRow
		if err := rows.Scan(&job.JobID, &job.Status, &job.Phase, &job.ProgressJSON, &job.ErrorMessage,
			&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.LeaseExpiresAt); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate import jobs: %w", err)
	}
	return jobs, nil
}

func writeResult(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("write import job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrLeaseLost
	}
	return nil
}

// StartRunning transitions a queued job to running under a fresh lease.
func (s *Store) StartRunning(jobID, phase, progressJSON string) error {
	now := time.Now().UTC()
	return writeResult(s.db.ExecWrite(
		`UPDATE import_jobs SET status = 'running', phase = ?, progress_json = ?, started_at = ?, lease_expires_at = ?
         WHERE id = ? AND status = 'queued' AND lease_expires_at > ?`,
		phase, progressJSON, now, now.Add(LeaseDuration).Unix(), jobID, now.Unix()))
}

// Finish closes a job with its terminal status.
func (s *Store) Finish(jobID, status, phase, progressJSON, errorMessage string) error {
	now := time.Now().UTC()
	return writeResult(s.db.ExecWrite(
		`UPDATE import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = ?
         WHERE id = ? AND status IN ('queued', 'running') AND lease_expires_at > ?`,
		status, phase, progressJSON, errorMessage, now, jobID, now.Unix()))
}

// UpdateStatus records an interim status without leaving the active set.
func (s *Store) UpdateStatus(jobID, status, phase, progressJSON string) error {
	now := time.Now().UTC()
	return writeResult(s.db.ExecWrite(
		`UPDATE import_jobs SET status = ?, phase = ?, progress_json = ?, lease_expires_at = ?
         WHERE id = ? AND status IN ('queued', 'running') AND lease_expires_at > ?`,
		status, phase, progressJSON, now.Add(LeaseDuration).Unix(), jobID, now.Unix()))
}

// UpdateProgress records phase and progress while refreshing the lease.
func (s *Store) UpdateProgress(jobID, phase, progressJSON string) error {
	now := time.Now().UTC()
	return writeResult(s.db.ExecWrite(
		`UPDATE import_jobs SET phase = ?, progress_json = ?, lease_expires_at = ?
         WHERE id = ? AND status = 'running' AND lease_expires_at > ?`,
		phase, progressJSON, now.Add(LeaseDuration).Unix(), jobID, now.Unix()))
}

// RenewLeaseInTx fences row insertion against reconciliation. The job lock
// remains held until the caller's transaction commits.
func (s *Store) RenewLeaseInTx(tx database.Tx, jobID string) error {
	now := time.Now().UTC()
	return writeResult(tx.ExecWrite(`UPDATE import_jobs SET lease_expires_at = ?
		WHERE id = ? AND status = 'running' AND lease_expires_at > ?`,
		now.Add(LeaseDuration).Unix(), jobID, now.Unix()))
}

// ReconcileExpired atomically claims and rolls back abandoned jobs of one
// kind. The rollback hook runs in the claiming transaction: assets delete the
// job's rows, tickets delete the job's imported items. A live worker renewing
// its lease wins over recovery of a stale candidate.
func (s *Store) ReconcileExpired(kind string, now time.Time, rollback func(tx database.Tx, jobID string) error) (int, error) {
	rows, err := s.db.Query(`SELECT id FROM import_jobs
		WHERE kind = ? AND status IN ('queued', 'running') AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`, kind, now.Unix())
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		claimed := false
		err := database.WithTx(s.db, func(tx database.Tx) error {
			result, err := tx.ExecWrite(`UPDATE import_jobs
				SET status = 'failed', phase = '', error_message = 'Import worker lease expired', completed_at = ?
				WHERE id = ? AND status IN ('queued', 'running') AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`, now, id, now.Unix())
			if err := writeResult(result, err); err != nil {
				if errors.Is(err, ErrLeaseLost) {
					return nil
				}
				return err
			}
			claimed = true
			if rollback == nil {
				return nil
			}
			return rollback(tx, id)
		})
		if err != nil {
			return count, fmt.Errorf("recover import %s: %w", id, err)
		}
		if claimed {
			count++
		}
	}
	return count, nil
}

func notFoundOrWrap(err error, context string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUploadNotFound
	}
	return fmt.Errorf("%s: %w", context, err)
}
