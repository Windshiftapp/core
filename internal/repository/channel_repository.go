package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ChannelRepository provides data access methods for channels
type ChannelRepository struct {
	db database.Database
}

// NewChannelRepository creates a new channel repository
func NewChannelRepository(db database.Database) *ChannelRepository {
	return &ChannelRepository{db: db}
}

// ChannelListFilters contains filter parameters for listing channels
type ChannelListFilters struct {
	CategoryID      *int   // Filter by category (nil = all, -1 = uncategorized)
	Type            string // Filter by channel type
	Direction       string // Filter by direction (inbound/outbound)
	Status          string // Filter by status
	IncludeDisabled bool   // Include disabled channels
}

// FindAll returns channels visible to the user
// If isAdmin is true, returns all channels; otherwise returns only channels the user manages
func (r *ChannelRepository) FindAll(ctx context.Context, userID int, isAdmin bool, filters ChannelListFilters) ([]models.Channel, error) {
	var query string
	var args []interface{}

	baseSelect := `
		SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
	`

	if isAdmin {
		query = baseSelect
		if filters.CategoryID != nil {
			if *filters.CategoryID == -1 {
				query += " WHERE c.category_id IS NULL"
			} else {
				query += " WHERE c.category_id = ?"
				args = append(args, *filters.CategoryID)
			}
		}
	} else {
		query = baseSelect + `
			INNER JOIN channel_managers cm ON c.id = cm.channel_id
			WHERE ((cm.manager_type = 'user' AND cm.manager_id = ?)
			   OR (cm.manager_type = 'group' AND cm.manager_id IN (
				   SELECT group_id FROM group_members WHERE user_id = ?
			   )))
		`
		args = append(args, userID, userID)

		if filters.CategoryID != nil {
			if *filters.CategoryID == -1 {
				query += " AND c.category_id IS NULL"
			} else {
				query += " AND c.category_id = ?"
				args = append(args, *filters.CategoryID)
			}
		}
	}

	query += " ORDER BY c.is_default DESC, c.created_at ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query channels: %w", err)
	}
	defer rows.Close()

	var channels []models.Channel
	for rows.Next() {
		channel, err := r.scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, *channel)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading channels: %w", err)
	}

	return channels, nil
}

// FindByID retrieves a single channel by ID
func (r *ChannelRepository) FindByID(ctx context.Context, id int) (*models.Channel, error) {
	query := `
		SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
		WHERE c.id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanChannelRow(row)
}

// FindBySlug retrieves a portal channel by its slug
func (r *ChannelRepository) FindBySlug(ctx context.Context, slug string) (*models.Channel, error) {
	query := `
		SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
		WHERE c.type = 'portal' AND json_extract(c.config, '$.portal_slug') = ?
	`

	row := r.db.QueryRowContext(ctx, query, slug)
	return r.scanChannelRow(row)
}

// Create inserts a new channel and returns its ID
func (r *ChannelRepository) Create(ctx context.Context, tx database.Tx, channel *models.Channel) (int, error) {
	now := time.Now()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	var id int64
	err := tx.QueryRow(`
		INSERT INTO channels (name, type, direction, description, status, is_default, config, category_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		channel.Name, channel.Type, channel.Direction, channel.Description,
		channel.Status, channel.IsDefault, channel.Config, channel.CategoryID, channel.CreatedAt, channel.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create channel: %w", err)
	}

	return int(id), nil
}

// Update updates an existing channel
func (r *ChannelRepository) Update(ctx context.Context, tx database.Tx, channel *models.Channel) error {
	channel.UpdatedAt = time.Now()

	result, err := tx.Exec(`
		UPDATE channels
		SET name = ?, description = ?, status = ?, is_default = ?, config = ?, category_id = ?, updated_at = ?
		WHERE id = ? AND plugin_name IS NULL`,
		channel.Name, channel.Description, channel.Status, channel.IsDefault,
		channel.Config, channel.CategoryID, channel.UpdatedAt, channel.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update channel: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a channel by ID (only non-plugin channels)
func (r *ChannelRepository) Delete(ctx context.Context, tx database.Tx, id int) error {
	// First delete channel managers
	_, err := tx.Exec("DELETE FROM channel_managers WHERE channel_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete channel managers: %w", err)
	}

	// Then delete the channel
	result, err := tx.Exec("DELETE FROM channels WHERE id = ? AND plugin_name IS NULL", id)
	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateLastActivity updates the last_activity timestamp
func (r *ChannelRepository) UpdateLastActivity(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE channels SET last_activity = ? WHERE id = ?", time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update last activity: %w", err)
	}
	return nil
}

// SetDefault marks a channel as the default for its type
func (r *ChannelRepository) SetDefault(ctx context.Context, tx database.Tx, id int) error {
	// Get the channel type first
	var channelType string
	err := r.db.QueryRowContext(ctx, "SELECT type FROM channels WHERE id = ?", id).Scan(&channelType)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get channel type: %w", err)
	}

	// Clear existing default for this type
	_, err = tx.Exec("UPDATE channels SET is_default = FALSE WHERE type = ?", channelType)
	if err != nil {
		return fmt.Errorf("failed to clear existing default: %w", err)
	}

	// Set new default
	_, err = tx.Exec("UPDATE channels SET is_default = TRUE WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to set default: %w", err)
	}

	return nil
}

