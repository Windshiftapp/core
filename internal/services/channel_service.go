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
// INSERT OR IGNORE silently no-ops).
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
func (s *ChannelService) RemoveManager(ctx context.Context, id, channelID int) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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
