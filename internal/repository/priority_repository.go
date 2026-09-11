package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type PriorityRepository struct {
	db database.Database
}

func (r *PriorityRepository) ConfigurationSetExists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

func (r *PriorityRepository) NameExists(name string, excludeID int) (bool, error) {
	var exists bool
	var err error
	if excludeID == 0 {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE name = ?)", name).Scan(&exists)
	} else {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE name = ? AND id != ?)", name, excludeID).Scan(&exists)
	}
	return exists, err
}

func (r *PriorityRepository) Create(priority *models.Priority) (int, error) {
	now := time.Now()
	var id int
	err := r.db.QueryRow(`
		INSERT INTO priorities (name, description, is_default, icon, color, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, priority.Name, priority.Description, priority.IsDefault, priority.Icon, priority.Color, priority.SortOrder, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create priority: %w", err)
	}
	if err := r.replaceConfigurationSets(id, priority.ConfigurationSetIDs, now); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *PriorityRepository) Update(id int, priority *models.Priority) error {
	now := time.Now()
	if _, err := r.db.ExecWrite(`
		UPDATE priorities SET name = ?, description = ?, is_default = ?, icon = ?, color = ?, sort_order = ?, updated_at = ? WHERE id = ?
	`, priority.Name, priority.Description, priority.IsDefault, priority.Icon, priority.Color, priority.SortOrder, now, id); err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update priority %d: %w", id, err)
	}
	return r.replaceConfigurationSets(id, priority.ConfigurationSetIDs, now)
}

func (r *PriorityRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM priorities WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete priority %d: %w", id, err)
	}
	return nil
}

func (r *PriorityRepository) ClearOtherDefaults(excludeID int) error {
	if excludeID == 0 {
		_, err := r.db.ExecWrite("UPDATE priorities SET is_default = false WHERE is_default = true")
		return err
	}
	_, err := r.db.ExecWrite("UPDATE priorities SET is_default = false WHERE is_default = true AND id != ?", excludeID)
	return err
}

func (r *PriorityRepository) replaceConfigurationSets(priorityID int, ids []int, now time.Time) error {
	if _, err := r.db.ExecWrite("DELETE FROM configuration_set_priorities WHERE priority_id = ?", priorityID); err != nil {
		return fmt.Errorf("clear configuration sets for priority %d: %w", priorityID, err)
	}
	for _, id := range ids {
		if _, err := r.db.ExecWrite(`INSERT INTO configuration_set_priorities (configuration_set_id, priority_id, created_at) VALUES (?, ?, ?)`, id, priorityID, now); err != nil {
			return fmt.Errorf("associate priority %d with configuration set %d: %w", priorityID, id, err)
		}
	}
	return nil
}

func NewPriorityRepository(db database.Database) *PriorityRepository {
	return &PriorityRepository{db: db}
}

const priorityColumns = `p.id, COALESCE(p.builtin_key, ''), p.name,
	COALESCE(p.description, ''), p.is_default, COALESCE(p.icon, ''),
	COALESCE(p.color, ''), p.sort_order, p.created_at, p.updated_at`

// List returns the global priority catalog with configuration-set associations.
func (r *PriorityRepository) List(configurationSetID *int) ([]models.Priority, error) {
	query := "SELECT " + priorityColumns + " FROM priorities p"
	args := []any{}
	if configurationSetID != nil {
		query += " INNER JOIN configuration_set_priorities csp ON p.id = csp.priority_id WHERE csp.configuration_set_id = ?"
		args = append(args, *configurationSetID)
	}
	query += " ORDER BY p.sort_order, p.name"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list priorities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	priorities := []models.Priority{}
	for rows.Next() {
		var priority models.Priority
		if err := scanPriority(rows, &priority); err != nil {
			return nil, fmt.Errorf("scan priority: %w", err)
		}
		if err := r.populateConfigurationSets(&priority); err != nil {
			return nil, err
		}
		priorities = append(priorities, priority)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate priorities: %w", err)
	}
	return priorities, nil
}

// GetByID returns a priority with its configuration-set associations.
func (r *PriorityRepository) GetByID(id int) (*models.Priority, error) {
	var priority models.Priority
	err := scanPriority(
		r.db.QueryRow("SELECT "+priorityColumns+" FROM priorities p WHERE p.id = ?", id),
		&priority,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get priority %d: %w", id, err)
	}
	if err := r.populateConfigurationSets(&priority); err != nil {
		return nil, err
	}
	return &priority, nil
}

func scanPriority(row interface{ Scan(...any) error }, priority *models.Priority) error {
	return row.Scan(&priority.ID, &priority.BuiltinKey, &priority.Name, &priority.Description,
		&priority.IsDefault, &priority.Icon, &priority.Color, &priority.SortOrder,
		&priority.CreatedAt, &priority.UpdatedAt)
}

func (r *PriorityRepository) populateConfigurationSets(priority *models.Priority) error {
	rows, err := r.db.Query(`
		SELECT cs.id, cs.name, COALESCE(cs.builtin_key, '')
		FROM configuration_set_priorities csp
		JOIN configuration_sets cs ON csp.configuration_set_id = cs.id
		WHERE csp.priority_id = ?
		ORDER BY cs.name
	`, priority.ID)
	if err != nil {
		return fmt.Errorf("load configuration sets for priority %d: %w", priority.ID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name, builtinKey string
		if err := rows.Scan(&id, &name, &builtinKey); err != nil {
			return fmt.Errorf("scan priority configuration set: %w", err)
		}
		priority.ConfigurationSetIDs = append(priority.ConfigurationSetIDs, id)
		priority.ConfigurationSetNames = append(priority.ConfigurationSetNames, name)
		priority.ConfigurationSetBuiltinKeys = append(priority.ConfigurationSetBuiltinKeys, builtinKey)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate priority configuration sets: %w", err)
	}
	return nil
}

// ListForWorkspace returns priorities enabled through the workspace's
// configuration set, falling back to the global catalog when none are mapped.
func (r *PriorityRepository) ListForWorkspace(workspaceID int) ([]models.Priority, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT p.id, COALESCE(p.builtin_key, ''), p.name, COALESCE(p.description, ''), COALESCE(p.icon, ''), COALESCE(p.color, ''),
		       p.sort_order, p.is_default
		FROM priorities p
		WHERE NOT EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_priorities csp ON wcs.configuration_set_id = csp.configuration_set_id
			WHERE wcs.workspace_id = ?
		)
		OR EXISTS (
			SELECT 1 FROM workspace_configuration_sets wcs
			JOIN configuration_set_priorities csp ON wcs.configuration_set_id = csp.configuration_set_id
			WHERE wcs.workspace_id = ? AND csp.priority_id = p.id
		)
		ORDER BY p.sort_order, p.name
	`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list priorities for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.Priority, 0)
	for rows.Next() {
		var priority models.Priority
		if err := rows.Scan(&priority.ID, &priority.BuiltinKey, &priority.Name, &priority.Description,
			&priority.Icon, &priority.Color, &priority.SortOrder, &priority.IsDefault); err != nil {
			return nil, fmt.Errorf("scan workspace priority: %w", err)
		}
		out = append(out, priority)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace priorities: %w", err)
	}
	return out, nil
}
