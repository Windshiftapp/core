package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// SlugCandidate is a minimal public-channel row together with its parsed
// configuration.
type SlugCandidate struct {
	Channel models.Channel
	Config  models.ChannelConfig
}

// FindEnabledByPublicSlug resolves a public channel through the normalized,
// uniquely indexed slug column. The old implementation loaded and decoded
// every portal/form config on every public request, making anonymous traffic
// O(number of channels) and allowing one malformed row to affect lookup.
func (r *ChannelRepository) FindEnabledByPublicSlug(ctx context.Context, channelType, slug string) (*SlugCandidate, error) {
	var candidate SlugCandidate
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, COALESCE(config, '{}'), status
		FROM channels
		WHERE type = ? AND direction = 'inbound' AND status = 'enabled'
		  AND public_slug = ?
	`, channelType, slug).Scan(
		&candidate.Channel.ID,
		&candidate.Channel.Name,
		&candidate.Channel.Type,
		&candidate.Channel.Config,
		&candidate.Channel.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find enabled %s channel by public slug: %w", channelType, err)
	}
	actualSlug, err := publicSlugForConfig(candidate.Channel.Type, "inbound", candidate.Channel.Config)
	if err != nil {
		return nil, fmt.Errorf("parse public channel %d config: %w", candidate.Channel.ID, err)
	}
	if actualSlug != slug {
		// public_slug is derived metadata. Refuse a stale/corrupt row instead of
		// routing a request to a config that advertises a different public URL.
		return nil, ErrNotFound
	}
	if err := json.Unmarshal([]byte(candidate.Channel.Config), &candidate.Config); err != nil {
		return nil, fmt.Errorf("parse public channel %d config: %w", candidate.Channel.ID, err)
	}
	return &candidate, nil
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
	var args []any

	baseSelect := `
		SELECT c.id, c.name, c.type, c.direction, COALESCE(c.description, ''), c.status, COALESCE(c.is_default, false), COALESCE(c.config, '{}'),
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
	`

	// Build WHERE clauses incrementally so admin and non-admin share filter
	// logic — keeping the visibility predicate as the first clause.
	var where []string

	if !isAdmin {
		where = append(where, `EXISTS (
			SELECT 1 FROM channel_managers cm
			WHERE cm.channel_id = c.id
			  AND (
			      (cm.manager_type = 'user' AND cm.manager_id = ?)
			   OR (cm.manager_type = 'group' AND cm.manager_id IN (
			          SELECT gm.group_id
			          FROM group_members gm
			          JOIN groups g ON g.id = gm.group_id
			          WHERE gm.user_id = ? AND g.is_active = true
			      ))
			  )
		) AND COALESCE(c.is_default, false) = false`)
		args = append(args, userID, userID)
	}

	if filters.CategoryID != nil {
		if *filters.CategoryID == -1 {
			where = append(where, "c.category_id IS NULL")
		} else {
			where = append(where, "c.category_id = ?")
			args = append(args, *filters.CategoryID)
		}
	}

	if filters.Type != "" {
		where = append(where, "c.type = ?")
		args = append(args, filters.Type)
	}
	if filters.Direction != "" {
		where = append(where, "c.direction = ?")
		args = append(args, filters.Direction)
	}
	// Status filter is the explicit form. IncludeDisabled is a coarser
	// shorthand: when false (default) and no explicit Status filter is set,
	// hide disabled channels from non-admin callers — admins keep seeing
	// everything so the admin UI can show the toggle state.
	if filters.Status != "" {
		where = append(where, "c.status = ?")
		args = append(args, filters.Status)
	} else if !filters.IncludeDisabled && !isAdmin {
		where = append(where, "c.status = 'enabled'")
	}

	query := baseSelect
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
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

// UserCanManage returns true if userID is a direct or group-assigned manager
// of channelID. Mirrors the manager-scope clause used by FindAll for
// non-admin callers, so admin checks remain the caller's responsibility.
func (r *ChannelRepository) UserCanManage(ctx context.Context, userID, channelID int) (bool, error) {
	var found bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM channel_managers cm
			WHERE cm.channel_id = ?
			  AND (
			      (cm.manager_type = 'user' AND cm.manager_id = ?)
			   OR (cm.manager_type = 'group' AND cm.manager_id IN (
			          SELECT gm.group_id
			          FROM group_members gm
			          JOIN groups g ON g.id = gm.group_id
			          WHERE gm.user_id = ? AND g.is_active = true
			      ))
			  )
		)
	`, channelID, userID, userID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("failed to check channel manager: %w", err)
	}
	return found, nil
}

