package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type ZammadRepository struct {
	db database.Database
}

type ZammadTicketLinkReservationSnapshot struct {
	BaseURL             string
	CorrelationField    string
	ConnectionEnabled   bool
	DefaultGroupID      int
	DefaultGroupName    string
	AllowedGroups       []models.ZammadGroupRef
	DefaultCustomer     string
	WorkspaceID         int
	WorkspaceItemNumber int
	WorkspaceKey        string
	WorkspaceAllowed    bool
}

type ZammadTicketLinkScope struct {
	ItemID        int
	WorkspaceID   int
	GroupID       int
	GroupName     string
	SetupComplete bool
	SyncLocked    bool
}

type ZammadConnectionMutationSnapshot struct {
	BaseURL            string
	CorrelationField   string
	DefaultGroupID     int
	DefaultGroupName   string
	AllowedGroups      []models.ZammadGroupRef
	ClosedStateIDs     []int
	CompletionStatusID *int
}

func NewZammadRepository(db database.Database) *ZammadRepository {
	return &ZammadRepository{db: db}
}

const zammadConnectionColumns = `
	zc.provider_id, ip.slug, ip.name, ip.enabled, zc.base_url,
	zc.credential_id, zc.auth_method, zc.oauth_generation, zc.config_revision, COALESCE(zc.oauth_attempt_id, ''), ip.oauth_client_id, ip.oauth_client_secret_encrypted,
	EXISTS(SELECT 1 FROM zammad_oauth_tokens zot WHERE zot.provider_id = zc.provider_id AND zot.reauthorization_required = false),
	COALESCE((SELECT reauthorization_required FROM zammad_oauth_tokens zot WHERE zot.provider_id = zc.provider_id), false),
	zc.default_group_id, zc.default_group_name,
	zc.allowed_groups, zc.default_customer, zc.correlation_field, zc.closed_state_ids,
	zc.completion_status_id, (SELECT applies_to_all_workspaces FROM action_credentials ac WHERE ac.id = zc.credential_id),
	zc.last_tested_at, zc.last_test_error, zc.created_by,
	zc.created_at, zc.updated_at`

func (r *ZammadRepository) ListConnections() ([]*models.ZammadConnection, error) {
	rows, err := r.db.Query(`SELECT ` + zammadConnectionColumns + `
		FROM zammad_connections zc
		JOIN integration_providers ip ON ip.id = zc.provider_id
		ORDER BY ip.name`)
	if err != nil {
		return nil, fmt.Errorf("list Zammad connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return r.scanConnections(rows)
}

func (r *ZammadRepository) ListConnectionsForWorkspace(workspaceID int) ([]*models.ZammadConnection, error) {
	rows, err := r.db.Query(`SELECT `+zammadConnectionColumns+`
		FROM zammad_connections zc
		JOIN integration_providers ip ON ip.id = zc.provider_id
		WHERE ip.enabled = true AND (
			EXISTS (SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = true) OR EXISTS (
				SELECT 1 FROM action_credential_workspaces acw
				WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = ?
			)
		)
		ORDER BY ip.name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace Zammad connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return r.scanConnections(rows)
}

func (r *ZammadRepository) GetConnection(id string) (*models.ZammadConnection, error) {
	row := r.db.QueryRow(`SELECT `+zammadConnectionColumns+`
		FROM zammad_connections zc
		JOIN integration_providers ip ON ip.id = zc.provider_id
		WHERE zc.provider_id = ?`, id)
	connection, err := scanZammadConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get Zammad connection: %w", err)
	}
	connection.WorkspaceIDs, err = r.connectionWorkspaceIDs(id)
	if err != nil {
		return nil, err
	}
	return connection, nil
}

func (r *ZammadRepository) IsConnectionAvailableToWorkspace(id string, workspaceID int) (bool, error) {
	var available bool
	err := r.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM zammad_connections zc
		JOIN integration_providers ip ON ip.id = zc.provider_id
		WHERE zc.provider_id = ? AND ip.enabled = true AND (
			EXISTS (SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = true) OR EXISTS (
				SELECT 1 FROM action_credential_workspaces acw
				WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = ?
			)
		))`, id, workspaceID).Scan(&available)
	return available, err
}

// IsConnectionScopedToWorkspace checks only the configured workspace scope.
// Cleanup paths use it so an administrator can disable a connection without
// making its existing links impossible to remove.
func (r *ZammadRepository) IsConnectionScopedToWorkspace(id string, workspaceID int) (bool, error) {
	var scoped bool
	err := r.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM zammad_connections zc
		WHERE zc.provider_id = ? AND (
			EXISTS (SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = true) OR EXISTS (
				SELECT 1 FROM action_credential_workspaces acw
				WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = ?
			)
		))`, id, workspaceID).Scan(&scoped)
	return scoped, err
}

func (r *ZammadRepository) CreateConnection(connection *models.ZammadConnection) error {
	closedJSON, err := json.Marshal(connection.ClosedStateIDs)
	if err != nil {
		return err
	}
	allowedGroupsJSON, err := json.Marshal(connection.AllowedGroups)
	if err != nil {
		return err
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		if _, err := tx.Exec(`INSERT INTO integration_providers
			(id, slug, name, provider_type, enabled, oauth_client_id, oauth_client_secret_encrypted, provider_config)
			VALUES (?, ?, ?, ?, ?, ?, ?, '{}')`, connection.ProviderID, connection.Slug,
			connection.Name, models.IntegrationProviderZammad, connection.Enabled,
			nullableString(connection.OAuthClientID), nullableString(connection.OAuthClientSecretEncrypted)); err != nil {
			if database.IsUniqueConstraintError(err) {
				return ErrDuplicateEntry
			}
			return err
		}
		if _, err := tx.Exec(`INSERT INTO zammad_connections
			(provider_id, credential_id, auth_method, base_url, default_group_id,
			 default_group_name, allowed_groups, default_customer, correlation_field,
			 closed_state_ids, completion_status_id,
			 created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, connection.ProviderID,
			nullablePositiveInt(connection.CredentialID), connection.AuthMethod, connection.BaseURL, nullablePositiveInt(connection.DefaultGroupID),
			connection.DefaultGroupName, string(allowedGroupsJSON), connection.DefaultCustomer, connection.CorrelationField,
			string(closedJSON), connection.CompletionStatusID, connection.CreatedBy); err != nil {
			return err
		}
		return nil
	})
}

