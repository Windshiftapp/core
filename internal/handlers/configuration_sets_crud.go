package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type ConfigurationSetHandler struct {
	db                  database.Database
	repo                *repository.ConfigurationSetRepository
	notificationService interface {
		ForceRefreshCache() error
	} // Notification service for cache refresh (optional, can be nil)
	permissionService *services.PermissionService
}

func NewConfigurationSetHandler(db database.Database, notificationService interface{ ForceRefreshCache() error }, permissionService *services.PermissionService) *ConfigurationSetHandler {
	return &ConfigurationSetHandler{
		db:                  db,
		repo:                repository.NewConfigurationSetRepository(db),
		notificationService: notificationService,
		permissionService:   permissionService,
	}
}

func (h *ConfigurationSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters
	page := 1
	limit := 10 // Default to 10 configuration sets per page

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Parse search parameter
	search := r.URL.Query().Get("search")

	// Use repository to fetch configuration sets with all relations
	configSets, totalCount, err := h.repo.List(page, limit, search)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create paginated response
	response := models.PaginatedConfigurationSetsResponse{
		ConfigurationSets: configSets,
		Pagination: models.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      totalCount,
			TotalPages: (totalCount + limit - 1) / limit,
		},
	}

	respondJSONOK(w, response)
}

func (h *ConfigurationSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Use repository to fetch configuration set with all relations
	cs, err := h.repo.FindByID(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "configuration_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cs)
}

func (h *ConfigurationSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cs models.ConfigurationSet
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(cs.Name) == "" {
		respondValidationError(w, r, "Configuration set name is required")
		return
	}

	// Verify workspaces exist
	for _, workspaceID := range cs.WorkspaceIDs {
		exists, err := h.repo.WorkspaceExists(workspaceID)
		if err != nil || !exists {
			respondValidationError(w, r, "One or more workspaces not found")
			return
		}
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Create the configuration set
	id, err := h.repo.Create(tx, &cs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	configSetID := int(id)

	// Save notification setting
	var notificationSettingID *int
	if cs.NotificationSettingID != nil {
		nsID := *cs.NotificationSettingID
		notificationSettingID = &nsID
	}
	if err = h.repo.SaveNotificationSetting(tx, configSetID, notificationSettingID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save workspace assignments
	if err = h.repo.SaveWorkspaceAssignments(tx, configSetID, cs.WorkspaceIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save screen assignments
	if err = h.repo.SaveScreenAssignments(tx, configSetID, cs.CreateScreenID, cs.EditScreenID, cs.ViewScreenID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save item type configurations
	if err = h.repo.SaveItemTypeConfigs(tx, configSetID, cs.ItemTypeConfigs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save priority assignments
	if err = h.repo.SavePriorityAssignments(tx, configSetID, cs.PriorityIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache for affected workspaces
	_ = h.permissionService.OnConfigurationSetChanged(configSetID)

	// Refresh notification cache if service is available
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err = h.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, createCacheWarning("notification", err, fmt.Sprintf("configuration_set_id:%d", id)))
		}
	}

	// Load and return the created configuration set with all relations
	createdCS, err := h.repo.FindByID(configSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetCreate,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &configSetID,
			ResourceName: createdCS.Name,
			Details: map[string]interface{}{
				"description":     createdCS.Description,
				"workflow_id":     createdCS.WorkflowID,
				"workspace_count": len(createdCS.WorkspaceIDs),
			},
			Success: true,
		})
	}

	respondJSONCreatedWithWarnings(w, createdCS, warnings)
}

func (h *ConfigurationSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the configuration set details for audit logging before deletion
	cs, err := h.repo.FindByIDBasic(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "configuration_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache before deletion (while workspace associations still exist)
	_ = h.permissionService.OnConfigurationSetChanged(id)

	// Delete the configuration set (including all associations)
	if err := h.repo.Delete(id); err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "configuration_set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetDelete,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &id,
			ResourceName: cs.Name,
			Details: map[string]interface{}{
				"description": cs.Description,
				"is_default":  cs.IsDefault,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
