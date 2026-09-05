package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type AssetRepository struct {
	db database.Database
}

// PortalReportAsset is the public projection returned by an asset report.
type PortalReportAsset struct {
	ID                int            `json:"id"`
	Title             string         `json:"title"`
	AssetTag          string         `json:"asset_tag"`
	AssetTypeID       *int           `json:"asset_type_id,omitempty"`
	StatusID          *int           `json:"status_id,omitempty"`
	CategoryID        *int           `json:"category_id,omitempty"`
	CustomFieldValues map[string]any `json:"custom_field_values,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	AssetTypeName     *string        `json:"asset_type_name,omitempty"`
	StatusName        *string        `json:"status_name,omitempty"`
	StatusColor       *string        `json:"status_color,omitempty"`
	CategoryName      *string        `json:"category_name,omitempty"`
}

// ListPortalReportAssets returns one page of assets for a report and projects
// custom fields down to the explicitly allowed keys.
func (r *AssetRepository) ListPortalReportAssets(ctx context.Context, setID int, cqlSQL string, cqlArgs []any, limit, offset int, allowedCustomFieldKeys map[string]struct{}) ([]PortalReportAsset, error) {
	where := "a.set_id = ?"
	args := []any{setID}
	if cqlSQL != "" {
		where += " AND (" + cqlSQL + ")"
		args = append(args, cqlArgs...)
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.title, COALESCE(a.asset_tag, ''), a.asset_type_id, a.status_id, a.category_id,
		       a.custom_field_values, a.created_at, a.updated_at,
		       at.name, ast.name, ast.color, ac.name
		FROM assets a
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		WHERE `+where+`
		ORDER BY a.created_at DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list portal report assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	assets := make([]PortalReportAsset, 0)
	for rows.Next() {
		var asset PortalReportAsset
		var assetTypeID, statusID, categoryID sql.NullInt64
		var customFieldValues sql.NullString
		var assetTypeName, statusName, statusColor, categoryName sql.NullString
		if err := rows.Scan(&asset.ID, &asset.Title, &asset.AssetTag, &assetTypeID, &statusID, &categoryID,
			&customFieldValues, &asset.CreatedAt, &asset.UpdatedAt,
			&assetTypeName, &statusName, &statusColor, &categoryName); err != nil {
			return nil, fmt.Errorf("scan portal report asset: %w", err)
		}
		if assetTypeID.Valid {
			id := int(assetTypeID.Int64)
			asset.AssetTypeID = &id
		}
		if statusID.Valid {
			id := int(statusID.Int64)
			asset.StatusID = &id
		}
		if categoryID.Valid {
			id := int(categoryID.Int64)
			asset.CategoryID = &id
		}
		if customFieldValues.Valid && customFieldValues.String != "" && len(allowedCustomFieldKeys) > 0 {
			var values map[string]any
			if json.Unmarshal([]byte(customFieldValues.String), &values) == nil {
				projected := make(map[string]any, len(allowedCustomFieldKeys))
				for key := range allowedCustomFieldKeys {
					if value, ok := values[key]; ok {
						projected[key] = value
					}
				}
				if len(projected) > 0 {
					asset.CustomFieldValues = projected
				}
			}
		}
		if assetTypeName.Valid {
			asset.AssetTypeName = &assetTypeName.String
		}
		if statusName.Valid {
			asset.StatusName = &statusName.String
		}
		if statusColor.Valid {
			asset.StatusColor = &statusColor.String
		}
		if categoryName.Valid {
			asset.CategoryName = &categoryName.String
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portal report assets: %w", err)
	}
	return assets, nil
}

func (r *AssetRepository) CountPortalReportAssets(ctx context.Context, setID int, cqlSQL string, cqlArgs []any) (int, error) {
	query := "SELECT COUNT(*) FROM assets a WHERE a.set_id = ?"
	args := []any{setID}
	if cqlSQL != "" {
		query += " AND (" + cqlSQL + ")"
		args = append(args, cqlArgs...)
	}
	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count portal report assets: %w", err)
	}
	return total, nil
}

// FindAssetSummariesByIDs returns compact display data in one query. Callers
// must apply set-level authorization before returning these rows.
func (r *AssetRepository) FindAssetSummariesByIDs(ids []int) ([]models.AssetSummary, error) {
	if len(ids) == 0 {
		return []models.AssetSummary{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT id, set_id, title, COALESCE(asset_tag, '')
		FROM assets
		WHERE id IN (`+placeholders+`)
		ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("find asset summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	summaries := make([]models.AssetSummary, 0, len(ids))
	for rows.Next() {
		var summary models.AssetSummary
		if err := rows.Scan(&summary.ID, &summary.SetID, &summary.Title, &summary.AssetTag); err != nil {
			return nil, fmt.Errorf("scan asset summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset summaries: %w", err)
	}
	return summaries, nil
}

// ListLinkedToItem returns display rows for assets linked to the given item in
// either direction: "outgoing" when the item is the link source (forward label)
// and "incoming" when the asset is the link source (reverse label). Callers
// remain responsible for set-level authorization via SetID.
func (r *AssetRepository) ListLinkedToItem(itemID int) ([]models.LinkedAsset, error) {
	queries := []struct {
		query     string
		direction string
	}{
		{
			query: `
				SELECT a.id, a.title, COALESCE(a.description, '') AS description,
				       a.set_id, ams.name AS set_name,
				       COALESCE(at.name, '') AS type_name,
				       COALESCE(ac.name, '') AS category_name,
				       il.id AS link_id, lt.name AS link_type_name, lt.forward_label
				FROM item_links il
				JOIN assets a ON il.target_type = 'asset' AND il.target_id = a.id
				LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
				LEFT JOIN asset_types at ON a.asset_type_id = at.id
				LEFT JOIN asset_categories ac ON a.category_id = ac.id
				JOIN link_types lt ON il.link_type_id = lt.id
				WHERE il.source_type = 'item' AND il.source_id = ?
				ORDER BY a.title`,
			direction: "outgoing",
		},
		{
			query: `
				SELECT a.id, a.title, COALESCE(a.description, '') AS description,
				       a.set_id, ams.name AS set_name,
				       COALESCE(at.name, '') AS type_name,
				       COALESCE(ac.name, '') AS category_name,
				       il.id AS link_id, lt.name AS link_type_name, lt.reverse_label
				FROM item_links il
				JOIN assets a ON il.source_type = 'asset' AND il.source_id = a.id
				LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
				LEFT JOIN asset_types at ON a.asset_type_id = at.id
				LEFT JOIN asset_categories ac ON a.category_id = ac.id
				JOIN link_types lt ON il.link_type_id = lt.id
				WHERE il.target_type = 'item' AND il.target_id = ?
				ORDER BY a.title`,
			direction: "incoming",
		},
	}

	var assets []models.LinkedAsset
	for _, segment := range queries {
		rows, err := r.db.Query(segment.query, itemID)
		if err != nil {
			return nil, fmt.Errorf("list linked assets (%s): %w", segment.direction, err)
		}
		for rows.Next() {
			var asset models.LinkedAsset
			var description, setName, typeName, categoryName, linkLabel sql.NullString
			if err := rows.Scan(&asset.ID, &asset.Title, &description, &asset.SetID, &setName,
				&typeName, &categoryName, &asset.LinkID, &asset.LinkTypeName, &linkLabel); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan linked asset (%s): %w", segment.direction, err)
			}
			asset.Description = description.String
			asset.SetName = setName.String
			asset.TypeName = typeName.String
			asset.CategoryName = categoryName.String
			asset.LinkLabel = linkLabel.String
			asset.Direction = segment.direction
			assets = append(assets, asset)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate linked assets (%s): %w", segment.direction, err)
		}
		_ = rows.Close()
	}
	return assets, nil
}

// Search returns linkable asset rows matching query in the given sets. Callers
// must supply set IDs the user can access (nil set IDs yields no rows).
func (r *AssetRepository) Search(query string, setIDs []int, limit int) ([]models.LinkableItem, error) {
	if len(setIDs) == 0 {
		return []models.LinkableItem{}, nil
	}
	setPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(setIDs)), ",")
	setArgs := make([]any, len(setIDs))
	for i, id := range setIDs {
		setArgs[i] = id
	}

	sqlQuery := fmt.Sprintf(`
		SELECT a.id, a.title, COALESCE(a.description, '') AS description,
		       a.set_id, ams.name AS set_name,
		       COALESCE(at.name, '') AS type_name,
		       COALESCE(ac.name, '') AS category_name
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		WHERE (a.title LIKE ? OR a.description LIKE ?)
		  AND a.set_id IN (%s)
		ORDER BY a.title
		LIMIT ?
	`, setPlaceholders)

	searchTerm := "%" + query + "%"
	args := make([]any, 0, 3+len(setArgs))
	args = append(args, searchTerm, searchTerm)
	args = append(args, setArgs...)
	args = append(args, limit)
	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.LinkableItem
	for rows.Next() {
		var item models.LinkableItem
		var description, setName, typeName, categoryName sql.NullString
		var setID sql.NullInt64

		if err := rows.Scan(&item.ID, &item.Title, &description, &setID, &setName, &typeName, &categoryName); err != nil {
			return nil, fmt.Errorf("scan asset search result: %w", err)
		}

		item.Description = description.String
		item.AssetSetID = utils.NullInt64ToPtr(setID)
		item.AssetSetName = setName.String
		item.AssetTypeName = typeName.String
		item.AssetCategoryName = categoryName.String

		item.Type = "asset"
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset search results: %w", err)
	}

	return items, nil
}

func NewAssetRepository(db database.Database) *AssetRepository {
	return &AssetRepository{db: db}
}

// Asset-set operations.

func (r *AssetRepository) ListSetsForUser(userID int, isAdmin bool) ([]models.AssetManagementSet, error) {
	query := `
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
	`

	var args []any

	if !isAdmin {
		query += ` WHERE (
			EXISTS (SELECT 1 FROM user_asset_set_roles WHERE set_id = ams.id AND user_id = ?)
			OR EXISTS (
				SELECT 1 FROM group_asset_set_roles gasr
				JOIN group_members gm ON gasr.group_id = gm.group_id
				WHERE gasr.set_id = ams.id AND gm.user_id = ?
			)
			OR EXISTS (SELECT 1 FROM asset_set_everyone_roles WHERE set_id = ams.id AND role_id IS NOT NULL)
		)`
		args = append(args, userID, userID)
	}

	query += ` ORDER BY ams.is_default DESC, ams.name`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list asset sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sets []models.AssetManagementSet
	for rows.Next() {
		var set models.AssetManagementSet
		var creatorName sql.NullString
		var description sql.NullString

		err := rows.Scan(
			&set.ID, &set.Name, &description, &set.IsDefault,
			&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
			&creatorName, &set.AssetTypeCount, &set.AssetCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset set: %w", err)
		}

		set.CreatorName = creatorName.String
		set.Description = description.String
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset sets: %w", err)
	}

	return sets, nil
}

func (r *AssetRepository) GetSetByID(setID int) (*models.AssetManagementSet, error) {
	var set models.AssetManagementSet
	var creatorName sql.NullString
	var description sql.NullString

	err := r.db.QueryRow(`
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
		WHERE ams.id = ?
	`, setID).Scan(
		&set.ID, &set.Name, &description, &set.IsDefault,
		&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
		&creatorName, &set.AssetTypeCount, &set.AssetCount,
	)

	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get asset set")
	}

	set.CreatorName = creatorName.String
	set.Description = description.String

	return &set, nil
}

// FindSetIDByName returns the ID of the asset set with the exact name.
func (r *AssetRepository) FindSetIDByName(name string) (int, error) {
	var setID int
	err := r.db.QueryRow(
		"SELECT id FROM asset_management_sets WHERE name = ?",
		name,
	).Scan(&setID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find asset set by name: %w", err)
	}
	return setID, nil
}

// CreateImportedSet creates an asset set for an external import, seeds its
// default Active status, and grants the importing user Administrator access
// when one is available.
func (r *AssetRepository) CreateImportedSet(name, description string, creatorUserID int) (int, error) {
	var setID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		now := time.Now()
		var creator any
		if creatorUserID > 0 {
			creator = creatorUserID
		}
		if err := tx.QueryRow(`
			INSERT INTO asset_management_sets (name, description, is_default, created_by, created_at, updated_at)
			VALUES (?, ?, false, ?, ?, ?) RETURNING id
		`, name, description, creator, now, now).Scan(&setID); err != nil {
			return fmt.Errorf("create imported asset set: %w", err)
		}
		if _, err := tx.ExecWrite(`
			INSERT INTO asset_statuses
				(set_id, name, color, description, is_default, display_order, created_at, updated_at)
			VALUES (?, 'Active', '#22c55e', 'Default status for imported Jira Assets', true, 0, ?, ?)
		`, setID, now, now); err != nil {
			return fmt.Errorf("create imported asset status: %w", err)
		}
		if creatorUserID <= 0 {
			return nil
		}
		var roleID int
		if err := tx.QueryRow(
			"SELECT id FROM asset_roles WHERE name = ?",
			assetRoleAdministrator,
		).Scan(&roleID); err != nil {
			return fmt.Errorf("find imported asset administrator role: %w", err)
		}
		if _, err := tx.ExecWrite(`
			INSERT INTO user_asset_set_roles (user_id, set_id, role_id, granted_by)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id, set_id) DO NOTHING
		`, creatorUserID, setID, roleID, creatorUserID); err != nil {
			return fmt.Errorf("grant imported asset administrator role: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return setID, nil
}

const assetRoleAdministrator = "Administrator"

// CreateSetAndInitialize creates an asset set, assigns the Administrator role
// to the creator, and seeds the default statuses — all in one transaction.
// If set.IsDefault is true, the previous default set is also cleared atomically.
func (r *AssetRepository) CreateSetAndInitialize(set *models.AssetManagementSet, creatorUserID int) (int, error) {
	var setID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		now := time.Now()
		if set.IsDefault {
			if _, err := tx.ExecWrite("UPDATE asset_management_sets SET is_default = false"); err != nil {
				return fmt.Errorf("failed to clear default set: %w", err)
			}
		}
		if err := tx.QueryRow(`
			INSERT INTO asset_management_sets (name, description, is_default, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, set.Name, set.Description, set.IsDefault, set.CreatedBy, now, now).Scan(&setID); err != nil {
			return fmt.Errorf("failed to create asset set: %w", err)
		}

		adminRoleID, err := r.GetAssetRoleIDByName(assetRoleAdministrator)
		if err != nil {
			return fmt.Errorf("failed to get administrator role: %w", err)
		}
		if _, err := tx.ExecWrite(`
			INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (set_id, user_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
		`, setID, creatorUserID, adminRoleID, creatorUserID, now, adminRoleID, creatorUserID, now); err != nil {
			return fmt.Errorf("failed to assign admin role: %w", err)
		}

		defaultStatuses := []struct {
			Name         string
			Color        string
			IsDefault    bool
			DisplayOrder int
		}{
			{"Active", "#22c55e", true, 0},
			{"Inactive", "#6b7280", false, 1},
			{"Maintenance", "#f59e0b", false, 2},
			{"Retired", "#ef4444", false, 3},
		}
		for _, s := range defaultStatuses {
			if _, err := tx.ExecWrite(`
				INSERT INTO asset_statuses (set_id, name, color, is_default, display_order, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, setID, s.Name, s.Color, s.IsDefault, s.DisplayOrder, now, now); err != nil {
				return fmt.Errorf("failed to create default status: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return setID, nil
}

// UpdateSetAndPromotion updates an asset set and clears the previous default
// set when this set is being promoted to default. All writes happen in one
// transaction so a failed update cannot leave the module without a default.
func (r *AssetRepository) UpdateSetAndPromotion(set *models.AssetManagementSet) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if set.IsDefault {
			if _, err := tx.ExecWrite(
				"UPDATE asset_management_sets SET is_default = false WHERE is_default = true AND id != ?",
				set.ID,
			); err != nil {
				return fmt.Errorf("failed to clear default set: %w", err)
			}
		}
		now := time.Now()
		result, err := tx.ExecWrite(`
			UPDATE asset_management_sets SET name = ?, description = ?, is_default = ?, updated_at = ?
			WHERE id = ?
		`, set.Name, set.Description, set.IsDefault, now, set.ID)
		if err != nil {
			return fmt.Errorf("failed to update asset set: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// HardDeleteSet deletes a set and relies on foreign-key cascades for its owned
// rows. Polymorphic item_links cannot carry an asset foreign key, so links in
// either direction are removed explicitly in the same transaction first.
func (r *AssetRepository) HardDeleteSet(setID int, eventMetadata ...itemevents.Metadata) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin asset set deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query("SELECT id FROM assets WHERE set_id = ? ORDER BY id", setID)
	if err != nil {
		return fmt.Errorf("load asset set links for deletion: %w", err)
	}
	var assetIDs []int
	for rows.Next() {
		var assetID int
		if err := rows.Scan(&assetID); err != nil {
			_ = rows.Close()
			return err
		}
		assetIDs = append(assetIDs, assetID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	metadata := itemevents.System("application")
	if len(eventMetadata) > 0 {
		metadata = eventMetadata[0]
	}
	if err := itemevents.NewRecorder(r.db).RemovedLinks(context.Background(), tx, "asset", assetIDs, metadata); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM item_links
		WHERE (source_type = 'asset' AND source_id IN (SELECT id FROM assets WHERE set_id = ?))
		   OR (target_type = 'asset' AND target_id IN (SELECT id FROM assets WHERE set_id = ?))
	`, setID, setID); err != nil {
		return fmt.Errorf("failed to delete asset set links: %w", err)
	}

	result, err := tx.Exec("DELETE FROM asset_management_sets WHERE id = ?", setID)
	if err != nil {
		return fmt.Errorf("failed to delete asset set: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit asset set deletion: %w", err)
	}
	return nil
}

