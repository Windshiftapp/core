package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type BoardConfigurationHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewBoardConfigurationHandler(db database.Database, permissionService *services.PermissionService) *BoardConfigurationHandler {
	return &BoardConfigurationHandler{db: db, permissionService: permissionService}
}

// checkCollectionAccess verifies the user can READ the collection (public or
// owned by user). Returns true if access is granted, false if denied
// (response already written). Do NOT use this for write paths — see
// checkCollectionWriteAccess.
func (h *BoardConfigurationHandler) checkCollectionAccess(w http.ResponseWriter, r *http.Request, collectionID int) bool {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return false
	}

	var isPublic bool
	var createdBy sql.NullInt64
	err := h.db.QueryRow("SELECT is_public, created_by FROM collections WHERE id = ?", collectionID).
		Scan(&isPublic, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "collection")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}

	if !isPublic && (!createdBy.Valid || int(createdBy.Int64) != currentUser.ID) {
		respondNotFound(w, r, "collection")
		return false
	}
	return true
}

// checkCollectionWriteAccess verifies the user can MUTATE board configs for
// the collection. `is_public = true` does NOT grant write access — only
// ownership (created_by == currentUser.ID) does. Returns 404 on denial to
// avoid leaking collection existence.
func (h *BoardConfigurationHandler) checkCollectionWriteAccess(w http.ResponseWriter, r *http.Request, collectionID int) bool {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return false
	}

	var createdBy sql.NullInt64
	err := h.db.QueryRow("SELECT created_by FROM collections WHERE id = ?", collectionID).Scan(&createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "collection")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}

	if !createdBy.Valid || int(createdBy.Int64) != currentUser.ID {
		respondNotFound(w, r, "collection")
		return false
	}
	return true
}

// checkBoardConfigWriteAccess looks up the collection/workspace associated
// with a board config and verifies the user has WRITE access.
func (h *BoardConfigurationHandler) checkBoardConfigWriteAccess(w http.ResponseWriter, r *http.Request, configID int) bool {
	collID, wsID, ok := h.loadBoardConfigScope(w, r, configID)
	if !ok {
		return false
	}
	if wsID.Valid {
		return h.checkWorkspaceWriteAccess(w, r, int(wsID.Int64))
	}
	if collID.Valid {
		return h.checkCollectionWriteAccess(w, r, int(collID.Int64))
	}
	return true
}

// loadBoardConfigScope reads the (collection_id, workspace_id) pair for a
// board config or writes the appropriate not-found response.
func (h *BoardConfigurationHandler) loadBoardConfigScope(w http.ResponseWriter, r *http.Request, configID int) (collID, wsID sql.NullInt64, ok bool) {
	err := h.db.QueryRow("SELECT collection_id, workspace_id FROM board_configurations WHERE id = ?", configID).
		Scan(&collID, &wsID)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "board_configuration")
		return collID, wsID, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return collID, wsID, false
	}
	return collID, wsID, true
}

// checkWorkspaceAccess verifies the user has READ (`item.view`) permission on
// the workspace. Returns 404 on permission denial to prevent workspace
// existence leakage.
func (h *BoardConfigurationHandler) checkWorkspaceAccess(w http.ResponseWriter, r *http.Request, workspaceID int) bool {
	return h.checkWorkspacePerm(w, r, workspaceID, models.PermissionItemView)
}

// checkWorkspaceWriteAccess verifies the user has ADMIN (`workspace.admin`)
// permission on the workspace. The workspace-default board configuration
// reshapes the layout (columns, backlog, list/card fields, roadmap) for every
// viewer of the workspace, so write access is gated to workspace admins —
// `item.edit` is not enough. Returns 404 on permission denial.
func (h *BoardConfigurationHandler) checkWorkspaceWriteAccess(w http.ResponseWriter, r *http.Request, workspaceID int) bool {
	return h.checkWorkspacePerm(w, r, workspaceID, models.PermissionWorkspaceAdmin)
}

func (h *BoardConfigurationHandler) checkWorkspacePerm(w http.ResponseWriter, r *http.Request, workspaceID int, perm string) bool {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return false
	}
	hasPermission, err := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, perm)
	if err != nil || !hasPermission {
		respondNotFound(w, r, "board_configuration")
		return false
	}
	return true
}

