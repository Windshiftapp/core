package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"uuid"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// Valid channel attribute values. Kept as exported sets so handlers and tests
// can reuse them. The DB schema is permissive for back-compat with installs
// predating these constants, but Create/Update reject anything outside the
// list to keep schedulers and webhook dispatch from seeing surprise types.
var (
	ValidChannelTypes = map[string]bool{
		"smtp":    true,
		"webhook": true,
		"email":   true,
		"portal":  true,
		"form":    true,
		"widget":  true,
		"imap":    true,
	}
	ValidChannelDirections = map[string]bool{
		"inbound":  true,
		"outbound": true,
	}
	ValidChannelStatuses = map[string]bool{
		"enabled":  true,
		"disabled": true,
		// Seeded default notification channels start as pending until first
		// configured. Treat pending as a valid persisted state so metadata/config
		// updates can run before ToggleChannel promotes the channel to enabled.
		"pending": true,
	}
)

var requiredChannelDirection = map[string]string{
	"smtp":    "outbound",
	"webhook": "outbound",
	"email":   "inbound",
	"portal":  "inbound",
	"form":    "inbound",
	"imap":    "inbound",
	"widget":  "inbound",
}

// ErrInvalidChannelField is returned by Create/Update when a caller supplies
// an unknown type/direction/status, or an empty name. The handler maps it to
// a 400 with the wrapped message.
var ErrInvalidChannelField = fmt.Errorf("invalid channel field")

// ErrLastManager is returned by RemoveManager when removing the targeted row
// would drop the channel's manager count to zero and the caller isn't a
// system admin. Without this guard a manager can self-evict and leave the
// channel manageable only by admins, which is rarely the operator's intent.
var ErrLastManager = fmt.Errorf("cannot remove the last channel manager")

// ChannelService handles channel business logic
type ChannelService struct {
	db                database.Database
	repo              *repository.ChannelRepository
	permissionService *PermissionService
}

// NewChannelService creates a new channel service
func NewChannelService(db database.Database, permService *PermissionService) *ChannelService {
	return &ChannelService{
		db:                db,
		repo:              repository.NewChannelRepository(db),
		permissionService: permService,
	}
}

// ChannelListFilters contains filter parameters for listing channels
type ChannelListFilters struct {
	CategoryID      *int
	Type            string
	Direction       string
	Status          string
	IncludeDisabled bool
}

// List retrieves channels visible to the user
func (s *ChannelService) List(ctx context.Context, userID int, filters ChannelListFilters) ([]models.Channel, error) {
	// Check if user is admin
	isAdmin, err := s.permissionService.IsSystemAdmin(userID)
	if err != nil {
		isAdmin = false
	}

	return s.repo.FindAll(ctx, userID, isAdmin, repository.ChannelListFilters{
		CategoryID:      filters.CategoryID,
		Type:            filters.Type,
		Direction:       filters.Direction,
		Status:          filters.Status,
		IncludeDisabled: filters.IncludeDisabled,
	})
}

// GetByID retrieves a single channel
func (s *ChannelService) GetByID(ctx context.Context, id int) (*models.Channel, error) {
	return s.repo.FindByID(ctx, id)
}

// UserCanManage returns true if the user is a system admin, or a direct /
// group-assigned manager of the channel. Use this whenever a channel mutation
// (e.g. config update, manual webhook trigger) needs to be gated by manager
// scope rather than by item-view scope.
func (s *ChannelService) UserCanManage(ctx context.Context, userID, channelID int) (bool, error) {
	if s.permissionService != nil {
		if isAdmin, err := s.permissionService.IsSystemAdmin(userID); err == nil && isAdmin {
			// Preserve existence-hiding semantics for non-admins while ensuring
			// admins still receive a clean 404 for a channel that does not exist.
			return s.repo.Exists(ctx, channelID)
		}
	}
	return s.repo.UserCanManage(ctx, userID, channelID)
}

// ManagesChannels reports whether the user should see the channel-management
// surface. System admins always can; other users need a direct or active-group
// assignment to at least one non-default channel.
func (s *ChannelService) ManagesChannels(ctx context.Context, userID int) (bool, error) {
	if s.permissionService != nil {
		isAdmin, err := s.permissionService.IsSystemAdminContext(ctx, userID)
		if err != nil {
			return false, fmt.Errorf("check system administrator: %w", err)
		}
		if isAdmin {
			return true, nil
		}
	}
	return s.repo.UserManagesAny(ctx, userID)
}

