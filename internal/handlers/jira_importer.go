package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/logger"
	"windshift/internal/utils"

	"github.com/google/uuid"
)

// GetJobStatus handles GET /api/admin/jira-import/jobs/{jobId}
func (h *JiraImportHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	var status, phase, progressJSON, resultJSON, errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime

	err := h.db.QueryRow(`
		SELECT status, phase, progress_json, result_json, error_message, started_at, completed_at
		FROM jira_import_jobs
		WHERE id = ?
	`, jobID).Scan(&status, &phase, &progressJSON, &resultJSON, &errorMessage, &startedAt, &completedAt)
	if err != nil {
		respondNotFound(w, r, "job")
		return
	}

	response := ImportJobStatus{
		JobID:  jobID,
		Status: status.String,
	}
	if phase.Valid {
		response.Phase = phase.String
	}
	if errorMessage.Valid {
		response.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		response.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		response.CompletedAt = &completedAt.Time
	}
	if progressJSON.Valid {
		var progress map[string]interface{}
		if err := json.Unmarshal([]byte(progressJSON.String), &progress); err == nil {
			response.Progress = progress
		}
	}

	respondJSONOK(w, response)
}

// GetImportJobs handles GET /api/admin/jira-import/jobs
func (h *JiraImportHandler) GetImportJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT j.id, j.connection_id, c.instance_url, c.instance_name, j.status, j.phase, j.scope,
		       j.progress_json, j.result_json, j.error_message, j.created_at, j.started_at, j.completed_at
		FROM jira_import_jobs j
		LEFT JOIN jira_import_connections c ON j.connection_id = c.id
		ORDER BY j.created_at DESC
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]ImportJobInfo, 0)
	for rows.Next() {
		var job ImportJobInfo
		var instanceURL, instanceName, phase, progressJSON, resultJSON, errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(&job.ID, &job.ConnectionID, &instanceURL, &instanceName, &job.Status,
			&phase, &job.Scope, &progressJSON, &resultJSON, &errorMessage,
			&job.CreatedAt, &startedAt, &completedAt); err != nil {
			slog.Warn("Failed to scan job", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}

		if instanceURL.Valid {
			job.InstanceURL = instanceURL.String
		}
		if instanceName.Valid {
			job.InstanceName = instanceName.String
		}
		if phase.Valid {
			job.Phase = phase.String
		}
		if errorMessage.Valid {
			job.ErrorMessage = errorMessage.String
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}
		if progressJSON.Valid {
			var progress map[string]interface{}
			if err := json.Unmarshal([]byte(progressJSON.String), &progress); err == nil {
				job.Progress = progress
			}
		}
		if resultJSON.Valid {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
				job.Result = result
			}
		}

		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, jobs)
}

// StartImport handles POST /api/admin/jira-import/start
// Starts a background import job and returns immediately with the job ID
func (h *JiraImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[StartImportRequest](w, r)
	if !ok {
		return
	}

	if req.ConnectionID == "" || len(req.ProjectKeys) == 0 {
		respondValidationError(w, r, "connection_id and project_keys are required")
		return
	}

	// Get user ID from context
	userID := getUserIDFromContext(r)

	// Generate a new job ID
	jobID := generateUUID()

	// Store the configuration as JSON
	configJSON, err := json.Marshal(map[string]interface{}{
		"project_keys":     req.ProjectKeys,
		"open_issues_only": req.OpenIssuesOnly,
		"mappings":         req.Mappings,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create the import job in the database
	_, err = h.db.ExecWrite(`
		INSERT INTO jira_import_jobs (id, connection_id, status, scope, config_json, created_by)
		VALUES (?, ?, 'queued', 'work_items', ?, ?)
	`, jobID, req.ConnectionID, string(configJSON), userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionJiraImport,
			ResourceType: logger.ResourceJiraImport,
			ResourceName: jobID,
			Details: map[string]interface{}{
				"connection_id": req.ConnectionID,
				"project_keys":  req.ProjectKeys,
			},
			Success: true,
		})
	}

	// Start the import in a background goroutine
	go h.executeImport(jobID, req)

	respondJSONOK(w, StartImportResponse{
		JobID:   jobID,
		Message: "Import started successfully",
	})
}

