package jiraimport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// JiraImportLeaseDuration bounds how long a queued/running job is considered
// owned by a live worker without a renewal. executeImport renews the lease on
// a heartbeat while it runs, so startup reconciliation only fails jobs whose
// worker died.
const JiraImportLeaseDuration = 2 * time.Minute

type Service struct {
	db           database.Database
	statuses     *repository.StatusRepository
	itemTypes    *repository.ItemTypeRepository
	customFields *repository.CustomFieldRepository
	planning     *services.PlanningService
	collections  *repository.CollectionRepository
	boards       *repository.BoardConfigurationRepository
	assets       *repository.AssetRepository
	users        *repository.UserRepository
	labels       *repository.LabelRepository
	items        *repository.ItemRepository
	timeProjects *repository.TimeProjectRepository
	worklogs     *repository.TimeWorklogRepository
	attachments  *repository.AttachmentRepository
	linkTypes    *repository.LinkTypeRepository
	channels     *repository.ChannelRepository
	requestTypes *repository.RequestTypeRepository
	portalUsers  *repository.PortalCustomerRepository
	customerOrgs *repository.CustomerOrganisationRepository
	testCases    *repository.TestCaseRepository
	workflows    *repository.WorkflowRepository
	screens      *repository.ScreenRepository
	workspaces   *repository.WorkspaceRepository
}

func New(db database.Database) *Service {
	return &Service{
		db:           db,
		statuses:     repository.NewStatusRepository(db),
		itemTypes:    repository.NewItemTypeRepository(db),
		customFields: repository.NewCustomFieldRepository(db),
		planning:     services.NewPlanningService(db),
		collections:  repository.NewCollectionRepository(db),
		boards:       repository.NewBoardConfigurationRepository(db),
		assets:       repository.NewAssetRepository(db),
		users:        repository.NewUserRepository(db),
		labels:       repository.NewLabelRepository(db),
		items:        repository.NewItemRepository(db),
		timeProjects: repository.NewTimeProjectRepository(db),
		worklogs:     repository.NewTimeWorklogRepository(db),
		attachments:  repository.NewAttachmentRepository(db),
		linkTypes:    repository.NewLinkTypeRepository(db),
		channels:     repository.NewChannelRepository(db),
		requestTypes: repository.NewRequestTypeRepository(db),
		portalUsers:  repository.NewPortalCustomerRepository(db),
		customerOrgs: repository.NewCustomerOrganisationRepository(db),
		testCases:    repository.NewTestCaseRepository(db),
		workflows:    repository.NewWorkflowRepository(db),
		screens:      repository.NewScreenRepository(db),
		workspaces:   repository.NewWorkspaceRepository(db),
	}
}

func (s *Service) GetJobStatus(jobID string) (JobStatus, error) {
	var status, phase, progressJSON, resultJSON, errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT status, phase, progress_json, result_json, error_message, started_at, completed_at
		FROM jira_import_jobs WHERE id = ?
	`, jobID).Scan(&status, &phase, &progressJSON, &resultJSON, &errorMessage, &startedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return JobStatus{}, ErrJobNotFound
	}
	if err != nil {
		return JobStatus{}, err
	}
	result := JobStatus{JobID: jobID, Status: status.String}
	result.Phase = phase.String
	result.ErrorMessage = errorMessage.String
	if startedAt.Valid {
		result.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		result.CompletedAt = &completedAt.Time
	}
	if progressJSON.Valid {
		_ = json.Unmarshal([]byte(progressJSON.String), &result.Progress)
	}
	if resultJSON.Valid {
		_ = json.Unmarshal([]byte(resultJSON.String), &result.Result)
	}
	return result, nil
}

func (s *Service) JobConfigJSON(jobID string) (string, error) {
	var configJSON string
	err := s.db.QueryRow(`SELECT config_json FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&configJSON)
	return configJSON, err
}

