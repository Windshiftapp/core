package services

import (
	"context"
	"fmt"

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

// GetBySlug retrieves a portal channel by its slug
func (s *ChannelService) GetBySlug(ctx context.Context, slug string) (*models.Channel, error) {
	return s.repo.FindBySlug(ctx, slug)
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

// SetDefault marks a channel as the default for its type
func (s *ChannelService) SetDefault(ctx context.Context, id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.SetDefault(ctx, tx, id); err != nil {
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

// AddManager adds a manager to a channel
func (s *ChannelService) AddManager(ctx context.Context, channelID int, managerType string, managerID int) error {
	if managerType != "user" && managerType != "group" {
		return fmt.Errorf("manager type must be 'user' or 'group'")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.AddManager(ctx, tx, channelID, managerType, managerID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RemoveManager removes a manager from a channel
func (s *ChannelService) RemoveManager(ctx context.Context, channelID int, managerType string, managerID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.RemoveManager(ctx, tx, channelID, managerType, managerID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// IsManager checks if a user is a manager of a channel
func (s *ChannelService) IsManager(ctx context.Context, channelID, userID int) (bool, error) {
	return s.repo.IsManager(ctx, channelID, userID)
}

// CanUserAccessChannel checks if a user can access a channel (is admin or manager)
func (s *ChannelService) CanUserAccessChannel(ctx context.Context, channelID, userID int) (bool, error) {
	// Check if user is admin
	isAdmin, err := s.permissionService.IsSystemAdmin(userID)
	if err == nil && isAdmin {
		return true, nil
	}

	// Check if user is manager
	return s.repo.IsManager(ctx, channelID, userID)
}