func (r *AssetRepository) GetAssetRoleIDByName(name string) (int, error) {
	var id int
	err := r.db.QueryRow(`SELECT id FROM asset_roles WHERE name = ?`, name).Scan(&id)
	if err != nil {
		return 0, notFoundOrWrap(err, "failed to find asset role")
	}
	return id, nil
}

// GetAssetSetCoreByID returns the basic set fields (no joined creator or counts),
// used after an update to return the fresh row.
func (r *AssetRepository) GetAssetSetCoreByID(setID int) (*models.AssetManagementSet, error) {
	var set models.AssetManagementSet
	err := r.db.QueryRow(`
		SELECT id, name, description, is_default, created_by, created_at, updated_at
		FROM asset_management_sets WHERE id = ?
	`, setID).Scan(&set.ID, &set.Name, &set.Description, &set.IsDefault, &set.CreatedBy, &set.CreatedAt, &set.UpdatedAt)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to fetch asset set")
	}
	return &set, nil
}

// Role and permission operations.

// GetUserSetRole returns the role a user has for an asset set
// Priority: Direct User Role > Group Role > Everyone Default
// Note: System admin check should be done in the handler layer
func (r *AssetRepository) GetUserSetRole(userID, setID int) (*models.AssetRole, error) {
	var role models.AssetRole

	err := r.db.QueryRow(`
		SELECT ar.id, ar.name, ar.description, ar.is_system, ar.display_order
		FROM user_asset_set_roles uasr
		JOIN asset_roles ar ON uasr.role_id = ar.id
		WHERE uasr.set_id = ? AND uasr.user_id = ?
	`, setID, userID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err == nil {
		return &role, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get user role: %w", err)
	}

	err = r.db.QueryRow(`
		SELECT ar.id, ar.name, ar.description, ar.is_system, ar.display_order
		FROM group_asset_set_roles gasr
		JOIN group_members gm ON gasr.group_id = gm.group_id
		JOIN asset_roles ar ON gasr.role_id = ar.id
		WHERE gasr.set_id = ? AND gm.user_id = ?
		ORDER BY ar.display_order DESC
		LIMIT 1
	`, setID, userID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err == nil {
		return &role, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get group role: %w", err)
	}

	var roleID sql.NullInt64
	err = r.db.QueryRow(`
		SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?
	`, setID).Scan(&roleID)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get everyone role: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) || !roleID.Valid {
		return nil, nil
	}

	err = r.db.QueryRow(`
		SELECT id, name, description, is_system, display_order
		FROM asset_roles WHERE id = ?
	`, roleID.Int64).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder)

	if err != nil {
		return nil, fmt.Errorf("failed to get role details: %w", err)
	}

	return &role, nil
}

