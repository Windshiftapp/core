package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

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
	}
)

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
			return true, nil
		}
	}
	return s.repo.UserCanManage(ctx, userID, channelID)
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
	if req.Name == "" || req.Type == "" || req.Direction == "" {
		return nil, fmt.Errorf("name, type, and direction are required")
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
	if !ValidChannelStatuses[req.Status] {
		return nil, fmt.Errorf("%w: status %q", ErrInvalidChannelField, req.Status)
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := s.repo.Create(ctx, tx, channel)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	channel.ID = id
	// Scrub sensitive data before returning
	channel.Config = repository.ScrubChannelConfig(channel.Config)
	return channel, nil
}

// ChannelUpdateRequest contains data for updating a channel
type ChannelUpdateRequest struct {
	Name        string
	Description string
	Status      string
	IsDefault   bool
	Config      string
	CategoryID  *int
}

// Update updates an existing channel
func (s *ChannelService) Update(ctx context.Context, id int, req ChannelUpdateRequest) (*models.Channel, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidChannelField)
	}
	if req.Status != "" && !ValidChannelStatuses[req.Status] {
		return nil, fmt.Errorf("%w: status %q", ErrInvalidChannelField, req.Status)
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
		Config:      req.Config,
		CategoryID:  req.CategoryID,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Update(ctx, tx, channel); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated channel
	return s.repo.FindByID(ctx, id)
}

// Delete removes a channel
func (s *ChannelService) Delete(ctx context.Context, id int) error {
	// Check if channel is plugin-managed
	isPluginManaged, err := s.repo.IsPluginManaged(ctx, id)
	if err != nil {
		return err
	}
	if isPluginManaged {
		return fmt.Errorf("cannot delete plugin-managed channel")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.Delete(ctx, tx, id); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateLastActivity updates the last_activity timestamp
func (s *ChannelService) UpdateLastActivity(ctx context.Context, id int) error {
	return s.repo.UpdateLastActivity(ctx, id)
}

// SetStatus updates only the status column. Plugin-managed channels are
// rejected at the SQL level (plugin_name IS NULL), surfacing as ErrNotFound.
func (s *ChannelService) SetStatus(ctx context.Context, id int, status string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.SetStatus(ctx, tx, id, status); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UpdateConfig updates only the config column with caller-prepared JSON.
func (s *ChannelService) UpdateConfig(ctx context.Context, id int, config string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.UpdateConfig(ctx, tx, id, config); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.AddManager(ctx, tx, channelID, managerType, managerID, addedBy); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RemoveManager deletes a single channel_managers row by its primary key,
// scoped to channelID. Returns true if a row was removed, false if no row
// matched (caller should treat as 404).
//
// actorIsAdmin bypasses the last-manager guard so admins can still empty a
// channel's manager list (e.g. when archiving). Non-admin managers get
// ErrLastManager when their removal would drop the count to zero.
func (s *ChannelService) RemoveManager(ctx context.Context, id, channelID int, actorIsAdmin bool) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if !actorIsAdmin {
		count, err := s.repo.CountManagers(ctx, tx, channelID)
		if err != nil {
			return false, err
		}
		if count <= 1 {
			return false, ErrLastManager
		}
	}

	removed, err := s.repo.RemoveManager(ctx, tx, id, channelID)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return removed, nil
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
	}

	if existing, ok := cfg["portal_sections"].([]any); ok && len(existing) > 0 {
		return config, nil
	}

	cfg["portal_sections"] = []any{
		map[string]any{
			"id":               uuid.NewString(),
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
