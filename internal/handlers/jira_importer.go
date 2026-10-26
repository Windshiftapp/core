package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"windshift/internal/jiraimport"
	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
	"windshift/internal/xray"

	"uuid"
)

// ReconcileInterruptedImports fails Jira import jobs whose worker lease
// expired, so a server restart cannot leave a queued/running job blocking
// retry or cleanup forever. Safe to call at startup and on demand.
func (h *JiraImportHandler) ReconcileInterruptedImports() (int, error) {
	return h.imports.ReconcileInterrupted(time.Now().UTC())
}

// GetJobStatus handles GET /api/admin/jira-import/jobs/{jobId}
func (h *JiraImportHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.imports.GetJobStatus(r.PathValue("jobId"))
	if errors.Is(err, jiraimport.ErrJobNotFound) {
		respondNotFound(w, r, "job")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, status)
}

// GetImportJobs handles GET /api/admin/jira-import/jobs
func (h *JiraImportHandler) GetImportJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.imports.ListJobs()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, jobs)
}

// sanitizeStartImportRequest bounds persisted/rendered Jira identifiers and
// names; numeric Windshift IDs remain untouched.
func sanitizeStartImportRequest(req *StartImportRequest) {
	sanitize.Apply(&req.ConnectionID, sanitize.ShortIdentifier)
	for i := range req.ProjectKeys {
		sanitize.Apply(&req.ProjectKeys[i], sanitize.ShortIdentifier)
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Xray.Region, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Xray.ClientID, Policy: sanitize.ShortIdentifier},
	)
	for i := range req.Xray.TestIssueTypeIDs {
		sanitize.Apply(&req.Xray.TestIssueTypeIDs[i], sanitize.ShortIdentifier)
	}
	m := &req.Mappings
	for i := range m.Workspaces {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &m.Workspaces[i].JiraKey, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.Workspaces[i].JiraName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.Workspaces[i].NewWorkspaceName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.Workspaces[i].NewWorkspaceKey, Policy: sanitize.ShortIdentifier},
		)
	}
	for i := range m.IssueTypes {
		for j := range m.IssueTypes[i].JiraIDs {
			sanitize.Apply(&m.IssueTypes[i].JiraIDs[j], sanitize.ShortIdentifier)
		}
		sanitize.Apply(&m.IssueTypes[i].JiraName, sanitize.PlainTextField)
	}
	for i := range m.Statuses {
		for j := range m.Statuses[i].JiraIDs {
			sanitize.Apply(&m.Statuses[i].JiraIDs[j], sanitize.ShortIdentifier)
		}
		sanitize.ApplyAll(
			sanitize.Pair{Target: &m.Statuses[i].JiraName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.Statuses[i].CategoryKey, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.Statuses[i].CategoryName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.Statuses[i].Color, Policy: sanitize.ShortIdentifier},
		)
	}
	for i := range m.CustomFields {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &m.CustomFields[i].JiraID, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.CustomFields[i].JiraName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.CustomFields[i].JiraType, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.CustomFields[i].WindshiftType, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.CustomFields[i].Notes, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.CustomFields[i].Action, Policy: sanitize.ShortIdentifier},
		)
	}
	for i := range m.Versions {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &m.Versions[i].JiraID, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.Versions[i].JiraName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &m.Versions[i].ProjectKey, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &m.Versions[i].ReleaseDate, Policy: sanitize.ShortIdentifier},
		)
	}
}

