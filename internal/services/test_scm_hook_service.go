package services

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// TestSCMInjectRefParams contains the inputs for injecting a synthetic SCM ref event.
type TestSCMInjectRefParams struct {
	WorkspaceRepositoryID int
	RefType               string
	RefName               string
	RefShort              string
	SHA                   string
	PrevName              string
	RepoFullName          string
}

// TestSetupMockRepoResult contains IDs created or reused for an e2e mock SCM repository.
type TestSetupMockRepoResult struct {
	ProviderID               int
	WorkspaceSCMConnectionID int
	WorkspaceRepositoryID    int
	RepositoryName           string
}

// TestSCMHookService owns DB-backed operations for e2e-only SCM hook handlers.
type TestSCMHookService struct {
	db            database.Database
	actionService *ActionService
}

// NewTestSCMHookService creates a service for e2e-only SCM hook endpoints.
func NewTestSCMHookService(db database.Database, actionService *ActionService) *TestSCMHookService {
	return &TestSCMHookService{db: db, actionService: actionService}
}

// InjectRef resolves the repository workspace and emits the synthetic SCM action event.
func (s *TestSCMHookService) InjectRef(params TestSCMInjectRefParams) (models.ActionTriggerType, error) {
	// Resolve workspace + repo_name for the payload (the action engine's
	// ref.short/repo.full_name substitution reads from this).
	var workspaceID int
	var repoName sql.NullString
	if err := s.db.QueryRow(`
		SELECT wsc.workspace_id, wr.repository_name
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, params.WorkspaceRepositoryID).Scan(&workspaceID, &repoName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", repository.ErrNotFound
		}
		return "", err
	}

	short := params.RefShort
	if short == "" {
		short = defaultTestSCMRefShort(params.RefType, params.RefName)
	}
	sha := params.SHA
	if sha == "" {
		sha = "deadbeef"
	}
	fullName := params.RepoFullName
	if fullName == "" {
		fullName = repoName.String
	}

	eventType := models.ActionTriggerSCMTagCreated
	if params.RefType == "branch" {
		eventType = models.ActionTriggerSCMReleaseBranchCreated
	}

	if s.actionService == nil {
		return "", fmt.Errorf("action service is not configured")
	}

	s.actionService.EmitActionEvent(&models.ActionEvent{
		EventType:   eventType,
		WorkspaceID: workspaceID,
		NewValues: map[string]any{
			"ref.name":                     params.RefName,
			"ref.short":                    short,
			"ref.sha":                      sha,
			"ref.type":                     params.RefType,
			"ref.prev_name":                params.PrevName,
			"repo.full_name":               fullName,
			"repo.workspace_repository_id": params.WorkspaceRepositoryID,
		},
	})

	return eventType, nil
}

// SetupMockRepo creates or reuses the minimal SCM rows needed by e2e tests.
func (s *TestSCMHookService) SetupMockRepo(workspaceID int, repoName string) (*TestSetupMockRepoResult, error) {
	if repoName == "" {
		repoName = "octo/demo"
	}

	// Provider — keyed by slug "test-mock". One row per server lifetime.
	var providerID int
	if err := s.db.QueryRow(`SELECT id FROM scm_providers WHERE slug = ?`, "test-mock").Scan(&providerID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err := s.db.QueryRowWrite(`
			INSERT INTO scm_providers(slug, name, provider_type, auth_method, enabled)
			VALUES ('test-mock', 'Test Mock SCM', 'github', 'pat', true)
			RETURNING id
		`).Scan(&providerID); err != nil {
			return nil, err
		}
	}

	// Connection — one per (workspace, provider).
	var connID int
	if err := s.db.QueryRow(
		`SELECT id FROM workspace_scm_connections WHERE workspace_id = ? AND scm_provider_id = ?`,
		workspaceID, providerID,
	).Scan(&connID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err := s.db.QueryRowWrite(`
			INSERT INTO workspace_scm_connections(workspace_id, scm_provider_id, enabled)
			VALUES (?, ?, true)
			RETURNING id
		`, workspaceID, providerID).Scan(&connID); err != nil {
			return nil, err
		}
	}

	// Repository — keyed by (connection, repository_name).
	var repoID int
	if err := s.db.QueryRow(
		`SELECT id FROM workspace_repositories WHERE workspace_scm_connection_id = ? AND repository_name = ?`,
		connID, repoName,
	).Scan(&repoID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err := s.db.QueryRowWrite(`
			INSERT INTO workspace_repositories(
				workspace_scm_connection_id, repository_external_id,
				repository_name, repository_url, default_branch, is_active
			) VALUES (?, ?, ?, ?, 'main', true)
			RETURNING id
		`, connID, "ext-"+repoName, repoName, "https://example.invalid/"+repoName).Scan(&repoID); err != nil {
			return nil, err
		}
	}

	return &TestSetupMockRepoResult{
		ProviderID:               providerID,
		WorkspaceSCMConnectionID: connID,
		WorkspaceRepositoryID:    repoID,
		RepositoryName:           repoName,
	}, nil
}

// defaultTestSCMRefShort mirrors the sync layer's ref-short rules so a test
// that passes ref_name without ref_short still gets the right value.
func defaultTestSCMRefShort(refType, refName string) string {
	switch refType {
	case "tag":
		if len(refName) > 1 && (refName[0] == 'v' || refName[0] == 'V') && refName[1] >= '0' && refName[1] <= '9' {
			return refName[1:]
		}
		return refName
	case "branch":
		const prefix = "release/"
		if len(refName) > len(prefix) && refName[:len(prefix)] == prefix {
			return refName[len(prefix):]
		}
		return refName
	default:
		return refName
	}
}
