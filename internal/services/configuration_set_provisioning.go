package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// ConfigurationSetProvisioningNotifier is the optional notification-cache
// refresh hook (satisfied by the notification service), injected to avoid an
// import cycle.
type ConfigurationSetProvisioningNotifier interface {
	ForceRefreshCache() error
}

// ConfigurationSetProvisioningService owns the configuration-set provisioning
// flows shared by the admin browser surface and public API v2 (WI-1306):
// validation, sanitization, permission/notification cache refreshes, and audit.
type ConfigurationSetProvisioningService struct {
	db                  database.Database
	repo                *repository.ConfigurationSetRepository
	permissions         *PermissionService
	notificationService ConfigurationSetProvisioningNotifier
}

func NewConfigurationSetProvisioningService(db database.Database, repo *repository.ConfigurationSetRepository, permissions *PermissionService, notifier ConfigurationSetProvisioningNotifier) *ConfigurationSetProvisioningService {
	return &ConfigurationSetProvisioningService{db: db, repo: repo, permissions: permissions, notificationService: notifier}
}

// ConfigurationSetMutationResult carries the persisted set plus the cache
// warnings the HTTP surfaces render.
type ConfigurationSetMutationResult struct {
	Set      *models.ConfigurationSet
	Warnings []models.APIWarning
}

// List returns a page of configuration sets with their relations.
func (s *ConfigurationSetProvisioningService) List(page, limit int, search string) ([]models.ConfigurationSet, int, error) {
	return s.repo.List(page, limit, search)
}

// Get returns one configuration set with all relations.
func (s *ConfigurationSetProvisioningService) Get(id int) (*models.ConfigurationSet, error) {
	cs, err := s.repo.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, NewServiceError(404, "Configuration set not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get configuration set: %w", err)
	}
	return cs, nil
}

// Create validates and persists a configuration set with workspace
// assignments, refreshing dependent caches.
func (s *ConfigurationSetProvisioningService) Create(actor AuditActor, cs *models.ConfigurationSet) (*ConfigurationSetMutationResult, error) {
	// Configuration set Name + Description render in the admin
	// directory and the workspace assignment picker. The Create flow
	// uses the existing APIWarning channel for notification-cache
	// invalidation — keep sanitize warnings silent here; the contract
	// test pins the scrub.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &cs.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &cs.Description, Policy: sanitize.RichText},
	)

	if err := s.validateWorkspaceRefs(cs); err != nil {
		return nil, err
	}

	id, err := s.repo.CreateFull(cs)
	if err != nil {
		return nil, fmt.Errorf("create configuration set: %w", err)
	}
	configSetID := int(id)

	// Invalidate permission cache for affected workspaces
	if s.permissions != nil {
		_ = s.permissions.OnConfigurationSetChanged(configSetID)
	}

	warnings := s.refreshNotificationCache(configSetID)

	created, err := s.repo.FindByID(configSetID)
	if err != nil {
		return nil, fmt.Errorf("create configuration set: %w", err)
	}

	emitServiceAudit(s.db, actor, logger.ActionConfigSetCreate, logger.ResourceConfigurationSet, &configSetID, created.Name, map[string]any{
		"description":     created.Description,
		"workflow_id":     created.WorkflowID,
		"workspace_count": len(created.WorkspaceIDs),
	})

	return &ConfigurationSetMutationResult{Set: created, Warnings: warnings}, nil
}

// ImportTemplate sanitizes and applies a portable configuration-set template
// document — the programmatic (API v2) counterpart of the browser multipart
// import. Import creates a fresh set under one transaction; warnings carry
// the non-fatal reuse notes (same-named entities reused rather than created).
func (s *ConfigurationSetProvisioningService) ImportTemplate(ctx context.Context, actor AuditActor, tpl *ConfigSetTemplate) (*ConfigurationSetMutationResult, error) {
	if err := SanitizeConfigSetTemplate(tpl); err != nil {
		return nil, NewServiceError(400, err.Error())
	}

	importSvc := NewConfigSetImportService(s.db, s.repo)
	newID, warnings, err := importSvc.Import(ctx, tpl)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.FindByID(newID)
	if err != nil {
		return nil, fmt.Errorf("load imported configuration set: %w", err)
	}

	if s.permissions != nil {
		_ = s.permissions.OnConfigurationSetChanged(newID)
	}

	apiWarnings := make([]models.APIWarning, 0, len(warnings))
	for _, w := range warnings {
		apiWarnings = append(apiWarnings, models.APIWarning{
			Code:    "import_reuse",
			Message: w,
			Context: "configuration_set_import",
		})
	}

	emitServiceAudit(s.db, actor, logger.ActionConfigSetImport, logger.ResourceConfigurationSet, &newID, created.Name, map[string]any{
		"warning_count": len(warnings),
	})

	return &ConfigurationSetMutationResult{Set: created, Warnings: apiWarnings}, nil
}

// Delete removes a configuration set and its associations, invalidating
// dependent caches first.
func (s *ConfigurationSetProvisioningService) Delete(actor AuditActor, id int) error {
	cs, err := s.repo.FindByIDBasic(id)
	if errors.Is(err, repository.ErrNotFound) {
		return NewServiceError(404, "Configuration set not found")
	}
	if err != nil {
		return fmt.Errorf("delete configuration set: %w", err)
	}

	// Invalidate permission cache before deletion (while workspace associations still exist)
	if s.permissions != nil {
		_ = s.permissions.OnConfigurationSetChanged(id)
	}

	if err := s.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return NewServiceError(404, "Configuration set not found")
		}
		return fmt.Errorf("delete configuration set: %w", err)
	}

	emitServiceAudit(s.db, actor, logger.ActionConfigSetDelete, logger.ResourceConfigurationSet, &id, cs.Name, map[string]any{
		"description": cs.Description,
		"is_default":  cs.IsDefault,
	})
	return nil
}

func (s *ConfigurationSetProvisioningService) validateWorkspaceRefs(cs *models.ConfigurationSet) error {
	if strings.TrimSpace(cs.Name) == "" {
		return NewServiceError(400, "Configuration set name is required")
	}
	for _, workspaceID := range cs.WorkspaceIDs {
		exists, err := s.repo.WorkspaceExists(workspaceID)
		if err != nil || !exists {
			return NewServiceError(400, "One or more workspaces not found")
		}
	}
	return nil
}

func (s *ConfigurationSetProvisioningService) refreshNotificationCache(configSetID int) []models.APIWarning {
	var warnings []models.APIWarning
	if s.notificationService != nil {
		if err := s.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, models.APIWarning{
				Code:    "cache_invalidation_failed",
				Message: fmt.Sprintf("Failed to invalidate notification cache: %v", err),
				Context: fmt.Sprintf("configuration_set_id:%d", configSetID),
			})
		}
	}
	return warnings
}

// AuditExport records a configuration-set export (or a refusal to export the
// default set) so attempts to extract configuration are visible in the
// security log.
func (s *ConfigurationSetProvisioningService) AuditExport(actor AuditActor, configSetID int, success bool) {
	var errorMessage string
	if !success {
		errorMessage = "default_not_exportable"
	}
	resourceID := configSetID
	_ = logger.LogAudit(s.db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   logger.ActionConfigSetExport,
		ResourceType: logger.ResourceConfigurationSet,
		ResourceID:   &resourceID,
		Success:      success,
		ErrorMessage: errorMessage,
	})
}
