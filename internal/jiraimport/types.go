// Package jiraimport owns the Jira import application's durable job lifecycle,
// provenance, and cleanup rules. HTTP handlers translate requests and responses;
// this package decides what import state means and performs its persistence.
package jiraimport

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrJobNotFound = errors.New("jira import job not found")
	ErrJobActive   = errors.New("jira import job is queued or running")
)

type WorkspaceCountMismatchError struct {
	Confirmed int
	Current   int
}

func (e *WorkspaceCountMismatchError) Error() string {
	return fmt.Sprintf("workspace confirmation count mismatch: confirmed %d, current %d", e.Confirmed, e.Current)
}

// CleanupFailure identifies one imported entity that could not be removed.
type CleanupFailure struct {
	EntityType  string `json:"entity_type"`
	WindshiftID int    `json:"windshift_id"`
	Error       string `json:"error"`
}

// CleanupError reports a partial cleanup. The job mappings remain available so
// the cleanup can be retried after the underlying problem is corrected.
type CleanupError struct {
	Failures []CleanupFailure
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("jira import cleanup failed for %d entity mapping(s)", len(e.Failures))
}

type JobStatus struct {
	JobID        string         `json:"job_id"`
	Status       string         `json:"status"`
	Phase        string         `json:"phase,omitempty"`
	Progress     map[string]any `json:"progress,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
}

type Progress struct {
	Phase               string `json:"phase"`
	CurrentProject      string `json:"current_project,omitempty"`
	TotalProjects       int    `json:"total_projects"`
	CompletedProjects   int    `json:"completed_projects"`
	FailedProjects      int    `json:"failed_projects"`
	TotalIssues         int    `json:"total_issues"`
	ImportedIssues      int    `json:"imported_issues"`
	FailedIssues        int    `json:"failed_issues"`
	TotalTests          int    `json:"total_tests"`
	ImportedTests       int    `json:"imported_tests"`
	FailedTests         int    `json:"failed_tests"`
	FailedLinks         int    `json:"failed_links"`
	TotalAttachments    int    `json:"total_attachments"`
	ImportedAttachments int    `json:"imported_attachments"`
	TotalComments       int    `json:"total_comments"`
	ImportedComments    int    `json:"imported_comments"`
	TotalWorklogs       int    `json:"total_worklogs"`
	ImportedWorklogs    int    `json:"imported_worklogs"`
}

type ImportedWorkspace struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type JobInfo struct {
	ID                     string              `json:"id"`
	ConnectionID           string              `json:"connection_id"`
	InstanceURL            string              `json:"instance_url,omitempty"`
	InstanceName           string              `json:"instance_name,omitempty"`
	Status                 string              `json:"status"`
	Phase                  string              `json:"phase,omitempty"`
	Scope                  string              `json:"scope"`
	ProjectKeys            []string            `json:"project_keys,omitempty"`
	ImportedWorkspaces     []ImportedWorkspace `json:"imported_workspaces,omitempty"`
	ImportedWorkspaceCount int                 `json:"imported_workspace_count"`
	ImportedItemCount      int                 `json:"imported_item_count"`
	Progress               map[string]any      `json:"progress,omitempty"`
	Result                 map[string]any      `json:"result,omitempty"`
	ErrorMessage           string              `json:"error_message,omitempty"`
	CreatedAt              time.Time           `json:"created_at"`
	StartedAt              *time.Time          `json:"started_at,omitempty"`
	CompletedAt            *time.Time          `json:"completed_at,omitempty"`
}

type Conflict struct {
	JobID                   string     `json:"job_id"`
	Status                  string     `json:"status"`
	ProjectKeys             []string   `json:"project_keys"`
	PreviousPlanFingerprint string     `json:"previous_plan_fingerprint,omitempty"`
	ConfigurationDrift      bool       `json:"configuration_drift"`
	CreatedAt               time.Time  `json:"created_at"`
	CompletedAt             *time.Time `json:"completed_at,omitempty"`
}

type PreviousImport struct {
	JobID          string     `json:"job_id"`
	ConnectionID   string     `json:"connection_id"`
	Status         string     `json:"status"`
	ProjectKeys    []string   `json:"project_keys"`
	WorkspaceCount int        `json:"workspace_count"`
	ItemCount      int        `json:"item_count"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type CreateJobInput struct {
	ConnectionID string
	ConfigJSON   []byte
	CreatedBy    *int
}