func (r *AssetRepository) RoleHasPermission(roleID int, permissionKey string) (bool, error) {
	if roleID == -1 {
		return true, nil
	}

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ? AND ap.permission_key = ?
	`, roleID, permissionKey).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check role permission: %w", err)
	}

	return count > 0, nil
}

func (r *AssetRepository) GetEveryoneRoleForSet(setID int) (*int, error) {
	var roleID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?
	`, setID).Scan(&roleID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get everyone role: %w", err)
	}

	if !roleID.Valid {
		return nil, nil
	}

	id := int(roleID.Int64)
	return &id, nil
}

func (r *AssetRepository) ListAllRoles() ([]models.AssetRole, error) {
	rows, err := r.db.Query(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles ORDER BY display_order
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.AssetRole
	for rows.Next() {
		var role models.AssetRole
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate roles: %w", err)
	}

	return roles, nil
}

func (r *AssetRepository) GetRoleByID(roleID int) (*models.AssetRole, error) {
	var role models.AssetRole
	err := r.db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM asset_roles WHERE id = ?
	`, roleID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)

	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get role")
	}

	return &role, nil
}

func (r *AssetRepository) GetRolePermissions(roleID int) ([]models.AssetPermission, error) {
	rows, err := r.db.Query(`
		SELECT ap.id, ap.permission_key, ap.permission_name, ap.description, ap.created_at
		FROM asset_role_permissions arp
		JOIN asset_permissions ap ON arp.permission_id = ap.id
		WHERE arp.role_id = ?
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var permissions []models.AssetPermission
	for rows.Next() {
		var perm models.AssetPermission
		if err := rows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName, &perm.Description, &perm.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate role permissions: %w", err)
	}

	return permissions, nil
}

// Role-assignment operations.

func (r *AssetRepository) GetSetUserRoles(setID int) ([]models.UserAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT uasr.id, uasr.user_id, uasr.set_id, uasr.role_id, uasr.granted_by, uasr.granted_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
		       u.email as user_email,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM user_asset_set_roles uasr
		JOIN users u ON uasr.user_id = u.id
		JOIN asset_roles ar ON uasr.role_id = ar.id
		LEFT JOIN users g ON uasr.granted_by = g.id
		WHERE uasr.set_id = ?
		ORDER BY u.first_name, u.last_name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.UserAssetSetRole
	for rows.Next() {
		var role models.UserAssetSetRole
		var grantedByName sql.NullString
		if err := rows.Scan(&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&role.UserName, &role.UserEmail, &role.RoleName, &grantedByName); err != nil {
			return nil, fmt.Errorf("failed to scan user role: %w", err)
		}
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user roles: %w", err)
	}

	return roles, nil
}

func (r *AssetRepository) GetSetGroupRoles(setID int) ([]models.GroupAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT gasr.id, gasr.group_id, gasr.set_id, gasr.role_id, gasr.granted_by, gasr.granted_at,
		       tg.name as group_name,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM group_asset_set_roles gasr
		JOIN team_groups tg ON gasr.group_id = tg.id
		JOIN asset_roles ar ON gasr.role_id = ar.id
		LEFT JOIN users g ON gasr.granted_by = g.id
		WHERE gasr.set_id = ?
		ORDER BY tg.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []models.GroupAssetSetRole
	for rows.Next() {
		var role models.GroupAssetSetRole
		var grantedByName sql.NullString
		if err := rows.Scan(&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&role.GroupName, &role.RoleName, &grantedByName); err != nil {
			return nil, fmt.Errorf("failed to scan group role: %w", err)
		}
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate group roles: %w", err)
	}

	return roles, nil
}

// FindSetUserRolesByGrantDate returns user role assignments for a set, ordered by
// when they were granted (most recent first), using LEFT JOINs so orphaned rows
// (deleted user or role) still appear.
func (r *AssetRepository) FindSetUserRolesByGrantDate(setID int) ([]models.UserAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT uasr.id, uasr.user_id, uasr.set_id, uasr.role_id, uasr.granted_by, uasr.granted_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
		       u.email as user_email,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM user_asset_set_roles uasr
		LEFT JOIN users u ON uasr.user_id = u.id
		LEFT JOIN asset_roles ar ON uasr.role_id = ar.id
		LEFT JOIN users g ON uasr.granted_by = g.id
		WHERE uasr.set_id = ?
		ORDER BY uasr.granted_at DESC
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]models.UserAssetSetRole, 0)
	for rows.Next() {
		var role models.UserAssetSetRole
		var userName, userEmail, roleName, grantedByName sql.NullString
		if err := rows.Scan(
			&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&userName, &userEmail, &roleName, &grantedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user role: %w", err)
		}
		role.UserName = userName.String
		role.UserEmail = userEmail.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user roles by grant date: %w", err)
	}
	return roles, nil
}

// FindSetGroupRolesByGrantDate returns group role assignments for a set ordered
// by granted_at (most recent first), using LEFT JOINs consistent with the user variant.
func (r *AssetRepository) FindSetGroupRolesByGrantDate(setID int) ([]models.GroupAssetSetRole, error) {
	rows, err := r.db.Query(`
		SELECT gasr.id, gasr.group_id, gasr.set_id, gasr.role_id, gasr.granted_by, gasr.granted_at,
		       g.name as group_name,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM group_asset_set_roles gasr
		LEFT JOIN groups g ON gasr.group_id = g.id
		LEFT JOIN asset_roles ar ON gasr.role_id = ar.id
		LEFT JOIN users u ON gasr.granted_by = u.id
		WHERE gasr.set_id = ?
		ORDER BY gasr.granted_at DESC
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query group roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]models.GroupAssetSetRole, 0)
	for rows.Next() {
		var role models.GroupAssetSetRole
		var groupName, roleName, grantedByName sql.NullString
		if err := rows.Scan(
			&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&groupName, &roleName, &grantedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan group role: %w", err)
		}
		role.GroupName = groupName.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate group roles by grant date: %w", err)
	}
	return roles, nil
}

func (r *AssetRepository) AssetRoleExists(roleID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", roleID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check asset role existence: %w", err)
	}
	return exists, nil
}

func (r *AssetRepository) DeleteUserRoleAssignment(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(
		"DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?",
		assignmentID, setID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete user role assignment: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) DeleteGroupRoleAssignment(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(
		"DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?",
		assignmentID, setID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete group role assignment: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAssignmentRoleID returns ErrNotFound when the assignment does not exist.
func (r *AssetRepository) GetAssignmentRoleID(setID, assignmentID int, kind string) (int, error) {
	var query string
	if kind == "group" {
		query = `SELECT role_id FROM group_asset_set_roles WHERE id = ? AND set_id = ?`
	} else {
		query = `SELECT role_id FROM user_asset_set_roles WHERE id = ? AND set_id = ?`
	}
	var roleID int
	err := r.db.QueryRow(query, assignmentID, setID).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return roleID, err
}

func (r *AssetRepository) GetEveryoneRoleIDValueForSet(setID int) (sql.NullInt64, error) {
	var roleID sql.NullInt64
	err := r.db.QueryRow(`SELECT role_id FROM asset_set_everyone_roles WHERE set_id = ?`, setID).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return roleID, nil
	}
	if err != nil {
		return roleID, fmt.Errorf("failed to query everyone role id: %w", err)
	}
	return roleID, nil
}

func (r *AssetRepository) CountAdminAssignmentsExcluding(setID, adminRoleID, excludeID int, excludeKind string) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM user_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (id = ? AND ? = 'user'))
			+
			(SELECT COUNT(*) FROM group_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (id = ? AND ? = 'group'))
	`,
		setID, adminRoleID, excludeID, excludeKind,
		setID, adminRoleID, excludeID, excludeKind,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admin assignments: %w", err)
	}
	return count, nil
}

func (r *AssetRepository) GetPrincipalDirectRoleID(setID int, kind string, principalID int) (int, error) {
	var query string
	if kind == "group" {
		query = `SELECT role_id FROM group_asset_set_roles WHERE set_id = ? AND group_id = ?`
	} else {
		query = `SELECT role_id FROM user_asset_set_roles WHERE set_id = ? AND user_id = ?`
	}
	var roleID int
	err := r.db.QueryRow(query, setID, principalID).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return roleID, err
}

func (r *AssetRepository) CountAdminAssignmentsExcludingPrincipal(setID, adminRoleID int, excludeKind string, excludePrincipalID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM user_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (user_id = ? AND ? = 'user'))
			+
			(SELECT COUNT(*) FROM group_asset_set_roles WHERE set_id = ? AND role_id = ? AND NOT (group_id = ? AND ? = 'group'))
	`,
		setID, adminRoleID, excludePrincipalID, excludeKind,
		setID, adminRoleID, excludePrincipalID, excludeKind,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admin assignments: %w", err)
	}
	return count, nil
}

func (r *AssetRepository) CountAdminAssignments(setID, adminRoleID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM user_asset_set_roles WHERE set_id = ? AND role_id = ?)
			+
			(SELECT COUNT(*) FROM group_asset_set_roles WHERE set_id = ? AND role_id = ?)
	`, setID, adminRoleID, setID, adminRoleID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admin assignments: %w", err)
	}
	return count, nil
}

func (r *AssetRepository) AssignUserRole(setID, userID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (set_id, user_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, userID, roleID, grantedBy, now, roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to assign user role: %w", err)
	}
	return nil
}

