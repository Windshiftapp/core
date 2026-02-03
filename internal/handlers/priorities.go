package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type PriorityHandler struct {
	db database.Database
}

func NewPriorityHandler(db database.Database) *PriorityHandler {
	return &PriorityHandler{db: db}
}

func (h *PriorityHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Base query for priorities
	query := `
		SELECT p.id, p.name, p.description, p.is_default,
		       p.icon, p.color, p.sort_order, p.created_at, p.updated_at
		FROM priorities p`

	args := []interface{}{}
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
	defer rows.Close()

	var priorities []models.Priority
	for rows.Next() {
		var p models.Priority
		err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.IsDefault,
			&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Load configuration set associations from junction table
		configSetQuery := `
			SELECT cs.id, cs.name
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
		for configSetRows.Next() {
			var configSetID int
			var configSetName string
			if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
				configSetRows.Close()
				respondInternalError(w, r, err)
				return
			}
			configSetIDs = append(configSetIDs, configSetID)
			configSetNames = append(configSetNames, configSetName)
		}
		configSetRows.Close()

		p.ConfigurationSetIDs = configSetIDs
		p.ConfigurationSetNames = configSetNames

		priorities = append(priorities, p)
	}

	if priorities == nil {
		priorities = []models.Priority{}
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
		SELECT id, name, description, is_default,
		       icon, color, sort_order, created_at, updated_at
		FROM priorities
		WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.IsDefault,
		&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "priority")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load configuration set associations from junction table
	configSetQuery := `
		SELECT cs.id, cs.name
		FROM configuration_set_priorities csp
		JOIN configuration_sets cs ON csp.configuration_set_id = cs.id
		WHERE csp.priority_id = ?
		ORDER BY cs.name`

	configSetRows, err := h.db.Query(configSetQuery, p.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer configSetRows.Close()

	var configSetIDs []int
	var configSetNames []string
	for configSetRows.Next() {
		var configSetID int
		var configSetName string
		if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
			respondInternalError(w, r, err)
			return
		}
		configSetIDs = append(configSetIDs, configSetID)
		configSetNames = append(configSetNames, configSetName)
	}

	p.ConfigurationSetIDs = configSetIDs
	p.ConfigurationSetNames = configSetNames

	respondJSONOK(w, p)
}

func (h *PriorityHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p models.Priority
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Validate required fields
	if strings.TrimSpace(p.Name) == "" {
		respondValidationError(w, r, "Priority name is required")
		return
	}

	configSetIDs := p.ConfigurationSetIDs

	// Verify all configuration sets exist (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			var configSetExists bool
			err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", csID).Scan(&configSetExists)
			if err != nil || !configSetExists {
				respondBadRequest(w, r, fmt.Sprintf("Configuration set %d not found", csID))
				return
			}
		}
	}

	// If this priority is being set as default, clear is_default on all others
	if p.IsDefault {
		_, err := h.db.ExecWrite("UPDATE priorities SET is_default = false WHERE is_default = true")
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("Failed to clear existing default: %w", err))
			return
		}
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO priorities (name, description, is_default, icon, color, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, p.Name, p.Description, p.IsDefault, p.Icon, p.Color, p.SortOrder, now, now).Scan(&id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			respondConflict(w, r, "Priority with this name already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Insert configuration set associations into junction table (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			_, err := h.db.Exec(`
				INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("Failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	}

	// Load and return the created priority with configuration sets
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default,
		       icon, color, sort_order, created_at, updated_at
		FROM priorities
		WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.IsDefault,
		&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ConfigurationSetIDs = configSetIDs

	// Load configuration set names (if any are provided)
	var configSetNames []string
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			var csName string
			err := h.db.QueryRow("SELECT name FROM configuration_sets WHERE id = ?", csID).Scan(&csName)
			if err == nil {
				configSetNames = append(configSetNames, csName)
			}
		}
	}
	p.ConfigurationSetNames = configSetNames

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "priority.create",
			ResourceType: "priority",
			ResourceID:   &p.ID,
			ResourceName: p.Name,
			Details: map[string]interface{}{
				"icon":       p.Icon,
				"color":      p.Color,
				"sort_order": p.SortOrder,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, p)
}

func (h *PriorityHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var p models.Priority
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Validate required fields
	if strings.TrimSpace(p.Name) == "" {
		respondValidationError(w, r, "Priority name is required")
		return
	}

	configSetIDs := p.ConfigurationSetIDs

	// Verify all configuration sets exist (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			var configSetExists bool
			err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", csID).Scan(&configSetExists)
			if err != nil || !configSetExists {
				respondBadRequest(w, r, fmt.Sprintf("Configuration set %d not found", csID))
				return
			}
		}
	}

	// If this priority is being set as default, clear is_default on all others (except this one)
	if p.IsDefault {
		_, err := h.db.ExecWrite("UPDATE priorities SET is_default = false WHERE is_default = true AND id != ?", id)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("Failed to clear existing default: %w", err))
			return
		}
	}

	// Update priority
	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE priorities
		SET name = ?, description = ?, is_default = ?, icon = ?, color = ?, sort_order = ?, updated_at = ?
		WHERE id = ?
	`, p.Name, p.Description, p.IsDefault, p.Icon, p.Color, p.SortOrder, now, id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
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
			respondInternalError(w, r, fmt.Errorf("Failed to update configuration set associations: %w", err))
			return
		}

		// Insert new associations
		for _, csID := range configSetIDs {
			_, err := h.db.ExecWrite(`
				INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("Failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	} else {
		// If no config sets provided, delete all existing associations
		_, err = h.db.ExecWrite("DELETE FROM configuration_set_priorities WHERE priority_id = ?", id)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("Failed to clear configuration set associations: %w", err))
			return
		}
	}

	// Load and return the updated priority
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default,
		       icon, color, sort_order, created_at, updated_at
		FROM priorities
		WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.IsDefault,
		&p.Icon, &p.Color, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ConfigurationSetIDs = configSetIDs

	// Load configuration set names (if any are provided)
	var configSetNames []string
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			var csName string
			err := h.db.QueryRow("SELECT name FROM configuration_sets WHERE id = ?", csID).Scan(&csName)
			if err == nil {
				configSetNames = append(configSetNames, csName)
			}
		}
	}
	p.ConfigurationSetNames = configSetNames

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "priority.update",
			ResourceType: "priority",
			ResourceID:   &p.ID,
			ResourceName: p.Name,
			Details: map[string]interface{}{
				"icon":       p.Icon,
				"color":      p.Color,
				"sort_order": p.SortOrder,
			},
			Success: true,
		})
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
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "priority")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if priority is in use
	var itemCount int
	err = h.db.QueryRow("SELECT COUNT(*) FROM items WHERE priority_id = ?", id).Scan(&itemCount)
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
		logger.LogAudit(h.db, logger.AuditEvent{
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
