package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AssetReportRepository persists asset_reports and their fields. The shape
// mirrors RequestTypeRepository (channel-scoped CRUD + portal-section
// cleanup + screen-field availability), with the asset-report-specific
// columns: asset_set_id, cql_query, column_config, run_mode, config.
type AssetReportRepository struct {
	db database.Database
}

// NewAssetReportRepository creates an AssetReportRepository.
func NewAssetReportRepository(db database.Database) *AssetReportRepository {
	return &AssetReportRepository{db: db}
}

const assetReportSelectQuery = `
	SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
	       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
	       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
	       ar.run_mode, ar.item_type_id, ar.workspace_id, ar.config,
	       ar.created_at, ar.updated_at,
	       c.name as channel_name, ams.name as asset_set_name,
	       it.name as item_type_name
	FROM asset_reports ar
	LEFT JOIN channels c ON ar.channel_id = c.id
	LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
	LEFT JOIN item_types it ON ar.item_type_id = it.id`

func scanAssetReport(scanner interface{ Scan(...interface{}) error }) (models.AssetReport, error) {
	var ar models.AssetReport
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs, config *string
	var itemTypeName sql.NullString
	if err := scanner.Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.RunMode, &ar.ItemTypeID, &ar.WorkspaceID, &config,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName,
		&itemTypeName); err != nil {
		return ar, err
	}
	ar.ColumnConfig = decodeStringJSONArray(columnConfig)
	ar.VisibilityGroupIDs = decodeIntJSONArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = decodeIntJSONArray(visibilityOrgIDs)
	ar.Config = config
	if itemTypeName.Valid {
		ar.ItemTypeName = itemTypeName.String
	}
	return ar, nil
}

// ListByChannel returns all asset reports for a channel, ordered by
// display_order then name.
func (r *AssetReportRepository) ListByChannel(channelID int) ([]models.AssetReport, error) {
	rows, err := r.db.Query(assetReportSelectQuery+" WHERE ar.channel_id = ? ORDER BY ar.display_order, ar.name", channelID)
	if err != nil {
		return nil, fmt.Errorf("list asset_reports for channel %d: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.AssetReport
	for rows.Next() {
		ar, scanErr := scanAssetReport(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan asset_report: %w", scanErr)
		}
		out = append(out, ar)
	}
	if out == nil {
		out = []models.AssetReport{}
	}
	return out, nil
}

// GetByID returns one asset report with the joined channel/set/item-type
// names. Returns ErrNotFound when missing.
func (r *AssetReportRepository) GetByID(id int) (*models.AssetReport, error) {
	row := r.db.QueryRow(assetReportSelectQuery+" WHERE ar.id = ?", id)
	ar, err := scanAssetReport(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get asset_report %d: %w", id, err)
	}
	return &ar, nil
}

// AssetReportBasic is the small field set Update needs to detect changes.
type AssetReportBasic struct {
	Name       string
	AssetSetID int
	Icon       string
	Color      string
}

// GetBasicForChannel returns the editable-field snapshot for an asset_report
// scoped to channel. Returns ErrNotFound when missing.
func (r *AssetReportRepository) GetBasicForChannel(id, channelID int) (*AssetReportBasic, error) {
	var b AssetReportBasic
	err := r.db.QueryRow(
		`SELECT name, asset_set_id, icon, color FROM asset_reports WHERE id = ? AND channel_id = ?`,
		id, channelID,
	).Scan(&b.Name, &b.AssetSetID, &b.Icon, &b.Color)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get asset_report %d basic: %w", id, err)
	}
	return &b, nil
}

// GetNameForChannel returns the asset_report name scoped to channel.
// Returns ErrNotFound when missing.
func (r *AssetReportRepository) GetNameForChannel(id, channelID int) (string, error) {
	var name string
	err := r.db.QueryRow(
		"SELECT name FROM asset_reports WHERE id = ? AND channel_id = ?",
		id, channelID,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get asset_report %d name: %w", id, err)
	}
	return name, nil
}

// GetItemTypeAndWorkspace returns item_type_id and workspace_id for an
// asset_report (both nullable). Returns ErrNotFound when missing.
func (r *AssetReportRepository) GetItemTypeAndWorkspace(id int) (itemTypeID, workspaceID *int, err error) {
	err = r.db.QueryRow(
		"SELECT item_type_id, workspace_id FROM asset_reports WHERE id = ?",
		id,
	).Scan(&itemTypeID, &workspaceID)
	if err == sql.ErrNoRows {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get asset_report %d itemtype/workspace: %w", id, err)
	}
	return itemTypeID, workspaceID, nil
}

// ChannelExists / AssetSetExists are FK-style validators the handler runs
// before insert/update.
func (r *AssetReportRepository) ChannelExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check channel %d: %w", id, err)
	}
	return ok, nil
}

func (r *AssetReportRepository) AssetSetExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_management_sets WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check asset_set %d: %w", id, err)
	}
	return ok, nil
}