// UserIsSystemAdmin exposes the shared permission decision to channel-adjacent
// handlers that must re-authorize an unauthenticated callback state.
func (s *ChannelService) UserIsSystemAdmin(userID int) (bool, error) {
	if s.permissionService == nil {
		return false, nil
	}
	return s.permissionService.IsSystemAdmin(userID)
}

// ItemTypeAllowedInWorkspace verifies both that the item type exists and that
// the workspace's configuration set permits it.
func (s *ChannelService) ItemTypeAllowedInWorkspace(workspaceID, itemTypeID int) (bool, error) {
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", itemTypeID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	return IsItemTypeAllowedInWorkspace(s.db, workspaceID, itemTypeID)
}

// PriorityAllowedInWorkspace verifies both that the priority exists and that
// the target workspace's configuration set permits it.
func (s *ChannelService) PriorityAllowedInWorkspace(workspaceID, priorityID int) (bool, error) {
	return IsPriorityAllowedInWorkspace(s.db, workspaceID, priorityID)
}

// UserCanConnectWorkspace requires the administrative workspace grant used
// when a channel or request type is wired to accept external submissions.
func (s *ChannelService) UserCanConnectWorkspace(userID, workspaceID int) (bool, error) {
	if s.permissionService == nil {
		return false, nil
	}
	return s.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
}

// ChannelCreateRequest contains data for creating a channel
type ChannelCreateRequest struct {
	Name        string
	Type        string
	Direction   string
	Description string
	Status      string
	IsDefault   bool
	Config      string
	CategoryID  *int
}

// Create creates a new channel
func (s *ChannelService) Create(ctx context.Context, req ChannelCreateRequest) (*models.Channel, error) {
	if err := s.checkChannelPolicy(req.Type, req.Config); err != nil {
		return nil, err
	}
	if req.Name == "" || req.Type == "" || req.Direction == "" {
		return nil, fmt.Errorf("name, type, and direction are required")
	}
	// Default channels are seeded system infrastructure. Letting the generic
	// create endpoint make one would atomically demote the working default and
	// replace it with the intentionally-disabled, not-yet-configured row below.
	// There is no public default-promotion workflow, so fail closed here as well
	// as in the HTTP handler.
	if req.IsDefault {
		return nil, fmt.Errorf("%w: default channels cannot be created through this endpoint", ErrInvalidChannelField)
	}

	if req.Status == "" {
		req.Status = "disabled"
	}

	if !ValidChannelTypes[req.Type] {
		return nil, fmt.Errorf("%w: type %q", ErrInvalidChannelField, req.Type)
	}
	if !ValidChannelDirections[req.Direction] {
		return nil, fmt.Errorf("%w: direction %q", ErrInvalidChannelField, req.Direction)
	}
	if required := requiredChannelDirection[req.Type]; required != "" && req.Direction != required {
		return nil, fmt.Errorf("%w: %s channels must be %s", ErrInvalidChannelField, req.Type, required)
	}
	if !ValidChannelStatuses[req.Status] {
		return nil, fmt.Errorf("%w: status %q", ErrInvalidChannelField, req.Status)
	}

	// Newly created integrations are configured in a second UI step and must
	// never begin dispatching/accepting traffic with an incomplete config.
	req.Status = "disabled"
	if strings.TrimSpace(req.Config) == "" {
		req.Config = "{}"
	}

	if req.Type == "portal" {
		cfg, err := ensureDefaultPortalSection(req.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid portal config: %w", err)
		}
		req.Config = cfg
	}

	channel := &models.Channel{
		Name:        req.Name,
		Type:        req.Type,
		Direction:   req.Direction,
		Description: req.Description,
		Status:      req.Status,
		IsDefault:   req.IsDefault,
		Config:      req.Config,
		CategoryID:  req.CategoryID,
	}

	id, err := database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		return s.repo.Create(ctx, tx, channel)
	})
	if err != nil {
		return nil, err
	}

	channel.ID = id
	// Portal channels get their system Email request type up front so admins can
	// see and edit its item type before the first email arrives. Best-effort: a
	// portal without served workspaces is provisioned on a later config save, and
	// email intake heals a missing row lazily.
	if req.Type == "portal" {
		if _, err := EnsureEmailRequestType(s.db, id, 0, nil); err != nil {
			slog.Warn("failed to provision portal email request type",
				"channel_id", id, "error", err)
		}
	}
	// Scrub sensitive data before returning
	repository.ScrubChannelForResponse(channel)
	return channel, nil
}