func (s *Service) UpdateJobResult(jobID, token string, resultJSON []byte) error {
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_jobs SET result_json = ?
		WHERE id = ? AND status = 'running' AND lease_token = ?
	`, string(resultJSON), jobID, token)
	if err != nil {
		return err
	}
	return requireOneAffected(result, ErrLeaseLost)
}

// requireOneAffected turns a fenced write's row count into an ownership check:
// a write that matched no row means the worker lost the job to recovery or
// cleanup.
func requireOneAffected(result sql.Result, sentinel error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sentinel
	}
	return nil
}

func (s *Service) ListJobs() ([]JobInfo, error) {
	rows, err := s.db.Query(`
		SELECT j.id, j.connection_id, c.instance_url, c.instance_name, j.status, j.phase, j.scope,
		       j.config_json, j.progress_json, j.result_json, j.error_message, j.created_at, j.started_at, j.completed_at
		FROM jira_import_jobs j
		LEFT JOIN jira_import_connections c ON j.connection_id = c.id
		WHERE j.hidden_at IS NULL
		ORDER BY j.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]JobInfo, 0)
	for rows.Next() {
		var job JobInfo
		var instanceURL, instanceName, phase, configJSON, progressJSON, resultJSON, errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&job.ID, &job.ConnectionID, &instanceURL, &instanceName, &job.Status,
			&phase, &job.Scope, &configJSON, &progressJSON, &resultJSON, &errorMessage,
			&job.CreatedAt, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		job.InstanceURL = instanceURL.String
		job.InstanceName = instanceName.String
		job.Phase = phase.String
		job.ErrorMessage = errorMessage.String
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}
		if progressJSON.Valid {
			_ = json.Unmarshal([]byte(progressJSON.String), &job.Progress)
		}
		if resultJSON.Valid {
			_ = json.Unmarshal([]byte(resultJSON.String), &job.Result)
		}
		if configJSON.Valid {
			job.ProjectKeys = ProjectKeys(configJSON.String)
		}
		job.ImportedWorkspaceCount, job.ImportedItemCount = s.EntityCounts(job.ID)
		job.ImportedWorkspaces = s.Workspaces(job.ID)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Service) EntityCounts(jobID string) (workspaceCount, itemCount int) {
	rows, err := s.db.Query(`
		SELECT metadata_json FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'workspace'
	`, jobID)
	if err == nil {
		for rows.Next() {
			var metadata sql.NullString
			if rows.Scan(&metadata) == nil && MappingWasCreated(metadata) {
				workspaceCount++
			}
		}
		if err := rows.Err(); err != nil {
			slog.Warn("failed to read Jira import workspaces", slog.String("job_id", jobID), slog.Any("error", err))
		}
		_ = rows.Close()
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID).Scan(&itemCount); err != nil {
		slog.Warn("failed to count Jira import items", slog.String("job_id", jobID), slog.Any("error", err))
	}
	return workspaceCount, itemCount
}