func (r *AssetRepository) AssignGroupRole(setID, groupID, roleID, grantedBy int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		INSERT INTO group_asset_set_roles (set_id, group_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (set_id, group_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, groupID, roleID, grantedBy, now, roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to assign group role: %w", err)
	}
	return nil
}

func (r *AssetRepository) RevokeUserRole(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(`
		DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?
	`, assignmentID, setID)
	if err != nil {
		return fmt.Errorf("failed to revoke user role: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) RevokeGroupRole(assignmentID, setID int) error {
	result, err := r.db.ExecWrite(`
		DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?
	`, assignmentID, setID)
	if err != nil {
		return fmt.Errorf("failed to revoke group role: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) SetEveryoneRole(setID int, roleID *int, grantedBy int) error {
	now := time.Now()
	if roleID == nil {
		_, err := r.db.ExecWrite(`DELETE FROM asset_set_everyone_roles WHERE set_id = ?`, setID)
		return err
	}

	_, err := r.db.ExecWrite(`
		INSERT INTO asset_set_everyone_roles (set_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (set_id) DO UPDATE SET role_id = ?, granted_by = ?, granted_at = ?
	`, setID, *roleID, grantedBy, now, *roleID, grantedBy, now)

	if err != nil {
		return fmt.Errorf("failed to set everyone role: %w", err)
	}
	return nil
}

// Asset operations.

func (r *AssetRepository) GetAssetByID(assetID int) (*models.Asset, error) {
	var asset models.Asset
	var categoryID, statusID, createdBy sql.NullInt64
	var description, assetTag, fracIndex sql.NullString
	var categoryName, categoryPath, statusName, statusColor sql.NullString
	var assetTypeIcon, assetTypeColor sql.NullString
	var creatorName, creatorEmail sql.NullString
	var customFieldValuesJSON sql.NullString

	err := r.db.QueryRow(`
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id,
		       a.title, a.description, a.asset_tag, a.custom_field_values,
		       a.frac_index, a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email
		FROM assets a
		JOIN asset_management_sets ams ON a.set_id = ams.id
		JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, assetID).Scan(
		&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID,
		&asset.Title, &description, &assetTag, &customFieldValuesJSON,
		&fracIndex, &createdBy, &asset.CreatedAt, &asset.UpdatedAt,
		&asset.SetName, &asset.AssetTypeName, &assetTypeIcon, &assetTypeColor,
		&categoryName, &categoryPath, &statusName, &statusColor,
		&creatorName, &creatorEmail,
	)

	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get asset")
	}

	if categoryID.Valid {
		id := int(categoryID.Int64)
		asset.CategoryID = &id
	}
	if statusID.Valid {
		id := int(statusID.Int64)
		asset.StatusID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		asset.CreatedBy = &id
	}
	asset.Description = description.String
	asset.AssetTag = assetTag.String
	if fracIndex.Valid {
		asset.FracIndex = &fracIndex.String
	}
	asset.AssetTypeIcon = assetTypeIcon.String
	asset.AssetTypeColor = assetTypeColor.String
	asset.CategoryName = categoryName.String
	asset.CategoryPath = categoryPath.String
	asset.StatusName = statusName.String
	asset.StatusColor = statusColor.String
	asset.CreatorName = creatorName.String
	asset.CreatorEmail = creatorEmail.String

	return &asset, nil
}

func (r *AssetRepository) GetAssetSetID(assetID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err != nil {
		return 0, notFoundOrWrap(err, "failed to get asset set ID")
	}
	return setID, nil
}

func (r *AssetRepository) DeleteAsset(assetID int) error {
	result, err := r.db.ExecWrite("DELETE FROM assets WHERE id = ?", assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Validation helpers.

func (r *AssetRepository) AssetTypeBelongsToSet(typeID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_types WHERE id = ? AND set_id = ?)", typeID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check asset type: %w", err)
	}
	return exists, nil
}

func (r *AssetRepository) CategoryBelongsToSet(categoryID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_categories WHERE id = ? AND set_id = ?)", categoryID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check category: %w", err)
	}
	return exists, nil
}

func (r *AssetRepository) StatusBelongsToSet(statusID, setID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_statuses WHERE id = ? AND set_id = ?)", statusID, setID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check status: %w", err)
	}
	return exists, nil
}

func (r *AssetRepository) GetDefaultStatus(setID int) (*int, error) {
	var statusID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id FROM asset_statuses WHERE set_id = ? AND is_default = true LIMIT 1
	`, setID).Scan(&statusID)

	if errors.Is(err, sql.ErrNoRows) || !statusID.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get default status: %w", err)
	}

	id := int(statusID.Int64)
	return &id, nil
}

// EnsureImportedDefaultStatus returns a set's default status, creating the
// Jira-import default when a legacy or partially-created set has none.
func (r *AssetRepository) EnsureImportedDefaultStatus(setID int) (int, error) {
	statusID, err := r.GetDefaultStatus(setID)
	if err != nil {
		return 0, err
	}
	if statusID != nil {
		return *statusID, nil
	}
	now := time.Now()
	var id int
	err = r.db.QueryRow(`
		INSERT INTO asset_statuses
			(set_id, name, color, description, is_default, display_order, created_at, updated_at)
		VALUES (?, 'Active', '#22c55e', 'Default status for imported Jira Assets', true, 0, ?, ?)
		RETURNING id
	`, setID, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create imported default asset status: %w", err)
	}
	return id, nil
}

// RoleExists checks if a role exists
func (r *AssetRepository) RoleExists(roleID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", roleID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role: %w", err)
	}
	return exists, nil
}

// Asset-type operations.

func (r *AssetRepository) FindAssetTypesForSet(setID int) ([]models.AssetType, error) {
	rows, err := r.db.Query(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.set_id = ?
		ORDER BY at.display_order, at.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	types := make([]models.AssetType, 0)
	for rows.Next() {
		at, err := scanAssetTypeRow(rows)
		if err != nil {
			return nil, err
		}
		types = append(types, at)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset types: %w", err)
	}
	return types, nil
}

// FindAssetTypeIDByName returns the ID of an exact-name type within a set.
func (r *AssetRepository) FindAssetTypeIDByName(setID int, name string) (int, error) {
	var typeID int
	err := r.db.QueryRow(
		"SELECT id FROM asset_types WHERE set_id = ? AND name = ?",
		setID,
		name,
	).Scan(&typeID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find asset type by name: %w", err)
	}
	return typeID, nil
}

// FindAssetTypeByID returns a single asset type with set name and asset count.
// Returns ErrNotFound if the type does not exist.
func (r *AssetRepository) FindAssetTypeByID(typeID int) (*models.AssetType, error) {
	row := r.db.QueryRow(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.id = ?
	`, typeID)
	at, err := scanAssetTypeRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &at, nil
}

func (r *AssetRepository) GetAssetTypeSetID(typeID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err != nil {
		return 0, notFoundOrWrap(err, "failed to get asset type set")
	}
	return setID, nil
}

func (r *AssetRepository) GetAssetTypeSetAndCount(typeID int) (setID, assetCount int, err error) {
	err = r.db.QueryRow(`
		SELECT set_id, (SELECT COUNT(*) FROM assets WHERE asset_type_id = ?) as asset_count
		FROM asset_types WHERE id = ?
	`, typeID, typeID).Scan(&setID, &assetCount)
	if err != nil {
		return 0, 0, notFoundOrWrap(err, "failed to get asset type set/count")
	}
	return setID, assetCount, nil
}

func (r *AssetRepository) CreateAssetType(at *models.AssetType) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, at.SetID, at.Name, at.Description, at.Icon, at.Color, at.DisplayOrder, at.IsActive, at.CreatedAt, at.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset type: %w", err)
	}
	return int(id), nil
}

// AssetTypeUpdate holds patchable asset-type fields. Nil pointer fields preserve
// their current values.
type AssetTypeUpdate struct {
	Name         string
	Description  string
	Icon         string
	Color        string
	DisplayOrder *int
	IsActive     *bool
}

func (r *AssetRepository) UpdateAssetType(typeID int, patch AssetTypeUpdate) error {
	query := "UPDATE asset_types SET name = ?, description = ?, icon = ?, color = ?, updated_at = ?"
	args := []any{patch.Name, patch.Description, patch.Icon, patch.Color, time.Now()}

	if patch.DisplayOrder != nil {
		query += ", display_order = ?"
		args = append(args, *patch.DisplayOrder)
	}

	if patch.IsActive != nil {
		query += ", is_active = ?"
		args = append(args, *patch.IsActive)
	}
	query += " WHERE id = ?"
	args = append(args, typeID)

	result, err := r.db.ExecWrite(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update asset type: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) DeleteAssetType(typeID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID); err != nil {
		return fmt.Errorf("failed to delete asset type fields: %w", err)
	}

	result, err := tx.Exec("DELETE FROM asset_types WHERE id = ?", typeID)
	if err != nil {
		return fmt.Errorf("failed to delete asset type: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (r *AssetRepository) GetAssetTypeCoreByID(typeID int) (*models.AssetType, error) {
	var at models.AssetType
	err := r.db.QueryRow(`
		SELECT id, set_id, name, description, icon, color, display_order, is_active, created_at, updated_at
		FROM asset_types WHERE id = ?
	`, typeID).Scan(
		&at.ID, &at.SetID, &at.Name, &at.Description,
		&at.Icon, &at.Color, &at.DisplayOrder, &at.IsActive,
		&at.CreatedAt, &at.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to fetch asset type")
	}
	return &at, nil
}

func (r *AssetRepository) FindAssetTypeFields(typeID int) ([]models.AssetTypeField, error) {
	rows, err := r.db.Query(`
		SELECT atf.id, atf.asset_type_id, atf.custom_field_id, atf.is_required, atf.display_order, atf.created_at,
		       cfd.name as field_name, cfd.field_type, cfd.description as field_description, cfd.options
		FROM asset_type_fields atf
		JOIN custom_field_definitions cfd ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ?
		ORDER BY atf.display_order, cfd.name
	`, typeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset type fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fields := make([]models.AssetTypeField, 0)
	for rows.Next() {
		var field models.AssetTypeField
		var fieldDescription, options sql.NullString
		if err := rows.Scan(
			&field.ID, &field.AssetTypeID, &field.CustomFieldID, &field.IsRequired,
			&field.DisplayOrder, &field.CreatedAt,
			&field.FieldName, &field.FieldType, &fieldDescription, &options,
		); err != nil {
			return nil, fmt.Errorf("failed to scan asset type field: %w", err)
		}
		if fieldDescription.Valid {
			field.FieldDescription = fieldDescription.String
		}
		if options.Valid {
			field.Options = options.String
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset type fields: %w", err)
	}
	return fields, nil
}

type AssetTypeFieldAssignment struct {
	CustomFieldID int
	IsRequired    bool
	DisplayOrder  int
}

// UpsertAssetTypeField creates or updates one custom-field assignment.
func (r *AssetRepository) UpsertAssetTypeField(typeID, fieldID int, required bool, displayOrder int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(asset_type_id, custom_field_id) DO UPDATE SET
			is_required = excluded.is_required,
			display_order = excluded.display_order
	`, typeID, fieldID, required, displayOrder)
	if err != nil {
		return fmt.Errorf("upsert asset type field: %w", err)
	}
	return nil
}

// ReplaceAssetTypeFields atomically replaces an asset type's custom field assignments.
// It deletes existing rows and inserts the provided set in a single transaction.
func (r *AssetRepository) ReplaceAssetTypeFields(typeID int, fields []AssetTypeFieldAssignment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`
		SELECT atf.custom_field_id, cfd.name
		FROM asset_type_fields atf
		JOIN custom_field_definitions cfd ON cfd.id = atf.custom_field_id
		WHERE atf.asset_type_id = ?
	`, typeID)
	if err != nil {
		return fmt.Errorf("failed to load existing type fields: %w", err)
	}
	existingKeys := make(map[int]string)
	for rows.Next() {
		var fieldID int
		var fieldName string
		if err := rows.Scan(&fieldID, &fieldName); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan existing type field: %w", err)
		}
		existingKeys[fieldID] = fieldName
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate existing type fields: %w", err)
	}
	_ = rows.Close()

	if _, err := tx.Exec("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID); err != nil {
		return fmt.Errorf("failed to delete existing type fields: %w", err)
	}

	retained := make(map[int]struct{}, len(fields))
	now := time.Now()
	for _, f := range fields {
		retained[f.CustomFieldID] = struct{}{}
		if _, err := tx.Exec(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, typeID, f.CustomFieldID, f.IsRequired, f.DisplayOrder, now); err != nil {
			return fmt.Errorf("failed to insert type field: %w", err)
		}
	}

	removedKeys := make(map[string]struct{})
	for fieldID, fieldName := range existingKeys {
		if _, ok := retained[fieldID]; ok {
			continue
		}
		removedKeys[fmt.Sprintf("%d", fieldID)] = struct{}{}
		removedKeys[fieldName] = struct{}{}
		removedKeys[strings.ToLower(fieldName)] = struct{}{}
	}
	if len(removedKeys) > 0 {
		if err := pruneRemovedAssetTypeValues(tx, typeID, removedKeys, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func pruneRemovedAssetTypeValues(tx database.Tx, typeID int, removedKeys map[string]struct{}, updatedAt time.Time) error {
	rows, err := tx.Query(`
		SELECT id, custom_field_values
		FROM assets
		WHERE asset_type_id = ? AND custom_field_values IS NOT NULL
	`, typeID)
	if err != nil {
		return fmt.Errorf("failed to load assets for custom-field pruning: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type prunedAsset struct {
		id     int
		values string
	}
	var updates []prunedAsset
	for rows.Next() {
		var assetID int
		var raw string
		if err := rows.Scan(&assetID, &raw); err != nil {
			return fmt.Errorf("failed to scan asset custom fields: %w", err)
		}
		values := map[string]any{}
		if raw == "" {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return fmt.Errorf("failed to decode custom fields for asset %d: %w", assetID, err)
		}
		changed := false
		for key := range values {
			if _, remove := removedKeys[key]; remove {
				delete(values, key)
				changed = true
			}
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return fmt.Errorf("failed to encode custom fields for asset %d: %w", assetID, err)
		}
		updates = append(updates, prunedAsset{id: assetID, values: string(encoded)})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate assets for custom-field pruning: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close assets custom-field rows: %w", err)
	}
	for _, update := range updates {
		if _, err := tx.Exec(
			"UPDATE assets SET custom_field_values = ?, updated_at = ? WHERE id = ?",
			update.values, updatedAt, update.id,
		); err != nil {
			return fmt.Errorf("failed to prune custom fields for asset %d: %w", update.id, err)
		}
	}
	return nil
}

func scanAssetTypeRow(scanner interface {
	Scan(dest ...any) error
}) (models.AssetType, error) {
	var at models.AssetType
	var description, setName sql.NullString
	if err := scanner.Scan(
		&at.ID, &at.SetID, &at.Name, &description,
		&at.Icon, &at.Color, &at.DisplayOrder,
		&at.IsActive, &at.CreatedAt, &at.UpdatedAt,
		&setName, &at.AssetCount,
	); err != nil {
		return at, err
	}
	if description.Valid {
		at.Description = description.String
	}
	if setName.Valid {
		at.SetName = setName.String
	}
	return at, nil
}

// Asset-category operations.

func (r *AssetRepository) FindAssetCategoriesForSet(setID int) ([]models.AssetCategory, error) {
	rows, err := r.db.Query(`
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.set_id = ?
		ORDER BY ac.frac_index, ac.name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	categories := make([]models.AssetCategory, 0)
	for rows.Next() {
		cat, err := scanAssetCategoryRow(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset categories: %w", err)
	}
	return categories, nil
}

func (r *AssetRepository) FindAssetCategoryByID(categoryID int) (*models.AssetCategory, error) {
	row := r.db.QueryRow(`
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.id = ?
	`, categoryID)
	cat, err := scanAssetCategoryRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *AssetRepository) GetAssetCategoryCoreByID(categoryID int) (*models.AssetCategory, error) {
	row := r.db.QueryRow(`
		SELECT id, set_id, name, description, parent_id, path,
		       has_children, children_count, descendants_count, frac_index,
		       created_at, updated_at
		FROM asset_categories WHERE id = ?
	`, categoryID)
	cat, err := scanAssetCategoryCoreRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *AssetRepository) GetAssetCategorySetID(categoryID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", categoryID).Scan(&setID)
	if err != nil {
		return 0, notFoundOrWrap(err, "failed to get asset category set")
	}
	return setID, nil
}

func (r *AssetRepository) GetAssetCategoryParentID(categoryID int) (sql.NullInt64, error) {
	var parentID sql.NullInt64
	err := r.db.QueryRow("SELECT parent_id FROM asset_categories WHERE id = ?", categoryID).Scan(&parentID)
	if err != nil {
		return parentID, notFoundOrWrap(err, "failed to get parent id")
	}
	return parentID, nil
}

func (r *AssetRepository) GetAssetCategoryDeletionInfo(categoryID int) (setID int, hasChildren bool, parentID sql.NullInt64, assetCount int, err error) {
	err = r.db.QueryRow(`
		SELECT set_id, has_children, parent_id,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ?) as asset_count
		FROM asset_categories WHERE id = ?
	`, categoryID, categoryID).Scan(&setID, &hasChildren, &parentID, &assetCount)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to get category deletion info: %w", err)
	}
	return
}

type CreateAssetCategoryInput struct {
	SetID       int
	Name        string
	Description string
	ParentID    *int
}

func (r *AssetRepository) CreateAssetCategory(input CreateAssetCategoryInput) (int, time.Time, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO asset_categories (set_id, name, description, parent_id, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, '/', ?, ?) RETURNING id
	`, input.SetID, input.Name, input.Description, input.ParentID, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to create asset category: %w", err)
	}

	if input.ParentID != nil {
		if err := updateCategoryParentCounts(tx, *input.ParentID); err != nil {
			return 0, time.Time{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to commit category create: %w", err)
	}
	return int(id), now, nil
}

func (r *AssetRepository) UpdateAssetCategoryNameDescription(categoryID int, name, description string) error {
	result, err := r.db.ExecWrite(`
		UPDATE asset_categories SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, name, description, time.Now(), categoryID)
	if err != nil {
		return fmt.Errorf("failed to update asset category: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) DeleteAssetCategory(categoryID int, oldParentID sql.NullInt64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM asset_categories WHERE id = ?", categoryID)
	if err != nil {
		return fmt.Errorf("failed to delete asset category: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}

	if oldParentID.Valid {
		if err := updateCategoryParentCounts(tx, int(oldParentID.Int64)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *AssetRepository) MoveAssetCategory(categoryID int, oldParentID sql.NullInt64, newParentID *int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		"UPDATE asset_categories SET parent_id = ?, updated_at = ? WHERE id = ?",
		newParentID, time.Now(), categoryID,
	); err != nil {
		return fmt.Errorf("failed to move asset category: %w", err)
	}

	if oldParentID.Valid {
		if err := updateCategoryParentCounts(tx, int(oldParentID.Int64)); err != nil {
			return err
		}
	}
	if newParentID != nil {
		if err := updateCategoryParentCounts(tx, *newParentID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *AssetRepository) IsAssetCategoryDescendantOf(potentialDescendant, ancestor int) (bool, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id FROM asset_categories WHERE id = ?
			UNION ALL
			SELECT ac.parent_id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.parent_id
			WHERE ac.parent_id IS NOT NULL
		)
		SELECT 1 FROM ancestors WHERE parent_id = ? LIMIT 1
	`, potentialDescendant, ancestor)
	if err != nil {
		return false, fmt.Errorf("failed to query category ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to iterate category ancestors: %w", err)
	}
	return found, nil
}

func updateCategoryParentCounts(tx database.Tx, parentID int) error {
	var childrenCount int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM asset_categories WHERE parent_id = ?",
		parentID,
	).Scan(&childrenCount); err != nil {
		return fmt.Errorf("failed to count children: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE asset_categories
		SET children_count = ?, has_children = ?, updated_at = ?
		WHERE id = ?
	`, childrenCount, childrenCount > 0, time.Now(), parentID); err != nil {
		return fmt.Errorf("failed to update parent counts: %w", err)
	}

	if _, err := tx.Exec(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id as id FROM asset_categories WHERE id = ? AND parent_id IS NOT NULL
			UNION ALL
			SELECT ac.parent_id as id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.id
			WHERE ac.parent_id IS NOT NULL
		)
		UPDATE asset_categories
		SET descendants_count = (
			WITH RECURSIVE descendants AS (
				SELECT id FROM asset_categories WHERE parent_id = asset_categories.id
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN descendants d ON ac.parent_id = d.id
			)
			SELECT COUNT(*) FROM descendants
		)
		WHERE id IN (SELECT id FROM ancestors)
	`, parentID); err != nil {
		return fmt.Errorf("failed to update ancestor descendants: %w", err)
	}

	return nil
}

func scanAssetCategoryRow(scanner interface{ Scan(...any) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex, setName, parentName sql.NullString
	var parentID sql.NullInt64

	if err := scanner.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
		&setName, &parentName, &cat.AssetCount,
	); err != nil {
		return cat, err
	}
	cat.Description = description.String
	if parentID.Valid {
		v := int(parentID.Int64)
		cat.ParentID = &v
	}
	cat.Path = path.String
	if fracIndex.Valid {
		v := fracIndex.String
		cat.FracIndex = &v
	}
	cat.SetName = setName.String
	cat.ParentName = parentName.String
	return cat, nil
}

func scanAssetCategoryCoreRow(scanner interface{ Scan(...any) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex sql.NullString
	var parentID sql.NullInt64
	if err := scanner.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
	); err != nil {
		return cat, err
	}
	cat.Description = description.String
	if parentID.Valid {
		v := int(parentID.Int64)
		cat.ParentID = &v
	}
	cat.Path = path.String
	if fracIndex.Valid {
		v := fracIndex.String
		cat.FracIndex = &v
	}
	return cat, nil
}

// Asset CRUD operations.

// AssetRowToModel is the shared row-to-model conversion for both API surfaces.
// Invalid custom-field JSON becomes an empty map with a warning.
func AssetRowToModel(row AssetRow) models.Asset {
	asset := models.Asset{
		ID:              row.ID,
		SetID:           row.SetID,
		AssetTypeID:     row.AssetTypeID,
		CategoryID:      utils.NullInt64ToPtr(row.CategoryID),
		StatusID:        utils.NullInt64ToPtr(row.StatusID),
		Title:           row.Title,
		Description:     row.Description.String,
		AssetTag:        row.AssetTag.String,
		FracIndex:       utils.NullStringToPtr(row.FracIndex),
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		SetName:         row.SetName.String,
		AssetTypeName:   row.AssetTypeName.String,
		AssetTypeIcon:   row.AssetTypeIcon.String,
		AssetTypeColor:  row.AssetTypeColor.String,
		CategoryName:    row.CategoryName.String,
		CategoryPath:    row.CategoryPath.String,
		StatusName:      row.StatusName.String,
		StatusColor:     row.StatusColor.String,
		CreatorName:     row.CreatorName.String,
		CreatorEmail:    row.CreatorEmail.String,
		LinkedItemCount: row.LinkedItemCount,
	}
	if row.CustomFieldValues.Valid && row.CustomFieldValues.String != "" {
		if err := json.Unmarshal([]byte(row.CustomFieldValues.String), &asset.CustomFieldValues); err != nil {
			slog.Error("failed to unmarshal asset custom_field_values",
				slog.Int("asset_id", asset.ID),
				slog.String("raw", row.CustomFieldValues.String),
				slog.Any("error", err))
			asset.CustomFieldValues = make(map[string]any)
			asset.Warnings = append(asset.Warnings, "custom field values could not be parsed")
		}
	}
	return asset
}

type AssetRow struct {
	ID                int
	SetID             int
	AssetTypeID       int
	CategoryID        sql.NullInt64
	StatusID          sql.NullInt64
	Title             string
	Description       sql.NullString
	AssetTag          sql.NullString
	CustomFieldValues sql.NullString
	FracIndex         sql.NullString
	CreatedBy         *int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SetName           sql.NullString
	AssetTypeName     sql.NullString
	AssetTypeIcon     sql.NullString
	AssetTypeColor    sql.NullString
	CategoryName      sql.NullString
	CategoryPath      sql.NullString
	StatusName        sql.NullString
	StatusColor       sql.NullString
	CreatorName       sql.NullString
	CreatorEmail      sql.NullString
	LinkedItemCount   int
}

type AssetListFilter struct {
	SetID                int
	AssetTypeID          string // raw string from query param (empty for no filter)
	CategoryID           string // raw string; if set and IncludeSubcategories is true, recursive CTE is used
	IncludeSubcategories bool
	StatusID             string // raw string
	Search               string
	CQLSQL               string
	CQLArgs              []any
	Limit                int
	Offset               int
}

func (r *AssetRepository) CountAssets(f AssetListFilter) (int, error) {
	cte, where, args := buildAssetListWhere(f)
	query := cte + `SELECT COUNT(*) FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + where

	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count assets: %w", err)
	}
	return total, nil
}

func (r *AssetRepository) ListAssets(f AssetListFilter) ([]AssetRow, error) {
	cte, where, args := buildAssetListWhere(f)
	args = append(args, f.Limit, f.Offset)

	query := cte + `
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + where + `
		ORDER BY a.frac_index, a.title
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]AssetRow, 0)
	for rows.Next() {
		row, err := scanAssetRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate assets: %w", err)
	}
	return result, nil
}

func (r *AssetRepository) FindAssetFullByID(assetID int) (*AssetRow, error) {
	row := r.db.QueryRow(`
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, assetID)
	assetRow, err := scanAssetRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &assetRow, nil
}

type AssetUpdateSnapshot struct {
	SetID                 int
	StatusID              sql.NullInt64
	AssetTypeID           int
	CustomFieldValuesJSON sql.NullString
	UpdatedAt             time.Time
}

func (row AssetRow) UpdateSnapshot() AssetUpdateSnapshot {
	return AssetUpdateSnapshot{SetID: row.SetID, StatusID: row.StatusID, AssetTypeID: row.AssetTypeID,
		CustomFieldValuesJSON: row.CustomFieldValues, UpdatedAt: row.UpdatedAt}
}

func (r *AssetRepository) GetAssetUpdateSnapshot(assetID int) (*AssetUpdateSnapshot, error) {
	var snap AssetUpdateSnapshot
	err := r.db.QueryRow(
		`SELECT set_id, status_id, asset_type_id, custom_field_values, updated_at FROM assets WHERE id = ?`,
		assetID,
	).Scan(&snap.SetID, &snap.StatusID, &snap.AssetTypeID, &snap.CustomFieldValuesJSON, &snap.UpdatedAt)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to fetch asset snapshot")
	}
	return &snap, nil
}

// LockAssetVersion holds the row until the transaction ends. SQLite transactions
// already hold the write lock; PostgreSQL requires an explicit row lock.
func (r *AssetRepository) LockAssetVersion(tx database.Tx, assetID int) (time.Time, error) {
	query := "SELECT updated_at FROM assets WHERE id = ?"
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	var updatedAt time.Time
	if err := tx.QueryRow(query, assetID).Scan(&updatedAt); err != nil {
		return time.Time{}, notFoundOrWrap(err, "lock asset version")
	}
	return updatedAt, nil
}

func (r *AssetRepository) GetAssetSetAndTitle(assetID int) (setID int, title string, err error) {
	err = r.db.QueryRow(`SELECT set_id, title FROM assets WHERE id = ?`, assetID).Scan(&setID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to fetch asset set/title: %w", err)
	}
	return
}

// GetResourceSetID reads set_id from an allowlisted asset child table.
func (r *AssetRepository) GetResourceSetID(table string, resourceID int) (int, error) {
	allowed := map[string]bool{
		"asset_types":      true,
		"asset_categories": true,
		"asset_statuses":   true,
	}
	if !allowed[table] {
		return 0, fmt.Errorf("table %q is not a valid asset-scoped resource table", table)
	}
	var setID int
	//nolint:gosec // table name validated via allowlist above
	err := r.db.QueryRow(`SELECT set_id FROM `+table+` WHERE id = ?`, resourceID).Scan(&setID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to fetch resource set id: %w", err)
	}
	return setID, nil
}

type CreateAssetInput struct {
	SetID                 int
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
	CreatedBy             int
	CreatedAt             time.Time
}

func (r *AssetRepository) CreateAsset(in CreateAssetInput) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var err error
		id, err = r.CreateAssetInTx(tx, in)
		return err
	})
	return id, err
}

func (r *AssetRepository) CreateAssetInTx(tx database.Tx, in CreateAssetInput) (int, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, in.CreatedBy, in.CreatedAt, in.CreatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset: %w", err)
	}
	return int(id), nil
}

