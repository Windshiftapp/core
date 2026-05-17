package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// executeImport runs the actual import process in the background
func (h *JiraImportHandler) executeImport(jobID string, req StartImportRequest) {
	ctx := context.Background()

	// Update job status to running
	h.updateJobStatus(jobID, "running", "initializing", nil, "")

	// Look up the user who initiated this job so imported workspaces can grant
	// them admin access. Without this the importer would create workspaces with
	// no user_workspace_roles rows, making them invisible to non-system-admins.
	var createdBy sql.NullInt64
	if err := h.db.QueryRow(`SELECT created_by FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&createdBy); err != nil {
		slog.Warn("Failed to look up job creator", slog.String("component", "jira"), slog.String("job_id", jobID), slog.Any("error", err))
	}

	// Get the Jira client
	client, err := h.getClientForConnection(ctx, req.ConnectionID)
	if err != nil {
		h.updateJobStatus(jobID, "failed", "", nil, fmt.Sprintf("Failed to connect to Jira: %v", err))
		return
	}

	// When JIRA_CAPTURE_PAYLOADS is configured, save the request and wrap the client
	captureDir := h.capturePayloadsDir
	if captureDir != "" {
		if err := os.MkdirAll(captureDir, 0o750); err != nil { //nolint:gosec // path from server operator env var JIRA_CAPTURE_PAYLOADS
			slog.Error("Failed to create capture directory", slog.String("component", "jira"), slog.Any("error", err))
		} else {
			// Save import_request.json
			reqData, _ := json.MarshalIndent(req, "", "  ")
			if err := os.WriteFile(captureDir+"/import_request.json", reqData, 0o600); err != nil { //nolint:gosec // G703: captureDir from server operator env var
				slog.Error("Failed to save import request", slog.String("component", "jira"), slog.Any("error", err))
			}

			// Wrap client in recording client
			rc := newRecordingClient(client, captureDir)
			client = rc

			// Save responses + post-import windshift snapshot when import
			// completes (deferred so partial/failed runs still get a snapshot —
			// that's the diff signal we want).
			defer func() {
				if err := rc.saveToFile(captureDir); err != nil {
					slog.Error("Failed to save captured payloads", slog.String("component", "jira"), slog.Any("error", err))
				}
				if err := writeWindshiftExport(h.db, jobID, captureDir); err != nil {
					slog.Error("Failed to save windshift export", slog.String("component", "jira"), slog.Any("error", err))
				}
			}()
		}
	}

	createdByID := 0
	if createdBy.Valid {
		createdByID = int(createdBy.Int64)
	}
	h.executeImportWithClient(jobID, req, client, createdByID)
}

// executeImportWithClient runs the import using the provided Jira client.
// Extracted from executeImport to allow testing with a mock client.
// createdByUserID is the ID of the user who initiated the import (0 if unknown),
// used to grant workspace admin access on imported workspaces.
func (h *JiraImportHandler) executeImportWithClient(jobID string, req StartImportRequest, client jira.Client, createdByUserID int) {
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
		workspaceID, err := h.ensureWorkspace(ctx, jobID, wsMapping, createdByUserID)
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
		// Track user map across all batches for this project. usernameMap holds
		// the same accountID keys mapped to Windshift usernames so the ADF
		// converter can render @mentions as `@<username>` rather than display
		// text — letting MentionService pick them up via its standard regex.
		userMap := make(map[string]int)
		usernameMap := make(map[string]string)

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
				newUserMappings, newUsernameMappings, err := h.ensureUsers(ctx, jobID, usersToProcess, client)
				if err != nil {
					slog.Error("Failed to ensure users", slog.String("component", "jira"), slog.Any("error", err))
				}
				// Merge new mappings into userMap and usernameMap
				for k, v := range newUserMappings {
					userMap[k] = v
				}
				for k, v := range newUsernameMappings {
					usernameMap[k] = v
				}
			}

			// Import each issue
			for _, issue := range fetchResult.Issues {
				err := h.importIssue(ctx, jobID, workspaceID, &issue, statusMap, itemTypeMap, userMap, usernameMap, versionMap, req.Mappings.CustomFields, client, progress)
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

// ensureWorkspace creates or finds a workspace for import.
// createdByUserID grants the import initiator workspace admin access; pass 0 if unknown.
func (h *JiraImportHandler) ensureWorkspace(_ context.Context, jobID string, mapping *WorkspaceMapping, createdByUserID int) (int, error) {
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
		CreatorID:   createdByUserID,
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
		var jiraTargetDate *string
		if m.ReleaseDate != "" {
			jiraTargetDate = &m.ReleaseDate
		}
		milestone, err := planningSvc.CreateMilestone(services.CreateMilestoneParams{
			Name:        m.JiraName,
			TargetDate:  jiraTargetDate,
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
