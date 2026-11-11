package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"windshift/internal/jira"
)

// CapturedPayloads holds the *small* recorded artifacts of an import: the
// JQL→keys index and the accountID→email cache. Per-issue data is intentionally
// not held here — BulkFetchIssues responses are streamed to a sibling JSONL
// file (jira_bulk_fetch.jsonl) as they arrive, so a multi-thousand-issue
// capture doesn't pin gigabytes of issue payloads in process memory.
type CapturedPayloads struct {
	IssueKeys  map[string][]string `json:"issue_keys"`  // JQL -> keys
	UserEmails map[string]string   `json:"user_emails"` // accountID -> email
}

// recordingClient wraps a jira.Client and records API responses.
//
// Memory profile: O(distinct accountIDs + JQL queries) — both small constants
// per import. Per-issue data is appended to a JSONL file on disk per page, so
// a 100k-issue Cloud capture peaks at ~one BulkFetchResponse worth of RAM
// during marshaling, not the cumulative total.
type recordingClient struct {
	inner      jira.Client
	mu         sync.Mutex
	payloads   CapturedPayloads
	jsonlPaths []string
}

// newRecordingClient builds a streaming recorder. captureDir is created by the
// caller before this is invoked; we truncate the JSONL file here so re-running
// an import into the same directory starts fresh instead of doubling up.
func newRecordingClient(inner jira.Client, captureDir string) *recordingClient {
	rc := &recordingClient{
		inner: inner,
		payloads: CapturedPayloads{
			IssueKeys:  make(map[string][]string),
			UserEmails: make(map[string]string),
		},
	}
	rc.jsonlPaths = []string{
		filepath.Join(captureDir, "jira_bulk_fetch.jsonl"),
		filepath.Join(captureDir, "jira_boards.jsonl"),
		filepath.Join(captureDir, "jira_board_configurations.jsonl"),
		filepath.Join(captureDir, "jira_filters.jsonl"),
		filepath.Join(captureDir, "jira_filter_details.jsonl"),
		filepath.Join(captureDir, "jira_sprints.jsonl"),
		filepath.Join(captureDir, "jira_issue_comments.jsonl"),
		filepath.Join(captureDir, "jira_issue_worklogs.jsonl"),
		filepath.Join(captureDir, "jira_issue_watchers.jsonl"),
		filepath.Join(captureDir, "jira_custom_field_configurations.jsonl"),
		filepath.Join(captureDir, "jira_service_desks.jsonl"),
		filepath.Join(captureDir, "jira_service_desk_request_types.jsonl"),
		filepath.Join(captureDir, "jira_service_desk_request_comments.jsonl"),
		filepath.Join(captureDir, "jira_service_desk_organizations.jsonl"),
		filepath.Join(captureDir, "jira_service_desk_organization_users.jsonl"),
		filepath.Join(captureDir, "jira_project_workflow_configuration.jsonl"),
		filepath.Join(captureDir, "jira_project_screen_configuration.jsonl"),
	}
	for _, path := range rc.jsonlPaths {
		if err := os.WriteFile(path, nil, 0o600); err != nil { //nolint:gosec // path built from operator-supplied dir
			slog.Warn("Failed to truncate Jira capture JSONL file", slog.String("component", "jira"),
				slog.String("path", path), slog.Any("error", err))
		}
	}
	return rc
}

// saveToFile flushes the small in-memory metadata (issue_keys + user_emails)
// to jira_responses.json. The per-issue payloads already live in
// jira_bulk_fetch.jsonl by the time this runs — so a crashed/partial import
// keeps whatever pages arrived on disk.
func (r *recordingClient) saveToFile(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r.payloads, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal payloads: %w", err)
	}

	path := filepath.Join(dir, "jira_responses.json")
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // G703: path built from filepath.Join with known dir
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	slog.Info("Saved captured Jira responses", slog.String("component", "jira"),
		slog.String("path", path), slog.Any("jsonl_paths", r.jsonlPaths))
	return nil
}

// --- Recorded methods used during import ---

func (r *recordingClient) GetAllIssueKeys(ctx context.Context, jql string) ([]string, error) {
	keys, err := r.inner.GetAllIssueKeys(ctx, jql)
	if err == nil {
		r.mu.Lock()
		r.payloads.IssueKeys[jql] = keys
		r.mu.Unlock()
	}
	return keys, err
}