// UserManagesAny reports whether userID directly manages, or belongs to an
// active group that manages, at least one non-default channel. Default channels
// are intentionally excluded because non-admin managers cannot operate them.
func (r *ChannelRepository) UserManagesAny(ctx context.Context, userID int) (bool, error) {
	var found bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM channel_managers cm
			JOIN channels c ON c.id = cm.channel_id
			WHERE COALESCE(c.is_default, false) = false
			  AND (
			      (cm.manager_type = 'user' AND cm.manager_id = ?)
			   OR (cm.manager_type = 'group' AND cm.manager_id IN (
			          SELECT gm.group_id
			          FROM group_members gm
			          JOIN groups g ON g.id = gm.group_id
			          WHERE gm.user_id = ? AND g.is_active = true
			      ))
			  )
		)
	`, userID, userID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("failed to check channel manager availability: %w", err)
	}
	return found, nil
}

// ListEnabledByTypeAndDirection returns all enabled channels of a given
// type/direction, regardless of manager scope. Used by the
// GET /api/items/{id}/webhooks endpoint to enumerate triggerable outbound
// webhooks; the per-item permission check happens above this in the handler.
func (r *ChannelRepository) ListEnabledByTypeAndDirection(ctx context.Context, channelType, direction string) ([]models.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(config, '{}')
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}
	return channels, nil
}

// FindByID retrieves a single channel by ID
func (r *ChannelRepository) FindByID(ctx context.Context, id int) (*models.Channel, error) {
	query := `
		SELECT c.id, c.name, c.type, c.direction, COALESCE(c.description, ''), c.status, COALESCE(c.is_default, false), COALESCE(c.config, '{}'),
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
		WHERE c.id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanChannelRow(row)
}

// FindInboundPortalBySlug returns a portal channel for import-time slug
// collision resolution.
func (r *ChannelRepository) FindInboundPortalBySlug(ctx context.Context, slug string) (*models.Channel, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.name, c.type, c.direction, COALESCE(c.description, ''),
		       c.status, COALESCE(c.is_default, false), COALESCE(c.config, '{}'),
		       c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at,
		       c.updated_at, c.last_activity, cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
		WHERE c.type = 'portal' AND c.direction = 'inbound' AND c.public_slug = ?
	`, slug)
	return r.scanChannelRow(row)
}

// CreatePortalWithManager atomically creates a portal and grants its importing
// user channel-manager access when one is available.
func (r *ChannelRepository) CreatePortalWithManager(
	ctx context.Context,
	channel *models.Channel,
	managerUserID int,
) (int, error) {
	return database.WithTxResult(r.db, func(tx database.Tx) (int, error) {
		id, err := r.Create(ctx, tx, channel)
		if err != nil {
			return 0, err
		}
		if managerUserID > 0 {
			if _, err := r.AddManager(ctx, tx, id, "user", managerUserID, managerUserID); err != nil {
				return 0, err
			}
		}
		return id, nil
	})
}

// SetConfig performs a repository-owned transaction for callers that only
// need to replace channel configuration.
func (r *ChannelRepository) SetConfig(ctx context.Context, id int, config string) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		return r.UpdateConfig(ctx, tx, id, config)
	})
}

// Create inserts a new channel and returns its ID
func (r *ChannelRepository) Create(ctx context.Context, tx database.Tx, channel *models.Channel) (int, error) {
	now := time.Now()
	channel.CreatedAt = now
	channel.UpdatedAt = now
	publicSlug, err := publicSlugForConfig(channel.Type, channel.Direction, channel.Config)
	if err != nil {
		return 0, fmt.Errorf("derive channel public slug: %w", err)
	}

	var id int64
	err = tx.QueryRow(`
		INSERT INTO channels (name, type, direction, description, status, is_default, config, public_slug, category_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		channel.Name, channel.Type, channel.Direction, channel.Description,
		channel.Status, channel.IsDefault, channel.Config, nullableSlug(publicSlug), channel.CategoryID, channel.CreatedAt, channel.UpdatedAt,
	).Scan(&id)
	if err != nil {
		if isPublicSlugConstraintError(err) {
			return 0, ErrChannelSlugConflict
		}
		return 0, fmt.Errorf("failed to create channel: %w", err)
	}

	return int(id), nil
}

