package jiraimport

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrConnectionNotFound = errors.New("jira import connection not found")
	ErrConnectionActive   = errors.New("jira import connection has an active import")
)

type Connection struct {
	ID             string     `json:"id"`
	InstanceURL    string     `json:"instance_url"`
	Email          string     `json:"email"`
	InstanceName   string     `json:"instance_name"`
	DeploymentType string     `json:"deployment_type"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

type NewConnection struct {
	ID                   string
	InstanceURL          string
	Email                string
	EncryptedCredentials string
	InstanceName         string
	DeploymentType       string
	CreatedBy            *int
}

type ConnectionCredentials struct {
	InstanceURL          string
	Email                string
	EncryptedCredentials string
	DeploymentType       string
}

// ConnectionUpdate carries the mutable fields of an existing connection. The
// instance URL and deployment type are identity and cannot change.
type ConnectionUpdate struct {
	Email                string
	InstanceName         string
	EncryptedCredentials string
}

func (s *Service) CreateConnection(input NewConnection) error {
	_, err := s.db.ExecWrite(`
		INSERT INTO jira_import_connections
			(id, instance_url, email, encrypted_credentials, instance_name, deployment_type, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, input.ID, input.InstanceURL, input.Email, input.EncryptedCredentials, input.InstanceName, input.DeploymentType, input.CreatedBy)
	return err
}

func (s *Service) ListConnections() ([]Connection, error) {
	rows, err := s.db.Query(`
		SELECT id, instance_url, email, instance_name, deployment_type, created_at, last_used_at
		FROM jira_import_connections WHERE deleted_at IS NULL ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	connections := make([]Connection, 0)
	for rows.Next() {
		var connection Connection
		var instanceName, deploymentType sql.NullString
		var lastUsedAt sql.NullTime
		if err := rows.Scan(&connection.ID, &connection.InstanceURL, &connection.Email,
			&instanceName, &deploymentType, &connection.CreatedAt, &lastUsedAt); err != nil {
			return nil, err
		}
		connection.InstanceName = instanceName.String
		connection.DeploymentType = deploymentType.String
		if connection.DeploymentType == "" {
			connection.DeploymentType = "cloud"
		}
		if lastUsedAt.Valid {
			connection.LastUsedAt = &lastUsedAt.Time
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

// UpdateConnection replaces the credentials and account details of an active
// connection. It is the supported way to recover from an expired or revoked
// token without deleting the connection and its import provenance.
func (s *Service) UpdateConnection(connectionID string, update ConnectionUpdate) error {
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_connections
		SET email = ?, instance_name = ?, encrypted_credentials = ?
		WHERE id = ? AND deleted_at IS NULL
	`, update.Email, update.InstanceName, update.EncryptedCredentials, connectionID)
	if err != nil {
		return err
	}
	return requireActiveConnection(result)
}

// DeleteConnection removes the stored credentials and hides the connection
// from every active-connection read, while retaining the row so import jobs
// and their ID mappings keep valid provenance.
func (s *Service) DeleteConnection(connectionID string) error {
	active, err := s.HasActiveImport(connectionID)
	if err != nil {
		return err
	}
	if active {
		return ErrConnectionActive
	}
	result, err := s.db.ExecWrite(`
		UPDATE jira_import_connections
		SET deleted_at = CURRENT_TIMESTAMP, encrypted_credentials = ''
		WHERE id = ? AND deleted_at IS NULL
	`, connectionID)
	if err != nil {
		return err
	}
	return requireActiveConnection(result)
}

func requireActiveConnection(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// HasActiveImport reports whether a queued or running import still depends on
// the connection's credentials.
func (s *Service) HasActiveImport(connectionID string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM jira_import_jobs
		WHERE connection_id = ? AND status IN ('queued', 'running')
	`, connectionID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// LoadConnectionCredentials loads the stored credentials for an active
// connection without marking it as used.
func (s *Service) LoadConnectionCredentials(connectionID string) (ConnectionCredentials, error) {
	var result ConnectionCredentials
	var deploymentType sql.NullString
	err := s.db.QueryRow(`
		SELECT instance_url, email, encrypted_credentials, deployment_type
		FROM jira_import_connections WHERE id = ? AND deleted_at IS NULL
	`, connectionID).Scan(&result.InstanceURL, &result.Email, &result.EncryptedCredentials, &deploymentType)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectionCredentials{}, ErrConnectionNotFound
	}
	if err != nil {
		return ConnectionCredentials{}, err
	}
	result.DeploymentType = deploymentType.String
	return result, nil
}

func (s *Service) UseConnection(connectionID string) (ConnectionCredentials, error) {
	result, err := s.LoadConnectionCredentials(connectionID)
	if err != nil {
		return ConnectionCredentials{}, err
	}
	if _, err := s.db.ExecWrite(`
		UPDATE jira_import_connections SET last_used_at = CURRENT_TIMESTAMP
		WHERE id = ? AND deleted_at IS NULL
	`, connectionID); err != nil {
		return ConnectionCredentials{}, err
	}
	return result, nil
}

func (s *Service) ConnectionDeploymentType(connectionID string) (string, error) {
	var deploymentType sql.NullString
	err := s.db.QueryRow(`SELECT deployment_type FROM jira_import_connections WHERE id = ? AND deleted_at IS NULL`, connectionID).Scan(&deploymentType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConnectionNotFound
	}
	if err != nil {
		return "", err
	}
	return deploymentType.String, nil
}

func (s *Service) WorkspaceKeys() (map[string]struct{}, error) {
	workspaces, err := s.workspaces.ListIDKeys()
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{})
	for _, workspace := range workspaces {
		keys[normalizeProjectKey(workspace.Key)] = struct{}{}
	}
	return keys, nil
}

type WorkspaceKeyPlan struct {
	Key       string
	Collision bool
}

func (s *Service) PlanWorkspaceKeys(projectKeys []string) (map[string]WorkspaceKeyPlan, error) {
	occupied, err := s.WorkspaceKeys()
	if err != nil {
		return nil, fmt.Errorf("load existing workspace keys: %w", err)
	}
	plans := make(map[string]WorkspaceKeyPlan, len(projectKeys))
	for _, projectKey := range projectKeys {
		jiraKey := normalizeProjectKey(projectKey)
		if jiraKey == "" {
			continue
		}
		if _, exists := plans[jiraKey]; exists {
			continue
		}
		targetKey := jiraKey
		_, collision := occupied[targetKey]
		if collision {
			base := "JIRA_" + jiraKey
			targetKey = base
			for suffix := 2; ; suffix++ {
				if _, exists := occupied[targetKey]; !exists {
					break
				}
				targetKey = fmt.Sprintf("%s_%d", base, suffix)
			}
		}
		plans[jiraKey] = WorkspaceKeyPlan{Key: targetKey, Collision: collision}
		occupied[strings.ToUpper(targetKey)] = struct{}{}
	}
	return plans, nil
}
