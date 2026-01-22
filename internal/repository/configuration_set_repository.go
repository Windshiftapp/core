package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// ConfigurationSetRepository provides data access methods for configuration sets
type ConfigurationSetRepository struct {
	db database.Database
}

// NewConfigurationSetRepository creates a new configuration set repository
func NewConfigurationSetRepository(db database.Database) *ConfigurationSetRepository {
	return &ConfigurationSetRepository{db: db}
}

// Subquery for notification setting columns
const notificationSettingSubquery = `
	(
		SELECT csns.notification_setting_id
		FROM configuration_set_notification_settings csns
		WHERE csns.configuration_set_id = cs.id
		ORDER BY csns.created_at DESC
		LIMIT 1
	) AS notification_setting_id,
	(
		SELECT ns.name
		FROM configuration_set_notification_settings csns2
		JOIN notification_settings ns ON ns.id = csns2.notification_setting_id
		WHERE csns2.configuration_set_id = cs.id
		ORDER BY csns2.created_at DESC
		LIMIT 1
	) AS notification_setting_name`

// FindByID loads a configuration set by ID with all related data
func (r *ConfigurationSetRepository) FindByID(id int) (*models.ConfigurationSet, error) {
	cs, err := r.findByIDBasic(id)
	if err != nil {
		return nil, err
	}

	if err := r.loadRelations(cs); err != nil {
		return nil, err
	}

	return cs, nil
}

// FindByIDBasic loads a configuration set by ID without related data
func (r *ConfigurationSetRepository) FindByIDBasic(id int) (*models.ConfigurationSet, error) {
	return r.findByIDBasic(id)
}

// findByIDBasic is the internal implementation
func (r *ConfigurationSetRepository) findByIDBasic(id int) (*models.ConfigurationSet, error) {
	var cs models.ConfigurationSet
	var workflowName sql.NullString
	var workflowID sql.NullInt64
	var defaultItemTypeID sql.NullInt64
	var notificationSettingID sql.NullInt64
	var notificationSettingName sql.NullString
	var defaultItemTypeName sql.NullString

	query := fmt.Sprintf(`
		SELECT cs.id, cs.name, cs.description, cs.is_default, cs.differentiate_by_item_type, cs.workflow_id,
		       cs.default_item_type_id,
		       %s,
		       cs.created_at, cs.updated_at,
		       wf.name as workflow_name,
		       dit.name as default_item_type_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		LEFT JOIN item_types dit ON cs.default_item_type_id = dit.id
		WHERE cs.id = ?
	`, notificationSettingSubquery)

	err := r.db.QueryRow(query, id).Scan(
		&cs.ID, &cs.Name, &cs.Description,
		&cs.IsDefault, &cs.DifferentiateByItemType, &workflowID, &defaultItemTypeID,
		&notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt,
		&workflowName, &defaultItemTypeName,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find configuration set: %w", err)
	}

	cs.WorkflowName = workflowName.String
	cs.NotificationSettingName = notificationSettingName.String
	cs.DefaultItemTypeName = defaultItemTypeName.String
	cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
	cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)
	cs.DefaultItemTypeID = utils.NullInt64ToPtr(defaultItemTypeID)

	return &cs, nil
}