func (r *ZammadRepository) UpdateConnection(connection *models.ZammadConnection) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		return r.UpdateConnectionTx(tx, connection)
	})
}

func (r *ZammadRepository) UpdateConnectionTx(tx database.Tx, connection *models.ZammadConnection) error {
	closedJSON, err := json.Marshal(connection.ClosedStateIDs)
	if err != nil {
		return err
	}
	allowedGroupsJSON, err := json.Marshal(connection.AllowedGroups)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE zammad_connections SET
			credential_id = ?, auth_method = ?, base_url = ?, default_group_id = ?, default_group_name = ?, allowed_groups = ?,
			default_customer = ?, correlation_field = ?, closed_state_ids = ?,
			completion_status_id = ?, config_revision = config_revision + 1,
			updated_at = CURRENT_TIMESTAMP
			WHERE provider_id = ? AND config_revision = ?`, nullablePositiveInt(connection.CredentialID), connection.AuthMethod, connection.BaseURL, nullablePositiveInt(connection.DefaultGroupID),
		connection.DefaultGroupName, string(allowedGroupsJSON), connection.DefaultCustomer, connection.CorrelationField,
		string(closedJSON), connection.CompletionStatusID, connection.ProviderID, connection.ConfigRevision)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrConcurrentUpdate
	}
	connection.ConfigRevision++
	result, err = tx.Exec(`UPDATE integration_providers
			SET slug = ?, name = ?, enabled = ?, oauth_client_id = ?, oauth_client_secret_encrypted = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND provider_type = ?`, connection.Slug, connection.Name,
		connection.Enabled, nullableString(connection.OAuthClientID), nullableString(connection.OAuthClientSecretEncrypted), connection.ProviderID, models.IntegrationProviderZammad)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ZammadRepository) DeleteConnectionTx(tx database.Tx, id string) error {
	var credentialID sql.NullInt64
	if err := tx.QueryRow("SELECT credential_id FROM zammad_connections WHERE provider_id = ?", id).Scan(&credentialID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec("DELETE FROM integration_providers WHERE id = ?", id); err != nil {
		return err
	}
	if credentialID.Valid {
		_, err := tx.Exec("DELETE FROM action_credentials WHERE id = ?", credentialID.Int64)
		return err
	}
	return nil
}

func (r *ZammadRepository) HasTicketLinksForConnectionTx(tx database.Tx, id string) (bool, error) {
	var hasLinks bool
	err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM zammad_ticket_links WHERE provider_id = ?)", id).Scan(&hasLinks)
	return hasLinks, err
}

func (r *ZammadRepository) HasTicketLinksForItemsTx(tx database.Tx, itemIDs []int) (bool, error) {
	if len(itemIDs) == 0 {
		return false, nil
	}
	placeholders, args := inPlaceholders(itemIDs)
	var hasLinks bool
	err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM zammad_ticket_links WHERE item_id IN ("+placeholders+"))", args...).Scan(&hasLinks)
	if err != nil {
		return false, fmt.Errorf("check Zammad ticket links for items: %w", err)
	}
	return hasLinks, nil
}

func (r *ZammadRepository) HasTicketLinksForWorkspaceTx(tx database.Tx, workspaceID int) (bool, error) {
	var hasLinks bool
	err := tx.QueryRow(`SELECT EXISTS(
		SELECT 1
		FROM zammad_ticket_links ztl
		JOIN items i ON i.id = ztl.item_id
		WHERE i.workspace_id = ?
	)`, workspaceID).Scan(&hasLinks)
	if err != nil {
		return false, fmt.Errorf("check Zammad ticket links for workspace: %w", err)
	}
	return hasLinks, nil
}

// LockConnectionTx is the first step of every connection mutation and ticket
// reservation. PostgreSQL uses an explicit row lock; SQLite write transactions
// are already started with _txlock=immediate.
func (r *ZammadRepository) LockConnectionTx(tx database.Tx, id string) error {
	query := "SELECT provider_id FROM zammad_connections WHERE provider_id = ?"
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	var providerID string
	if err := tx.QueryRow(query, id).Scan(&providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// lockTicketLinkConnectionTx reads a ticket link's provider before acquiring
// the provider's connection lock. The preliminary read intentionally carries
// no row lock, so every actual lock acquisition remains connection first and
// no ticket-link row can be locked ahead of its connection on PostgreSQL.
func (r *ZammadRepository) lockTicketLinkConnectionTx(tx database.Tx, linkID string) error {
	var providerID string
	if err := tx.QueryRow("SELECT provider_id FROM zammad_ticket_links WHERE id = ?", linkID).Scan(&providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := r.LockConnectionTx(tx, providerID); err != nil {
		return err
	}
	return nil
}

func (r *ZammadRepository) ConnectionMutationSnapshotTx(tx database.Tx, id string) (*ZammadConnectionMutationSnapshot, error) {
	snapshot := &ZammadConnectionMutationSnapshot{}
	var allowedGroupsJSON, closedStateIDsJSON string
	var completionStatusID sql.NullInt64
	if err := tx.QueryRow(`SELECT base_url, correlation_field, COALESCE(default_group_id, 0),
		default_group_name, allowed_groups, closed_state_ids, completion_status_id
		FROM zammad_connections WHERE provider_id = ?`, id).Scan(
		&snapshot.BaseURL, &snapshot.CorrelationField, &snapshot.DefaultGroupID,
		&snapshot.DefaultGroupName, &allowedGroupsJSON, &closedStateIDsJSON, &completionStatusID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedGroupsJSON), &snapshot.AllowedGroups); err != nil {
		return nil, fmt.Errorf("decode Zammad allowed groups: %w", err)
	}
	if err := json.Unmarshal([]byte(closedStateIDsJSON), &snapshot.ClosedStateIDs); err != nil {
		return nil, fmt.Errorf("decode Zammad closed state IDs: %w", err)
	}
	if completionStatusID.Valid {
		value := int(completionStatusID.Int64)
		snapshot.CompletionStatusID = &value
	}
	return snapshot, nil
}

func (r *ZammadRepository) ConnectionGroupPolicyTx(tx database.Tx, id string) (defaultGroupID int, defaultGroupName string, allowedGroups []models.ZammadGroupRef, queryErr error) {
	var allowedGroupsJSON string
	if err := tx.QueryRow(`SELECT COALESCE(default_group_id, 0), default_group_name, allowed_groups
		FROM zammad_connections WHERE provider_id = ?`, id).Scan(&defaultGroupID, &defaultGroupName, &allowedGroupsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil, ErrNotFound
		}
		return 0, "", nil, err
	}
	if err := json.Unmarshal([]byte(allowedGroupsJSON), &allowedGroups); err != nil {
		return 0, "", nil, fmt.Errorf("decode Zammad allowed groups: %w", err)
	}
	return defaultGroupID, defaultGroupName, allowedGroups, nil
}

func (r *ZammadRepository) LockTicketLinkReservationTx(tx database.Tx, providerID string, itemID int) (*ZammadTicketLinkReservationSnapshot, error) {
	connectionQuery := `SELECT zc.base_url, zc.correlation_field, ip.enabled,
		COALESCE(zc.default_group_id, 0), zc.default_group_name, zc.allowed_groups, zc.default_customer
		FROM zammad_connections zc
		JOIN integration_providers ip ON ip.id = zc.provider_id
		WHERE zc.provider_id = ?`
	if r.db.GetDriverName() == "postgres" {
		connectionQuery += " FOR UPDATE OF zc"
	}
	snapshot := &ZammadTicketLinkReservationSnapshot{}
	var allowedGroupsJSON string
	if err := tx.QueryRow(connectionQuery, providerID).Scan(
		&snapshot.BaseURL, &snapshot.CorrelationField, &snapshot.ConnectionEnabled,
		&snapshot.DefaultGroupID, &snapshot.DefaultGroupName, &allowedGroupsJSON, &snapshot.DefaultCustomer,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedGroupsJSON), &snapshot.AllowedGroups); err != nil {
		return nil, fmt.Errorf("decode Zammad allowed groups: %w", err)
	}
	itemQuery := `SELECT i.workspace_id, i.workspace_item_number, w.key
		FROM items i JOIN workspaces w ON w.id = i.workspace_id WHERE i.id = ?`
	if r.db.GetDriverName() == "postgres" {
		itemQuery += " FOR UPDATE OF i"
	}
	if err := tx.QueryRow(itemQuery, itemID).Scan(&snapshot.WorkspaceID, &snapshot.WorkspaceItemNumber, &snapshot.WorkspaceKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := tx.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM zammad_connections zc
		WHERE zc.provider_id = ? AND (
			EXISTS (SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = true) OR EXISTS (
				SELECT 1 FROM action_credential_workspaces acw
				WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = ?
			)
		))`, providerID, snapshot.WorkspaceID).Scan(&snapshot.WorkspaceAllowed); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *ZammadRepository) ListTicketLinkScopesForUpdateTx(tx database.Tx, providerID string) ([]ZammadTicketLinkScope, error) {
	query := `SELECT i.id, i.workspace_id, COALESCE(ztl.group_id, 0), COALESCE(ztl.group_name, ''),
		ztl.item_integration_link_id IS NOT NULL,
		ztl.sync_lock_until IS NOT NULL AND ztl.sync_lock_until > CURRENT_TIMESTAMP
		FROM zammad_ticket_links ztl
		JOIN items i ON i.id = ztl.item_id
		WHERE ztl.provider_id = ? ORDER BY i.id`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE OF i"
	}
	rows, err := tx.Query(query, providerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scopes := []ZammadTicketLinkScope{}
	for rows.Next() {
		var scope ZammadTicketLinkScope
		if err := rows.Scan(&scope.ItemID, &scope.WorkspaceID, &scope.GroupID, &scope.GroupName, &scope.SetupComplete, &scope.SyncLocked); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func (r *ZammadRepository) HasTicketLinkUnavailableInWorkspaceTx(tx database.Tx, itemID, workspaceID int) (bool, error) {
	var unavailable bool
	err := tx.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM zammad_ticket_links ztl
		JOIN zammad_connections zc ON zc.provider_id = ztl.provider_id
		WHERE ztl.item_id = ? AND EXISTS (
			SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = false
		)
		AND NOT EXISTS (
			SELECT 1 FROM action_credential_workspaces acw
			WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = ?
		))`, itemID, workspaceID).Scan(&unavailable)
	return unavailable, err
}

func (r *ZammadRepository) SetConnectionTestResult(id string, testedAt time.Time, testError string) error {
	_, err := r.db.ExecWrite(`UPDATE zammad_connections
		SET last_tested_at = ?, last_test_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE provider_id = ?`, testedAt, testError, id)
	return err
}

func (r *ZammadRepository) scanConnections(rows *sql.Rows) ([]*models.ZammadConnection, error) {
	connections := []*models.ZammadConnection{}
	for rows.Next() {
		connection, err := scanZammadConnection(rows)
		if err != nil {
			return nil, err
		}
		connection.WorkspaceIDs, err = r.connectionWorkspaceIDs(connection.ProviderID)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

func scanZammadConnection(scanner interface{ Scan(...any) error }) (*models.ZammadConnection, error) {
	var connection models.ZammadConnection
	var credentialID, groupID, completionStatusID, createdBy sql.NullInt64
	var groupName, lastError sql.NullString
	var authMethod, oauthClientID, oauthClientSecret sql.NullString
	var testedAt sql.NullTime
	var oauthConnected, reauthorizationRequired bool
	var allowedGroupsJSON, closedJSON string
	err := scanner.Scan(&connection.ProviderID, &connection.Slug, &connection.Name,
		&connection.Enabled, &connection.BaseURL, &credentialID, &authMethod, &connection.OAuthGeneration, &connection.ConfigRevision, &connection.OAuthAttemptID, &oauthClientID, &oauthClientSecret,
		&oauthConnected, &reauthorizationRequired, &groupID,
		&groupName, &allowedGroupsJSON, &connection.DefaultCustomer, &connection.CorrelationField,
		&closedJSON, &completionStatusID, &connection.AppliesToAllWorkspaces,
		&testedAt, &lastError, &createdBy, &connection.CreatedAt, &connection.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if groupID.Valid {
		connection.DefaultGroupID = int(groupID.Int64)
	}
	if credentialID.Valid {
		v := int(credentialID.Int64)
		connection.CredentialID = v
	}
	if authMethod.Valid {
		connection.AuthMethod = models.ZammadAuthMethod(authMethod.String)
	}
	if oauthClientID.Valid {
		connection.OAuthClientID = oauthClientID.String
	}
	connection.HasOAuthClientSecret = oauthClientSecret.Valid && oauthClientSecret.String != ""
	if oauthClientSecret.Valid {
		connection.OAuthClientSecretEncrypted = oauthClientSecret.String
	}
	connection.OAuthConnected = oauthConnected
	connection.ReauthorizationRequired = reauthorizationRequired
	if groupName.Valid {
		connection.DefaultGroupName = groupName.String
	}
	if completionStatusID.Valid {
		v := int(completionStatusID.Int64)
		connection.CompletionStatusID = &v
	}
	if testedAt.Valid {
		connection.LastTestedAt = &testedAt.Time
	}
	if lastError.Valid {
		connection.LastTestError = lastError.String
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		connection.CreatedBy = &v
	}
	if err := json.Unmarshal([]byte(closedJSON), &connection.ClosedStateIDs); err != nil {
		return nil, fmt.Errorf("decode closed_state_ids: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedGroupsJSON), &connection.AllowedGroups); err != nil {
		return nil, fmt.Errorf("decode allowed_groups: %w", err)
	}
	connection.HasAPIToken = connection.AuthMethod == models.ZammadAuthMethodAPIToken && connection.CredentialID > 0
	return &connection, nil
}

func (r *ZammadRepository) connectionWorkspaceIDs(providerID string) ([]int, error) {
	rows, err := r.db.Query(`SELECT acw.workspace_id
		FROM zammad_connections zc
		JOIN action_credential_workspaces acw ON acw.credential_id = zc.credential_id
		WHERE zc.provider_id = ? ORDER BY acw.workspace_id`, providerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *ZammadRepository) GetTicketLinksForItem(itemID int) ([]*models.ZammadTicketLink, error) {
	rows, err := r.db.Query(`SELECT `+zammadTicketLinkColumns+`
		FROM zammad_ticket_links ztl
		JOIN integration_providers ip ON ip.id = ztl.provider_id
		JOIN zammad_connections zc ON zc.provider_id = ztl.provider_id
		JOIN items i ON i.id = ztl.item_id
		WHERE ztl.item_id = ? AND (
			EXISTS (SELECT 1 FROM action_credentials ac WHERE ac.id = zc.credential_id AND ac.applies_to_all_workspaces = true) OR EXISTS (
				SELECT 1 FROM action_credential_workspaces acw
				WHERE acw.credential_id = zc.credential_id AND acw.workspace_id = i.workspace_id
			)
		) ORDER BY ztl.created_at DESC`, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	links := []*models.ZammadTicketLink{}
	for rows.Next() {
		link, err := scanZammadTicketLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

const zammadTicketLinkColumns = `
	ztl.id, ztl.item_id, ztl.provider_id, ip.name,
	ztl.item_integration_link_id, ztl.ticket_id, ztl.ticket_number,
	ztl.ticket_url, ztl.group_id, ztl.group_name, ztl.owner_id, ztl.owner_name, ztl.correlation_key,
	ztl.sync_state, ztl.last_status_id, ztl.last_status_name,
	ztl.last_synced_at, ztl.last_attempt_at, ztl.next_attempt_at, ztl.last_error, ztl.completion_applied, ztl.created_by,
	ztl.created_at, ztl.updated_at`

func (r *ZammadRepository) GetTicketLink(id string) (*models.ZammadTicketLink, error) {
	link, err := scanZammadTicketLink(r.db.QueryRow(`SELECT `+zammadTicketLinkColumns+`
		FROM zammad_ticket_links ztl
		JOIN integration_providers ip ON ip.id = ztl.provider_id
		WHERE ztl.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

func (r *ZammadRepository) GetTicketLinkForItem(itemID int, providerID string) (*models.ZammadTicketLink, error) {
	link, err := scanZammadTicketLink(r.db.QueryRow(`SELECT `+zammadTicketLinkColumns+`
		FROM zammad_ticket_links ztl
		JOIN integration_providers ip ON ip.id = ztl.provider_id
		WHERE ztl.item_id = ? AND ztl.provider_id = ?`, itemID, providerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// GetItemDestinationByCorrelationKey resolves the durable key and current
// workspace in one snapshot through the provider/key uniqueness boundary.
func (r *ZammadRepository) GetItemDestinationByCorrelationKey(providerID, correlationKey string) (itemID, workspaceID int, err error) {
	err = r.db.QueryRow(`SELECT ztl.item_id, i.workspace_id
		FROM zammad_ticket_links ztl
		JOIN items i ON i.id = ztl.item_id
		WHERE ztl.provider_id = ? AND ztl.correlation_key = ?`, providerID, correlationKey).Scan(&itemID, &workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return itemID, workspaceID, err
}

func (r *ZammadRepository) CreatePendingTicketLinkTx(tx database.Tx, link *models.ZammadTicketLink) error {
	_, err := tx.Exec(`INSERT INTO zammad_ticket_links
		(id, item_id, provider_id, group_id, group_name, correlation_key,
		 sync_state, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, link.ID, link.ItemID, link.ProviderID,
		nullablePositiveInt(link.GroupID), link.GroupName, link.CorrelationKey,
		models.ZammadSyncPending, link.CreatedBy)
	if database.IsUniqueConstraintError(err) {
		return ErrDuplicateEntry
	}
	return err
}

// ReserveExistingTicketLinkTx claims the provider/ticket pair before changing
// the remote correlation field. The unique constraint makes competing item
// links fail closed rather than attaching one ticket to two Windshift items.
func (r *ZammadRepository) ReserveExistingTicketLinkTx(tx database.Tx, link *models.ZammadTicketLink) error {
	_, err := tx.Exec(`INSERT INTO zammad_ticket_links
		(id, item_id, provider_id, ticket_id, ticket_number, ticket_url, group_id, group_name,
		 owner_id, owner_name, correlation_key, sync_state, last_status_id, last_status_name, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID, link.ItemID, link.ProviderID, link.TicketID, link.TicketNumber, link.TicketURL,
		nullablePositiveInt(link.GroupID), link.GroupName, nullablePositiveInt(link.OwnerID), link.OwnerName,
		link.CorrelationKey, models.ZammadSyncCreating, nullablePositiveInt(link.LastStatusID), link.LastStatusName, link.CreatedBy)
	if database.IsUniqueConstraintError(err) {
		return ErrDuplicateEntry
	}
	return err
}

// ClaimTicketCreation atomically claims a pending or retryable ticket
// creation. Newly reserved existing tickets use the creating state with no
// started timestamp and are eligible immediately; an older in-flight creation
// is eligible only once its creation marker is stale. The sync lease is set in
// the same update, so remote work has one owner from reservation to completion.
func (r *ZammadRepository) ClaimTicketCreation(linkID, syncOwner string, now, until time.Time) (claimed, wasUncertain bool, err error) {
	if syncOwner == "" {
		return false, false, ErrInvalidInput
	}
	staleBefore := now.Add(-2 * time.Minute)
	err = database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, linkID); err != nil {
			return err
		}
		stateQuery := "SELECT sync_state FROM zammad_ticket_links WHERE id = ?"
		if r.db.GetDriverName() == "postgres" {
			stateQuery += " FOR UPDATE"
		}
		var previousState string
		if err := tx.QueryRow(stateQuery, linkID).Scan(&previousState); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		result, err := tx.Exec(`UPDATE zammad_ticket_links
			SET sync_state = ?, creating_started_at = ?, sync_lock_until = ?, sync_lock_owner = ?,
				last_error = '', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
			  AND (sync_lock_until IS NULL OR sync_lock_until < CURRENT_TIMESTAMP)
			  AND (sync_state IN (?, ?, ?)
			       OR (sync_state = ? AND (creating_started_at IS NULL OR creating_started_at < ?)))`,
			models.ZammadSyncCreating, now, until, syncOwner, linkID,
			models.ZammadSyncPending, models.ZammadSyncFailed, models.ZammadSyncUncertain,
			models.ZammadSyncCreating, staleBefore)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		claimed = rows == 1
		// A stale creating state may have crashed while the POST response was in
		// flight. Treat it like an explicitly uncertain outcome so the next
		// worker searches by correlation only instead of risking a duplicate.
		wasUncertain = claimed && (previousState == string(models.ZammadSyncUncertain) || previousState == string(models.ZammadSyncCreating))
		return nil
	})
	return claimed, wasUncertain, err
}

func (r *ZammadRepository) CompleteTicketCreation(linkID, syncOwner string, ticketID int, number, ticketURL string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName string, linkedBy int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, linkID); err != nil {
			return err
		}
		return r.CompleteTicketCreationTx(tx, linkID, syncOwner, ticketID, number, ticketURL, statusID, statusName, groupID, groupName, ownerID, ownerName, linkedBy)
	})
}

// CompleteTicketCreationTx finalizes a created ticket inside the caller's
// connection-policy transaction.
func (r *ZammadRepository) CompleteTicketCreationTx(tx database.Tx, linkID, syncOwner string, ticketID int, number, ticketURL string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName string, linkedBy int) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	genericLinkID := linkID + "-external"
	metadata, _ := json.Marshal(map[string]any{
		"status_id": statusID, "status_name": statusName,
		"group_id": groupID, "group_name": groupName,
		"owner_id": ownerID, "owner_name": ownerName,
	})
	result, err := tx.Exec(`INSERT INTO item_integration_links
			(id, item_id, integration_provider_id, external_id, external_url,
			 title, icon, link_type, link_metadata, linked_by)
			SELECT ?, item_id, provider_id, ?, ?, ?, ?, 'ticket', ?, ?
			FROM zammad_ticket_links WHERE id = ? AND sync_lock_owner = ?
			ON CONFLICT (item_id, integration_provider_id, external_id) DO UPDATE SET
				external_url = excluded.external_url,
				title = excluded.title,
				link_metadata = excluded.link_metadata,
				updated_at = CURRENT_TIMESTAMP`,
		genericLinkID, strconv.Itoa(ticketID), ticketURL, "Zammad #"+number,
		"ticket", string(metadata), strconv.Itoa(linkedBy), linkID, syncOwner)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return ErrConcurrentUpdate
	}
	if err := tx.QueryRow(`SELECT iil.id FROM item_integration_links iil
			JOIN zammad_ticket_links ztl ON CAST(ztl.item_id AS TEXT) = iil.item_id
				AND ztl.provider_id = iil.integration_provider_id
		WHERE ztl.id = ? AND iil.external_id = ?`, linkID, strconv.Itoa(ticketID)).Scan(&genericLinkID); err != nil {
		return err
	}
	result, err = tx.Exec(`UPDATE zammad_ticket_links SET
			item_integration_link_id = ?, ticket_id = ?, ticket_number = ?,
			ticket_url = ?, group_id = ?, group_name = ?, owner_id = ?, owner_name = ?,
			sync_state = ?, creating_started_at = NULL,
			last_status_id = ?, last_status_name = ?, last_synced_at = CURRENT_TIMESTAMP,
			last_attempt_at = CURRENT_TIMESTAMP, next_attempt_at = NULL,
		last_error = '', sync_lock_until = NULL, sync_lock_owner = NULL,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND sync_lock_owner = ?`, genericLinkID, ticketID, number, ticketURL,
		nullablePositiveInt(groupID), groupName, nullablePositiveInt(ownerID), ownerName,
		models.ZammadSyncLinked, nullablePositiveInt(statusID), statusName, linkID, syncOwner)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrConcurrentUpdate
	}
	return nil
}

