package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

type PriorityHandler struct {
	db           database.Database
	translations *objecttranslation.Service
}

func (h *PriorityHandler) WithObjectTranslations(service *objecttranslation.Service) *PriorityHandler {
	h.translations = service
	return h
}

func NewPriorityHandler(db database.Database) *PriorityHandler {
	return &PriorityHandler{db: db}
}

func (h *PriorityHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Base query for priorities
	query := `
		SELECT p.id, COALESCE(p.builtin_key, ''), p.name, p.description, p.is_default,
		       p.icon, p.color, p.sort_order, p.created_at, p.updated_at
		FROM priorities p`

	args := []any{}
	whereClause := ""

	// Filter by configuration set if specified (via junction table)
	if configSetID := r.URL.Query().Get("configuration_set_id"); configSetID != "" {
		query += `
		INNER JOIN configuration_set_priorities csp ON p.id = csp.priority_id`
		whereClause = " WHERE csp.configuration_set_id = ?"
		args = append(args, configSetID)
	}

	query += whereClause + " ORDER BY p.sort_order, p.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var priorities []models.Priority
	for rows.Next() {
		var p models.Priority
		err := rows.Scan(&p.ID, &p.BuiltinKey, &p.Name, &p.Description, &p.IsDefault,
			&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Load configuration set associations from junction table
		configSetQuery := `
			SELECT cs.id, cs.name, COALESCE(cs.builtin_key, '')
			FROM configuration_set_priorities csp
			JOIN configuration_sets cs ON csp.configuration_set_id = cs.id
			WHERE csp.priority_id = ?
			ORDER BY cs.name`

		configSetRows, err := h.db.Query(configSetQuery, p.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		var configSetIDs []int
		var configSetNames []string
		var configSetBuiltinKeys []string
		for configSetRows.Next() {
			var configSetID int
			var configSetName, configSetBuiltinKey string
			if err := configSetRows.Scan(&configSetID, &configSetName, &configSetBuiltinKey); err != nil {
				_ = configSetRows.Close()
				respondInternalError(w, r, err)
				return
			}
			configSetIDs = append(configSetIDs, configSetID)
			configSetNames = append(configSetNames, configSetName)
			configSetBuiltinKeys = append(configSetBuiltinKeys, configSetBuiltinKey)
		}
		if err := configSetRows.Err(); err != nil {
			_ = configSetRows.Close()
			respondInternalError(w, r, err)
			return
		}
		_ = configSetRows.Close()

		p.ConfigurationSetIDs = configSetIDs
		p.ConfigurationSetNames = configSetNames
		p.ConfigurationSetBuiltinKeys = configSetBuiltinKeys

		priorities = append(priorities, p)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if priorities == nil {
		priorities = []models.Priority{}
	}
	if !localizeObjectResponse(w, r, h.translations, "priority", priorities) {
		return
	}

	respondJSONOK(w, priorities)
}

func (h *PriorityHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var p models.Priority
	err := h.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, is_default,
		       icon, color, sort_order, created_at, updated_at
		FROM priorities
		WHERE id = ?
	`, id).Scan(&p.ID, &p.BuiltinKey, &p.Name, &p.Description, &p.IsDefault,
		&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "priority")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load configuration set associations from junction table
	configSetQuery := `
		SELECT cs.id, cs.name, COALESCE(cs.builtin_key, '')
		FROM configuration_set_priorities csp
		JOIN configuration_sets cs ON csp.configuration_set_id = cs.id
		WHERE csp.priority_id = ?
		ORDER BY cs.name`

	configSetRows, err := h.db.Query(configSetQuery, p.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = configSetRows.Close() }()

	var configSetIDs []int
	var configSetNames []string
	var configSetBuiltinKeys []string
	for configSetRows.Next() {
		var configSetID int
		var configSetName, configSetBuiltinKey string
		if err := configSetRows.Scan(&configSetID, &configSetName, &configSetBuiltinKey); err != nil {
			respondInternalError(w, r, err)
			return
		}
		configSetIDs = append(configSetIDs, configSetID)
		configSetNames = append(configSetNames, configSetName)
		configSetBuiltinKeys = append(configSetBuiltinKeys, configSetBuiltinKey)
	}
	if err := configSetRows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ConfigurationSetIDs = configSetIDs
	p.ConfigurationSetNames = configSetNames
	p.ConfigurationSetBuiltinKeys = configSetBuiltinKeys
	if !localizeObjectResponse(w, r, h.translations, "priority", &p) {
		return
	}

	respondJSONOK(w, p)
}

// validatePriority checks required fields, config sets, is_default uniqueness, and name uniqueness.
// excludeID should be 0 for create operations or the priority ID for updates.
func (h *PriorityHandler) validatePriority(w http.ResponseWriter, r *http.Request, p models.Priority, excludeID int) bool {
	if strings.TrimSpace(p.Name) == "" {
		respondValidationError(w, r, "Priority name is required")
		return false
	}

	if len(p.ConfigurationSetIDs) > 0 && !h.validateConfigurationSets(w, r, p.ConfigurationSetIDs) {
		return false
	}

	if p.IsDefault {
		query := "UPDATE priorities SET is_default = false WHERE is_default = true"
		args := []any{}
		if excludeID > 0 {
			query += " AND id != ?"
			args = append(args, excludeID)
		}
		if _, err := h.db.ExecWrite(query, args...); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to clear existing default: %w", err))
			return false
		}
	}

	var nameExists bool
	var err error
	if excludeID > 0 {
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE name = ? AND id != ?)", p.Name, excludeID).Scan(&nameExists)
	} else {
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE name = ?)", p.Name).Scan(&nameExists)
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if nameExists {
		respondConflict(w, r, "Priority with this name already exists")
		return false
	}

	return true
}

// validateConfigurationSets verifies all provided configuration set IDs exist.
func (h *PriorityHandler) validateConfigurationSets(w http.ResponseWriter, r *http.Request, configSetIDs []int) bool {
	for _, csID := range configSetIDs {
		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", csID).Scan(&exists)
		if err != nil || !exists {
			respondBadRequest(w, r, fmt.Sprintf("Configuration set %d not found", csID))
			return false
		}
	}
	return true
}

// loadPriorityWithConfigSets loads a priority by ID and attaches configuration set info.
func (h *PriorityHandler) loadPriorityWithConfigSets(p *models.Priority, id int, configSetIDs []int) error {
	err := h.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, is_default,
		       icon, color, sort_order, created_at, updated_at
		FROM priorities
		WHERE id = ?
	`, id).Scan(&p.ID, &p.BuiltinKey, &p.Name, &p.Description, &p.IsDefault,
		&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return err
	}

	p.ConfigurationSetIDs = configSetIDs

	var configSetNames []string
	var configSetBuiltinKeys []string
	for _, csID := range configSetIDs {
		var csName, csBuiltinKey string
		if err := h.db.QueryRow(
			"SELECT name, COALESCE(builtin_key, '') FROM configuration_sets WHERE id = ?",
			csID,
		).Scan(&csName, &csBuiltinKey); err == nil {
			configSetNames = append(configSetNames, csName)
			configSetBuiltinKeys = append(configSetBuiltinKeys, csBuiltinKey)
		}
	}
	p.ConfigurationSetNames = configSetNames
	p.ConfigurationSetBuiltinKeys = configSetBuiltinKeys

	return nil
}

func (h *PriorityHandler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := decodeJSON[models.Priority](w, r)
	if !ok {
		return
	}

	if !h.validatePriority(w, r, p, 0) {
		return
	}

	configSetIDs := p.ConfigurationSetIDs
	p.Name = sanitize.PlainTextField.Sanitize(p.Name)
	p.Description = sanitize.Comment.Sanitize(p.Description)

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO priorities (name, description, is_default, icon, color, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, p.Name, p.Description, p.IsDefault, p.Icon, p.Color, p.SortOrder, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Priority with this name already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Insert configuration set associations into junction table (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			_, err = h.db.ExecWrite(`
				INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	}

	// Load and return the created priority with configuration sets
	if err = h.loadPriorityWithConfigSets(&p, int(id), configSetIDs); err != nil {
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
			ActionType:   "priority.create",
			ResourceType: "priority",
			ResourceID:   &p.ID,
			ResourceName: p.Name,
			Details: map[string]any{
				"icon":       p.Icon,
				"color":      p.Color,
				"sort_order": p.SortOrder,
			},
			Success: true,
		})
	}

	if !localizeObjectResponse(w, r, h.translations, "priority", &p) {
		return
	}
	respondJSONCreated(w, p)
}

func (h *PriorityHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	p, ok := decodeJSON[models.Priority](w, r)
	if !ok {
		return
	}

	if !h.validatePriority(w, r, p, id) {
		return
	}

	configSetIDs := p.ConfigurationSetIDs
	p.Name = sanitize.PlainTextField.Sanitize(p.Name)
	p.Description = sanitize.Comment.Sanitize(p.Description)

	// Update priority
	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE priorities
		SET name = ?, description = ?, is_default = ?, icon = ?, color = ?, sort_order = ?, updated_at = ?
		WHERE id = ?
	`, p.Name, p.Description, p.IsDefault, p.Icon, p.Color, p.SortOrder, now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Priority with this name already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Update configuration set associations (if any are provided)
	if len(configSetIDs) > 0 {
		// Delete existing associations
		_, err = h.db.ExecWrite("DELETE FROM configuration_set_priorities WHERE priority_id = ?", id)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to update configuration set associations: %w", err))
			return
		}

		// Insert new associations
		for _, csID := range configSetIDs {
			_, err = h.db.ExecWrite(`
				INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	} else {
		// If no config sets provided, delete all existing associations
		_, err = h.db.ExecWrite("DELETE FROM configuration_set_priorities WHERE priority_id = ?", id)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to clear configuration set associations: %w", err))
			return
		}
	}

	// Load and return the updated priority
	if err = h.loadPriorityWithConfigSets(&p, id, configSetIDs); err != nil {
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
			ActionType:   "priority.update",
			ResourceType: "priority",
			ResourceID:   &p.ID,
			ResourceName: p.Name,
			Details: map[string]any{
				"icon":       p.Icon,
				"color":      p.Color,
				"sort_order": p.SortOrder,
			},
			Success: true,
		})
	}

	if !localizeObjectResponse(w, r, h.translations, "priority", &p) {
		return
	}
	respondJSONOK(w, p)
}

func (h *PriorityHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get priority details for audit log before deletion
	var priorityName string
	err := h.db.QueryRow("SELECT name FROM priorities WHERE id = ?", id).Scan(&priorityName)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "priority")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if priority is in use
	itemCount, err := repository.NewItemRepository(h.db).CountByField("priority_id", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if itemCount > 0 {
		respondConflict(w, r, fmt.Sprintf("Cannot delete priority: it is used by %d item(s)", itemCount))
		return
	}

	// Delete priority (cascade will handle junction table)
	_, err = h.db.ExecWrite("DELETE FROM priorities WHERE id = ?", id)
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
			ActionType:   "priority.delete",
			ResourceType: "priority",
			ResourceID:   &id,
			ResourceName: priorityName,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