// Update updates an existing channel
func (r *ChannelRepository) Update(ctx context.Context, tx database.Tx, channel *models.Channel) error {
	channel.UpdatedAt = time.Now()

	result, err := tx.Exec(`
		UPDATE channels
		SET name = ?, description = ?, category_id = ?, updated_at = ?
		WHERE id = ? AND plugin_name IS NULL`,
		channel.Name, channel.Description, channel.CategoryID, channel.UpdatedAt, channel.ID,
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
	// Keep the default predicate on the DELETE itself. The handler's earlier
	// read is only for a friendly fast-path; another transaction may promote
	// this channel before we reach the write.
	result, err := tx.Exec(`
		DELETE FROM channels
		WHERE id = ? AND plugin_name IS NULL AND COALESCE(is_default, false) = false
	`, id)
	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		var isDefault bool
		err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(is_default, false) FROM channels WHERE id = ?
		`, id).Scan(&isDefault)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("inspect channel after rejected delete: %w", err)
		}
		if isDefault {
			return ErrDefaultChannel
		}
		return ErrNotFound
	}

	return nil
}

// UpdateLastActivity updates the last_activity timestamp
func (r *ChannelRepository) UpdateLastActivity(ctx context.Context, id int) error {
	_, err := r.db.ExecWriteContext(ctx, "UPDATE channels SET last_activity = ? WHERE id = ?", time.Now(), id)
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

// SetStatusIfConfigUnchanged enables/disables a channel only when the exact
// config that the caller validated is still current. This closes the window
// where a concurrent config edit could land between readiness validation and
// the status mutation.
func (r *ChannelRepository) SetStatusIfConfigUnchanged(ctx context.Context, tx database.Tx, id int, status, expectedConfig string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE channels SET status = ?, updated_at = ?
		WHERE id = ? AND plugin_name IS NULL AND COALESCE(config, '{}') = ?
	`, status, time.Now(), id, expectedConfig)
	if err != nil {
		return false, fmt.Errorf("conditionally update channel status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count conditionally updated channel statuses: %w", err)
	}
	return rows > 0, nil
}

