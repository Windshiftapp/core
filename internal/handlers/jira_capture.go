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

// CapturedPayloads holds all recorded Jira API responses during an import
type CapturedPayloads struct {
	IssueKeys  map[string][]string          `json:"issue_keys"`  // JQL -> keys
	BulkFetch  []jira.BulkFetchResponse     `json:"bulk_fetch"`
	UserEmails map[string]string            `json:"user_emails"` // accountID -> email
}

// recordingClient wraps a jira.Client and records API responses
type recordingClient struct {
	inner    jira.Client
	mu       sync.Mutex
	payloads CapturedPayloads
}

func newRecordingClient(inner jira.Client) *recordingClient {
	return &recordingClient{
		inner: inner,
		payloads: CapturedPayloads{
			IssueKeys:  make(map[string][]string),
			UserEmails: make(map[string]string),
		},
	}
}

// saveToFile writes all captured payloads to a JSON file in the given directory
func (r *recordingClient) saveToFile(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r.payloads, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal payloads: %w", err)
	}

	path := filepath.Join(dir, "jira_responses.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	slog.Info("Saved captured Jira responses", slog.String("component", "jira"), slog.String("path", path))
	return nil
}

// --- Recorded methods (the 3 used during import) ---

func (r *recordingClient) GetAllIssueKeys(ctx context.Context, jql string) ([]string, error) {
	keys, err := r.inner.GetAllIssueKeys(ctx, jql)
	if err == nil {
		r.mu.Lock()
		r.payloads.IssueKeys[jql] = keys
		r.mu.Unlock()
	}
	return keys, err
}

func (r *recordingClient) BulkFetchIssues(ctx context.Context, req jira.BulkFetchRequest) (*jira.BulkFetchResponse, error) {
	resp, err := r.inner.BulkFetchIssues(ctx, req)
	if err == nil && resp != nil {
		r.mu.Lock()
		r.payloads.BulkFetch = append(r.payloads.BulkFetch, *resp)
		r.mu.Unlock()
	}
	return resp, err
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

func (r *recordingClient) TestConnection(ctx context.Context) (*jira.JiraInstanceInfo, error) {
	return r.inner.TestConnection(ctx)
}

func (r *recordingClient) ListProjects(ctx context.Context) ([]jira.JiraProject, error) {
	return r.inner.ListProjects(ctx)
}

func (r *recordingClient) GetProject(ctx context.Context, projectKey string) (*jira.JiraProject, error) {
	return r.inner.GetProject(ctx, projectKey)
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

func (r *recordingClient) GetProjectIssueTypeStatuses(ctx context.Context, projectKey string) ([]jira.JiraIssueTypeWithStatuses, error) {
	return r.inner.GetProjectIssueTypeStatuses(ctx, projectKey)
}

func (r *recordingClient) SearchIssues(ctx context.Context, opts jira.SearchOptions) (*jira.SearchResult, error) {
	return r.inner.SearchIssues(ctx, opts)
}

func (r *recordingClient) GetIssue(ctx context.Context, issueKey string, expand []string) (*jira.JiraIssue, error) {
	return r.inner.GetIssue(ctx, issueKey, expand)
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
	return r.inner.ListBoards(ctx, projectKey)
}

func (r *recordingClient) GetBoardSprints(ctx context.Context, boardID int) (*jira.SprintListResult, error) {
	return r.inner.GetBoardSprints(ctx, boardID)
}

func (r *recordingClient) DownloadAttachment(ctx context.Context, attachmentURL string) (io.ReadCloser, string, error) {
	return r.inner.DownloadAttachment(ctx, attachmentURL)
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