func (h *JiraImportHandler) validateJiraWorkspaceMappings(req StartImportRequest) error {
	existingKeys, err := h.imports.WorkspaceKeys()
	if err != nil {
		return fmt.Errorf("load existing workspace keys: %w", err)
	}

	mappingsByJiraKey := make(map[string]WorkspaceMapping, len(req.Mappings.Workspaces))
	for _, mapping := range req.Mappings.Workspaces {
		jiraKey := normalizeJiraProjectKey(mapping.JiraKey)
		if jiraKey == "" {
			continue
		}
		if _, duplicate := mappingsByJiraKey[jiraKey]; duplicate {
			return fmt.Errorf("jira project %s has more than one workspace mapping", jiraKey)
		}
		mappingsByJiraKey[jiraKey] = mapping
	}

	targetKeys := make(map[string]string, len(req.ProjectKeys))
	targetWorkspaceIDs := make(map[int]string, len(req.ProjectKeys))
	for _, requestedKey := range req.ProjectKeys {
		jiraKey := normalizeJiraProjectKey(requestedKey)
		mapping, ok := mappingsByJiraKey[jiraKey]
		if !ok {
			return fmt.Errorf("jira project %s is missing a workspace mapping", jiraKey)
		}

		if !mapping.CreateNew {
			if mapping.WindshiftID == nil {
				return fmt.Errorf("jira project %s must choose a target workspace", jiraKey)
			}
			target, err := h.imports.WorkspaceImportTarget(*mapping.WindshiftID)
			if errors.Is(err, repository.ErrNotFound) || (err == nil && target == nil) {
				return fmt.Errorf("jira project %s references a workspace that cannot be imported into", jiraKey)
			}
			if err != nil {
				return fmt.Errorf("load workspace target for %s: %w", jiraKey, err)
			}
			if otherJiraKey, duplicate := targetWorkspaceIDs[target.ID]; duplicate {
				return fmt.Errorf("jira projects %s and %s cannot import into the same workspace", otherJiraKey, jiraKey)
			}
			targetWorkspaceIDs[target.ID] = jiraKey
			continue
		}

		if mapping.WindshiftID != nil {
			return fmt.Errorf("jira project %s cannot both create a new workspace and reuse an existing one", jiraKey)
		}
		if strings.TrimSpace(mapping.NewWorkspaceName) == "" {
			return fmt.Errorf("jira project %s requires a workspace name", jiraKey)
		}

		targetKey := normalizeJiraProjectKey(mapping.NewWorkspaceKey)
		if targetKey == "" {
			return fmt.Errorf("jira project %s requires a workspace key", jiraKey)
		}
		if _, exists := existingKeys[targetKey]; exists {
			return fmt.Errorf("workspace key %s is already in use; analyze the projects again to assign a new Jira alias", targetKey)
		}
		if otherJiraKey, duplicate := targetKeys[targetKey]; duplicate {
			return fmt.Errorf("jira projects %s and %s cannot use the same workspace key %s", otherJiraKey, jiraKey, targetKey)
		}
		targetKeys[targetKey] = jiraKey

		_, originalKeyExists := existingKeys[jiraKey]
		aliasRequired := originalKeyExists || targetKey != jiraKey
		if aliasRequired && !mapping.KeyAliasAcknowledged {
			return fmt.Errorf("acknowledge that Jira project %s will use workspace key alias %s", jiraKey, targetKey)
		}
	}
	return nil
}

// validateJiraEntityMappings rejects stale reuse targets so a deleted status,
// item type, or custom field cannot silently degrade into a new entity.
func (h *JiraImportHandler) validateJiraEntityMappings(req StartImportRequest) error {
	for _, mapping := range req.Mappings.Statuses {
		if mapping.CreateNew || mapping.WindshiftID == nil {
			continue
		}
		exists, err := h.imports.StatusExists(*mapping.WindshiftID)
		if err != nil {
			return fmt.Errorf("load mapped status for %s: %w", mapping.JiraName, err)
		}
		if !exists {
			return fmt.Errorf("jira status %s references a status that does not exist", mapping.JiraName)
		}
	}
	for _, mapping := range req.Mappings.IssueTypes {
		if mapping.CreateNew || mapping.WindshiftID == nil {
			continue
		}
		exists, err := h.imports.ItemTypeExists(*mapping.WindshiftID)
		if err != nil {
			return fmt.Errorf("load mapped item type for %s: %w", mapping.JiraName, err)
		}
		if !exists {
			return fmt.Errorf("jira issue type %s references an item type that does not exist", mapping.JiraName)
		}
	}
	for _, mapping := range req.Mappings.CustomFields {
		if mapping.Action != "map" || mapping.WindshiftID == nil {
			continue
		}
		existingType, err := h.imports.CustomFieldTargetType(*mapping.WindshiftID)
		if err != nil {
			return fmt.Errorf("load mapped custom field for %s: %w", mapping.JiraName, err)
		}
		if existingType == "" {
			return fmt.Errorf("jira field %s references a custom field that does not exist", mapping.JiraName)
		}
		if !jiraimport.CustomFieldTypesCompatible(existingType, mapping.WindshiftType) {
			return fmt.Errorf("jira field %s (%s) cannot map to a %s custom field", mapping.JiraName, mapping.WindshiftType, existingType)
		}
	}
	return nil
}