func (r *ZammadRepository) CompleteExistingTicketLink(linkID, syncOwner, genericLinkID string, ticket *models.ZammadTicketLink, linkedBy int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, linkID); err != nil {
			return err
		}
		return r.CompleteExistingTicketLinkTx(tx, linkID, syncOwner, genericLinkID, ticket, linkedBy)
	})
}

// CompleteExistingTicketLinkTx finalizes an existing ticket link inside the
// caller's connection-policy transaction.
func (r *ZammadRepository) CompleteExistingTicketLinkTx(tx database.Tx, linkID, syncOwner, genericLinkID string, ticket *models.ZammadTicketLink, linkedBy int) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	metadata, _ := json.Marshal(map[string]any{"status_id": ticket.LastStatusID, "status_name": ticket.LastStatusName, "group_id": ticket.GroupID, "group_name": ticket.GroupName, "owner_id": ticket.OwnerID, "owner_name": ticket.OwnerName})
	result, err := tx.Exec(`INSERT INTO item_integration_links
			(id, item_id, integration_provider_id, external_id, external_url, title, icon, link_type, link_metadata, linked_by)
			SELECT ?, item_id, provider_id, ?, ?, ?, ?, 'ticket', ?, ? FROM zammad_ticket_links WHERE id = ? AND sync_lock_owner = ?
			ON CONFLICT (item_id, integration_provider_id, external_id) DO UPDATE SET external_url=excluded.external_url, title=excluded.title, link_metadata=excluded.link_metadata, updated_at=CURRENT_TIMESTAMP`,
		genericLinkID, strconv.Itoa(ticket.TicketID), ticket.TicketURL, "Zammad #"+ticket.TicketNumber, "ticket", string(metadata), strconv.Itoa(linkedBy), linkID, syncOwner)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return ErrConcurrentUpdate
	}
	if err := tx.QueryRow(`SELECT iil.id FROM item_integration_links iil
			JOIN zammad_ticket_links ztl ON CAST(ztl.item_id AS TEXT) = iil.item_id
				AND ztl.provider_id = iil.integration_provider_id
			WHERE ztl.id = ? AND ztl.sync_lock_owner = ? AND iil.external_id = ?`,
		linkID, syncOwner, strconv.Itoa(ticket.TicketID)).Scan(&genericLinkID); err != nil {
		return err
	}
	result, err = tx.Exec(`UPDATE zammad_ticket_links SET item_integration_link_id=?, ticket_id=?, ticket_number=?, ticket_url=?, group_id=?, group_name=?, owner_id=?, owner_name=?, sync_state=?, creating_started_at=NULL, last_status_id=?, last_status_name=?, last_synced_at=CURRENT_TIMESTAMP, last_attempt_at=CURRENT_TIMESTAMP, next_attempt_at=NULL, last_error='', sync_lock_until=NULL, sync_lock_owner=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=? AND sync_lock_owner=?`,
		genericLinkID, ticket.TicketID, ticket.TicketNumber, ticket.TicketURL, nullablePositiveInt(ticket.GroupID), ticket.GroupName, nullablePositiveInt(ticket.OwnerID), ticket.OwnerName, models.ZammadSyncLinked, nullablePositiveInt(ticket.LastStatusID), ticket.LastStatusName, linkID, syncOwner)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrConcurrentUpdate
	}
	return nil
}