type UpdateAssetInput struct {
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
}

func (r *AssetRepository) UpdateAsset(assetID int, in UpdateAssetInput) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		return r.UpdateAssetInTx(tx, assetID, in)
	})
}

func (r *AssetRepository) UpdateAssetInTx(tx database.Tx, assetID int, in UpdateAssetInput) error {
	result, err := tx.ExecWrite(`
		UPDATE assets
		SET asset_type_id = ?, category_id = ?, status_id = ?, title = ?, description = ?,
		    asset_tag = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, time.Now(), assetID)
	if err != nil {
		return fmt.Errorf("failed to update asset: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssetRepository) DeleteAssetWithLinks(assetID int, eventMetadata ...itemevents.Metadata) error {
	metadata := itemevents.System("application")
	if len(eventMetadata) > 0 {
		metadata = eventMetadata[0]
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		return r.DeleteAssetWithLinksInTx(context.Background(), tx, assetID, metadata)
	})
}

func (r *AssetRepository) DeleteAssetWithLinksInTx(ctx context.Context, tx database.Tx, assetID int, metadata itemevents.Metadata) error {
	if err := itemevents.NewRecorder(r.db).RemovedLinks(ctx, tx, "asset", []int{assetID}, metadata); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM item_links WHERE (source_type = 'asset' AND source_id = ?) OR (target_type = 'asset' AND target_id = ?)`,
		assetID, assetID,
	); err != nil {
		return fmt.Errorf("failed to delete asset links: %w", err)
	}

	result, err := tx.Exec(`DELETE FROM assets WHERE id = ?`, assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}

	return nil
}

