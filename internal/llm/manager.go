package llm

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/sso"
)

// ConnectionInfo represents an LLM connection without sensitive fields.
type ConnectionInfo struct {
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"provider_type"`
	Model        string       `json:"model"`
	HasAPIKey    bool         `json:"has_api_key"`
	BaseURL      string       `json:"base_url,omitempty"`
	IsDefault    bool         `json:"is_default"`
	IsEnabled    bool         `json:"is_enabled"`
	Features     []string     `json:"features"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// ConnectionManager bridges the database and the LLM client layer.
type ConnectionManager struct {
	db         database.Database
	encryption *sso.SecretEncryption
	fallback   Client
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(db database.Database, encryption *sso.SecretEncryption, fallback Client) *ConnectionManager {
	return &ConnectionManager{
		db:         db,
		encryption: encryption,
		fallback:   fallback,
	}
}

// ResolveForFeature returns a Client for the given feature.
// If connectionID > 0, uses that specific connection.
// Otherwise, uses the default connection for the feature.
// Falls back to the env-var-based client if no DB connections exist.
func (m *ConnectionManager) ResolveForFeature(feature string, connectionID int) (Client, error) {
	var row *sql.Row
	if connectionID > 0 {
		row = m.db.QueryRow(
			`SELECT c.id, c.provider_type, c.model, c.api_key_encrypted, c.base_url
			 FROM llm_connections c
			 JOIN llm_connection_features f ON f.connection_id = c.id
			 WHERE c.id = ? AND c.is_enabled = 1 AND f.feature = ?`,
			connectionID, feature,
		)
	} else {
		row = m.db.QueryRow(
			`SELECT c.id, c.provider_type, c.model, c.api_key_encrypted, c.base_url
			 FROM llm_connections c
			 JOIN llm_connection_features f ON f.connection_id = c.id
			 WHERE c.is_enabled = 1 AND f.feature = ?
			 ORDER BY c.is_default DESC, c.id ASC
			 LIMIT 1`,
			feature,
		)
	}

	var id int
	var providerType, model string
	var apiKeyEncrypted, baseURL sql.NullString
	err := row.Scan(&id, &providerType, &model, &apiKeyEncrypted, &baseURL)
	if err == sql.ErrNoRows {
		// No DB connections configured — fall back to the env-var client
		return m.fallback, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query connection: %w", err)
	}

	var apiKey string
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		apiKey, err = m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
	}

	return NewProviderClient(ConnectionConfig{
		ProviderType: ProviderType(providerType),
		Model:        model,
		APIKey:       apiKey,
		BaseURL:      baseURL.String,
	}), nil
}

// ListConnections returns all connections (without secrets) for admin listing.
func (m *ConnectionManager) ListConnections() ([]ConnectionInfo, error) {
	rows, err := m.db.Query(
		`SELECT id, name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled, created_at, updated_at
		 FROM llm_connections ORDER BY is_default DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	var connections []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		var apiKeyEncrypted, baseURL sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.ProviderType, &c.Model, &apiKeyEncrypted, &baseURL, &c.IsDefault, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}
		c.HasAPIKey = apiKeyEncrypted.Valid && apiKeyEncrypted.String != ""
		if baseURL.Valid {
			c.BaseURL = baseURL.String
		}
		c.Features = m.getFeatures(c.ID)
		connections = append(connections, c)
	}
	if connections == nil {
		connections = []ConnectionInfo{}
	}
	return connections, nil
}

// ListForFeature returns enabled connections assigned to a feature (for user dropdown).
func (m *ConnectionManager) ListForFeature(feature string) ([]ConnectionInfo, error) {
	rows, err := m.db.Query(
		`SELECT c.id, c.name, c.provider_type, c.model, c.api_key_encrypted, c.base_url, c.is_default, c.is_enabled, c.created_at, c.updated_at
		 FROM llm_connections c
		 JOIN llm_connection_features f ON f.connection_id = c.id
		 WHERE c.is_enabled = 1 AND f.feature = ?
		 ORDER BY c.is_default DESC, c.name ASC`,
		feature,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections for feature: %w", err)
	}
	defer rows.Close()

	var connections []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		var apiKeyEncrypted, baseURL sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.ProviderType, &c.Model, &apiKeyEncrypted, &baseURL, &c.IsDefault, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}
		c.HasAPIKey = apiKeyEncrypted.Valid && apiKeyEncrypted.String != ""
		if baseURL.Valid {
			c.BaseURL = baseURL.String
		}
		c.Features = []string{feature}
		connections = append(connections, c)
	}
	if connections == nil {
		connections = []ConnectionInfo{}
	}
	return connections, nil
}

// GetConnection returns a single connection by ID.
func (m *ConnectionManager) GetConnection(id int) (*ConnectionInfo, error) {
	var c ConnectionInfo
	var apiKeyEncrypted, baseURL sql.NullString
	err := m.db.QueryRow(
		`SELECT id, name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled, created_at, updated_at
		 FROM llm_connections WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.ProviderType, &c.Model, &apiKeyEncrypted, &baseURL, &c.IsDefault, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	c.HasAPIKey = apiKeyEncrypted.Valid && apiKeyEncrypted.String != ""
	if baseURL.Valid {
		c.BaseURL = baseURL.String
	}
	c.Features = m.getFeatures(c.ID)
	return &c, nil
}

// CreateConnectionRequest is the input for creating a connection.
type CreateConnectionRequest struct {
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"provider_type"`
	Model        string       `json:"model"`
	APIKey       string       `json:"api_key,omitempty"`
	BaseURL      string       `json:"base_url,omitempty"`
	IsDefault    bool         `json:"is_default"`
	IsEnabled    bool         `json:"is_enabled"`
	Features     []string     `json:"features,omitempty"`
}

// CreateConnection creates a new LLM connection.
func (m *ConnectionManager) CreateConnection(req CreateConnectionRequest) (*ConnectionInfo, error) {
	var encryptedKey sql.NullString
	if req.APIKey != "" {
		encrypted, err := m.encryption.Encrypt(req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		encryptedKey = sql.NullString{String: encrypted, Valid: true}
	}

	var baseURL sql.NullString
	if req.BaseURL != "" {
		baseURL = sql.NullString{String: req.BaseURL, Valid: true}
	}

	// If setting as default, clear existing defaults
	if req.IsDefault {
		if _, err := m.db.Exec("UPDATE llm_connections SET is_default = 0 WHERE is_default = 1"); err != nil {
			return nil, fmt.Errorf("failed to clear existing defaults: %w", err)
		}
	}

	result, err := m.db.Exec(
		`INSERT INTO llm_connections (name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.Name, string(req.ProviderType), req.Model, encryptedKey, baseURL, req.IsDefault, req.IsEnabled,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	id, _ := result.LastInsertId()

	// Insert features
	for _, feature := range req.Features {
		if _, err := m.db.Exec(
			"INSERT INTO llm_connection_features (connection_id, feature) VALUES (?, ?)",
			id, feature,
		); err != nil {
			return nil, fmt.Errorf("failed to insert feature: %w", err)
		}
	}

	return m.GetConnection(int(id))
}

// UpdateConnectionRequest is the input for updating a connection.
type UpdateConnectionRequest struct {
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"provider_type"`
	Model        string       `json:"model"`
	APIKey       string       `json:"api_key,omitempty"`
	BaseURL      string       `json:"base_url,omitempty"`
	IsDefault    bool         `json:"is_default"`
	IsEnabled    bool         `json:"is_enabled"`
	Features     []string     `json:"features,omitempty"`
}

// UpdateConnection updates an existing LLM connection.
func (m *ConnectionManager) UpdateConnection(id int, req UpdateConnectionRequest) (*ConnectionInfo, error) {
	// If setting as default, clear existing defaults
	if req.IsDefault {
		if _, err := m.db.Exec("UPDATE llm_connections SET is_default = 0 WHERE is_default = 1 AND id != ?", id); err != nil {
			return nil, fmt.Errorf("failed to clear existing defaults: %w", err)
		}
	}

	if req.APIKey != "" {
		encrypted, err := m.encryption.Encrypt(req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		_, err = m.db.Exec(
			`UPDATE llm_connections SET name = ?, provider_type = ?, model = ?, api_key_encrypted = ?, base_url = ?, is_default = ?, is_enabled = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			req.Name, string(req.ProviderType), req.Model, encrypted, req.BaseURL, req.IsDefault, req.IsEnabled, id,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update connection: %w", err)
		}
	} else {
		// Don't overwrite API key if not provided
		_, err := m.db.Exec(
			`UPDATE llm_connections SET name = ?, provider_type = ?, model = ?, base_url = ?, is_default = ?, is_enabled = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			req.Name, string(req.ProviderType), req.Model, req.BaseURL, req.IsDefault, req.IsEnabled, id,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update connection: %w", err)
		}
	}

	// Replace features
	if err := m.SetFeatures(id, req.Features); err != nil {
		return nil, err
	}

	return m.GetConnection(id)
}

// DeleteConnection deletes an LLM connection.
func (m *ConnectionManager) DeleteConnection(id int) error {
	_, err := m.db.Exec("DELETE FROM llm_connections WHERE id = ?", id)
	return err
}

// TestConnection tests a connection by creating a client and calling Health.
func (m *ConnectionManager) TestConnection(id int) error {
	var providerType, model string
	var apiKeyEncrypted, baseURL sql.NullString
	err := m.db.QueryRow(
		"SELECT provider_type, model, api_key_encrypted, base_url FROM llm_connections WHERE id = ?", id,
	).Scan(&providerType, &model, &apiKeyEncrypted, &baseURL)
	if err != nil {
		return fmt.Errorf("connection not found: %w", err)
	}

	var apiKey string
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		apiKey, err = m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return fmt.Errorf("failed to decrypt API key: %w", err)
		}
	}

	client := NewProviderClient(ConnectionConfig{
		ProviderType: ProviderType(providerType),
		Model:        model,
		APIKey:       apiKey,
		BaseURL:      baseURL.String,
		Timeout:      30 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return client.Health(ctx)
}

// SetFeatures replaces the feature assignments for a connection.
func (m *ConnectionManager) SetFeatures(id int, features []string) error {
	if _, err := m.db.Exec("DELETE FROM llm_connection_features WHERE connection_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete existing features: %w", err)
	}
	for _, feature := range features {
		if _, err := m.db.Exec(
			"INSERT INTO llm_connection_features (connection_id, feature) VALUES (?, ?)",
			id, feature,
		); err != nil {
			return fmt.Errorf("failed to insert feature: %w", err)
		}
	}
	return nil
}

func (m *ConnectionManager) getFeatures(connectionID int) []string {
	rows, err := m.db.Query("SELECT feature FROM llm_connection_features WHERE connection_id = ?", connectionID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	var features []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err == nil {
			features = append(features, f)
		}
	}
	if features == nil {
		features = []string{}
	}
	return features
}
