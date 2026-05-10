package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// RequestTypeRepository persists request_types, their fields, and the
// surrounding lookups (channel/item-type existence, portal-section cleanup,
// screen-field resolution) that the request-type admin endpoints need.
type RequestTypeRepository struct {
	db database.Database
}

// NewRequestTypeRepository creates a RequestTypeRepository.
func NewRequestTypeRepository(db database.Database) *RequestTypeRepository {
	return &RequestTypeRepository{db: db}
}

const requestTypeSelectColumns = `
	rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
	rt.icon, rt.color, rt.display_order, rt.is_active, rt.config,
	rt.visibility_group_ids, rt.visibility_org_ids, rt.workspace_id,
	rt.title_template, rt.created_at, rt.updated_at,
	c.name as channel_name, it.name as item_type_name`

const requestTypeFromJoins = `
	FROM request_types rt
	LEFT JOIN channels c ON rt.channel_id = c.id
	LEFT JOIN item_types it ON rt.item_type_id = it.id`

func scanRequestType(scanner interface {
	Scan(dest ...interface{}) error
}) (models.RequestType, error) {
	var rt models.RequestType
	var visibilityGroupIDs, visibilityOrgIDs *string
	if err := scanner.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
		&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.Config,
		&visibilityGroupIDs, &visibilityOrgIDs, &rt.WorkspaceID,
		&rt.TitleTemplate, &rt.CreatedAt, &rt.UpdatedAt,
		&rt.ChannelName, &rt.ItemTypeName); err != nil {
		return rt, err
	}
	rt.VisibilityGroupIDs = decodeIntJSONArray(visibilityGroupIDs)
	rt.VisibilityOrgIDs = decodeIntJSONArray(visibilityOrgIDs)
	return rt, nil
}

// ListByChannel returns all request types for a channel, ordered by display_order then name.
func (r *RequestTypeRepository) ListByChannel(channelID int) ([]models.RequestType, error) {
	rows, err := r.db.Query(`SELECT`+requestTypeSelectColumns+requestTypeFromJoins+`
		WHERE rt.channel_id = ?
		ORDER BY rt.display_order, rt.name`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list request_types for channel %d: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.RequestType
	for rows.Next() {
		rt, scanErr := scanRequestType(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan request_type: %w", scanErr)
		}
		out = append(out, rt)
	}
	if out == nil {
		out = []models.RequestType{}
	}
	return out, nil
}

// GetByID returns a request_type by id (with channel + item-type names joined).
// Returns ErrNotFound when missing.
func (r *RequestTypeRepository) GetByID(id int) (*models.RequestType, error) {
	row := r.db.QueryRow(`SELECT`+requestTypeSelectColumns+requestTypeFromJoins+`
		WHERE rt.id = ?`, id)
	rt, err := scanRequestType(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get request_type %d: %w", id, err)
	}
	return &rt, nil
}

// RequestTypeBasic carries the small subset of columns the Update audit
// path uses to detect what changed.
type RequestTypeBasic struct {
	Name          string
	ItemTypeID    int
	Icon          string
	Color         string
	TitleTemplate string
}

// GetBasicForChannel returns the editable-field snapshot for a request_type
// whose row is in the given channel. Returns ErrNotFound when missing.
func (r *RequestTypeRepository) GetBasicForChannel(id, channelID int) (*RequestTypeBasic, error) {
	var b RequestTypeBasic
	err := r.db.QueryRow(
		`SELECT name, item_type_id, icon, color, title_template FROM request_types WHERE id = ? AND channel_id = ?`,
		id, channelID,
	).Scan(&b.Name, &b.ItemTypeID, &b.Icon, &b.Color, &b.TitleTemplate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get request_type %d basic: %w", id, err)
	}
	return &b, nil
}

// GetNameForChannel returns the request_type name for an id scoped to channel.
// Returns ErrNotFound when missing.
func (r *RequestTypeRepository) GetNameForChannel(id, channelID int) (string, error) {
	var name string
	err := r.db.QueryRow(
		`SELECT name FROM request_types WHERE id = ? AND channel_id = ?`,
		id, channelID,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get request_type %d name: %w", id, err)
	}
	return name, nil
}

// GetItemTypeAndWorkspace returns the item_type_id + workspace_id for a
// request_type. workspace_id is nullable. Returns ErrNotFound when missing.
func (r *RequestTypeRepository) GetItemTypeAndWorkspace(id int) (itemTypeID int, workspaceID *int, err error) {
	err = r.db.QueryRow(
		"SELECT item_type_id, workspace_id FROM request_types WHERE id = ?",
		id,
	).Scan(&itemTypeID, &workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, ErrNotFound
	}
	if err != nil {
		return 0, nil, fmt.Errorf("get request_type %d itemtype/workspace: %w", id, err)
	}
	return itemTypeID, workspaceID, nil
}

// ChannelExists reports whether a channel row with the given id exists.
func (r *RequestTypeRepository) ChannelExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check channel %d: %w", id, err)
	}
	return ok, nil
}

