package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// BoardConfigurationRepository owns SQL access to board_configurations,
// board_columns and board_column_statuses.
type BoardConfigurationRepository struct {
	db database.Database
}

// NewBoardConfigurationRepository creates a board configuration repository.
func NewBoardConfigurationRepository(db database.Database) *BoardConfigurationRepository {
	return &BoardConfigurationRepository{db: db}
}

// GetScope returns the (collection_id, workspace_id) pair for a board config.
// Returns ErrNotFound when the configuration does not exist.
func (r *BoardConfigurationRepository) GetScope(configID int) (collectionID, workspaceID *int, err error) {
	var collID, wsID sql.NullInt64
	err = r.db.QueryRow("SELECT collection_id, workspace_id FROM board_configurations WHERE id = ?", configID).
		Scan(&collID, &wsID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if collID.Valid {
		v := int(collID.Int64)
		collectionID = &v
	}
	if wsID.Valid {
		v := int(wsID.Int64)
		workspaceID = &v
	}
	return collectionID, workspaceID, nil
}

// GetByID fetches a board configuration by its id (without columns).
func (r *BoardConfigurationRepository) GetByID(configID int) (*models.BoardConfiguration, error) {
	return r.getConfig("id = ?", configID)
}

// GetByWorkspaceID fetches the workspace-default board configuration (without columns).
func (r *BoardConfigurationRepository) GetByWorkspaceID(workspaceID int) (*models.BoardConfiguration, error) {
	return r.getConfig("workspace_id = ?", workspaceID)
}

// GetByCollectionID fetches the board configuration for a collection (without columns).
func (r *BoardConfigurationRepository) GetByCollectionID(collectionID int) (*models.BoardConfiguration, error) {
	return r.getConfig("collection_id = ?", collectionID)
}

func (r *BoardConfigurationRepository) getConfig(where string, arg any) (*models.BoardConfiguration, error) {
	var config models.BoardConfiguration
	var collID, wsID sql.NullInt64
	var backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON, viewSettingsJSON sql.NullString
	var completedItemRetentionDays sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, collection_id, workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, view_settings, show_rightmost_column_last_50, completed_item_retention_days, created_at, updated_at
		FROM board_configurations
		WHERE `+where,
		arg,
	).Scan(&config.ID, &collID, &wsID, &backlogStatusIDsJSON, &listColumnsJSON, &cardFieldsJSON, &roadmapConfigJSON, &viewSettingsJSON, &config.ShowRightmostColumnLast50, &completedItemRetentionDays, &config.CreatedAt, &config.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if collID.Valid {
		cid := int(collID.Int64)
		config.CollectionID = &cid
	}
	if wsID.Valid {
		wid := int(wsID.Int64)
		config.WorkspaceID = &wid
	}
	if completedItemRetentionDays.Valid {
		days := int(completedItemRetentionDays.Int64)
		config.CompletedItemRetentionDays = &days
	}
	unmarshalBoardConfigFields(&config, backlogStatusIDsJSON, listColumnsJSON, cardFieldsJSON, roadmapConfigJSON, viewSettingsJSON)
	return &config, nil
}

// Create inserts a board configuration scoped to either a collection or a
// workspace (exactly one of the two must be non-nil), plus its columns and
// status mappings, in a single transaction. Returns the new configuration id.
func (r *BoardConfigurationRepository) Create(collectionID, workspaceID *int, req *models.BoardConfigurationRequest) (int, error) {
	configBytes, err := marshalBoardConfigFields(req)
	if err != nil {
		return 0, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var configID int64
	if workspaceID != nil {
		// Create workspace board configuration
		err = tx.QueryRow(`
			INSERT INTO board_configurations (workspace_id, backlog_status_ids, list_columns, card_fields, roadmap_config, view_settings, show_rightmost_column_last_50, completed_item_retention_days, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			*workspaceID, jsonOrNil(configBytes.BacklogStatusIDs), jsonOrNil(configBytes.ListColumns), jsonOrNil(configBytes.CardFields), jsonOrNil(configBytes.RoadmapConfig), jsonOrNil(configBytes.ViewSettings), req.ShowRightmostColumnLast50, req.CompletedItemRetentionDays, time.Now(), time.Now(),
		).Scan(&configID)
	} else {
		err = tx.QueryRow(`
			INSERT INTO board_configurations (collection_id, backlog_status_ids, list_columns, card_fields, roadmap_config, view_settings, show_rightmost_column_last_50, completed_item_retention_days, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			*collectionID, jsonOrNil(configBytes.BacklogStatusIDs), jsonOrNil(configBytes.ListColumns), jsonOrNil(configBytes.CardFields), jsonOrNil(configBytes.RoadmapConfig), jsonOrNil(configBytes.ViewSettings), req.ShowRightmostColumnLast50, req.CompletedItemRetentionDays, time.Now(), time.Now(),
		).Scan(&configID)
	}
	if err != nil {
		return 0, err
	}

	// Create columns
	if err := r.createColumns(tx, int(configID), req.Columns); err != nil {
		slog.Error("failed to create board columns", "error", err, "config_id", configID)
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(configID), nil
}

// Update rewrites a board configuration and reconciles its columns and status
// mappings (update existing, create new, delete removed) in one transaction.
func (r *BoardConfigurationRepository) Update(configID int, req *models.BoardConfigurationRequest) error {
	configBytes, err := marshalBoardConfigFields(req)
	if err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Update the configuration
	_, err = tx.Exec(`
		UPDATE board_configurations
		SET backlog_status_ids = ?, list_columns = ?, card_fields = ?, roadmap_config = ?, view_settings = ?, show_rightmost_column_last_50 = ?, completed_item_retention_days = ?, updated_at = ?
		WHERE id = ?`,
		jsonOrNil(configBytes.BacklogStatusIDs), jsonOrNil(configBytes.ListColumns), jsonOrNil(configBytes.CardFields), jsonOrNil(configBytes.RoadmapConfig), jsonOrNil(configBytes.ViewSettings), req.ShowRightmostColumnLast50, req.CompletedItemRetentionDays, time.Now(), configID,
	)
	if err != nil {
		return err
	}

	// Get existing columns
	existingColumns, err := getBoardColumns(tx, configID)
	if err != nil {
		return err
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
				return err
			}

			// Delete existing status mappings
			slog.Info("deleting existing status mappings", "column_id", *colReq.ID)
			_, err = tx.Exec(`DELETE FROM board_column_statuses WHERE board_column_id = ?`, *colReq.ID)
			if err != nil {
				slog.Error("failed to delete existing status mappings", "error", err)
				return err
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
					return fmt.Errorf("failed to insert status mapping for status_id=%d, board_column_id=%d: %w", statusID, *colReq.ID, err)
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
				return err
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
					return fmt.Errorf("failed to insert status mapping for status_id=%d, board_column_id=%d: %w", statusID, colID, err)
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
				return err
			}
			// Delete the column
			_, err = tx.Exec(`DELETE FROM board_columns WHERE id = ?`, existingID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// SetViewSettings replaces only the view_settings JSON column, leaving the
// rest of the configuration (columns, card fields, …) untouched. Used by
// bundle/pack import, which applies a nav preset to an existing board.
func (r *BoardConfigurationRepository) SetViewSettings(configID int, settings *models.ViewSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = r.db.ExecWrite(
		`UPDATE board_configurations SET view_settings = ?, updated_at = ? WHERE id = ?`,
		jsonOrNil(raw), time.Now(), configID,
	)
	return err
}

// Delete removes a board configuration (cascade handles columns and status mappings).
func (r *BoardConfigurationRepository) Delete(configID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM board_configurations WHERE id = ?`, configID)
	return err
}

// GetColumns returns the columns of a board configuration in display order
// (without status mappings).
func (r *BoardConfigurationRepository) GetColumns(configID int) ([]models.BoardColumn, error) {
	return getBoardColumns(r.db, configID)
}

func getBoardColumns(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, configID int) ([]models.BoardColumn, error) {
	rows, err := q.Query(`
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// GetColumnsWithStatuses returns the columns of a board configuration with
// their mapped status ids populated.
func (r *BoardConfigurationRepository) GetColumnsWithStatuses(configID int) ([]models.BoardColumn, error) {
	columns, err := r.GetColumns(configID)
	if err != nil {
		return nil, err
	}

	// Get status mappings for all columns
	for i := range columns {
		rows, err := r.db.Query(`
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
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
		columns[i].StatusIDs = statusIDs
	}

	return columns, nil
}

func (r *BoardConfigurationRepository) createColumns(tx database.Tx, configID int, columns []models.BoardColumnRequest) error {
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

// jsonOrNil binds marshaled JSON bytes as SQL NULL when empty. PostgreSQL
// would otherwise coerce an empty string to the jsonb type and reject it.
func jsonOrNil(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// boardConfigBytes holds the JSON-encoded board configuration fields.
type boardConfigBytes struct {
	BacklogStatusIDs []byte
	ListColumns      []byte
	CardFields       []byte
	RoadmapConfig    []byte
	ViewSettings     []byte
}

// marshalBoardConfigFields marshals the JSON config fields from a request.
func marshalBoardConfigFields(req *models.BoardConfigurationRequest) (*boardConfigBytes, error) {
	result := &boardConfigBytes{}
	var err error

	if len(req.BacklogStatusIDs) > 0 {
		result.BacklogStatusIDs, err = json.Marshal(req.BacklogStatusIDs)
		if err != nil {
			return nil, err
		}
		slog.Info("marshaled backlog status IDs", "json", string(result.BacklogStatusIDs))
	}

	if len(req.ListColumns) > 0 {
		result.ListColumns, err = json.Marshal(req.ListColumns)
		if err != nil {
			return nil, err
		}
		slog.Info("marshaled list columns", "json", string(result.ListColumns))
	}

	if len(req.CardFields) > 0 {
		result.CardFields, err = json.Marshal(req.CardFields)
		if err != nil {
			return nil, err
		}
	}

	if req.RoadmapConfig != nil {
		result.RoadmapConfig, err = json.Marshal(req.RoadmapConfig)
		if err != nil {
			return nil, err
		}
	}

	if req.ViewSettings != nil {
		result.ViewSettings, err = json.Marshal(req.ViewSettings)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// unmarshalBoardConfigFields decodes the JSON config fields into a BoardConfiguration.
func unmarshalBoardConfigFields(config *models.BoardConfiguration, backlogJSON, listColumnsJSON, cardFieldsJSON, roadmapJSON, viewSettingsJSON sql.NullString) {
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
	if viewSettingsJSON.Valid && viewSettingsJSON.String != "" {
		var viewSettings models.ViewSettings
		if err := json.Unmarshal([]byte(viewSettingsJSON.String), &viewSettings); err == nil {
			config.ViewSettings = &viewSettings
		}
	}
}
