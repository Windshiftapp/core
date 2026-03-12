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

// Resolve returns a Client for the given connection ID.
// If connectionID > 0, uses that specific enabled connection.
// Otherwise, picks the default enabled connection (or the first enabled one).
// Falls back to the env-var-based client if no DB connections exist.
func (m *ConnectionManager) Resolve(connectionID int) (Client, error) {
	var row *sql.Row
	if connectionID > 0 {
		row = m.db.QueryRow(
			`SELECT id, provider_type, model, api_key_encrypted, base_url
			 FROM llm_connections
			 WHERE id = ? AND is_enabled = true`,
			connectionID,
		)
	} else {
		row = m.db.QueryRow(
			`SELECT id, provider_type, model, api_key_encrypted, base_url
			 FROM llm_connections
			 WHERE is_enabled = true
			 ORDER BY is_default DESC, id ASC
			 LIMIT 1`,
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
		connections = append(connections, c)
	}
	if connections == nil {
		connections = []ConnectionInfo{}
	}
	return connections, nil
}

// ListEnabled returns all enabled connections (for user dropdown).
func (m *ConnectionManager) ListEnabled() ([]ConnectionInfo, error) {
	rows, err := m.db.Query(
		`SELECT id, name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled, created_at, updated_at
		 FROM llm_connections
		 WHERE is_enabled = true
		 ORDER BY is_default DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled connections: %w", err)
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
		if _, err := m.db.Exec("UPDATE llm_connections SET is_default = false WHERE is_default = true"); err != nil {
			return nil, fmt.Errorf("failed to clear existing defaults: %w", err)
		}
	}

	var id int64
	err := m.db.QueryRow(
		`INSERT INTO llm_connections (name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		req.Name, string(req.ProviderType), req.Model, encryptedKey, baseURL, req.IsDefault, req.IsEnabled,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
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
}

// UpdateConnection updates an existing LLM connection.
func (m *ConnectionManager) UpdateConnection(id int, req UpdateConnectionRequest) (*ConnectionInfo, error) {
	// If setting as default, clear existing defaults
	if req.IsDefault {
		if _, err := m.db.Exec("UPDATE llm_connections SET is_default = false WHERE is_default = true AND id != ?", id); err != nil {
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