// List returns a paginated list of configuration sets
func (r *ConfigurationSetRepository) List(page, limit int, search string) ([]models.ConfigurationSet, int, error) {
	// Build WHERE clause for search
	whereClause := ""
	args := []interface{}{}
	if search != "" {
		whereClause = "WHERE LOWER(cs.name) LIKE ?"
		args = append(args, "%"+strings.ToLower(search)+"%")
	}

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM configuration_sets cs
		%s`, whereClause)

	var totalCount int
	if err := r.db.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count configuration sets: %w", err)
	}

	// Build data query with pagination
	offset := (page - 1) * limit
	query := fmt.Sprintf(`
		SELECT cs.id, cs.name, cs.description, cs.is_default, cs.differentiate_by_item_type, cs.workflow_id,
		       cs.default_item_type_id,
		       %s,
		       cs.created_at, cs.updated_at,
		       wf.name as workflow_name,
		       dit.name as default_item_type_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		LEFT JOIN item_types dit ON cs.default_item_type_id = dit.id
		%s
		ORDER BY cs.is_default DESC, cs.name
		LIMIT ? OFFSET ?`, notificationSettingSubquery, whereClause)

	paginationArgs := append(args, limit, offset)
	rows, err := r.db.Query(query, paginationArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list configuration sets: %w", err)
	}
	defer rows.Close()

	var configSets []models.ConfigurationSet
	for rows.Next() {
		var cs models.ConfigurationSet
		var workflowName sql.NullString
		var workflowID sql.NullInt64
		var defaultItemTypeID sql.NullInt64
		var notificationSettingID sql.NullInt64
		var notificationSettingName sql.NullString
		var defaultItemTypeName sql.NullString

		err := rows.Scan(
			&cs.ID, &cs.Name, &cs.Description,
			&cs.IsDefault, &cs.DifferentiateByItemType, &workflowID, &defaultItemTypeID,
			&notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt,
			&workflowName, &defaultItemTypeName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan configuration set: %w", err)
		}

		cs.WorkflowName = workflowName.String
		cs.NotificationSettingName = notificationSettingName.String
		cs.DefaultItemTypeName = defaultItemTypeName.String
		cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
		cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)
		cs.DefaultItemTypeID = utils.NullInt64ToPtr(defaultItemTypeID)

		// Load related data for each config set
		if err := r.loadRelations(&cs); err != nil {
			return nil, 0, err
		}

		configSets = append(configSets, cs)
	}

	if configSets == nil {
		configSets = []models.ConfigurationSet{}
	}

	return configSets, totalCount, nil
}

// Exists checks if a configuration set exists by ID
func (r *ConfigurationSetRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if configuration set exists: %w", err)
	}
	return exists, nil
}

// Delete removes a configuration set and all its associations
func (r *ConfigurationSetRepository) Delete(id int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete associations first
	if _, err := tx.Exec("DELETE FROM configuration_set_notification_settings WHERE configuration_set_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete notification settings: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete workspace assignments: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM configuration_set_screens WHERE configuration_set_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete screen assignments: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete item type assignments: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete priority assignments: %w", err)
	}

	// Delete the configuration set
	result, err := tx.Exec("DELETE FROM configuration_sets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete configuration set: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}

// loadRelations loads all related data for a configuration set
func (r *ConfigurationSetRepository) loadRelations(cs *models.ConfigurationSet) error {
	// Load workspaces
	workspaceIDs, workspaceNames, err := r.loadWorkspaces(cs.ID)
	if err != nil {
		return err
	}
	cs.WorkspaceIDs = workspaceIDs
	cs.Workspaces = workspaceNames

	// Load screens
	if err := r.loadScreens(cs); err != nil {
		return err
	}

	// Load item type configs
	itemTypeConfigs, err := r.loadItemTypeConfigs(cs.ID)
	if err != nil {
		return err
	}
	cs.ItemTypeConfigs = itemTypeConfigs

	// Populate backward-compatible fields
	var itemTypeNames []string
	var itemTypesDetailed []models.ItemTypeDisplay
	for _, config := range itemTypeConfigs {
		itemTypeNames = append(itemTypeNames, config.ItemTypeName)
		itemTypesDetailed = append(itemTypesDetailed, models.ItemTypeDisplay{
			Name:           config.ItemTypeName,
			Icon:           config.ItemTypeIcon,
			Color:          config.ItemTypeColor,
			HierarchyLevel: config.HierarchyLevel,
		})
	}
	cs.ItemTypes = itemTypeNames
	cs.ItemTypesDetailed = itemTypesDetailed

	// Load priorities
	priorityIDs, priorities, err := r.loadPriorities(cs.ID)
	if err != nil {
		return err
	}
	cs.PriorityIDs = priorityIDs
	cs.PrioritiesDetailed = priorities

	// Populate backward-compatible priority names
	var priorityNames []string
	for _, p := range priorities {
		priorityNames = append(priorityNames, p.Name)
	}
	cs.Priorities = priorityNames

	return nil
}

// loadWorkspaces loads workspace assignments for a configuration set
func (r *ConfigurationSetRepository) loadWorkspaces(configSetID int) ([]int, []string, error) {
	query := `
		SELECT w.id, w.name
		FROM workspace_configuration_sets wcs
		JOIN workspaces w ON wcs.workspace_id = w.id
		WHERE wcs.configuration_set_id = ?
		ORDER BY w.name`

	rows, err := r.db.Query(query, configSetID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load workspaces: %w", err)
	}
	defer rows.Close()

	var workspaceIDs []int
	var workspaceNames []string
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, fmt.Errorf("failed to scan workspace: %w", err)
		}
		workspaceIDs = append(workspaceIDs, id)
		workspaceNames = append(workspaceNames, name)
	}

	return workspaceIDs, workspaceNames, nil
}

// loadScreens loads screen assignments for a configuration set
func (r *ConfigurationSetRepository) loadScreens(cs *models.ConfigurationSet) error {
	query := `
		SELECT css.context, css.screen_id, s.name
		FROM configuration_set_screens css
		JOIN screens s ON css.screen_id = s.id
		WHERE css.configuration_set_id = ?`

	rows, err := r.db.Query(query, cs.ID)
	if err != nil {
		return fmt.Errorf("failed to load screens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var context string
		var screenID int
		var screenName string
		if err := rows.Scan(&context, &screenID, &screenName); err != nil {
			return fmt.Errorf("failed to scan screen: %w", err)
		}

		switch context {
		case "create":
			cs.CreateScreenID = &screenID
			cs.CreateScreenName = screenName
		case "edit":
			cs.EditScreenID = &screenID
			cs.EditScreenName = screenName
		case "view":
			cs.ViewScreenID = &screenID
			cs.ViewScreenName = screenName
		}
	}

	return nil
}

// loadItemTypeConfigs loads item type configurations for a configuration set
func (r *ConfigurationSetRepository) loadItemTypeConfigs(configSetID int) ([]models.ItemTypeConfig, error) {
	query := `
		SELECT
			it.id, it.name, it.icon, it.color, it.hierarchy_level,
			csit.workflow_id, wf.name as workflow_name,
			csit.create_screen_id, cs_create.name as create_screen_name,
			csit.edit_screen_id, cs_edit.name as edit_screen_name,
			csit.view_screen_id, cs_view.name as view_screen_name
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		LEFT JOIN workflows wf ON csit.workflow_id = wf.id
		LEFT JOIN screens cs_create ON csit.create_screen_id = cs_create.id
		LEFT JOIN screens cs_edit ON csit.edit_screen_id = cs_edit.id
		LEFT JOIN screens cs_view ON csit.view_screen_id = cs_view.id
		WHERE csit.configuration_set_id = ?
		ORDER BY it.hierarchy_level, it.sort_order`

	rows, err := r.db.Query(query, configSetID)
	if err != nil {
		return nil, fmt.Errorf("failed to load item type configs: %w", err)
	}
	defer rows.Close()

	var configs []models.ItemTypeConfig
	for rows.Next() {
		var config models.ItemTypeConfig
		var workflowID sql.NullInt64
		var workflowName sql.NullString
		var createScreenID sql.NullInt64
		var createScreenName sql.NullString
		var editScreenID sql.NullInt64
		var editScreenName sql.NullString
		var viewScreenID sql.NullInt64
		var viewScreenName sql.NullString

		if err := rows.Scan(
			&config.ItemTypeID, &config.ItemTypeName, &config.ItemTypeIcon, &config.ItemTypeColor, &config.HierarchyLevel,
			&workflowID, &workflowName,
			&createScreenID, &createScreenName,
			&editScreenID, &editScreenName,
			&viewScreenID, &viewScreenName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan item type config: %w", err)
		}

		config.WorkflowID = utils.NullInt64ToPtr(workflowID)
		config.WorkflowName = "Default"
		if workflowName.Valid {
			config.WorkflowName = workflowName.String
		}
		config.CreateScreenID = utils.NullInt64ToPtr(createScreenID)
		config.CreateScreenName = "Default"
		if createScreenName.Valid {
			config.CreateScreenName = createScreenName.String
		}
		config.EditScreenID = utils.NullInt64ToPtr(editScreenID)
		config.EditScreenName = "Default"
		if editScreenName.Valid {
			config.EditScreenName = editScreenName.String
		}
		config.ViewScreenID = utils.NullInt64ToPtr(viewScreenID)
		config.ViewScreenName = "Default"
		if viewScreenName.Valid {
			config.ViewScreenName = viewScreenName.String
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// loadPriorities loads priority assignments for a configuration set
func (r *ConfigurationSetRepository) loadPriorities(configSetID int) ([]int, []models.PriorityDisplay, error) {
	query := `
		SELECT p.id, p.name, p.icon, p.color, p.sort_order
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
		ORDER BY p.sort_order`

	rows, err := r.db.Query(query, configSetID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load priorities: %w", err)
	}
	defer rows.Close()

	var priorityIDs []int
	var priorities []models.PriorityDisplay
	for rows.Next() {
		var priority models.PriorityDisplay
		if err := rows.Scan(&priority.ID, &priority.Name, &priority.Icon, &priority.Color, &priority.SortOrder); err != nil {
			return nil, nil, fmt.Errorf("failed to scan priority: %w", err)
		}
		priorityIDs = append(priorityIDs, priority.ID)
		priorities = append(priorities, priority)
	}

	return priorityIDs, priorities, nil
}

// Validation methods

// WorkspaceExists checks if a workspace exists
func (r *ConfigurationSetRepository) WorkspaceExists(workspaceID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", workspaceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check workspace existence: %w", err)
	}
	return exists, nil
}

// GetWorkspaceConfigSetID returns the configuration set ID for a workspace
func (r *ConfigurationSetRepository) GetWorkspaceConfigSetID(workspaceID int) (*int, error) {
	var configSetID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT configuration_set_id
		FROM workspace_configuration_sets
		WHERE workspace_id = ?
	`, workspaceID).Scan(&configSetID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace config set: %w", err)
	}

	return utils.NullInt64ToPtr(configSetID), nil
}