// StartImport handles POST /api/admin/jira-import/start
// Starts a background import job and returns immediately with the job ID
func (h *JiraImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[StartImportRequest](w, r)
	if !ok {
		return
	}
	sanitizeStartImportRequest(&req)

	if req.ConnectionID == "" || len(req.ProjectKeys) == 0 {
		respondValidationError(w, r, "connection_id and project_keys are required")
		return
	}
	if err := h.validateJiraWorkspaceMappings(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if err := h.validateJiraEntityMappings(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if req.Xray.ImportTests {
		deploymentType, err := h.imports.ConnectionDeploymentType(req.ConnectionID)
		if err != nil {
			respondValidationError(w, r, "Jira connection was not found")
			return
		}
		isDataCenter := deploymentType == "datacenter"
		if !isDataCenter {
			if strings.TrimSpace(req.Xray.ClientID) == "" || strings.TrimSpace(req.Xray.ClientSecret) == "" {
				respondValidationError(w, r, "Xray Cloud client ID and client secret are required")
				return
			}
			switch req.Xray.Region {
			case "", "global":
				req.Xray.Region = "global"
			case "us", "eu", "au":
			default:
				respondValidationError(w, r, "Xray Cloud region must be global, us, eu, or au")
				return
			}
			xrayClient, err := xray.NewCloudClient(xray.CloudConfig{
				ClientID:     req.Xray.ClientID,
				ClientSecret: req.Xray.ClientSecret,
				Region:       req.Xray.Region,
			})
			if err != nil {
				respondValidationError(w, r, err.Error())
				return
			}
			if err := xrayClient.Validate(r.Context()); err != nil {
				slog.Debug("Xray Cloud credential validation failed",
					slog.String("component", "jira"),
					slog.Any("error", err))
				respondError(w, r, restapi.NewAPIError(
					http.StatusBadRequest,
					"XRAY_AUTH_FAILED",
					"Xray Cloud credentials could not be validated.",
				))
				return
			}
		}
	}

	planFingerprint, err := jiraImportPlanFingerprint(req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get user ID from context
	userID := getUserIDFromContext(r)

	enqueueResult, err := h.enqueueJiraImport(req, planFingerprint, userID)
	if err != nil {
		var conflictErr *jiraImportConflictError
		if errors.As(err, &conflictErr) {
			apiErr := restapi.NewAPIError(
				http.StatusConflict,
				"JIRA_IMPORT_CONFLICT",
				conflictErr.Message,
			).WithDetails(map[string]any{
				"conflicting_imports": conflictErr.Conflicts,
			})
			respondError(w, r, apiErr)
			return
		}
		respondInternalError(w, r, err)
		return
	}
	jobID := enqueueResult.JobID

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
			Details: map[string]any{
				"connection_id": req.ConnectionID,
				"project_keys":  req.ProjectKeys,
			},
			Success: true,
		})
	}

	// Start the import in a background goroutine
	go h.executeImport(jobID, req) //nolint:gosec // G118: an import job must outlive its initiating HTTP request.

	respondJSONOK(w, StartImportResponse{
		JobID:   jobID,
		Message: "Import started successfully",
	})
}

type jiraImportEnqueueResult struct {
	JobID     string
	Conflicts []jiraImportConflict
}

type jiraImportConflictError struct {
	Message   string
	Conflicts []jiraImportConflict
}

func (e *jiraImportConflictError) Error() string {
	return e.Message
}