func (s *Service) Workspaces(jobID string) []ImportedWorkspace {
	rows, err := s.db.Query(`
		SELECT DISTINCT w.id, w.key, w.name
		FROM jira_import_id_mappings m
		JOIN workspaces w ON w.id = m.windshift_id
		WHERE m.job_id = ? AND m.entity_type = 'workspace'
		ORDER BY w.key
	`, jobID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	workspaces := []ImportedWorkspace{}
	for rows.Next() {
		var workspace ImportedWorkspace
		if rows.Scan(&workspace.ID, &workspace.Key, &workspace.Name) == nil {
			workspaces = append(workspaces, workspace)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("failed to read Jira import workspaces", slog.String("job_id", jobID), slog.Any("error", err))
		return nil
	}
	return workspaces
}

func (s *Service) PreviousImports(projectKeys []string) ([]PreviousImport, error) {
	requested := projectKeySet(projectKeys)
	rows, err := s.db.Query(`
		SELECT j.id, j.connection_id, j.status, j.config_json, j.created_at, j.completed_at,
		       (SELECT COUNT(*) FROM jira_import_id_mappings m WHERE m.job_id = j.id AND m.entity_type = 'workspace'),
		       (SELECT COUNT(*) FROM jira_import_id_mappings m WHERE m.job_id = j.id AND m.entity_type = 'item')
		FROM jira_import_jobs j
		WHERE j.status IN ('completed', 'completed_with_errors')
		ORDER BY j.completed_at DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	imports := make([]PreviousImport, 0)
	for rows.Next() {
		var previous PreviousImport
		var configJSON string
		var completedAt sql.NullTime
		if err := rows.Scan(&previous.JobID, &previous.ConnectionID, &previous.Status, &configJSON,
			&previous.CreatedAt, &completedAt, &previous.WorkspaceCount, &previous.ItemCount); err != nil {
			return nil, err
		}
		previous.ProjectKeys = ProjectKeys(configJSON)
		if !projectKeysOverlap(requested, previous.ProjectKeys) {
			continue
		}
		if completedAt.Valid {
			previous.CompletedAt = &completedAt.Time
		}
		imports = append(imports, previous)
	}
	return imports, rows.Err()
}

// HideJob clears a finished import from the history listing without touching
// the job or its imported data. The job keeps driving conflict detection, so a
// re-import of the same projects still requires an explicit force.
func (s *Service) HideJob(jobID string) error {
	var status string
	var hiddenAt sql.NullTime
	err := s.db.QueryRow(`SELECT status, hidden_at FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&status, &hiddenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrJobNotFound
	}
	if err != nil {
		return err
	}
	if status == "queued" || status == "running" {
		return ErrJobActive
	}
	if hiddenAt.Valid {
		return nil
	}
	_, err = s.db.ExecWrite(`
		UPDATE jira_import_jobs SET hidden_at = CURRENT_TIMESTAMP
		WHERE id = ? AND hidden_at IS NULL
	`, jobID)
	return err
}

func (s *Service) Creator(jobID string) (int, error) {
	var createdBy sql.NullInt64
	if err := s.db.QueryRow(`SELECT created_by FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&createdBy); err != nil {
		return 0, err
	}
	if !createdBy.Valid {
		return 0, nil
	}
	return int(createdBy.Int64), nil
}

// ClaimJob assigns the job to this worker and moves it to running. A job that
// is no longer queued (recovered, already claimed, or cleaned up) cannot be
// claimed, so a stale worker never starts.
func (s *Service) ClaimJob(jobID, token string) error {
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_jobs
		SET status = 'running', phase = 'initializing', progress_json = '{}',
		    started_at = CURRENT_TIMESTAMP, lease_token = ?, lease_expires_at = ?
		WHERE id = ? AND status = 'queued'
	`, token, leaseExpiry(), jobID)
	if err != nil {
		return fmt.Errorf("claim Jira import job: %w", err)
	}
	return requireOneAffected(result, ErrLeaseLost)
}

// UpdateStatus applies a fenced status write. Only the worker that still holds
// the running job's token may change its state, so a recovered worker cannot
// overwrite a failure or a completed cleanup with a terminal status.
func (s *Service) UpdateStatus(jobID, token, status, phase string, progress *Progress, errorMessage string) error {
	progressJSON := "{}"
	if progress != nil {
		data, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		progressJSON = string(data)
	}
	var result sql.Result
	var err error
	switch status {
	case "completed", "completed_with_errors", "failed":
		result, err = s.db.ExecWrite(`
			UPDATE jira_import_jobs
			SET status = ?, phase = ?, progress_json = ?, error_message = ?,
			    completed_at = CURRENT_TIMESTAMP, lease_expires_at = NULL, lease_token = NULL
			WHERE id = ? AND status = 'running' AND lease_token = ?
		`, status, phase, progressJSON, errorMessage, jobID, token)
	default:
		result, err = s.db.ExecWrite(`
			UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ?
			WHERE id = ? AND status = 'running' AND lease_token = ?
		`, status, phase, progressJSON, jobID, token)
	}
	if err != nil {
		return err
	}
	return requireOneAffected(result, ErrLeaseLost)
}

func leaseExpiry() int64 {
	return time.Now().UTC().Add(JiraImportLeaseDuration).Unix()
}

// RenewLease extends the job's lease while its worker is alive. It only renews
// a running job still owned by the token and reports whether the lease was
// retained; a recovered or superseded worker cannot revive it.
func (s *Service) RenewLease(jobID, token string) (bool, error) {
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_jobs SET lease_expires_at = ?
		WHERE id = ? AND status = 'running' AND lease_token = ? AND lease_expires_at > ?
	`, leaseExpiry(), jobID, token, time.Now().UTC().Unix())
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// CheckJobOwnership reports whether the token still owns a running job. It is
// the pre-write gate for a long import: a recovered or cleaned-up job no longer
// matches and the worker must stop persisting.
func (s *Service) CheckJobOwnership(jobID, token string) error {
	var owned bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM jira_import_jobs
			WHERE id = ? AND status = 'running' AND lease_token = ?
		)
	`, jobID, token).Scan(&owned)
	if err != nil {
		return err
	}
	if !owned {
		return ErrLeaseLost
	}
	return nil
}

// ReconcileInterrupted fails queued/running jobs whose worker lease expired
// (for example after a server restart) so they no longer block retry or
// cleanup. A live worker that keeps renewing its lease is left untouched.
func (s *Service) ReconcileInterrupted(now time.Time) (int, error) {
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_jobs
		SET status = 'failed', phase = 'failed',
		    error_message = 'Import interrupted by server restart',
		    completed_at = CURRENT_TIMESTAMP, lease_expires_at = NULL, lease_token = NULL
		WHERE status IN ('queued', 'running')
		  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
	`, now.UTC().Unix())
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (s *Service) UpdateProgress(jobID, token string, progress *Progress) error {
	data, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_jobs SET phase = ?, progress_json = ?, lease_expires_at = ?
		WHERE id = ? AND status = 'running' AND lease_token = ?
	`, progress.Phase, string(data), leaseExpiry(), jobID, token)
	if err != nil {
		return err
	}
	return requireOneAffected(result, ErrLeaseLost)
}

func ProjectKeys(configJSON string) []string {
	var config map[string]any
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		return nil
	}
	rawKeys, ok := config["project_keys"].([]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(rawKeys))
	seen := make(map[string]struct{}, len(rawKeys))
	for _, raw := range rawKeys {
		key, ok := raw.(string)
		if !ok {
			continue
		}
		key = normalizeProjectKey(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func projectKeySet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = normalizeProjectKey(key); key != "" {
			set[key] = struct{}{}
		}
	}
	return set
}

func projectKeysOverlap(requested map[string]struct{}, existing []string) bool {
	for _, key := range existing {
		if _, ok := requested[normalizeProjectKey(key)]; ok {
			return true
		}
	}
	return false
}

func normalizeProjectKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}