// scanAssetRow populates an AssetRow from the full joined projection.
func scanAssetRow(scanner interface{ Scan(...any) error }) (AssetRow, error) {
	var row AssetRow
	err := scanner.Scan(
		&row.ID, &row.SetID, &row.AssetTypeID, &row.CategoryID, &row.StatusID, &row.Title, &row.Description,
		&row.AssetTag, &row.CustomFieldValues, &row.FracIndex,
		&row.CreatedBy, &row.CreatedAt, &row.UpdatedAt,
		&row.SetName, &row.AssetTypeName, &row.AssetTypeIcon, &row.AssetTypeColor,
		&row.CategoryName, &row.CategoryPath, &row.StatusName, &row.StatusColor,
		&row.CreatorName, &row.CreatorEmail, &row.LinkedItemCount,
	)
	return row, err
}

func buildAssetListWhere(f AssetListFilter) (ctePrefix, whereClause string, args []any) {
	whereClause = "WHERE a.set_id = ?"
	args = []any{f.SetID}

	if f.AssetTypeID != "" {
		whereClause += " AND a.asset_type_id = ?"
		args = append(args, f.AssetTypeID)
	}

	if f.CategoryID != "" {
		if f.IncludeSubcategories {
			ctePrefix = `WITH RECURSIVE category_tree AS (
				SELECT id FROM asset_categories WHERE id = ?
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN category_tree ct ON ac.parent_id = ct.id
			) `
			whereClause += " AND a.category_id IN (SELECT id FROM category_tree)"
			// CTE parameter comes first.
			args = append([]any{f.CategoryID}, args...)
		} else {
			whereClause += " AND a.category_id = ?"
			args = append(args, f.CategoryID)
		}
	}

	if f.StatusID != "" {
		whereClause += " AND a.status_id = ?"
		args = append(args, f.StatusID)
	}

	if f.Search != "" {
		whereClause += " AND (a.title LIKE ? OR a.description LIKE ? OR a.asset_tag LIKE ?)"
		term := "%" + f.Search + "%"
		args = append(args, term, term, term)
	}

	if f.CQLSQL != "" {
		whereClause += " AND (" + f.CQLSQL + ")"
		args = append(args, f.CQLArgs...)
	}
	return ctePrefix, whereClause, args
}