func (r *ZammadRepository) MarkTicketLinkFailed(id, syncOwner, safeError string) error {
	return r.markTicketLinkWithClaim(id, syncOwner, `sync_state = ?, creating_started_at = NULL, last_error = ?,
		sync_lock_until = NULL, sync_lock_owner = NULL`, models.ZammadSyncFailed, safeError)
}

func (r *ZammadRepository) MarkTicketLinkUncertain(id, syncOwner, safeError string) error {
	return r.markTicketLinkWithClaim(id, syncOwner, `sync_state = ?, creating_started_at = NULL, last_error = ?,
		sync_lock_until = NULL, sync_lock_owner = NULL`, models.ZammadSyncUncertain, safeError)
}

// MarkTicketLinkSetupError retains an existing-ticket reservation after an
// upstream correlation update failed. A retry can safely inspect the remote
// field and finish the same reservation without allowing background sync to
// treat it as a complete link.
func (r *ZammadRepository) MarkTicketLinkSetupError(id, syncOwner, safeError string) error {
	return r.markTicketLinkWithClaim(id, syncOwner, `last_error = ?,
		sync_lock_until = NULL, sync_lock_owner = NULL`, safeError)
}

func (r *ZammadRepository) markTicketLinkWithClaim(id, syncOwner, setClause string, args ...any) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		args = append(args, id, syncOwner)
		result, err := tx.Exec(`UPDATE zammad_ticket_links SET `+setClause+`, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND sync_lock_owner = ?`, args...)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrConcurrentUpdate
		}
		return nil
	})
}

