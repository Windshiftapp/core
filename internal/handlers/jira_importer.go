package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sort"
	"strconv"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"

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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

// StartImport handles POST /api/admin/jira-import/start
// Starts a background import job and returns immediately with the job ID
func (h *JiraImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	var req StartImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	// Start the import in a background goroutine
	go h.executeImport(jobID, req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(StartImportResponse{
		JobID:   jobID,
		Message: "Import started successfully",
	})
}

// executeImport runs the actual import process in the background
func (h *JiraImportHandler) executeImport(jobID string, req StartImportRequest) {
	ctx := context.Background()

	// Update job status to running
	h.updateJobStatus(jobID, "running", "initializing", nil, "")

	// Get the Jira client
	client, err := h.getClientForConnection(ctx, req.ConnectionID)
	if err != nil {
		h.updateJobStatus(jobID, "failed", "", nil, fmt.Sprintf("Failed to connect to Jira: %v", err))
		return
	}

	// When JIRA_CAPTURE_PAYLOADS is set, save the request and wrap the client
	captureDir := os.Getenv("JIRA_CAPTURE_PAYLOADS")
	if captureDir != "" {
		if err := os.MkdirAll(captureDir, 0o750); err != nil {
			slog.Error("Failed to create capture directory", slog.String("component", "jira"), slog.Any("error", err))
		} else {
			// Save import_request.json
			reqData, _ := json.MarshalIndent(req, "", "  ")
			if err := os.WriteFile(captureDir+"/import_request.json", reqData, 0o600); err != nil {
				slog.Error("Failed to save import request", slog.String("component", "jira"), slog.Any("error", err))
			}

			// Wrap client in recording client
			rc := newRecordingClient(client)
			client = rc

			// Save responses when import completes (deferred)
			defer func() {
				if err := rc.saveToFile(captureDir); err != nil {
					slog.Error("Failed to save captured payloads", slog.String("component", "jira"), slog.Any("error", err))
				}
			}()
		}
	}

	h.executeImportWithClient(jobID, req, client)
}

// executeImportWithClient runs the import using the provided Jira client.
// Extracted from executeImport to allow testing with a mock client.
func (h *JiraImportHandler) executeImportWithClient(jobID string, req StartImportRequest, client jira.Client) {
	ctx := context.Background()

	progress := &ImportProgress{
		Phase:         "initializing",
		TotalProjects: len(req.ProjectKeys),
	}

	// Calculate total issues
	for _, projectKey := range req.ProjectKeys {
		for _, ws := range req.Mappings.Workspaces {
			if ws.JiraKey == projectKey {
				progress.TotalIssues += ws.IssueCount
				break
			}
		}
	}

	// Create statuses and item types once (global model - shared across all workspaces)
	statusMap, err := h.ensureStatuses(ctx, jobID, req.Mappings.Statuses)
	if err != nil {
		slog.Error("Failed to ensure statuses", slog.String("component", "jira"), slog.Any("error", err))
	}

	itemTypeMap, err := h.ensureItemTypes(ctx, jobID, req.Mappings.IssueTypes)
	if err != nil {
		slog.Error("Failed to ensure item types", slog.String("component", "jira"), slog.Any("error", err))
	}

	// Process each project
	for i, projectKey := range req.ProjectKeys {
		progress.CurrentProject = projectKey
		progress.Phase = "importing_project"
		h.updateJobProgress(jobID, progress)

		// Find the workspace mapping for this project
		var wsMapping *WorkspaceMapping
		for j := range req.Mappings.Workspaces {
			if req.Mappings.Workspaces[j].JiraKey == projectKey {
				wsMapping = &req.Mappings.Workspaces[j]
				break
			}
		}
		if wsMapping == nil {
			slog.Warn("No workspace mapping found for project", slog.String("component", "jira"), slog.String("project", projectKey))
			continue
		}

		// Create or use existing workspace
		workspaceID, err := h.ensureWorkspace(ctx, jobID, wsMapping)
		if err != nil {
			slog.Error("Failed to ensure workspace", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			continue
		}

		// Create workflows and configuration set for this project
		if err = h.ensureWorkflowsAndConfigSet(ctx, jobID, projectKey, workspaceID, statusMap, itemTypeMap, client); err != nil {
			slog.Error("Failed to create workflows/config set", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			// Non-fatal: continue importing
		}

		// Create milestones from version mappings for this project
		var projectVersionMappings []VersionMapping
		for _, vm := range req.Mappings.Versions {
			if vm.ProjectKey == projectKey {
				projectVersionMappings = append(projectVersionMappings, vm)
			}
		}
		versionMap, err := h.ensureMilestones(ctx, jobID, workspaceID, projectVersionMappings)
		if err != nil {
			slog.Error("Failed to ensure milestones", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
		}

		// Import issues for this project
		jql := fmt.Sprintf("project = %s ORDER BY created ASC", projectKey)
		if req.OpenIssuesOnly {
			jql = fmt.Sprintf("project = %s AND statusCategory != Done ORDER BY created ASC", projectKey)
		}

		issueKeys, err := client.GetAllIssueKeys(ctx, jql)
		if err != nil {
			slog.Error("Failed to get issue keys", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			continue
		}

		// Fetch and import issues in batches
		// Track user map across all batches for this project
		userMap := make(map[string]int)

		batchSize := 100
		for j := 0; j < len(issueKeys); j += batchSize {
			end := j + batchSize
			if end > len(issueKeys) {
				end = len(issueKeys)
			}
			batch := issueKeys[j:end]

			// Bulk fetch issues
			fetchResult, err := client.BulkFetchIssues(ctx, jira.BulkFetchRequest{
				IssueIdsOrKeys: batch,
				Fields:         []string{"*all"},
				Expand:         []string{"renderedFields"},
			})
			if err != nil {
				slog.Error("Failed to fetch issues batch", slog.String("component", "jira"), slog.Any("error", err))
				progress.FailedIssues += len(batch)
				continue
			}

			// Collect users from this batch
			var usersToProcess []JiraUserSummary
			usersSeen := make(map[string]bool)
			for _, issue := range fetchResult.Issues {
				// Collect assignee
				if issue.Fields.Assignee != nil && issue.Fields.Assignee.GetIdentifier() != "" {
					userID := issue.Fields.Assignee.GetIdentifier()
					if _, exists := userMap[userID]; !exists && !usersSeen[userID] {
						avatarURL := ""
						if issue.Fields.Assignee.AvatarURLs != nil {
							avatarURL = issue.Fields.Assignee.AvatarURLs["48x48"]
						}
						usersToProcess = append(usersToProcess, JiraUserSummary{
							AccountID:   userID, // Using GetIdentifier() result (AccountID for Cloud, Name/Key for DC)
							Email:       issue.Fields.Assignee.EmailAddress,
							DisplayName: issue.Fields.Assignee.DisplayName,
							AvatarURL:   avatarURL,
						})
						usersSeen[userID] = true
					}
				}
				// Collect reporter
				if issue.Fields.Reporter != nil && issue.Fields.Reporter.GetIdentifier() != "" {
					userID := issue.Fields.Reporter.GetIdentifier()
					if _, exists := userMap[userID]; !exists && !usersSeen[userID] {
						avatarURL := ""
						if issue.Fields.Reporter.AvatarURLs != nil {
							avatarURL = issue.Fields.Reporter.AvatarURLs["48x48"]
						}
						usersToProcess = append(usersToProcess, JiraUserSummary{
							AccountID:   userID, // Using GetIdentifier() result (AccountID for Cloud, Name/Key for DC)
							Email:       issue.Fields.Reporter.EmailAddress,
							DisplayName: issue.Fields.Reporter.DisplayName,
							AvatarURL:   avatarURL,
						})
						usersSeen[userID] = true
					}
				}

				// Collect users from custom user fields (single and multi-user pickers)
				for _, mapping := range req.Mappings.CustomFields {
					if mapping.WindshiftType != "user" && mapping.WindshiftType != "users" {
						continue
					}
					if mapping.Action == "skip" {
						continue
					}

					value, exists := issue.Fields.CustomFields[mapping.JiraID]
					if !exists || value == nil {
						continue
					}

					collectUsersFromCustomField(value, mapping.WindshiftType, userMap, &usersToProcess, usersSeen)
				}
			}

			// Ensure users are created/matched
			if len(usersToProcess) > 0 {
				newUserMappings, err := h.ensureUsers(ctx, jobID, usersToProcess, client)
				if err != nil {
					slog.Error("Failed to ensure users", slog.String("component", "jira"), slog.Any("error", err))
				}
				// Merge new mappings into userMap
				for k, v := range newUserMappings {
					userMap[k] = v
				}
			}

			// Import each issue
			for _, issue := range fetchResult.Issues {
				err := h.importIssue(ctx, jobID, workspaceID, &issue, statusMap, itemTypeMap, userMap, versionMap, req.Mappings.CustomFields, client, progress)
				if err != nil {
					slog.Error("Failed to import issue", slog.String("component", "jira"), slog.String("issue", issue.Key), slog.Any("error", err))
					progress.FailedIssues++
				} else {
					progress.ImportedIssues++
				}
			}

			h.updateJobProgress(jobID, progress)
		}

		// After all issues imported for this project, link parents
		h.linkParents(jobID)

		// After all issues imported for this project, import issue links
		h.importIssueLinks(jobID)

		progress.CompletedProjects = i + 1
	}

	// Mark job as completed
	progress.Phase = "completed"
	h.updateJobStatus(jobID, "completed", "completed", progress, "")
}

// ensureWorkflowsAndConfigSet fetches per-issue-type statuses from Jira,
// creates Windshift workflow(s) with transitions, and assigns a configuration set to the workspace.
func (h *JiraImportHandler) ensureWorkflowsAndConfigSet(
	ctx context.Context, jobID string, projectKey string, workspaceID int,
	statusMap map[string]int, itemTypeMap map[string]int, client jira.Client,
) error {
	// Check if workspace already has a configuration set
	csRepo := repository.NewConfigurationSetRepository(h.db)
	existingCSID, err := csRepo.GetWorkspaceConfigSetID(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to check existing config set: %w", err)
	}
	if existingCSID != nil {
		slog.Info("Workspace already has a configuration set, skipping",
			slog.String("component", "jira"), slog.Int("workspaceID", workspaceID), slog.Int("configSetID", *existingCSID))
		return nil
	}

	// Fetch per-issue-type statuses from Jira
	issueTypeStatuses, err := client.GetProjectIssueTypeStatuses(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project issue type statuses: %w", err)
	}

	// Map Jira issue types and statuses to Windshift IDs
	// Group item types by their set of statuses
	type issueTypeInfo struct {
		windshiftItemTypeID int
		windshiftStatusIDs  []int
		jiraName            string
	}
	var issueTypeInfos []issueTypeInfo

	for _, its := range issueTypeStatuses {
		wsItemTypeID, ok := itemTypeMap[its.ID]
		if !ok {
			continue
		}

		// Map statuses to Windshift IDs
		statusIDSet := make(map[int]bool)
		for _, s := range its.Statuses {
			if wsStatusID, ok := statusMap[s.ID]; ok {
				statusIDSet[wsStatusID] = true
			}
		}
		if len(statusIDSet) == 0 {
			continue
		}

		var statusIDs []int
		for id := range statusIDSet {
			statusIDs = append(statusIDs, id)
		}
		sort.Ints(statusIDs)

		issueTypeInfos = append(issueTypeInfos, issueTypeInfo{
			windshiftItemTypeID: wsItemTypeID,
			windshiftStatusIDs:  statusIDs,
			jiraName:            its.Name,
		})
	}

	if len(issueTypeInfos) == 0 {
		slog.Warn("No issue types with mapped statuses found, skipping workflow creation",
			slog.String("component", "jira"), slog.String("project", projectKey))
		return nil
	}

	// Group item types by status set (sorted comma-joined IDs as key)
	type workflowGroup struct {
		statusIDs   []int
		itemTypeIDs []int
		typeNames   []string
	}
	groups := make(map[string]*workflowGroup)

	for _, info := range issueTypeInfos {
		// Build key from sorted status IDs
		parts := make([]string, len(info.windshiftStatusIDs))
		for i, id := range info.windshiftStatusIDs {
			parts[i] = strconv.Itoa(id)
		}
		key := strings.Join(parts, ",")

		if g, ok := groups[key]; ok {
			g.itemTypeIDs = append(g.itemTypeIDs, info.windshiftItemTypeID)
			g.typeNames = append(g.typeNames, info.jiraName)
		} else {
			groups[key] = &workflowGroup{
				statusIDs:   info.windshiftStatusIDs,
				itemTypeIDs: []int{info.windshiftItemTypeID},
				typeNames:   []string{info.jiraName},
			}
		}
	}

	// Determine which status IDs have category_id = 1 (To Do/New) for initial transitions
	newStatusIDs := make(map[int]bool)
	for _, statusIDs := range groups {
		for _, sid := range statusIDs.statusIDs {
			var catID int
			err = h.db.QueryRow("SELECT category_id FROM statuses WHERE id = ?", sid).Scan(&catID)
			if err == nil && catID == 1 {
				newStatusIDs[sid] = true
			}
		}
	}

	// Create workflow(s)
	multipleWorkflows := len(groups) > 1
	type createdWorkflow struct {
		workflowID  int
		itemTypeIDs []int
	}
	var workflows []createdWorkflow

	for _, group := range groups {
		// Build workflow name
		var wfName string
		if multipleWorkflows {
			wfName = projectKey + " - " + strings.Join(group.typeNames, ", ") + " Workflow"
		} else {
			wfName = projectKey + " Workflow"
		}

		// Insert workflow
		var workflowID int
		err = h.db.QueryRow(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES (?, '', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, wfName).Scan(&workflowID)
		if err != nil {
			return fmt.Errorf("failed to create workflow: %w", err)
		}

		// Create transitions
		order := 0

		// Initial transitions: NULL -> status where category_id = 1
		for _, sid := range group.statusIDs {
			if newStatusIDs[sid] {
				order++
				_, _ = h.db.ExecWrite(`
					INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
					VALUES (?, NULL, ?, ?, '', '', CURRENT_TIMESTAMP)
				`, workflowID, sid, order)
			}
		}

		// All-to-all transitions
		for _, fromID := range group.statusIDs {
			for _, toID := range group.statusIDs {
				if fromID != toID {
					order++
					_, _ = h.db.ExecWrite(`
						INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
						VALUES (?, ?, ?, ?, '', '', CURRENT_TIMESTAMP)
					`, workflowID, fromID, toID, order)
				}
			}
		}

		h.recordMapping(jobID, "workflow", fmt.Sprintf("wf-%s-%d", projectKey, workflowID), wfName, workflowID, nil)
		workflows = append(workflows, createdWorkflow{workflowID: workflowID, itemTypeIDs: group.itemTypeIDs})
	}

	// Pick default workflow (the one used by the most item types)
	defaultWfIdx := 0
	maxTypes := 0
	for i, wf := range workflows {
		if len(wf.itemTypeIDs) > maxTypes {
			maxTypes = len(wf.itemTypeIDs)
			defaultWfIdx = i
		}
	}
	defaultWfID := workflows[defaultWfIdx].workflowID

	// Create configuration set in a transaction
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	csName := projectKey + " Configuration"
	cs := &models.ConfigurationSet{
		Name:                    csName,
		WorkflowID:              &defaultWfID,
		DifferentiateByItemType: multipleWorkflows,
	}
	csID, err := csRepo.Create(tx, cs)
	if err != nil {
		return fmt.Errorf("failed to create configuration set: %w", err)
	}
	configSetID := int(csID)

	// Save item type configs with per-type workflow overrides
	var itemTypeConfigs []models.ItemTypeConfig
	for _, wf := range workflows {
		for _, itemTypeID := range wf.itemTypeIDs {
			config := models.ItemTypeConfig{
				ItemTypeID: itemTypeID,
			}
			// Only set workflow override if it differs from default
			if wf.workflowID != defaultWfID {
				wfID := wf.workflowID
				config.WorkflowID = &wfID
			}
			itemTypeConfigs = append(itemTypeConfigs, config)
		}
	}
	if err := csRepo.SaveItemTypeConfigs(tx, configSetID, itemTypeConfigs); err != nil {
		return fmt.Errorf("failed to save item type configs: %w", err)
	}

	// Assign workspace
	if err := csRepo.SaveWorkspaceAssignments(tx, configSetID, []int{workspaceID}); err != nil {
		return fmt.Errorf("failed to save workspace assignment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit configuration set: %w", err)
	}

	h.recordMapping(jobID, "configuration_set", fmt.Sprintf("cs-%s", projectKey), csName, configSetID, nil)

	slog.Info("Created workflows and configuration set for import",
		slog.String("component", "jira"),
		slog.String("project", projectKey),
		slog.Int("workflows", len(workflows)),
		slog.Int("configSetID", configSetID))

	return nil
}

// ensureWorkspace creates or finds a workspace for import
func (h *JiraImportHandler) ensureWorkspace(_ context.Context, jobID string, mapping *WorkspaceMapping) (int, error) {
	if !mapping.CreateNew && mapping.WindshiftID != nil {
		return *mapping.WindshiftID, nil
	}

	workspaceSvc := services.NewWorkspaceService(h.db)

	// Check if workspace already exists by key
	var existingID int
	err := h.db.QueryRow(`SELECT id FROM workspaces WHERE key = ?`, mapping.NewWorkspaceKey).Scan(&existingID)
	if err == nil {
		// Workspace exists, return existing ID
		h.recordMapping(jobID, "workspace", mapping.JiraKey, mapping.JiraKey, existingID, nil)
		return existingID, nil
	}

	// Create new workspace using service
	result, err := workspaceSvc.Create(services.CreateWorkspaceParams{
		Name:        mapping.NewWorkspaceName,
		Key:         mapping.NewWorkspaceKey,
		Description: "Imported from Jira",
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Record the mapping
	h.recordMapping(jobID, "workspace", mapping.JiraKey, mapping.JiraKey, result.Workspace.ID, nil)

	return result.Workspace.ID, nil
}

// ensureMilestones creates milestones for Jira versions in a workspace
// Returns a map from Jira version ID to Windshift milestone ID
//
//nolint:unparam // error return kept for interface consistency with other ensure* methods
func (h *JiraImportHandler) ensureMilestones(_ context.Context, jobID string, workspaceID int, mappings []VersionMapping) (map[string]int, error) {
	result := make(map[string]int)
	planningSvc := services.NewPlanningService(h.db)

	for _, m := range mappings {
		if !m.CreateNew {
			continue
		}

		// Check if milestone already exists by name in this workspace
		var existingID int
		err := h.db.QueryRow(`SELECT id FROM milestones WHERE name = ? AND workspace_id = ?`, m.JiraName, workspaceID).Scan(&existingID)
		if err == nil {
			result[m.JiraID] = existingID
			h.recordMapping(jobID, "milestone", m.JiraID, m.JiraName, existingID, nil)
			continue
		}

		// Determine status based on released flag
		status := "planning"
		if m.Released {
			status = "completed"
		}

		// Create milestone
		milestone, err := planningSvc.CreateMilestone(services.CreateMilestoneParams{
			Name:        m.JiraName,
			TargetDate:  m.ReleaseDate,
			Status:      status,
			IsGlobal:    false,
			WorkspaceID: &workspaceID,
		})
		if err != nil {
			slog.Error("Failed to create milestone", slog.String("component", "jira"), slog.String("version", m.JiraName), slog.Any("error", err))
			continue
		}

		result[m.JiraID] = milestone.ID
		h.recordMapping(jobID, "milestone", m.JiraID, m.JiraName, milestone.ID, nil)
	}

	return result, nil
}

// ensureStatuses creates or maps statuses (global model - shared across workspaces)
//
//nolint:unparam // error return kept for interface consistency with other ensure* methods
func (h *JiraImportHandler) ensureStatuses(_ context.Context, jobID string, mappings []StatusMapping) (map[string]int, error) {
	result := make(map[string]int)
	statusSvc := services.NewEnumService(h.db, services.NewStatusConfig())

	for _, m := range mappings {
		if !m.CreateNew && m.WindshiftID != nil {
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = *m.WindshiftID
			}
			continue
		}

		// Map Jira category to Windshift category ID
		// Default category IDs: 1="To Do", 2="In Progress", 3="Done"
		categoryID := 1
		switch m.CategoryKey {
		case "new":
			categoryID = 1
		case "indeterminate":
			categoryID = 2
		case "done":
			categoryID = 3
		}

		// Check if status already exists by name
		var existingID int
		err := h.db.QueryRow(`SELECT id FROM statuses WHERE name = ?`, m.JiraName).Scan(&existingID)
		if err == nil {
			// Status exists, use existing ID
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = existingID
			}
			if len(m.JiraIDs) > 0 {
				h.recordMapping(jobID, "status", m.JiraIDs[0], m.JiraName, existingID, nil)
			}
			continue
		}

		// Create new status using service
		status := &models.Status{
			Name:       m.JiraName,
			CategoryID: categoryID,
		}
		entity, err := statusSvc.Create(status, nil)
		if err != nil {
			slog.Error("Failed to create status", slog.String("component", "jira"), slog.String("status", m.JiraName), slog.Any("error", err))
			continue
		}

		statusID := entity.GetID()
		for _, jiraID := range m.JiraIDs {
			result[jiraID] = statusID
		}

		// Record the mapping
		if len(m.JiraIDs) > 0 {
			h.recordMapping(jobID, "status", m.JiraIDs[0], m.JiraName, statusID, nil)
		}
	}

	return result, nil
}

// ensureItemTypes creates or maps item types (global model - shared across workspaces)
//
//nolint:unparam // error return kept for interface consistency with other ensure* methods
func (h *JiraImportHandler) ensureItemTypes(_ context.Context, jobID string, mappings []IssueTypeMapping) (map[string]int, error) {
	result := make(map[string]int)
	itemTypeSvc := services.NewEnumService(h.db, services.NewItemTypeConfig())

	for _, m := range mappings {
		if !m.CreateNew && m.WindshiftID != nil {
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = *m.WindshiftID
			}
			continue
		}

		// Check if item type already exists by name
		var existingID int
		err := h.db.QueryRow(`SELECT id FROM item_types WHERE name = ?`, m.JiraName).Scan(&existingID)
		if err == nil {
			// Item type exists, use existing ID
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = existingID
			}
			if len(m.JiraIDs) > 0 {
				h.recordMapping(jobID, "item_type", m.JiraIDs[0], m.JiraName, existingID, nil)
			}
			continue
		}

		// Create new item type using service
		itemType := &models.ItemType{
			Name:           m.JiraName,
			Icon:           "Circle",
			Color:          "#3B82F6",
			HierarchyLevel: m.HierarchyLevel,
		}
		entity, err := itemTypeSvc.Create(itemType, nil)
		if err != nil {
			slog.Error("Failed to create item type", slog.String("component", "jira"), slog.String("itemType", m.JiraName), slog.Any("error", err))
			continue
		}

		itemTypeID := entity.GetID()
		for _, jiraID := range m.JiraIDs {
			result[jiraID] = itemTypeID
		}

		// Record the mapping
		if len(m.JiraIDs) > 0 {
			h.recordMapping(jobID, "item_type", m.JiraIDs[0], m.JiraName, itemTypeID, nil)
		}
	}

	return result, nil
}

// ensureUsers matches or creates users for import
// Returns a map from Jira account ID to Windshift user ID
// Fetches missing emails via the Jira API when needed
func (h *JiraImportHandler) ensureUsers(ctx context.Context, jobID string, users []JiraUserSummary, client jira.Client) (map[string]int, error) { //nolint:unparam // error return kept for API consistency
	result := make(map[string]int)

	// First pass: fetch missing emails via API
	for i := range users {
		if users[i].AccountID == "" {
			continue
		}
		if users[i].Email != "" {
			continue // Already have email
		}

		// Try to fetch email via API
		email, err := client.GetUserEmail(ctx, users[i].AccountID)
		if err != nil {
			slog.Debug("Failed to fetch email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.Any("error", err))
		} else if email != "" {
			users[i].Email = email
			slog.Debug("Fetched email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.String("email", email))
		}
	}

	// Second pass: create/match users
	for _, u := range users {
		// Skip users without account ID
		if u.AccountID == "" {
			continue
		}

		// Skip users without email - they can't be matched later anyway
		// and empty emails cause UNIQUE constraint violations
		if u.Email == "" {
			slog.Debug("Skipping user without email", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("accountID", u.AccountID))
			continue
		}

		// Check if we already have a mapping for this user in this job
		var existingUserID int
		err := h.db.QueryRow(`
			SELECT windshift_user_id FROM jira_import_user_mappings
			WHERE job_id = ? AND jira_account_id = ?
		`, jobID, u.AccountID).Scan(&existingUserID)
		if err == nil {
			result[u.AccountID] = existingUserID
			continue
		}

		// Try to find existing Windshift user by email
		var userID int
		if u.Email != "" {
			err = h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, u.Email).Scan(&userID)
			if err == nil {
				// Found existing user
				result[u.AccountID] = userID
				h.recordUserMapping(jobID, u, userID, false)
				continue
			}
		}

		// Create new inactive user
		firstName, lastName := parseDisplayName(u.DisplayName)
		username := generateUsername(u.Email, u.DisplayName)

		var newUserID int64
		err = h.db.QueryRow(`
			INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, created_at, updated_at)
			VALUES (?, ?, ?, ?, false, ?, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, u.Email, username, firstName, lastName, u.AvatarURL).Scan(&newUserID)
		if err != nil {
			slog.Error("Failed to create user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Any("error", err))
			continue
		}

		result[u.AccountID] = int(newUserID)
		h.recordUserMapping(jobID, u, int(newUserID), true)

		slog.Debug("Created user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Int64("userID", newUserID))
	}

	return result, nil
}

// recordUserMapping stores a Jira user to Windshift user mapping
func (h *JiraImportHandler) recordUserMapping(jobID string, user JiraUserSummary, windshiftUserID int, wasCreated bool) {
	_, err := h.db.ExecWrite(`
		INSERT INTO jira_import_user_mappings (job_id, jira_account_id, jira_email, jira_display_name, windshift_user_id, was_created)
		VALUES (?, ?, ?, ?, ?, ?)
	`, jobID, user.AccountID, user.Email, user.DisplayName, windshiftUserID, wasCreated)
	if err != nil {
		slog.Error("Failed to record user mapping", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// parseDisplayName splits a display name into first and last name
func parseDisplayName(displayName string) (firstName, lastName string) {
	parts := strings.SplitN(strings.TrimSpace(displayName), " ", 2)
	if len(parts) >= 1 {
		firstName = parts[0]
	}
	if len(parts) >= 2 {
		lastName = parts[1]
	}
	if firstName == "" {
		firstName = "Imported"
	}
	if lastName == "" {
		lastName = "User"
	}
	return
}

// generateUsername creates a unique username from email or display name
func generateUsername(email, displayName string) string {
	// Try to use email prefix first
	if email != "" {
		parts := strings.Split(email, "@")
		if len(parts) > 0 && parts[0] != "" {
			return strings.ToLower(parts[0])
		}
	}
	// Fall back to display name
	if displayName != "" {
		return strings.ToLower(strings.ReplaceAll(displayName, " ", "."))
	}
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

// collectUsersFromCustomField extracts users from a custom field value
func collectUsersFromCustomField(value interface{}, fieldType string,
	existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	switch fieldType {
	case "user":
		if userObj, ok := value.(map[string]interface{}); ok {
			addUserFromObject(userObj, existingMap, usersToProcess, seen)
		}
	case "users":
		if users, ok := value.([]interface{}); ok {
			for _, u := range users {
				if userObj, ok := u.(map[string]interface{}); ok {
					addUserFromObject(userObj, existingMap, usersToProcess, seen)
				}
			}
		}
	}
}

// addUserFromObject extracts user data from a Jira user object and adds it to the processing list
func addUserFromObject(userObj map[string]interface{}, existingMap map[string]int,
	usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	accountID, _ := userObj["accountId"].(string)
	if accountID == "" {
		return
	}
	if _, exists := existingMap[accountID]; exists {
		return
	}
	if seen[accountID] {
		return
	}

	email, _ := userObj["emailAddress"].(string)
	displayName, _ := userObj["displayName"].(string)
	avatarURL := ""
	if avatars, ok := userObj["avatarUrls"].(map[string]interface{}); ok {
		avatarURL, _ = avatars["48x48"].(string)
	}

	*usersToProcess = append(*usersToProcess, JiraUserSummary{
		AccountID:   accountID,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	})
	seen[accountID] = true
}

// importIssue imports a single Jira issue as a Windshift work item
func (h *JiraImportHandler) importIssue(ctx context.Context, jobID string, workspaceID int, issue *jira.JiraIssue, statusMap, itemTypeMap, userMap, versionMap map[string]int, customFieldMappings []CustomFieldMapping, client jira.Client, progress *ImportProgress) error {
	// Get mapped status and item type (use nil instead of 0 for missing mappings)
	var statusID *int
	if issue.Fields.Status != nil {
		if sid, ok := statusMap[issue.Fields.Status.ID]; ok {
			statusID = &sid
		}
	}

	var itemTypeID *int
	if issue.Fields.IssueType != nil {
		if tid, ok := itemTypeMap[issue.Fields.IssueType.ID]; ok {
			itemTypeID = &tid
		}
	}

	// Map assignee and reporter
	var assigneeID *int
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Assignee.GetIdentifier()]; ok {
			assigneeID = &uid
		}
	}

	var reporterID *int
	if issue.Fields.Reporter != nil && issue.Fields.Reporter.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Reporter.GetIdentifier()]; ok {
			reporterID = &uid
		}
	}

	// Map fixVersion to milestone (use first version)
	var milestoneID *int
	if len(issue.Fields.FixVersions) > 0 {
		if mid, ok := versionMap[issue.Fields.FixVersions[0].ID]; ok {
			milestoneID = &mid
		}
	}

	// Map priority
	var priorityName string
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != "" {
		priorityName = issue.Fields.Priority.Name
	}

	// Parse due date
	var dueDate *time.Time
	if issue.Fields.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
			dueDate = &parsed
		}
	}

	// Convert description from ADF to markdown
	description := ""
	if issue.Fields.Description != nil {
		description = jira.ConvertADFToMarkdown(issue.Fields.Description)
	}

	// Process custom fields (user/users types only for now)
	customFieldValues := make(map[string]interface{})
	for _, mapping := range customFieldMappings {
		if mapping.Action == "skip" {
			continue
		}

		// Only process user/users types for now
		if mapping.WindshiftType != "user" && mapping.WindshiftType != "users" {
			continue
		}

		value, exists := issue.Fields.CustomFields[mapping.JiraID]
		if !exists || value == nil {
			continue
		}

		switch mapping.WindshiftType {
		case "user":
			// Single user picker
			if userObj, ok := value.(map[string]interface{}); ok {
				if accountID, ok := userObj["accountId"].(string); ok {
					if uid, ok := userMap[accountID]; ok {
						customFieldValues[mapping.JiraID] = uid
					}
				}
			}
		case "users":
			// Multi-user picker (like Approvers)
			if users, ok := value.([]interface{}); ok {
				var userIDs []int
				for _, u := range users {
					if userObj, ok := u.(map[string]interface{}); ok {
						if accountID, ok := userObj["accountId"].(string); ok {
							if uid, ok := userMap[accountID]; ok {
								userIDs = append(userIDs, uid)
							}
						}
					}
				}
				if len(userIDs) > 0 {
					customFieldValues[mapping.JiraID] = userIDs
				}
			}
		}
	}

	// Serialize custom field values to JSON
	customFieldValuesJSON := ""
	if len(customFieldValues) > 0 {
		if jsonBytes, err := json.Marshal(customFieldValues); err == nil {
			customFieldValuesJSON = string(jsonBytes)
		}
	}

	// Create the work item using centralized service (handles workspace_item_number, frac_index, etc.)
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:           workspaceID,
		Title:                 issue.Fields.Summary,
		Description:           description,
		StatusID:              statusID,
		ItemTypeID:            itemTypeID,
		Priority:              priorityName,
		DueDate:               dueDate,
		AssigneeID:            assigneeID,
		ReporterID:            reporterID,
		MilestoneID:           milestoneID,
		CustomFieldValuesJSON: customFieldValuesJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Build metadata for the mapping (includes parent key for later linking)
	meta := map[string]interface{}{
		"summary": issue.Fields.Summary,
	}
	if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		meta["parent_key"] = issue.Fields.Parent.Key
	}
	if len(issue.Fields.IssueLinks) > 0 {
		var links []map[string]interface{}
		for _, link := range issue.Fields.IssueLinks {
			entry := map[string]interface{}{}
			if link.Type != nil {
				entry["type_name"] = link.Type.Name
				entry["inward"] = link.Type.Inward
				entry["outward"] = link.Type.Outward
			}
			if link.InwardIssue != nil {
				entry["inward_key"] = link.InwardIssue.Key
			}
			if link.OutwardIssue != nil {
				entry["outward_key"] = link.OutwardIssue.Key
			}
			links = append(links, entry)
		}
		meta["issue_links"] = links
	}

	// Record the mapping
	h.recordMapping(jobID, "item", issue.ID, issue.Key, int(itemID), meta)

	// Import comments for this issue
	h.importComments(jobID, int(itemID), issue, userMap)

	// Import attachments for this issue
	h.importAttachments(ctx, jobID, int(itemID), issue, userMap, client, progress)

	return nil
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(imports)
}

// ================================================================
// Phase 3: Parent/Hierarchy Linking
// ================================================================

// linkParents sets parent_id on imported items whose Jira issue had a parent field.
// Must be called after all issues for a project are imported so that both
// parent and child exist in jira_import_id_mappings.
func (h *JiraImportHandler) linkParents(jobID string) {
	// Find all item mappings that have a parent_key in metadata
	rows, err := h.db.Query(`
		SELECT windshift_id, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for parent linking", slog.String("component", "jira"), slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type parentLink struct {
		childID   int
		parentKey string
	}
	var links []parentLink

	for rows.Next() {
		var windshiftID int
		var metadataJSON sql.NullString
		if err := rows.Scan(&windshiftID, &metadataJSON); err != nil {
			continue
		}
		if !metadataJSON.Valid {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &meta); err != nil {
			continue
		}
		if parentKey, ok := meta["parent_key"].(string); ok && parentKey != "" {
			links = append(links, parentLink{childID: windshiftID, parentKey: parentKey})
		}
	}

	for _, link := range links {
		// Look up parent's Windshift ID from mappings
		var parentID int
		err := h.db.QueryRow(`
			SELECT windshift_id FROM jira_import_id_mappings
			WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
		`, jobID, link.parentKey).Scan(&parentID)
		if err != nil {
			slog.Debug("Parent not found in import mappings",
				slog.String("component", "jira"),
				slog.String("parentKey", link.parentKey),
				slog.Int("childID", link.childID))
			continue
		}

		// Update the child item's parent_id directly.
		// We cannot use ItemUpdateService here because it requires a valid user ID
		// for history tracking, and the import runs without a user context.
		_, err = h.db.ExecWrite(`UPDATE items SET parent_id = ? WHERE id = ?`, parentID, link.childID)
		if err != nil {
			slog.Error("Failed to set parent_id",
				slog.String("component", "jira"),
				slog.Int("childID", link.childID),
				slog.Int("parentID", parentID),
				slog.Any("error", err))
		}
	}
}

// ================================================================
// Phase 4: Comment Import
// ================================================================

// importComments imports comments from a Jira issue into Windshift
func (h *JiraImportHandler) importComments(jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int) {
	if issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) == 0 {
		return
	}

	// Create a CommentService without notification/webhook/mention services
	// so bulk import doesn't generate notifications
	commentSvc := services.NewCommentService(h.db)

	for _, comment := range issue.Fields.Comment.Comments {
		content := jira.ConvertADFToMarkdown(comment.Body)
		if content == "" {
			continue
		}

		authorID := 0
		if comment.Author != nil && comment.Author.GetIdentifier() != "" {
			if uid, ok := userMap[comment.Author.GetIdentifier()]; ok {
				authorID = uid
			}
		}

		// Parse created timestamp
		var createdAt *time.Time
		if comment.Created != "" {
			if parsed, err := time.Parse("2006-01-02T15:04:05.000-0700", comment.Created); err == nil {
				createdAt = &parsed
			} else if parsed, err := time.Parse("2006-01-02T15:04:05.000Z0700", comment.Created); err == nil {
				createdAt = &parsed
			}
		}

		result, err := commentSvc.Create(services.CreateCommentParams{
			ItemID:      itemID,
			AuthorID:    authorID,
			Content:     content,
			ActorUserID: authorID,
			CreatedAt:   createdAt,
		})
		if err != nil {
			slog.Error("Failed to import comment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("commentID", comment.ID),
				slog.Any("error", err))
			continue
		}

		h.recordMapping(jobID, "comment", comment.ID, issue.Key, int(result.CommentID), nil)
	}
}

// ================================================================
// Phase 5: Issue Link Import
// ================================================================

// importIssueLinks creates item_links from Jira issue links stored in mapping metadata.
// Must be called after all issues for a project are imported.
func (h *JiraImportHandler) importIssueLinks(jobID string) {
	// Query all item mappings with issue_links metadata
	rows, err := h.db.Query(`
		SELECT windshift_id, jira_key, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for link import", slog.String("component", "jira"), slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type issueLinkInfo struct {
		sourceID  int
		sourceKey string
		links     []map[string]interface{}
	}
	var allLinks []issueLinkInfo

	for rows.Next() {
		var windshiftID int
		var jiraKey string
		var metadataJSON sql.NullString
		if err := rows.Scan(&windshiftID, &jiraKey, &metadataJSON); err != nil {
			continue
		}
		if !metadataJSON.Valid {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &meta); err != nil {
			continue
		}
		linksRaw, ok := meta["issue_links"].([]interface{})
		if !ok || len(linksRaw) == 0 {
			continue
		}
		var links []map[string]interface{}
		for _, l := range linksRaw {
			if m, ok := l.(map[string]interface{}); ok {
				links = append(links, m)
			}
		}
		if len(links) > 0 {
			allLinks = append(allLinks, issueLinkInfo{sourceID: windshiftID, sourceKey: jiraKey, links: links})
		}
	}

	// Cache link type lookups
	linkTypeCache := make(map[string]int) // link type name -> ID
	linkSvc := services.NewItemLinkService(h.db)

	for _, info := range allLinks {
		for _, link := range info.links {
			typeName, _ := link["type_name"].(string)
			if typeName == "" {
				continue
			}

			// Determine source and target
			// For outward links: this issue is the source, outward_key is target
			// For inward links: inward_key is the source, this issue is target
			// We only process outward links to avoid duplicates
			outwardKey, _ := link["outward_key"].(string)
			if outwardKey == "" {
				continue
			}

			// Look up target Windshift ID
			var targetID int
			err := h.db.QueryRow(`
				SELECT windshift_id FROM jira_import_id_mappings
				WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
			`, jobID, outwardKey).Scan(&targetID)
			if err != nil {
				// Target issue not imported (different project or not selected)
				continue
			}

			// Ensure link type exists
			linkTypeID, ok := linkTypeCache[typeName]
			if !ok {
				linkTypeID, err = h.ensureLinkType(typeName, link)
				if err != nil {
					slog.Error("Failed to ensure link type",
						slog.String("component", "jira"),
						slog.String("typeName", typeName),
						slog.Any("error", err))
					continue
				}
				linkTypeCache[typeName] = linkTypeID
			}

			// Create item link via service (handles duplicate check)
			linkID, err := linkSvc.CreateLink(services.CreateItemLinkParams{
				LinkTypeID: linkTypeID,
				SourceType: "item",
				SourceID:   info.sourceID,
				TargetType: "item",
				TargetID:   targetID,
			})
			if err != nil {
				slog.Error("Failed to create item link",
					slog.String("component", "jira"),
					slog.String("source", info.sourceKey),
					slog.String("target", outwardKey),
					slog.Any("error", err))
				continue
			}

			if linkID > 0 {
				h.recordMapping(jobID, "link", fmt.Sprintf("%s-%s-%s", info.sourceKey, typeName, outwardKey), "", int(linkID), nil)
			}
		}
	}
}

// ensureLinkType finds or creates a link type matching the Jira link type
func (h *JiraImportHandler) ensureLinkType(typeName string, linkData map[string]interface{}) (int, error) {
	// Try to find existing by name
	var existingID int
	err := h.db.QueryRow(`SELECT id FROM link_types WHERE name = ?`, typeName).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	// Create new link type
	forwardLabel, _ := linkData["outward"].(string)
	reverseLabel, _ := linkData["inward"].(string)
	if forwardLabel == "" {
		forwardLabel = typeName
	}
	if reverseLabel == "" {
		reverseLabel = typeName
	}

	linkTypeSvc := services.NewEnumService(h.db, services.NewLinkTypeConfig())
	linkType := &models.LinkType{
		Name:         typeName,
		ForwardLabel: forwardLabel,
		ReverseLabel: reverseLabel,
		Color:        "#6B7280",
		Active:       true,
	}
	entity, err := linkTypeSvc.Create(linkType, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create link type: %w", err)
	}
	return entity.GetID(), nil
}

// ================================================================
// Phase 6: Attachment Import
// ================================================================

// importAttachments downloads and stores attachments from a Jira issue
func (h *JiraImportHandler) importAttachments(ctx context.Context, jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int, client jira.Client, progress *ImportProgress) {
	if len(issue.Fields.Attachment) == 0 {
		return
	}

	// Get attachment storage path from settings
	var attachmentPath string
	err := h.db.QueryRow(`SELECT attachment_path FROM attachment_settings WHERE enabled = true LIMIT 1`).Scan(&attachmentPath)
	if err != nil || attachmentPath == "" {
		slog.Warn("Attachment settings not configured, skipping attachment import",
			slog.String("component", "jira"), slog.String("issue", issue.Key))
		return
	}

	for _, attachment := range issue.Fields.Attachment {
		if attachment.Content == "" {
			continue
		}

		progress.TotalAttachments++

		// Download the attachment
		reader, _, err := client.DownloadAttachment(ctx, attachment.Content)
		if err != nil {
			slog.Error("Failed to download attachment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		// Generate a unique filename to avoid collisions
		storedFilename := fmt.Sprintf("%s_%s", uuid.New().String(), attachment.Filename)
		filePath := filepath.Join(attachmentPath, storedFilename)

		// Save to disk
		file, err := os.Create(filePath)
		if err != nil {
			_ = reader.Close()
			slog.Error("Failed to create attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		written, err := io.Copy(file, reader)
		_ = file.Close()
		_ = reader.Close()
		if err != nil {
			_ = os.Remove(filePath)
			slog.Error("Failed to write attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		// Use actual written size if Jira didn't report one
		fileSize := attachment.Size
		if fileSize == 0 {
			fileSize = written
		}

		// Map uploader
		var uploadedBy *int
		if attachment.Author != nil && attachment.Author.GetIdentifier() != "" {
			if uid, ok := userMap[attachment.Author.GetIdentifier()]; ok {
				uploadedBy = &uid
			}
		}

		mimeType := attachment.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		// Insert attachment record via service
		attachmentSvc := services.NewAttachmentService(h.db)
		attachmentID, err := attachmentSvc.CreateRecord(services.CreateAttachmentParams{
			ItemID:           itemID,
			EntityType:       "item",
			Filename:         storedFilename,
			OriginalFilename: attachment.Filename,
			FilePath:         filePath,
			MimeType:         mimeType,
			FileSize:         fileSize,
			UploadedBy:       uploadedBy,
		})
		if err != nil {
			slog.Error("Failed to insert attachment record",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		h.recordMapping(jobID, "attachment", attachment.ID, issue.Key, int(attachmentID), nil)
		progress.ImportedAttachments++
	}
}
