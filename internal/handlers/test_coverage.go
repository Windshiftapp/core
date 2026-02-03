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
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TestCoverageHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewTestCoverageHandler(db database.Database, permissionService *services.PermissionService) *TestCoverageHandler {
	return &TestCoverageHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// GetConfig returns the test coverage configuration for a collection or workspace
func (h *TestCoverageHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var config models.TestCoverageConfiguration
	var err error

	if id == "default" {
		// Workspace-level configuration
		workspaceIDStr := r.URL.Query().Get("workspace_id")
		if workspaceIDStr == "" {
			respondValidationError(w, r, "workspace_id query parameter required for default configuration")
			return
		}

		workspaceID, err := strconv.Atoi(workspaceIDStr)
		if err != nil {
			respondInvalidID(w, r, "workspace_id")
			return
		}

		var collectionID, wsID sql.NullInt64
		var typeIDsJSON sql.NullString
		err = h.db.QueryRow(`
			SELECT id, collection_id, workspace_id, requirement_item_type_ids, created_at, updated_at
			FROM test_coverage_configurations
			WHERE workspace_id = ? AND collection_id IS NULL`,
			workspaceID,
		).Scan(&config.ID, &collectionID, &wsID, &typeIDsJSON, &config.CreatedAt, &config.UpdatedAt)

		if err == nil {
			if collectionID.Valid {
				cid := int(collectionID.Int64)
				config.CollectionID = &cid
			}
			if wsID.Valid {
				wid := int(wsID.Int64)
				config.WorkspaceID = &wid
			}
			if typeIDsJSON.Valid && typeIDsJSON.String != "" {
				json.Unmarshal([]byte(typeIDsJSON.String), &config.RequirementItemTypeIDs)
			}
		}
	} else {
		// Collection-level configuration
		collectionID, err := strconv.Atoi(id)
		if err != nil {
			respondInvalidID(w, r, "collectionId")
			return
		}

		var collID, wsID sql.NullInt64
		var typeIDsJSON sql.NullString
		err = h.db.QueryRow(`
			SELECT id, collection_id, workspace_id, requirement_item_type_ids, created_at, updated_at
			FROM test_coverage_configurations
			WHERE collection_id = ?`,
			collectionID,
		).Scan(&config.ID, &collID, &wsID, &typeIDsJSON, &config.CreatedAt, &config.UpdatedAt)

		if err == nil {
			if collID.Valid {
				cid := int(collID.Int64)
				config.CollectionID = &cid
			}
			if wsID.Valid {
				wid := int(wsID.Int64)
				config.WorkspaceID = &wid
			}
			if typeIDsJSON.Valid && typeIDsJSON.String != "" {
				json.Unmarshal([]byte(typeIDsJSON.String), &config.RequirementItemTypeIDs)
			}
		}
	}

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "test_coverage_configuration")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// CreateConfig creates a new test coverage configuration
func (h *TestCoverageHandler) CreateConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req models.TestCoverageConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	typeIDsBytes, err := json.Marshal(req.RequirementItemTypeIDs)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("Failed to marshal item type IDs"))
		return
	}

	var configID int64
	var collectionID *int
	var workspaceID *int

	if id == "default" {
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
		workspaceID = &wsID

		err = h.db.QueryRow(`
			INSERT INTO test_coverage_configurations (workspace_id, requirement_item_type_ids, created_at, updated_at)
			VALUES (?, ?, ?, ?) RETURNING id`,
			wsID, typeIDsBytes, time.Now(), time.Now(),
		).Scan(&configID)
	} else {
		collID, parseErr := strconv.Atoi(id)
		if parseErr != nil {
			respondInvalidID(w, r, "collectionId")
			return
		}
		collectionID = &collID

		err = h.db.QueryRow(`
			INSERT INTO test_coverage_configurations (collection_id, requirement_item_type_ids, created_at, updated_at)
			VALUES (?, ?, ?, ?) RETURNING id`,
			collID, typeIDsBytes, time.Now(), time.Now(),
		).Scan(&configID)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	config := models.TestCoverageConfiguration{
		ID:                     int(configID),
		CollectionID:           collectionID,
		WorkspaceID:            workspaceID,
		RequirementItemTypeIDs: req.RequirementItemTypeIDs,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(config)
}

// UpdateConfig updates the test coverage configuration
func (h *TestCoverageHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	configID, err := strconv.Atoi(r.PathValue("configId"))
	if err != nil {
		respondInvalidID(w, r, "configId")
		return
	}

	var req models.TestCoverageConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	typeIDsBytes, err := json.Marshal(req.RequirementItemTypeIDs)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("Failed to marshal item type IDs"))
		return
	}

	_, err = h.db.ExecWrite(`
		UPDATE test_coverage_configurations
		SET requirement_item_type_ids = ?, updated_at = ?
		WHERE id = ?`,
		typeIDsBytes, time.Now(), configID,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated configuration
	var config models.TestCoverageConfiguration
	var collID, wsID sql.NullInt64
	var typeIDsJSON sql.NullString
	err = h.db.QueryRow(`
		SELECT id, collection_id, workspace_id, requirement_item_type_ids, created_at, updated_at
		FROM test_coverage_configurations
		WHERE id = ?`,
		configID,
	).Scan(&config.ID, &collID, &wsID, &typeIDsJSON, &config.CreatedAt, &config.UpdatedAt)

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
	if typeIDsJSON.Valid && typeIDsJSON.String != "" {
		json.Unmarshal([]byte(typeIDsJSON.String), &config.RequirementItemTypeIDs)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// DeleteConfig deletes the test coverage configuration
func (h *TestCoverageHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	configID, err := strconv.Atoi(r.PathValue("configId"))
	if err != nil {
		respondInvalidID(w, r, "configId")
		return
	}

	_, err = h.db.ExecWrite(`DELETE FROM test_coverage_configurations WHERE id = ?`, configID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetSummary returns the coverage summary (for pie chart)
func (h *TestCoverageHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Get the configuration
	typeIDs, workspaceID, err := h.getRequirementTypeIDs(id, r.URL.Query().Get("workspace_id"))
	if err != nil {
		if err == sql.ErrNoRows {
			// No config, return empty summary
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(models.TestCoverageSummary{})
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if len(typeIDs) == 0 {
		// No requirement types configured
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.TestCoverageSummary{})
		return
	}

	// Build the query for coverage summary
	placeholders := make([]string, len(typeIDs))
	args := make([]interface{}, len(typeIDs)+1)
	args[0] = workspaceID
	for i, id := range typeIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN linked_count > 0 THEN 1 ELSE 0 END), 0) as covered
		FROM (
			SELECT
				i.id,
				(
					SELECT COUNT(*) FROM item_links il
					WHERE (
						(il.source_type = 'item' AND il.source_id = i.id AND il.target_type = 'test_case' AND il.link_type_id = 1)
						OR
						(il.target_type = 'item' AND il.target_id = i.id AND il.source_type = 'test_case' AND il.link_type_id = 1)
					)
				) as linked_count
			FROM items i
			WHERE i.workspace_id = ? AND i.item_type_id IN (` + strings.Join(placeholders, ",") + `)
		) sub
	`

	var total, covered int
	err = h.db.QueryRow(query, args...).Scan(&total, &covered)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	notCovered := total - covered
	var coverageRate float64
	if total > 0 {
		coverageRate = float64(covered) / float64(total) * 100
	}

	summary := models.TestCoverageSummary{
		Total:        total,
		Covered:      covered,
		NotCovered:   notCovered,
		CoverageRate: coverageRate,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// GetRequirements returns the paginated list of requirements with coverage status
func (h *TestCoverageHandler) GetRequirements(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Parse pagination parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}
	offset := (page - 1) * limit

	// Parse filter parameters
	coveredFilter := r.URL.Query().Get("covered") // "true", "false", or empty for all
	itemTypeFilter := r.URL.Query().Get("item_type_id")
	searchFilter := r.URL.Query().Get("search")

	// Get the configuration
	typeIDs, workspaceID, err := h.getRequirementTypeIDs(id, r.URL.Query().Get("workspace_id"))
	if err != nil {
		if err == sql.ErrNoRows {
			// No config, return empty list
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(models.TestCoverageListResponse{
				Items:      []models.RequirementCoverageItem{},
				Pagination: models.PaginationMeta{Page: page, Limit: limit, Total: 0},
				Summary:    models.TestCoverageSummary{},
			})
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if len(typeIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.TestCoverageListResponse{
			Items:      []models.RequirementCoverageItem{},
			Pagination: models.PaginationMeta{Page: page, Limit: limit, Total: 0},
			Summary:    models.TestCoverageSummary{},
		})
		return
	}

	// Override typeIDs if specific item type filter is provided
	if itemTypeFilter != "" {
		itemTypeID, err := strconv.Atoi(itemTypeFilter)
		if err == nil {
			// Check if the filtered type is in the configured types
			found := false
			for _, tid := range typeIDs {
				if tid == itemTypeID {
					found = true
					break
				}
			}
			if found {
				typeIDs = []int{itemTypeID}
			}
		}
	}

	// Build placeholders
	placeholders := make([]string, len(typeIDs))
	args := make([]interface{}, 0)
	args = append(args, workspaceID)
	for i, id := range typeIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	// Build WHERE clause for filters
	whereClause := "WHERE i.workspace_id = ? AND i.item_type_id IN (" + strings.Join(placeholders, ",") + ")"
	havingClause := ""

	if searchFilter != "" {
		whereClause += " AND i.title LIKE ?"
		args = append(args, "%"+searchFilter+"%")
	}

	if coveredFilter == "true" {
		havingClause = " HAVING linked_count > 0"
	} else if coveredFilter == "false" {
		havingClause = " HAVING linked_count = 0"
	}

	// Count total
	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT
				i.id,
				(
					SELECT COUNT(*) FROM item_links il
					WHERE (
						(il.source_type = 'item' AND il.source_id = i.id AND il.target_type = 'test_case' AND il.link_type_id = 1)
						OR
						(il.target_type = 'item' AND il.target_id = i.id AND il.source_type = 'test_case' AND il.link_type_id = 1)
					)
				) as linked_count
			FROM items i
			` + whereClause + `
			GROUP BY i.id
			` + havingClause + `
		) sub
	`

	var total int
	err = h.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Fetch items
	dataQuery := `
		SELECT
			i.id,
			w.key as workspace_key,
			i.workspace_item_number,
			i.title,
			i.item_type_id,
			it.name as item_type_name,
			it.icon as item_type_icon,
			it.color as item_type_color,
			i.status_id,
			COALESCE(s.name, '') as status_name,
			(
				SELECT COUNT(*) FROM item_links il
				WHERE (
					(il.source_type = 'item' AND il.source_id = i.id AND il.target_type = 'test_case' AND il.link_type_id = 1)
					OR
					(il.target_type = 'item' AND il.target_id = i.id AND il.source_type = 'test_case' AND il.link_type_id = 1)
				)
			) as linked_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses s ON i.status_id = s.id
		` + whereClause + `
		GROUP BY i.id
		` + havingClause + `
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`

	dataArgs := append(args, limit, offset)
	rows, err := h.db.Query(dataQuery, dataArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	items := []models.RequirementCoverageItem{}
	for rows.Next() {
		var item models.RequirementCoverageItem
		var statusID sql.NullInt64
		err := rows.Scan(
			&item.ItemID,
			&item.WorkspaceKey,
			&item.WorkspaceItemNum,
			&item.Title,
			&item.ItemTypeID,
			&item.ItemTypeName,
			&item.ItemTypeIcon,
			&item.ItemTypeColor,
			&statusID,
			&item.StatusName,
			&item.LinkedTestCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if statusID.Valid {
			sid := int(statusID.Int64)
			item.StatusID = &sid
		}
		item.IsCovered = item.LinkedTestCount > 0
		items = append(items, item)
	}

	// Calculate summary for the filtered results
	var covered, notCovered int
	for _, item := range items {
		if item.IsCovered {
			covered++
		} else {
			notCovered++
		}
	}

	// Get overall summary (unfiltered)
	summaryArgs := make([]interface{}, 0)
	summaryArgs = append(summaryArgs, workspaceID)
	for _, id := range typeIDs {
		summaryArgs = append(summaryArgs, id)
	}

	var summaryTotal, summaryCovered int
	summaryQuery := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN linked_count > 0 THEN 1 ELSE 0 END), 0) as covered
		FROM (
			SELECT
				i.id,
				(
					SELECT COUNT(*) FROM item_links il
					WHERE (
						(il.source_type = 'item' AND il.source_id = i.id AND il.target_type = 'test_case' AND il.link_type_id = 1)
						OR
						(il.target_type = 'item' AND il.target_id = i.id AND il.source_type = 'test_case' AND il.link_type_id = 1)
					)
				) as linked_count
			FROM items i
			WHERE i.workspace_id = ? AND i.item_type_id IN (` + strings.Join(placeholders, ",") + `)
		) sub
	`
	h.db.QueryRow(summaryQuery, summaryArgs...).Scan(&summaryTotal, &summaryCovered)

	var coverageRate float64
	if summaryTotal > 0 {
		coverageRate = float64(summaryCovered) / float64(summaryTotal) * 100
	}

	response := models.TestCoverageListResponse{
		Items: items,
		Pagination: models.PaginationMeta{
			Page:  page,
			Limit: limit,
			Total: total,
		},
		Summary: models.TestCoverageSummary{
			Total:        summaryTotal,
			Covered:      summaryCovered,
			NotCovered:   summaryTotal - summaryCovered,
			CoverageRate: coverageRate,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getRequirementTypeIDs retrieves the requirement type IDs from the configuration
func (h *TestCoverageHandler) getRequirementTypeIDs(id string, workspaceIDStr string) ([]int, int, error) {
	var typeIDsJSON sql.NullString
	var workspaceID int

	if id == "default" {
		if workspaceIDStr == "" {
			return nil, 0, sql.ErrNoRows
		}

		wsID, err := strconv.Atoi(workspaceIDStr)
		if err != nil {
			return nil, 0, err
		}
		workspaceID = wsID

		err = h.db.QueryRow(`
			SELECT requirement_item_type_ids
			FROM test_coverage_configurations
			WHERE workspace_id = ? AND collection_id IS NULL`,
			wsID,
		).Scan(&typeIDsJSON)
		if err != nil {
			return nil, 0, err
		}
	} else {
		collectionID, err := strconv.Atoi(id)
		if err != nil {
			return nil, 0, err
		}

		// First try to get collection-specific config
		err = h.db.QueryRow(`
			SELECT tcc.requirement_item_type_ids, c.workspace_id
			FROM test_coverage_configurations tcc
			JOIN collections c ON tcc.collection_id = c.id
			WHERE tcc.collection_id = ?`,
			collectionID,
		).Scan(&typeIDsJSON, &workspaceID)

		if err == sql.ErrNoRows {
			// Fall back to workspace default
			err = h.db.QueryRow(`
				SELECT tcc.requirement_item_type_ids, c.workspace_id
				FROM collections c
				JOIN test_coverage_configurations tcc ON tcc.workspace_id = c.workspace_id AND tcc.collection_id IS NULL
				WHERE c.id = ?`,
				collectionID,
			).Scan(&typeIDsJSON, &workspaceID)
		}

		if err != nil {
			return nil, 0, err
		}
	}

	var typeIDs []int
	if typeIDsJSON.Valid && typeIDsJSON.String != "" {
		json.Unmarshal([]byte(typeIDsJSON.String), &typeIDs)
	}

	return typeIDs, workspaceID, nil
}

// Helper to get current user (for permission checks)
func (h *TestCoverageHandler) getCurrentUser(r *http.Request) *models.User {
	return utils.GetCurrentUser(r)
}