// ChannelUpdateRequest contains data for updating a channel
type ChannelUpdateRequest struct {
	Name        string
	Description string
	Status      string
	IsDefault   bool
	CategoryID  *int
}

// Update updates an existing channel
func (s *ChannelService) Update(ctx context.Context, id int, req ChannelUpdateRequest) (*models.Channel, error) {
	if err := s.CheckTransportMutation(ctx, id); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidChannelField)
	}
	// Check if channel is plugin-managed
	isPluginManaged, err := s.repo.IsPluginManaged(ctx, id)
	if err != nil {
		return nil, err
	}
	if isPluginManaged {
		return nil, fmt.Errorf("cannot modify plugin-managed channel")
	}
	channel := &models.Channel{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		IsDefault:   req.IsDefault,
		CategoryID:  req.CategoryID,
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Update(ctx, tx, channel)
	}); err != nil {
		return nil, err
	}

	// Fetch updated channel
	return s.repo.FindByID(ctx, id)
}

// Delete removes a channel
func (s *ChannelService) Delete(ctx context.Context, id int) error {
	if err := s.CheckTransportMutation(ctx, id); err != nil {
		return err
	}
	// Check if channel is plugin-managed
	isPluginManaged, err := s.repo.IsPluginManaged(ctx, id)
	if err != nil {
		return err
	}
	if isPluginManaged {
		return fmt.Errorf("cannot delete plugin-managed channel")
	}

	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.Delete(ctx, tx, id)
	})
}

// UpdateLastActivity updates the last_activity timestamp
func (s *ChannelService) UpdateLastActivity(ctx context.Context, id int) error {
	return s.repo.UpdateLastActivity(ctx, id)
}

// SetStatus updates only the status column. Plugin-managed channels are
// rejected at the SQL level (plugin_name IS NULL), surfacing as ErrNotFound.
func (s *ChannelService) SetStatus(ctx context.Context, id int, status string) error {
	if err := s.CheckTransportMutation(ctx, id); err != nil {
		return err
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.SetStatus(ctx, tx, id, status)
	})
}

func (s *ChannelService) SetStatusIfConfigUnchanged(ctx context.Context, id int, status, expectedConfig string) (bool, error) {
	if err := s.CheckTransportMutation(ctx, id); err != nil {
		return false, err
	}
	return database.WithTxResult(s.db, func(tx database.Tx) (bool, error) {
		return s.repo.SetStatusIfConfigUnchanged(ctx, tx, id, status, expectedConfig)
	})
}

// UpdateConfig updates only the config column with caller-prepared JSON.
func (s *ChannelService) UpdateConfig(ctx context.Context, id int, config string) error {
	if err := s.CheckTransportConfig(ctx, id, config); err != nil {
		return err
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.UpdateConfig(ctx, tx, id, config)
	})
}

// UpdateConfigIfUnchanged avoids losing a concurrent channel-config edit.
func (s *ChannelService) UpdateConfigIfUnchanged(ctx context.Context, id int, expectedConfig, expectedStatus, config string) (bool, error) {
	if err := s.CheckTransportConfig(ctx, id, config); err != nil {
		return false, err
	}
	return database.WithTxResult(s.db, func(tx database.Tx) (bool, error) {
		return s.repo.UpdateConfigIfUnchanged(ctx, tx, id, expectedConfig, expectedStatus, config)
	})
}

// Exists checks if a channel exists
func (s *ChannelService) Exists(ctx context.Context, id int) (bool, error) {
	return s.repo.Exists(ctx, id)
}

// IsPluginManaged checks if a channel is managed by a plugin
func (s *ChannelService) IsPluginManaged(ctx context.Context, id int) (bool, error) {
	return s.repo.IsPluginManaged(ctx, id)
}

// GetConfig retrieves the raw config for a channel (for internal use)
func (s *ChannelService) GetConfig(ctx context.Context, id int) (string, error) {
	return s.repo.GetConfig(ctx, id)
}

