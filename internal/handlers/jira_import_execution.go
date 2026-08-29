package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/jiraimport"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

const (
	jiraIssueKeyFieldSourceID = "system:jira-issue-key"
	jiraIssueKeyFieldName     = "Jira Key"
)

// executeImport runs the actual import process in the background
func (h *JiraImportHandler) executeImport(jobID string, req StartImportRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	// Update job status to running
	h.updateJobStatus(jobID, "running", "initializing", nil, "")

	// Look up the user who initiated this job so imported workspaces can grant
	// them admin access. Without this the importer would create workspaces with
	// no user_workspace_roles rows, making them invisible to non-system-admins.
	createdByID, err := h.imports.Creator(jobID)
	if err != nil {
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
			capturedReq := req
			capturedReq.Xray.ClientSecret = ""
			reqData, _ := json.MarshalIndent(capturedReq, "", "  ")
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
				if err := services.WriteWindshiftExport(h.db, jobID, captureDir); err != nil {
					slog.Error("Failed to save windshift export", slog.String("component", "jira"), slog.Any("error", err))
				}
			}()
		}
	}

	h.executeImportWithClientContext(ctx, jobID, req, client, createdByID)
}

// executeImportWithClient runs the import using the provided Jira client.
// Extracted from executeImport to allow testing with a mock client.
// createdByUserID is the ID of the user who initiated the import (0 if unknown),
// used to grant workspace admin access on imported workspaces.
//
//nolint:unused // Kept for importer tests that inject a mock Jira client.
func (h *JiraImportHandler) executeImportWithClient(jobID string, req StartImportRequest, client jira.Client, createdByUserID int) {
	h.executeImportWithClientContext(context.Background(), jobID, req, client, createdByUserID)
}

func jiraIssueImportJQL(projectKey string, openIssuesOnly, rankOrdered bool) string {
	statusClause := ""
	if openIssuesOnly {
		statusClause = " AND statusCategory != Done"
	}
	orderBy := "created ASC, key ASC"
	if rankOrdered {
		// Jira Software's Rank field is a LexoRank value. Ask Jira to perform
		// that comparison instead of attempting to interpret the opaque value
		// locally. The remaining fields make unranked/tied results deterministic.
		orderBy = "Rank ASC, created ASC, key ASC"
	}
	return fmt.Sprintf("project = %s%s ORDER BY %s", projectKey, statusClause, orderBy)
}

func getJiraIssueKeysInImportOrder(
	ctx context.Context,
	client jira.Client,
	projectKey string,
	openIssuesOnly bool,
) ([]string, error) {
	rankedJQL := jiraIssueImportJQL(projectKey, openIssuesOnly, true)
	keys, err := client.GetAllIssueKeys(ctx, rankedJQL)
	if err == nil {
		return keys, nil
	}

	// Jira Core-only/Data Center installations may not expose the Jira
	// Software Rank field. Preserve import availability with the previous
	// chronological order while making the degraded ordering visible in logs.
	fallbackJQL := jiraIssueImportJQL(projectKey, openIssuesOnly, false)
	fallbackKeys, fallbackErr := client.GetAllIssueKeys(ctx, fallbackJQL)
	if fallbackErr != nil {
		return nil, fmt.Errorf(
			"list Jira issues by rank: %v; chronological fallback also failed: %w",
			err,
			fallbackErr,
		)
	}
	slog.Warn("Jira Rank ordering unavailable; using chronological issue order",
		slog.String("component", "jira"),
		slog.String("project", projectKey),
		slog.Any("error", err))
	return fallbackKeys, nil
}

func sortJiraIssuesByRequestedKeyOrder(issues []jira.JiraIssue, orderedKeys []string) {
	if len(issues) < 2 || len(orderedKeys) == 0 {
		return
	}
	positions := make(map[string]int, len(orderedKeys))
	for position, key := range orderedKeys {
		positions[key] = position
	}
	sort.SliceStable(issues, func(i, j int) bool {
		left, leftKnown := positions[issues[i].Key]
		right, rightKnown := positions[issues[j].Key]
		switch {
		case leftKnown && rightKnown:
			return left < right
		case leftKnown:
			return true
		case rightKnown:
			return false
		default:
			// Preserve Jira's response order for unexpected issues.
			return false
		}
	})
}

func recordJiraBulkFetchErrors(
	projectKey string,
	bulkErrors []jira.BulkFetchError,
	xrayPlan *xrayImportPlan,
	progress *ImportProgress,
) {
	for _, fetchError := range bulkErrors {
		slog.Error("Failed to fetch Jira issue",
			slog.String("component", "jira"),
			slog.String("issue", fetchError.IssueIDOrKey),
			slog.String("error", fetchError.ErrorMessage))
		if xrayPlan.isTest(projectKey, fetchError.IssueIDOrKey) {
			progress.FailedTests++
		} else {
			progress.FailedIssues++
		}
	}
}

func jiraImportTerminalOutcome(progress *ImportProgress) (status, phase, errorMessage string) {
	failures := progress.FailedProjects + progress.FailedIssues + progress.FailedTests + progress.FailedLinks
	if failures == 0 {
		return "completed", "completed", ""
	}
	if progress.ImportedIssues+progress.ImportedTests > 0 {
		return "completed_with_errors", "completed_with_errors", ""
	}
	return "failed", "failed", fmt.Sprintf(
		"No Jira issues or Xray tests were imported (%d project, %d issue, %d test, and %d issue-link failures).",
		progress.FailedProjects,
		progress.FailedIssues,
		progress.FailedTests,
		progress.FailedLinks,
	)
}

