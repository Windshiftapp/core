package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/jira"

	"github.com/google/uuid"
)

// GetJobStatus handles GET /api/jira-import/jobs/{jobId}
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
		http.Error(w, "Job not found", http.StatusNotFound)
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
	json.NewEncoder(w).Encode(response)
}

// GetImportJobs handles GET /api/jira-import/jobs
func (h *JiraImportHandler) GetImportJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT j.id, j.connection_id, c.instance_url, c.instance_name, j.status, j.phase, j.scope,
		       j.progress_json, j.result_json, j.error_message, j.created_at, j.started_at, j.completed_at
		FROM jira_import_jobs j
		LEFT JOIN jira_import_connections c ON j.connection_id = c.id
		ORDER BY j.created_at DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list import jobs: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(jobs)
}

// StartImport handles POST /api/jira-import/start
// Starts a background import job and returns immediately with the job ID
func (h *JiraImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	var req StartImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectionID == "" || len(req.ProjectKeys) == 0 {
		http.Error(w, "connection_id and project_keys are required", http.StatusBadRequest)
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
		http.Error(w, "Failed to serialize config", http.StatusInternalServerError)
		return
	}

	// Create the import job in the database
	_, err = h.db.ExecWrite(`
		INSERT INTO jira_import_jobs (id, connection_id, status, scope, config_json, created_by)
		VALUES (?, ?, 'queued', 'work_items', ?, ?)
	`, jobID, req.ConnectionID, string(configJSON), userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create import job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the import in a background goroutine
	go h.executeImport(jobID, req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(StartImportResponse{
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

		// Create statuses, item types, and custom fields for this workspace
		statusMap, err := h.ensureStatuses(ctx, jobID, workspaceID, req.Mappings.Statuses)
		if err != nil {
			slog.Error("Failed to ensure statuses", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
		}

		itemTypeMap, err := h.ensureItemTypes(ctx, jobID, workspaceID, req.Mappings.IssueTypes)
		if err != nil {
			slog.Error("Failed to ensure item types", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
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
				if issue.Fields.Assignee != nil && issue.Fields.Assignee.AccountID != "" {
					if _, exists := userMap[issue.Fields.Assignee.AccountID]; !exists && !usersSeen[issue.Fields.Assignee.AccountID] {
						avatarURL := ""
						if issue.Fields.Assignee.AvatarURLs != nil {
							avatarURL = issue.Fields.Assignee.AvatarURLs["48x48"]
						}
						usersToProcess = append(usersToProcess, JiraUserSummary{
							AccountID:   issue.Fields.Assignee.AccountID,
							Email:       issue.Fields.Assignee.EmailAddress,
							DisplayName: issue.Fields.Assignee.DisplayName,
							AvatarURL:   avatarURL,
						})
						usersSeen[issue.Fields.Assignee.AccountID] = true
					}
				}
				// Collect reporter
				if issue.Fields.Reporter != nil && issue.Fields.Reporter.AccountID != "" {
					if _, exists := userMap[issue.Fields.Reporter.AccountID]; !exists && !usersSeen[issue.Fields.Reporter.AccountID] {
						avatarURL := ""
						if issue.Fields.Reporter.AvatarURLs != nil {
							avatarURL = issue.Fields.Reporter.AvatarURLs["48x48"]
						}
						usersToProcess = append(usersToProcess, JiraUserSummary{
							AccountID:   issue.Fields.Reporter.AccountID,
							Email:       issue.Fields.Reporter.EmailAddress,
							DisplayName: issue.Fields.Reporter.DisplayName,
							AvatarURL:   avatarURL,
						})
						usersSeen[issue.Fields.Reporter.AccountID] = true
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
				newUserMappings, err := h.ensureUsers(ctx, jobID, usersToProcess)
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
				err := h.importIssue(ctx, jobID, workspaceID, &issue, statusMap, itemTypeMap, userMap, req.Mappings.CustomFields)
				if err != nil {
					slog.Error("Failed to import issue", slog.String("component", "jira"), slog.String("issue", issue.Key), slog.Any("error", err))
					progress.FailedIssues++
				} else {
					progress.ImportedIssues++
				}
			}

			h.updateJobProgress(jobID, progress)
		}

		progress.CompletedProjects = i + 1
	}

	// Mark job as completed
	progress.Phase = "completed"
	h.updateJobStatus(jobID, "completed", "completed", progress, "")
}

// ensureWorkspace creates or finds a workspace for import
func (h *JiraImportHandler) ensureWorkspace(ctx context.Context, jobID string, mapping *WorkspaceMapping) (int, error) {
	if !mapping.CreateNew && mapping.WindshiftID != nil {
		return *mapping.WindshiftID, nil
	}

	// Create new workspace
	result, err := h.db.ExecWrite(`
		INSERT INTO workspaces (key, name, description)
		VALUES (?, ?, ?)
	`, mapping.NewWorkspaceKey, mapping.NewWorkspaceName, "Imported from Jira")
	if err != nil {
		return 0, err
	}

	workspaceID, _ := result.LastInsertId()

	// Record the mapping
	h.recordMapping(jobID, "workspace", mapping.JiraKey, mapping.JiraKey, int(workspaceID), nil)

	return int(workspaceID), nil
}

// ensureStatuses creates or maps statuses for a workspace
func (h *JiraImportHandler) ensureStatuses(ctx context.Context, jobID string, workspaceID int, mappings []StatusMapping) (map[string]int, error) {
	result := make(map[string]int)

	for _, m := range mappings {
		if !m.CreateNew && m.WindshiftID != nil {
			result[m.JiraID] = *m.WindshiftID
			continue
		}

		// Get or create status category
		categoryID := 1 // Default to "To Do"
		switch m.CategoryKey {
		case "new":
			categoryID = 1
		case "indeterminate":
			categoryID = 2
		case "done":
			categoryID = 3
		}

		// Create status
		res, err := h.db.ExecWrite(`
			INSERT INTO statuses (workspace_id, name, description, category_id, color)
			VALUES (?, ?, ?, ?, ?)
		`, workspaceID, m.JiraName, "", categoryID, m.Color)
		if err != nil {
			slog.Error("Failed to create status", slog.String("component", "jira"), slog.String("status", m.JiraName), slog.Any("error", err))
			continue
		}

		statusID, _ := res.LastInsertId()
		result[m.JiraID] = int(statusID)

		// Record the mapping
		h.recordMapping(jobID, "status", m.JiraID, m.JiraName, int(statusID), nil)
	}

	return result, nil
}

// ensureItemTypes creates or maps item types for a workspace
func (h *JiraImportHandler) ensureItemTypes(ctx context.Context, jobID string, workspaceID int, mappings []IssueTypeMapping) (map[string]int, error) {
	result := make(map[string]int)

	for _, m := range mappings {
		if !m.CreateNew && m.WindshiftID != nil {
			result[m.JiraID] = *m.WindshiftID
			continue
		}

		// Create item type
		res, err := h.db.ExecWrite(`
			INSERT INTO item_types (workspace_id, name, icon, color, hierarchy_level)
			VALUES (?, ?, ?, ?, ?)
		`, workspaceID, m.JiraName, "circle", "#3B82F6", m.HierarchyLevel)
		if err != nil {
			slog.Error("Failed to create item type", slog.String("component", "jira"), slog.String("itemType", m.JiraName), slog.Any("error", err))
			continue
		}

		itemTypeID, _ := res.LastInsertId()
		result[m.JiraID] = int(itemTypeID)

		// Record the mapping
		h.recordMapping(jobID, "item_type", m.JiraID, m.JiraName, int(itemTypeID), nil)
	}

	return result, nil
}

// ensureUsers matches or creates users for import
// Returns a map from Jira account ID to Windshift user ID
func (h *JiraImportHandler) ensureUsers(ctx context.Context, jobID string, users []JiraUserSummary) (map[string]int, error) {
	result := make(map[string]int)

	for _, u := range users {
		// Skip users without account ID
		if u.AccountID == "" {
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
			err := h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, u.Email).Scan(&userID)
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

		res, err := h.db.ExecWrite(`
			INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, created_at, updated_at)
			VALUES (?, ?, ?, ?, false, ?, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, u.Email, username, firstName, lastName, u.AvatarURL)
		if err != nil {
			slog.Error("Failed to create user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Any("error", err))
			continue
		}

		newUserID, _ := res.LastInsertId()
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

	if fieldType == "user" {
		if userObj, ok := value.(map[string]interface{}); ok {
			addUserFromObject(userObj, existingMap, usersToProcess, seen)
		}
	} else if fieldType == "users" {
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
func (h *JiraImportHandler) importIssue(ctx context.Context, jobID string, workspaceID int, issue *jira.JiraIssue, statusMap map[string]int, itemTypeMap map[string]int, userMap map[string]int, customFieldMappings []CustomFieldMapping) error {
	// Get mapped status and item type
	statusID := 0
	if issue.Fields.Status != nil {
		if sid, ok := statusMap[issue.Fields.Status.ID]; ok {
			statusID = sid
		}
	}

	itemTypeID := 0
	if issue.Fields.IssueType != nil {
		if tid, ok := itemTypeMap[issue.Fields.IssueType.ID]; ok {
			itemTypeID = tid
		}
	}

	// Map assignee and reporter
	var assigneeID *int
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.AccountID != "" {
		if uid, ok := userMap[issue.Fields.Assignee.AccountID]; ok {
			assigneeID = &uid
		}
	}

	var reporterID *int
	if issue.Fields.Reporter != nil && issue.Fields.Reporter.AccountID != "" {
		if uid, ok := userMap[issue.Fields.Reporter.AccountID]; ok {
			reporterID = &uid
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
	var customFieldJSON *string
	if len(customFieldValues) > 0 {
		jsonBytes, err := json.Marshal(customFieldValues)
		if err == nil {
			jsonStr := string(jsonBytes)
			customFieldJSON = &jsonStr
		}
	}

	// Create the work item with assignee, reporter, and custom fields
	result, err := h.db.ExecWrite(`
		INSERT INTO items (workspace_id, title, description, status_id, item_type_id, assignee_id, reporter_id, custom_field_values)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, workspaceID, issue.Fields.Summary, description, statusID, itemTypeID, assigneeID, reporterID, customFieldJSON)
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	itemID, _ := result.LastInsertId()

	// Record the mapping
	h.recordMapping(jobID, "item", issue.ID, issue.Key, int(itemID), map[string]interface{}{
		"summary": issue.Fields.Summary,
	})

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

	if status == "running" {
		query = `UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{status, phase, progressJSON, jobID}
	} else if status == "completed" || status == "failed" {
		query = `UPDATE jira_import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{status, phase, progressJSON, errorMessage, jobID}
	} else {
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

// DeleteImportedData handles DELETE /api/jira-import/jobs/{jobId}/data
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
				WHEN 'custom_field' THEN 5
				WHEN 'status' THEN 6
				WHEN 'item_type' THEN 7
				WHEN 'workspace' THEN 8
				ELSE 9
			END
	`, jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get mappings: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type mapping struct {
		entityType  string
		windshiftID int
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.entityType, &m.windshiftID); err != nil {
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
		case "custom_field":
			tableName = "custom_fields"
		case "attachment":
			tableName = "attachments"
		case "comment":
			tableName = "comments"
		case "link":
			tableName = "item_links"
		default:
			slog.Warn("Unknown entity type", slog.String("component", "jira"), slog.String("entityType", m.entityType))
			continue
		}

		_, err := h.db.ExecWrite(fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName), m.windshiftID)
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": deleted,
	})
}

// GetPreviousImports handles GET /api/jira-import/previous-imports
// Returns previous imports for the same projects to enable re-import
func (h *JiraImportHandler) GetPreviousImports(w http.ResponseWriter, r *http.Request) {
	projectKeys := r.URL.Query()["project_key"]
	if len(projectKeys) == 0 {
		http.Error(w, "At least one project_key is required", http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("Failed to query previous imports: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(imports)
}