func (h *JiraImportHandler) enqueueJiraImport(
	req StartImportRequest,
	planFingerprint string,
	userID *int,
) (*jiraImportEnqueueResult, error) {
	// Recover jobs whose worker lease expired before checking conflicts so a
	// server crash cannot permanently block a retry.
	if _, err := h.imports.ReconcileInterrupted(time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("reconcile interrupted Jira imports: %w", err)
	}

	jobID := uuid.New().String()
	jobRepository := repository.NewJiraImportJobRepository(h.db)
	var result *jiraImportEnqueueResult
	err := jobRepository.WithLockedConnection(req.ConnectionID, func(jobs repository.JiraImportJobStore) error {
		conflicts, err := findConflictingJiraImportsWithQuery(jobs, req, planFingerprint)
		if err != nil {
			return err
		}
		activeConflicts := make([]jiraImportConflict, 0, len(conflicts))
		for _, conflict := range conflicts {
			if conflict.Status == "queued" || conflict.Status == "running" {
				activeConflicts = append(activeConflicts, conflict)
			}
		}
		if len(activeConflicts) > 0 {
			return &jiraImportConflictError{
				Message:   "One or more selected Jira projects already have an import queued or running.",
				Conflicts: activeConflicts,
			}
		}
		if !req.ForceReimport && len(conflicts) > 0 {
			return &jiraImportConflictError{
				Message:   "One or more selected Jira projects have already been imported. Delete the previous import data or explicitly force a re-import.",
				Conflicts: conflicts,
			}
		}

		// Store only the durable, non-secret configuration. The Xray Cloud
		// client ID and secret remain solely in the in-memory request.
		configJSON, err := jiraImportJobConfigJSON(req, conflicts...)
		if err != nil {
			return err
		}
		if err := jobs.Insert(repository.JiraImportJobInsert{
			ID:             jobID,
			ConnectionID:   req.ConnectionID,
			ConfigJSON:     string(configJSON),
			CreatedBy:      userID,
			LeaseExpiresAt: time.Now().UTC().Add(jiraimport.JiraImportLeaseDuration).Unix(),
		}); err != nil {
			return err
		}
		result = &jiraImportEnqueueResult{JobID: jobID, Conflicts: conflicts}
		return nil
	})
	return result, err
}

func jiraImportJobConfigJSON(req StartImportRequest, conflicts ...jiraImportConflict) ([]byte, error) {
	fingerprint, err := jiraImportPlanFingerprint(req)
	if err != nil {
		return nil, err
	}
	configurationDrift := false
	for _, conflict := range conflicts {
		if conflict.ConfigurationDrift {
			configurationDrift = true
			break
		}
	}
	return json.Marshal(map[string]any{
		"project_keys":        req.ProjectKeys,
		"open_issues_only":    req.OpenIssuesOnly,
		"mappings":            req.Mappings,
		"plan_fingerprint":    fingerprint,
		"configuration_drift": configurationDrift,
		"previous_imports":    conflicts,
		"xray": map[string]any{
			"import_tests": req.Xray.ImportTests,
			"region":       req.Xray.Region,
		},
		"force_reimport": req.ForceReimport,
	})
}

type jiraImportCandidateLister interface {
	ListCandidates(connectionID string) ([]repository.JiraImportJobRecord, error)
}

func findConflictingJiraImportsWithQuery(
	jobs jiraImportCandidateLister,
	req StartImportRequest,
	fingerprints ...string,
) ([]jiraImportConflict, error) {
	fingerprint := ""
	if len(fingerprints) > 0 {
		fingerprint = fingerprints[0]
	} else {
		var err error
		fingerprint, err = jiraImportPlanFingerprint(req)
		if err != nil {
			return nil, err
		}
	}
	requested := projectKeySet(req.ProjectKeys)
	if len(requested) == 0 || req.ConnectionID == "" {
		return nil, nil
	}

	candidates, err := jobs.ListCandidates(req.ConnectionID)
	if err != nil {
		return nil, err
	}

	var conflicts []jiraImportConflict
	for _, candidate := range candidates {
		projectKeys := extractJiraImportProjectKeys(candidate.ConfigJSON)
		if !projectKeysOverlap(requested, projectKeys) {
			continue
		}
		conflict := jiraImportConflict{
			JobID:                   candidate.ID,
			Status:                  candidate.Status,
			ProjectKeys:             projectKeys,
			PreviousPlanFingerprint: jiraImportPlanFingerprintFromConfig(candidate.ConfigJSON),
			CreatedAt:               candidate.CreatedAt,
			CompletedAt:             candidate.CompletedAt,
		}
		conflict.ConfigurationDrift = conflict.PreviousPlanFingerprint == "" ||
			conflict.PreviousPlanFingerprint != fingerprint
		conflicts = append(conflicts, conflict)
	}
	return conflicts, nil
}