// MaxDisplayOrder returns the largest display_order in use within a channel,
// or 0 when none exist.
func (r *AssetReportRepository) MaxDisplayOrder(channelID int) (int, error) {
	var maxOrder int
	if err := r.db.QueryRow(
		"SELECT COALESCE(MAX(display_order), 0) FROM asset_reports WHERE channel_id = ?",
		channelID,
	).Scan(&maxOrder); err != nil {
		return 0, fmt.Errorf("max display_order for channel %d: %w", channelID, err)
	}
	return maxOrder, nil
}

// NameExistsInChannel reports whether another asset_report row with the
// given name exists in the channel. excludeID > 0 excludes that row from
// the check.
func (r *AssetReportRepository) NameExistsInChannel(channelID int, name string, excludeID int) (bool, error) {
	var ok bool
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM asset_reports WHERE name = ? AND channel_id = ? AND id != ?)",
			name, channelID, excludeID,
		).Scan(&ok)
	} else {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM asset_reports WHERE name = ? AND channel_id = ?)",
			name, channelID,
		).Scan(&ok)
	}
	if err != nil {
		return false, fmt.Errorf("check asset_report name %q in channel %d: %w", name, channelID, err)
	}
	return ok, nil
}

// Create inserts an asset_report. Returns ErrDuplicateEntry on
// (name, channel_id) collision.
func (r *AssetReportRepository) Create(ar *models.AssetReport) (int64, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO asset_reports (channel_id, asset_set_id, name, description, cql_query, icon, color, display_order, is_active, column_config, visibility_group_ids, visibility_org_ids, run_mode, item_type_id, workspace_id, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, ar.ChannelID, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		encodeStringJSONArray(ar.ColumnConfig), encodeIntJSONArray(ar.VisibilityGroupIDs), encodeIntJSONArray(ar.VisibilityOrgIDs),
		ar.RunMode, ar.ItemTypeID, ar.WorkspaceID, ar.Config, now, now,
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create asset_report: %w", err)
	}
	return id, nil
}

