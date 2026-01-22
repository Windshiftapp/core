package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

const notificationSettingColumns = `
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

type ConfigurationSetHandler struct {
	db                  database.Database
	notificationService interface {
		ForceRefreshCache() error
	} // Notification service for cache refresh (optional, can be nil)
}

func NewConfigurationSetHandler(db database.Database, notificationService interface{ ForceRefreshCache() error }) *ConfigurationSetHandler {
	return &ConfigurationSetHandler{
		db:                  db,
		notificationService: notificationService,
	}
}

// normalizeStatusName normalizes status names for comparison by replacing underscores with spaces and converting to lowercase
func normalizeStatusName(name string) string {
	// Replace underscores with spaces and convert to lowercase
	normalized := strings.ReplaceAll(name, "_", " ")
	return strings.ToLower(normalized)
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

	// Build WHERE clause for search
	whereClause := ""
	args := []interface{}{}
	if search != "" {
		whereClause = "WHERE LOWER(cs.name) LIKE ?"
		args = append(args, "%"+strings.ToLower(search)+"%")
	}

	// Build count query
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM configuration_sets cs
		%s`, whereClause)

	var totalCount int
	err := h.db.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		http.Error(w, "Failed to get total count: "+err.Error(), http.StatusInternalServerError)
		return
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
		LIMIT ? OFFSET ?`, notificationSettingColumns, whereClause)

	// Add pagination parameters to args
	paginationArgs := append(args, limit, offset)

	rows, err := h.db.Query(query, paginationArgs...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
		err := rows.Scan(&cs.ID, &cs.Name, &cs.Description,
			&cs.IsDefault, &cs.DifferentiateByItemType, &workflowID, &defaultItemTypeID, &notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt, &workflowName, &defaultItemTypeName)

		cs.WorkflowName = workflowName.String
		cs.NotificationSettingName = notificationSettingName.String
		cs.DefaultItemTypeName = defaultItemTypeName.String
		cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
		cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)
		cs.DefaultItemTypeID = utils.NullInt64ToPtr(defaultItemTypeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Load workspace assignments for this configuration set
		workspaceQuery := `
			SELECT w.id, w.name
			FROM workspace_configuration_sets wcs
			JOIN workspaces w ON wcs.workspace_id = w.id
			WHERE wcs.configuration_set_id = ?
			ORDER BY w.name`

		workspaceRows, err := h.db.Query(workspaceQuery, cs.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var workspaceIDs []int
		var workspaceNames []string
		for workspaceRows.Next() {
			var workspaceID int
			var workspaceName string
			if err := workspaceRows.Scan(&workspaceID, &workspaceName); err != nil {
				workspaceRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			workspaceIDs = append(workspaceIDs, workspaceID)
			workspaceNames = append(workspaceNames, workspaceName)
		}
		workspaceRows.Close()

		cs.WorkspaceIDs = workspaceIDs
		cs.Workspaces = workspaceNames

		// Load screen assignments for this configuration set
		screenQuery := `
			SELECT css.context, css.screen_id, s.name
			FROM configuration_set_screens css
			JOIN screens s ON css.screen_id = s.id
			WHERE css.configuration_set_id = ?`

		screenRows, err := h.db.Query(screenQuery, cs.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for screenRows.Next() {
			var context string
			var screenID int
			var screenName string
			if err := screenRows.Scan(&context, &screenID, &screenName); err != nil {
				screenRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Assign to appropriate context field
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
		screenRows.Close()

		// Load item types for this configuration set via junction table with override information
		itemTypeQuery := `
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

		itemTypeRows, err := h.db.Query(itemTypeQuery, cs.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var itemTypeNames []string
		var itemTypesDetailed []models.ItemTypeDisplay
		var itemTypeConfigs []models.ItemTypeConfig
		for itemTypeRows.Next() {
			var config models.ItemTypeConfig
			var workflowID sql.NullInt64
			var workflowName sql.NullString
			var createScreenID sql.NullInt64
			var createScreenName sql.NullString
			var editScreenID sql.NullInt64
			var editScreenName sql.NullString
			var viewScreenID sql.NullInt64
			var viewScreenName sql.NullString

			if err := itemTypeRows.Scan(
				&config.ItemTypeID, &config.ItemTypeName, &config.ItemTypeIcon, &config.ItemTypeColor, &config.HierarchyLevel,
				&workflowID, &workflowName,
				&createScreenID, &createScreenName,
				&editScreenID, &editScreenName,
				&viewScreenID, &viewScreenName,
			); err != nil {
				itemTypeRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Populate override fields (NULL means use config set default)
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

			itemTypeNames = append(itemTypeNames, config.ItemTypeName)
			itemTypesDetailed = append(itemTypesDetailed, models.ItemTypeDisplay{
				Name:           config.ItemTypeName,
				Icon:           config.ItemTypeIcon,
				Color:          config.ItemTypeColor,
				HierarchyLevel: config.HierarchyLevel,
			})
			itemTypeConfigs = append(itemTypeConfigs, config)
		}
		itemTypeRows.Close()

		cs.ItemTypes = itemTypeNames             // Keep for backward compatibility
		cs.ItemTypesDetailed = itemTypesDetailed // Keep for backward compatibility
		cs.ItemTypeConfigs = itemTypeConfigs     // New field with override information

		// Load priorities for this configuration set via junction table
		priorityQuery := `
			SELECT p.id, p.name, p.icon, p.color, p.sort_order
			FROM configuration_set_priorities csp
			JOIN priorities p ON csp.priority_id = p.id
			WHERE csp.configuration_set_id = ?
			ORDER BY p.sort_order`

		priorityRows, err := h.db.Query(priorityQuery, cs.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var priorityIDs []int
		var priorityNames []string
		var prioritiesDetailed []models.PriorityDisplay
		for priorityRows.Next() {
			var priority models.PriorityDisplay
			if err := priorityRows.Scan(&priority.ID, &priority.Name, &priority.Icon, &priority.Color, &priority.SortOrder); err != nil {
				priorityRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			priorityIDs = append(priorityIDs, priority.ID)
			priorityNames = append(priorityNames, priority.Name)
			prioritiesDetailed = append(prioritiesDetailed, priority)
		}
		priorityRows.Close()

		cs.PriorityIDs = priorityIDs
		cs.Priorities = priorityNames // Keep for backward compatibility
		cs.PrioritiesDetailed = prioritiesDetailed

		configSets = append(configSets, cs)
	}

	if configSets == nil {
		configSets = []models.ConfigurationSet{}
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
	`, notificationSettingColumns)
	err := h.db.QueryRow(query, id).Scan(&cs.ID, &cs.Name, &cs.Description,
		&cs.IsDefault, &cs.DifferentiateByItemType, &workflowID, &defaultItemTypeID, &notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt, &workflowName, &defaultItemTypeName)

	cs.WorkflowName = workflowName.String
	cs.NotificationSettingName = notificationSettingName.String
	cs.DefaultItemTypeName = defaultItemTypeName.String
	cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
	cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)
	cs.DefaultItemTypeID = utils.NullInt64ToPtr(defaultItemTypeID)

	if err == sql.ErrNoRows {
		http.Error(w, "Configuration set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load workspace assignments
	workspaceQuery := `
		SELECT w.id, w.name
		FROM workspace_configuration_sets wcs
		JOIN workspaces w ON wcs.workspace_id = w.id
		WHERE wcs.configuration_set_id = ?
		ORDER BY w.name`

	workspaceRows, err := h.db.Query(workspaceQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer workspaceRows.Close()

	var workspaceIDs []int
	var workspaceNames []string
	for workspaceRows.Next() {
		var workspaceID int
		var workspaceName string
		if err := workspaceRows.Scan(&workspaceID, &workspaceName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
		workspaceNames = append(workspaceNames, workspaceName)
	}

	cs.WorkspaceIDs = workspaceIDs
	cs.Workspaces = workspaceNames

	// Load screen assignments
	screenQuery := `
		SELECT css.context, css.screen_id, s.name
		FROM configuration_set_screens css
		JOIN screens s ON css.screen_id = s.id
		WHERE css.configuration_set_id = ?`

	screenRows, err := h.db.Query(screenQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer screenRows.Close()

	for screenRows.Next() {
		var context string
		var screenID int
		var screenName string
		if err := screenRows.Scan(&context, &screenID, &screenName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Assign to appropriate context field
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

	// Load item types for this configuration set with override information
	itemTypeQuery := `
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

	itemTypeRows, err := h.db.Query(itemTypeQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer itemTypeRows.Close()

	var itemTypeNames []string
	var itemTypesDetailed []models.ItemTypeDisplay
	var itemTypeConfigs []models.ItemTypeConfig
	for itemTypeRows.Next() {
		var config models.ItemTypeConfig
		var workflowID sql.NullInt64
		var workflowName sql.NullString
		var createScreenID sql.NullInt64
		var createScreenName sql.NullString
		var editScreenID sql.NullInt64
		var editScreenName sql.NullString
		var viewScreenID sql.NullInt64
		var viewScreenName sql.NullString

		if err := itemTypeRows.Scan(
			&config.ItemTypeID, &config.ItemTypeName, &config.ItemTypeIcon, &config.ItemTypeColor, &config.HierarchyLevel,
			&workflowID, &workflowName,
			&createScreenID, &createScreenName,
			&editScreenID, &editScreenName,
			&viewScreenID, &viewScreenName,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Populate override fields (NULL means use config set default)
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

		itemTypeNames = append(itemTypeNames, config.ItemTypeName)
		itemTypesDetailed = append(itemTypesDetailed, models.ItemTypeDisplay{
			Name:           config.ItemTypeName,
			Icon:           config.ItemTypeIcon,
			Color:          config.ItemTypeColor,
			HierarchyLevel: config.HierarchyLevel,
		})
		itemTypeConfigs = append(itemTypeConfigs, config)
	}

	cs.ItemTypes = itemTypeNames             // Keep for backward compatibility
	cs.ItemTypesDetailed = itemTypesDetailed // Keep for backward compatibility
	cs.ItemTypeConfigs = itemTypeConfigs     // New field with override information

	// Load priorities for this configuration set via junction table
	priorityQuery := `
		SELECT p.id, p.name, p.icon, p.color, p.sort_order
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
		ORDER BY p.sort_order`

	priorityRows, err := h.db.Query(priorityQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer priorityRows.Close()

	var priorityIDs []int
	var priorityNames []string
	var prioritiesDetailed []models.PriorityDisplay
	for priorityRows.Next() {
		var priority models.PriorityDisplay
		if err := priorityRows.Scan(&priority.ID, &priority.Name, &priority.Icon, &priority.Color, &priority.SortOrder); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		priorityIDs = append(priorityIDs, priority.ID)
		priorityNames = append(priorityNames, priority.Name)
		prioritiesDetailed = append(prioritiesDetailed, priority)
	}

	cs.PriorityIDs = priorityIDs
	cs.Priorities = priorityNames // Keep for backward compatibility
	cs.PrioritiesDetailed = prioritiesDetailed

	respondJSONOK(w, cs)
}

func (h *ConfigurationSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cs models.ConfigurationSet
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(cs.Name) == "" {
		http.Error(w, "Configuration set name is required", http.StatusBadRequest)
		return
	}

	// Verify workspaces exist
	for _, workspaceID := range cs.WorkspaceIDs {
		var workspaceExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", workspaceID).Scan(&workspaceExists)
		if err != nil || !workspaceExists {
			http.Error(w, "One or more workspaces not found", http.StatusBadRequest)
			return
		}
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO configuration_sets (name, description, is_default, differentiate_by_item_type, workflow_id, default_item_type_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, cs.Name, cs.Description, cs.IsDefault, cs.DifferentiateByItemType, cs.WorkflowID, cs.DefaultItemTypeID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert notification setting into join table if provided
	if cs.NotificationSettingID != nil {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
			VALUES (?, ?, ?)
		`, id, *cs.NotificationSettingID, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Insert workspace assignments
	for _, workspaceID := range cs.WorkspaceIDs {
		_, err = tx.Exec(`
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, id, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Insert screen assignments
	screenAssignments := []struct {
		screenID *int
		context  string
	}{
		{cs.CreateScreenID, "create"},
		{cs.EditScreenID, "edit"},
		{cs.ViewScreenID, "view"},
	}

	for _, assignment := range screenAssignments {
		if assignment.screenID != nil {
			_, err = tx.Exec(`
				INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context, created_at)
				VALUES (?, ?, ?, ?)
			`, id, *assignment.screenID, assignment.context, now)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// Insert item type configurations with overrides
	for _, itemTypeConfig := range cs.ItemTypeConfigs {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_item_types (
				configuration_set_id, item_type_id,
				workflow_id, create_screen_id, edit_screen_id, view_screen_id,
				created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, id, itemTypeConfig.ItemTypeID,
			itemTypeConfig.WorkflowID, itemTypeConfig.CreateScreenID,
			itemTypeConfig.EditScreenID, itemTypeConfig.ViewScreenID,
			now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Insert priority associations
	for _, priorityID := range cs.PriorityIDs {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
			VALUES (?, ?, ?)
		`, id, priorityID, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh notification cache if service is available
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err := h.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, createCacheWarning("notification", err, fmt.Sprintf("configuration_set_id:%d", id)))
		}
	}

	// Return the created configuration set
	var workflowName sql.NullString
	var workflowID sql.NullInt64
	var notificationSettingID sql.NullInt64
	var notificationSettingName sql.NullString
	query := fmt.Sprintf(`
		SELECT cs.id, cs.name, cs.description, cs.is_default, cs.workflow_id,
		       %s,
		       cs.created_at, cs.updated_at,
		       wf.name as workflow_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, notificationSettingColumns)
	err = h.db.QueryRow(query, id).Scan(&cs.ID, &cs.Name, &cs.Description,
		&cs.IsDefault, &workflowID, &notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt, &workflowName)

	cs.WorkflowName = workflowName.String
	cs.NotificationSettingName = notificationSettingName.String
	cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
	cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load workspace assignments
	workspaceQuery := `
		SELECT w.id, w.name
		FROM workspace_configuration_sets wcs
		JOIN workspaces w ON wcs.workspace_id = w.id
		WHERE wcs.configuration_set_id = ?
		ORDER BY w.name`

	workspaceRows, err := h.db.Query(workspaceQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer workspaceRows.Close()

	var workspaceIDs []int
	var workspaceNames []string
	for workspaceRows.Next() {
		var workspaceID int
		var workspaceName string
		if err := workspaceRows.Scan(&workspaceID, &workspaceName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
		workspaceNames = append(workspaceNames, workspaceName)
	}

	cs.WorkspaceIDs = workspaceIDs
	cs.Workspaces = workspaceNames

	// Load screen assignments
	screenQuery := `
		SELECT css.context, css.screen_id, s.name
		FROM configuration_set_screens css
		JOIN screens s ON css.screen_id = s.id
		WHERE css.configuration_set_id = ?`

	screenRows, err := h.db.Query(screenQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer screenRows.Close()

	for screenRows.Next() {
		var context string
		var screenID int
		var screenName string
		if err := screenRows.Scan(&context, &screenID, &screenName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Assign to appropriate context field
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

	// Load item types for this configuration set with override information
	itemTypeQuery := `
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

	itemTypeRows, err := h.db.Query(itemTypeQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer itemTypeRows.Close()

	var itemTypeNames []string
	var itemTypesDetailed []models.ItemTypeDisplay
	var itemTypeConfigs []models.ItemTypeConfig
	for itemTypeRows.Next() {
		var config models.ItemTypeConfig
		var workflowID sql.NullInt64
		var workflowName sql.NullString
		var createScreenID sql.NullInt64
		var createScreenName sql.NullString
		var editScreenID sql.NullInt64
		var editScreenName sql.NullString
		var viewScreenID sql.NullInt64
		var viewScreenName sql.NullString

		if err := itemTypeRows.Scan(
			&config.ItemTypeID, &config.ItemTypeName, &config.ItemTypeIcon, &config.ItemTypeColor, &config.HierarchyLevel,
			&workflowID, &workflowName,
			&createScreenID, &createScreenName,
			&editScreenID, &editScreenName,
			&viewScreenID, &viewScreenName,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Populate override fields (NULL means use config set default)
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

		itemTypeNames = append(itemTypeNames, config.ItemTypeName)
		itemTypesDetailed = append(itemTypesDetailed, models.ItemTypeDisplay{
			Name:           config.ItemTypeName,
			Icon:           config.ItemTypeIcon,
			Color:          config.ItemTypeColor,
			HierarchyLevel: config.HierarchyLevel,
		})
		itemTypeConfigs = append(itemTypeConfigs, config)
	}

	cs.ItemTypes = itemTypeNames             // Keep for backward compatibility
	cs.ItemTypesDetailed = itemTypesDetailed // Keep for backward compatibility
	cs.ItemTypeConfigs = itemTypeConfigs     // New field with override information

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		csID := int(id)
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetCreate,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &csID,
			ResourceName: cs.Name,
			Details: map[string]interface{}{
				"description":     cs.Description,
				"workflow_id":     cs.WorkflowID,
				"workspace_count": len(cs.WorkspaceIDs),
			},
			Success: true,
		})
	}

	respondJSONCreatedWithWarnings(w, cs, warnings)
}

func (h *ConfigurationSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old configuration set for audit logging
	var oldCS models.ConfigurationSet
	err := h.db.QueryRow(`
		SELECT id, name, description, is_default, differentiate_by_item_type, workflow_id
		FROM configuration_sets
		WHERE id = ?
	`, id).Scan(&oldCS.ID, &oldCS.Name, &oldCS.Description, &oldCS.IsDefault, &oldCS.DifferentiateByItemType, &oldCS.WorkflowID)

	if err == sql.ErrNoRows {
		http.Error(w, "Configuration set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var cs models.ConfigurationSet
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(cs.Name) == "" {
		http.Error(w, "Configuration set name is required", http.StatusBadRequest)
		return
	}

	// Verify workspaces exist (only if workspaces are provided)
	for _, workspaceID := range cs.WorkspaceIDs {
		var workspaceExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", workspaceID).Scan(&workspaceExists)
		if err != nil || !workspaceExists {
			http.Error(w, "One or more workspaces not found", http.StatusBadRequest)
			return
		}
	}

	// Check if any workspace is moving from a different config set (requires migration)
	// Skip this check if skip_migration_check query param is set (used after migration is complete)
	skipMigrationCheck := r.URL.Query().Get("skip_migration_check") == "true"
	if !skipMigrationCheck {
		for _, workspaceID := range cs.WorkspaceIDs {
			var currentConfigSetID sql.NullInt64
			h.db.QueryRow(`
				SELECT configuration_set_id FROM workspace_configuration_sets WHERE workspace_id = ?
			`, workspaceID).Scan(&currentConfigSetID)

			// If workspace is currently assigned to a different config set
			if currentConfigSetID.Valid && int(currentConfigSetID.Int64) != id {
				// Analyze migration requirements
				sourceID := int(currentConfigSetID.Int64)

				itemTypeMigrations, _, requiresItemTypeMigration := h.analyzeItemTypeMigration(workspaceID, sourceID, id)
				customFieldMigrations, requiresFieldMigration := h.analyzeCustomFieldMigration(workspaceID, sourceID, id)
				priorityMigrations, _, requiresPriorityMigration := h.analyzePriorityMigration(workspaceID, sourceID, id)
				statusMigrations, requiresStatusMigration := h.analyzeStatusMigration(workspaceID, id)

				requiresMigration := requiresItemTypeMigration || requiresFieldMigration ||
					requiresPriorityMigration || requiresStatusMigration

				if requiresMigration {
					// Get config set names for the response
					var sourceConfigSetName, targetConfigSetName string
					h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, sourceID).Scan(&sourceConfigSetName)
					h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, id).Scan(&targetConfigSetName)

					var totalItems int
					h.db.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = ?`, workspaceID).Scan(&totalItems)

					analysis := models.ComprehensiveMigrationAnalysis{
						OldConfigSetID:            sourceID,
						OldConfigSetName:          sourceConfigSetName,
						NewConfigSetID:            id,
						NewConfigSetName:          targetConfigSetName,
						AffectedWorkspaces:        []int{workspaceID},
						TotalAffectedItems:        totalItems,
						ItemTypeMigrations:        itemTypeMigrations,
						CustomFieldMigrations:     customFieldMigrations,
						PriorityMigrations:        priorityMigrations,
						StatusMigrations:          statusMigrations,
						RequiresMigration:         true,
						RequiresItemTypeMigration: requiresItemTypeMigration,
						RequiresFieldMigration:    requiresFieldMigration,
						RequiresPriorityMigration: requiresPriorityMigration,
						RequiresStatusMigration:   requiresStatusMigration,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":    "migration_required",
						"message":  "Migration is required before this workspace can be assigned to the new configuration set",
						"analysis": analysis,
					})
					return
				}
			}
		}
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE configuration_sets
		SET name = ?, description = ?, is_default = ?, differentiate_by_item_type = ?, workflow_id = ?, default_item_type_id = ?, updated_at = ?
		WHERE id = ?
	`, cs.Name, cs.Description, cs.IsDefault, cs.DifferentiateByItemType, cs.WorkflowID, cs.DefaultItemTypeID, now, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete existing notification setting assignments from join table
	_, err = tx.Exec("DELETE FROM configuration_set_notification_settings WHERE configuration_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new notification setting into join table if provided
	if cs.NotificationSettingID != nil {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
			VALUES (?, ?, ?)
		`, id, *cs.NotificationSettingID, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete existing workspace assignments
	_, err = tx.Exec("DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new workspace assignments
	for _, workspaceID := range cs.WorkspaceIDs {
		_, err = tx.Exec(`
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, id, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete existing screen assignments
	_, err = tx.Exec("DELETE FROM configuration_set_screens WHERE configuration_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new screen assignments
	screenAssignments := []struct {
		screenID *int
		context  string
	}{
		{cs.CreateScreenID, "create"},
		{cs.EditScreenID, "edit"},
		{cs.ViewScreenID, "view"},
	}

	for _, assignment := range screenAssignments {
		if assignment.screenID != nil {
			_, err = tx.Exec(`
				INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context, created_at)
				VALUES (?, ?, ?, ?)
			`, id, *assignment.screenID, assignment.context, now)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// Delete existing item type configurations
	_, err = tx.Exec("DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new item type configurations with overrides
	for _, itemTypeConfig := range cs.ItemTypeConfigs {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_item_types (
				configuration_set_id, item_type_id,
				workflow_id, create_screen_id, edit_screen_id, view_screen_id,
				created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, id, itemTypeConfig.ItemTypeID,
			itemTypeConfig.WorkflowID, itemTypeConfig.CreateScreenID,
			itemTypeConfig.EditScreenID, itemTypeConfig.ViewScreenID,
			now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete existing priority associations
	_, err = tx.Exec("DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new priority associations
	for _, priorityID := range cs.PriorityIDs {
		_, err = tx.Exec(`
			INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at)
			VALUES (?, ?, ?)
		`, id, priorityID, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh notification cache if service is available
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err := h.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, createCacheWarning("notification", err, fmt.Sprintf("configuration_set_id:%d", id)))
		}
	}

	// Return the updated configuration set
	var workflowName sql.NullString
	var workflowID sql.NullInt64
	var notificationSettingID sql.NullInt64
	var notificationSettingName sql.NullString
	query := fmt.Sprintf(`
		SELECT cs.id, cs.name, cs.description, cs.is_default, cs.workflow_id,
		       %s,
		       cs.created_at, cs.updated_at,
		       wf.name as workflow_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, notificationSettingColumns)
	err = h.db.QueryRow(query, id).Scan(&cs.ID, &cs.Name, &cs.Description,
		&cs.IsDefault, &workflowID, &notificationSettingID, &notificationSettingName, &cs.CreatedAt, &cs.UpdatedAt, &workflowName)

	cs.WorkflowName = workflowName.String
	cs.NotificationSettingName = notificationSettingName.String
	cs.WorkflowID = utils.NullInt64ToPtr(workflowID)
	cs.NotificationSettingID = utils.NullInt64ToPtr(notificationSettingID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load workspace assignments
	workspaceQuery := `
		SELECT w.id, w.name
		FROM workspace_configuration_sets wcs
		JOIN workspaces w ON wcs.workspace_id = w.id
		WHERE wcs.configuration_set_id = ?
		ORDER BY w.name`

	workspaceRows, err := h.db.Query(workspaceQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer workspaceRows.Close()

	var workspaceIDs []int
	var workspaceNames []string
	for workspaceRows.Next() {
		var workspaceID int
		var workspaceName string
		if err := workspaceRows.Scan(&workspaceID, &workspaceName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
		workspaceNames = append(workspaceNames, workspaceName)
	}

	cs.WorkspaceIDs = workspaceIDs
	cs.Workspaces = workspaceNames

	// Load screen assignments
	screenQuery := `
		SELECT css.context, css.screen_id, s.name
		FROM configuration_set_screens css
		JOIN screens s ON css.screen_id = s.id
		WHERE css.configuration_set_id = ?`

	screenRows, err := h.db.Query(screenQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer screenRows.Close()

	for screenRows.Next() {
		var context string
		var screenID int
		var screenName string
		if err := screenRows.Scan(&context, &screenID, &screenName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Assign to appropriate context field
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

	// Load item types for this configuration set with override information
	itemTypeQuery := `
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

	itemTypeRows, err := h.db.Query(itemTypeQuery, cs.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer itemTypeRows.Close()

	var itemTypeNames []string
	var itemTypesDetailed []models.ItemTypeDisplay
	var itemTypeConfigs []models.ItemTypeConfig
	for itemTypeRows.Next() {
		var config models.ItemTypeConfig
		var workflowID sql.NullInt64
		var workflowName sql.NullString
		var createScreenID sql.NullInt64
		var createScreenName sql.NullString
		var editScreenID sql.NullInt64
		var editScreenName sql.NullString
		var viewScreenID sql.NullInt64
		var viewScreenName sql.NullString

		if err := itemTypeRows.Scan(
			&config.ItemTypeID, &config.ItemTypeName, &config.ItemTypeIcon, &config.ItemTypeColor, &config.HierarchyLevel,
			&workflowID, &workflowName,
			&createScreenID, &createScreenName,
			&editScreenID, &editScreenName,
			&viewScreenID, &viewScreenName,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Populate override fields (NULL means use config set default)
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

		itemTypeNames = append(itemTypeNames, config.ItemTypeName)
		itemTypesDetailed = append(itemTypesDetailed, models.ItemTypeDisplay{
			Name:           config.ItemTypeName,
			Icon:           config.ItemTypeIcon,
			Color:          config.ItemTypeColor,
			HierarchyLevel: config.HierarchyLevel,
		})
		itemTypeConfigs = append(itemTypeConfigs, config)
	}

	cs.ItemTypes = itemTypeNames             // Keep for backward compatibility
	cs.ItemTypesDetailed = itemTypesDetailed // Keep for backward compatibility
	cs.ItemTypeConfigs = itemTypeConfigs     // New field with override information

	// Log audit event with change tracking
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldCS.Name != cs.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldCS.Name,
				"new": cs.Name,
			}
		}
		if oldCS.Description != cs.Description {
			details["description_changed"] = map[string]interface{}{
				"old": oldCS.Description,
				"new": cs.Description,
			}
		}
		// Track workflow change
		oldWorkflowID := 0
		if oldCS.WorkflowID != nil {
			oldWorkflowID = *oldCS.WorkflowID
		}
		newWorkflowID := 0
		if cs.WorkflowID != nil {
			newWorkflowID = *cs.WorkflowID
		}
		if oldWorkflowID != newWorkflowID {
			details["workflow_changed"] = map[string]interface{}{
				"old": oldWorkflowID,
				"new": newWorkflowID,
			}
		}
		details["workspace_count"] = len(cs.WorkspaceIDs)

		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetUpdate,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &id,
			ResourceName: cs.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOKWithWarnings(w, cs, warnings)
}

func (h *ConfigurationSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the configuration set details for audit logging before deletion
	var csName, description string
	var isDefault bool
	err := h.db.QueryRow(`
		SELECT name, description, is_default
		FROM configuration_sets
		WHERE id = ?
	`, id).Scan(&csName, &description, &isDefault)

	if err == sql.ErrNoRows {
		http.Error(w, "Configuration set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM configuration_sets WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			ActionType:   logger.ActionConfigSetDelete,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &id,
			ResourceName: csName,
			Details: map[string]interface{}{
				"description": description,
				"is_default":  isDefault,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// Migration Assistant endpoints

func (h *ConfigurationSetHandler) AnalyzeMigration(w http.ResponseWriter, r *http.Request) {
	configSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get configuration set details including differentiate_by_item_type
	var configSet models.ConfigurationSet
	var workflowName sql.NullString
	err := h.db.QueryRow(`
		SELECT cs.id, cs.name, cs.workflow_id, cs.differentiate_by_item_type, wf.name as workflow_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, configSetID).Scan(&configSet.ID, &configSet.Name, &configSet.WorkflowID, &configSet.DifferentiateByItemType, &workflowName)

	if err == sql.ErrNoRows {
		http.Error(w, "Configuration set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	configSet.WorkflowName = workflowName.String

	// Get affected workspaces - check for workspace filter in query params
	workspaceFilter := r.URL.Query().Get("workspace_id")
	var affectedWorkspaces []int

	if workspaceFilter != "" {
		// If workspace filter is provided, only analyze that specific workspace
		workspaceID, err := strconv.Atoi(workspaceFilter)
		if err != nil {
			http.Error(w, "Invalid workspace_id parameter", http.StatusBadRequest)
			return
		}
		affectedWorkspaces = []int{workspaceID}
	} else {
		// Otherwise, get all workspaces assigned to this configuration set
		workspaceQuery := `
			SELECT w.id
			FROM workspace_configuration_sets wcs
			JOIN workspaces w ON wcs.workspace_id = w.id
			WHERE wcs.configuration_set_id = ?`

		workspaceRows, err := h.db.Query(workspaceQuery, configSetID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer workspaceRows.Close()

		for workspaceRows.Next() {
			var workspaceID int
			if err := workspaceRows.Scan(&workspaceID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			affectedWorkspaces = append(affectedWorkspaces, workspaceID)
		}
	}

	// Handle empty workspace list
	if len(affectedWorkspaces) == 0 {
		analysis := models.WorkflowMigrationAnalysis{
			NewWorkflowID:      configSet.WorkflowID,
			NewWorkflowName:    configSet.WorkflowName,
			AffectedWorkspaces: affectedWorkspaces,
			StatusMigrations:   []models.StatusMigrationInfo{},
			RequiresMigration:  false,
			TotalAffectedItems: 0,
		}
		respondJSONOK(w, analysis)
		return
	}

	// Use WorkflowService for proper item type workflow resolution
	workflowService := services.NewWorkflowService(h.db)

	var statusMigrations []models.StatusMigrationInfo
	totalAffectedItems := 0
	requiresMigration := false

	// When differentiate_by_item_type is enabled, analyze per item type
	if configSet.DifferentiateByItemType {
		// Get items grouped by item type and status
		statusQuery := `
			SELECT i.item_type_id, COALESCE(it.name, '') as item_type_name,
			       COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name,
			       COUNT(*) as item_count
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			LEFT JOIN statuses s ON i.status_id = s.id
			WHERE i.workspace_id IN (?` + strings.Repeat(",?", len(affectedWorkspaces)-1) + `)
			GROUP BY i.item_type_id, it.name, s.id, s.name
			ORDER BY it.name, s.name`

		statusArgs := make([]interface{}, len(affectedWorkspaces))
		for i, wsID := range affectedWorkspaces {
			statusArgs[i] = wsID
		}

		statusRows, err := h.db.Query(statusQuery, statusArgs...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer statusRows.Close()

		// Cache workflow statuses by workflow ID to avoid repeated queries
		workflowStatusesCache := make(map[int]map[string]models.Status)

		for statusRows.Next() {
			var itemTypeID sql.NullInt64
			var itemTypeName string
			var currentStatusID int
			var currentStatusName string
			var itemCount int

			if err := statusRows.Scan(&itemTypeID, &itemTypeName, &currentStatusID, &currentStatusName, &itemCount); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			totalAffectedItems += itemCount

			// Get the workflow for this item type using WorkflowService
			itemTypeIDPtr := utils.NullInt64ToPtr(itemTypeID)

			// Use first workspace for workflow lookup (they all share the same config set)
			workflowID, err := workflowService.GetWorkflowIDForItem(affectedWorkspaces[0], itemTypeIDPtr)
			if err != nil {
				http.Error(w, "Failed to get workflow for item type: "+err.Error(), http.StatusInternalServerError)
				return
			}

			migration := models.StatusMigrationInfo{
				CurrentStatus:     currentStatusName,
				CurrentStatusID:   &currentStatusID,
				ItemTypeID:        itemTypeIDPtr,
				ItemTypeName:      itemTypeName,
				RequiresMigration: false,
				ItemCount:         itemCount,
			}

			// No workflow configured - no migration needed for this item type
			if workflowID == nil {
				statusMigrations = append(statusMigrations, migration)
				continue
			}

			// Get or cache workflow statuses
			workflowStatuses, exists := workflowStatusesCache[*workflowID]
			if !exists {
				workflowStatuses = make(map[string]models.Status)
				workflowStatusQuery := `
					SELECT DISTINCT s.id, s.name
					FROM workflow_transitions wt
					JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
					WHERE wt.workflow_id = ?
					ORDER BY s.name`

				workflowStatusRows, err := h.db.Query(workflowStatusQuery, *workflowID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				for workflowStatusRows.Next() {
					var status models.Status
					if err := workflowStatusRows.Scan(&status.ID, &status.Name); err != nil {
						workflowStatusRows.Close()
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					normalizedName := normalizeStatusName(status.Name)
					workflowStatuses[normalizedName] = status
				}
				workflowStatusRows.Close()

				workflowStatusesCache[*workflowID] = workflowStatuses
			}

			// Check if current status exists in workflow
			normalizedCurrentStatus := normalizeStatusName(currentStatusName)
			if workflowStatus, exists := workflowStatuses[normalizedCurrentStatus]; exists {
				migration.SuggestedStatusID = &workflowStatus.ID
				migration.SuggestedStatusName = workflowStatus.Name
			} else {
				migration.RequiresMigration = true
				requiresMigration = true
				h.suggestStatusMapping(&migration, normalizedCurrentStatus, workflowStatuses)
			}

			statusMigrations = append(statusMigrations, migration)
		}
	} else {
		// Original behavior: no item type differentiation, use config set default workflow
		if configSet.WorkflowID == nil {
			analysis := models.WorkflowMigrationAnalysis{
				NewWorkflowID:      configSet.WorkflowID,
				NewWorkflowName:    configSet.WorkflowName,
				AffectedWorkspaces: affectedWorkspaces,
				StatusMigrations:   []models.StatusMigrationInfo{},
				RequiresMigration:  false,
				TotalAffectedItems: 0,
			}
			respondJSONOK(w, analysis)
			return
		}

		// Get current statuses used in affected workspaces and their counts
		statusQuery := `
			SELECT COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name, COUNT(*) as item_count
			FROM items i
			LEFT JOIN statuses s ON i.status_id = s.id
			WHERE i.workspace_id IN (?` + strings.Repeat(",?", len(affectedWorkspaces)-1) + `)
			GROUP BY s.id, s.name
			ORDER BY s.name`

		statusArgs := make([]interface{}, len(affectedWorkspaces))
		for i, wsID := range affectedWorkspaces {
			statusArgs[i] = wsID
		}

		statusRows, err := h.db.Query(statusQuery, statusArgs...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer statusRows.Close()

		// Get available statuses in the workflow
		workflowStatusQuery := `
			SELECT DISTINCT s.id, s.name
			FROM workflow_transitions wt
			JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
			WHERE wt.workflow_id = ?
			ORDER BY s.name`

		workflowStatusRows, err := h.db.Query(workflowStatusQuery, *configSet.WorkflowID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer workflowStatusRows.Close()

		// Build map of available workflow statuses
		workflowStatuses := make(map[string]models.Status)
		for workflowStatusRows.Next() {
			var status models.Status
			if err := workflowStatusRows.Scan(&status.ID, &status.Name); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			normalizedName := normalizeStatusName(status.Name)
			workflowStatuses[normalizedName] = status
		}

		// Analyze each current status
		for statusRows.Next() {
			var currentStatusID int
			var currentStatusName string
			var itemCount int

			if err := statusRows.Scan(&currentStatusID, &currentStatusName, &itemCount); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			totalAffectedItems += itemCount

			migration := models.StatusMigrationInfo{
				CurrentStatus:     currentStatusName,
				CurrentStatusID:   &currentStatusID,
				RequiresMigration: false,
				ItemCount:         itemCount,
			}

			normalizedCurrentStatus := normalizeStatusName(currentStatusName)
			if workflowStatus, exists := workflowStatuses[normalizedCurrentStatus]; exists {
				migration.SuggestedStatusID = &workflowStatus.ID
				migration.SuggestedStatusName = workflowStatus.Name
			} else {
				migration.RequiresMigration = true
				requiresMigration = true
				h.suggestStatusMapping(&migration, normalizedCurrentStatus, workflowStatuses)
			}

			statusMigrations = append(statusMigrations, migration)
		}
	}

	analysis := models.WorkflowMigrationAnalysis{
		NewWorkflowID:      configSet.WorkflowID,
		NewWorkflowName:    configSet.WorkflowName,
		AffectedWorkspaces: affectedWorkspaces,
		StatusMigrations:   statusMigrations,
		RequiresMigration:  requiresMigration,
		TotalAffectedItems: totalAffectedItems,
	}

	respondJSONOK(w, analysis)
}

// AnalyzeComprehensiveMigration analyzes all migration dimensions when moving a workspace to a new config set
// It compares item types, custom fields, priorities, and statuses between old and new configuration sets
func (h *ConfigurationSetHandler) AnalyzeComprehensiveMigration(w http.ResponseWriter, r *http.Request) {
	targetConfigSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// workspace_id is required for comprehensive migration
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	if workspaceIDStr == "" {
		http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
		return
	}
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		http.Error(w, "Invalid workspace_id parameter", http.StatusBadRequest)
		return
	}

	// Get workspace's current configuration set
	var sourceConfigSetID sql.NullInt64
	var sourceConfigSetName string
	err = h.db.QueryRow(`
		SELECT wcs.configuration_set_id, COALESCE(cs.name, '') as config_set_name
		FROM workspace_configuration_sets wcs
		LEFT JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&sourceConfigSetID, &sourceConfigSetName)

	if err == sql.ErrNoRows {
		// Workspace has no config set assigned - treat source as having no restrictions
		// But still need to check if items are compatible with target restrictions
		sourceConfigSetID = sql.NullInt64{Int64: 0, Valid: false}
		sourceConfigSetName = "(No Configuration Set)"
		// Don't return early - continue with migration analysis
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If workspace is already assigned to the target config set, no migration needed
	if sourceConfigSetID.Valid && int(sourceConfigSetID.Int64) == targetConfigSetID {
		analysis := models.ComprehensiveMigrationAnalysis{
			OldConfigSetID:     targetConfigSetID,
			NewConfigSetID:     targetConfigSetID,
			AffectedWorkspaces: []int{workspaceID},
			RequiresMigration:  false,
		}
		respondJSONOK(w, analysis)
		return
	}

	sourceID := int(sourceConfigSetID.Int64)

	// Get target configuration set details
	var targetConfigSetName string
	var targetWorkflowID sql.NullInt64
	var targetWorkflowName sql.NullString
	err = h.db.QueryRow(`
		SELECT cs.name, cs.workflow_id, wf.name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, targetConfigSetID).Scan(&targetConfigSetName, &targetWorkflowID, &targetWorkflowName)
	if err == sql.ErrNoRows {
		http.Error(w, "Target configuration set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Count total items in workspace
	var totalItems int
	h.db.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = ?`, workspaceID).Scan(&totalItems)

	// Initialize analysis
	analysis := models.ComprehensiveMigrationAnalysis{
		OldConfigSetID:     sourceID,
		OldConfigSetName:   sourceConfigSetName,
		NewConfigSetID:     targetConfigSetID,
		NewConfigSetName:   targetConfigSetName,
		AffectedWorkspaces: []int{workspaceID},
		TotalAffectedItems: totalItems,
		NewWorkflowID:      utils.NullInt64ToPtr(targetWorkflowID),
		NewWorkflowName:    targetWorkflowName.String,
	}

	// Analyze item types
	itemTypeMigrations, availableItemTypes, requiresItemTypeMigration := h.analyzeItemTypeMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.ItemTypeMigrations = itemTypeMigrations
	analysis.AvailableItemTypes = availableItemTypes
	analysis.RequiresItemTypeMigration = requiresItemTypeMigration

	// Analyze custom fields
	customFieldMigrations, requiresFieldMigration := h.analyzeCustomFieldMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.CustomFieldMigrations = customFieldMigrations
	analysis.RequiresFieldMigration = requiresFieldMigration

	// Analyze priorities
	priorityMigrations, availablePriorities, requiresPriorityMigration := h.analyzePriorityMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.PriorityMigrations = priorityMigrations
	analysis.AvailablePriorities = availablePriorities
	analysis.RequiresPriorityMigration = requiresPriorityMigration

	// Analyze statuses (reuse existing logic)
	statusMigrations, requiresStatusMigration := h.analyzeStatusMigration(workspaceID, targetConfigSetID)
	analysis.StatusMigrations = statusMigrations
	analysis.RequiresStatusMigration = requiresStatusMigration

	// Overall migration flag
	analysis.RequiresMigration = requiresItemTypeMigration || requiresFieldMigration ||
		requiresPriorityMigration || requiresStatusMigration

	respondJSONOK(w, analysis)
}

// analyzeItemTypeMigration compares item types between source and target config sets
func (h *ConfigurationSetHandler) analyzeItemTypeMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.ItemTypeMigrationInfo, []models.ItemTypeTarget, bool) {
	// Get item types in source config set
	sourceItemTypes := make(map[int]string)
	rows, err := h.db.Query(`
		SELECT it.id, it.name
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		WHERE csit.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var name string
			rows.Scan(&id, &name)
			sourceItemTypes[id] = name
		}
	}

	// Get item types in target config set
	targetItemTypes := make(map[int]models.ItemTypeTarget)
	var availableTargets []models.ItemTypeTarget
	rows, err = h.db.Query(`
		SELECT it.id, it.name, it.icon, it.color, it.hierarchy_level
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		WHERE csit.configuration_set_id = ?
		ORDER BY it.hierarchy_level, it.sort_order
	`, targetConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var target models.ItemTypeTarget
			rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel)
			targetItemTypes[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
	}

	// If target config set has no explicit item types, it accepts ALL item types
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, hierarchy_level
			FROM item_types
			ORDER BY hierarchy_level, sort_order
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var target models.ItemTypeTarget
				rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel)
				targetItemTypes[target.ID] = target
				availableTargets = append(availableTargets, target)
			}
		}
	}

	// Build map by normalized name for suggestion matching
	targetByName := make(map[string]models.ItemTypeTarget)
	for _, t := range targetItemTypes {
		targetByName[strings.ToLower(t.Name)] = t
	}

	// Count items by type in workspace
	var migrations []models.ItemTypeMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(i.item_type_id, 0) as type_id,
		       COALESCE(it.name, '(No Type)') as type_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.workspace_id = ?
		GROUP BY i.item_type_id, it.name
		ORDER BY it.name
	`, workspaceID)
	if err != nil {
		return migrations, availableTargets, false
	}
	defer rows.Close()

	for rows.Next() {
		var typeID int
		var typeName string
		var itemCount int
		rows.Scan(&typeID, &typeName, &itemCount)

		migration := models.ItemTypeMigrationInfo{
			CurrentItemTypeName: typeName,
			ItemCount:           itemCount,
			AvailableTargets:    availableTargets,
		}

		if typeID == 0 {
			migration.CurrentItemTypeID = nil
		} else {
			migration.CurrentItemTypeID = &typeID
		}

		// Check if type exists in target by ID
		if target, exists := targetItemTypes[typeID]; exists {
			migration.SuggestedItemTypeID = &target.ID
			migration.SuggestedItemTypeName = target.Name
			migration.RequiresMigration = false
		} else if target, exists := targetByName[strings.ToLower(typeName)]; exists {
			// Match by name
			migration.SuggestedItemTypeID = &target.ID
			migration.SuggestedItemTypeName = target.Name
			migration.RequiresMigration = false
		} else if typeID == 0 && len(availableTargets) > 0 {
			// No type set - suggest first available
			migration.SuggestedItemTypeID = &availableTargets[0].ID
			migration.SuggestedItemTypeName = availableTargets[0].Name
			migration.RequiresMigration = true
			requiresMigration = true
		} else if typeID != 0 {
			// Type not in target config set
			migration.RequiresMigration = true
			requiresMigration = true
		}

		migrations = append(migrations, migration)
	}

	return migrations, availableTargets, requiresMigration
}

// analyzeCustomFieldMigration compares custom fields between source and target screens
func (h *ConfigurationSetHandler) analyzeCustomFieldMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.CustomFieldMigrationInfo, bool) {
	// Get custom fields from source config set screens
	sourceFields := make(map[int]struct {
		name      string
		fieldType string
	})
	rows, err := h.db.Query(`
		SELECT DISTINCT cfd.id, cfd.name, cfd.field_type
		FROM configuration_set_screens css
		JOIN screen_fields sf ON css.screen_id = sf.screen_id
		JOIN custom_field_definitions cfd ON sf.field_type = 'custom'
			AND CAST(sf.field_identifier AS INTEGER) = cfd.id
		WHERE css.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var name, fieldType string
			rows.Scan(&id, &name, &fieldType)
			sourceFields[id] = struct {
				name      string
				fieldType string
			}{name, fieldType}
		}
	}

	// Get custom fields from target config set screens
	targetFields := make(map[int]struct {
		name      string
		fieldType string
		required  bool
	})
	rows, err = h.db.Query(`
		SELECT DISTINCT cfd.id, cfd.name, cfd.field_type, sf.is_required
		FROM configuration_set_screens css
		JOIN screen_fields sf ON css.screen_id = sf.screen_id
		JOIN custom_field_definitions cfd ON sf.field_type = 'custom'
			AND CAST(sf.field_identifier AS INTEGER) = cfd.id
		WHERE css.configuration_set_id = ?
	`, targetConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var name, fieldType string
			var required bool
			rows.Scan(&id, &name, &fieldType, &required)
			targetFields[id] = struct {
				name      string
				fieldType string
				required  bool
			}{name, fieldType, required}
		}
	}

	var migrations []models.CustomFieldMigrationInfo
	requiresMigration := false

	// Count items with values for each source field
	fieldValueCounts := make(map[int]int)
	rows, err = h.db.Query(`
		SELECT custom_field_values FROM items
		WHERE workspace_id = ?
		AND custom_field_values IS NOT NULL
		AND custom_field_values != ''
		AND custom_field_values != '{}'
	`, workspaceID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cfvJSON string
			rows.Scan(&cfvJSON)
			var cfv map[string]interface{}
			if json.Unmarshal([]byte(cfvJSON), &cfv) == nil {
				for key := range cfv {
					if fieldID, err := strconv.Atoi(key); err == nil {
						fieldValueCounts[fieldID]++
					}
				}
			}
		}
	}

	// Analyze each source field
	for fieldID, sourceField := range sourceFields {
		if _, existsInTarget := targetFields[fieldID]; existsInTarget {
			// Field exists in both - keep
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:   fieldID,
				FieldName: sourceField.name,
				FieldType: sourceField.fieldType,
				ItemCount: fieldValueCounts[fieldID],
				Action:    "keep",
			})
		} else {
			// Field in source but not target - orphan (data preserved but hidden)
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:   fieldID,
				FieldName: sourceField.name,
				FieldType: sourceField.fieldType,
				ItemCount: fieldValueCounts[fieldID],
				Action:    "orphan",
			})
		}
	}

	// Check for new required fields in target that aren't in source
	for fieldID, targetField := range targetFields {
		if _, existsInSource := sourceFields[fieldID]; !existsInSource && targetField.required {
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:         fieldID,
				FieldName:       targetField.name,
				FieldType:       targetField.fieldType,
				ItemCount:       0,
				Action:          "add_default",
				RequiresDefault: true,
			})
			requiresMigration = true
		}
	}

	return migrations, requiresMigration
}

// analyzePriorityMigration compares priorities between source and target config sets
func (h *ConfigurationSetHandler) analyzePriorityMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.PriorityMigrationInfo, []models.PriorityTarget, bool) {
	// Get priorities in source config set
	sourcePriorities := make(map[int]string)
	rows, err := h.db.Query(`
		SELECT p.id, p.name
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var name string
			rows.Scan(&id, &name)
			sourcePriorities[id] = name
		}
	}

	// Get priorities in target config set
	targetPriorities := make(map[int]models.PriorityTarget)
	var availableTargets []models.PriorityTarget
	rows, err = h.db.Query(`
		SELECT p.id, p.name, p.icon, p.color, p.sort_order
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
		ORDER BY p.sort_order
	`, targetConfigSetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var target models.PriorityTarget
			rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder)
			targetPriorities[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
	}

	// If target config set has no explicit priorities, it accepts ALL priorities
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, sort_order
			FROM priorities
			ORDER BY sort_order
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var target models.PriorityTarget
				rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder)
				targetPriorities[target.ID] = target
				availableTargets = append(availableTargets, target)
			}
		}
	}

	// Build map by normalized name for suggestion matching
	targetByName := make(map[string]models.PriorityTarget)
	for _, t := range targetPriorities {
		targetByName[strings.ToLower(t.Name)] = t
	}

	// Count items by priority in workspace
	var migrations []models.PriorityMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(i.priority_id, 0) as priority_id,
		       COALESCE(p.name, '(No Priority)') as priority_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.workspace_id = ?
		GROUP BY i.priority_id, p.name
		ORDER BY p.name
	`, workspaceID)
	if err != nil {
		return migrations, availableTargets, false
	}
	defer rows.Close()

	for rows.Next() {
		var priorityID int
		var priorityName string
		var itemCount int
		rows.Scan(&priorityID, &priorityName, &itemCount)

		migration := models.PriorityMigrationInfo{
			CurrentPriorityName: priorityName,
			ItemCount:           itemCount,
		}

		if priorityID == 0 {
			migration.CurrentPriorityID = nil
		} else {
			migration.CurrentPriorityID = &priorityID
		}

		// Check if priority exists in target by ID
		if target, exists := targetPriorities[priorityID]; exists {
			migration.SuggestedPriorityID = &target.ID
			migration.SuggestedPriorityName = target.Name
			migration.RequiresMigration = false
		} else if target, exists := targetByName[strings.ToLower(priorityName)]; exists {
			// Match by name
			migration.SuggestedPriorityID = &target.ID
			migration.SuggestedPriorityName = target.Name
			migration.RequiresMigration = false
		} else if priorityID == 0 {
			// No priority set - this is ok, don't require migration
			migration.RequiresMigration = false
		} else {
			// Priority not in target config set
			migration.RequiresMigration = true
			requiresMigration = true
		}

		migrations = append(migrations, migration)
	}

	return migrations, availableTargets, requiresMigration
}

// analyzeStatusMigration analyzes status migration for a workspace moving to a new config set
func (h *ConfigurationSetHandler) analyzeStatusMigration(workspaceID, targetConfigSetID int) ([]models.StatusMigrationInfo, bool) {
	// Get target workflow
	var targetWorkflowID sql.NullInt64
	h.db.QueryRow(`
		SELECT workflow_id FROM configuration_sets WHERE id = ?
	`, targetConfigSetID).Scan(&targetWorkflowID)

	if !targetWorkflowID.Valid {
		// No workflow configured - no status migration needed
		return nil, false
	}

	// Get available statuses in target workflow
	workflowStatuses := make(map[string]models.Status)
	rows, err := h.db.Query(`
		SELECT DISTINCT s.id, s.name
		FROM workflow_transitions wt
		JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
		WHERE wt.workflow_id = ?
	`, targetWorkflowID.Int64)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	for rows.Next() {
		var status models.Status
		rows.Scan(&status.ID, &status.Name)
		normalizedName := normalizeStatusName(status.Name)
		workflowStatuses[normalizedName] = status
	}

	// Get current statuses used in workspace
	var migrations []models.StatusMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(s.id, 0) as status_id,
		       COALESCE(s.name, '') as status_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id = ?
		GROUP BY s.id, s.name
		ORDER BY s.name
	`, workspaceID)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	for rows.Next() {
		var statusID int
		var statusName string
		var itemCount int
		rows.Scan(&statusID, &statusName, &itemCount)

		migration := models.StatusMigrationInfo{
			CurrentStatus:   statusName,
			CurrentStatusID: &statusID,
			ItemCount:       itemCount,
		}

		normalizedStatus := normalizeStatusName(statusName)
		if status, exists := workflowStatuses[normalizedStatus]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
			migration.RequiresMigration = false
		} else {
			migration.RequiresMigration = true
			requiresMigration = true
			h.suggestStatusMapping(&migration, normalizedStatus, workflowStatuses)
		}

		migrations = append(migrations, migration)
	}

	return migrations, requiresMigration
}

// suggestStatusMapping suggests a reasonable default status based on common status mappings
func (h *ConfigurationSetHandler) suggestStatusMapping(migration *models.StatusMigrationInfo, normalizedCurrentStatus string, workflowStatuses map[string]models.Status) {
	switch normalizedCurrentStatus {
	case "open", "new", "to do", "todo":
		if status, exists := workflowStatuses["to do"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		} else if status, exists := workflowStatuses["open"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "in progress", "doing":
		if status, exists := workflowStatuses["in progress"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "completed", "done", "closed":
		if status, exists := workflowStatuses["done"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		} else if status, exists := workflowStatuses["completed"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "cancelled", "canceled":
		if status, exists := workflowStatuses["cancelled"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	}
}

func (h *ConfigurationSetHandler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	var migrationReq models.WorkflowMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&migrationReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate configuration set exists
	var configSetExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", migrationReq.ConfigurationSetID).Scan(&configSetExists)
	if err != nil || !configSetExists {
		http.Error(w, "Configuration set not found", http.StatusBadRequest)
		return
	}

	// Validate workspace IDs provided
	if len(migrationReq.WorkspaceIDs) == 0 {
		http.Error(w, "At least one workspace ID is required", http.StatusBadRequest)
		return
	}

	// Validate all target status IDs exist
	for _, mapping := range migrationReq.StatusMappings {
		var statusExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", mapping.ToStatusID).Scan(&statusExists)
		if err != nil || !statusExists {
			http.Error(w, fmt.Sprintf("Target status ID %d not found", mapping.ToStatusID), http.StatusBadRequest)
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	totalMigrated := 0

	// Execute status migrations
	for _, mapping := range migrationReq.StatusMappings {
		var updateQuery string
		var updateArgs []interface{}

		// Build query with optional item_type_id filter
		if mapping.ItemTypeID != nil {
			// Update only items of specific type
			updateQuery = `
				UPDATE items
				SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND item_type_id = ?
				AND workspace_id IN (?` + strings.Repeat(",?", len(migrationReq.WorkspaceIDs)-1) + `)`

			updateArgs = []interface{}{mapping.ToStatusID, time.Now(), mapping.FromStatusID, *mapping.ItemTypeID}
		} else {
			// Update all items regardless of type (original behavior)
			updateQuery = `
				UPDATE items
				SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND workspace_id IN (?` + strings.Repeat(",?", len(migrationReq.WorkspaceIDs)-1) + `)`

			updateArgs = []interface{}{mapping.ToStatusID, time.Now(), mapping.FromStatusID}
		}

		for _, wsID := range migrationReq.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := result.RowsAffected()
		totalMigrated += int(rowsAffected)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":        true,
		"message":        fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items": totalMigrated,
	}

	respondJSONOK(w, response)
}

// ExecuteComprehensiveMigration executes all migration dimensions in a single transaction
func (h *ConfigurationSetHandler) ExecuteComprehensiveMigration(w http.ResponseWriter, r *http.Request) {
	var req models.ComprehensiveMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate configuration sets exist
	var oldConfigSetExists, newConfigSetExists bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", req.OldConfigurationSetID).Scan(&oldConfigSetExists)
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", req.NewConfigurationSetID).Scan(&newConfigSetExists)

	if !oldConfigSetExists {
		http.Error(w, "Old configuration set not found", http.StatusBadRequest)
		return
	}
	if !newConfigSetExists {
		http.Error(w, "New configuration set not found", http.StatusBadRequest)
		return
	}

	// Validate workspace IDs provided
	if len(req.WorkspaceIDs) == 0 {
		http.Error(w, "At least one workspace ID is required", http.StatusBadRequest)
		return
	}

	// Validate all target IDs exist
	for _, mapping := range req.ItemTypeMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", mapping.ToItemTypeID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target item type ID %d not found", mapping.ToItemTypeID), http.StatusBadRequest)
			return
		}
	}

	for _, mapping := range req.StatusMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", mapping.ToStatusID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target status ID %d not found", mapping.ToStatusID), http.StatusBadRequest)
			return
		}
	}

	for _, mapping := range req.PriorityMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE id = ?)", mapping.ToPriorityID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target priority ID %d not found", mapping.ToPriorityID), http.StatusBadRequest)
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	stats := struct {
		ItemTypesMigrated   int `json:"item_types_migrated"`
		StatusesMigrated    int `json:"statuses_migrated"`
		PrioritiesMigrated  int `json:"priorities_migrated"`
		CustomFieldsUpdated int `json:"custom_fields_updated"`
	}{}

	// Build workspace placeholders
	wsPlaceholders := "?" + strings.Repeat(",?", len(req.WorkspaceIDs)-1)

	// 1. Execute Item Type Migrations
	for _, mapping := range req.ItemTypeMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.FromItemTypeID == nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET item_type_id = ?, updated_at = ?
				WHERE item_type_id IS NULL
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToItemTypeID, now}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET item_type_id = ?, updated_at = ?
				WHERE item_type_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToItemTypeID, now, *mapping.FromItemTypeID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate item types: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.ItemTypesMigrated += int(rowsAffected)
	}

	// 2. Execute Custom Field Migrations (only add_default needs action)
	for _, mapping := range req.CustomFieldMappings {
		if mapping.Action == "add_default" && mapping.DefaultValue != nil {
			count, err := h.addDefaultFieldValue(tx, req.WorkspaceIDs, mapping.FieldID, mapping.DefaultValue)
			if err != nil {
				http.Error(w, "Failed to add default field values: "+err.Error(), http.StatusInternalServerError)
				return
			}
			stats.CustomFieldsUpdated += count
		}
		// "keep" and "orphan" require no action - data is preserved in JSON
	}

	// 3. Execute Status Migrations
	for _, mapping := range req.StatusMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.ItemTypeID != nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND item_type_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToStatusID, now, mapping.FromStatusID, *mapping.ItemTypeID}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToStatusID, now, mapping.FromStatusID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate statuses: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.StatusesMigrated += int(rowsAffected)
	}

	// 4. Execute Priority Migrations
	for _, mapping := range req.PriorityMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.FromPriorityID == nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET priority_id = ?, updated_at = ?
				WHERE priority_id IS NULL
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToPriorityID, now}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET priority_id = ?, updated_at = ?
				WHERE priority_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToPriorityID, now, *mapping.FromPriorityID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate priorities: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.PrioritiesMigrated += int(rowsAffected)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalMigrated := stats.ItemTypesMigrated + stats.StatusesMigrated + stats.PrioritiesMigrated

	response := map[string]interface{}{
		"success":        true,
		"message":        fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items": totalMigrated,
		"details":        stats,
	}

	respondJSONOK(w, response)
}

// addDefaultFieldValue adds a default value for a custom field to items that don't have it
func (h *ConfigurationSetHandler) addDefaultFieldValue(tx database.Tx, workspaceIDs []int, fieldID int, defaultValue interface{}) (int, error) {
	fieldKey := strconv.Itoa(fieldID)
	count := 0

	// Build workspace placeholders
	wsPlaceholders := "?" + strings.Repeat(",?", len(workspaceIDs)-1)
	wsArgs := make([]interface{}, len(workspaceIDs))
	for i, wsID := range workspaceIDs {
		wsArgs[i] = wsID
	}

	// Get all items in the workspaces
	query := fmt.Sprintf(`
		SELECT id, COALESCE(custom_field_values, '{}') as cfv
		FROM items
		WHERE workspace_id IN (%s)
	`, wsPlaceholders)

	rows, err := tx.Query(query, wsArgs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type itemUpdate struct {
		id     int
		newCFV string
	}
	var updates []itemUpdate

	for rows.Next() {
		var id int
		var cfvJSON string
		if err := rows.Scan(&id, &cfvJSON); err != nil {
			return 0, err
		}

		var cfv map[string]interface{}
		if err := json.Unmarshal([]byte(cfvJSON), &cfv); err != nil {
			cfv = make(map[string]interface{})
		}

		// Only add if field not already set
		if _, exists := cfv[fieldKey]; !exists {
			cfv[fieldKey] = defaultValue
			newJSON, err := json.Marshal(cfv)
			if err != nil {
				return 0, err
			}
			updates = append(updates, itemUpdate{id: id, newCFV: string(newJSON)})
		}
	}

	// Apply updates
	now := time.Now()
	for _, update := range updates {
		_, err := tx.Exec(`UPDATE items SET custom_field_values = ?, updated_at = ? WHERE id = ?`,
			update.newCFV, now, update.id)
		if err != nil {
			return 0, err
		}
		count++
	}

	return count, nil
}