func jiraImportPlanFingerprintFromConfig(configJSON string) string {
	var stored struct {
		ProjectKeys     []string          `json:"project_keys"`
		OpenIssuesOnly  bool              `json:"open_issues_only"`
		Mappings        ImportMappings    `json:"mappings"`
		Xray            XrayImportOptions `json:"xray"`
		PlanFingerprint string            `json:"plan_fingerprint"`
	}
	if err := json.Unmarshal([]byte(configJSON), &stored); err != nil {
		return ""
	}
	if stored.PlanFingerprint != "" {
		return stored.PlanFingerprint
	}
	fingerprint, err := jiraImportPlanFingerprint(StartImportRequest{
		ProjectKeys:    stored.ProjectKeys,
		OpenIssuesOnly: stored.OpenIssuesOnly,
		Mappings:       stored.Mappings,
		Xray:           stored.Xray,
	})
	if err != nil {
		return ""
	}
	return fingerprint
}

func jiraImportPlanFingerprint(req StartImportRequest) (string, error) {
	projectKeys := make([]string, 0, len(req.ProjectKeys))
	for key := range projectKeySet(req.ProjectKeys) {
		projectKeys = append(projectKeys, key)
	}
	sort.Strings(projectKeys)

	mappings := req.Mappings
	mappings.Workspaces = append([]WorkspaceMapping(nil), mappings.Workspaces...)
	for i := range mappings.Workspaces {
		mappings.Workspaces[i].IssueCount = 0
		mappings.Workspaces[i].KeyAliasAcknowledged = false
	}
	sort.Slice(mappings.Workspaces, func(i, j int) bool {
		return normalizeJiraProjectKey(mappings.Workspaces[i].JiraKey) <
			normalizeJiraProjectKey(mappings.Workspaces[j].JiraKey)
	})

	mappings.IssueTypes = append([]IssueTypeMapping(nil), mappings.IssueTypes...)
	for i := range mappings.IssueTypes {
		mappings.IssueTypes[i].JiraIDs = append([]string(nil), mappings.IssueTypes[i].JiraIDs...)
		sort.Strings(mappings.IssueTypes[i].JiraIDs)
	}
	sort.Slice(mappings.IssueTypes, func(i, j int) bool {
		left := strings.Join(mappings.IssueTypes[i].JiraIDs, ",") + "\x00" + mappings.IssueTypes[i].JiraName
		right := strings.Join(mappings.IssueTypes[j].JiraIDs, ",") + "\x00" + mappings.IssueTypes[j].JiraName
		return left < right
	})

	mappings.Statuses = append([]StatusMapping(nil), mappings.Statuses...)
	for i := range mappings.Statuses {
		mappings.Statuses[i].JiraIDs = append([]string(nil), mappings.Statuses[i].JiraIDs...)
		sort.Strings(mappings.Statuses[i].JiraIDs)
	}
	sort.Slice(mappings.Statuses, func(i, j int) bool {
		left := strings.Join(mappings.Statuses[i].JiraIDs, ",") + "\x00" + mappings.Statuses[i].JiraName
		right := strings.Join(mappings.Statuses[j].JiraIDs, ",") + "\x00" + mappings.Statuses[j].JiraName
		return left < right
	})

	mappings.CustomFields = append([]CustomFieldMapping(nil), mappings.CustomFields...)
	sort.Slice(mappings.CustomFields, func(i, j int) bool {
		return mappings.CustomFields[i].JiraID < mappings.CustomFields[j].JiraID
	})
	mappings.Versions = append([]VersionMapping(nil), mappings.Versions...)
	sort.Slice(mappings.Versions, func(i, j int) bool {
		left := normalizeJiraProjectKey(mappings.Versions[i].ProjectKey) + "\x00" + mappings.Versions[i].JiraID
		right := normalizeJiraProjectKey(mappings.Versions[j].ProjectKey) + "\x00" + mappings.Versions[j].JiraID
		return left < right
	})

	testIssueTypeIDs := append([]string(nil), req.Xray.TestIssueTypeIDs...)
	sort.Strings(testIssueTypeIDs)
	plan := struct {
		ProjectKeys    []string          `json:"project_keys"`
		OpenIssuesOnly bool              `json:"open_issues_only"`
		Mappings       ImportMappings    `json:"mappings"`
		Xray           XrayImportOptions `json:"xray"`
	}{
		ProjectKeys:    projectKeys,
		OpenIssuesOnly: req.OpenIssuesOnly,
		Mappings:       mappings,
		Xray: XrayImportOptions{
			ImportTests:      req.Xray.ImportTests,
			Region:           req.Xray.Region,
			TestIssueTypeIDs: testIssueTypeIDs,
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func extractJiraImportProjectKeys(configJSON string) []string {
	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
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
		normalized := normalizeJiraProjectKey(key)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		keys = append(keys, normalized)
		seen[normalized] = struct{}{}
	}
	return keys
}

func projectKeysOverlap(requested map[string]struct{}, existing []string) bool {
	for _, key := range existing {
		if _, ok := requested[normalizeJiraProjectKey(key)]; ok {
			return true
		}
	}
	return false
}

func projectKeySet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if normalized := normalizeJiraProjectKey(key); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

type jiraImportConflict = jiraimport.Conflict

func normalizeJiraProjectKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

type deleteImportedDataRequest struct {
	ConfirmJobID              string `json:"confirm_job_id"`
	ConfirmWorkspaceCount     int    `json:"confirm_workspace_count"`
	ConfirmDeleteImportedData bool   `json:"confirm_delete_imported_data"`
}

// DeleteImportedData handles DELETE /api/admin/jira-import/jobs/{jobId}/data.
func (h *JiraImportHandler) DeleteImportedData(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if jobID == "" {
		respondInvalidID(w, r, "jobId")
		return
	}
	req, ok := decodeJSON[deleteImportedDataRequest](w, r)
	if !ok {
		return
	}
	if req.ConfirmJobID != jobID || !req.ConfirmDeleteImportedData {
		respondValidationError(w, r, "Deleting imported Jira data requires confirm_job_id to match the job path and confirm_delete_imported_data=true.")
		return
	}
	deleted, err := h.imports.DeleteImportedData(jobID, req.ConfirmWorkspaceCount)
	if errors.Is(err, jiraimport.ErrJobNotFound) {
		respondNotFound(w, r, "job")
		return
	}
	if errors.Is(err, jiraimport.ErrJobActive) {
		respondConflict(w, r, "Cannot delete imported data while the import job is queued or running.")
		return
	}
	var mismatch *jiraimport.WorkspaceCountMismatchError
	if errors.As(err, &mismatch) {
		respondValidationError(w, r, fmt.Sprintf("Workspace confirmation count mismatch: request confirmed %d workspace(s), but this import currently maps %d workspace(s). Refresh and try again.", mismatch.Confirmed, mismatch.Current))
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID: currentUser.ID, Username: currentUser.Username,
			IPAddress: utils.GetClientIP(r), UserAgent: r.UserAgent(),
			ActionType:   logger.ActionJiraImportDeleteData,
			ResourceType: logger.ResourceJiraImport, ResourceName: jobID,
			Details: map[string]any{"job_id": jobID, "deleted": deleted},
			Success: true,
		})
	}
	respondJSONOK(w, map[string]any{"success": true, "deleted": deleted})
}

