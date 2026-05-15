package repository

import (
	"database/sql"
	"errors"
	"fmt"

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

// ListFields returns the screen_fields rows for a screen, joined with
// custom_field_definitions for the "custom" entries.
func (r *ScreenRepository) ListFields(screenID int) ([]ScreenFieldRow, error) {
	rows, err := r.db.Query(`
		SELECT sf.field_type, sf.field_identifier,
		       CASE WHEN sf.field_type = 'custom' THEN cfd.name ELSE '' END as field_name,
		       CASE WHEN sf.field_type = 'custom' THEN cfd.field_type ELSE '' END as custom_field_type
		FROM screen_fields sf
		LEFT JOIN custom_field_definitions cfd ON sf.field_type = 'custom' AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE sf.screen_id = ?
		ORDER BY sf.display_order, sf.id
	`, screenID)
	if err != nil {
		return nil, fmt.Errorf("list screen_fields for screen %d: %w", screenID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []ScreenFieldRow
	for rows.Next() {
		var sfr ScreenFieldRow
		if err := rows.Scan(&sfr.FieldType, &sfr.FieldIdentifier, &sfr.FieldName, &sfr.CustomFieldType); err != nil {
			return nil, fmt.Errorf("scan screen_field: %w", err)
		}
		out = append(out, sfr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate screen_fields: %w", err)
	}
	return out, nil
}
