package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
)

// JiraImportJobRecord is the persisted state needed to decide whether a new
// Jira import overlaps an existing one.
type JiraImportJobRecord struct {
	ID          string
	Status      string
	ConfigJSON  string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// JiraImportJobInsert contains the durable fields written when an import is
// admitted to the queue.
type JiraImportJobInsert struct {
	ID           string
	ConnectionID string
	ConfigJSON   string
	CreatedBy    *int
	// LeaseExpiresAt reserves the job the moment it is queued, so a reconcile
	// between the insert commit and worker startup does not treat it as an
	// interrupted job and admit an overlapping import.
	LeaseExpiresAt int64
}

// JiraImportJobStore exposes the job operations available while a connection
// is locked for an atomic conflict-check-and-enqueue decision.
type JiraImportJobStore interface {
	ListCandidates(connectionID string) ([]JiraImportJobRecord, error)
	Insert(job JiraImportJobInsert) error
}

// JiraImportJobRepository owns persistence and transaction boundaries for
// Jira import queue admission.
type JiraImportJobRepository struct {
	db database.Database
}

// NewJiraImportJobRepository creates a Jira import job repository.
func NewJiraImportJobRepository(db database.Database) *JiraImportJobRepository {
	return &JiraImportJobRepository{db: db}
}

// WithLockedConnection serializes queue admission for one Jira connection.
// PostgreSQL obtains a row lock from the no-op UPDATE; SQLite transactions use
// the database package's dedicated write connection.
func (r *JiraImportJobRepository) WithLockedConnection(
	connectionID string,
	fn func(JiraImportJobStore) error,
) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		lockResult, err := tx.ExecWrite(`
			UPDATE jira_import_connections SET id = id WHERE id = ? AND deleted_at IS NULL
		`, connectionID)
		if err != nil {
			return fmt.Errorf("lock Jira import connection: %w", err)
		}
		locked, err := lockResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect Jira import connection lock: %w", err)
		}
		if locked == 0 {
			return fmt.Errorf("jira import connection %q was not found", connectionID)
		}

		return fn(jiraImportJobTxStore{tx: tx})
	})
}

// ListCandidates returns non-deleted work-item imports for a connection.
func (r *JiraImportJobRepository) ListCandidates(connectionID string) ([]JiraImportJobRecord, error) {
	return listJiraImportJobCandidates(r.db, connectionID)
}

type jiraImportJobTxStore struct {
	tx database.Tx
}

func (s jiraImportJobTxStore) ListCandidates(connectionID string) ([]JiraImportJobRecord, error) {
	return listJiraImportJobCandidates(s.tx, connectionID)
}

func (s jiraImportJobTxStore) Insert(job JiraImportJobInsert) error {
	if _, err := s.tx.ExecWrite(`
		INSERT INTO jira_import_jobs (id, connection_id, status, scope, config_json, created_by, lease_expires_at)
		VALUES (?, ?, 'queued', 'work_items', ?, ?, ?)
	`, job.ID, job.ConnectionID, job.ConfigJSON, job.CreatedBy, job.LeaseExpiresAt); err != nil {
		return fmt.Errorf("create Jira import job: %w", err)
	}
	return nil
}

type jiraImportJobQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func listJiraImportJobCandidates(
	db jiraImportJobQueryer,
	connectionID string,
) ([]JiraImportJobRecord, error) {
	rows, err := db.Query(`
		SELECT id, status, config_json, created_at, completed_at
		FROM jira_import_jobs
		WHERE connection_id = ?
		  AND scope = 'work_items'
		  AND status <> 'data_deleted'
		ORDER BY created_at DESC
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var jobs []JiraImportJobRecord
	for rows.Next() {
		var job JiraImportJobRecord
		var completedAt sql.NullTime
		if err := rows.Scan(
			&job.ID,
			&job.Status,
			&job.ConfigJSON,
			&job.CreatedAt,
			&completedAt,
		); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}