func (r *recordingClient) GetAllIssueRefs(ctx context.Context, jql string) ([]jira.JiraIssueRef, error) {
	return r.inner.GetAllIssueRefs(ctx, jql)
}

func (r *recordingClient) BulkFetchIssues(ctx context.Context, req jira.BulkFetchRequest) (*jira.BulkFetchResponse, error) {
	resp, err := r.inner.BulkFetchIssues(ctx, req)
	if err == nil && resp != nil {
		r.appendJSONL("jira_bulk_fetch.jsonl", resp)
	}
	return resp, err
}

// appendJSONL serializes a single response and appends it as one JSONL record.
// Open/append/close per call so a crashed import leaves a well-formed prefix
// on disk and no file-handle leak; the syscall cost is negligible next to the
// Jira API round-trip that produced the page.
func (r *recordingClient) appendJSONL(filename string, payload any) {
	line, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("Failed to marshal Jira capture page", slog.String("component", "jira"), slog.String("file", filename), slog.Any("error", err))
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var path string
	for _, candidate := range r.jsonlPaths {
		if filepath.Base(candidate) == filename {
			path = candidate
			break
		}
	}
	if path == "" {
		slog.Warn("Unknown Jira capture JSONL target", slog.String("component", "jira"), slog.String("file", filename))
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path built from operator-supplied dir
	if err != nil {
		slog.Warn("Failed to open Jira capture JSONL file", slog.String("component", "jira"),
			slog.String("path", path), slog.Any("error", err))
		return
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(line, '\n')); err != nil {
		slog.Warn("Failed to append Jira capture JSONL page", slog.String("component", "jira"),
			slog.String("path", path), slog.Any("error", err))
	}
}

func (r *recordingClient) GetUserEmail(ctx context.Context, accountID string) (string, error) {
	email, err := r.inner.GetUserEmail(ctx, accountID)
	if err == nil {
		r.mu.Lock()
		r.payloads.UserEmails[accountID] = email
		r.mu.Unlock()
	}
	return email, err
}

// --- Pass-through methods ---
//
// NOT RECORDED — extend the recording wrapper before adding importer calls that
// the diff harness must compare. scripts/jira_import_diff.py keeps a matching
// EXPECTED_PASSTHROUGH_GAPS set for intentionally ignored endpoints.

func (r *recordingClient) TestConnection(ctx context.Context) (*jira.JiraInstanceInfo, error) {
	return r.inner.TestConnection(ctx)
}

func (r *recordingClient) ListProjects(ctx context.Context) ([]jira.JiraProject, error) {
	return r.inner.ListProjects(ctx)
}

func (r *recordingClient) GetProject(ctx context.Context, projectKey string) (*jira.JiraProject, error) {
	return r.inner.GetProject(ctx, projectKey)
}

func (r *recordingClient) ListServiceDesks(ctx context.Context) ([]jira.JiraServiceDesk, error) {
	resp, err := r.inner.ListServiceDesks(ctx)
	if err == nil {
		r.appendJSONL("jira_service_desks.jsonl", resp)
	}
	return resp, err
}

func (r *recordingClient) ListServiceDeskRequestTypes(ctx context.Context, serviceDeskID string) ([]jira.JiraServiceDeskRequestType, error) {
	resp, err := r.inner.ListServiceDeskRequestTypes(ctx, serviceDeskID)
	if err == nil {
		r.appendJSONL("jira_service_desk_request_types.jsonl", map[string]any{
			"service_desk_id": serviceDeskID,
			"response":        resp,
		})
	}
	return resp, err
}

func (r *recordingClient) ListServiceDeskRequestComments(ctx context.Context, issueKey string) ([]jira.JiraServiceDeskComment, error) {
	resp, err := r.inner.ListServiceDeskRequestComments(ctx, issueKey)
	if err == nil {
		r.appendJSONL("jira_service_desk_request_comments.jsonl", map[string]any{
			"issue_key": issueKey,
			"response":  resp,
		})
	}
	return resp, err
}

func (r *recordingClient) ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]jira.JiraServiceDeskOrganization, error) {
	resp, err := r.inner.ListServiceDeskOrganizations(ctx, serviceDeskID)
	if err == nil {
		r.appendJSONL("jira_service_desk_organizations.jsonl", map[string]any{
			"service_desk_id": serviceDeskID,
			"response":        resp,
		})
	}
	return resp, err
}