// StatusExists checks if a status exists
func (r *ConfigurationSetRepository) StatusExists(statusID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", statusID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check status existence: %w", err)
	}
	return exists, nil
}

// ItemTypeExists checks if an item type exists
func (r *ConfigurationSetRepository) ItemTypeExists(itemTypeID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", itemTypeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check item type existence: %w", err)
	}
	return exists, nil
}

// PriorityExists checks if a priority exists
func (r *ConfigurationSetRepository) PriorityExists(priorityID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE id = ?)", priorityID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check priority existence: %w", err)
	}
	return exists, nil
}

// Create inserts a new configuration set and returns its ID
func (r *ConfigurationSetRepository) Create(tx database.Tx, cs *models.ConfigurationSet) (int64, error) {
	now := time.Now()
	var id int64
	err := tx.QueryRow(`
		INSERT INTO configuration_sets (name, description, is_default, differentiate_by_item_type, workflow_id, default_item_type_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, cs.Name, cs.Description, cs.IsDefault, cs.DifferentiateByItemType, cs.WorkflowID, cs.DefaultItemTypeID, now, now).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create configuration set: %w", err)
	}
	return id, nil
}

// Update updates a configuration set
func (r *ConfigurationSetRepository) Update(tx database.Tx, id int, cs *models.ConfigurationSet) error {
	now := time.Now()
	result, err := tx.Exec(`
		UPDATE configuration_sets
		SET name = ?, description = ?, is_default = ?, differentiate_by_item_type = ?, workflow_id = ?, default_item_type_id = ?, updated_at = ?
		WHERE id = ?
	`, cs.Name, cs.Description, cs.IsDefault, cs.DifferentiateByItemType, cs.WorkflowID, cs.DefaultItemTypeID, now, id)

	if err != nil {
		return fmt.Errorf("failed to update configuration set: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// SaveNotificationSetting saves the notification setting for a configuration set
func (r *ConfigurationSetRepository) SaveNotificationSetting(tx database.Tx, configSetID int, notificationSettingID *int) error {
	// Delete existing
	if _, err := tx.Exec("DELETE FROM configuration_set_notification_settings WHERE configuration_set_id = ?", configSetID); err != nil {
		return fmt.Errorf("failed to delete notification setting: %w", err)
	}

	// Insert new if provided
	if notificationSettingID != nil {
		now := time.Now()
		_, err := tx.Exec(`
			INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
			VALUES (?, ?, ?)
		`, configSetID, *notificationSettingID, now)
		if err != nil {
			return fmt.Errorf("failed to insert notification setting: %w", err)
		}
	}

	return nil
}

// SaveWorkspaceAssignments saves workspace assignments for a configuration set
func (r *ConfigurationSetRepository) SaveWorkspaceAssignments(tx database.Tx, configSetID int, workspaceIDs []int) error {
	// Delete existing
	if _, err := tx.Exec("DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?", configSetID); err != nil {
		return fmt.Errorf("failed to delete workspace assignments: %w", err)
	}

	// Insert new
	now := time.Now()
	for _, workspaceID := range workspaceIDs {
		_, err := tx.Exec(`
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, configSetID, now)
		if err != nil {
			return fmt.Errorf("failed to insert workspace assignment: %w", err)
		}
	}

	return nil
}