func (h *JiraImportHandler) executeImportWithClientContext(ctx context.Context, jobID string, req StartImportRequest, client jira.Client, createdByUserID int) {
	h.clearMappingFailure(jobID)
	defer h.clearMappingFailure(jobID)

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

	xrayPlan, err := prepareXrayImport(ctx, client, req.Xray, req.ProjectKeys, req.OpenIssuesOnly)
	if err != nil {
		h.updateJobStatus(jobID, "failed", "", progress, fmt.Sprintf("Failed to prepare Xray import: %v", err))
		return
	}
	if xrayPlan != nil {
		progress.TotalTests = xrayPlan.total
		progress.TotalIssues -= xrayPlan.total
		if progress.TotalIssues < 0 {
			progress.TotalIssues = 0
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

	h.importJiraAssets(ctx, jobID, client, createdByUserID)
	if h.failOnMappingFailure(jobID, progress) {
		return
	}

	fieldConfigurations := loadJiraCustomFieldConfigurations(ctx, client, req.Mappings.CustomFields)
	issueKeysByProject, assetFieldSetIDs, applicableFieldsByProject, choiceLabelsByField := h.preflightJiraCustomFields(
		ctx, jobID, client, req.ProjectKeys, req.OpenIssuesOnly, req.Mappings.CustomFields,
	)
	mergeJiraConfiguredChoiceLabels(choiceLabelsByField, fieldConfigurations)
	customFieldIDMap, choiceOptionIDs, err := h.ensureCustomFields(
		ctx,
		jobID,
		req.Mappings.CustomFields,
		assetFieldSetIDs,
		choiceLabelsByField,
		fieldConfigurations,
	)
	if err != nil {
		slog.Error("Failed to ensure custom fields", slog.String("component", "jira"), slog.Any("error", err))
		customFieldIDMap = make(map[string]int)
	}
	jiraKeyFieldID, err := h.ensureJiraIssueKeyCustomField(jobID)
	if err != nil {
		slog.Error("Failed to ensure searchable Jira Key field", slog.String("component", "jira"), slog.Any("error", err))
	} else {
		customFieldIDMap[jiraIssueKeyFieldSourceID] = jiraKeyFieldID
	}
	affectsVersionField, err := h.ensureAffectsVersionCustomField(ctx, jobID, req.Mappings.Versions)
	if err != nil {
		slog.Error("Failed to ensure Affects Version custom field", slog.String("component", "jira"), slog.Any("error", err))
	}
	if h.failOnMappingFailure(jobID, progress) {
		return
	}

	var issueLinkImportErr error

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
			progress.FailedProjects++
			continue
		}

		// Create or use existing workspace
		workspaceID, err := h.ensureWorkspace(ctx, jobID, wsMapping, createdByUserID)
		if err != nil {
			slog.Error("Failed to ensure workspace", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			progress.FailedProjects++
			continue
		}
		if h.failOnMappingFailure(jobID, progress) {
			return
		}

		jsmImport, err := h.prepareJiraServiceManagementImport(
			ctx, jobID, projectKey, workspaceID, itemTypeMap, client, createdByUserID,
			req.Mappings.ServiceManagement.ImportOrganizations,
		)
		if err != nil {
			slog.Error("Failed to prepare Jira Service Management portal",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Any("error", err))
			progress.FailedProjects++
			continue
		}
		if h.failOnMappingFailure(jobID, progress) {
			return
		}

		// Create workflows and configuration set for this project
		if err = h.ensureWorkflowsAndConfigSet(ctx, jobID, projectKey, workspaceID, statusMap, itemTypeMap, client); err != nil {
			slog.Error("Failed to create workflows/config set", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			// Non-fatal: continue importing
		}
		if err = h.ensureJiraProjectScreens(
			ctx, jobID, projectKey, workspaceID, itemTypeMap, customFieldIDMap,
			req.Mappings.CustomFields, client,
		); err != nil {
			slog.Warn("Jira screen configuration was not imported",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Any("error", err))
		}
		if err = h.bindJiraImportFieldsToWorkspace(
			workspaceID, projectKey, customFieldIDMap, req.Mappings.CustomFields, applicableFieldsByProject[projectKey],
		); err != nil {
			slog.Error("Failed to bind Jira import fields to workspace screens",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Any("error", err))
		}
		if h.failOnMappingFailure(jobID, progress) {
			return
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

		iterationMap, err := h.ensureJiraIterations(ctx, jobID, workspaceID, projectKey, client)
		if err != nil {
			slog.Error("Failed to ensure Jira iterations", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			iterationMap = make(map[string]int)
		}

		h.importJiraBoardsAndFilters(ctx, jobID, projectKey, workspaceID, statusMap, client, createdByUserID)

		timeProjectID, err := h.ensureJiraTimeProject(jobID, workspaceID, projectKey, wsMapping.NewWorkspaceName)
		if err != nil {
			slog.Error("Failed to ensure Jira time project", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			timeProjectID = nil
		}
		if h.failOnMappingFailure(jobID, progress) {
			return
		}

		issueKeys, prefetched := issueKeysByProject[projectKey]
		if !prefetched {
			issueKeys, err = getJiraIssueKeysInImportOrder(ctx, client, projectKey, req.OpenIssuesOnly)
			if err != nil {
				slog.Error("Failed to get issue keys", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
				progress.FailedProjects++
				continue
			}
		}

		// Fetch and import issues in batches
		// Track user map across all batches for this project. usernameMap holds
		// the same accountID keys mapped to Windshift usernames so the ADF
		// converter can render @mentions as `@<username>` rather than display
		// text — letting MentionService pick them up via its standard regex.
		userMap := make(map[string]int)
		usernameMap := make(map[string]string)
		portalCustomerMap := make(map[string]int)
		if jsmImport != nil && len(jsmImport.OrganizationCustomers) > 0 {
			organizationMappings, organizationErr := h.ensurePortalCustomers(
				jobID, jsmImport.ChannelID, jsmImport.OrganizationCustomers, jsmImport.CustomerOrganizations,
			)
			if organizationErr != nil {
				slog.Error("Failed to ensure Jira organization customers", slog.String("component", "jira"), slog.Any("error", organizationErr))
			}
			for accountID, customerID := range organizationMappings {
				portalCustomerMap[accountID] = customerID
			}
		}

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
				for _, issueKey := range batch {
					if xrayPlan.isTest(projectKey, issueKey) {
						progress.FailedTests++
					} else {
						progress.FailedIssues++
					}
				}
				continue
			}
			recordJiraBulkFetchErrors(projectKey, fetchResult.Errors, xrayPlan, progress)
			// Bulk fetch is a set-oriented API and does not guarantee request
			// ordering. Restore the Rank-ordered key sequence so CreateItem's
			// append-only fractional index generation preserves Jira order.
			sortJiraIssuesByRequestedKeyOrder(fetchResult.Issues, batch)

			xrayDefinitions, xrayDefinitionErrors := xrayPlan.definitions(ctx, projectKey, fetchResult.Issues)

			// Complete paginated issue subresources before collecting users/importing
			// rows. Jira embeds only the first comment/worklog page in issue payloads;
			// fetching the rest here lets author mapping include every referenced user.
			for idx := range fetchResult.Issues {
				if xrayPlan.isTest(projectKey, fetchResult.Issues[idx].Key) {
					continue
				}
				if err := h.completePagedIssueContainers(ctx, &fetchResult.Issues[idx], client); err != nil {
					slog.Warn("Failed to complete paged Jira issue containers",
						slog.String("component", "jira"),
						slog.String("issue", fetchResult.Issues[idx].Key),
						slog.Any("error", err))
				}
				h.completeIssueWatchers(ctx, &fetchResult.Issues[idx], client)
				if jsmImport != nil {
					if err := h.annotateJiraServiceDeskCommentVisibility(ctx, &fetchResult.Issues[idx], client); err != nil {
						slog.Warn("Failed to load Jira Service Management comment visibility",
							slog.String("component", "jira"),
							slog.String("issue", fetchResult.Issues[idx].Key),
							slog.Any("error", err))
					}
				}
			}

			// Collect users from this batch
			var usersToProcess []JiraUserSummary
			usersSeen := make(map[string]bool)
			knownIdentityMap := make(map[string]int, len(userMap)+len(portalCustomerMap))
			for accountID, userID := range userMap {
				knownIdentityMap[accountID] = userID
			}
			for accountID := range portalCustomerMap {
				knownIdentityMap[accountID] = 0
			}
			for _, issue := range fetchResult.Issues {
				if xrayPlan.isTest(projectKey, issue.Key) {
					continue
				}
				// Collect every first-class user reference that can be written during
				// issue import. If we only pre-collect assignee/reporter, creator,
				// comment author, update author, and attachment uploader references
				// degrade to nil or the shared fallback user even though Jira supplied
				// enough identity data in the issue payload.
				addJiraUserSummaryFromUser(issue.Fields.Assignee, knownIdentityMap, &usersToProcess, usersSeen)
				addJiraUserSummaryFromUser(issue.Fields.Reporter, knownIdentityMap, &usersToProcess, usersSeen)
				addJiraUserSummaryFromUser(issue.Fields.Creator, knownIdentityMap, &usersToProcess, usersSeen)
				for watcherIdx := range issue.Fields.Watchers {
					addJiraUserSummaryFromUser(&issue.Fields.Watchers[watcherIdx], knownIdentityMap, &usersToProcess, usersSeen)
				}
				if issue.Fields.Votes != nil {
					for voterIdx := range issue.Fields.Votes.Voters {
						addJiraUserSummaryFromUser(&issue.Fields.Votes.Voters[voterIdx], knownIdentityMap, &usersToProcess, usersSeen)
					}
				}
				collectUsersFromADF(issue.Fields.Description, knownIdentityMap, &usersToProcess, usersSeen)
				if issue.Fields.Comment != nil {
					for _, comment := range issue.Fields.Comment.Comments {
						addJiraUserSummaryFromUser(comment.Author, knownIdentityMap, &usersToProcess, usersSeen)
						addJiraUserSummaryFromUser(comment.UpdateAuthor, knownIdentityMap, &usersToProcess, usersSeen)
						collectUsersFromADF(comment.Body, knownIdentityMap, &usersToProcess, usersSeen)
					}
				}
				for _, attachment := range issue.Fields.Attachment {
					addJiraUserSummaryFromUser(attachment.Author, knownIdentityMap, &usersToProcess, usersSeen)
				}
				if issue.Fields.Worklog != nil {
					for _, worklog := range issue.Fields.Worklog.Worklogs {
						addJiraUserSummaryFromUser(worklog.Author, knownIdentityMap, &usersToProcess, usersSeen)
						collectUsersFromADF(worklog.Comment, knownIdentityMap, &usersToProcess, usersSeen)
					}
				}
				for _, value := range issue.Fields.CustomFields {
					collectUsersFromADF(value, knownIdentityMap, &usersToProcess, usersSeen)
				}

				// Collect users from custom user fields (single and multi-user pickers)
				for _, mapping := range req.Mappings.CustomFields {
					if mapping.WindshiftType != "user" && mapping.WindshiftType != "multi_user" {
						continue
					}
					if mapping.Action == "skip" {
						continue
					}

					value, exists := issue.Fields.CustomFields[mapping.JiraID]
					if !exists || value == nil {
						continue
					}

					collectUsersFromCustomField(value, mapping.WindshiftType, knownIdentityMap, &usersToProcess, usersSeen)
				}
			}

			// Ensure users are created/matched
			if len(usersToProcess) > 0 {
				internalUsers := usersToProcess
				var portalCustomers []JiraUserSummary
				if jsmImport != nil {
					internalUsers, portalCustomers = splitJiraImportUsers(usersToProcess)
				}
				newUserMappings, newUsernameMappings, err := h.ensureUsers(ctx, jobID, internalUsers, client)
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
				if len(portalCustomers) > 0 {
					newPortalMappings, portalErr := h.ensurePortalCustomers(
						jobID, jsmImport.ChannelID, portalCustomers, jsmImport.CustomerOrganizations,
					)
					if portalErr != nil {
						slog.Error("Failed to ensure Jira portal customers", slog.String("component", "jira"), slog.Any("error", portalErr))
					}
					for k, v := range newPortalMappings {
						portalCustomerMap[k] = v
					}
				}
			}

			// Import each issue
			for _, issue := range fetchResult.Issues {
				if xrayPlan.isTest(projectKey, issue.Key) {
					if definitionErr, failed := xrayDefinitionErrors[issue.Key]; failed {
						slog.Error("Failed to load Xray Test definition",
							slog.String("component", "jira"),
							slog.String("issue", issue.Key),
							slog.Any("error", definitionErr))
						progress.FailedTests++
						continue
					}
					definition, exists := xrayDefinitions[issue.ID]
					if !exists {
						slog.Error("Missing Xray Test definition",
							slog.String("component", "jira"),
							slog.String("issue", issue.Key))
						progress.FailedTests++
						continue
					}
					if _, err := h.importXrayTestCase(jobID, workspaceID, &issue, definition); err != nil {
						slog.Error("Failed to import Xray Test",
							slog.String("component", "jira"),
							slog.String("issue", issue.Key),
							slog.Any("error", err))
						progress.FailedTests++
					} else {
						progress.ImportedTests++
					}
					continue
				}
				err := h.importIssue(ctx, jobID, workspaceID, &issue, statusMap, itemTypeMap, userMap, usernameMap, portalCustomerMap, versionMap, iterationMap, customFieldIDMap, choiceOptionIDs, timeProjectID, affectsVersionField, req.Mappings.CustomFields, jsmImport, client, progress, req.ForceReimport)
				if h.failOnMappingFailure(jobID, progress) {
					return
				}
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
		if err := h.importIssueLinks(jobID); err != nil {
			if h.failOnMappingFailure(jobID, progress) {
				return
			}
			slog.Error("Failed to import Jira issue links", slog.String("component", "jira"), slog.Any("error", err))
			issueLinkImportErr = err
		} else {
			issueLinkImportErr = nil
		}
		if h.failOnMappingFailure(jobID, progress) {
			return
		}

		progress.CompletedProjects = i + 1
	}

	if h.failOnMappingFailure(jobID, progress) {
		return
	}
	if issueLinkImportErr != nil {
		progress.FailedLinks = 1
	}
	status, phase, errorMessage := jiraImportTerminalOutcome(progress)
	progress.Phase = phase
	h.persistJiraImportResult(jobID, progress)
	h.updateJobStatus(jobID, status, phase, progress, errorMessage)
}

type jiraImportFidelityFinding struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Disposition string `json:"disposition"`
	Summary     string `json:"summary"`
	Count       int    `json:"count,omitempty"`
}

func (h *JiraImportHandler) persistJiraImportResult(jobID string, progress *ImportProgress) {
	type configurationEntity struct {
		SourceID   string         `json:"source_id"`
		SourceName string         `json:"source_name,omitempty"`
		TargetID   int            `json:"target_id"`
		Metadata   map[string]any `json:"metadata"`
	}
	planFingerprint, configurationDrift, previousImports := h.jiraImportPlanResult(jobID)
	result := struct {
		Summary            *ImportProgress             `json:"summary"`
		PlanFingerprint    string                      `json:"plan_fingerprint,omitempty"`
		ConfigurationDrift bool                        `json:"configuration_drift"`
		PreviousImports    []jiraImportConflict        `json:"previous_imports,omitempty"`
		CustomFields       []configurationEntity       `json:"custom_fields"`
		Workflows          []configurationEntity       `json:"workflows"`
		Screens            []configurationEntity       `json:"screens"`
		Findings           []jiraImportFidelityFinding `json:"fidelity_findings"`
	}{
		Summary:            progress,
		PlanFingerprint:    planFingerprint,
		ConfigurationDrift: configurationDrift,
		PreviousImports:    previousImports,
		CustomFields:       []configurationEntity{},
		Workflows:          []configurationEntity{},
		Screens:            []configurationEntity{},
		Findings:           h.jiraImportFidelityFindings(jobID),
	}
	records, err := h.imports.ConfigurationRecords(jobID)
	if err != nil {
		slog.Warn("Failed to build Jira import configuration result",
			slog.String("component", "jira"),
			slog.String("job_id", jobID),
			slog.Any("error", err))
		return
	}
	for _, record := range records {
		entity := configurationEntity{
			SourceID:   record.JiraID,
			SourceName: record.JiraKey,
			TargetID:   record.WindshiftID,
			Metadata:   record.Metadata,
		}
		switch record.EntityType {
		case "custom_field":
			result.CustomFields = append(result.CustomFields, entity)
		case "workflow":
			result.Workflows = append(result.Workflows, entity)
		case "screen":
			result.Screens = append(result.Screens, entity)
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		slog.Warn("Failed to encode Jira import configuration result",
			slog.String("component", "jira"),
			slog.String("job_id", jobID),
			slog.Any("error", err))
		return
	}
	if err := h.imports.UpdateJobResult(jobID, data); err != nil {
		slog.Warn("Failed to persist Jira import configuration result",
			slog.String("component", "jira"),
			slog.String("job_id", jobID),
			slog.Any("error", err))
	}
}

func (h *JiraImportHandler) jiraImportPlanResult(jobID string) (
	fingerprint string,
	configurationDrift bool,
	previousImports []jiraImportConflict,
) {
	configJSON, err := h.imports.JobConfigJSON(jobID)
	if err != nil {
		return "", false, nil
	}
	var config struct {
		PlanFingerprint    string               `json:"plan_fingerprint"`
		ConfigurationDrift bool                 `json:"configuration_drift"`
		PreviousImports    []jiraImportConflict `json:"previous_imports"`
	}
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		return "", false, nil
	}
	return config.PlanFingerprint, config.ConfigurationDrift, config.PreviousImports
}

func (h *JiraImportHandler) jiraImportFidelityFindings(jobID string) []jiraImportFidelityFinding {
	findings := []jiraImportFidelityFinding{
		{
			Code:        "jira_project_roles_not_imported",
			Severity:    "warning",
			Disposition: "unsupported",
			Summary:     "Jira project roles are not copied because Windshift workspace roles and grants have different permission semantics.",
		},
		{
			Code:        "jira_permission_schemes_not_imported",
			Severity:    "warning",
			Disposition: "unsupported",
			Summary:     "Jira permission schemes are not converted into Windshift permissions; existing Windshift access controls remain authoritative.",
		},
	}
	if _, drift, previousImports := h.jiraImportPlanResult(jobID); drift {
		findings = append(findings, jiraImportFidelityFinding{
			Code:        "jira_import_plan_changed",
			Severity:    "warning",
			Disposition: "converted",
			Summary:     "The selected Jira scope or mapping plan changed since a previous import; the current plan fingerprint and prior imports are retained for review.",
			Count:       len(previousImports),
		})
	}

	itemValues, err := h.imports.ItemCustomFieldValues(jobID)
	if err == nil {
		voteCount := 0
		securityCount := 0
		watcherUnavailableCount := 0
		for _, raw := range itemValues {
			var values map[string]any
			if json.Unmarshal([]byte(raw), &values) != nil {
				continue
			}
			if _, ok := values["_jira_votes"]; ok {
				voteCount++
			}
			if _, ok := values["_jira_security_level"]; ok {
				securityCount++
			}
			if _, ok := values["_jira_watcher_fetch_error"]; ok {
				watcherUnavailableCount++
			}
		}
		if voteCount > 0 {
			findings = append(findings, jiraImportFidelityFinding{
				Code:        "jira_votes_preserved",
				Severity:    "info",
				Disposition: "preserved_metadata",
				Summary:     "Jira votes and available voter identities were preserved as item metadata because Windshift has no voting model.",
				Count:       voteCount,
			})
		}
		if securityCount > 0 {
			findings = append(findings, jiraImportFidelityFinding{
				Code:        "jira_issue_security_preserved",
				Severity:    "warning",
				Disposition: "preserved_metadata",
				Summary:     "Jira issue-security levels were preserved as item metadata and were not translated into broader Windshift workspace access.",
				Count:       securityCount,
			})
		}
		if watcherUnavailableCount > 0 {
			findings = append(findings, jiraImportFidelityFinding{
				Code:        "jira_watcher_identities_unavailable",
				Severity:    "warning",
				Disposition: "unavailable",
				Summary:     "Jira exposed watcher counts but did not allow the importer to read all watcher identities.",
				Count:       watcherUnavailableCount,
			})
		}
	}

	mappingRecords, err := h.imports.FidelityRecords(jobID)
	if err != nil {
		return findings
	}
	rawFieldCount := 0
	dateTimeCount := 0
	slaCount := 0
	for _, record := range mappingRecords {
		metadata := record.Metadata
		if record.EntityType == "fidelity_finding" {
			findings = append(findings, jiraImportFidelityFinding{
				Code:        metadataString(metadata, "code"),
				Severity:    metadataString(metadata, "severity"),
				Disposition: metadataString(metadata, "disposition"),
				Summary:     record.JiraKey,
				Count:       metadataInt(metadata, "source_count") - metadataInt(metadata, "resolved_count"),
			})
			continue
		}
		if preserved, _ := metadata["preserve_raw"].(bool); preserved {
			rawFieldCount++
		}
		jiraType := strings.ToLower(metadataString(metadata, "jira_field_type"))
		if strings.Contains(jiraType, "datetime") {
			dateTimeCount++
		}
		if strings.Contains(jiraType, "sla") || strings.Contains(strings.ToLower(record.JiraKey), "sla") {
			slaCount++
		}
	}
	if rawFieldCount > 0 {
		findings = append(findings, jiraImportFidelityFinding{
			Code:        "jira_custom_fields_preserved_raw",
			Severity:    "warning",
			Disposition: "preserved_raw",
			Summary:     "App-owned or complex Jira custom fields were preserved as raw JSON rather than converted to a guessed native type.",
			Count:       rawFieldCount,
		})
	}
	if dateTimeCount > 0 {
		findings = append(findings, jiraImportFidelityFinding{
			Code:        "jira_datetime_editing_lossy",
			Severity:    "warning",
			Disposition: "lossy",
			Summary:     "Jira datetime values retain their timestamp text, but Windshift currently edits them through a date-only field model.",
			Count:       dateTimeCount,
		})
	}
	if slaCount > 0 {
		findings = append(findings, jiraImportFidelityFinding{
			Code:        "jira_service_sla_state_preserved",
			Severity:    "warning",
			Disposition: "preserved_raw",
			Summary:     "Jira Service Management SLA values were preserved, but calendars, goals, pause conditions, and running timers were not recreated.",
			Count:       slaCount,
		})
	}
	return findings
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func metadataInt(metadata map[string]any, key string) int {
	value, _ := numericMetadataInt(metadata[key])
	return value
}

func numericMetadataInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

type jiraCustomFieldConfigurationCapture struct {
	configuration     *jira.JiraCustomFieldConfiguration
	unavailableReason string
}

func loadJiraCustomFieldConfigurations(
	ctx context.Context,
	client jira.Client,
	mappings []CustomFieldMapping,
) map[string]jiraCustomFieldConfigurationCapture {
	captures := make(map[string]jiraCustomFieldConfigurationCapture)
	capable, supported := client.(jira.CustomFieldConfigurationClient)
	for _, mapping := range mappings {
		if mapping.Action == "skip" || mapping.JiraID == "" {
			continue
		}
		includeOptions := mapping.WindshiftType == string(jira.FieldTypeSelect) ||
			mapping.WindshiftType == string(jira.FieldTypeMultiselect)
		if !supported {
			captures[mapping.JiraID] = jiraCustomFieldConfigurationCapture{
				unavailableReason: jira.ErrCustomFieldConfigurationNotAvailable.Error(),
			}
			continue
		}
		configuration, err := capable.GetCustomFieldConfiguration(ctx, mapping.JiraID, includeOptions)
		if err != nil {
			slog.Warn("Jira custom-field configuration was unavailable",
				slog.String("component", "jira"),
				slog.String("jiraFieldID", mapping.JiraID),
				slog.Any("error", err))
			captures[mapping.JiraID] = jiraCustomFieldConfigurationCapture{
				unavailableReason: err.Error(),
			}
			continue
		}
		captures[mapping.JiraID] = jiraCustomFieldConfigurationCapture{configuration: configuration}
	}
	return captures
}

func mergeJiraConfiguredChoiceLabels(
	labelsByField map[string][]string,
	captures map[string]jiraCustomFieldConfigurationCapture,
) {
	for fieldID, capture := range captures {
		configuration := capture.configuration
		if configuration == nil {
			continue
		}
		labelsByKey := make(map[string]string)
		for _, label := range labelsByField[fieldID] {
			label = strings.TrimSpace(label)
			if label != "" {
				labelsByKey[strings.ToLower(label)] = label
			}
		}
		for _, context := range configuration.Contexts {
			optionValues := make(map[string]string, len(context.Options))
			for _, option := range context.Options {
				optionValues[option.ID] = strings.TrimSpace(option.Value)
			}
			for _, option := range context.Options {
				label := strings.TrimSpace(option.Value)
				if parent := optionValues[option.ParentOptionID]; parent != "" {
					label = parent + " / " + label
				}
				if label == "" {
					continue
				}
				key := strings.ToLower(label)
				if _, exists := labelsByKey[key]; !exists {
					labelsByKey[key] = label
				}
			}
		}
		labels := make([]string, 0, len(labelsByKey))
		for _, label := range labelsByKey {
			labels = append(labels, label)
		}
		sort.Slice(labels, func(i, j int) bool {
			return strings.ToLower(labels[i]) < strings.ToLower(labels[j])
		})
		labelsByField[fieldID] = labels
	}
}

// preflightJiraCustomFields resolves configuration that must exist before any
// work item is written: Jira custom-field → Assets schema relationships and
// the complete set of populated select/multiselect labels in the import scope.
// Jira permits one Assets schema per custom field while allowing that field on
// multiple projects, so this resolution is global by Jira field ID rather than
// per project. Windshift choice fields store numeric option IDs, so labels are
// discovered up front and normalized before values are written. All source
// calls are read-only searches.
func (h *JiraImportHandler) preflightJiraCustomFields(
	ctx context.Context,
	jobID string,
	client jira.Client,
	projectKeys []string,
	openIssuesOnly bool,
	mappings []CustomFieldMapping,
) (
	issueKeysByProject map[string][]string,
	resolved map[string]int,
	applicableFieldsByProject map[string]map[string]bool,
	choiceLabelsByField map[string][]string,
) {
	issueKeysByProject = make(map[string][]string, len(projectKeys))
	applicableFieldsByProject = make(map[string]map[string]bool, len(projectKeys))
	assetMappings := make([]CustomFieldMapping, 0)
	choiceMappings := make([]CustomFieldMapping, 0)
	fieldIDs := make([]string, 0)
	seenFieldIDs := make(map[string]bool)
	for _, mapping := range mappings {
		if mapping.Action == "skip" || mapping.JiraID == "" {
			continue
		}
		switch mapping.WindshiftType {
		case string(jira.FieldTypeAsset):
			assetMappings = append(assetMappings, mapping)
		case string(jira.FieldTypeSelect), string(jira.FieldTypeMultiselect):
			choiceMappings = append(choiceMappings, mapping)
		}
		if !seenFieldIDs[mapping.JiraID] {
			seenFieldIDs[mapping.JiraID] = true
			fieldIDs = append(fieldIDs, mapping.JiraID)
		}
	}
	sort.Strings(fieldIDs)

	for _, projectKey := range projectKeys {
		project, projectErr := client.GetProject(ctx, projectKey)
		if projectErr == nil && project != nil && !project.Simplified && project.Style != "next-gen" {
			if fields, fieldsErr := client.GetProjectFields(ctx, []string{project.ID}); fieldsErr == nil {
				for _, field := range fields {
					for _, mapping := range mappings {
						if field.ID == mapping.JiraID {
							if applicableFieldsByProject[projectKey] == nil {
								applicableFieldsByProject[projectKey] = make(map[string]bool)
							}
							applicableFieldsByProject[projectKey][mapping.JiraID] = true
						}
					}
				}
			}
		}
		keys, err := getJiraIssueKeysInImportOrder(ctx, client, projectKey, openIssuesOnly)
		if err != nil {
			slog.Warn("Jira custom-field preflight could not list issue keys",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Any("error", err))
			continue
		}
		issueKeysByProject[projectKey] = keys
	}
	if len(fieldIDs) == 0 {
		return issueKeysByProject, map[string]int{}, applicableFieldsByProject, map[string][]string{}
	}

	setCandidates := make(map[string]map[int]struct{}, len(assetMappings))
	choiceLabels := make(map[string]map[string]string, len(choiceMappings))
	for _, projectKey := range projectKeys {
		keys := issueKeysByProject[projectKey]
		for start := 0; start < len(keys); start += 100 {
			end := start + 100
			if end > len(keys) {
				end = len(keys)
			}
			result, err := client.BulkFetchIssues(ctx, jira.BulkFetchRequest{
				IssueIdsOrKeys: keys[start:end],
				Fields:         fieldIDs,
			})
			if err != nil {
				slog.Warn("Jira custom-field preflight could not read issue fields",
					slog.String("component", "jira"),
					slog.Any("error", err))
				continue
			}
			for _, issue := range result.Issues {
				for _, mapping := range mappings {
					if _, present := issue.Fields.CustomFields[mapping.JiraID]; !present {
						continue
					}
					if applicableFieldsByProject[projectKey] == nil {
						applicableFieldsByProject[projectKey] = make(map[string]bool)
					}
					applicableFieldsByProject[projectKey][mapping.JiraID] = true
				}
				for _, mapping := range assetMappings {
					value := issue.Fields.CustomFields[mapping.JiraID]
					for _, ref := range h.resolveJiraIssueAssetReferences(jobID, value) {
						if ref.SetID == 0 {
							continue
						}
						if setCandidates[mapping.JiraID] == nil {
							setCandidates[mapping.JiraID] = make(map[int]struct{})
						}
						setCandidates[mapping.JiraID][ref.SetID] = struct{}{}
					}
				}
				for _, mapping := range choiceMappings {
					value := issue.Fields.CustomFields[mapping.JiraID]
					for _, label := range customFieldDisplayValues(value) {
						label = strings.TrimSpace(label)
						if label == "" {
							continue
						}
						if choiceLabels[mapping.JiraID] == nil {
							choiceLabels[mapping.JiraID] = make(map[string]string)
						}
						key := strings.ToLower(label)
						if _, exists := choiceLabels[mapping.JiraID][key]; !exists {
							choiceLabels[mapping.JiraID][key] = label
						}
					}
				}
			}
		}
	}

	resolved = make(map[string]int, len(assetMappings))
	for _, mapping := range assetMappings {
		switch strings.TrimSpace(mapping.AssetSchemaID) {
		case "text":
			continue
		case "", "auto":
			if len(setCandidates[mapping.JiraID]) == 1 {
				for setID := range setCandidates[mapping.JiraID] {
					resolved[mapping.JiraID] = setID
				}
			}
		default:
			if setID, ok := h.importedJiraAssetSetID(jobID, mapping.AssetSchemaID); ok {
				resolved[mapping.JiraID] = setID
			}
		}
	}
	choiceLabelsByField = make(map[string][]string, len(choiceLabels))
	for fieldID, labelsByKey := range choiceLabels {
		labels := make([]string, 0, len(labelsByKey))
		for _, label := range labelsByKey {
			labels = append(labels, label)
		}
		sort.Slice(labels, func(i, j int) bool {
			return strings.ToLower(labels[i]) < strings.ToLower(labels[j])
		})
		choiceLabelsByField[fieldID] = labels
	}
	return issueKeysByProject, resolved, applicableFieldsByProject, choiceLabelsByField
}

func (h *JiraImportHandler) bindJiraImportFieldsToWorkspace(
	workspaceID int,
	projectKey string,
	customFieldIDMap map[string]int,
	mappings []CustomFieldMapping,
	applicableFields map[string]bool,
) error {
	fieldIDs := make([]int, 0)
	seen := make(map[int]struct{})
	if fieldID := customFieldIDMap[jiraIssueKeyFieldSourceID]; fieldID > 0 {
		seen[fieldID] = struct{}{}
		fieldIDs = append(fieldIDs, fieldID)
	}
	for _, mapping := range mappings {
		if mapping.Action == "skip" || !applicableFields[mapping.JiraID] {
			continue
		}
		fieldID := customFieldIDMap[mapping.JiraID]
		if fieldID == 0 || h.imports.CustomFieldType(fieldID) != string(jira.FieldTypeAsset) {
			continue
		}
		if _, exists := seen[fieldID]; exists {
			continue
		}
		seen[fieldID] = struct{}{}
		fieldIDs = append(fieldIDs, fieldID)
	}
	if len(fieldIDs) == 0 {
		return nil
	}
	sort.Ints(fieldIDs)
	return h.imports.BindFieldsToWorkspace(workspaceID, projectKey, fieldIDs)
}

func (h *JiraImportHandler) completePagedIssueContainers(ctx context.Context, issue *jira.JiraIssue, client jira.Client) error {
	if issue == nil || issue.Key == "" {
		return nil
	}
	var errs []error
	if err := h.completeIssueComments(ctx, issue, client); err != nil {
		errs = append(errs, err)
	}
	if err := h.completeIssueWorklogs(ctx, issue, client); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (h *JiraImportHandler) completeIssueWatchers(ctx context.Context, issue *jira.JiraIssue, client jira.Client) {
	if issue == nil || issue.Key == "" {
		return
	}
	if issue.Fields.Watches != nil && issue.Fields.Watches.WatchCount == 0 {
		issue.Fields.WatcherIdentitiesAvailable = true
		issue.Fields.Watchers = nil
		return
	}
	watcherClient, ok := client.(jira.IssueWatchersClient)
	if !ok {
		if issue.Fields.Watches != nil && issue.Fields.Watches.WatchCount > 0 {
			issue.Fields.WatcherFetchError = "watcher identities are unavailable from this Jira client"
		}
		return
	}
	watchers, err := watcherClient.GetIssueWatchers(ctx, issue.Key)
	if err != nil {
		issue.Fields.WatcherFetchError = err.Error()
		slog.Warn("Failed to load Jira issue watcher identities",
			slog.String("component", "jira"),
			slog.String("issue", issue.Key),
			slog.Any("error", err))
		return
	}
	if watchers == nil {
		issue.Fields.WatcherFetchError = "Jira returned an empty watcher response"
		return
	}
	issue.Fields.WatcherIdentitiesAvailable = true
	issue.Fields.Watchers = watchers.Watchers
	if issue.Fields.Watches == nil {
		issue.Fields.Watches = &jira.JiraWatchSummary{
			Self:       watchers.Self,
			WatchCount: watchers.WatchCount,
			IsWatching: watchers.IsWatching,
		}
	}
}

func (h *JiraImportHandler) completeIssueComments(ctx context.Context, issue *jira.JiraIssue, client jira.Client) error {
	container := issue.Fields.Comment
	if container == nil || container.Total <= len(container.Comments) {
		return nil
	}
	maxResults := container.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	comments := append([]jira.JiraComment{}, container.Comments...)
	for startAt := len(comments); startAt < container.Total; startAt = len(comments) {
		page, err := client.GetIssueComments(ctx, issue.Key, startAt, maxResults)
		if err != nil {
			return fmt.Errorf("fetch comments page startAt=%d: %w", startAt, err)
		}
		if page == nil || len(page.Comments) == 0 {
			break
		}
		comments = append(comments, page.Comments...)
		if page.Total > 0 {
			container.Total = page.Total
		}
	}
	container.Comments = comments
	return nil
}

func (h *JiraImportHandler) annotateJiraServiceDeskCommentVisibility(ctx context.Context, issue *jira.JiraIssue, client jira.Client) error {
	if issue == nil || issue.Key == "" || issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) == 0 {
		return nil
	}
	// Fail closed: if JSM visibility cannot be fetched or a comment is absent
	// from that response, importing it as an internal note cannot expose an
	// agent-only Jira comment to portal customers.
	for idx := range issue.Fields.Comment.Comments {
		public := false
		issue.Fields.Comment.Comments[idx].ServiceDeskPublic = &public
	}
	serviceDeskComments, err := client.ListServiceDeskRequestComments(ctx, issue.Key)
	if err != nil {
		return err
	}
	visibilityByID := make(map[string]bool, len(serviceDeskComments))
	for _, comment := range serviceDeskComments {
		visibilityByID[comment.ID] = comment.Public
	}
	for idx := range issue.Fields.Comment.Comments {
		if public, ok := visibilityByID[issue.Fields.Comment.Comments[idx].ID]; ok {
			issue.Fields.Comment.Comments[idx].ServiceDeskPublic = &public
		}
	}
	return nil
}

func (h *JiraImportHandler) completeIssueWorklogs(ctx context.Context, issue *jira.JiraIssue, client jira.Client) error {
	container := issue.Fields.Worklog
	if container == nil || container.Total <= len(container.Worklogs) {
		return nil
	}
	maxResults := container.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	worklogs := append([]jira.JiraWorklog{}, container.Worklogs...)
	for startAt := len(worklogs); startAt < container.Total; startAt = len(worklogs) {
		page, err := client.GetIssueWorklogs(ctx, issue.Key, startAt, maxResults)
		if err != nil {
			return fmt.Errorf("fetch worklogs page startAt=%d: %w", startAt, err)
		}
		if page == nil || len(page.Worklogs) == 0 {
			break
		}
		worklogs = append(worklogs, page.Worklogs...)
		if page.Total > 0 {
			container.Total = page.Total
		}
	}
	container.Worklogs = worklogs
	return nil
}

type jiraImportWorkflowGroup struct {
	sourceID            string
	name                string
	description         string
	statusIDs           []int
	itemTypeIDs         []int
	typeNames           []string
	transitions         []jira.JiraConfiguredWorkflowTransition
	authoritative       bool
	rulesComplete       bool
	guardedTransitions  int
	unsupportedLoops    int
	unsupportedActions  int
	unsupportedTriggers int
}

type jiraImportIssueTypeInfo struct {
	jiraID              string
	windshiftItemTypeID int
	windshiftStatusIDs  []int
	jiraName            string
}

type jiraImportWorkflowEdge struct {
	fromStatusID *int
	toStatusID   int
	reviewLocked bool
}

// ensureWorkflowsAndConfigSet fetches Jira's configured graph when the client
// supports it, creates Windshift workflow(s), and assigns a configuration set
// to the workspace. Status membership is only a fallback: it is never expanded
// into guessed all-to-all transitions.
func (h *JiraImportHandler) ensureWorkflowsAndConfigSet(
	ctx context.Context, jobID string, projectKey string, workspaceID int,
	statusMap map[string]int, itemTypeMap map[string]int, client jira.Client,
) error {
	// Check if workspace already has a configuration set
	existingCSID, err := h.imports.WorkspaceConfigurationSetID(workspaceID)
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

	var issueTypeInfos []jiraImportIssueTypeInfo

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

		issueTypeInfos = append(issueTypeInfos, jiraImportIssueTypeInfo{
			jiraID:              its.ID,
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

	groups := make([]jiraImportWorkflowGroup, 0)
	if capable, ok := client.(jira.WorkflowConfigurationClient); ok {
		project, projectErr := client.GetProject(ctx, projectKey)
		if projectErr == nil && project != nil {
			issueTypeIDs := make([]string, 0, len(issueTypeInfos))
			for _, info := range issueTypeInfos {
				issueTypeIDs = append(issueTypeIDs, info.jiraID)
			}
			sort.Strings(issueTypeIDs)
			config, configErr := capable.GetProjectWorkflowConfiguration(ctx, project.ID, issueTypeIDs)
			if configErr == nil && config != nil {
				groups = authoritativeJiraWorkflowGroups(config, issueTypeInfos, statusMap)
			} else if configErr != nil {
				slog.Warn("Jira configured workflow graph unavailable; using conservative status-membership fallback",
					slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", configErr))
			}
		} else {
			slog.Warn("Could not resolve Jira project ID for configured workflow graph; using conservative fallback",
				slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", projectErr))
		}
	}
	if len(groups) == 0 {
		groups = fallbackJiraWorkflowGroups(projectKey, issueTypeInfos)
	}

	definitions := make([]jiraimport.WorkflowDefinition, 0, len(groups))
	for groupIndex := range groups {
		group := &groups[groupIndex]
		workflowName := group.name
		if group.authoritative {
			workflowName = projectKey + " - " + group.name
		}
		var edges []jiraImportWorkflowEdge
		if group.authoritative {
			edges = authoritativeJiraWorkflowEdges(group, statusMap)
		} else {
			initialStatusID, initialErr := h.conservativeJiraInitialStatus(group.statusIDs)
			if initialErr != nil {
				return initialErr
			}
			edges = []jiraImportWorkflowEdge{{toStatusID: initialStatusID}}
		}
		serviceEdges := make([]jiraimport.WorkflowEdge, 0, len(edges))
		for _, edge := range edges {
			serviceEdges = append(serviceEdges, jiraimport.WorkflowEdge{
				FromStatusID: edge.fromStatusID, ToStatusID: edge.toStatusID, ReviewLocked: edge.reviewLocked,
			})
		}
		fidelity := "authoritative_graph"
		if !group.authoritative {
			fidelity = "status_membership_fallback"
		}
		sourceID := group.sourceID
		if sourceID == "" {
			sourceID = fmt.Sprintf("fallback-%s-%d", projectKey, groupIndex)
		}
		definitions = append(definitions, jiraimport.WorkflowDefinition{
			SourceID: sourceID, Name: workflowName, Description: group.description,
			Edges: serviceEdges, ItemTypeIDs: group.itemTypeIDs,
			Metadata: map[string]any{
				"fidelity": fidelity, "transition_count": len(edges),
				"transition_rules_complete":         group.rulesComplete,
				"guarded_transition_count":          group.guardedTransitions,
				"unsupported_loop_transition_count": group.unsupportedLoops,
				"review_locked_transition_count":    countReviewLockedJiraEdges(edges),
				"unsupported_transition_actions":    group.unsupportedActions,
				"unsupported_transition_triggers":   group.unsupportedTriggers,
			},
		})
	}
	configSetID, err := h.imports.CreateDetailedWorkflowConfiguration(ctx, jobID, projectKey, workspaceID, definitions)
	if err != nil {
		return err
	}

	slog.Info("Created workflows and configuration set for import",
		slog.String("component", "jira"),
		slog.String("project", projectKey),
		slog.Int("workflows", len(definitions)),
		slog.Int("configSetID", configSetID))

	return nil
}

func authoritativeJiraWorkflowGroups(
	config *jira.JiraProjectWorkflowConfiguration,
	issueTypeInfos []jiraImportIssueTypeInfo,
	statusMap map[string]int,
) []jiraImportWorkflowGroup {
	itemTypeIDsByWorkflow := make(map[string][]int)
	assignedItemTypeCount := 0
	for _, info := range issueTypeInfos {
		workflowID := config.IssueTypeWorkflowIDs[info.jiraID]
		if workflowID != "" {
			itemTypeIDsByWorkflow[workflowID] = append(itemTypeIDsByWorkflow[workflowID], info.windshiftItemTypeID)
			assignedItemTypeCount++
		}
	}
	if assignedItemTypeCount != len(issueTypeInfos) {
		return nil
	}

	groups := make([]jiraImportWorkflowGroup, 0, len(config.Workflows))
	groupedItemTypeCount := 0
	for _, workflow := range config.Workflows {
		itemTypeIDs := itemTypeIDsByWorkflow[workflow.ID]
		if len(itemTypeIDs) == 0 {
			continue
		}
		statusIDs := make([]int, 0, len(workflow.StatusIDs))
		seenStatuses := make(map[int]bool)
		seenJiraStatuses := make(map[string]bool)
		for _, jiraStatusID := range workflow.StatusIDs {
			if seenJiraStatuses[jiraStatusID] {
				continue
			}
			seenJiraStatuses[jiraStatusID] = true
			statusID, ok := statusMap[jiraStatusID]
			if !ok {
				return nil
			}
			if !seenStatuses[statusID] {
				seenStatuses[statusID] = true
				statusIDs = append(statusIDs, statusID)
			}
		}
		if len(statusIDs) == 0 {
			return nil
		}
		sort.Ints(statusIDs)
		sort.Ints(itemTypeIDs)
		groupedItemTypeCount += len(itemTypeIDs)
		groups = append(groups, jiraImportWorkflowGroup{
			sourceID:      workflow.ID,
			name:          workflow.Name,
			description:   workflow.Description,
			statusIDs:     statusIDs,
			itemTypeIDs:   itemTypeIDs,
			transitions:   workflow.Transitions,
			authoritative: true,
			rulesComplete: config.RulesComplete,
		})
	}
	if groupedItemTypeCount != len(issueTypeInfos) {
		return nil
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].sourceID < groups[j].sourceID
	})
	return groups
}

func fallbackJiraWorkflowGroups(
	projectKey string,
	issueTypeInfos []jiraImportIssueTypeInfo,
) []jiraImportWorkflowGroup {
	groupsByStatuses := make(map[string]*jiraImportWorkflowGroup)
	for _, info := range issueTypeInfos {
		parts := make([]string, len(info.windshiftStatusIDs))
		for i, id := range info.windshiftStatusIDs {
			parts[i] = strconv.Itoa(id)
		}
		key := strings.Join(parts, ",")
		group, ok := groupsByStatuses[key]
		if !ok {
			group = &jiraImportWorkflowGroup{
				name:        projectKey + " Workflow",
				description: "Generated from Jira status membership because the configured workflow graph was unavailable. No directed transitions were inferred.",
				statusIDs:   append([]int(nil), info.windshiftStatusIDs...),
			}
			groupsByStatuses[key] = group
		}
		group.itemTypeIDs = append(group.itemTypeIDs, info.windshiftItemTypeID)
		group.typeNames = append(group.typeNames, info.jiraName)
	}

	keys := make([]string, 0, len(groupsByStatuses))
	for key := range groupsByStatuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]jiraImportWorkflowGroup, 0, len(keys))
	for _, key := range keys {
		group := groupsByStatuses[key]
		sort.Ints(group.itemTypeIDs)
		sort.Strings(group.typeNames)
		if len(groupsByStatuses) > 1 {
			group.name = projectKey + " - " + strings.Join(group.typeNames, ", ") + " Workflow"
		}
		groups = append(groups, *group)
	}
	return groups
}

func authoritativeJiraWorkflowEdges(
	group *jiraImportWorkflowGroup,
	statusMap map[string]int,
) []jiraImportWorkflowEdge {
	edgesByKey := make(map[string]jiraImportWorkflowEdge)
	addEdge := func(fromStatusID *int, toStatusID int, reviewLocked bool) {
		key := "initial:" + strconv.Itoa(toStatusID)
		if fromStatusID != nil {
			if *fromStatusID == toStatusID {
				group.unsupportedLoops++
				return
			}
			key = strconv.Itoa(*fromStatusID) + ":" + strconv.Itoa(toStatusID)
		}
		edge := edgesByKey[key]
		edge.fromStatusID = fromStatusID
		edge.toStatusID = toStatusID
		edge.reviewLocked = edge.reviewLocked || reviewLocked
		edgesByKey[key] = edge
	}

	for _, transition := range group.transitions {
		group.unsupportedActions += transition.ActionCount
		group.unsupportedTriggers += transition.TriggerCount
		explicitlyGuarded := transition.ValidatorCount > 0 || transition.ConditionCount > 0
		if explicitlyGuarded {
			group.guardedTransitions++
		}
		// Do not turn incomplete source visibility into a synthetic deny rule.
		// Only a guard that Jira explicitly reports should require review.
		reviewLocked := explicitlyGuarded
		toStatusID, ok := statusMap[transition.ToStatusID]
		if !ok {
			continue
		}
		switch transition.Type {
		case jira.JiraWorkflowTransitionInitial:
			addEdge(nil, toStatusID, false)
		case jira.JiraWorkflowTransitionDirected:
			for _, jiraFromStatusID := range transition.FromStatusIDs {
				if fromStatusID, exists := statusMap[jiraFromStatusID]; exists {
					fromID := fromStatusID
					addEdge(&fromID, toStatusID, reviewLocked)
				}
			}
		case jira.JiraWorkflowTransitionGlobal:
			for _, fromStatusID := range group.statusIDs {
				fromID := fromStatusID
				addEdge(&fromID, toStatusID, reviewLocked)
			}
		}
	}

	keys := make([]string, 0, len(edgesByKey))
	for key := range edgesByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	edges := make([]jiraImportWorkflowEdge, 0, len(keys))
	for _, key := range keys {
		edges = append(edges, edgesByKey[key])
	}
	return edges
}

func countReviewLockedJiraEdges(edges []jiraImportWorkflowEdge) int {
	count := 0
	for _, edge := range edges {
		if edge.reviewLocked {
			count++
		}
	}
	return count
}

func (h *JiraImportHandler) conservativeJiraInitialStatus(statusIDs []int) (int, error) {
	return h.imports.ConservativeInitialStatus(statusIDs)
}

type jiraImportedScreenContext struct {
	createID int
	editID   int
	viewID   int
}

func (c jiraImportedScreenContext) key() string {
	return fmt.Sprintf("%d:%d:%d", c.createID, c.editID, c.viewID)
}

func (h *JiraImportHandler) ensureJiraProjectScreens(
	ctx context.Context,
	jobID string,
	projectKey string,
	workspaceID int,
	itemTypeMap map[string]int,
	customFieldIDMap map[string]int,
	customFieldMappings []CustomFieldMapping,
	client jira.Client,
) error {
	capable, ok := client.(jira.ScreenConfigurationClient)
	if !ok {
		return jira.ErrScreenConfigurationNotAvailable
	}
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("resolve Jira project for screens: %w", err)
	}
	projectIssueTypes, err := client.GetProjectIssueTypes(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("resolve Jira project issue types for screens: %w", err)
	}
	issueTypeIDs := make([]string, 0, len(projectIssueTypes))
	for _, issueType := range projectIssueTypes {
		if itemTypeMap[issueType.ID] != 0 {
			issueTypeIDs = append(issueTypeIDs, issueType.ID)
		}
	}
	if len(issueTypeIDs) == 0 {
		return fmt.Errorf("%w: no mapped issue types for project %s", jira.ErrScreenConfigurationNotAvailable, projectKey)
	}
	sort.Strings(issueTypeIDs)
	config, err := capable.GetProjectScreenConfiguration(ctx, project.ID, projectKey, issueTypeIDs)
	if err != nil {
		return err
	}
	if config == nil || len(config.Screens) == 0 {
		return jira.ErrScreenConfigurationNotAvailable
	}

	screenIDMap := make(map[string]int, len(config.Screens))
	for _, sourceScreen := range config.Screens {
		name := fmt.Sprintf("%s - %s (Jira %s)", projectKey, sourceScreen.Name, sourceScreen.ID)
		screenID, wasCreated, omittedFields, screenErr := h.ensureJiraConfiguredScreen(
			name,
			sourceScreen,
			customFieldIDMap,
			customFieldMappings,
		)
		if screenErr != nil {
			return screenErr
		}
		screenIDMap[sourceScreen.ID] = screenID
		action := "reuse_existing"
		if wasCreated {
			action = "create"
		}
		if err := h.recordMapping(jobID, "screen", sourceScreen.ID, sourceScreen.Name, screenID, map[string]any{
			"action":         action,
			"tab_count":      sourceScreen.TabCount,
			"tabs_flattened": sourceScreen.TabCount > 1,
			"field_count":    len(sourceScreen.Fields),
			"omitted_fields": omittedFields,
		}); err != nil {
			return fmt.Errorf("record Jira configured screen mapping: %w", err)
		}
	}

	contextByIssueType := make(map[string]jiraImportedScreenContext)
	contextFrequency := make(map[string]int)
	contextByKey := make(map[string]jiraImportedScreenContext)
	for issueTypeID, source := range config.IssueTypeScreens {
		screenContext := jiraImportedScreenContext{
			createID: screenIDMap[source.CreateScreenID],
			editID:   screenIDMap[source.EditScreenID],
			viewID:   screenIDMap[source.ViewScreenID],
		}
		if screenContext.createID == 0 || screenContext.editID == 0 || screenContext.viewID == 0 {
			return fmt.Errorf("jira issue type %s references a screen definition that was not returned", issueTypeID)
		}
		contextByIssueType[issueTypeID] = screenContext
		contextFrequency[screenContext.key()]++
		contextByKey[screenContext.key()] = screenContext
	}
	if len(contextFrequency) == 0 {
		return jira.ErrScreenConfigurationNotAvailable
	}

	defaultKey := ""
	defaultCount := -1
	keys := make([]string, 0, len(contextFrequency))
	for key := range contextFrequency {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if contextFrequency[key] > defaultCount {
			defaultKey = key
			defaultCount = contextFrequency[key]
		}
	}
	defaultContext := contextByKey[defaultKey]

	itemTypeScreens := make(map[int][3]*int)
	for issueTypeID, context := range contextByIssueType {
		itemTypeID := itemTypeMap[issueTypeID]
		if itemTypeID == 0 {
			continue
		}
		var createID, editID, viewID *int
		if context.createID != defaultContext.createID {
			value := context.createID
			createID = &value
		}
		if context.editID != defaultContext.editID {
			value := context.editID
			editID = &value
		}
		if context.viewID != defaultContext.viewID {
			value := context.viewID
			viewID = &value
		}
		itemTypeScreens[itemTypeID] = [3]*int{createID, editID, viewID}
	}
	configSetID, err := h.imports.AssignConfigurationScreens(workspaceID, map[string]int{
		"create": defaultContext.createID,
		"edit":   defaultContext.editID,
		"view":   defaultContext.viewID,
	}, itemTypeScreens)
	if err != nil {
		return fmt.Errorf("assign Jira screens: %w", err)
	}

	if err := h.recordMapping(jobID, "screen_configuration", project.ID, projectKey, configSetID, map[string]any{
		"fidelity":              "authoritative_layout",
		"screen_count":          len(config.Screens),
		"issue_type_count":      len(contextByIssueType),
		"default_context_usage": defaultCount,
		"was_created":           false,
	}); err != nil {
		return fmt.Errorf("record Jira screen configuration mapping: %w", err)
	}
	return nil
}

func (h *JiraImportHandler) ensureJiraConfiguredScreen(
	name string,
	source jira.JiraScreenConfiguration,
	customFieldIDMap map[string]int,
	customFieldMappings []CustomFieldMapping,
) (screenID int, wasCreated bool, omittedFields int, err error) {
	seen := make(map[string]bool)
	fields := make([]jiraimport.ScreenField, 0, len(source.Fields))
	for _, sourceField := range source.Fields {
		fieldType, identifier, mapped := mapJiraScreenField(
			sourceField.ID,
			customFieldIDMap,
			customFieldMappings,
		)
		if !mapped {
			omittedFields++
			continue
		}
		key := fieldType + ":" + identifier
		if seen[key] {
			continue
		}
		seen[key] = true
		fields = append(fields, jiraimport.ScreenField{
			Type: fieldType, Identifier: identifier,
			Required: fieldType == "system" && identifier == "title",
		})
	}
	screenID, wasCreated, err = h.imports.EnsureConfiguredScreen(name, source.Description, fields)
	return screenID, wasCreated, omittedFields, err
}

func mapJiraScreenField(
	jiraFieldID string,
	customFieldIDMap map[string]int,
	customFieldMappings []CustomFieldMapping,
) (fieldType, identifier string, ok bool) {
	systemFields := map[string]string{
		"summary":     "title",
		"description": "description",
		"status":      "status",
		"priority":    "priority",
		"assignee":    "assignee",
		"duedate":     "due_date",
		"fixVersions": "milestone",
		"labels":      "labels",
	}
	if identifier = systemFields[jiraFieldID]; identifier != "" {
		return "system", identifier, true
	}
	for _, mapping := range customFieldMappings {
		if mapping.JiraID == jiraFieldID && isJiraSprintField(mapping) {
			return "system", "iteration", true
		}
	}
	if customFieldID := customFieldIDMap[jiraFieldID]; customFieldID > 0 {
		return "custom", strconv.Itoa(customFieldID), true
	}
	return "", "", false
}

func (h *JiraImportHandler) ensureJiraIterations(ctx context.Context, jobID string, workspaceID int, projectKey string, client jira.Client) (map[string]int, error) {
	result := make(map[string]int)
	boards, err := client.ListBoards(ctx, projectKey)
	if err != nil {
		return result, err
	}
	if boards == nil || len(boards.Values) == 0 {
		return result, nil
	}

	typeID, err := h.ensureIterationType("Sprint", "#3b82f6", "Imported Jira Software sprint")
	if err != nil {
		return result, err
	}

	seen := make(map[string]struct{})
	for _, board := range boards.Values {
		if !jiraBoardSupportsSprints(board) {
			continue
		}
		sprints, err := client.GetBoardSprints(ctx, board.ID)
		if err != nil {
			slog.Warn("Failed to fetch Jira board sprints",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Int("boardID", board.ID),
				slog.Any("error", err))
			continue
		}
		if sprints == nil {
			continue
		}
		for _, sprint := range sprints.Values {
			sprintID := strconv.Itoa(sprint.ID)
			if _, ok := seen[sprintID]; ok {
				continue
			}
			seen[sprintID] = struct{}{}

			iterationID, ok := h.ensureJiraSprintIteration(workspaceID, typeID, sprint)
			if !ok {
				slog.Warn("Skipping Jira sprint without usable dates",
					slog.String("component", "jira"),
					slog.String("project", projectKey),
					slog.Int("sprintID", sprint.ID),
					slog.String("sprint", sprint.Name))
				continue
			}
			result[sprintID] = iterationID
			if err := h.recordMapping(jobID, "iteration", sprintID, sprint.Name, iterationID, map[string]any{
				"jira_board_id":   board.ID,
				"jira_board_name": board.Name,
				"jira_state":      sprint.State,
				"jira_goal":       sprint.Goal,
			}); err != nil {
				return nil, fmt.Errorf("record Jira iteration mapping: %w", err)
			}
		}
	}
	return result, nil
}

func jiraBoardSupportsSprints(board jira.JiraBoard) bool {
	return strings.EqualFold(strings.TrimSpace(board.Type), "scrum")
}

func (h *JiraImportHandler) ensureIterationType(name, color, description string) (int, error) {
	return h.imports.EnsureIterationType(name, color, description)
}

func (h *JiraImportHandler) ensureJiraSprintIteration(workspaceID, typeID int, sprint jira.JiraSprint) (int, bool) {
	return h.imports.EnsureSprintIteration(workspaceID, typeID, sprint)
}

func (h *JiraImportHandler) ensureJiraTimeProject(jobID string, workspaceID int, projectKey, projectName string) (*int, error) {
	return h.imports.EnsureTimeProject(jobID, workspaceID, projectKey, projectName)
}

// ensureWorkspace creates a dedicated workspace for an imported Jira project.
// createdByUserID grants the import initiator workspace admin access; pass 0 if unknown.
func (h *JiraImportHandler) ensureWorkspace(ctx context.Context, jobID string, mapping *WorkspaceMapping, createdByUserID int) (int, error) {
	if !mapping.CreateNew || mapping.WindshiftID != nil {
		return 0, fmt.Errorf("jira project %s must create a new workspace; existing workspaces cannot be reused", mapping.JiraKey)
	}

	workspaceSvc := services.NewWorkspaceService(h.db)

	// Create new workspace using service
	result, err := workspaceSvc.Create(ctx, services.CreateWorkspaceParams{
		Name:        mapping.NewWorkspaceName,
		Key:         mapping.NewWorkspaceKey,
		Description: "Imported from Jira",
		CreatorID:   createdByUserID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Record the mapping
	if err := h.recordMapping(jobID, "workspace", mapping.JiraKey, mapping.JiraKey, result.Workspace.ID, map[string]any{
		"action":      "create",
		"was_created": true,
	}); err != nil {
		return 0, fmt.Errorf("record Jira workspace mapping: %w", err)
	}

	return result.Workspace.ID, nil
}

type jiraAffectsVersionCustomField struct {
	FieldID              int
	OptionIDsByJiraID    map[string]int
	OptionLabelsByJiraID map[string]string
}

type jiraImportCustomFieldOption struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

type jiraImportCustomFieldOptions struct {
	NextID int                           `json:"next_id"`
	Items  []jiraImportCustomFieldOption `json:"items"`
}

func mergeJiraChoiceOptions(
	raw string,
	labels []string,
) (encodedOptions string, optionIDs map[string]int, err error) {
	options := jiraImportCustomFieldOptions{NextID: 1, Items: []jiraImportCustomFieldOption{}}
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &options); err != nil {
			var legacyLabels []string
			if legacyErr := json.Unmarshal([]byte(raw), &legacyLabels); legacyErr != nil {
				return "", nil, fmt.Errorf("decode existing choice options: %w", err)
			}
			for _, label := range legacyLabels {
				options.Items = append(options.Items, jiraImportCustomFieldOption{
					ID:    options.NextID,
					Label: label,
				})
				options.NextID++
			}
		}
	}
	if options.NextID <= 0 {
		options.NextID = 1
	}
	optionIDs = make(map[string]int, len(options.Items)+len(labels))
	for _, item := range options.Items {
		if item.ID >= options.NextID {
			options.NextID = item.ID + 1
		}
		label := strings.TrimSpace(item.Label)
		if label != "" {
			optionIDs[strings.ToLower(label)] = item.ID
		}
	}
	sortedLabels := append([]string(nil), labels...)
	sort.Slice(sortedLabels, func(i, j int) bool {
		return strings.ToLower(sortedLabels[i]) < strings.ToLower(sortedLabels[j])
	})
	for _, label := range sortedLabels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, exists := optionIDs[key]; exists {
			continue
		}
		optionIDs[key] = options.NextID
		options.Items = append(options.Items, jiraImportCustomFieldOption{
			ID:    options.NextID,
			Label: label,
		})
		options.NextID++
	}
	data, err := json.Marshal(options)
	if err != nil {
		return "", nil, err
	}
	return string(data), optionIDs, nil
}

func (h *JiraImportHandler) ensureJiraChoiceFieldOptions(
	fieldID int,
	labels []string,
) (map[string]int, error) {
	fieldType, rawOptions, err := h.imports.ChoiceFieldOptions(fieldID)
	if err != nil {
		return nil, err
	}
	if fieldType != string(jira.FieldTypeSelect) && fieldType != string(jira.FieldTypeMultiselect) {
		return nil, fmt.Errorf("custom field %d has type %q, not a choice type", fieldID, fieldType)
	}
	merged, optionIDs, err := mergeJiraChoiceOptions(rawOptions, labels)
	if err != nil {
		return nil, err
	}
	if merged != strings.TrimSpace(rawOptions) {
		if err := h.imports.UpdateChoiceFieldOptions(fieldID, merged); err != nil {
			return nil, err
		}
	}
	return optionIDs, nil
}

func (h *JiraImportHandler) ensureAffectsVersionCustomField(_ context.Context, jobID string, mappings []VersionMapping) (*jiraAffectsVersionCustomField, error) {
	if len(mappings) == 0 {
		return nil, nil
	}
	const (
		jiraID    = "system:versions"
		name      = "Jira Affects Version/s"
		fieldType = "multiselect"
	)

	options := jiraImportCustomFieldOptions{NextID: 1}
	optionIDByLabel := make(map[string]int)
	optionIDsByJiraID := make(map[string]int)
	optionLabelsByJiraID := make(map[string]string)
	ensureOption := func(label string) int {
		label = strings.TrimSpace(label)
		if label == "" {
			return 0
		}
		key := strings.ToLower(label)
		if id, ok := optionIDByLabel[key]; ok {
			return id
		}
		id := options.NextID
		if id <= 0 {
			id = len(options.Items) + 1
		}
		options.Items = append(options.Items, jiraImportCustomFieldOption{ID: id, Label: label})
		optionIDByLabel[key] = id
		options.NextID = id + 1
		return id
	}

	field, err := h.imports.FindAffectsVersionField(name, fieldType)
	fieldID := 0
	existingType := ""
	existingOptions := ""
	if field != nil {
		fieldID = field.ID
		existingType = field.FieldType
		existingOptions = field.Options
	}
	if err == nil && strings.TrimSpace(existingOptions) != "" {
		var parsed jiraImportCustomFieldOptions
		if json.Unmarshal([]byte(existingOptions), &parsed) == nil {
			options = parsed
			if options.NextID <= 0 {
				options.NextID = 1
			}
			for _, item := range options.Items {
				if item.ID >= options.NextID {
					options.NextID = item.ID + 1
				}
				if strings.TrimSpace(item.Label) != "" {
					optionIDByLabel[strings.ToLower(strings.TrimSpace(item.Label))] = item.ID
				}
			}
		}
	}

	for _, m := range mappings {
		label := strings.TrimSpace(m.JiraName)
		if label == "" || m.JiraID == "" {
			continue
		}
		optionID := ensureOption(label)
		if optionID == 0 {
			continue
		}
		optionIDsByJiraID[m.JiraID] = optionID
		optionLabelsByJiraID[m.JiraID] = label
	}

	optionBytes, marshalErr := json.Marshal(options)
	if marshalErr != nil {
		return nil, marshalErr
	}
	description := "Imported from Jira system field versions (Affects Version/s). Stores all affected versions as multiselect values; Jira version IDs and metadata are also preserved in item metadata."

	wasCreated := errors.Is(err, repository.ErrNotFound)
	if wasCreated {
		now := time.Now()
		fieldID, err = h.imports.CreateCustomField(name, fieldType, description, string(optionBytes), now)
	} else if err == nil {
		err = h.imports.UpdateCustomField(fieldID, name, fieldType, description, string(optionBytes), time.Now())
	}
	if err != nil {
		return nil, err
	}

	meta := map[string]any{
		"action":          "create_or_reuse",
		"jira_field_type": "system:versions",
		"windshift_type":  fieldType,
		"option_count":    len(options.Items),
		"was_created":     wasCreated,
	}
	if existingType != "" && existingType != fieldType {
		meta["previous_windshift_type"] = existingType
	}
	if err := h.recordMapping(jobID, "custom_field", jiraID, name, fieldID, meta); err != nil {
		return nil, fmt.Errorf("record Jira affects-version mapping: %w", err)
	}
	return &jiraAffectsVersionCustomField{FieldID: fieldID, OptionIDsByJiraID: optionIDsByJiraID, OptionLabelsByJiraID: optionLabelsByJiraID}, nil
}

// ensureCustomFields creates or maps global Windshift custom fields selected
// in the Jira mapping step. The returned map is Jira customfield_* ID →
// Windshift custom_field_definitions.id and is used when writing an item's
// custom_field_values JSON so imported values are keyed by Windshift field IDs,
// not transient Jira keys.
//
// Story Points is intentionally excluded: it maps to items.story_points as a
// first-class field during issue import.
//
//nolint:unparam // context kept for symmetry with the other ensure* helpers.
func (h *JiraImportHandler) ensureCustomFields(
	_ context.Context,
	jobID string,
	mappings []CustomFieldMapping,
	assetFieldSetIDs map[string]int,
	choiceLabelsByField map[string][]string,
	configurationCaptures ...map[string]jiraCustomFieldConfigurationCapture,
) (fieldIDs map[string]int, choiceOptionIDs map[string]map[string]int, err error) {
	fieldIDs = make(map[string]int)
	choiceOptionIDs = make(map[string]map[string]int)
	now := time.Now()
	var capturedConfigurations map[string]jiraCustomFieldConfigurationCapture
	if len(configurationCaptures) > 0 {
		capturedConfigurations = configurationCaptures[0]
	}

	for _, m := range mappings {
		if m.Action == "skip" || m.JiraID == "" || isJiraStoryPointsField(m) || isJiraSprintField(m) ||
			m.JiraType == jiraRequestTypeFieldType {
			continue
		}

		fieldType := models.CanonicalCustomFieldType(strings.TrimSpace(m.WindshiftType))
		if fieldType == "" || fieldType == string(jira.FieldTypeUnmapped) {
			continue
		}
		fieldOptions := ""
		isChoice := fieldType == string(jira.FieldTypeSelect) || fieldType == string(jira.FieldTypeMultiselect)
		if isChoice {
			fieldOptions = `{"next_id":1,"items":[]}`
		}
		if fieldType == string(jira.FieldTypeAsset) {
			assetSetID, ok := assetFieldSetIDs[m.JiraID]
			if ok && assetSetID > 0 {
				fieldOptions = fmt.Sprintf(`{"asset_set_id":%d,"ql_query":"","multi":true}`, assetSetID)
			} else {
				// An Assets custom field has one configured Jira schema, but an empty
				// field does not expose that schema in issue payloads. Unless the
				// operator selected a schema explicitly, preserve its values as text
				// rather than inventing an asset-set relationship.
				fieldType = "textarea"
			}
		}
		if !isValidFieldType(fieldType) {
			slog.Warn("Skipping Jira custom field with unsupported Windshift type",
				slog.String("component", "jira"),
				slog.String("jiraFieldID", m.JiraID),
				slog.String("jiraFieldName", m.JiraName),
				slog.String("windshiftType", fieldType))
			continue
		}

		if m.Action == "map" && m.WindshiftID != nil {
			if isChoice {
				optionIDs, optionErr := h.ensureJiraChoiceFieldOptions(*m.WindshiftID, choiceLabelsByField[m.JiraID])
				if optionErr != nil {
					return nil, nil, fmt.Errorf("ensure Jira choice options for %s: %w", m.JiraID, optionErr)
				}
				choiceOptionIDs[m.JiraID] = optionIDs
			}
			fieldIDs[m.JiraID] = *m.WindshiftID
			metadata := map[string]any{
				"action":          "map",
				"option_count":    len(choiceOptionIDs[m.JiraID]),
				"preserve_raw":    m.PreserveRaw,
				"jira_field_type": m.JiraType,
				"windshift_type":  fieldType,
			}
			addJiraCustomFieldConfigurationMetadata(metadata, capturedConfigurations[m.JiraID])
			if err := h.recordMapping(jobID, "custom_field", m.JiraID, m.JiraName, *m.WindshiftID, metadata); err != nil {
				return nil, nil, fmt.Errorf("record Jira mapped custom field mapping: %w", err)
			}
			continue
		}

		name := strings.TrimSpace(m.JiraName)
		if name == "" {
			name = m.JiraID
		}
		sourceDescription := fmt.Sprintf("Imported from Jira field %s (%s)", m.JiraID, m.JiraType)

		var fieldID int
		wasCreated := false
		baseName := name
		for attempt := 0; attempt < 10; attempt++ {
			existing, findErr := h.imports.FindCustomField(name)
			err = findErr
			existingType := ""
			existingOptions := ""
			if existing != nil {
				fieldID = existing.ID
				existingType = existing.FieldType
				existingOptions = existing.Options
			}
			if err == nil {
				// Action=create must retain Jira source identity. Jira permits
				// distinct custom fields with the same display name and type, so
				// only reuse a definition previously created for this Jira field.
				// Operators can explicitly choose action=map to reuse an unrelated
				// existing Windshift definition.
				compatible := models.CanonicalCustomFieldType(existingType) == fieldType && existing.Description == sourceDescription
				if compatible && fieldType == string(jira.FieldTypeAsset) {
					var existingConfig struct {
						AssetSetID int `json:"asset_set_id"`
					}
					var requestedConfig struct {
						AssetSetID int `json:"asset_set_id"`
					}
					compatible = json.Unmarshal([]byte(existingOptions), &existingConfig) == nil &&
						json.Unmarshal([]byte(fieldOptions), &requestedConfig) == nil &&
						existingConfig.AssetSetID > 0 &&
						existingConfig.AssetSetID == requestedConfig.AssetSetID
				}
				if compatible {
					break
				}
				// Same field name but a different source or type: keep both fields
				// with a deterministic Jira-specific name.
				if attempt == 9 {
					err = fmt.Errorf("custom field name %q exists with incompatible type %q", name, existingType)
					break
				}
				name = fmt.Sprintf("%s (Jira %s%s)", baseName, strings.TrimPrefix(m.JiraID, "customfield_"), strings.Repeat("-", attempt))
				continue
			}
			if !errors.Is(err, repository.ErrNotFound) {
				break
			}
			description := fmt.Sprintf("Imported from Jira field %s (%s)", m.JiraID, m.JiraType)
			fieldID, err = h.imports.CreateCustomField(name, fieldType, description, fieldOptions, now)
			wasCreated = err == nil
			break
		}
		if err != nil {
			slog.Error("Failed to ensure Jira custom field",
				slog.String("component", "jira"),
				slog.String("jiraFieldID", m.JiraID),
				slog.String("jiraFieldName", name),
				slog.Any("error", err))
			continue
		}

		if isChoice {
			optionIDs, optionErr := h.ensureJiraChoiceFieldOptions(fieldID, choiceLabelsByField[m.JiraID])
			if optionErr != nil {
				return nil, nil, fmt.Errorf("ensure Jira choice options for %s: %w", m.JiraID, optionErr)
			}
			choiceOptionIDs[m.JiraID] = optionIDs
		}
		fieldIDs[m.JiraID] = fieldID
		metadata := map[string]any{
			"action":          "create_or_reuse",
			"jira_field_type": m.JiraType,
			"windshift_type":  fieldType,
			"was_created":     wasCreated,
			"option_count":    len(choiceOptionIDs[m.JiraID]),
			"preserve_raw":    m.PreserveRaw,
		}
		addJiraCustomFieldConfigurationMetadata(metadata, capturedConfigurations[m.JiraID])
		if err := h.recordMapping(jobID, "custom_field", m.JiraID, name, fieldID, metadata); err != nil {
			return nil, nil, fmt.Errorf("record Jira custom field mapping: %w", err)
		}
	}

	return fieldIDs, choiceOptionIDs, nil
}

func addJiraCustomFieldConfigurationMetadata(
	metadata map[string]any,
	capture jiraCustomFieldConfigurationCapture,
) {
	if metadata == nil {
		return
	}
	if capture.configuration == nil {
		metadata["configuration_status"] = "unavailable"
		if capture.unavailableReason != "" {
			metadata["configuration_unavailable_reason"] = capture.unavailableReason
		}
		return
	}
	metadata["configuration_status"] = "preserved"
	metadata["contexts"] = capture.configuration.Contexts
	metadata["context_count"] = len(capture.configuration.Contexts)
	if capture.configuration.DefaultsUnavailableReason != "" {
		metadata["defaults_unavailable_reason"] = capture.configuration.DefaultsUnavailableReason
	}
	optionCount := 0
	optionUnavailableContextCount := 0
	disabledOptionCount := 0
	defaultCount := 0
	for _, context := range capture.configuration.Contexts {
		optionCount += len(context.Options)
		defaultCount += len(context.Defaults)
		if context.OptionsUnavailableReason != "" {
			optionUnavailableContextCount++
		}
		for _, option := range context.Options {
			if option.Disabled {
				disabledOptionCount++
			}
		}
	}
	metadata["configured_option_count"] = optionCount
	metadata["option_unavailable_context_count"] = optionUnavailableContextCount
	metadata["disabled_option_count"] = disabledOptionCount
	metadata["default_count"] = defaultCount
	metadata["applicability_enforcement"] = "preserved_only_global_windshift_field"
}

// ensureJiraIssueKeyCustomField provides an item-facing, queryable home for
// keys such as SP-42. The importer also retains _jira_issue_key metadata for
// backwards compatibility, but underscore metadata is not a declared custom
// field and therefore is not addressable by name in QL.
func (h *JiraImportHandler) ensureJiraIssueKeyCustomField(jobID string) (int, error) {
	const description = "Original Jira issue key. Automatically populated by Jira import and queryable as `cf_Jira Key`."
	names := []string{jiraIssueKeyFieldName, "Imported Jira Key"}
	for suffix := 2; suffix <= 10; suffix++ {
		names = append(names, fmt.Sprintf("Imported Jira Key %d", suffix))
	}

	fieldID, name, created, err := h.imports.EnsureIssueKeyField(
		names, description, string(jira.FieldTypeText),
	)
	if err != nil {
		return 0, fmt.Errorf("ensure Jira Key custom field: %w", err)
	}
	action := "reuse_existing"
	if created {
		action = "create"
	}
	if err := h.recordMapping(jobID, "custom_field", jiraIssueKeyFieldSourceID, name, fieldID, map[string]any{
		"action": action, "jira_field_type": "system:issue-key",
		"windshift_type": string(jira.FieldTypeText), "was_created": created, "searchable": true,
	}); err != nil {
		return 0, fmt.Errorf("record Jira issue-key mapping: %w", err)
	}
	return fieldID, nil
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
		existingID, exists, err := h.imports.FindMilestone(m.JiraName, workspaceID)
		if err != nil {
			return result, err
		}
		if exists {
			result[m.JiraID] = existingID
			if err := h.recordMapping(jobID, "milestone", m.JiraID, m.JiraName, existingID, map[string]any{
				"action":      "reuse_existing",
				"was_created": false,
			}); err != nil {
				return nil, fmt.Errorf("record Jira milestone mapping: %w", err)
			}
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
		if err := h.recordMapping(jobID, "milestone", m.JiraID, m.JiraName, milestone.ID, map[string]any{
			"action":      "create",
			"was_created": true,
		}); err != nil {
			return nil, fmt.Errorf("record Jira milestone mapping: %w", err)
		}
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
		existing, err := h.imports.FindStatus(m.JiraName)
		if err == nil {
			existingID := existing.ID
			// Status exists, use existing ID
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = existingID
			}
			if len(m.JiraIDs) > 0 {
				if err := h.recordMapping(jobID, "status", m.JiraIDs[0], m.JiraName, existingID, map[string]any{
					"action":      "reuse_existing",
					"was_created": false,
				}); err != nil {
					return nil, fmt.Errorf("record Jira status mapping: %w", err)
				}
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
			if err := h.recordMapping(jobID, "status", m.JiraIDs[0], m.JiraName, statusID, map[string]any{
				"action":      "create",
				"was_created": true,
			}); err != nil {
				return nil, fmt.Errorf("record Jira status mapping: %w", err)
			}
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
		existing, err := h.imports.FindItemType(m.JiraName)
		if err == nil {
			existingID := existing.ID
			// Item type exists, use existing ID
			for _, jiraID := range m.JiraIDs {
				result[jiraID] = existingID
			}
			if len(m.JiraIDs) > 0 {
				if err := h.recordMapping(jobID, "item_type", m.JiraIDs[0], m.JiraName, existingID, map[string]any{
					"action":      "reuse_existing",
					"was_created": false,
				}); err != nil {
					return nil, fmt.Errorf("record Jira item type mapping: %w", err)
				}
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
			if err := h.recordMapping(jobID, "item_type", m.JiraIDs[0], m.JiraName, itemTypeID, map[string]any{
				"action":      "create",
				"was_created": true,
			}); err != nil {
				return nil, fmt.Errorf("record Jira item type mapping: %w", err)
			}
		}
	}

	return result, nil
}
