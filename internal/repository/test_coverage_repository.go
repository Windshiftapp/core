package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestCoverageRepository provides data access methods for test coverage configurations
// and coverage reports.
type TestCoverageRepository struct {
	db database.Database
}

// NewTestCoverageRepository creates a new test coverage repository
func NewTestCoverageRepository(db database.Database) *TestCoverageRepository {
	return &TestCoverageRepository{db: db}
}

// RequirementListParams holds filter/pagination parameters for the legacy item list.
type RequirementListParams struct {
	WorkspaceID   int
	TypeIDs       []int
	CoveredFilter string // "true", "false", or ""
	Search        string
	Limit         int
	Offset        int
}

// CoverageConfigWrite stores create/update payload for test coverage configuration.
type CoverageConfigWrite struct {
	RequirementItemTypeIDs []int
	RequirementTypes       []string
}

const testCoverageConfigColumns = `id, collection_id, workspace_id, requirement_item_type_ids, requirement_types, created_at, updated_at`

// FindConfigForWorkspace returns the workspace-level coverage configuration.
// Returns ErrNotFound if no config exists.
func (r *TestCoverageRepository) FindConfigForWorkspace(workspaceID int) (*models.TestCoverageConfiguration, error) {
	config, err := r.scanConfig(r.db.QueryRow(`
		SELECT `+testCoverageConfigColumns+`
		FROM test_coverage_configurations
		WHERE workspace_id = ? AND collection_id IS NULL`,
		workspaceID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return config, err
}

// FindConfigForCollection returns the collection-level coverage configuration.
// Returns ErrNotFound if no config exists.
func (r *TestCoverageRepository) FindConfigForCollection(collectionID int) (*models.TestCoverageConfiguration, error) {
	config, err := r.scanConfig(r.db.QueryRow(`
		SELECT `+testCoverageConfigColumns+`
		FROM test_coverage_configurations
		WHERE collection_id = ?`,
		collectionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return config, err
}

// FindConfigByID returns the coverage configuration by its primary key.
func (r *TestCoverageRepository) FindConfigByID(configID int) (*models.TestCoverageConfiguration, error) {
	config, err := r.scanConfig(r.db.QueryRow(`
		SELECT `+testCoverageConfigColumns+`
		FROM test_coverage_configurations
		WHERE id = ?`,
		configID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return config, err
}

// GetCollectionWorkspaceID returns the non-null workspace owning a collection.
func (r *TestCoverageRepository) GetCollectionWorkspaceID(collectionID int) (int, error) {
	var workspaceID sql.NullInt64
	err := r.db.QueryRow(`SELECT workspace_id FROM collections WHERE id = ?`, collectionID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to resolve collection workspace: %w", err)
	}
	if !workspaceID.Valid {
		return 0, ErrNotFound
	}
	return int(workspaceID.Int64), nil
}

// CreateConfigForWorkspace inserts a workspace-scoped coverage configuration and returns the new row.
func (r *TestCoverageRepository) CreateConfigForWorkspace(workspaceID int, input CoverageConfigWrite) (*models.TestCoverageConfiguration, error) {
	itemTypeIDsJSON, requirementTypesJSON, err := encodeCoverageConfigWrite(input)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var id int64
	err = r.db.QueryRow(`
		INSERT INTO test_coverage_configurations (workspace_id, requirement_item_type_ids, requirement_types, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id`,
		workspaceID, itemTypeIDsJSON, requirementTypesJSON, now, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace coverage config: %w", err)
	}

	return r.FindConfigByID(int(id))
}

// CreateConfigForCollection inserts a collection-scoped coverage configuration and returns the new row.
func (r *TestCoverageRepository) CreateConfigForCollection(collectionID int, input CoverageConfigWrite) (*models.TestCoverageConfiguration, error) {
	itemTypeIDsJSON, requirementTypesJSON, err := encodeCoverageConfigWrite(input)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var id int64
	err = r.db.QueryRow(`
		INSERT INTO test_coverage_configurations (collection_id, requirement_item_type_ids, requirement_types, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id`,
		collectionID, itemTypeIDsJSON, requirementTypesJSON, now, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection coverage config: %w", err)
	}

	return r.FindConfigByID(int(id))
}

// UpdateConfig updates an existing configuration and returns the updated row.
func (r *TestCoverageRepository) UpdateConfig(configID int, input CoverageConfigWrite) (*models.TestCoverageConfiguration, error) {
	itemTypeIDsJSON, requirementTypesJSON, err := encodeCoverageConfigWrite(input)
	if err != nil {
		return nil, err
	}

	result, err := r.db.ExecWrite(`
		UPDATE test_coverage_configurations
		SET requirement_item_type_ids = ?, requirement_types = ?, updated_at = ?
		WHERE id = ?`,
		itemTypeIDsJSON, requirementTypesJSON, time.Now(), configID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update coverage config: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return nil, ErrNotFound
	}

	return r.FindConfigByID(configID)
}

// DeleteConfig removes a coverage configuration by ID.
func (r *TestCoverageRepository) DeleteConfig(configID int) error {
	result, err := r.db.ExecWrite(`DELETE FROM test_coverage_configurations WHERE id = ?`, configID)
	if err != nil {
		return fmt.Errorf("failed to delete coverage config: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetRequirementTypeIDsForWorkspace returns the configured requirement type IDs for a workspace-level config.
func (r *TestCoverageRepository) GetRequirementTypeIDsForWorkspace(workspaceID int) ([]int, error) {
	var typeIDsJSON sql.NullString
	err := r.db.QueryRow(`
		SELECT requirement_item_type_ids
		FROM test_coverage_configurations
		WHERE workspace_id = ? AND collection_id IS NULL`,
		workspaceID,
	).Scan(&typeIDsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return decodeTypeIDs(typeIDsJSON), nil
}

// GetRequirementTypeIDsForCollection returns the requirement type IDs and owning workspace ID for a
// collection-scoped configuration, falling back to the workspace default if no collection-specific
// config exists.
func (r *TestCoverageRepository) GetRequirementTypeIDsForCollection(collectionID int) (typeIDs []int, workspaceID int, err error) {
	var typeIDsJSON sql.NullString

	err = r.db.QueryRow(`
		SELECT tcc.requirement_item_type_ids, c.workspace_id
		FROM test_coverage_configurations tcc
		JOIN collections c ON tcc.collection_id = c.id
		WHERE tcc.collection_id = ?`,
		collectionID,
	).Scan(&typeIDsJSON, &workspaceID)

	if errors.Is(err, sql.ErrNoRows) {
		err = r.db.QueryRow(`
			SELECT tcc.requirement_item_type_ids, c.workspace_id
			FROM collections c
			JOIN test_coverage_configurations tcc ON tcc.workspace_id = c.workspace_id AND tcc.collection_id IS NULL
			WHERE c.id = ?`,
			collectionID,
		).Scan(&typeIDsJSON, &workspaceID)
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	return decodeTypeIDs(typeIDsJSON), workspaceID, nil
}

// GetRequirementTypesForWorkspace returns configured page-backed requirement types.
func (r *TestCoverageRepository) GetRequirementTypesForWorkspace(workspaceID int) ([]string, error) {
	var typesJSON sql.NullString
	err := r.db.QueryRow(`
		SELECT requirement_types
		FROM test_coverage_configurations
		WHERE workspace_id = ? AND collection_id IS NULL`,
		workspaceID,
	).Scan(&typesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeStringSlice(typesJSON), nil
}

// GetRequirementTypesForCollection returns configured page-backed requirement types and workspace ID.
func (r *TestCoverageRepository) GetRequirementTypesForCollection(collectionID int) (types []string, workspaceID int, err error) {
	var typesJSON sql.NullString

	err = r.db.QueryRow(`
		SELECT tcc.requirement_types, c.workspace_id
		FROM test_coverage_configurations tcc
		JOIN collections c ON tcc.collection_id = c.id
		WHERE tcc.collection_id = ?`,
		collectionID,
	).Scan(&typesJSON, &workspaceID)

	if errors.Is(err, sql.ErrNoRows) {
		err = r.db.QueryRow(`
			SELECT tcc.requirement_types, c.workspace_id
			FROM collections c
			JOIN test_coverage_configurations tcc ON tcc.workspace_id = c.workspace_id AND tcc.collection_id IS NULL
			WHERE c.id = ?`,
			collectionID,
		).Scan(&typesJSON, &workspaceID)
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	return decodeStringSlice(typesJSON), workspaceID, nil
}

// ListAllPageBackedRequirements returns page-backed requirements for coverage analysis.
func (r *TestCoverageRepository) ListAllPageBackedRequirements(workspaceID int, requirementTypes []string) ([]models.RequirementCoverageItem, error) {
	if len(requirementTypes) == 0 {
		return []models.RequirementCoverageItem{}, nil
	}

	placeholders, args := stringSlicePlaceholders(requirementTypes)
	args = append([]any{workspaceID}, args...)

	query := `
		SELECT
			r.page_id,
			r.requirement_number,
			w.key as workspace_key,
			p.title,
			r.requirement_type,
			r.status,
			` + RequirementLinkedTestCountSubquery + ` as linked_count
		FROM requirements r
		JOIN pages p ON p.id = r.page_id
		JOIN workspaces w ON w.id = r.workspace_id
		WHERE r.workspace_id = ?
		  AND p.archived_at IS NULL
		  AND r.requirement_type IN (` + placeholders + `)
		ORDER BY r.requirement_number ASC
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query page-backed requirements: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []models.RequirementCoverageItem{}
	for rows.Next() {
		var item models.RequirementCoverageItem
		if err := rows.Scan(
			&item.PageID,
			&item.RequirementNumber,
			&item.WorkspaceKey,
			&item.Title,
			&item.RequirementType,
			&item.Status,
			&item.LinkedTestCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan page-backed requirement row: %w", err)
		}
		item.RequirementKey = models.FormatRequirementKey(item.WorkspaceKey, item.RequirementNumber)
		item.IsCovered = item.LinkedTestCount > 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate page-backed requirements: %w", err)
	}
	return items, nil
}

// GetCoverageSummary returns total and covered counts for the given workspace/type filter.
func (r *TestCoverageRepository) GetCoverageSummary(workspaceID int, typeIDs []int) (total, covered int, err error) {
	if len(typeIDs) == 0 {
		return 0, 0, nil
	}

	placeholders, args := coverageWhereArgs(workspaceID, typeIDs)

	query := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN linked_count > 0 THEN 1 ELSE 0 END), 0) as covered
		FROM (
			SELECT
				i.id,
				(` + coverageLinkedCountSubquery + `) as linked_count
			FROM items i
			WHERE i.workspace_id = ? AND i.item_type_id IN (` + placeholders + `)
		) sub
	`

	err = r.db.QueryRow(query, args...).Scan(&total, &covered)
	return total, covered, err
}

// CountRequirements returns the total count of requirements matching the filter.
func (r *TestCoverageRepository) CountRequirements(params RequirementListParams) (int, error) {
	if len(params.TypeIDs) == 0 {
		return 0, nil
	}

	whereClause, args := buildRequirementFilters(params)

	query := `SELECT COUNT(*) FROM items i ` + whereClause

	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// ListRequirements returns a page of requirement items with coverage status.
func (r *TestCoverageRepository) ListRequirements(params RequirementListParams) ([]models.RequirementCoverageItem, error) {
	if len(params.TypeIDs) == 0 {
		return []models.RequirementCoverageItem{}, nil
	}

	whereClause, args := buildRequirementFilters(params)
	args = append(args, params.Limit, params.Offset)

	query := `
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
			(` + coverageLinkedCountSubquery + `) as linked_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses s ON i.status_id = s.id
		` + whereClause + `
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query requirements: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []models.RequirementCoverageItem{}
	for rows.Next() {
		var item models.RequirementCoverageItem
		var statusID sql.NullInt64
		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("failed to scan requirement row: %w", err)
		}
		if statusID.Valid {
			sid := int(statusID.Int64)
			item.StatusID = &sid
		}
		item.IsCovered = item.LinkedTestCount > 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate requirements: %w", err)
	}
	return items, nil
}

// coverageLinkedCountSubquery counts item_links that connect an item to a test_case
// in either direction via the system Tests link type.
const coverageLinkedCountSubquery = `
	SELECT COUNT(*)
	FROM item_links il
	JOIN link_types lt ON lt.id = il.link_type_id
	WHERE lt.builtin_key = 'tests' AND (
		(il.source_type = 'item' AND il.source_id = i.id AND il.target_type = 'test_case')
		OR
		(il.target_type = 'item' AND il.target_id = i.id AND il.source_type = 'test_case')
	)
`

func (r *TestCoverageRepository) scanConfig(row *sql.Row) (*models.TestCoverageConfiguration, error) {
	var config models.TestCoverageConfiguration
	var collID, wsID sql.NullInt64
	var typeIDsJSON, requirementTypesJSON sql.NullString

	if err := row.Scan(&config.ID, &collID, &wsID, &typeIDsJSON, &requirementTypesJSON, &config.CreatedAt, &config.UpdatedAt); err != nil {
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
	config.RequirementItemTypeIDs = decodeTypeIDs(typeIDsJSON)
	config.RequirementTypes = decodeStringSlice(requirementTypesJSON)
	return &config, nil
}

func encodeCoverageConfigWrite(input CoverageConfigWrite) (itemTypeIDsJSON, requirementTypesJSON interface{}, err error) {
	if len(input.RequirementTypes) > 0 {
		requirementTypesBytes, err := json.Marshal(input.RequirementTypes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal requirement types: %w", err)
		}
		return nil, requirementTypesBytes, nil
	}
	itemTypeIDsBytes, err := json.Marshal(input.RequirementItemTypeIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal item type IDs: %w", err)
	}
	return itemTypeIDsBytes, nil, nil
}

func decodeTypeIDs(v sql.NullString) []int {
	if !v.Valid || v.String == "" {
		return nil
	}
	var ids []int
	_ = json.Unmarshal([]byte(v.String), &ids)
	return ids
}

func decodeStringSlice(v sql.NullString) []string {
	if !v.Valid || v.String == "" {
		return nil
	}
	var values []string
	_ = json.Unmarshal([]byte(v.String), &values)
	return values
}

func stringSlicePlaceholders(values []string) (placeholders string, args []any) {
	slots := make([]string, len(values))
	args = make([]any, len(values))
	for i, value := range values {
		slots[i] = "?"
		args[i] = value
	}
	return strings.Join(slots, ","), args
}

func coverageWhereArgs(workspaceID int, typeIDs []int) (placeholders string, args []any) {
	slots := make([]string, len(typeIDs))
	args = make([]any, 0, len(typeIDs)+1)
	args = append(args, workspaceID)
	for i, id := range typeIDs {
		slots[i] = "?"
		args = append(args, id)
	}
	return strings.Join(slots, ","), args
}

func buildRequirementFilters(params RequirementListParams) (where string, args []any) {
	placeholders, filterArgs := coverageWhereArgs(params.WorkspaceID, params.TypeIDs)
	args = filterArgs
	where = "WHERE i.workspace_id = ? AND i.item_type_id IN (" + placeholders + ")"

	if params.Search != "" {
		where += " AND i.title LIKE ?"
		args = append(args, "%"+params.Search+"%")
	}

	switch params.CoveredFilter {
	case "true":
		where += " AND (" + coverageLinkedCountSubquery + ") > 0"
	case "false":
		where += " AND (" + coverageLinkedCountSubquery + ") = 0"
	}
	return where, args
}