func (r *recordingClient) ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]jira.JiraUser, error) {
	resp, err := r.inner.ListServiceDeskOrganizationUsers(ctx, organizationID)
	if err == nil {
		r.appendJSONL("jira_service_desk_organization_users.jsonl", map[string]any{
			"organization_id": organizationID,
			"response":        resp,
		})
	}
	return resp, err
}

func (r *recordingClient) ListIssueTypes(ctx context.Context) ([]jira.JiraIssueType, error) {
	return r.inner.ListIssueTypes(ctx)
}

func (r *recordingClient) GetProjectIssueTypes(ctx context.Context, projectKey string) ([]jira.JiraIssueType, error) {
	return r.inner.GetProjectIssueTypes(ctx, projectKey)
}

func (r *recordingClient) ListCustomFields(ctx context.Context) ([]jira.JiraCustomField, error) {
	return r.inner.ListCustomFields(ctx)
}

func (r *recordingClient) GetProjectFields(ctx context.Context, projectIDs []string) ([]jira.JiraCustomField, error) {
	return r.inner.GetProjectFields(ctx, projectIDs)
}

func (r *recordingClient) ListStatuses(ctx context.Context) ([]jira.JiraStatus, error) {
	return r.inner.ListStatuses(ctx)
}

func (r *recordingClient) GetStatusCategories(ctx context.Context) ([]jira.JiraStatusCategory, error) {
	return r.inner.GetStatusCategories(ctx)
}

func (r *recordingClient) GetProjectWorkflowScheme(ctx context.Context, projectKey string) (*jira.JiraWorkflow, error) {
	return r.inner.GetProjectWorkflowScheme(ctx, projectKey)
}

func (r *recordingClient) GetProjectWorkflowConfiguration(
	ctx context.Context,
	projectID string,
	issueTypeIDs []string,
) (*jira.JiraProjectWorkflowConfiguration, error) {
	capable, ok := r.inner.(jira.WorkflowConfigurationClient)
	if !ok {
		return nil, jira.ErrWorkflowConfigurationNotAvailable
	}
	resp, err := capable.GetProjectWorkflowConfiguration(ctx, projectID, issueTypeIDs)
	if err == nil {
		r.appendJSONL("jira_project_workflow_configuration.jsonl", map[string]any{
			"project_id":     projectID,
			"issue_type_ids": issueTypeIDs,
			"response":       resp,
		})
	}
	return resp, err
}

func (r *recordingClient) GetProjectScreenConfiguration(
	ctx context.Context,
	projectID string,
	projectKey string,
	issueTypeIDs []string,
) (*jira.JiraProjectScreenConfiguration, error) {
	capable, ok := r.inner.(jira.ScreenConfigurationClient)
	if !ok {
		return nil, jira.ErrScreenConfigurationNotAvailable
	}
	resp, err := capable.GetProjectScreenConfiguration(ctx, projectID, projectKey, issueTypeIDs)
	if err == nil {
		r.appendJSONL("jira_project_screen_configuration.jsonl", map[string]any{
			"project_id":     projectID,
			"project_key":    projectKey,
			"issue_type_ids": issueTypeIDs,
			"response":       resp,
		})
	}
	return resp, err
}

func (r *recordingClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]jira.JiraIssueTypeWithStatuses, error) {
	return r.inner.GetProjectIssueTypeStatuses(ctx, projectKey)
}

func (r *recordingClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*jira.JiraIssue, error) {
	return r.inner.GetIssue(ctx, issueKey, expand)
}