// Exists checks if a channel exists
func (r *ChannelRepository) Exists(ctx context.Context, id int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check channel existence: %w", err)
	}
	return exists, nil
}

// IsPluginManaged checks if a channel is managed by a plugin
func (r *ChannelRepository) IsPluginManaged(ctx context.Context, id int) (bool, error) {
	var pluginName sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT plugin_name FROM channels WHERE id = ?", id).Scan(&pluginName)
	if err == sql.ErrNoRows {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("failed to check plugin managed: %w", err)
	}
	return pluginName.Valid && pluginName.String != "", nil
}

// GetConfig retrieves the raw config JSON for a channel
func (r *ChannelRepository) GetConfig(ctx context.Context, id int) (string, error) {
	var config string
	err := r.db.QueryRowContext(ctx, "SELECT config FROM channels WHERE id = ?", id).Scan(&config)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	return config, nil
}

// Channel Manager methods

// FindManagers returns all managers for a channel
func (r *ChannelRepository) FindManagers(ctx context.Context, channelID int) ([]models.ChannelManager, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, channel_id, manager_type, manager_id, created_at
		FROM channel_managers
		WHERE channel_id = ?
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel managers: %w", err)
	}
	defer rows.Close()

	var managers []models.ChannelManager
	for rows.Next() {
		var m models.ChannelManager
		err := rows.Scan(&m.ID, &m.ChannelID, &m.ManagerType, &m.ManagerID, &m.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel manager: %w", err)
		}
		managers = append(managers, m)
	}

	return managers, nil
}

// AddManager adds a manager to a channel
func (r *ChannelRepository) AddManager(ctx context.Context, tx database.Tx, channelID int, managerType string, managerID int) error {
	_, err := tx.Exec(`
		INSERT INTO channel_managers (channel_id, manager_type, manager_id, created_at)
		VALUES (?, ?, ?, ?)
	`, channelID, managerType, managerID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add channel manager: %w", err)
	}
	return nil
}

// RemoveManager removes a manager from a channel
func (r *ChannelRepository) RemoveManager(ctx context.Context, tx database.Tx, channelID int, managerType string, managerID int) error {
	_, err := tx.Exec(`
		DELETE FROM channel_managers
		WHERE channel_id = ? AND manager_type = ? AND manager_id = ?
	`, channelID, managerType, managerID)
	if err != nil {
		return fmt.Errorf("failed to remove channel manager: %w", err)
	}
	return nil
}

// IsManager checks if a user is a manager of a channel (directly or through group membership)
func (r *ChannelRepository) IsManager(ctx context.Context, channelID, userID int) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM channel_managers cm
		WHERE cm.channel_id = ?
		  AND ((cm.manager_type = 'user' AND cm.manager_id = ?)
		       OR (cm.manager_type = 'group' AND cm.manager_id IN (
		           SELECT group_id FROM group_members WHERE user_id = ?
		       )))
	`, channelID, userID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check manager status: %w", err)
	}
	return count > 0, nil
}

// Helper methods

func (r *ChannelRepository) scanChannel(rows *sql.Rows) (*models.Channel, error) {
	var channel models.Channel
	var categoryName, categoryColor sql.NullString

	err := rows.Scan(
		&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
		&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
		&channel.PluginName, &channel.PluginWebhookID, &channel.CategoryID,
		&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
		&categoryName, &categoryColor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan channel: %w", err)
	}

	if categoryName.Valid {
		channel.CategoryName = categoryName.String
	}
	if categoryColor.Valid {
		channel.CategoryColor = categoryColor.String
	}

	// Scrub sensitive data from config
	channel.Config = ScrubChannelConfig(channel.Config)

	return &channel, nil
}

func (r *ChannelRepository) scanChannelRow(row *sql.Row) (*models.Channel, error) {
	var channel models.Channel
	var categoryName, categoryColor sql.NullString

	err := row.Scan(
		&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
		&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
		&channel.PluginName, &channel.PluginWebhookID, &channel.CategoryID,
		&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
		&categoryName, &categoryColor,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan channel: %w", err)
	}

	if categoryName.Valid {
		channel.CategoryName = categoryName.String
	}
	if categoryColor.Valid {
		channel.CategoryColor = categoryColor.String
	}

	// Scrub sensitive data from config
	channel.Config = ScrubChannelConfig(channel.Config)

	return &channel, nil
}

// ScrubChannelConfig removes sensitive fields from the configuration JSON
func ScrubChannelConfig(configJSON string) string {
	if configJSON == "" {
		return ""
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return configJSON // Return as is if invalid JSON
	}

	// Remove sensitive fields
	delete(config, "smtp_password")
	delete(config, "imap_password")
	delete(config, "webhook_secret")

	// Re-marshal
	scrubbed, err := json.Marshal(config)
	if err != nil {
		return configJSON
	}
	return string(scrubbed)
}