// Asset-import operations.

type ImportJobRow struct {
	JobID        string
	Status       sql.NullString
	Phase        sql.NullString
	ProgressJSON sql.NullString
	ErrorMessage sql.NullString
	CreatedAt    sql.NullTime
	StartedAt    sql.NullTime
	CompletedAt  sql.NullTime
}

func (r *AssetRepository) CreateImportJob(jobID string, setID int, filePath, configJSON string, createdBy int, createdAt time.Time) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO asset_import_jobs (id, set_id, status, phase, file_path, config_json, created_by, created_at, lease_expires_at)
		VALUES (?, ?, 'queued', 'initializing', ?, ?, ?, ?, ?)
	`, jobID, setID, filePath, configJSON, createdBy, createdAt, createdAt.Add(assetImportLeaseDuration).Unix())
	if err != nil {
		return fmt.Errorf("failed to create import job: %w", err)
	}
	return nil
}

func (r *AssetRepository) GetImportJob(jobID string, setID int) (*ImportJobRow, error) {
	row := ImportJobRow{JobID: jobID}
	err := r.db.QueryRow(`
		SELECT status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE id = ? AND set_id = ?
	`, jobID, setID).Scan(&row.Status, &row.Phase, &row.ProgressJSON, &row.ErrorMessage, &row.CreatedAt, &row.StartedAt, &row.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get import job: %w", err)
	}
	return &row, nil
}

func (r *AssetRepository) ListImportJobs(setID, limit int) ([]ImportJobRow, error) {
	rows, err := r.db.Query(`
		SELECT id, status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE set_id = ? ORDER BY created_at DESC LIMIT ?
	`, setID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]ImportJobRow, 0)
	for rows.Next() {
		var job ImportJobRow
		if err := rows.Scan(&job.JobID, &job.Status, &job.Phase, &job.ProgressJSON, &job.ErrorMessage, &job.CreatedAt, &job.StartedAt, &job.CompletedAt); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate import jobs: %w", err)
	}
	return jobs, nil
}

func (r *AssetRepository) deleteAssetsFromImportJobInTx(tx database.Tx, jobID string) error {
	rows, err := tx.Query("SELECT id FROM assets WHERE import_job_id = ? ORDER BY id", jobID)
	if err != nil {
		return fmt.Errorf("load asset import links: %w", err)
	}
	var assetIDs []int
	for rows.Next() {
		var assetID int
		if err := rows.Scan(&assetID); err != nil {
			_ = rows.Close()
			return err
		}
		assetIDs = append(assetIDs, assetID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := itemevents.NewRecorder(r.db).RemovedLinks(context.Background(), tx, "asset", assetIDs, itemevents.Import(jobID)); err != nil {
		return err
	}
	if _, err := tx.ExecWrite(`DELETE FROM item_links
        WHERE (source_type = 'asset' AND source_id IN (SELECT id FROM assets WHERE import_job_id = ?))
           OR (target_type = 'asset' AND target_id IN (SELECT id FROM assets WHERE import_job_id = ?))`, jobID, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecWrite(`DELETE FROM assets WHERE import_job_id = ?`, jobID); err != nil {
		return fmt.Errorf("failed to delete assets for job %s: %w", jobID, err)
	}
	return nil
}

type ImportAssetRowInput struct {
	SetID                 int
	AssetTypeID           int
	CategoryID            *int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON *string
	ImportJobID           string
	CreatedBy             int
	CreatedAt             time.Time
}

// JiraImportAssetRowInput preserves Jira's optional created/updated timestamps.
type JiraImportAssetRowInput struct {
	SetID                 int
	AssetTypeID           int
	StatusID              *int
	Title                 string
	Description           string
	AssetTag              string
	CustomFieldValuesJSON string
	ImportJobID           string
	CreatedAt             *time.Time
	UpdatedAt             *time.Time
}

// InsertJiraImportedAsset inserts an asset while retaining Jira timestamps
// when present and otherwise using the database clock.
func (r *AssetRepository) InsertJiraImportedAsset(in JiraImportAssetRowInput) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var err error
		id, err = r.InsertJiraImportedAssetInTx(tx, in)
		return err
	})
	return id, err
}

func (r *AssetRepository) InsertJiraImportedAssetInTx(tx database.Tx, in JiraImportAssetRowInput) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO assets
			(set_id, asset_type_id, status_id, title, description, asset_tag,
			 custom_field_values, import_job_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP), COALESCE(?, CURRENT_TIMESTAMP))
		RETURNING id
	`, in.SetID, in.AssetTypeID, in.StatusID, in.Title, in.Description,
		in.AssetTag, in.CustomFieldValuesJSON, in.ImportJobID, in.CreatedAt, in.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert Jira imported asset: %w", err)
	}
	return id, nil
}

// InsertImportedAsset inserts a single asset row during CSV import.
func (r *AssetRepository) InsertImportedAsset(in ImportAssetRowInput) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var err error
		id, err = r.InsertImportedAssetInTx(tx, in)
		return err
	})
	return id, err
}

func (r *AssetRepository) InsertImportedAssetInTx(tx database.Tx, in ImportAssetRowInput) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, import_job_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag,
		in.CustomFieldValuesJSON, in.ImportJobID, in.CreatedBy, in.CreatedAt, in.CreatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert imported asset: %w", err)
	}
	return id, nil
}

func (r *AssetRepository) GetCustomFieldTypeAndOptions(fieldID int) (fieldType string, options sql.NullString, err error) {
	err = r.db.QueryRow(
		`SELECT field_type, options FROM custom_field_definitions WHERE id = ?`,
		fieldID,
	).Scan(&fieldType, &options)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to query custom field definition: %w", err)
	}
	return
}

func (r *AssetRepository) StartImportJobRunning(jobID, phase, progressJSON string) error {
	now := time.Now().UTC()
	return assetImportWriteResult(r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = 'running', phase = ?, progress_json = ?, started_at = ?, lease_expires_at = ?
         WHERE id = ? AND status = 'queued' AND lease_expires_at > ?`,
		phase, progressJSON, now, now.Add(assetImportLeaseDuration).Unix(), jobID, now.Unix()))
}

func (r *AssetRepository) FinishImportJob(jobID, status, phase, progressJSON, errorMessage string) error {
	now := time.Now().UTC()
	return assetImportWriteResult(r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = ?
         WHERE id = ? AND status IN ('queued', 'running') AND lease_expires_at > ?`,
		status, phase, progressJSON, errorMessage, now, jobID, now.Unix()))
}

func (r *AssetRepository) UpdateImportJobStatus(jobID, status, phase, progressJSON string) error {
	now := time.Now().UTC()
	return assetImportWriteResult(r.db.ExecWrite(
		`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ?, lease_expires_at = ?
         WHERE id = ? AND status IN ('queued', 'running') AND lease_expires_at > ?`,
		status, phase, progressJSON, now.Add(assetImportLeaseDuration).Unix(), jobID, now.Unix()))
}

func (r *AssetRepository) UpdateImportJobProgress(jobID, phase, progressJSON string) error {
	now := time.Now().UTC()
	return assetImportWriteResult(r.db.ExecWrite(
		`UPDATE asset_import_jobs SET phase = ?, progress_json = ?, lease_expires_at = ?
         WHERE id = ? AND status = 'running' AND lease_expires_at > ?`,
		phase, progressJSON, now.Add(assetImportLeaseDuration).Unix(), jobID, now.Unix()))
}

type ImportTypeFieldInput struct {
	Name         string
	FieldType    string
	OptionsJSON  *string
	IsRequired   bool
	DisplayOrder int
}

type ImportTypeFieldResult struct {
	AssetTypeFieldID int
	CustomFieldID    int
}

