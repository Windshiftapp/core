package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ItemTypeRepository owns data access for the item_types table and its
// associations to configuration sets (the configuration_set_item_types
// junction). It backs the item-type admin handler plus the small validation
// lookups several other domains (templates, request types) need.
type ItemTypeRepository struct {
	db database.Database
}

// NewItemTypeRepository constructs an ItemTypeRepository.
func NewItemTypeRepository(db database.Database) *ItemTypeRepository {
	return &ItemTypeRepository{db: db}
}

const (
	itemTypeColumns   = "id, COALESCE(builtin_key, ''), name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at"
	itemTypeColumnsIT = "it.id, COALESCE(it.builtin_key, ''), it.name, it.description, it.is_default, it.icon, it.color, it.hierarchy_level, it.sort_order, it.created_at, it.updated_at"
)

// scanItemType reads a full item_types row (in itemTypeColumns order) into it.
func scanItemType(row interface{ Scan(...any) error }, it *models.ItemType) error {
	return row.Scan(&it.ID, &it.BuiltinKey, &it.Name, &it.Description, &it.IsDefault,
		&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder, &it.CreatedAt, &it.UpdatedAt)
}

// List returns all item types ordered by hierarchy/sort/name, each with its
// configuration-set associations populated. A non-nil configurationSetID
// filters to types belonging to that set via the junction table.
func (r *ItemTypeRepository) List(configurationSetID *int) ([]models.ItemType, error) {
	query := "SELECT " + itemTypeColumnsIT + " FROM item_types it"
	args := []any{}
	if configurationSetID != nil {
		query += " INNER JOIN configuration_set_item_types csit ON it.id = csit.item_type_id" +
			" WHERE csit.configuration_set_id = ?"
		args = append(args, *configurationSetID)
	}
	query += " ORDER BY CASE WHEN it.hierarchy_level = -1 THEN 1 ELSE 0 END, it.hierarchy_level, it.sort_order, it.name"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list item types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	itemTypes := []models.ItemType{}
	for rows.Next() {
		var it models.ItemType
		if err := scanItemType(rows, &it); err != nil {
			return nil, fmt.Errorf("scan item type: %w", err)
		}
		if err := r.populateConfigurationSets(&it); err != nil {
			return nil, err
		}
		itemTypes = append(itemTypes, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list item types: %w", err)
	}
	return itemTypes, nil
}

// ListForWorkspace returns the workspace-applicable item type catalog.
func (r *ItemTypeRepository) ListForWorkspace(workspaceID int) ([]models.ItemType, error) {
	rows, err := r.db.Query(`
		SELECT it.id, COALESCE(it.builtin_key, ''), it.name, COALESCE(it.description, ''), COALESCE(it.icon, ''), COALESCE(it.color, ''),
		       it.hierarchy_level, it.sort_order, it.is_default
		FROM item_types it
		WHERE NOT EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_item_types csit ON wcs.configuration_set_id = csit.configuration_set_id
			WHERE wcs.workspace_id = ?
		)
		OR EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_item_types csit ON wcs.configuration_set_id = csit.configuration_set_id
			WHERE wcs.workspace_id = ? AND csit.item_type_id = it.id
		)
		ORDER BY CASE WHEN it.hierarchy_level = -1 THEN 1 ELSE 0 END,
		         it.hierarchy_level, it.sort_order, it.name
	`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item types for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.ItemType, 0)
	for rows.Next() {
		var itemType models.ItemType
		if err := rows.Scan(&itemType.ID, &itemType.BuiltinKey, &itemType.Name, &itemType.Description,
			&itemType.Icon, &itemType.Color, &itemType.HierarchyLevel, &itemType.SortOrder,
			&itemType.IsDefault); err != nil {
			return nil, fmt.Errorf("scan workspace item type: %w", err)
		}
		out = append(out, itemType)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace item types: %w", err)
	}
	return out, nil
}

// ListChildNames returns item types allowed below a parent type in a workspace.
func (r *ItemTypeRepository) ListChildNames(parentTypeID, workspaceID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT it.name
		FROM item_types it
		JOIN workspace_hierarchy wh ON wh.child_type_id = it.id
		WHERE wh.parent_type_id = ? AND it.workspace_id = ?
	`, parentTypeID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list child item types: %w", err)
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan child item type: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate child item types: %w", err)
	}
	return names, nil
}

// GetByID returns the item type with the given id (configuration sets
// populated), or ErrNotFound when it does not exist.
func (r *ItemTypeRepository) GetByID(id int) (*models.ItemType, error) {
	var it models.ItemType
	err := scanItemType(
		r.db.QueryRow("SELECT "+itemTypeColumns+" FROM item_types WHERE id = ?", id),
		&it,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item type %d: %w", id, err)
	}
	if err := r.populateConfigurationSets(&it); err != nil {
		return nil, err
	}
	return &it, nil
}

// FindByName returns the item type with the exact name, or ErrNotFound.
func (r *ItemTypeRepository) FindByName(name string) (*models.ItemType, error) {
	var itemType models.ItemType
	err := scanItemType(
		r.db.QueryRow("SELECT "+itemTypeColumns+" FROM item_types WHERE name = ?", name),
		&itemType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find item type %q: %w", name, err)
	}
	if err := r.populateConfigurationSets(&itemType); err != nil {
		return nil, err
	}
	return &itemType, nil
}

// Exists reports whether an item_type row with the given id exists. Used as an
// FK-style validator before writes that reference an item type.
func (r *ItemTypeRepository) Exists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check item_type %d exists: %w", id, err)
	}
	return ok, nil
}

// NameExists reports whether an item type with the given name already exists.
// excludeID > 0 excludes that row, so an update does not collide with itself.
func (r *ItemTypeRepository) NameExists(name string, excludeID int) (bool, error) {
	var ok bool
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ? AND id != ?)",
			name, excludeID,
		).Scan(&ok)
	} else {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ?)",
			name,
		).Scan(&ok)
	}
	if err != nil {
		return false, fmt.Errorf("check item_type name %q exists: %w", name, err)
	}
	return ok, nil
}

// ConfigurationSetExists reports whether a configuration_sets row with the
// given id exists. The item-type admin validates supplied set ids before
// associating them.
func (r *ItemTypeRepository) ConfigurationSetExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check configuration_set %d exists: %w", id, err)
	}
	return ok, nil
}

// Create inserts a new item type and associates it with the given configuration
// sets, returning the new id. Returns ErrDuplicateEntry on a unique-name
// collision.
func (r *ItemTypeRepository) Create(it *models.ItemType, configSetIDs []int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO item_types (name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("insert item type: %w", err)
	}
	if err := r.insertConfigurationSets(int(id), configSetIDs, now); err != nil {
		return 0, err
	}
	return int(id), nil
}

// Update mutates the item type's columns. When configSetIDs is non-empty the
// configuration-set associations are replaced wholesale; an empty slice leaves
// them untouched. Returns ErrDuplicateEntry on a unique-name collision.
func (r *ItemTypeRepository) Update(id int, it *models.ItemType, configSetIDs []int) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE item_types
		SET name = ?, description = ?, is_default = ?, icon = ?, color = ?, hierarchy_level = ?, sort_order = ?, updated_at = ?
		WHERE id = ?
	`, it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update item type %d: %w", id, err)
	}
	if len(configSetIDs) > 0 {
		if _, err := r.db.ExecWrite("DELETE FROM configuration_set_item_types WHERE item_type_id = ?", id); err != nil {
			return fmt.Errorf("clear configuration sets for item type %d: %w", id, err)
		}
		if err := r.insertConfigurationSets(id, configSetIDs, now); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the item type row. Callers are responsible for any in-use
// guard (the items.item_type_id FK is ON DELETE SET NULL).
func (r *ItemTypeRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM item_types WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete item type %d: %w", id, err)
	}
	return nil
}

// insertConfigurationSets links the item type to each configuration set.
func (r *ItemTypeRepository) insertConfigurationSets(itemTypeID int, ids []int, now time.Time) error {
	for _, csID := range ids {
		if _, err := r.db.ExecWrite(`
			INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id, created_at)
			VALUES (?, ?, ?)
		`, csID, itemTypeID, now); err != nil {
			return fmt.Errorf("associate item type %d with configuration set %d: %w", itemTypeID, csID, err)
		}
	}
	return nil
}

// loadConfigurationSets returns the configuration-set ids, names, and stable
// built-in keys linked to the item type, ordered by name.
func (r *ItemTypeRepository) loadConfigurationSets(itemTypeID int) (ids []int, names, builtinKeys []string, err error) {
	rows, err := r.db.Query(`
		SELECT cs.id, cs.name, COALESCE(cs.builtin_key, '')
		FROM configuration_set_item_types csit
		JOIN configuration_sets cs ON csit.configuration_set_id = cs.id
		WHERE csit.item_type_id = ?
		ORDER BY cs.name
	`, itemTypeID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load configuration sets for item type %d: %w", itemTypeID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int
		var name, builtinKey string
		if err := rows.Scan(&id, &name, &builtinKey); err != nil {
			return nil, nil, nil, fmt.Errorf("scan configuration set: %w", err)
		}
		ids = append(ids, id)
		names = append(names, name)
		builtinKeys = append(builtinKeys, builtinKey)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("load configuration sets for item type %d: %w", itemTypeID, err)
	}
	return ids, names, builtinKeys, nil
}

// populateConfigurationSets fills it.ConfigurationSet* from the junction table,
// keeping the deprecated single-set fields in sync for backward compatibility.
func (r *ItemTypeRepository) populateConfigurationSets(it *models.ItemType) error {
	ids, names, builtinKeys, err := r.loadConfigurationSets(it.ID)
	if err != nil {
		return err
	}
	it.ConfigurationSetIDs = ids
	it.ConfigurationSetNames = names
	it.ConfigurationSetBuiltinKeys = builtinKeys
	if len(ids) > 0 {
		it.ConfigurationSetID = ids[0]
		it.ConfigurationSetName = names[0]
	}
	return nil
}
