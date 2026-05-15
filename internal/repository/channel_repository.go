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

// SlugCandidate is a minimal channel row used for slug matching from JSON config.
type SlugCandidate struct {
	Channel models.Channel
	Config  models.ChannelConfig
}

// ListSlugCandidates returns channels of a type with parsed config, newest first.
func (r *ChannelRepository) ListSlugCandidates(ctx context.Context, channelType string) ([]SlugCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = ?
		ORDER BY created_at DESC
	`, channelType)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s channels: %w", channelType, err)
	}
	defer func() { _ = rows.Close() }()

	candidates := []SlugCandidate{}
	for rows.Next() {
		var ch models.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Status); err != nil {
			slog.Warn("ListSlugCandidates scan failed", slog.String("component", "repository"), slog.String("channel_type", channelType), slog.Any("error", err))
			continue
		}
		var cfg models.ChannelConfig
		if ch.Config != "" {
			if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
				slog.Warn("ListSlugCandidates config unmarshal failed", slog.String("component", "repository"), slog.String("channel_type", channelType), slog.Int("channel_id", ch.ID), slog.Any("error", err))
				continue
			}
		}
		candidates = append(candidates, SlugCandidate{Channel: ch, Config: cfg})
	}
	return candidates, rows.Err()
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
	var args []interface{}

	baseSelect := `
		SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
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
			          SELECT group_id FROM group_members WHERE user_id = ?
			      ))
			  )
		)`)
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
			          SELECT group_id FROM group_members WHERE user_id = ?
			      ))
			  )
		)
	`, channelID, userID, userID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("failed to check channel manager: %w", err)
	}
	return found, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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