func (r *ZammadRepository) ResetUncertainTicketCreation(id string) (bool, error) {
	result, err := r.db.ExecWrite(`UPDATE zammad_ticket_links SET
		sync_state = ?, last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND sync_state = ? AND ticket_id IS NULL`,
		models.ZammadSyncFailed, id, models.ZammadSyncUncertain)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows == 1, nil
}

func (r *ZammadRepository) UpdateTicketLinkSync(id, syncOwner string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName, safeError string, now time.Time, setCompletionApplied, completionApplied bool) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		return r.UpdateTicketLinkSyncTx(tx, id, syncOwner, statusID, statusName, groupID, groupName, ownerID, ownerName, safeError, now, setCompletionApplied, completionApplied)
	})
}

func (r *ZammadRepository) UpdateTicketLinkSyncTx(tx database.Tx, id, syncOwner string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName, safeError string, now time.Time, setCompletionApplied, completionApplied bool) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	state := models.ZammadSyncLinked
	var nextAttempt any
	if safeError != "" {
		state = models.ZammadSyncFailed
		nextAttempt = now.Add(time.Minute)
	}
	result, err := tx.ExecWrite(`UPDATE zammad_ticket_links SET
		last_status_id = ?, last_status_name = ?,
		group_id = ?, group_name = ?, owner_id = ?, owner_name = ?,
		last_synced_at = CASE WHEN ? = '' THEN ? ELSE last_synced_at END,
		last_attempt_at = ?, next_attempt_at = ?,
		last_error = ?, sync_state = ?, sync_lock_until = NULL, sync_lock_owner = NULL,
		completion_applied = CASE WHEN ? THEN ? ELSE completion_applied END,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND item_integration_link_id IS NOT NULL AND sync_lock_owner = ?`, nullablePositiveInt(statusID), statusName,
		nullablePositiveInt(groupID), groupName, nullablePositiveInt(ownerID), ownerName,
		safeError, now, now, nextAttempt, safeError, state, setCompletionApplied, completionApplied, id, syncOwner)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrConcurrentUpdate
	}
	return nil
}

