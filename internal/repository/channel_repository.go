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
	defer func() { _ = rows.Close() }()

	var channels []models.Channel
	for rows.Next() {
		var channel *models.Channel
		channel, err = r.scanChannel(rows)
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

// ListEnabledByTypeAndDirection returns all enabled channels of a given
// type/direction, regardless of manager scope. Used by the
// GET /api/items/{id}/webhooks endpoint to enumerate triggerable outbound
// webhooks; the per-item permission check happens above this in the handler.
func (r *ChannelRepository) ListEnabledByTypeAndDirection(ctx context.Context, channelType, direction string) ([]models.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, config
		FROM channels
		WHERE type = ? AND direction = ? AND status = 'enabled'
	`, channelType, direction)
	if err != nil {
		return nil, fmt.Errorf("list enabled %s/%s channels: %w", channelType, direction, err)
	}
	defer func() { _ = rows.Close() }()

	var channels []models.Channel
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Config); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
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

// SetStatus updates only the status column (used for enable/disable toggles
// where Update would overwrite unrelated fields).
func (r *ChannelRepository) SetStatus(ctx context.Context, tx database.Tx, id int, status string) error {
	result, err := tx.Exec(`UPDATE channels SET status = ?, updated_at = ? WHERE id = ? AND plugin_name IS NULL`, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update channel status: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateConfig updates only the config column. Caller is responsible for any
// merging or validation before passing the JSON in.
func (r *ChannelRepository) UpdateConfig(ctx context.Context, tx database.Tx, id int, config string) error {
	result, err := tx.Exec(`UPDATE channels SET config = ?, updated_at = ? WHERE id = ? AND plugin_name IS NULL`, config, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update channel config: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
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

// FindManagers returns all managers for a channel with joined display fields
// (manager name/email for users, name for groups, plus added-by display name).
func (r *ChannelRepository) FindManagers(ctx context.Context, channelID int) ([]models.ChannelManager, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			cm.id, cm.channel_id, cm.manager_type, cm.manager_id,
			cm.added_by, cm.created_at, cm.updated_at,
			CASE
				WHEN cm.manager_type = 'user' THEN (u.first_name || ' ' || u.last_name)
				WHEN cm.manager_type = 'group' THEN g.name
				ELSE NULL
			END as manager_name,
			CASE
				WHEN cm.manager_type = 'user' THEN u.email
				ELSE NULL
			END as manager_email,
			(added_by_user.first_name || ' ' || added_by_user.last_name) as added_by_name
		FROM channel_managers cm
		LEFT JOIN users u ON cm.manager_type = 'user' AND cm.manager_id = u.id
		LEFT JOIN groups g ON cm.manager_type = 'group' AND cm.manager_id = g.id
		LEFT JOIN users added_by_user ON cm.added_by = added_by_user.id
		WHERE cm.channel_id = ?
		ORDER BY cm.created_at ASC
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel managers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var managers []models.ChannelManager
	for rows.Next() {
		var m models.ChannelManager
		var addedBy sql.NullInt64
		var managerName, managerEmail, addedByName sql.NullString
		err := rows.Scan(
			&m.ID, &m.ChannelID, &m.ManagerType, &m.ManagerID,
			&addedBy, &m.CreatedAt, &m.UpdatedAt,
			&managerName, &managerEmail, &addedByName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel manager: %w", err)
		}
		if addedBy.Valid {
			val := int(addedBy.Int64)
			m.AddedBy = &val
		}
		m.ManagerName = managerName.String
		m.ManagerEmail = managerEmail.String
		m.AddedByName = addedByName.String
		managers = append(managers, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate channel managers: %w", err)
	}

	return managers, nil
}

// AddManager adds a manager to a channel. INSERT OR IGNORE so re-adding an
// existing (channel, type, id) row is a no-op rather than an error.
func (r *ChannelRepository) AddManager(ctx context.Context, tx database.Tx, channelID int, managerType string, managerID, addedBy int) error {
	now := time.Now()
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO channel_managers (channel_id, manager_type, manager_id, added_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, channelID, managerType, managerID, addedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to add channel manager: %w", err)
	}
	return nil
}

// RemoveManager deletes a single channel_managers row by its primary key,
// scoped to channelID so a caller can't cross-delete from another channel.
// Returns true if a row was actually removed.
func (r *ChannelRepository) RemoveManager(ctx context.Context, tx database.Tx, id, channelID int) (bool, error) {
	result, err := tx.Exec(`DELETE FROM channel_managers WHERE id = ? AND channel_id = ?`, id, channelID)
	if err != nil {
		return false, fmt.Errorf("failed to remove channel manager: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return rows > 0, nil
}

// FindManagerRow returns the (manager_type, manager_id) for a single
// channel_managers row. Used by callers that need to populate audit context
// before deletion.
func (r *ChannelRepository) FindManagerRow(ctx context.Context, id, channelID int) (managerType string, managerID int, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT manager_type, manager_id FROM channel_managers WHERE id = ? AND channel_id = ?`,
		id, channelID,
	).Scan(&managerType, &managerID)
	return
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
	delete(config, "email_oauth_client_secret")
	delete(config, "email_oauth_access_token")
	delete(config, "email_oauth_refresh_token")

	// Re-marshal
	scrubbed, err := json.Marshal(config)
	if err != nil {
		return configJSON
	}
	return string(scrubbed)
}