// AddManager adds a manager to a channel. ON CONFLICT DO NOTHING so re-adding
// an existing (channel, type, id) row is a no-op rather than an error.
func (r *ChannelRepository) AddManager(ctx context.Context, tx database.Tx, channelID int, managerType string, managerID, addedBy int) error {
	now := time.Now()
	_, err := tx.Exec(`
		INSERT INTO channel_managers (channel_id, manager_type, manager_id, added_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, channelID, managerType, managerID, addedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to add channel manager: %w", err)
	}
	return nil
}

// CountManagers returns the number of channel_managers rows for a channel.
// Callers use this to enforce the "can't remove the last manager" rule
// without racing against another concurrent remove (callers should hold the
// same Tx for the check + the delete).
func (r *ChannelRepository) CountManagers(ctx context.Context, tx database.Tx, channelID int) (int, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM channel_managers WHERE channel_id = ?`, channelID).Scan(&n)
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
	if errors.Is(err, sql.ErrNoRows) {
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

// UpdatePortalSections loads a channel's config JSON, hands it to mutator
// for in-place editing, and writes it back when mutator reports modified=true.
// Errors are swallowed (best-effort cleanup, matching prior handler behavior).
//
// This is the consolidated form of what request_type and asset_report
// handlers used to do separately: load config, walk PortalSections, mutate
// the type-specific ID slice (RequestTypeIDs / AssetReportIDs), and save.
// The mutator-callback shape lets each caller target the right field
// without each repo carrying its own copy of the load/save dance.
func (r *ChannelRepository) UpdatePortalSections(channelID int, mutator func(cfg *models.ChannelConfig) bool) {
	var configStr string
	err := r.db.QueryRow("SELECT config FROM channels WHERE id = ?", channelID).Scan(&configStr)
	if err != nil || configStr == "" {
		return
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return
	}
	if !mutator(&config) {
		return
	}
	updated, err := json.Marshal(config)
	if err != nil {
		return
	}
	_, _ = r.db.ExecWrite(
		"UPDATE channels SET config = ?, updated_at = ? WHERE id = ?",
		string(updated), time.Now(), channelID,
	)
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
	Items                  int `json:"items"`
}

// GetDeleteImpact gathers row counts for the cascading-or-orphaning tables
// referenced by a channel. Best-effort: a missing table (older schema) yields
// 0 rather than failing the whole call.
func (r *ChannelRepository) GetDeleteImpact(ctx context.Context, channelID int) (ChannelDeleteImpact, error) {
	var out ChannelDeleteImpact
	count := func(table, column string) int {
		var n int
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
		if err := r.db.QueryRowContext(ctx, q, channelID).Scan(&n); err != nil {
			// Treat unknown-table / missing-column errors as "no rows"
			// rather than failing the impact preview. The diagnostics UI
			// degrades gracefully.
			slog.Debug("delete-impact count failed", "table", table, "error", err)
			return 0
		}
		return n
	}
	out.RequestTypes = count("request_types", "channel_id")
	out.AssetReports = count("asset_reports", "channel_id")
	out.PortalCustomerChannels = count("portal_customer_channels", "channel_id")
	out.EmailMessageTracking = count("email_message_tracking", "channel_id")
	out.Items = count("items", "channel_id")
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

// SlugInUse reports whether another enabled channel of the same type already
// uses the given slug. excludeChannelID is the row currently being edited and
// is ignored from the comparison. JSON-path syntax differs between SQLite
// and Postgres; the field name is interpolated from a hardcoded mapping
// (slugFieldFor), never from user input, so the format string is safe.
func (r *ChannelRepository) SlugInUse(ctx context.Context, channelType, slug string, excludeChannelID int) (bool, error) {
	field := slugFieldFor(channelType)
	if field == "" {
		return false, nil
	}
	var jsonExpr string
	switch r.db.GetDriverName() {
	case "postgres":
		jsonExpr = fmt.Sprintf("config::jsonb ->> '%s'", field)
	default:
		jsonExpr = fmt.Sprintf("json_extract(config, '$.%s')", field)
	}
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM channels
			WHERE type = ? AND status = 'enabled' AND id <> ?
			  AND %s = ?
		)
	`, jsonExpr)
	var found bool
	err := r.db.QueryRowContext(ctx, query, channelType, excludeChannelID, slug).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check slug uniqueness: %w", err)
	}
	return found, nil
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

// GroupExists reports whether a row exists in the groups table for groupID.
// AddChannelManager uses this to fail fast with a clear 400 instead of
// relying on a deferred FK-violation string match (which differs between
// SQLite and Postgres).
func (r *ChannelRepository) GroupExists(ctx context.Context, groupID int) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM groups WHERE id = ?`, groupID,
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
		"SELECT last_uid, last_checked_at, error_count, last_error FROM email_channel_state WHERE channel_id = ?",
		channelID,
	).Scan(&state.LastUID, &lastCheckedAt, &state.ErrorCount, &lastError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get email_channel_state for channel %d: %w", channelID, err)
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
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)

	rows, err := r.db.QueryContext(ctx,
		"SELECT emt.id, emt.from_email, emt.from_name, emt.subject, emt.item_id, emt.comment_id, emt.processed_at, i.workspace_item_number, i.workspace_id, w.key as workspace_key "+
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
		if err := rows.Scan(&msg.ID, &msg.FromEmail, &fromName, &msg.Subject, &itemID, &commentID, &msg.ProcessedAt, &workspaceItemNumber, &workspaceID, &workspaceKey); err != nil {
			return nil, fmt.Errorf("scan email_message_tracking row: %w", err)
		}
		if fromName.Valid {
			msg.FromName = fromName.String
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

func emailMessageWhere(channelID int, search string) (whereClause string, args []interface{}) {
	whereClause = "WHERE emt.channel_id = ?"
	args = []interface{}{channelID}
	if search != "" {
		searchPattern := "%" + search + "%"
		whereClause += " AND (emt.from_email LIKE ? OR emt.from_name LIKE ? OR emt.subject LIKE ?)"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}
	return whereClause, args
}

// CreateOAuthState records an in-flight OAuth state for a channel-level
// OAuth flow. provider_id is fixed to 0 (the provider-flow uses non-zero).
// expiresAt is a hard cutoff the consume call enforces.
func (r *ChannelRepository) CreateOAuthState(ctx context.Context, state string, channelID, userID int, expiresAt time.Time) error {
	if _, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, expires_at)
		VALUES (0, ?, ?, ?, ?)
	`, channelID, state, userID, expiresAt); err != nil {
		return fmt.Errorf("create email_oauth_state: %w", err)
	}
	return nil
}

// ConsumeOAuthState looks up an OAuth state row, returns its associated IDs,
// and deletes the row in one call. Returns ErrNotFound when the state is
// missing or already expired.
func (r *ChannelRepository) ConsumeOAuthState(ctx context.Context, state string) (providerID, channelID, userID int, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT provider_id, channel_id, user_id
		FROM email_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &channelID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("get email_oauth_state: %w", err)
	}
	// Best-effort delete; the caller already has the data it needs.
	_, _ = r.db.ExecWriteContext(ctx, `DELETE FROM email_oauth_state WHERE state = ?`, state)
	return providerID, channelID, userID, nil
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
