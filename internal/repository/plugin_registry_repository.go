package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"windshift/internal/database"
)

// PluginRegistryEntry is the database-backed view of an installed plugin.
type PluginRegistryEntry struct {
	ID             int
	Name           string
	Version        string
	Description    string
	Author         string
	Enabled        bool
	Routes         []map[string]string
	ExtensionsJSON string
	InstalledAt    string
}

// PluginRegistryUpsert is the metadata persisted for a loaded plugin.
type PluginRegistryUpsert struct {
	Name           string
	Version        string
	Description    string
	Author         string
	Path           string
	Routes         []map[string]string
	ExtensionsJSON string
	Enabled        bool
}

// PluginRegistryRepository provides data access for plugin_registry.
type PluginRegistryRepository struct {
	db database.Database
}

// NewPluginRegistryRepository creates a plugin registry repository.
func NewPluginRegistryRepository(db database.Database) *PluginRegistryRepository {
	return &PluginRegistryRepository{db: db}
}

// List returns all registered plugins ordered by name.
func (r *PluginRegistryRepository) List() ([]PluginRegistryEntry, error) {
	rows, err := r.db.Query(`
		SELECT id, name, version, description, author, enabled, routes, extensions, installed_at
		FROM plugin_registry
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PluginRegistryEntry
	for rows.Next() {
		var p PluginRegistryEntry
		var routesJSON sql.NullString
		var extensionsJSON sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Enabled, &routesJSON, &extensionsJSON, &p.InstalledAt); err != nil {
			return nil, fmt.Errorf("scan plugin: %w", err)
		}
		if routesJSON.Valid && routesJSON.String != "" {
			_ = json.Unmarshal([]byte(routesJSON.String), &p.Routes)
		}
		if extensionsJSON.Valid {
			p.ExtensionsJSON = extensionsJSON.String
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugins: %w", err)
	}
	return out, nil
}

// SetEnabled updates a plugin enabled flag.
func (r *PluginRegistryRepository) SetEnabled(name string, enabled bool) error {
	_, err := r.db.ExecWrite("UPDATE plugin_registry SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?", enabled, name)
	if err != nil {
		return fmt.Errorf("set plugin enabled %q: %w", name, err)
	}
	return nil
}

// Delete removes a plugin registry row.
func (r *PluginRegistryRepository) Delete(name string) error {
	_, err := r.db.ExecWrite("DELETE FROM plugin_registry WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete plugin %q: %w", name, err)
	}
	return nil
}

// Upsert writes plugin metadata.
func (r *PluginRegistryRepository) Upsert(p PluginRegistryUpsert) error {
	routesJSON, _ := json.Marshal(p.Routes)
	_, err := r.db.ExecWrite(`
		INSERT INTO plugin_registry (name, version, description, author, path, routes, extensions, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			version = excluded.version,
			description = excluded.description,
			author = excluded.author,
			path = excluded.path,
			routes = excluded.routes,
			extensions = excluded.extensions,
			enabled = excluded.enabled,
			updated_at = CURRENT_TIMESTAMP
	`, p.Name, p.Version, p.Description, p.Author, p.Path, string(routesJSON), p.ExtensionsJSON, p.Enabled)
	if err != nil {
		return fmt.Errorf("upsert plugin %q: %w", p.Name, err)
	}
	return nil
}