// GetByCollection returns the board configuration for a specific collection or workspace
func (h *BoardConfigurationHandler) GetByCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	slog.Info("getting board configuration", "id", id, "workspace_id", r.URL.Query().Get("workspace_id"))

	var config models.BoardConfiguration
	var err error

	// Check if this is a workspace-level config request
	if id == "default" {
		// Workspace-level configuration
		workspaceIDStr := r.URL.Query().Get("workspace_id")
		if workspaceIDStr == "" {
			respondValidationError(w, r, "workspace_id query parameter required for default configuration")
			return
		}

		workspaceID, parseErr := strconv.Atoi(workspaceIDStr)
		if parseErr != nil {
			respondInvalidID(w, r, "workspace_id")
			return
		}

		if !h.checkWorkspaceAccess(w, r, workspaceID) {
			return
		}

		// Get workspace board configuration
		var collectionID, wsID sql.NullInt64
		var backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON sql.NullString
		err = h.db.QueryRow(`
			SELECT id, collection_id, workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at
			FROM board_configurations
			WHERE workspace_id = ?`,
			workspaceID,
		).Scan(&config.ID, &collectionID, &wsID, &backlogStatusIDsJSON, &listColumnsJSON, &cardFieldsJSON, &roadmapConfigJSON, &config.CreatedAt, &config.UpdatedAt)

		// Every workspace logically has a default board configuration even when
		// no row has been persisted yet — return an empty config scoped to the
		// workspace so the frontend can render defaults without a 404.
		if errors.Is(err, sql.ErrNoRows) {
			wid := workspaceID
			config = models.BoardConfiguration{WorkspaceID: &wid}
			respondJSONOK(w, config)
			return
		}

		if collectionID.Valid {
			cid := int(collectionID.Int64)
			config.CollectionID = &cid
		}
		if wsID.Valid {
			wid := int(wsID.Int64)
			config.WorkspaceID = &wid
		}
		unmarshalBoardConfigFields(&config, backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON)
	} else {
		// Collection-level configuration
		collectionID, parseErr := strconv.Atoi(id)
		if parseErr != nil {
			respondInvalidID(w, r, "id")
			return
		}

		if !h.checkCollectionAccess(w, r, collectionID) {
			return
		}

		var collID, wsID sql.NullInt64
		var backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON sql.NullString
		err = h.db.QueryRow(`
			SELECT id, collection_id, workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at
			FROM board_configurations
			WHERE collection_id = ?`,
			collectionID,
		).Scan(&config.ID, &collID, &wsID, &backlogStatusIDsJSON, &listColumnsJSON, &cardFieldsJSON, &roadmapConfigJSON, &config.CreatedAt, &config.UpdatedAt)

		if collID.Valid {
			cid := int(collID.Int64)
			config.CollectionID = &cid
		}
		if wsID.Valid {
			wid := int(wsID.Int64)
			config.WorkspaceID = &wid
		}
		unmarshalBoardConfigFields(&config, backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON)
	}

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "board_configuration")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get the columns with status mappings
	columns, err := h.getColumnsWithStatuses(config.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	config.Columns = columns

	respondJSONOK(w, config)
}