// DeleteImportedData handles DELETE /api/admin/jira-import/jobs/{jobId}/data
// Deletes all entities created during an import job for re-import purposes
func (h *JiraImportHandler) DeleteImportedData(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	// Get all mappings for this job, ordered for proper deletion
	rows, err := h.db.Query(`
		SELECT entity_type, windshift_id
		FROM jira_import_id_mappings
		WHERE job_id = ?
		ORDER BY
			CASE entity_type
				WHEN 'link' THEN 1
				WHEN 'comment' THEN 2
				WHEN 'attachment' THEN 3
				WHEN 'item' THEN 4
				WHEN 'milestone' THEN 5
				WHEN 'configuration_set' THEN 6
				WHEN 'workflow' THEN 7
				WHEN 'custom_field' THEN 8
				WHEN 'status' THEN 9
				WHEN 'item_type' THEN 10
				WHEN 'workspace' THEN 11
				ELSE 12
			END
	`, jobID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type mapping struct {
		entityType  string
		windshiftID int
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err = rows.Scan(&m.entityType, &m.windshiftID); err != nil {
			slog.Warn("Failed to scan mapping", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete entities in order (most dependent first)
	deleted := make(map[string]int)
	for _, m := range mappings {
		var tableName string
		switch m.entityType {
		case "item":
			tableName = "items"
		case "workspace":
			tableName = "workspaces"
		case "status":
			tableName = "statuses"
		case "item_type":
			tableName = "item_types"
		case "milestone":
			tableName = "milestones"
		case "custom_field":
			tableName = "custom_fields"
		case "attachment":
			tableName = "attachments"
		case "comment":
			tableName = "comments"
		case "link":
			tableName = "item_links"
		case "configuration_set":
			// Delete dependent rows first
			_, _ = h.db.ExecWrite("DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?", m.windshiftID)
			_, _ = h.db.ExecWrite("DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?", m.windshiftID)
			_, _ = h.db.ExecWrite("DELETE FROM configuration_set_screens WHERE configuration_set_id = ?", m.windshiftID)
			_, _ = h.db.ExecWrite("DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?", m.windshiftID)
			tableName = "configuration_sets"
		case "workflow":
			_, _ = h.db.ExecWrite("DELETE FROM workflow_transitions WHERE workflow_id = ?", m.windshiftID)
			tableName = "workflows"
		default:
			slog.Warn("Unknown entity type", slog.String("component", "jira"), slog.String("entityType", m.entityType))
			continue
		}

		_, err = h.db.ExecWrite(fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName), m.windshiftID) //nolint:gosec // G201: tableName is from the hardcoded whitelist switch above
		if err != nil {
			slog.Error("Failed to delete entity", slog.String("component", "jira"), slog.String("entityType", m.entityType), slog.Int("windshiftID", m.windshiftID), slog.Any("error", err))
		} else {
			deleted[m.entityType]++
		}
	}

	// Clear the mappings for this job
	_, err = h.db.ExecWrite(`DELETE FROM jira_import_id_mappings WHERE job_id = ?`, jobID)
	if err != nil {
		slog.Error("Failed to delete mappings", slog.String("component", "jira"), slog.Any("error", err))
	}

	// Update job status to indicate data was deleted
	if _, err := h.db.ExecWrite(`
		UPDATE jira_import_jobs
		SET status = 'data_deleted', result_json = ?
		WHERE id = ?
	`, fmt.Sprintf(`{"deleted": %v}`, deleted), jobID); err != nil {
		slog.Warn("failed to update job status after data deletion", slog.String("component", "jira"), slog.String("job_id", jobID), slog.Any("error", err))
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"deleted": deleted,
	})
}

// GetPreviousImports handles GET /api/admin/jira-import/previous-imports
// Returns previous imports for the same projects to enable re-import
func (h *JiraImportHandler) GetPreviousImports(w http.ResponseWriter, r *http.Request) {
	projectKeys := r.URL.Query()["project_key"]
	if len(projectKeys) == 0 {
		respondValidationError(w, r, "At least one project_key is required")
		return
	}

	// Query all completed imports and filter by project keys
	rows, err := h.db.Query(`
		SELECT j.id, j.connection_id, j.status, j.config_json, j.created_at, j.completed_at,
		       (SELECT COUNT(*) FROM jira_import_id_mappings m WHERE m.job_id = j.id AND m.entity_type = 'workspace') as workspace_count,
		       (SELECT COUNT(*) FROM jira_import_id_mappings m WHERE m.job_id = j.id AND m.entity_type = 'item') as item_count
		FROM jira_import_jobs j
		WHERE j.status = 'completed'
		ORDER BY j.completed_at DESC
		LIMIT 10
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type previousImport struct {
		JobID          string     `json:"job_id"`
		ConnectionID   string     `json:"connection_id"`
		Status         string     `json:"status"`
		ProjectKeys    []string   `json:"project_keys"`
		WorkspaceCount int        `json:"workspace_count"`
		ItemCount      int        `json:"item_count"`
		CreatedAt      time.Time  `json:"created_at"`
		CompletedAt    *time.Time `json:"completed_at,omitempty"`
	}

	imports := make([]previousImport, 0)
	for rows.Next() {
		var pi previousImport
		var configJSON string
		var completedAt sql.NullTime

		if err := rows.Scan(&pi.JobID, &pi.ConnectionID, &pi.Status, &configJSON,
			&pi.CreatedAt, &completedAt, &pi.WorkspaceCount, &pi.ItemCount); err != nil {
			slog.Warn("Failed to scan import", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}

		if completedAt.Valid {
			pi.CompletedAt = &completedAt.Time
		}

		// Extract project keys from config
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
			if keys, ok := config["project_keys"].([]interface{}); ok {
				for _, k := range keys {
					if str, ok := k.(string); ok {
						pi.ProjectKeys = append(pi.ProjectKeys, str)
					}
				}
			}
		}

		// Check if this import matches any of the requested project keys
		for _, requestedKey := range projectKeys {
			for _, importedKey := range pi.ProjectKeys {
				if requestedKey == importedKey {
					imports = append(imports, pi)
					goto nextRow
				}
			}
		}
	nextRow:
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, imports)
}

// recordMapping records an entity mapping in the database
func (h *JiraImportHandler) recordMapping(jobID, entityType, jiraID, jiraKey string, windshiftID int, metadata map[string]interface{}) {
	metadataJSON := "{}"
	if metadata != nil {
		if data, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(data)
		}
	}

	_, err := h.db.ExecWrite(`
		INSERT INTO jira_import_id_mappings (job_id, entity_type, jira_id, jira_key, windshift_id, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (job_id, entity_type, jira_id) DO UPDATE SET
			windshift_id = excluded.windshift_id,
			metadata_json = excluded.metadata_json
	`, jobID, entityType, jiraID, jiraKey, windshiftID, metadataJSON)
	if err != nil {
		slog.Error("Failed to record mapping", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// updateJobStatus updates the status of an import job
func (h *JiraImportHandler) updateJobStatus(jobID, status, phase string, progress *ImportProgress, errorMessage string) {
	progressJSON := "{}"
	if progress != nil {
		if data, err := json.Marshal(progress); err == nil {
			progressJSON = string(data)
		}
	}

	var query string
	var args []interface{}

	switch status {
	case "running":
		query = `UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{status, phase, progressJSON, jobID}
	case "completed", "failed":
		query = `UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{status, phase, progressJSON, errorMessage, jobID}
	default:
		query = `UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ? WHERE id = ?`
		args = []interface{}{status, phase, progressJSON, jobID}
	}

	_, err := h.db.ExecWrite(query, args...)
	if err != nil {
		slog.Error("Failed to update job status", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// updateJobProgress updates just the progress of a running job
func (h *JiraImportHandler) updateJobProgress(jobID string, progress *ImportProgress) {
	progressJSON := "{}"
	if progress != nil {
		if data, err := json.Marshal(progress); err == nil {
			progressJSON = string(data)
		}
	}

	_, err := h.db.ExecWrite(`
		UPDATE jira_import_jobs SET phase = ?, progress_json = ? WHERE id = ?
	`, progress.Phase, progressJSON, jobID)
	if err != nil {
		slog.Error("Failed to update job progress", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// generateUUID generates a UUID for job IDs
func generateUUID() string {
	return uuid.New().String()
}