// HideImportJob handles DELETE /api/admin/jira-import/jobs/{jobId}. It clears
// the job from the history listing without deleting the job or its imported
// data; destructive removal stays on DELETE .../jobs/{jobId}/data.
func (h *JiraImportHandler) HideImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if jobID == "" {
		respondInvalidID(w, r, "jobId")
		return
	}
	err := h.imports.HideJob(jobID)
	if errors.Is(err, jiraimport.ErrJobNotFound) {
		respondNotFound(w, r, "job")
		return
	}
	if errors.Is(err, jiraimport.ErrJobActive) {
		respondConflict(w, r, "Cannot clear an import from the history while it is queued or running.")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID: currentUser.ID, Username: currentUser.Username,
			IPAddress: utils.GetClientIP(r), UserAgent: r.UserAgent(),
			ActionType:   logger.ActionJiraImportHide,
			ResourceType: logger.ResourceJiraImport, ResourceName: jobID,
			Details: map[string]any{"job_id": jobID},
			Success: true,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPreviousImports handles GET /api/admin/jira-import/previous-imports.
func (h *JiraImportHandler) GetPreviousImports(w http.ResponseWriter, r *http.Request) {
	projectKeys := r.URL.Query()["project_key"]
	if len(projectKeys) == 0 {
		respondValidationError(w, r, "At least one project_key is required")
		return
	}
	imports, err := h.imports.PreviousImports(projectKeys)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, imports)
}