// CreateForCollection creates a new board configuration for a collection or workspace
func (h *BoardConfigurationHandler) CreateForCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	req, ok := decodeJSON[models.BoardConfigurationRequest](w, r)
	if !ok {
		return
	}

	slog.Info("creating board configuration", "id", id, "columns_count", len(req.Columns), "backlog_status_ids", req.BacklogStatusIDs)

	// Begin transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var configID int64
	var collectionID *int
	var workspaceID *int

	configBytes, ok := marshalBoardConfigFields(w, r, &req)
	if !ok {
		return
	}

	// Check if this is a workspace-level config request
	if id == "default" {
		// Workspace-level configuration
		workspaceIDStr := r.URL.Query().Get("workspace_id")
		if workspaceIDStr == "" {
			respondValidationError(w, r, "workspace_id query parameter required for default configuration")
			return
		}

		wsID, parseErr := strconv.Atoi(workspaceIDStr)
		if parseErr != nil {
			respondInvalidID(w, r, "workspace_id")
			return
		}

		if !h.checkWorkspaceWriteAccess(w, r, wsID) {
			return
		}
		workspaceID = &wsID

		// Create workspace board configuration
		err = tx.QueryRow(`
			INSERT INTO board_configurations (workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			wsID, configBytes.BacklogStatusIDs, configBytes.ListColumns, configBytes.CardFields, configBytes.RoadmapConfig, time.Now(), time.Now(),
		).Scan(&configID)
	} else {
		// Collection-level configuration
		collID, parseErr := strconv.Atoi(id)
		if parseErr != nil {
			respondInvalidID(w, r, "id")
			return
		}

		if !h.checkCollectionWriteAccess(w, r, collID) {
			return
		}
		collectionID = &collID

		err = tx.QueryRow(`
			INSERT INTO board_configurations (collection_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			collID, configBytes.BacklogStatusIDs, configBytes.ListColumns, configBytes.CardFields, configBytes.RoadmapConfig, time.Now(), time.Now(),
		).Scan(&configID)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create columns
	if err := h.createColumns(tx, int(configID), req.Columns); err != nil {
		slog.Error("failed to create board columns", "error", err, "config_id", configID)
		respondInternalError(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the created configuration
	config := models.BoardConfiguration{
		ID:           int(configID),
		CollectionID: collectionID,
		WorkspaceID:  workspaceID,
		ListColumns:  req.ListColumns,
		CardFields:   req.CardFields,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	columns, _ := h.getColumnsWithStatuses(int(configID))
	config.Columns = columns

	respondJSONCreated(w, config)
}

// UpdateForCollection updates the board configuration for a collection
func (h *BoardConfigurationHandler) UpdateForCollection(w http.ResponseWriter, r *http.Request) {
	configID, ok := requireIDParam(w, r, "configId")
	if !ok {
		return
	}

	var err error

	// Verify WRITE access to the board config's collection or workspace
	if !h.checkBoardConfigWriteAccess(w, r, configID) {
		return
	}

	req, ok := decodeJSON[models.BoardConfigurationRequest](w, r)
	if !ok {
		return
	}

	slog.Info("updating board configuration", "config_id", configID, "columns_count", len(req.Columns), "backlog_status_ids", req.BacklogStatusIDs)

	// Begin transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	configBytes, ok := marshalBoardConfigFields(w, r, &req)
	if !ok {
		return
	}

	// Update the configuration
	_, err = tx.Exec(`
		UPDATE board_configurations
		SET backlog_status_ids = ?, list_columns = ?, card_fields = ?, roadmap_config = ?, updated_at = ?
		WHERE id = ?`,
		configBytes.BacklogStatusIDs, configBytes.ListColumns, configBytes.CardFields, configBytes.RoadmapConfig, time.Now(), configID,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get existing columns
	existingColumns, err := h.getColumns(configID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create a map of existing column IDs
	existingIDs := make(map[int]bool)
	for _, col := range existingColumns {
		existingIDs[col.ID] = true
	}

	// Track which columns are in the request
	requestIDs := make(map[int]bool)

	// Update or create columns
	for i, colReq := range req.Columns {
		slog.Info("processing column request", "index", i, "id", colReq.ID, "name", colReq.Name, "status_ids", colReq.StatusIDs)
		if colReq.ID != nil {
			// Update existing column
			slog.Info("updating existing column", "column_id", *colReq.ID, "name", colReq.Name)
			requestIDs[*colReq.ID] = true
			_, err = tx.Exec(`
				UPDATE board_columns
				SET name = ?, display_order = ?, wip_limit = ?, color = ?, updated_at = ?
				WHERE id = ? AND board_configuration_id = ?`,
				colReq.Name, colReq.DisplayOrder, colReq.WIPLimit, colReq.Color, time.Now(),
				*colReq.ID, configID,
			)
			if err != nil {
				slog.Error("failed to update column", "error", err)
				respondInternalError(w, r, err)
				return
			}

			// Delete existing status mappings
			slog.Info("deleting existing status mappings", "column_id", *colReq.ID)
			_, err = tx.Exec(`DELETE FROM board_column_statuses WHERE board_column_id = ?`, *colReq.ID)
			if err != nil {
				slog.Error("failed to delete existing status mappings", "error", err)
				respondInternalError(w, r, err)
				return
			}

			// Create new status mappings
			slog.Info("creating new status mappings", "column_id", *colReq.ID, "status_count", len(colReq.StatusIDs))
			for _, statusID := range colReq.StatusIDs {
				slog.Info("inserting status mapping (update path)", "board_column_id", *colReq.ID, "status_id", statusID)
				_, err = tx.Exec(`
					INSERT INTO board_column_statuses (board_column_id, status_id, created_at)
					VALUES (?, ?, ?)`,
					*colReq.ID, statusID, time.Now(),
				)
				if err != nil {
					slog.Error("FOREIGN KEY ERROR (update path)", "status_id", statusID, "board_column_id", *colReq.ID, "error", err)
					respondInternalError(w, r, fmt.Errorf("failed to insert status mapping for status_id=%d, board_column_id=%d: %w", statusID, *colReq.ID, err))
					return
				}
			}
		} else {
			// Create new column
			slog.Info("creating new column", "name", colReq.Name)
			var colID int64
			err = tx.QueryRow(`
				INSERT INTO board_columns (board_configuration_id, name, display_order, wip_limit, color, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
				configID, colReq.Name, colReq.DisplayOrder, colReq.WIPLimit, colReq.Color, time.Now(), time.Now(),
			).Scan(&colID)
			if err != nil {
				slog.Error("failed to create new column", "error", err)
				respondInternalError(w, r, err)
				return
			}
			slog.Info("new column created", "column_id", colID, "name", colReq.Name)

			// Create status mappings
			slog.Info("creating status mappings for new column", "column_id", colID, "status_count", len(colReq.StatusIDs))
			for _, statusID := range colReq.StatusIDs {
				slog.Info("inserting status mapping (create path)", "board_column_id", colID, "status_id", statusID)
				_, err = tx.Exec(`
					INSERT INTO board_column_statuses (board_column_id, status_id, created_at)
					VALUES (?, ?, ?)`,
					colID, statusID, time.Now(),
				)
				if err != nil {
					slog.Error("FOREIGN KEY ERROR (create path)", "status_id", statusID, "board_column_id", colID, "error", err)
					respondInternalError(w, r, fmt.Errorf("failed to insert status mapping for status_id=%d, board_column_id=%d: %w", statusID, colID, err))
					return
				}
			}
		}
	}

	// Delete columns that are no longer in the request
	for existingID := range existingIDs {
		if !requestIDs[existingID] {
			// Delete status mappings first (cascade should handle this, but be explicit)
			_, err = tx.Exec(`DELETE FROM board_column_statuses WHERE board_column_id = ?`, existingID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			// Delete the column
			_, err = tx.Exec(`DELETE FROM board_columns WHERE id = ?`, existingID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated configuration
	var config models.BoardConfiguration
	var collID, wsID sql.NullInt64
	var backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON sql.NullString
	err = h.db.QueryRow(`
		SELECT id, collection_id, workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, created_at, updated_at
		FROM board_configurations
		WHERE id = ?`,
		configID,
	).Scan(&config.ID, &collID, &wsID, &backlogStatusIDsJSON, &listColumnsJSON, &cardFieldsJSON, &roadmapConfigJSON, &config.CreatedAt, &config.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if collID.Valid {
		cid := int(collID.Int64)
		config.CollectionID = &cid
	}
	if wsID.Valid {
		wid := int(wsID.Int64)
		config.WorkspaceID = &wid
	}
	unmarshalBoardConfigFields(&config, backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON)

	columns, _ := h.getColumnsWithStatuses(configID)
	config.Columns = columns

	respondJSONOK(w, config)
}

// DeleteForCollection deletes the board configuration for a collection
func (h *BoardConfigurationHandler) DeleteForCollection(w http.ResponseWriter, r *http.Request) {
	configID, ok := requireIDParam(w, r, "configId")
	if !ok {
		return
	}

	var err error

	// Verify WRITE access to the board config's collection or workspace
	if !h.checkBoardConfigWriteAccess(w, r, configID) {
		return
	}

	// Delete the configuration (cascade will handle columns and status mappings)
	_, err = h.db.ExecWrite(`DELETE FROM board_configurations WHERE id = ?`, configID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// boardConfigBytes holds the JSON-encoded board configuration fields.
type boardConfigBytes struct {
	BacklogStatusIDs []byte
	ListColumns      []byte
	CardFields       []byte
	RoadmapConfig    []byte
}

// marshalBoardConfigFields marshals the three JSON config fields from a request.
// Returns false (with response written) on error.
func marshalBoardConfigFields(w http.ResponseWriter, r *http.Request, req *models.BoardConfigurationRequest) (*boardConfigBytes, bool) {
	result := &boardConfigBytes{}
	var err error

	if len(req.BacklogStatusIDs) > 0 {
		result.BacklogStatusIDs, err = json.Marshal(req.BacklogStatusIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		slog.Info("marshaled backlog status IDs", "json", string(result.BacklogStatusIDs))
	}

	if len(req.ListColumns) > 0 {
		result.ListColumns, err = json.Marshal(req.ListColumns)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		slog.Info("marshaled list columns", "json", string(result.ListColumns))
	}

	if len(req.CardFields) > 0 {
		result.CardFields, err = json.Marshal(req.CardFields)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
	}

	if req.RoadmapConfig != nil {
		result.RoadmapConfig, err = json.Marshal(req.RoadmapConfig)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
	}

	return result, true
}

// unmarshalBoardConfigFields decodes the three JSON config fields into a BoardConfiguration.
func unmarshalBoardConfigFields(config *models.BoardConfiguration, backlogJSON, listColumnsJSON, cardFieldsJSON, roadmapJSON sql.NullString) {
	if backlogJSON.Valid && backlogJSON.String != "" {
		var backlogStatusIDs []int
		if err := json.Unmarshal([]byte(backlogJSON.String), &backlogStatusIDs); err == nil {
			config.BacklogStatusIDs = backlogStatusIDs
		}
	}
	if listColumnsJSON.Valid && listColumnsJSON.String != "" {
		var listColumns []models.ListColumn
		if err := json.Unmarshal([]byte(listColumnsJSON.String), &listColumns); err == nil {
			config.ListColumns = listColumns
		}
	}
	if cardFieldsJSON.Valid && cardFieldsJSON.String != "" {
		var cardFields []models.ListColumn
		if err := json.Unmarshal([]byte(cardFieldsJSON.String), &cardFields); err == nil {
			config.CardFields = cardFields
		}
	}
	if roadmapJSON.Valid && roadmapJSON.String != "" {
		var roadmapConfig models.RoadmapConfig
		if err := json.Unmarshal([]byte(roadmapJSON.String), &roadmapConfig); err == nil {
			config.RoadmapConfig = &roadmapConfig
		}
	}
}

// Helper functions

func (h *BoardConfigurationHandler) getColumns(configID int) ([]models.BoardColumn, error) {
	rows, err := h.db.Query(`
		SELECT id, board_configuration_id, name, display_order, wip_limit, color, created_at, updated_at
		FROM board_columns
		WHERE board_configuration_id = ?
		ORDER BY display_order`,
		configID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := []models.BoardColumn{}
	for rows.Next() {
		var col models.BoardColumn
		var wipLimit sql.NullInt64
		err := rows.Scan(
			&col.ID, &col.BoardConfigurationID, &col.Name, &col.DisplayOrder,
			&wipLimit, &col.Color, &col.CreatedAt, &col.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if wipLimit.Valid {
			limit := int(wipLimit.Int64)
			col.WIPLimit = &limit
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (h *BoardConfigurationHandler) getColumnsWithStatuses(configID int) ([]models.BoardColumn, error) {
	columns, err := h.getColumns(configID)
	if err != nil {
		return nil, err
	}

	// Get status mappings for all columns
	for i := range columns {
		rows, err := h.db.Query(`
			SELECT status_id
			FROM board_column_statuses
			WHERE board_column_id = ?`,
			columns[i].ID,
		)
		if err != nil {
			return nil, err
		}

		var statusIDs []int
		for rows.Next() {
			var statusID int
			if err := rows.Scan(&statusID); err != nil {
				_ = rows.Close()
				return nil, err
			}
			statusIDs = append(statusIDs, statusID)
		}
		_ = rows.Close()
		columns[i].StatusIDs = statusIDs
	}

	return columns, nil
}

func (h *BoardConfigurationHandler) createColumns(tx database.Tx, configID int, columns []models.BoardColumnRequest) error {
	slog.Info("createColumns called", "config_id", configID, "columns_count", len(columns))
	for i, col := range columns {
		// Create the column
		var colID int64
		slog.Info("creating board column", "index", i, "name", col.Name, "status_ids", col.StatusIDs)
		err := tx.QueryRow(`
			INSERT INTO board_columns (board_configuration_id, name, display_order, wip_limit, color, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			configID, col.Name, col.DisplayOrder, col.WIPLimit, col.Color, time.Now(), time.Now(),
		).Scan(&colID)
		if err != nil {
			slog.Error("failed to create board column", "error", err, "name", col.Name)
			return err
		}
		slog.Info("board column created", "column_id", colID, "name", col.Name)

		// Create status mappings
		for _, statusID := range col.StatusIDs {
			slog.Info("inserting status mapping", "board_column_id", colID, "status_id", statusID)
			_, err = tx.Exec(`
				INSERT INTO board_column_statuses (board_column_id, status_id, created_at)
				VALUES (?, ?, ?)`,
				colID, statusID, time.Now(),
			)
			if err != nil {
				slog.Error("FOREIGN KEY ERROR", "status_id", statusID, "board_column_id", colID, "error", err)
				return fmt.Errorf("failed to insert status mapping for status_id=%d, board_column_id=%d: %w", statusID, colID, err)
			}
		}
	}
	return nil
}