// Update replaces editable fields scoped to channelID. Returns ErrNotFound on
// rowsAffected == 0; ErrDuplicateEntry on (name, channel_id) collision.
func (r *AssetReportRepository) Update(id, channelID int, ar *models.AssetReport) error {
	res, err := r.db.ExecWrite(`
		UPDATE asset_reports
		SET asset_set_id = ?, name = ?, description = ?, cql_query = ?, icon = ?, color = ?, display_order = ?, is_active = ?,
		    column_config = ?, visibility_group_ids = ?, visibility_org_ids = ?,
		    run_mode = ?, item_type_id = ?, config = ?,
		    updated_at = ?
		WHERE id = ? AND channel_id = ?
	`, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		encodeStringJSONArray(ar.ColumnConfig), encodeIntJSONArray(ar.VisibilityGroupIDs), encodeIntJSONArray(ar.VisibilityOrgIDs),
		ar.RunMode, ar.ItemTypeID, ar.Config, time.Now(), id, channelID,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update asset_report %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateVisibility writes only the visibility columns. ErrNotFound on miss.
func (r *AssetReportRepository) UpdateVisibility(id, channelID int, groupIDs, orgIDs []int) error {
	res, err := r.db.ExecWrite(
		"UPDATE asset_reports SET visibility_group_ids = ?, visibility_org_ids = ?, updated_at = ? WHERE id = ? AND channel_id = ?",
		encodeIntJSONArray(groupIDs), encodeIntJSONArray(orgIDs), time.Now(), id, channelID,
	)
	if err != nil {
		return fmt.Errorf("update asset_report %d visibility: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an asset_report row. asset_report_fields cascades by FK.
// Returns ErrNotFound on miss.
func (r *AssetReportRepository) Delete(id, channelID int) error {
	res, err := r.db.ExecWrite("DELETE FROM asset_reports WHERE id = ? AND channel_id = ?", id, channelID)
	if err != nil {
		return fmt.Errorf("delete asset_report %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveFromPortalSections is the asset-report-specific equivalent of
// RequestTypeRepository.RemoveFromPortalSections — same load/mutate/save
// dance, scoped to PortalSection.AssetReportIDs. Best-effort; errors are
// swallowed (matches prior handler behavior).
func (r *AssetReportRepository) RemoveFromPortalSections(channelID, idToRemove int) {
	var configStr string
	err := r.db.QueryRow("SELECT config FROM channels WHERE id = ?", channelID).Scan(&configStr)
	if err != nil || configStr == "" {
		return
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return
	}
	modified := false
	for i := range config.PortalSections {
		ids := config.PortalSections[i].AssetReportIDs
		newIDs := make([]int, 0, len(ids))
		for _, v := range ids {
			if v == idToRemove {
				modified = true
				continue
			}
			newIDs = append(newIDs, v)
		}
		config.PortalSections[i].AssetReportIDs = newIDs
	}
	if !modified {
		return
	}
	updated, err := json.Marshal(config)
	if err != nil {
		return
	}
	_, _ = r.db.ExecWrite(
		"UPDATE channels SET config = ?, updated_at = ? WHERE id = ?",
		string(updated), time.Now(), channelID,
	)
}

const assetReportFieldsSelectQuery = `
	SELECT arf.id, arf.asset_report_id, arf.field_identifier, arf.field_type,
	       arf.display_order, arf.is_required, arf.display_name, arf.description,
	       COALESCE(arf.step_number, 1) as step_number,
	       arf.virtual_field_type, arf.virtual_field_options,
	       arf.created_at, arf.updated_at,
	       CASE
	           WHEN arf.field_type = 'virtual' THEN arf.field_identifier
	           ELSE COALESCE(cfd.name, arf.field_identifier)
	       END as field_name,
	       CASE
	           WHEN arf.display_name IS NOT NULL AND arf.display_name != '' THEN arf.display_name
	           WHEN arf.field_type = 'virtual' THEN arf.field_identifier
	           ELSE COALESCE(cfd.name, arf.field_identifier)
	       END as field_label
	FROM asset_report_fields arf
	LEFT JOIN custom_field_definitions cfd ON arf.field_type = 'custom' AND arf.field_identifier = CAST(cfd.id AS TEXT)
	WHERE arf.asset_report_id = ?
	ORDER BY arf.step_number, arf.display_order, arf.id`

// ListFields returns the form-mode fields for an asset_report.
func (r *AssetReportRepository) ListFields(assetReportID int) ([]models.AssetReportField, error) {
	rows, err := r.db.Query(assetReportFieldsSelectQuery, assetReportID)
	if err != nil {
		return nil, fmt.Errorf("list fields for asset_report %d: %w", assetReportID, err)
	}
	defer func() { _ = rows.Close() }()

	var fields []models.AssetReportField
	for rows.Next() {
		var f models.AssetReportField
		if err := rows.Scan(&f.ID, &f.AssetReportID, &f.FieldIdentifier, &f.FieldType,
			&f.DisplayOrder, &f.IsRequired, &f.DisplayName, &f.Description,
			&f.StepNumber, &f.VirtualFieldType, &f.VirtualFieldOptions,
			&f.CreatedAt, &f.UpdatedAt,
			&f.FieldName, &f.FieldLabel); err != nil {
			return nil, fmt.Errorf("scan asset_report field: %w", err)
		}
		fields = append(fields, f)
	}
	if fields == nil {
		fields = []models.AssetReportField{}
	}
	return fields, nil
}

// ReplaceFields atomically replaces the field schema. step_number defaults
// to 1 when zero on input.
func (r *AssetReportRepository) ReplaceFields(assetReportID int, fields []models.AssetReportField) error {
	if _, err := r.db.ExecWrite("DELETE FROM asset_report_fields WHERE asset_report_id = ?", assetReportID); err != nil {
		return fmt.Errorf("clear fields for asset_report %d: %w", assetReportID, err)
	}

	now := time.Now()
	for _, f := range fields {
		step := f.StepNumber
		if step == 0 {
			step = 1
		}
		if _, err := r.db.ExecWrite(`
			INSERT INTO asset_report_fields (asset_report_id, field_identifier, field_type, display_order, is_required,
			                                 display_name, description, step_number, virtual_field_type, virtual_field_options,
			                                 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, assetReportID, f.FieldIdentifier, f.FieldType, f.DisplayOrder, f.IsRequired,
			f.DisplayName, f.Description, step, f.VirtualFieldType, f.VirtualFieldOptions,
			now, now); err != nil {
			return fmt.Errorf("insert field %q: %w", f.FieldIdentifier, err)
		}
	}
	return nil
}

// GetCreateScreenID resolves a workspace + item_type to a configured
// create_screen_id. Returns nil + nil when no mapping exists.
func (r *AssetReportRepository) GetCreateScreenID(workspaceID, itemTypeID int) (*int, error) {
	var screenID *int
	err := r.db.QueryRow(`
		SELECT csit.create_screen_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_set_item_types csit
		  ON csit.configuration_set_id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? AND csit.item_type_id = ?
		LIMIT 1
	`, workspaceID, itemTypeID).Scan(&screenID)
	if err == sql.ErrNoRows {
		return nil, nil //nolint:nilnil // null screen mapping is a real "no override" signal, distinct from an error
	}
	if err != nil {
		return nil, fmt.Errorf("get create_screen_id for workspace %d / item_type %d: %w", workspaceID, itemTypeID, err)
	}
	return screenID, nil
}

// ListScreenFields mirrors RequestTypeRepository.ListScreenFields — returns
// the screen_fields rows for a screen, joined with custom_field_definitions
// for the "custom" entries. Duplication is intentional for now; the two
// will dedupe into a shared screens repo when one of them needs to evolve.
func (r *AssetReportRepository) ListScreenFields(screenID int) ([]ScreenFieldRow, error) {
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
	return out, nil
}

func encodeStringJSONArray(strs []string) *string {
	if len(strs) == 0 {
		return nil
	}
	data, err := json.Marshal(strs)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

func decodeStringJSONArray(s *string) []string {
	if s == nil || *s == "" {
		return nil
	}
	var strs []string
	if err := json.Unmarshal([]byte(*s), &strs); err != nil {
		return nil
	}
	return strs
}
