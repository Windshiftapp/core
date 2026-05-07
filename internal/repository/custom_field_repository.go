package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CustomFieldRepository provides data access for custom_field_definitions,
// custom_field_indexes, asset_type_fields, and the per-row cleanup queries
// against items/assets/custom_field_values used by the custom field handler.
type CustomFieldRepository struct {
	db database.Database
}

// NewCustomFieldRepository creates a new CustomFieldRepository.
func NewCustomFieldRepository(db database.Database) *CustomFieldRepository {
	return &CustomFieldRepository{db: db}
}

// --- custom_field_definitions ----------------------------------------------

const customFieldDefinitionSelect = `
	SELECT id, name, field_type, description, required, options, display_order, system_default,
	       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
	FROM custom_field_definitions`

// List returns all custom field definitions ordered by display_order, name.
func (r *CustomFieldRepository) List() ([]models.CustomFieldDefinition, error) {
	//nolint:misspell // database uses British spelling
	rows, err := r.db.Query(customFieldDefinitionSelect + ` ORDER BY display_order, name`)
	if err != nil {
		return nil, fmt.Errorf("list custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []models.CustomFieldDefinition{}
	for rows.Next() {
		cf, err := scanCustomFieldDefinition(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, cf)
	}
	return results, rows.Err()
}

// FindByID returns the custom field definition with the given id, or
// ErrNotFound.
func (r *CustomFieldRepository) FindByID(id int) (*models.CustomFieldDefinition, error) {
	//nolint:misspell // database uses British spelling
	row := r.db.QueryRow(customFieldDefinitionSelect+` WHERE id = ?`, id)
	cf, err := scanCustomFieldDefinitionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find custom field: %w", err)
	}
	return &cf, nil
}

// CustomFieldDeleteInfo is the subset of fields needed by Delete flows for
// audit logs and cascade decisions.
type CustomFieldDeleteInfo struct {
	Name          string
	FieldType     string
	SystemDefault bool
}

// FindDeleteInfo returns the fields needed for deletion-time checks.
// Returns ErrNotFound if the id doesn't exist.
func (r *CustomFieldRepository) FindDeleteInfo(id int) (*CustomFieldDeleteInfo, error) {
	var info CustomFieldDeleteInfo
	err := r.db.QueryRow(`
		SELECT name, field_type, system_default
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&info.Name, &info.FieldType, &info.SystemDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find custom field delete info: %w", err)
	}
	return &info, nil
}

// FindOptions returns the raw options JSON for a custom field, or empty
// string when the field has no options or doesn't exist.
func (r *CustomFieldRepository) FindOptions(id int) (string, error) {
	var options sql.NullString
	err := r.db.QueryRow(
		"SELECT options FROM custom_field_definitions WHERE id = ?",
		id,
	).Scan(&options)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find custom field options: %w", err)
	}
	if !options.Valid {
		return "", nil
	}
	return options.String, nil
}

// Create inserts a new custom field definition and returns its id.
func (r *CustomFieldRepository) Create(cf *models.CustomFieldDefinition, now time.Time) (int64, error) {
	var id int64
	//nolint:misspell // database uses British spelling (applies_to_customer_organisations)
	err := r.db.QueryRow(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
		                                       applies_to_portal_customers, applies_to_customer_organisations,
		                                       created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
		cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create custom field: %w", err)
	}
	return id, nil
}

// CreateMirror inserts a mirror linking field with fixed metadata and returns
// its id.
func (r *CustomFieldRepository) CreateMirror(name, optionsJSON string, now time.Time) (int64, error) {
	var id int64
	//nolint:misspell // database uses British spelling
	err := r.db.QueryRow(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
		                                       applies_to_portal_customers, applies_to_customer_organisations,
		                                       created_at, updated_at)
		VALUES (?, 'linking', '', 0, ?, 0, 0, 0, ?, ?) RETURNING id
	`, name, optionsJSON, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create mirror custom field: %w", err)
	}
	return id, nil
}

// Update overwrites the editable fields of a custom field definition.
func (r *CustomFieldRepository) Update(id int, cf *models.CustomFieldDefinition, now time.Time) error {
	//nolint:misspell // customer_organisations is a database table column
	_, err := r.db.ExecWrite(`
		UPDATE custom_field_definitions
		SET name = ?, field_type = ?, description = ?, required = ?, options = ?, display_order = ?,
		    applies_to_portal_customers = ?, applies_to_customer_organisations = ?, updated_at = ?
		WHERE id = ?
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
		cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, id)
	if err != nil {
		return fmt.Errorf("update custom field: %w", err)
	}
	return nil
}

// UpdateOptions replaces just the options JSON for a custom field.
func (r *CustomFieldRepository) UpdateOptions(id int64, optionsJSON string) error {
	_, err := r.db.ExecWrite(
		"UPDATE custom_field_definitions SET options = ? WHERE id = ?",
		optionsJSON, id,
	)
	if err != nil {
		return fmt.Errorf("update custom field options: %w", err)
	}
	return nil
}

// Delete removes the custom field definition row.
func (r *CustomFieldRepository) Delete(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM custom_field_definitions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete custom field: %w", err)
	}
	return nil
}

// --- asset_type_fields ------------------------------------------------------

// AssetTypeUsageRow is one row of the asset-type usage listing: which asset
// type (and which management set) references which custom_field_id.
type AssetTypeUsageRow struct {
	CustomFieldID int
	AssetTypeName string
	SetName       string
}

// ListAssetTypeUsages returns asset-type usages for every custom field,
// ordered by custom_field_id, set name, asset type name.
func (r *CustomFieldRepository) ListAssetTypeUsages() ([]AssetTypeUsageRow, error) {
	rows, err := r.db.Query(`
		SELECT atf.custom_field_id, at.name, s.name
		FROM asset_type_fields atf
		JOIN asset_types at ON atf.asset_type_id = at.id
		JOIN asset_management_sets s ON at.set_id = s.id
		ORDER BY atf.custom_field_id, s.name, at.name`)
	if err != nil {
		return nil, fmt.Errorf("list asset type usages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []AssetTypeUsageRow
	for rows.Next() {
		var row AssetTypeUsageRow
		if err := rows.Scan(&row.CustomFieldID, &row.AssetTypeName, &row.SetName); err != nil {
			return nil, fmt.Errorf("scan asset type usage: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// --- custom_field_indexes ---------------------------------------------------

// CustomFieldIndexRow is one row from custom_field_indexes.
type CustomFieldIndexRow struct {
	CustomFieldID int
	TargetTable   string
}

// ListIndexes returns every (custom_field_id, target_table) pair currently
// tracked in custom_field_indexes.
func (r *CustomFieldRepository) ListIndexes() ([]CustomFieldIndexRow, error) {
	rows, err := r.db.Query(`SELECT custom_field_id, target_table FROM custom_field_indexes`)
	if err != nil {
		return nil, fmt.Errorf("list custom field indexes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []CustomFieldIndexRow
	for rows.Next() {
		var row CustomFieldIndexRow
		if err := rows.Scan(&row.CustomFieldID, &row.TargetTable); err != nil {
			return nil, fmt.Errorf("scan custom field index: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// ListIndexNamesForField returns the index_name values tracked for a field.
func (r *CustomFieldRepository) ListIndexNamesForField(fieldID int) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT index_name FROM custom_field_indexes WHERE custom_field_id = ?`,
		fieldID,
	)
	if err != nil {
		return nil, fmt.Errorf("list index names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan index name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// IsIndexRecorded reports whether a custom_field_indexes row exists for the
// given (fieldID, targetTable).
func (r *CustomFieldRepository) IsIndexRecorded(fieldID int, targetTable string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`,
		fieldID, targetTable,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check index recorded: %w", err)
	}
	return count > 0, nil
}

// CountIndexesForTable returns the number of indexes currently tracked on a
// given target_table.
func (r *CustomFieldRepository) CountIndexesForTable(targetTable string) (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM custom_field_indexes WHERE target_table = ?`,
		targetTable,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count indexes for table: %w", err)
	}
	return count, nil
}

// CountIndexesPerTable returns the number of indexes tracked per target
// table, keyed by table name.
func (r *CustomFieldRepository) CountIndexesPerTable() (map[string]int, error) {
	rows, err := r.db.Query(
		`SELECT target_table, COUNT(*) FROM custom_field_indexes GROUP BY target_table`,
	)
	if err != nil {
		return nil, fmt.Errorf("count indexes per table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var table string
		var count int
		if err := rows.Scan(&table, &count); err != nil {
			return nil, fmt.Errorf("scan index per table: %w", err)
		}
		counts[table] = count
	}
	return counts, rows.Err()
}

// RecordIndex inserts a row in custom_field_indexes.
func (r *CustomFieldRepository) RecordIndex(fieldID int, targetTable, indexName string) error {
	_, err := r.db.ExecWrite(
		`INSERT INTO custom_field_indexes (custom_field_id, target_table, index_name) VALUES (?, ?, ?)`,
		fieldID, targetTable, indexName,
	)
	if err != nil {
		return fmt.Errorf("record index: %w", err)
	}
	return nil
}

// DeleteIndexRecord removes a custom_field_indexes row for the given
// (fieldID, targetTable).
func (r *CustomFieldRepository) DeleteIndexRecord(fieldID int, targetTable string) error {
	_, err := r.db.ExecWrite(
		`DELETE FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`,
		fieldID, targetTable,
	)
	if err != nil {
		return fmt.Errorf("delete index record: %w", err)
	}
	return nil
}

// --- raw index DDL ----------------------------------------------------------

// ExecDDL runs a DDL statement. The caller is responsible for constructing
// the SQL safely; this is exposed as an escape hatch for driver-specific
// CREATE INDEX / DROP INDEX statements.
func (r *CustomFieldRepository) ExecDDL(query string) error {
	_, err := r.db.ExecWrite(query)
	if err != nil {
		return fmt.Errorf("exec ddl: %w", err)
	}
	return nil
}

// DriverName returns the underlying database driver name so that callers can
// pick between SQLite and Postgres DDL syntax.
func (r *CustomFieldRepository) DriverName() string {
	return r.db.GetDriverName()
}

// --- items / assets custom_field_values cleanup ----------------------------

// TableCFVRow is one (id, custom_field_values) pair from items or assets.
type TableCFVRow struct {
	ID    int
	Value string
}

// ListRowsWithCustomFields returns every (id, custom_field_values) pair from
// the given table whose custom_field_values is non-empty and non-'{}'.
// tableName must be one of the trusted names "items" or "assets".
func (r *CustomFieldRepository) ListRowsWithCustomFields(tableName string) ([]TableCFVRow, error) {
	if tableName != "items" && tableName != "assets" {
		return nil, fmt.Errorf("invalid table for custom field values: %q", tableName)
	}
	rows, err := r.db.Query(fmt.Sprintf(
		`SELECT id, custom_field_values FROM %s WHERE custom_field_values IS NOT NULL AND custom_field_values != '' AND custom_field_values != '{}'`,
		tableName,
	))
	if err != nil {
		return nil, fmt.Errorf("list rows with cfvs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []TableCFVRow
	for rows.Next() {
		var row TableCFVRow
		if err := rows.Scan(&row.ID, &row.Value); err != nil {
			return nil, fmt.Errorf("scan cfv row: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// UpdateRowCustomFields overwrites custom_field_values for a single row in
// items or assets. tableName is validated as in ListRowsWithCustomFields.
func (r *CustomFieldRepository) UpdateRowCustomFields(tableName string, id int, newVal string) error {
	if tableName != "items" && tableName != "assets" {
		return fmt.Errorf("invalid table for custom field values: %q", tableName)
	}
	_, err := r.db.ExecWrite(
		fmt.Sprintf(`UPDATE %s SET custom_field_values = ? WHERE id = ?`, tableName),
		newVal, id,
	)
	if err != nil {
		return fmt.Errorf("update cfv row: %w", err)
	}
	return nil
}

// --- portal custom_field_values -------------------------------------------

// PortalCFVRow is one (id, value) pair from custom_field_values for a given
// custom_field_id.
type PortalCFVRow struct {
	ID    int
	Value string
}

// ListPortalCFVsForField returns non-empty portal custom_field_values for
// the given custom field.
func (r *CustomFieldRepository) ListPortalCFVsForField(fieldID int) ([]PortalCFVRow, error) {
	rows, err := r.db.Query(
		`SELECT id, value FROM custom_field_values WHERE custom_field_id = ? AND value IS NOT NULL AND value != ''`,
		fieldID,
	)
	if err != nil {
		return nil, fmt.Errorf("list portal cfvs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []PortalCFVRow
	for rows.Next() {
		var row PortalCFVRow
		if err := rows.Scan(&row.ID, &row.Value); err != nil {
			return nil, fmt.Errorf("scan portal cfv: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// DeletePortalCFV removes a single portal custom_field_values row.
func (r *CustomFieldRepository) DeletePortalCFV(id int) error {
	_, err := r.db.ExecWrite(`DELETE FROM custom_field_values WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete portal cfv: %w", err)
	}
	return nil
}

// UpdatePortalCFV overwrites a portal custom_field_values row's value.
func (r *CustomFieldRepository) UpdatePortalCFV(id int, newVal string) error {
	_, err := r.db.ExecWrite(`UPDATE custom_field_values SET value = ? WHERE id = ?`, newVal, id)
	if err != nil {
		return fmt.Errorf("update portal cfv: %w", err)
	}
	return nil
}

// --- scan helpers ----------------------------------------------------------

// rowScanner mirrors the Scan methods of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanCustomFieldDefinition(rows *sql.Rows) (models.CustomFieldDefinition, error) {
	return scanCustomFieldDefinitionRow(rows)
}

func scanCustomFieldDefinitionRow(row rowScanner) (models.CustomFieldDefinition, error) {
	var cf models.CustomFieldDefinition
	var options, description sql.NullString
	err := row.Scan(
		&cf.ID, &cf.Name, &cf.FieldType, &description,
		&cf.Required, &options, &cf.DisplayOrder, &cf.SystemDefault,
		&cf.AppliesToPortalCustomers, &cf.AppliesToCustomerOrganisations,
		&cf.CreatedAt, &cf.UpdatedAt,
	)
	if err != nil {
		return cf, err
	}
	if description.Valid {
		cf.Description = description.String
	}
	if options.Valid {
		cf.Options = options.String
	}
	return cf, nil
}