// Channel Manager methods

// GetManagers returns all managers for a channel
func (s *ChannelService) GetManagers(ctx context.Context, channelID int) ([]models.ChannelManager, error) {
	return s.repo.FindManagers(ctx, channelID)
}

// AddManager adds a manager to a channel. Returns nil on success, including
// the case where the (channel, type, id) row already exists (the underlying
// ON CONFLICT DO NOTHING silently no-ops).
func (s *ChannelService) AddManager(ctx context.Context, channelID int, managerType string, managerID, addedBy int) error {
	if managerType != "user" && managerType != "group" {
		return fmt.Errorf("manager type must be 'user' or 'group'")
	}

	_, err := database.WithTxResult(s.db, func(tx database.Tx) (bool, error) {
		return s.repo.AddManager(ctx, tx, channelID, managerType, managerID, addedBy)
	})
	return err
}

// AddManagers inserts a validated batch atomically and returns only IDs whose
// manager rows were newly created.
func (s *ChannelService) AddManagers(ctx context.Context, channelID int, managerType string, managerIDs []int, addedBy int) ([]int, error) {
	if managerType != "user" && managerType != "group" {
		return nil, fmt.Errorf("manager type must be 'user' or 'group'")
	}
	return database.WithTxResult(s.db, func(tx database.Tx) ([]int, error) {
		if err := s.repo.LockManagerSet(ctx, tx, channelID); err != nil {
			return nil, err
		}
		inserted := make([]int, 0, len(managerIDs))
		for _, managerID := range managerIDs {
			created, err := s.repo.AddManager(ctx, tx, channelID, managerType, managerID, addedBy)
			if err != nil {
				return nil, err
			}
			if created {
				inserted = append(inserted, managerID)
			}
		}
		return inserted, nil
	})
}

// RemoveManager deletes a single channel_managers row by its primary key,
// scoped to channelID. Returns true if a row was removed, false if no row
// matched (caller should treat as 404).
//
// actorIsAdmin bypasses the last-manager guard so admins can still empty a
// channel's manager list (e.g. when archiving). Non-admin managers get
// ErrLastManager when their removal would drop the count to zero.
func (s *ChannelService) RemoveManager(ctx context.Context, id, channelID int, actorIsAdmin bool) (bool, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (bool, error) {
		if err := s.repo.LockManagerSet(ctx, tx, channelID); err != nil {
			return false, err
		}
		removed, err := s.repo.RemoveManager(ctx, tx, id, channelID)
		if err != nil || !removed || actorIsAdmin {
			return removed, err
		}

		// Count after the tentative delete. If no active manager remains, the
		// returned error rolls the transaction back; deleting a stale inactive
		// assignment is still allowed when a real manager remains.
		count, err := s.repo.CountManagers(ctx, tx, channelID)
		if err != nil {
			return false, err
		}
		if count == 0 {
			return false, ErrLastManager
		}
		return true, nil
	})
}

// LookupManagerRow returns the (manager_type, manager_id) for one
// channel_managers row. Handlers use this to populate audit context.
func (s *ChannelService) LookupManagerRow(ctx context.Context, id, channelID int) (managerType string, managerID int, err error) {
	return s.repo.FindManagerRow(ctx, id, channelID)
}

// ensureDefaultPortalSection guarantees a newly-created portal channel has at
// least one section in its config so admins can drop request types in
// immediately, without first clicking "Add Section". An existing non-empty
// portal_sections array is left untouched.
func ensureDefaultPortalSection(config string) (string, error) {
	cfg := map[string]any{}
	if config != "" {
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return "", err
		}
		if cfg == nil {
			return "", fmt.Errorf("channel configuration must be a JSON object")
		}
	}

	changed := false
	if _, ok := cfg["portal_gradient"]; !ok {
		cfg["portal_gradient"] = 1
		changed = true
	}

	if existing, ok := cfg["portal_sections"].([]any); ok && len(existing) > 0 {
		if !changed {
			return config, nil
		}
		out, err := json.Marshal(cfg)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}

	cfg["portal_sections"] = []any{
		map[string]any{
			"id":               uuid.New().String(),
			"title":            "",
			"subtitle":         "",
			"display_order":    0,
			"request_type_ids": []int{},
			"asset_report_ids": []int{},
		},
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
