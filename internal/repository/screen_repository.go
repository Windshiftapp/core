package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"windshift/internal/database"
)

// ScreenRepository serves the small "available fields for this create
// screen" lookup used by both the request-types and the asset-reports
// admin endpoints. Both used to carry their own copy of these methods;
// they're consolidated here.
type ScreenRepository struct {
	db database.Database
}

// NewScreenRepository creates a ScreenRepository.
func NewScreenRepository(db database.Database) *ScreenRepository {
	return &ScreenRepository{db: db}
}

// ScreenFieldRow is the slim shape of a screen_fields row used by the
// "available fields" lookup. Handlers map this into their wider
// AvailableField response shape.
type ScreenFieldRow struct {
	FieldType       string // "default" or "custom"
	FieldIdentifier string
	FieldName       string // populated when FieldType == "custom" (joined custom_field_definitions.name)
	CustomFieldType string // populated when FieldType == "custom"
	// CustomFieldOptions is the joined custom_field_definitions.options JSON.
	// Callers that need arity (for example to keep multi-asset fields out of
	// report forms) read it without a second lookup.
	CustomFieldOptions string
}

// GetCreateScreenID resolves a (workspace, item_type) pair to a configured
// create_screen_id via workspace_configuration_sets → configuration_set_item_types.
// Returns nil + nil when no mapping exists (callers treat that as "no override").
func (r *ScreenRepository) GetCreateScreenID(workspaceID, itemTypeID int) (*int, error) {
	var screenID *int
	err := r.db.QueryRow(`
		SELECT csit.create_screen_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_set_item_types csit
		  ON csit.configuration_set_id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? AND csit.item_type_id = ?
		LIMIT 1
	`, workspaceID, itemTypeID).Scan(&screenID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // null screen mapping is a real "no override" signal, distinct from an error
	}
	if err != nil {
		return nil, fmt.Errorf("get create_screen_id for workspace %d / item_type %d: %w", workspaceID, itemTypeID, err)
	}
	return screenID, nil
}

// GetEffectiveCreateScreenID resolves the item-type override and then the
// configuration set's default create screen.
func (r *ScreenRepository) GetEffectiveCreateScreenID(workspaceID, itemTypeID int) (*int, error) {
	var screenID *int
	err := r.db.QueryRow(`
		SELECT COALESCE(csit.create_screen_id, css.screen_id)
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON cs.id = wcs.configuration_set_id
		JOIN configuration_set_item_types csit
		  ON csit.configuration_set_id = cs.id AND csit.item_type_id = ?
		LEFT JOIN configuration_set_screens css
		  ON css.configuration_set_id = cs.id AND css.context = 'create'
		WHERE wcs.workspace_id = ?
	`, itemTypeID, workspaceID).Scan(&screenID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve effective create screen: %w", err)
	}
	return screenID, nil
}