func (r *recordingClient) GetIssueWatchers(ctx context.Context, issueKey string) (*jira.JiraIssueWatchers, error) {
	capable, ok := r.inner.(jira.IssueWatchersClient)
	if !ok {
		return nil, fmt.Errorf("jira issue watchers capability unavailable")
	}
	resp, err := capable.GetIssueWatchers(ctx, issueKey)
	if err == nil && resp != nil {
		r.appendJSONL("jira_issue_watchers.jsonl", map[string]any{"issue_key": issueKey, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetIssueComments(ctx context.Context, issueKey string, startAt, maxResults int) (*jira.JiraCommentContainer, error) {
	resp, err := r.inner.GetIssueComments(ctx, issueKey, startAt, maxResults)
	if err == nil && resp != nil {
		r.appendJSONL("jira_issue_comments.jsonl", map[string]any{"issue_key": issueKey, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetCustomFieldConfiguration(
	ctx context.Context,
	fieldID string,
	includeOptions bool,
) (*jira.JiraCustomFieldConfiguration, error) {
	capable, ok := r.inner.(jira.CustomFieldConfigurationClient)
	if !ok {
		return nil, jira.ErrCustomFieldConfigurationNotAvailable
	}
	resp, err := capable.GetCustomFieldConfiguration(ctx, fieldID, includeOptions)
	if err == nil && resp != nil {
		r.appendJSONL("jira_custom_field_configurations.jsonl", map[string]any{
			"field_id":        fieldID,
			"include_options": includeOptions,
			"response":        resp,
		})
	}
	return resp, err
}

func (r *recordingClient) GetIssueWorklogs(ctx context.Context, issueKey string, startAt, maxResults int) (*jira.JiraWorklogContainer, error) {
	resp, err := r.inner.GetIssueWorklogs(ctx, issueKey, startAt, maxResults)
	if err == nil && resp != nil {
		r.appendJSONL("jira_issue_worklogs.jsonl", map[string]any{"issue_key": issueKey, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetIssueCount(ctx context.Context, projectKey string, openOnly bool) (int, error) {
	return r.inner.GetIssueCount(ctx, projectKey, openOnly)
}

func (r *recordingClient) SearchIssuesJQL(ctx context.Context, req jira.JQLSearchRequest) (*jira.JQLSearchResponse, error) {
	return r.inner.SearchIssuesJQL(ctx, req)
}

func (r *recordingClient) GetProjectVersions(ctx context.Context, projectKey string) ([]jira.JiraVersion, error) {
	return r.inner.GetProjectVersions(ctx, projectKey)
}

func (r *recordingClient) ListBoards(ctx context.Context, projectKey string) (*jira.BoardListResult, error) {
	resp, err := r.inner.ListBoards(ctx, projectKey)
	if err == nil && resp != nil {
		r.appendJSONL("jira_boards.jsonl", map[string]any{"project_key": projectKey, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetBoardSprints(ctx context.Context, boardID int) (*jira.SprintListResult, error) {
	resp, err := r.inner.GetBoardSprints(ctx, boardID)
	if err == nil && resp != nil {
		r.appendJSONL("jira_sprints.jsonl", map[string]any{"board_id": boardID, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetBoardConfiguration(ctx context.Context, boardID int) (*jira.JiraBoardConfiguration, error) {
	resp, err := r.inner.GetBoardConfiguration(ctx, boardID)
	if err == nil && resp != nil {
		r.appendJSONL("jira_board_configurations.jsonl", map[string]any{"board_id": boardID, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) ListFilters(ctx context.Context, projectKey string) (*jira.FilterSearchResult, error) {
	resp, err := r.inner.ListFilters(ctx, projectKey)
	if err == nil && resp != nil {
		r.appendJSONL("jira_filters.jsonl", map[string]any{"project_key": projectKey, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) GetFilter(ctx context.Context, filterID string) (*jira.JiraFilter, error) {
	resp, err := r.inner.GetFilter(ctx, filterID)
	if err == nil && resp != nil {
		r.appendJSONL("jira_filter_details.jsonl", map[string]any{"filter_id": filterID, "response": resp})
	}
	return resp, err
}

func (r *recordingClient) DownloadAttachment(ctx context.Context, attachmentID, contentURL string) (io.ReadCloser, string, error) {
	return r.inner.DownloadAttachment(ctx, attachmentID, contentURL)
}

func (r *recordingClient) ListObjectSchemas(ctx context.Context) ([]jira.AssetObjectSchema, error) {
	return r.inner.ListObjectSchemas(ctx)
}

func (r *recordingClient) GetObjectSchema(ctx context.Context, schemaID string) (*jira.AssetObjectSchema, error) {
	return r.inner.GetObjectSchema(ctx, schemaID)
}

func (r *recordingClient) ListObjectTypes(ctx context.Context, schemaID string) ([]jira.AssetObjectType, error) {
	return r.inner.ListObjectTypes(ctx, schemaID)
}

func (r *recordingClient) GetObjectTypeAttributes(ctx context.Context, objectTypeID string) ([]jira.AssetObjectAttribute, error) {
	return r.inner.GetObjectTypeAttributes(ctx, objectTypeID)
}

func (r *recordingClient) SearchObjects(ctx context.Context, opts jira.ObjectSearchOptions) (*jira.ObjectSearchResult, error) {
	return r.inner.SearchObjects(ctx, opts)
}

func (r *recordingClient) GetObjectCount(ctx context.Context, schemaID string) (int, error) {
	return r.inner.GetObjectCount(ctx, schemaID)
}