func (r *ZammadRepository) ListDueTicketLinks(before time.Time, limit int) ([]*models.ZammadTicketLink, error) {
	rows, err := r.db.Query(`SELECT `+zammadTicketLinkColumns+`
		FROM zammad_ticket_links ztl
		JOIN integration_providers ip ON ip.id = ztl.provider_id
		JOIN zammad_connections zc ON zc.provider_id = ztl.provider_id
		WHERE ip.enabled = true AND ztl.ticket_id IS NOT NULL
		  AND ztl.item_integration_link_id IS NOT NULL
		  AND (zc.auth_method != 'oauth' OR EXISTS (
			SELECT 1 FROM zammad_oauth_tokens zot
			WHERE zot.provider_id = zc.provider_id AND zot.reauthorization_required = false
		  ))
		  AND (ztl.last_synced_at IS NULL OR ztl.last_synced_at < ?)
		  AND (ztl.next_attempt_at IS NULL OR ztl.next_attempt_at <= CURRENT_TIMESTAMP)
		  AND (ztl.sync_lock_until IS NULL OR ztl.sync_lock_until < CURRENT_TIMESTAMP)
		ORDER BY COALESCE(ztl.last_attempt_at, ztl.last_synced_at, ztl.created_at) ASC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	links := []*models.ZammadTicketLink{}
	for rows.Next() {
		link, err := scanZammadTicketLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// ListSyncableTicketLinks returns one stable snapshot of every complete ticket
// link covered by an enabled, authorized connection. Unlike the scheduler's
// due list, an explicit refresh ignores age, retry-delay, and claim gates so
// concurrent work can be reported as skipped instead of disappearing.
func (r *ZammadRepository) ListSyncableTicketLinks() ([]*models.ZammadTicketLink, error) {
	rows, err := r.db.Query(`SELECT ` + zammadTicketLinkColumns + `
		FROM zammad_ticket_links ztl
		JOIN integration_providers ip ON ip.id = ztl.provider_id
		JOIN zammad_connections zc ON zc.provider_id = ztl.provider_id
		WHERE ip.enabled = true AND ztl.ticket_id IS NOT NULL
		  AND ztl.item_integration_link_id IS NOT NULL
		  AND (zc.auth_method != 'oauth' OR EXISTS (
			SELECT 1 FROM zammad_oauth_tokens zot
			WHERE zot.provider_id = zc.provider_id AND zot.reauthorization_required = false
		  ))
		ORDER BY COALESCE(ztl.last_attempt_at, ztl.last_synced_at, ztl.created_at) ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	links := []*models.ZammadTicketLink{}
	for rows.Next() {
		link, err := scanZammadTicketLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (r *ZammadRepository) ClaimSync(id, syncOwner string, until time.Time) (bool, error) {
	if syncOwner == "" {
		return false, ErrInvalidInput
	}
	claimed := false
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE zammad_ticket_links SET sync_lock_until = ?, sync_lock_owner = ?
			WHERE id = ? AND (sync_lock_until IS NULL OR sync_lock_until < CURRENT_TIMESTAMP)`, until, syncOwner, id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		claimed = rows == 1
		return nil
	})
	return claimed, err
}

// RenewSyncClaim extends a lease only if it is still owned by syncOwner. An
// expired lease may be renewed by its original owner until another worker
// takes it over, which prevents an idle scheduler from leaving work stranded.
func (r *ZammadRepository) RenewSyncClaim(id, syncOwner string, until time.Time) (bool, error) {
	if syncOwner == "" {
		return false, ErrInvalidInput
	}
	renewed := false
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE zammad_ticket_links SET sync_lock_until = ?
			WHERE id = ? AND sync_lock_owner = ?`, until, id, syncOwner)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		renewed = rows == 1
		return nil
	})
	return renewed, err
}