// UpdateConfig updates only the config column. Caller is responsible for any
// merging or validation before passing the JSON in.
func (r *ChannelRepository) UpdateConfig(ctx context.Context, tx database.Tx, id int, config string) error {
	portalSlug, formSlug, err := configSlugValues(config)
	if err != nil {
		return fmt.Errorf("derive channel public slug: %w", err)
	}
	result, err := tx.Exec(`
		UPDATE channels
		SET config = ?,
			public_slug = CASE
				WHEN type = 'portal' AND direction = 'inbound' THEN ?
				WHEN type = 'form' AND direction = 'inbound' THEN ?
				ELSE NULL
			END,
			updated_at = ?
		WHERE id = ? AND plugin_name IS NULL
	`, config, nullableSlug(portalSlug), nullableSlug(formSlug), time.Now(), id)
	if err != nil {
		if isPublicSlugConstraintError(err) {
			return ErrChannelSlugConflict
		}
		return fmt.Errorf("failed to update channel config: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateConfigIfUnchanged performs an optimistic config write. It returns
// false when another writer changed the raw config or status after the caller
// loaded it, allowing the handler to report a conflict instead of racing a
// readiness-validated enable against an incomplete config patch.
func (r *ChannelRepository) UpdateConfigIfUnchanged(ctx context.Context, tx database.Tx, id int, expectedConfig, expectedStatus, config string) (bool, error) {
	portalSlug, formSlug, err := configSlugValues(config)
	if err != nil {
		return false, fmt.Errorf("derive channel public slug: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE channels
		SET config = ?,
			public_slug = CASE
				WHEN type = 'portal' AND direction = 'inbound' THEN ?
				WHEN type = 'form' AND direction = 'inbound' THEN ?
				ELSE NULL
			END,
			updated_at = ?
		WHERE id = ? AND plugin_name IS NULL AND COALESCE(config, '{}') = ? AND status = ?
	`, config, nullableSlug(portalSlug), nullableSlug(formSlug), time.Now(), id, expectedConfig, expectedStatus)
	if err != nil {
		if isPublicSlugConstraintError(err) {
			return false, ErrChannelSlugConflict
		}
		return false, fmt.Errorf("conditionally update channel config: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count conditionally updated channel configs: %w", err)
	}
	return rowsAffected > 0, nil
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

// CategoryExists validates the optional category FK before a write so API
// callers receive a 400 instead of a backend-specific constraint error/500.
func (r *ChannelRepository) CategoryExists(ctx context.Context, id int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM channel_categories WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check channel category: %w", err)
	}
	return exists, nil
}

// IsPluginManaged checks if a channel is managed by a plugin
func (r *ChannelRepository) IsPluginManaged(ctx context.Context, id int) (bool, error) {
	var pluginName sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT plugin_name FROM channels WHERE id = ?", id).Scan(&pluginName)
	if err != nil {
		return false, notFoundOrWrap(err, "failed to check plugin managed")
	}
	return pluginName.Valid && pluginName.String != "", nil
}

// GetConfig retrieves the raw config JSON for a channel
func (r *ChannelRepository) GetConfig(ctx context.Context, id int) (string, error) {
	var config string
	err := r.db.QueryRowContext(ctx, "SELECT COALESCE(config, '{}') FROM channels WHERE id = ?", id).Scan(&config)
	if err != nil {
		return "", notFoundOrWrap(err, "failed to get config")
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

// AddManager adds a manager to a channel. ON CONFLICT DO NOTHING so re-adding
// an existing (channel, type, id) row is a no-op rather than an error. The
// bool reports whether a new row was inserted.
func (r *ChannelRepository) AddManager(ctx context.Context, tx database.Tx, channelID int, managerType string, managerID, addedBy int) (bool, error) {
	now := time.Now()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO channel_managers (channel_id, manager_type, manager_id, added_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, channelID, managerType, managerID, addedBy, now, now)
	if err != nil {
		return false, fmt.Errorf("failed to add channel manager: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count added channel managers: %w", err)
	}
	return rows > 0, nil
}

// LockManagerSet serializes manager-count mutations for a channel. The no-op
// UPDATE acquires a row/write lock on both supported databases without
// changing user-visible data.
func (r *ChannelRepository) LockManagerSet(ctx context.Context, tx database.Tx, channelID int) error {
	result, err := tx.ExecContext(ctx, "UPDATE channels SET updated_at = updated_at WHERE id = ?", channelID)
	if err != nil {
		return fmt.Errorf("lock channel manager set: %w", err)
	}
	if n, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountManagers returns the number of effective channel managers: direct
// users and groups must still exist and be active. Callers use this after a
// tentative delete to enforce the last-manager rule without letting stale or
// deactivated assignments make an orphaned channel look managed.
func (r *ChannelRepository) CountManagers(ctx context.Context, tx database.Tx, channelID int) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM channel_managers cm
		WHERE cm.channel_id = ?
		  AND (
			(cm.manager_type = 'user' AND EXISTS (
				SELECT 1 FROM users u
				WHERE u.id = cm.manager_id AND u.is_active = true
			))
			OR
			(cm.manager_type = 'group' AND EXISTS (
				SELECT 1
				FROM groups g
				WHERE g.id = cm.manager_id
				  AND g.is_active = true
				  AND EXISTS (
					SELECT 1
					FROM group_members gm
					JOIN users u ON u.id = gm.user_id AND u.is_active = true
					WHERE gm.group_id = g.id
				  )
			))
		  )
	`, channelID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count channel managers: %w", err)
	}
	return n, nil
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
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
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
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to scan channel")
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

// GetGroupName fetches a group's name. Used to enrich audit details on
// channel-manager add/remove; returns empty string + nil if the row is
// missing (caller treats that as "unknown group"). User-side equivalent
// lives on UserRepository.GetFullName.
func (r *ChannelRepository) GetGroupName(ctx context.Context, groupID int) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx,
		"SELECT name FROM groups WHERE id = ?",
		groupID,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get group %d name: %w", groupID, err)
	}
	return name, nil
}

// ChannelDeleteImpact summarizes the rows that will cascade-delete (or
// SET-NULL) when a channel is removed. Surfaced to operators on the delete
// confirmation dialog so they can see what they're about to wipe.
type ChannelDeleteImpact struct {
	RequestTypes           int `json:"request_types"`
	AssetReports           int `json:"asset_reports"`
	PortalCustomerChannels int `json:"portal_customer_channels"`
	EmailMessageTracking   int `json:"email_message_tracking"`
	EmailReplyOutbox       int `json:"email_reply_outbox"`
	EmailOAuthStates       int `json:"email_oauth_states"`
	EmailCredentialLeases  int `json:"email_credential_leases"`
	EmailProcessingLeases  int `json:"email_processing_leases"`
	EmailChannelState      int `json:"email_channel_state"`
	WebhookDeliveries      int `json:"webhook_deliveries"`
	ChannelManagers        int `json:"channel_managers"`
	PortalRequestDrafts    int `json:"portal_request_drafts"`
	PortalMagicLinks       int `json:"portal_magic_links"`
	PortalSessions         int `json:"portal_sessions"`
	Items                  int `json:"items"`
}

// GetDeleteImpact gathers row counts for the cascading-or-orphaning tables
// referenced by a channel. Any failed count aborts the preview so callers do
// not present a dangerously incomplete impact summary.
func (r *ChannelRepository) GetDeleteImpact(ctx context.Context, channelID int) (ChannelDeleteImpact, error) {
	var out ChannelDeleteImpact
	count := func(table, column string) (int, error) {
		var n int
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
		if err := r.db.QueryRowContext(ctx, q, channelID).Scan(&n); err != nil {
			return 0, fmt.Errorf("count %s for channel delete impact: %w", table, err)
		}
		return n, nil
	}
	counts := []struct {
		table  string
		target *int
	}{
		{table: "request_types", target: &out.RequestTypes},
		{table: "asset_reports", target: &out.AssetReports},
		{table: "portal_customer_channels", target: &out.PortalCustomerChannels},
		{table: "email_message_tracking", target: &out.EmailMessageTracking},
		{table: "email_reply_outbox", target: &out.EmailReplyOutbox},
		{table: "email_oauth_state", target: &out.EmailOAuthStates},
		{table: "email_credential_leases", target: &out.EmailCredentialLeases},
		{table: "email_processing_leases", target: &out.EmailProcessingLeases},
		{table: "email_channel_state", target: &out.EmailChannelState},
		{table: "webhook_deliveries", target: &out.WebhookDeliveries},
		{table: "channel_managers", target: &out.ChannelManagers},
		{table: "portal_request_drafts", target: &out.PortalRequestDrafts},
		{table: "portal_customer_magic_links", target: &out.PortalMagicLinks},
		{table: "portal_customer_sessions", target: &out.PortalSessions},
		{table: "items", target: &out.Items},
	}
	for _, item := range counts {
		value, err := count(item.table, "channel_id")
		if err != nil {
			return ChannelDeleteImpact{}, err
		}
		*item.target = value
	}
	return out, nil
}

// FindBadWorkspaceIDs delegates to WorkspaceRepository.FindMissingOrPersonal
// using the same db handle. Channel-config validation needs this from inside
// the channel handler, which doesn't otherwise hold a workspace repository
// reference. Keeping the indirection here avoids adding a setter on the
// handler for this single use.
func (r *ChannelRepository) FindBadWorkspaceIDs(ids []int) ([]int, error) {
	return NewWorkspaceRepository(r.db).FindMissingOrPersonal(ids)
}

// ChannelRequestTypeRoute contains the fields needed to validate a public
// request type against its channel's current workspace routing.
type ChannelRequestTypeRoute struct {
	ID          int
	Name        string
	ItemTypeID  int
	WorkspaceID *int
}

// ListRequestTypeRoutes returns every request type owned by a channel,
// including legacy NULL workspace routes that resolve to the channel's first
// configured workspace at runtime.
func (r *ChannelRepository) ListRequestTypeRoutes(channelID int) ([]ChannelRequestTypeRoute, error) {
	rows, err := r.db.Query(
		`SELECT id, name, item_type_id, workspace_id FROM request_types WHERE channel_id = ?`,
		channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("query request types for channel %d: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var routes []ChannelRequestTypeRoute
	for rows.Next() {
		var route ChannelRequestTypeRoute
		if err := rows.Scan(&route.ID, &route.Name, &route.ItemTypeID, &route.WorkspaceID); err != nil {
			return nil, fmt.Errorf("scan request type: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request types: %w", err)
	}
	return routes, nil
}

// SlugInUse reports whether another channel of the same type already
// uses the given slug. excludeChannelID is the row currently being edited and
// is ignored from the comparison. Config is decoded in Go instead of cast to
// JSON in SQL: a malformed legacy row must not make every later slug update
// fail (Postgres's text-to-jsonb cast and SQLite's json_extract both error).
func (r *ChannelRepository) SlugInUse(ctx context.Context, channelType, slug string, excludeChannelID int) (bool, error) {
	field := slugFieldFor(channelType)
	if field == "" {
		return false, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, '{}')
		FROM channels
		WHERE type = ? AND direction = 'inbound' AND id <> ?
	`, channelType, excludeChannelID)
	if err != nil {
		return false, fmt.Errorf("check slug uniqueness: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var channelID int
		var raw string
		if err := rows.Scan(&channelID, &raw); err != nil {
			return false, fmt.Errorf("scan slug candidate: %w", err)
		}
		var cfg models.ChannelConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			slog.Warn("ignoring malformed legacy channel config during slug uniqueness check",
				"channel_id", channelID, "channel_type", channelType, "error", err)
			continue
		}
		candidate := cfg.PortalSlug
		if field == "form_slug" {
			candidate = cfg.FormSlug
		}
		if candidate == slug {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate slug candidates: %w", err)
	}
	return false, nil
}

// slugFieldFor maps a channel type to the JSON config key that holds its
// public slug. Empty string for types that don't have a slug.
func slugFieldFor(channelType string) string {
	switch channelType {
	case "portal":
		return "portal_slug"
	case "form":
		return "form_slug"
	default:
		return ""
	}
}

func publicSlugForConfig(channelType, direction, config string) (string, error) {
	portalSlug, formSlug, err := configSlugValues(config)
	if err != nil {
		return "", err
	}
	if direction != "inbound" || (channelType != "portal" && channelType != "form") {
		return "", nil
	}
	if channelType == "portal" {
		return portalSlug, nil
	}
	return formSlug, nil
}

func configSlugValues(config string) (portalSlug, formSlug string, err error) {
	if strings.TrimSpace(config) == "" {
		config = "{}"
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(config), &object); err != nil {
		return "", "", err
	}
	if object == nil {
		return "", "", fmt.Errorf("channel configuration must be a JSON object")
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return "", "", err
	}
	return cfg.PortalSlug, cfg.FormSlug, nil
}

func nullableSlug(slug string) any {
	if slug == "" {
		return nil
	}
	return slug
}

func isPublicSlugConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "uq_channels_public_slug") ||
		strings.Contains(message, "channels.type, channels.public_slug")
}

// GroupExists reports whether an active row exists in the groups table for
// groupID. Inactive groups cannot confer channel-management authority and
// therefore cannot be assigned as effective managers.
// AddChannelManager uses this to fail fast with a clear 400 instead of
// relying on a deferred FK-violation string match (which differs between
// SQLite and Postgres).
func (r *ChannelRepository) GroupExists(ctx context.Context, groupID int) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM groups WHERE id = ? AND is_active = true`, groupID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check group exists: %w", err)
	}
	return n > 0, nil
}

// EmailChannelState is the row in email_channel_state for a single channel
// — a small bag of last-poll metadata used by the email-channel UI.
// LastCheckedAt is nullable; nil means "never polled".
type EmailChannelState struct {
	LastUID       int
	UIDValidity   uint32
	LastCheckedAt *time.Time
	ErrorCount    int
	LastError     string
}

// GetEmailChannelState returns the email_channel_state row for a channel.
// Returns ErrNotFound when no state row exists yet (caller treats that as
// "fresh channel" — empty state).
func (r *ChannelRepository) GetEmailChannelState(ctx context.Context, channelID int) (*EmailChannelState, error) {
	var state EmailChannelState
	var lastCheckedAt sql.NullTime
	var lastError sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT last_uid, COALESCE(uid_validity, 0), last_checked_at, error_count, last_error FROM email_channel_state WHERE channel_id = ?",
		channelID,
	).Scan(&state.LastUID, &state.UIDValidity, &lastCheckedAt, &state.ErrorCount, &lastError)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get email_channel_state for channel %d", channelID))
	}
	if lastCheckedAt.Valid {
		state.LastCheckedAt = &lastCheckedAt.Time
	}
	if lastError.Valid {
		state.LastError = lastError.String
	}
	return &state, nil
}

// EmailMessageRow is one row in the channel email log: the joined view
// across email_message_tracking + items + workspaces that the FE renders.
type EmailMessageRow struct {
	ID                  int
	FromEmail           string
	FromName            string
	Subject             string
	ItemID              *int
	CommentID           *int
	ProcessedAt         time.Time
	RateLimitedAt       *time.Time
	WorkspaceID         *int
	WorkspaceItemNumber int
	WorkspaceKey        string
}

// CountEmailMessages returns the number of email_message_tracking rows for a
// channel, optionally narrowed by a substring match against from_email,
// from_name, or subject.
func (r *ChannelRepository) CountEmailMessages(ctx context.Context, channelID int, search string) (int, error) {
	whereClause, args := emailMessageWhere(channelID, search)
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM email_message_tracking emt "+whereClause,
		args...,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("count email_message_tracking for channel %d: %w", channelID, err)
	}
	return total, nil
}

// ListEmailMessages returns a page of email log rows joined with items +
// workspaces, ordered most-recent-first.
func (r *ChannelRepository) ListEmailMessages(ctx context.Context, channelID int, search string, page, pageSize int) ([]EmailMessageRow, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	whereClause, args := emailMessageWhere(channelID, search)
	offset := (page - 1) * pageSize
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)

	rows, err := r.db.QueryContext(ctx,
		"SELECT emt.id, emt.from_email, emt.from_name, COALESCE(emt.subject, ''), emt.item_id, emt.comment_id, emt.processed_at, emt.rate_limited_at, i.workspace_item_number, i.workspace_id, w.key as workspace_key "+
			"FROM email_message_tracking emt "+
			"LEFT JOIN items i ON emt.item_id = i.id "+
			"LEFT JOIN workspaces w ON i.workspace_id = w.id "+
			whereClause+
			" ORDER BY emt.processed_at DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("list email_message_tracking for channel %d: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []EmailMessageRow
	for rows.Next() {
		var msg EmailMessageRow
		var itemID, commentID, workspaceID, workspaceItemNumber sql.NullInt64
		var fromName, workspaceKey sql.NullString
		var rateLimitedAt sql.NullTime
		if err := rows.Scan(&msg.ID, &msg.FromEmail, &fromName, &msg.Subject, &itemID, &commentID, &msg.ProcessedAt, &rateLimitedAt, &workspaceItemNumber, &workspaceID, &workspaceKey); err != nil {
			return nil, fmt.Errorf("scan email_message_tracking row: %w", err)
		}
		if fromName.Valid {
			msg.FromName = fromName.String
		}
		if rateLimitedAt.Valid {
			t := rateLimitedAt.Time
			msg.RateLimitedAt = &t
		}
		if itemID.Valid {
			v := int(itemID.Int64)
			msg.ItemID = &v
		}
		if commentID.Valid {
			v := int(commentID.Int64)
			msg.CommentID = &v
		}
		if workspaceItemNumber.Valid {
			msg.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		}
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			msg.WorkspaceID = &v
		}
		if workspaceKey.Valid {
			msg.WorkspaceKey = workspaceKey.String
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email_message_tracking rows: %w", err)
	}
	return out, nil
}

func emailMessageWhere(channelID int, search string) (whereClause string, args []any) {
	whereClause = "WHERE emt.channel_id = ?"
	args = []any{channelID}
	if search != "" {
		searchPattern := "%" + search + "%"
		whereClause += " AND (emt.from_email LIKE ? OR emt.from_name LIKE ? OR emt.subject LIKE ?)"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}
	return whereClause, args
}

// CountRateLimitedEmails returns how many tracking rows for a channel were
// declined by per-sender flood protection and are awaiting operator requeue.
func (r *ChannelRepository) CountRateLimitedEmails(ctx context.Context, channelID int) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM email_message_tracking WHERE channel_id = ? AND rate_limited_at IS NOT NULL",
		channelID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("count rate-limited email_message_tracking for channel %d: %w", channelID, err)
	}
	return total, nil
}