// ItemTypeExists reports whether an item_type row with the given id exists.
func (r *RequestTypeRepository) ItemTypeExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check item_type %d: %w", id, err)
	}
	return ok, nil
}

// MaxDisplayOrder returns the largest display_order in use within a channel
// (0 when none exist). The handler uses it to compute a default for new rows.
func (r *RequestTypeRepository) MaxDisplayOrder(channelID int) (int, error) {
	var maxOrder int
	if err := r.db.QueryRow(
		"SELECT COALESCE(MAX(display_order), 0) FROM request_types WHERE channel_id = ?",
		channelID,
	).Scan(&maxOrder); err != nil {
		return 0, fmt.Errorf("max display_order for channel %d: %w", channelID, err)
	}
	return maxOrder, nil
}

// NameExistsInChannel reports whether another request_type already uses the
// given name in the channel. excludeID > 0 excludes that row from the check
// (so an Update doesn't collide with itself).
func (r *RequestTypeRepository) NameExistsInChannel(channelID int, name string, excludeID int) (bool, error) {
	var ok bool
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM request_types WHERE name = ? AND channel_id = ? AND id != ?)",
			name, channelID, excludeID,
		).Scan(&ok)
	} else {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM request_types WHERE name = ? AND channel_id = ?)",
			name, channelID,
		).Scan(&ok)
	}
	if err != nil {
		return false, fmt.Errorf("check request_type name %q in channel %d: %w", name, channelID, err)
	}
	return ok, nil
}

// Create inserts a request_type. Returns ErrDuplicateEntry when the unique
// (name, channel_id) constraint trips at the DB level.
func (r *RequestTypeRepository) Create(rt *models.RequestType) (int64, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO request_types (channel_id, name, description, item_type_id, icon, color, display_order, is_active, visibility_group_ids, visibility_org_ids, workspace_id, title_template, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, rt.ChannelID, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive,
		encodeIntJSONArray(rt.VisibilityGroupIDs), encodeIntJSONArray(rt.VisibilityOrgIDs), rt.WorkspaceID, rt.TitleTemplate, now, now,
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create request_type: %w", err)
	}
	return id, nil
}