// CreateAssetTypeWithFields creates a type and its custom fields atomically.
func (r *AssetRepository) CreateAssetTypeWithFields(setID int, typeCore models.AssetType, fields []ImportTypeFieldInput) (typeID int, createdAt time.Time, results []ImportTypeFieldResult, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, time.Time{}, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()

	var typeID64 int64
	if err = tx.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, true, ?, ?) RETURNING id
	`, setID, typeCore.Name, typeCore.Description, typeCore.Icon, typeCore.Color, now, now).Scan(&typeID64); err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, time.Time{}, nil, ErrDuplicateEntry
		}
		return 0, time.Time{}, nil, fmt.Errorf("failed to create asset type: %w", err)
	}
	typeID = int(typeID64)

	results = make([]ImportTypeFieldResult, 0, len(fields))
	for _, f := range fields {
		f.FieldType = models.CanonicalCustomFieldType(f.FieldType)
		var cfID int
		if models.IsBooleanCustomFieldType(f.FieldType) {
			err = tx.QueryRow(`
				SELECT id FROM custom_field_definitions
				WHERE LOWER(name) = LOWER(?) AND field_type IN (?, ?)
				ORDER BY CASE WHEN field_type = ? THEN 0 ELSE 1 END, id
				LIMIT 1
			`, f.Name, models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox, models.CustomFieldTypeBoolean).Scan(&cfID)
		} else {
			err = tx.QueryRow(`
				SELECT id FROM custom_field_definitions
				WHERE LOWER(name) = LOWER(?) AND field_type = ?
			`, f.Name, f.FieldType).Scan(&cfID)
		}
		if errors.Is(err, sql.ErrNoRows) {
			if err = tx.QueryRow(`
				INSERT INTO custom_field_definitions (name, field_type, options, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?) RETURNING id
			`, f.Name, f.FieldType, f.OptionsJSON, now, now).Scan(&cfID); err != nil {
				return 0, time.Time{}, nil, fmt.Errorf("failed to create custom field definition: %w", err)
			}
		} else if err != nil {
			return 0, time.Time{}, nil, fmt.Errorf("failed to look up custom field: %w", err)
		}

		var atfID int64
		if err = tx.QueryRow(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, typeID, cfID, f.IsRequired, f.DisplayOrder, now).Scan(&atfID); err != nil {
			return 0, time.Time{}, nil, fmt.Errorf("failed to link field to type: %w", err)
		}

		results = append(results, ImportTypeFieldResult{
			AssetTypeFieldID: int(atfID),
			CustomFieldID:    cfID,
		})
	}

	if err = tx.Commit(); err != nil {
		return 0, time.Time{}, nil, fmt.Errorf("failed to commit asset type creation: %w", err)
	}
	return typeID, now, results, nil
}

// Asset custom-field resolution.

func (r *AssetRepository) FindCustomFieldIDsByType(assetTypeID int, fieldType string) (map[int]bool, error) {
	rows, err := r.db.Query(`
		SELECT cfd.id
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ? AND cfd.field_type = ?
	`, assetTypeID, fieldType)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom field ids by type: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fieldIDs := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan custom field id: %w", err)
		}
		fieldIDs[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom field ids: %w", err)
	}
	return fieldIDs, nil
}

type AssetSummary struct {
	Title    string
	AssetTag string
}

// GetAssetSummary rejects cross-set lookups so asset references cannot leak data.
func (r *AssetRepository) GetAssetSummary(assetID, setID int) (*AssetSummary, error) {
	var title, assetTag sql.NullString
	err := r.db.QueryRow(`SELECT title, asset_tag FROM assets WHERE id = ? AND set_id = ?`, assetID, setID).Scan(&title, &assetTag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get asset summary: %w", err)
	}
	return &AssetSummary{Title: title.String, AssetTag: assetTag.String}, nil
}

type UserBasicInfo struct {
	FirstName sql.NullString
	LastName  sql.NullString
	Email     sql.NullString
	AvatarURL sql.NullString
}

func (r *AssetRepository) GetUserBasicInfo(userID int) (*UserBasicInfo, error) {
	var info UserBasicInfo
	err := r.db.QueryRow(`
		SELECT first_name, last_name, email, avatar_url
		FROM users WHERE id = ?
	`, userID).Scan(&info.FirstName, &info.LastName, &info.Email, &info.AvatarURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user basic info: %w", err)
	}
	return &info, nil
}

// Asset-status operations.

func (r *AssetRepository) FindAssetStatusesForSet(setID int) ([]models.AssetStatus, error) {
	rows, err := r.db.Query(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses
		WHERE set_id = ?
		ORDER BY display_order, name
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	statuses := make([]models.AssetStatus, 0)
	for rows.Next() {
		status, err := scanAssetStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset statuses: %w", err)
	}
	return statuses, nil
}

func (r *AssetRepository) FindAssetStatusByID(statusID int) (*models.AssetStatus, error) {
	row := r.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses WHERE id = ?
	`, statusID)
	status, err := scanAssetStatus(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *AssetRepository) GetAssetStatusSetID(statusID int) (int, error) {
	var setID int
	err := r.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get asset status set: %w", err)
	}
	return setID, nil
}

type AssetStatusUpdate struct {
	Name         string
	Color        string
	Description  string
	DisplayOrder int
	IsDefault    *bool
}

func (r *AssetRepository) DeleteAssetStatus(statusID int) error {
	result, err := r.db.ExecWrite("DELETE FROM asset_statuses WHERE id = ?", statusID)
	if err != nil {
		return fmt.Errorf("failed to delete asset status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateAssetStatusTransactional inserts a status and updates the set default atomically.
func (r *AssetRepository) CreateAssetStatusTransactional(s *models.AssetStatus) (int, error) {
	var id int64
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if s.IsDefault {
			if _, err := tx.ExecWrite("UPDATE asset_statuses SET is_default = false WHERE set_id = ?", s.SetID); err != nil {
				return fmt.Errorf("failed to clear default statuses: %w", err)
			}
		}
		if err := tx.QueryRow(`
			INSERT INTO asset_statuses (set_id, name, color, description, is_default, display_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
		`, s.SetID, s.Name, s.Color, s.Description, s.IsDefault, s.DisplayOrder, s.CreatedAt, s.UpdatedAt).Scan(&id); err != nil {
			return fmt.Errorf("failed to create asset status: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// UpdateAssetStatusTransactional applies a status patch and updates the default atomically.
func (r *AssetRepository) UpdateAssetStatusTransactional(statusID int, patch AssetStatusUpdate, setID int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if patch.IsDefault != nil && *patch.IsDefault {
			if _, err := tx.ExecWrite(
				"UPDATE asset_statuses SET is_default = false WHERE set_id = ? AND id != ?",
				setID, statusID,
			); err != nil {
				return fmt.Errorf("failed to clear default statuses: %w", err)
			}
		}
		query := "UPDATE asset_statuses SET name = ?, color = ?, description = ?, display_order = ?, updated_at = ?"
		args := []any{patch.Name, patch.Color, patch.Description, patch.DisplayOrder, time.Now()}
		if patch.IsDefault != nil {
			query += ", is_default = ?"
			args = append(args, *patch.IsDefault)
		}
		query += " WHERE id = ?"
		args = append(args, statusID)
		result, err := tx.ExecWrite(query, args...)
		if err != nil {
			return fmt.Errorf("failed to update asset status: %w", err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *AssetRepository) CountAssetsUsingStatus(statusID int) (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM assets WHERE status_id = ?", statusID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count assets using status: %w", err)
	}
	return count, nil
}

func scanAssetStatus(scanner interface{ Scan(...any) error }) (models.AssetStatus, error) {
	var status models.AssetStatus
	var description sql.NullString
	if err := scanner.Scan(
		&status.ID, &status.SetID, &status.Name, &status.Color, &description,
		&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
	); err != nil {
		return status, err
	}
	if description.Valid {
		status.Description = description.String
	}
	return status, nil
}

// CQL lookup maps.

func (r *AssetRepository) GetCQLSetMap() (map[string]int, error) {
	rows, err := r.db.Query("SELECT id, name FROM asset_management_sets")
	if err != nil {
		return nil, fmt.Errorf("failed to query asset sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	setMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("failed to scan asset set: %w", err)
		}
		setMap[strings.ToLower(name)] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset sets: %w", err)
	}
	return setMap, nil
}

func (r *AssetRepository) GetCQLCustomFieldMap(setID int) (cql.CustomFieldMap, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT cfd.id, LOWER(cfd.name), cfd.field_type, COALESCE(cfd.options, '')
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		JOIN asset_types at2 ON atf.asset_type_id = at2.id
		WHERE at2.set_id = ?
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cfMap := make(cql.CustomFieldMap)
	for rows.Next() {
		var id int
		var name, fieldType, options string
		if err := rows.Scan(&id, &name, &fieldType, &options); err != nil {
			return nil, fmt.Errorf("failed to scan custom field: %w", err)
		}
		info := cql.CustomFieldInfo{
			ID:        id,
			Kind:      cql.ClassifyCustomFieldKind(fieldType),
			FieldType: strings.ToLower(fieldType),
		}
		if info.Kind == cql.CFKindLinking {
			info.MirrorOfFieldID, info.AllowedTargetTypes = cql.LinkingFieldOptions(options)
		}
		cfMap[name] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom fields: %w", err)
	}
	return cfMap, nil
}

func (r *AssetRepository) GetEveryoneRoleDetailed(setID int) (*models.AssetSetEveryoneRole, error) {
	var role models.AssetSetEveryoneRole
	var roleID, grantedBy sql.NullInt64
	var roleName, grantedByName sql.NullString

	err := r.db.QueryRow(`
		SELECT aser.set_id, aser.role_id, aser.granted_by, aser.granted_at,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM asset_set_everyone_roles aser
		LEFT JOIN asset_roles ar ON aser.role_id = ar.id
		LEFT JOIN users u ON aser.granted_by = u.id
		WHERE aser.set_id = ?
	`, setID).Scan(&role.SetID, &roleID, &grantedBy, &role.GrantedAt, &roleName, &grantedByName)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query everyone role: %w", err)
	}

	if roleID.Valid {
		v := int(roleID.Int64)
		role.RoleID = &v
	}
	if grantedBy.Valid {
		v := int(grantedBy.Int64)
		role.GrantedBy = &v
	}
	role.RoleName = roleName.String
	role.GrantedByName = grantedByName.String
	return &role, nil
}

// Link operations.

func (r *AssetRepository) DeleteAssetLinks(assetID int) error {
	_, err := r.db.ExecWrite(`
		DELETE FROM item_links
		WHERE (source_type = 'asset' AND source_id = ?)
		   OR (target_type = 'asset' AND target_id = ?)
	`, assetID, assetID)
	if err != nil {
		return fmt.Errorf("failed to delete asset links: %w", err)
	}
	return nil
}