// GetEarliestRateLimitedUID returns the lowest IMAP UID among rate-limited
// messages recorded in the given UIDVALIDITY epoch. ok is false when there is
// nothing to requeue. UIDs from older epochs are ignored: they are meaningless
// in the current one, and an epoch change already resets the watermark to 0.
func (r *ChannelRepository) GetEarliestRateLimitedUID(ctx context.Context, channelID int, uidValidity uint32) (uid int, ok bool, err error) {
	var minUID sql.NullInt64
	err = r.db.QueryRowContext(ctx, `
		SELECT MIN(uid) FROM email_message_tracking
		WHERE channel_id = ? AND rate_limited_at IS NOT NULL AND uid_validity = ? AND uid > 0
	`, channelID, int64(uidValidity)).Scan(&minUID)
	if err != nil {
		return 0, false, fmt.Errorf("find earliest rate-limited email for channel %d: %w", channelID, err)
	}
	if !minUID.Valid {
		return 0, false, nil
	}
	return int(minUID.Int64), true, nil
}

// ResetEmailWatermarkToUID rewinds the channel's poll watermark so the next
// IMAP poll re-fetches from lastUID onward, and clears the poison-message
// tracker so a fresh retry starts unblocked. A channel that has never polled
// has no state row and nothing to rewind.
func (r *ChannelRepository) ResetEmailWatermarkToUID(ctx context.Context, channelID, lastUID int) error {
	if lastUID < 0 {
		lastUID = 0
	}
	if _, err := r.db.ExecWriteContext(ctx, `
		UPDATE email_channel_state
		SET last_uid = ?, failed_message_uid = 0, failed_message_uid_validity = 0,
		    failed_message_count = 0, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, lastUID, channelID); err != nil {
		return fmt.Errorf("rewind email watermark for channel %d: %w", channelID, err)
	}
	return nil
}

// EmailReplyOutboxStatus narrows the reply-outbox listing. The zero value
// lists everything the operator can still act on.
type EmailReplyOutboxStatus string

const (
	EmailReplyOutboxPending   EmailReplyOutboxStatus = "pending"
	EmailReplyOutboxDelivered EmailReplyOutboxStatus = "delivered"
	EmailReplyOutboxDiscarded EmailReplyOutboxStatus = "discarded"
	EmailReplyOutboxAll       EmailReplyOutboxStatus = "all"
)

// EmailReplyOutboxRow is one row of the outbound customer-reply queue as the
// channel UI renders it. Bodies are deliberately excluded — the queue shows
// envelope metadata and delivery state, never message content.
type EmailReplyOutboxRow struct {
	CommentID     int
	ItemID        int
	ToEmail       string
	ToName        string
	Subject       string
	AttemptCount  int
	NextAttemptAt sql.NullTime
	LastError     sql.NullString
	DeliveredAt   sql.NullTime
	DiscardedAt   sql.NullTime
	CreatedAt     time.Time
	WorkspaceID   *int
	WorkspaceKey  string
}

func emailReplyOutboxStatusWhere(status EmailReplyOutboxStatus) string {
	switch status {
	case EmailReplyOutboxPending:
		return " AND eo.delivered_at IS NULL AND eo.discarded_at IS NULL"
	case EmailReplyOutboxDelivered:
		return " AND eo.delivered_at IS NOT NULL"
	case EmailReplyOutboxDiscarded:
		return " AND eo.discarded_at IS NOT NULL"
	default: // EmailReplyOutboxAll
		return ""
	}
}

// CountEmailReplies returns how many email_reply_outbox rows a channel has
// for the given status filter.
func (r *ChannelRepository) CountEmailReplies(ctx context.Context, channelID int, status EmailReplyOutboxStatus) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM email_reply_outbox eo WHERE eo.channel_id = ?"+emailReplyOutboxStatusWhere(status),
		channelID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("count email_reply_outbox for channel %d: %w", channelID, err)
	}
	return total, nil
}

// ListEmailReplies returns a page of the channel's outbound reply queue,
// pending rows first (oldest attempt first), then delivered/discarded.
func (r *ChannelRepository) ListEmailReplies(ctx context.Context, channelID int, status EmailReplyOutboxStatus, page, pageSize int) ([]EmailReplyOutboxRow, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT eo.comment_id, eo.item_id, eo.to_email, eo.to_name, eo.subject, eo.attempt_count, eo.next_attempt_at, eo.last_error, eo.delivered_at, eo.discarded_at, eo.created_at, i.workspace_id, w.key "+
			"FROM email_reply_outbox eo "+
			"LEFT JOIN items i ON eo.item_id = i.id "+
			"LEFT JOIN workspaces w ON i.workspace_id = w.id "+
			"WHERE eo.channel_id = ?"+emailReplyOutboxStatusWhere(status)+
			" ORDER BY (eo.delivered_at IS NULL) DESC, eo.created_at DESC LIMIT ? OFFSET ?",
		channelID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("list email_reply_outbox for channel %d: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []EmailReplyOutboxRow
	for rows.Next() {
		var row EmailReplyOutboxRow
		var workspaceID sql.NullInt64
		var workspaceKey sql.NullString
		if err := rows.Scan(&row.CommentID, &row.ItemID, &row.ToEmail, &row.ToName, &row.Subject, &row.AttemptCount,
			&row.NextAttemptAt, &row.LastError, &row.DeliveredAt, &row.DiscardedAt, &row.CreatedAt, &workspaceID, &workspaceKey); err != nil {
			return nil, fmt.Errorf("scan email_reply_outbox row: %w", err)
		}
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			row.WorkspaceID = &v
		}
		if workspaceKey.Valid {
			row.WorkspaceKey = workspaceKey.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email_reply_outbox rows: %w", err)
	}
	return out, nil
}

// DiscardEmailReply marks a pending outbound reply as never-send. It reports
// whether a pending row was discarded; delivered, already-discarded, and
// missing rows report false so callers can distinguish idempotent success
// from a conflict.
func (r *ChannelRepository) DiscardEmailReply(ctx context.Context, channelID, commentID int) (bool, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE email_reply_outbox
		SET discarded_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ? AND comment_id = ?
		  AND delivered_at IS NULL AND discarded_at IS NULL
	`, channelID, commentID)
	if err != nil {
		return false, fmt.Errorf("discard email_reply_outbox row %d: %w", commentID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count discarded email_reply_outbox rows: %w", err)
	}
	return rows > 0, nil
}

// CreateOAuthState records an in-flight OAuth state for a channel-level
// OAuth flow. provider_id is NULL (the provider-flow uses non-zero).
// expiresAt is a hard cutoff the consume call enforces.
func (r *ChannelRepository) CreateOAuthState(ctx context.Context, state string, channelID, userID int, restoreChannelEnabled bool, expiresAt time.Time) error {
	if _, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, restore_channel_enabled, expires_at)
		VALUES (NULL, ?, ?, ?, ?, ?)
	`, channelID, state, userID, restoreChannelEnabled, expiresAt); err != nil {
		return fmt.Errorf("create email_oauth_state: %w", err)
	}
	return nil
}

// ConsumeOAuthState looks up an OAuth state row, returns its associated IDs,
// and deletes the row in one call. Returns ErrNotFound when the state is
// missing or already expired.
func (r *ChannelRepository) ConsumeOAuthState(ctx context.Context, state string, providerFlow bool) (providerID, channelID, userID int, restoreChannelEnabled bool, err error) {
	type oauthState struct {
		ProviderID            int
		ChannelID             int
		UserID                int
		RestoreChannelEnabled bool
	}
	providerPredicate := "provider_id IS NULL"
	if providerFlow {
		providerPredicate = "provider_id IS NOT NULL"
	}
	row, err := database.WithTxResult(r.db, func(tx database.Tx) (oauthState, error) {
		var out oauthState
		query := fmt.Sprintf(`
			DELETE FROM email_oauth_state
			WHERE state = ? AND expires_at > CURRENT_TIMESTAMP AND %s
			RETURNING COALESCE(provider_id, 0), channel_id, user_id, restore_channel_enabled
		`, providerPredicate)
		scanErr := tx.QueryRowContext(ctx, query, state).Scan(&out.ProviderID, &out.ChannelID, &out.UserID, &out.RestoreChannelEnabled)
		if scanErr != nil {
			return oauthState{}, notFoundOrWrap(scanErr, "consume email_oauth_state")
		}
		return out, nil
	})
	if err != nil {
		return 0, 0, 0, false, err
	}
	return row.ProviderID, row.ChannelID, row.UserID, row.RestoreChannelEnabled, nil
}

// ScrubChannelConfig removes sensitive fields from the configuration JSON
func ScrubChannelConfig(configJSON string) string {
	if configJSON == "" {
		return ""
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "{}"
	}
	if config == nil {
		return "{}"
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
		return "{}"
	}
	return string(scrubbed)
}