// ReleaseSyncClaim clears only the caller's own lease. A stale worker that
// lost a lease to a newer owner receives ErrConcurrentUpdate and cannot clear
// that newer owner's lock.
func (r *ZammadRepository) ReleaseSyncClaim(id, syncOwner string) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE zammad_ticket_links
			SET sync_lock_until = NULL, sync_lock_owner = NULL
			WHERE id = ? AND sync_lock_owner = ?`, id, syncOwner)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrConcurrentUpdate
		}
		return nil
	})
}

func scanZammadTicketLink(scanner interface{ Scan(...any) error }) (*models.ZammadTicketLink, error) {
	var link models.ZammadTicketLink
	var genericLinkID, number, ticketURL, groupName, ownerName, statusName, lastError sql.NullString
	var ticketID, groupID, ownerID, statusID, createdBy sql.NullInt64
	var lastSynced, lastAttempt, nextAttempt sql.NullTime
	err := scanner.Scan(&link.ID, &link.ItemID, &link.ProviderID, &link.ProviderName,
		&genericLinkID, &ticketID, &number, &ticketURL, &groupID, &groupName, &ownerID, &ownerName,
		&link.CorrelationKey, &link.SyncState, &statusID, &statusName,
		&lastSynced, &lastAttempt, &nextAttempt, &lastError, &link.CompletionApplied, &createdBy, &link.CreatedAt, &link.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if genericLinkID.Valid {
		link.ItemIntegrationLinkID = genericLinkID.String
	}
	if ticketID.Valid {
		link.TicketID = int(ticketID.Int64)
	}
	if number.Valid {
		link.TicketNumber = number.String
	}
	if ticketURL.Valid {
		link.TicketURL = ticketURL.String
	}
	if groupID.Valid {
		link.GroupID = int(groupID.Int64)
	}
	if groupName.Valid {
		link.GroupName = groupName.String
	}
	if ownerID.Valid {
		link.OwnerID = int(ownerID.Int64)
	}
	if ownerName.Valid {
		link.OwnerName = ownerName.String
	}
	if statusID.Valid {
		link.LastStatusID = int(statusID.Int64)
	}
	if statusName.Valid {
		link.LastStatusName = statusName.String
	}
	if lastSynced.Valid {
		link.LastSyncedAt = &lastSynced.Time
	}
	if lastAttempt.Valid {
		link.LastAttemptAt = &lastAttempt.Time
	}
	if nextAttempt.Valid {
		link.NextAttemptAt = &nextAttempt.Time
	}
	if lastError.Valid {
		link.LastError = lastError.String
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		link.CreatedBy = &v
	}
	return &link, nil
}

// DeleteTicketLinkClaimed removes a ticket link and its generic item link only
// while the caller still owns the sync lease. This keeps unlinking from
// deleting a reservation or completed link after a different worker has taken
// over an expired lease.
func (r *ZammadRepository) DeleteTicketLinkClaimed(id, syncOwner string) error {
	if syncOwner == "" {
		return ErrInvalidInput
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := r.lockTicketLinkConnectionTx(tx, id); err != nil {
			return err
		}
		var genericID sql.NullString
		if err := tx.QueryRow(`SELECT item_integration_link_id
			FROM zammad_ticket_links WHERE id = ? AND sync_lock_owner = ?`, id, syncOwner).Scan(&genericID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrConcurrentUpdate
			}
			return err
		}
		result, err := tx.Exec(`DELETE FROM zammad_ticket_links
			WHERE id = ? AND sync_lock_owner = ?`, id, syncOwner)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrConcurrentUpdate
		}
		if genericID.Valid {
			_, err = tx.Exec(`DELETE FROM item_integration_links WHERE id = ?`, genericID.String)
			return err
		}
		return nil
	})
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}