// ListFields returns the screen_fields rows for a screen, joined with
// custom_field_definitions for the "custom" entries.
func (r *ScreenRepository) ListFields(screenID int) ([]ScreenFieldRow, error) {
	rows, err := r.db.Query(`
		SELECT sf.field_type, sf.field_identifier,
		       CASE WHEN sf.field_type = 'custom' THEN cfd.name ELSE '' END as field_name,
		       CASE WHEN sf.field_type = 'custom' THEN cfd.field_type ELSE '' END as custom_field_type,
		       CASE WHEN sf.field_type = 'custom' THEN COALESCE(cfd.options, '') ELSE '' END as custom_field_options
		FROM screen_fields sf
		LEFT JOIN custom_field_definitions cfd ON sf.field_type = 'custom' AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE sf.screen_id = ?
		  AND (sf.field_type != 'custom' OR cfd.id IS NOT NULL)
		ORDER BY sf.display_order, sf.id
	`, screenID)
	if err != nil {
		return nil, fmt.Errorf("list screen_fields for screen %d: %w", screenID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []ScreenFieldRow
	for rows.Next() {
		var sfr ScreenFieldRow
		if err := rows.Scan(&sfr.FieldType, &sfr.FieldIdentifier, &sfr.FieldName, &sfr.CustomFieldType, &sfr.CustomFieldOptions); err != nil {
			return nil, fmt.Errorf("scan screen_field: %w", err)
		}
		out = append(out, sfr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate screen_fields: %w", err)
	}
	return out, nil
}

// BindImportedFields clones the configured screens and appends imported custom
// fields without mutating shared source screens.
func (r *ScreenRepository) BindImportedFields(workspaceID int, projectKey string, fieldIDs []int) error {
	var configSetID int
	if err := r.db.QueryRow(`
		SELECT configuration_set_id FROM workspace_configuration_sets WHERE workspace_id = ?
	`, workspaceID).Scan(&configSetID); err != nil {
		return fmt.Errorf("resolve workspace configuration set: %w", err)
	}
	defaultScreens, err := r.configurationScreens(configSetID)
	if err != nil {
		return err
	}
	for _, contextName := range []string{"create", "edit", "view"} {
		sourceID := defaultScreens[contextName]
		if sourceID == 0 {
			sourceID = defaultScreens["create"]
		}
		if sourceID == 0 {
			sourceID = 1
		}
		name := fmt.Sprintf(
			"%s Jira Import %s Screen (%d)",
			projectKey,
			strings.ToUpper(contextName[:1])+contextName[1:],
			workspaceID,
		)
		screenID, err := r.ensureImportedFieldScreen(name, sourceID, fieldIDs)
		if err != nil {
			return err
		}
		if _, err := r.db.ExecWrite(`
			INSERT INTO configuration_set_screens
				(configuration_set_id, screen_id, context, created_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(configuration_set_id, context) DO UPDATE SET screen_id = excluded.screen_id
		`, configSetID, screenID, contextName); err != nil {
			return fmt.Errorf("assign %s Jira import screen: %w", contextName, err)
		}
	}
	return r.bindImportedItemTypeScreens(configSetID, workspaceID, projectKey, fieldIDs)
}

func (r *ScreenRepository) configurationScreens(configSetID int) (map[string]int, error) {
	rows, err := r.db.Query(`
		SELECT context, screen_id
		FROM configuration_set_screens
		WHERE configuration_set_id = ?
	`, configSetID)
	if err != nil {
		return nil, fmt.Errorf("load configuration screens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]int)
	for rows.Next() {
		var contextName string
		var screenID int
		if err := rows.Scan(&contextName, &screenID); err != nil {
			return nil, fmt.Errorf("scan configuration screen: %w", err)
		}
		result[contextName] = screenID
	}
	return result, rows.Err()
}

func (r *ScreenRepository) bindImportedItemTypeScreens(
	configSetID, workspaceID int,
	projectKey string,
	fieldIDs []int,
) error {
	type itemTypeScreens struct {
		ID                       int
		CreateID, EditID, ViewID sql.NullInt64
	}
	rows, err := r.db.Query(`
		SELECT id, create_screen_id, edit_screen_id, view_screen_id
		FROM configuration_set_item_types
		WHERE configuration_set_id = ?
	`, configSetID)
	if err != nil {
		return fmt.Errorf("load item type screen overrides: %w", err)
	}
	var items []itemTypeScreens
	for rows.Next() {
		var item itemTypeScreens
		if err := rows.Scan(&item.ID, &item.CreateID, &item.EditID, &item.ViewID); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, item := range items {
		for _, override := range []struct {
			column string
			source sql.NullInt64
		}{
			{column: "create_screen_id", source: item.CreateID},
			{column: "edit_screen_id", source: item.EditID},
			{column: "view_screen_id", source: item.ViewID},
		} {
			if !override.source.Valid || override.source.Int64 <= 0 {
				continue
			}
			name := fmt.Sprintf(
				"%s Jira Import Item Screen (%d-%d-%s)",
				projectKey,
				workspaceID,
				item.ID,
				override.column,
			)
			screenID, err := r.ensureImportedFieldScreen(
				name,
				int(override.source.Int64),
				fieldIDs,
			)
			if err != nil {
				return err
			}
			query := fmt.Sprintf(
				"UPDATE configuration_set_item_types SET %s = ? WHERE id = ?",
				override.column,
			) //nolint:gosec // column is from the fixed list above.
			if _, err := r.db.ExecWrite(query, screenID, item.ID); err != nil {
				return fmt.Errorf("assign item type Jira import screen: %w", err)
			}
		}
	}
	return nil
}

func (r *ScreenRepository) ensureImportedFieldScreen(
	name string,
	sourceScreenID int,
	fieldIDs []int,
) (int, error) {
	var screenID int
	err := r.db.QueryRow("SELECT id FROM screens WHERE name = ?", name).Scan(&screenID)
	if errors.Is(err, sql.ErrNoRows) {
		var newID int64
		if err := r.db.QueryRowWrite(`
			INSERT INTO screens (name, description, created_at, updated_at)
			VALUES (?, 'Imported Jira field layout', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, name).Scan(&newID); err != nil {
			return 0, fmt.Errorf("create Jira import screen: %w", err)
		}
		screenID = int(newID)
		if sourceScreenID > 0 {
			if _, err := r.db.ExecWrite(`
				INSERT INTO screen_fields
					(screen_id, field_type, field_identifier, display_order, is_required, field_width, placement, hint)
				SELECT ?, field_type, field_identifier, display_order, is_required, field_width, placement, hint
				FROM screen_fields WHERE screen_id = ?
			`, screenID, sourceScreenID); err != nil {
				return 0, fmt.Errorf("copy Jira import screen fields: %w", err)
			}
			if _, err := r.db.ExecWrite(`
				INSERT INTO screen_system_fields (screen_id, field_name)
				SELECT ?, field_name FROM screen_system_fields WHERE screen_id = ?
				ON CONFLICT(screen_id, field_name) DO NOTHING
			`, screenID, sourceScreenID); err != nil {
				return 0, fmt.Errorf("copy Jira import system fields: %w", err)
			}
		}
	} else if err != nil {
		return 0, fmt.Errorf("find Jira import screen: %w", err)
	}
	var displayOrder int
	if err := r.db.QueryRow(
		"SELECT COALESCE(MAX(display_order), 0) FROM screen_fields WHERE screen_id = ?",
		screenID,
	).Scan(&displayOrder); err != nil {
		return 0, fmt.Errorf("load Jira import screen display order: %w", err)
	}
	for _, fieldID := range fieldIDs {
		identifier := strconv.Itoa(fieldID)
		var exists int
		if err := r.db.QueryRow(`
			SELECT COUNT(*) FROM screen_fields
			WHERE screen_id = ? AND field_type = 'custom' AND field_identifier = ?
		`, screenID, identifier).Scan(&exists); err != nil {
			return 0, fmt.Errorf("check Jira import screen field: %w", err)
		}
		if exists > 0 {
			continue
		}
		displayOrder++
		if _, err := r.db.ExecWrite(`
			INSERT INTO screen_fields
				(screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, 'custom', ?, ?, false, 'full')
		`, screenID, identifier, displayOrder); err != nil {
			return 0, fmt.Errorf("add Jira Assets field to screen: %w", err)
		}
	}
	return screenID, nil
}