// Update replaces the editable fields of a request_type scoped to channelID.
// Returns ErrNotFound when no row matches and ErrDuplicateEntry on
// (name, channel_id) collision.
func (r *RequestTypeRepository) Update(id, channelID int, rt *models.RequestType) error {
	res, err := r.db.ExecWrite(`
		UPDATE request_types
		SET name = ?, description = ?, item_type_id = ?, icon = ?, color = ?, display_order = ?, is_active = ?,
		    visibility_group_ids = ?, visibility_org_ids = ?, title_template = ?, updated_at = ?
		WHERE id = ? AND channel_id = ?
	`, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive,
		encodeIntJSONArray(rt.VisibilityGroupIDs), encodeIntJSONArray(rt.VisibilityOrgIDs), rt.TitleTemplate, time.Now(), id, channelID,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update request_type %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateVisibility writes only the visibility_group_ids / visibility_org_ids
// columns. Returns ErrNotFound when no row matches.
func (r *RequestTypeRepository) UpdateVisibility(id, channelID int, groupIDs, orgIDs []int) error {
	res, err := r.db.ExecWrite(
		"UPDATE request_types SET visibility_group_ids = ?, visibility_org_ids = ?, updated_at = ? WHERE id = ? AND channel_id = ?",
		encodeIntJSONArray(groupIDs), encodeIntJSONArray(orgIDs), time.Now(), id, channelID,
	)
	if err != nil {
		return fmt.Errorf("update request_type %d visibility: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a request_type's fields and the row itself, scoped to
// channel_id. Returns ErrNotFound when the row doesn't exist in that channel.
// Caller is responsible for calling RemoveFromPortalSections beforehand if
// portal-section cleanup is desired (the prior handler did so best-effort).
func (r *RequestTypeRepository) Delete(id, channelID int) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM request_type_fields WHERE request_type_id = ?",
		id,
	); err != nil {
		return fmt.Errorf("delete fields for request_type %d: %w", id, err)
	}
	res, err := r.db.ExecWrite(
		"DELETE FROM request_types WHERE id = ? AND channel_id = ?",
		id, channelID,
	)
	if err != nil {
		return fmt.Errorf("delete request_type %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListFields returns a request_type's fields with the joined custom-field
// definition name when applicable.
func (r *RequestTypeRepository) ListFields(requestTypeID int) ([]models.RequestTypeField, error) {
	rows, err := r.db.Query(`
		SELECT rtf.id, rtf.request_type_id, rtf.field_identifier, rtf.field_type,
		       rtf.display_order, rtf.is_required, rtf.display_name, rtf.description,
		       COALESCE(rtf.step_number, 1) as step_number,
		       rtf.virtual_field_type, rtf.virtual_field_options,
		       rtf.created_at, rtf.updated_at,
		       CASE
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_name,
		       CASE
		           WHEN rtf.display_name IS NOT NULL AND rtf.display_name != '' THEN rtf.display_name
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_label
		FROM request_type_fields rtf
		LEFT JOIN custom_field_definitions cfd ON rtf.field_type = 'custom' AND rtf.field_identifier = CAST(cfd.id AS TEXT)
		WHERE rtf.request_type_id = ?
		ORDER BY rtf.step_number, rtf.display_order, rtf.id
	`, requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("list fields for request_type %d: %w", requestTypeID, err)
	}
	defer func() { _ = rows.Close() }()

	var fields []models.RequestTypeField
	for rows.Next() {
		var f models.RequestTypeField
		if err := rows.Scan(&f.ID, &f.RequestTypeID, &f.FieldIdentifier, &f.FieldType,
			&f.DisplayOrder, &f.IsRequired, &f.DisplayName, &f.Description,
			&f.StepNumber, &f.VirtualFieldType, &f.VirtualFieldOptions,
			&f.CreatedAt, &f.UpdatedAt,
			&f.FieldName, &f.FieldLabel); err != nil {
			return nil, fmt.Errorf("scan request_type field: %w", err)
		}
		fields = append(fields, f)
	}
	if fields == nil {
		fields = []models.RequestTypeField{}
	}
	return fields, nil
}

// ReplaceFields atomically replaces the field schema for a request_type.
// step_number defaults to 1 when zero on input.
func (r *RequestTypeRepository) ReplaceFields(requestTypeID int, fields []models.RequestTypeField) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM request_type_fields WHERE request_type_id = ?",
		requestTypeID,
	); err != nil {
		return fmt.Errorf("clear fields for request_type %d: %w", requestTypeID, err)
	}

	now := time.Now()
	for _, f := range fields {
		stepNumber := f.StepNumber
		if stepNumber == 0 {
			stepNumber = 1
		}
		if _, err := r.db.ExecWrite(`
			INSERT INTO request_type_fields (request_type_id, field_identifier, field_type, display_order, is_required,
			                                  display_name, description, step_number, virtual_field_type, virtual_field_options,
			                                  created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, requestTypeID, f.FieldIdentifier, f.FieldType, f.DisplayOrder, f.IsRequired,
			f.DisplayName, f.Description, stepNumber, f.VirtualFieldType, f.VirtualFieldOptions,
			now, now); err != nil {
			return fmt.Errorf("insert field %q: %w", f.FieldIdentifier, err)
		}
	}
	return nil
}

func encodeIntJSONArray(ids []int) *string {
	if len(ids) == 0 {
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

func decodeIntJSONArray(s *string) []int {
	if s == nil || *s == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(*s), &ids); err != nil {
		return nil
	}
	return ids
}