// SaveScreenAssignments saves screen assignments for a configuration set
func (r *ConfigurationSetRepository) SaveScreenAssignments(tx database.Tx, configSetID int, createScreenID, editScreenID, viewScreenID *int) error {
	// Delete existing
	if _, err := tx.Exec("DELETE FROM configuration_set_screens WHERE configuration_set_id = ?", configSetID); err != nil {
		return fmt.Errorf("failed to delete screen assignments: %w", err)
	}

	// Insert new
	now := time.Now()
	assignments := []struct {
		screenID *int
		context  string
	}{
		{createScreenID, "create"},
		{editScreenID, "edit"},
		{viewScreenID, "view"},
	}

	for _, assignment := range assignments {
		if assignment.screenID != nil {
			_, err := tx.Exec(`
				INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context, created_at)
				VALUES (?, ?, ?, ?)
			`, configSetID, *assignment.screenID, assignment.context, now)
			if err != nil {
				return fmt.Errorf("failed to insert screen assignment: %w", err)
			}
		}
	}

	return nil
}

// SaveItemTypeConfigs saves item type configurations for a configuration set
func (r *ConfigurationSetRepository) SaveItemTypeConfigs(tx database.Tx, configSetID int, configs []models.ItemTypeConfig) error {
	// Delete existing
	if _, err := tx.Exec("DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?", configSetID); err != nil {
		return fmt.Errorf("failed to delete item type configs: %w", err)
	}

	// Insert new
	now := time.Now()
	for _, config := range configs {
		_, err := tx.Exec(`
			INSERT INTO configuration_set_item_types (
				configuration_set_id, item_type_id,
				workflow_id, create_screen_id, edit_screen_id, view_screen_id,
				created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, configSetID, config.ItemTypeID,
			config.WorkflowID, config.CreateScreenID,
			config.EditScreenID, config.ViewScreenID,
			now)
		if err != nil {
			return fmt.Errorf("failed to insert item type config: %w", err)
		}
	}

	return nil
}

// SavePriorityAssignments saves priority assignments for a configuration set
func (r *ConfigurationSetRepository) SavePriorityAssignments(tx database.Tx, configSetID int, priorityIDs []int) error {
	// Delete existing
	if _, err := tx.Exec("DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?", configSetID); err != nil {
		return fmt.Errorf("failed to delete priority assignments: %w", err)
	}

	// Insert new
	now := time.Now()
	for _, priorityID := range priorityIDs {
		_, err := tx.Exec(`
			INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
			VALUES (?, ?, ?)
		`, configSetID, priorityID, now)
		if err != nil {
			return fmt.Errorf("failed to insert priority assignment: %w", err)
		}
	}

	return nil
}

// GetWorkspaceIDs returns the workspace IDs for a configuration set
func (r *ConfigurationSetRepository) GetWorkspaceIDs(configSetID int) ([]int, error) {
	query := `
		SELECT workspace_id
		FROM workspace_configuration_sets
		WHERE configuration_set_id = ?`

	rows, err := r.db.Query(query, configSetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace IDs: %w", err)
	}
	defer rows.Close()

	var workspaceIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan workspace ID: %w", err)
		}
		workspaceIDs = append(workspaceIDs, id)
	}

	return workspaceIDs, nil
}

// ClearDefaultFlag clears the is_default flag from all configuration sets except the specified one
func (r *ConfigurationSetRepository) ClearDefaultFlag(tx database.Tx, exceptID int) error {
	_, err := tx.Exec(`
		UPDATE configuration_sets SET is_default = false WHERE id != ?
	`, exceptID)
	if err != nil {
		return fmt.Errorf("failed to clear default flag: %w", err)
	}
	return nil
}