type previousJiraImportMapping = jiraimport.PreviousMapping

func jiraImportMappingWasCreated(metadata sql.NullString) bool {
	return jiraimport.MappingWasCreated(metadata)
}

func jiraImportMappingMetadata(metadata sql.NullString) map[string]any {
	return jiraimport.Metadata(metadata)
}

func (h *JiraImportHandler) findPreviousJiraImportMapping(
	currentJobID, entityType, jiraID string,
) (*previousJiraImportMapping, error) {
	return h.imports.FindPreviousMapping(currentJobID, entityType, jiraID)
}

func (h *JiraImportHandler) rememberMappingFailure(jobID string, err error) error {
	if err == nil {
		return nil
	}
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	if h.mappingFailures == nil {
		h.mappingFailures = make(map[string]error)
	}
	if _, exists := h.mappingFailures[jobID]; !exists {
		h.mappingFailures[jobID] = err
	}
	return err
}

func (h *JiraImportHandler) mappingFailure(jobID string) error {
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	return h.mappingFailures[jobID]
}

func (h *JiraImportHandler) clearMappingFailure(jobID string) {
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	delete(h.mappingFailures, jobID)
}

func (h *JiraImportHandler) setJobToken(jobID, token string) {
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	if h.jobTokens == nil {
		h.jobTokens = make(map[string]string)
	}
	h.jobTokens[jobID] = token
}

func (h *JiraImportHandler) jobToken(jobID string) string {
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	return h.jobTokens[jobID]
}

func (h *JiraImportHandler) clearJobToken(jobID string) {
	h.mappingFailuresMu.Lock()
	defer h.mappingFailuresMu.Unlock()
	delete(h.jobTokens, jobID)
}

func (h *JiraImportHandler) failOnMappingFailure(jobID string, progress *ImportProgress) bool {
	err := h.mappingFailure(jobID)
	if err == nil {
		return false
	}
	if progress != nil {
		progress.Phase = "failed"
	}
	h.updateJobStatus(jobID, "failed", "mapping", progress, fmt.Sprintf("Failed to persist Jira import mapping: %v", err))
	return true
}

// recordMapping records an entity mapping through the Jira import module.
func (h *JiraImportHandler) recordMapping(jobID, entityType, jiraID, jiraKey string, windshiftID int, metadata map[string]any) error {
	if err := h.imports.RecordMapping(jobID, entityType, jiraID, jiraKey, windshiftID, metadata); err != nil {
		slog.Error("Failed to record mapping", slog.String("component", "jira"), slog.String("job_id", jobID),
			slog.String("entity_type", entityType), slog.String("jira_id", jiraID), slog.Any("error", err))
		return h.rememberMappingFailure(jobID, err)
	}
	return nil
}

func (h *JiraImportHandler) recordMappingAndTransferOwnership(
	jobID, entityType, jiraID, jiraKey string,
	windshiftID int,
	metadata map[string]any,
	previous *previousJiraImportMapping,
) error {
	err := h.imports.RecordMappingAndTransferOwnership(
		jobID, entityType, jiraID, jiraKey, windshiftID, metadata, previous,
	)
	return h.rememberMappingFailure(jobID, err)
}

// updateJobStatus updates the status of an import job. The write is fenced by
// the worker's ownership token so a recovered worker cannot change job state.
func (h *JiraImportHandler) updateJobStatus(jobID, status, phase string, progress *ImportProgress, errorMessage string) {
	if err := h.imports.UpdateStatus(jobID, h.jobToken(jobID), status, phase, progress, errorMessage); err != nil {
		slog.Error("Failed to update job status", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// updateJobProgress updates just the progress of a running job, fenced by the
// worker's ownership token.
func (h *JiraImportHandler) updateJobProgress(jobID string, progress *ImportProgress) {
	if err := h.imports.UpdateProgress(jobID, h.jobToken(jobID), progress); err != nil {
		slog.Error("Failed to update job progress", slog.String("component", "jira"), slog.Any("error", err))
	}
}
