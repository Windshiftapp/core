package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WorkspaceTemplateRepository owns the reads and writes used when cloning a
// template workspace into a newly created one. All clone mutations run inside
// the caller's transaction so the destination is written atomically.
type WorkspaceTemplateRepository struct {
	db database.Database
}

func NewWorkspaceTemplateRepository(db database.Database) *WorkspaceTemplateRepository {
	return &WorkspaceTemplateRepository{db: db}
}

// TemplateEligibility carries the fields needed to decide whether a workspace
// may be used as a clone source.
type TemplateEligibility struct {
	ID         int
	Active     bool
	IsPersonal bool
	IsTemplate bool
}

// TemplateCloneItemTemplate is one source work-item template plus its target
// item-type mappings.
type TemplateCloneItemTemplate struct {
	Name            string
	DescriptionBody string
	Mode            string
	IsActive        bool
	ItemTypeIDs     []int
}

// TemplateCloneItem is one source seed item read for cloning.
type TemplateCloneItem struct {
	ID                int
	ItemTypeID        sql.NullInt64
	Title             string
	Description       sql.NullString
	IsTask            bool
	StatusID          sql.NullInt64
	PriorityID        sql.NullInt64
	StartDate         sql.NullTime
	DueDate           sql.NullTime
	EndDate           sql.NullTime
	StoryPoints       sql.NullFloat64
	EstimateMinutes   sql.NullInt64
	FracIndex         string
	CustomFieldValues sql.NullString
	ParentID          sql.NullInt64
	RelatedWorkItemID sql.NullInt64
}

// TemplateCloneLabel is one global item-label association to copy.
type TemplateCloneLabel struct {
	ItemID  int
	LabelID int
}

// TemplateCloneLink is one item-to-item link to copy.
type TemplateCloneLink struct {
	LinkTypeID    int
	SourceID      int
	TargetID      int
	CustomFieldID sql.NullInt64
}

// TemplateCustomFieldDef is the definition slice the custom-field copy policy
// needs: the type and the current select options.
type TemplateCustomFieldDef struct {
	FieldType string
	Options   string
}

// ListTemplateSummaries returns every active, non-personal workspace marked as
// a template with picker metadata. Visibility filtering happens in the service
// layer; this query only selects structurally eligible templates.
func (r *WorkspaceTemplateRepository) ListTemplateSummaries(ctx context.Context) ([]models.WorkspaceTemplateSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.id, w.name, COALESCE(w.description, ''), COALESCE(w.icon, ''), COALESCE(w.color, ''),
		       COALESCE(cs.name, ''),
		       (SELECT COUNT(*) FROM item_templates t WHERE t.workspace_id = w.id),
		       (SELECT COUNT(*) FROM items i WHERE i.workspace_id = w.id)
		FROM workspaces w
		LEFT JOIN workspace_configuration_sets wcs ON wcs.workspace_id = w.id
		LEFT JOIN configuration_sets cs ON cs.id = wcs.configuration_set_id
		WHERE w.is_template = true
		  AND w.active = true
		  AND (w.is_personal = false OR w.is_personal IS NULL)
		ORDER BY w.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list workspace template summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.WorkspaceTemplateSummary
	for rows.Next() {
		var summary models.WorkspaceTemplateSummary
		if err := rows.Scan(&summary.ID, &summary.Name, &summary.Description, &summary.Icon, &summary.Color,
			&summary.ConfigurationSetName, &summary.TemplateCount, &summary.ItemCount); err != nil {
			return nil, fmt.Errorf("scan workspace template summary: %w", err)
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace template summaries: %w", err)
	}
	return out, nil
}

// LoadTemplateEligibilityTx reads the clone-eligibility flags for a workspace
// inside the clone transaction's snapshot.
func (r *WorkspaceTemplateRepository) LoadTemplateEligibilityTx(ctx context.Context, tx database.Tx, workspaceID int) (*TemplateEligibility, error) {
	var eligibility TemplateEligibility
	err := tx.QueryRowContext(ctx, `
		SELECT id, COALESCE(active, false), COALESCE(is_personal, false), COALESCE(is_template, false)
		FROM workspaces WHERE id = ?
	`, workspaceID).Scan(&eligibility.ID, &eligibility.Active, &eligibility.IsPersonal, &eligibility.IsTemplate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load template eligibility: %w", err)
	}
	return &eligibility, nil
}

// GetWorkspaceConfigSetTx returns the source configuration-set assignment.
// count > 1 means the source snapshot violates the one-assignment contract.
func (r *WorkspaceTemplateRepository) GetWorkspaceConfigSetTx(ctx context.Context, tx database.Tx, workspaceID int) (configSetID *int64, count int, err error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT configuration_set_id FROM workspace_configuration_sets WHERE workspace_id = ?
	`, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("load workspace configuration set: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("scan workspace configuration set: %w", err)
		}
		configSetID = &id
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate workspace configuration sets: %w", err)
	}
	return configSetID, count, nil
}

// AttachConfigurationSetTx inserts the shared configuration-set assignment for
// the destination workspace.
func (r *WorkspaceTemplateRepository) AttachConfigurationSetTx(ctx context.Context, tx database.Tx, workspaceID int, configSetID int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, ?)
	`, workspaceID, configSetID, now)
	if err != nil {
		return fmt.Errorf("attach configuration set to cloned workspace: %w", err)
	}
	return nil
}

// ListItemTemplatesTx reads every source work-item template with its item-type
// mappings, ordered deterministically by name.
func (r *WorkspaceTemplateRepository) ListItemTemplatesTx(ctx context.Context, tx database.Tx, workspaceID int) ([]TemplateCloneItemTemplate, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT name, description_body, mode, is_active FROM item_templates
		WHERE workspace_id = ? ORDER BY name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item templates for clone: %w", err)
	}
	var templates []TemplateCloneItemTemplate
	for rows.Next() {
		var tmpl TemplateCloneItemTemplate
		if err := rows.Scan(&tmpl.Name, &tmpl.DescriptionBody, &tmpl.Mode, &tmpl.IsActive); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan item template for clone: %w", err)
		}
		templates = append(templates, tmpl)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate item templates for clone: %w", err)
	}
	_ = rows.Close()
	if len(templates) == 0 {
		return templates, nil
	}

	names := make([]any, len(templates))
	placeholders := make([]string, len(templates))
	for i, tmpl := range templates {
		names[i] = tmpl.Name
		placeholders[i] = "?"
	}
	typeRows, err := tx.QueryContext(ctx, `
		SELECT t.name, tit.item_type_id
		FROM item_templates t
		JOIN item_template_item_types tit ON tit.template_id = t.id
		WHERE t.workspace_id = ? AND t.name IN (`+strings.Join(placeholders, ",")+`)
	`, append([]any{workspaceID}, names...)...)
	if err != nil {
		return nil, fmt.Errorf("list item template type mappings for clone: %w", err)
	}
	defer func() { _ = typeRows.Close() }()
	byName := make(map[string][]int, len(templates))
	for typeRows.Next() {
		var name string
		var itemTypeID int
		if err := typeRows.Scan(&name, &itemTypeID); err != nil {
			return nil, fmt.Errorf("scan item template type mapping for clone: %w", err)
		}
		byName[name] = append(byName[name], itemTypeID)
	}
	if err := typeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item template type mappings for clone: %w", err)
	}
	for i := range templates {
		templates[i].ItemTypeIDs = byName[templates[i].Name]
	}
	return templates, nil
}

// InsertItemTemplateTx writes one cloned work-item template and its type
// mappings with fresh IDs and the clone actor/timestamps.
func (r *WorkspaceTemplateRepository) InsertItemTemplateTx(ctx context.Context, tx database.Tx, workspaceID int, tmpl TemplateCloneItemTemplate, creatorID int, now time.Time) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO item_templates (workspace_id, name, description_body, mode, is_active, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, tmpl.Name, tmpl.DescriptionBody, tmpl.Mode, tmpl.IsActive, creatorID, creatorID, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert cloned item template: %w", err)
	}
	for _, itemTypeID := range tmpl.ItemTypeIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO item_template_item_types (template_id, item_type_id) VALUES (?, ?)
		`, id, itemTypeID); err != nil {
			return 0, fmt.Errorf("insert cloned item template type mapping: %w", err)
		}
	}
	return id, nil
}

// CountWorkspaceItemsTx counts seed items in the clone snapshot so the size
// limit can be enforced before the destination workspace is inserted.
func (r *WorkspaceTemplateRepository) CountWorkspaceItemsTx(ctx context.Context, tx database.Tx, workspaceID int) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE workspace_id = ?`, workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count workspace items for clone: %w", err)
	}
	return count, nil
}

// ListSeedItemsTx reads every source item in deterministic workspace item
// number order.
func (r *WorkspaceTemplateRepository) ListSeedItemsTx(ctx context.Context, tx database.Tx, workspaceID int) ([]TemplateCloneItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, item_type_id, title, description, COALESCE(is_task, false), status_id, priority_id,
		       start_date, due_date, end_date, story_points, estimate_minutes,
		       frac_index, custom_field_values, parent_id, related_work_item_id
		FROM items
		WHERE workspace_id = ?
		ORDER BY workspace_item_number
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list seed items for clone: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []TemplateCloneItem
	for rows.Next() {
		var item TemplateCloneItem
		if err := rows.Scan(&item.ID, &item.ItemTypeID, &item.Title, &item.Description, &item.IsTask,
			&item.StatusID, &item.PriorityID, &item.StartDate, &item.DueDate, &item.EndDate,
			&item.StoryPoints, &item.EstimateMinutes, &item.FracIndex,
			&item.CustomFieldValues, &item.ParentID, &item.RelatedWorkItemID); err != nil {
			return nil, fmt.Errorf("scan seed item for clone: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seed items for clone: %w", err)
	}
	return items, nil
}

// InsertClonedItemTx writes one cloned item with cleared operational fields,
// a fresh dense item number, and a fresh fractional index. It goes through
// ItemRepository.Create so a cloned row has the same shape as any other
// created item. Hierarchy and internal references are restored in a second
// pass.
func (r *WorkspaceTemplateRepository) InsertClonedItemTx(_ context.Context, tx database.Tx, insert ClonedItemInsert) (int64, error) {
	customFields, err := unmarshalCustomFieldsJSON(insert.CustomFieldValues)
	if err != nil {
		return 0, fmt.Errorf("decode cloned custom fields: %w", err)
	}
	item := &models.Item{
		WorkspaceID:         insert.WorkspaceID,
		WorkspaceItemNumber: insert.ItemNumber,
		ItemTypeID:          nullInt64Ptr(insert.ItemTypeID),
		Title:               insert.Title,
		Description:         insert.Description.String,
		IsTask:              insert.IsTask,
		StatusID:            nullInt64Ptr(insert.StatusID),
		PriorityID:          nullInt64Ptr(insert.PriorityID),
		StartDate:           nullTimePtr(insert.StartDate),
		DueDate:             nullTimePtr(insert.DueDate),
		EndDate:             nullTimePtr(insert.EndDate),
		StoryPoints:         nullFloat64Ptr(insert.StoryPoints),
		EstimateMinutes:     nullInt64Ptr(insert.EstimateMinutes),
		FracIndex:           &insert.FracIndex,
		CustomFieldValues:   customFields,
		CreatedAt:           insert.CreatedAt,
		UpdatedAt:           insert.CreatedAt,
		LastActiveAt:        insert.CreatedAt,
	}
	if insert.CreatorID > 0 {
		creatorID := insert.CreatorID
		item.CreatorID = &creatorID
	}
	id, err := NewItemRepository(r.db).Create(tx, item)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, fmt.Errorf("%w: %w", ErrDuplicateEntry, err)
		}
		return 0, fmt.Errorf("insert cloned item: %w", err)
	}
	return int64(id), nil
}

func unmarshalCustomFieldsJSON(value sql.NullString) (map[string]any, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(value.String), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func nullInt64Ptr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}

func nullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	f := value.Float64
	return &f
}

// ClonedItemInsert is the write-side shape of one cloned seed item.
type ClonedItemInsert struct {
	WorkspaceID       int
	ItemNumber        int
	ItemTypeID        sql.NullInt64
	Title             string
	Description       sql.NullString
	IsTask            bool
	StatusID          sql.NullInt64
	PriorityID        sql.NullInt64
	StartDate         sql.NullTime
	DueDate           sql.NullTime
	EndDate           sql.NullTime
	StoryPoints       sql.NullFloat64
	EstimateMinutes   sql.NullInt64
	FracIndex         string
	CustomFieldValues sql.NullString
	CreatorID         int
	CreatedAt         time.Time
}

// RestoreClonedHierarchyTx remaps parent and internal related-item references
// and stores the materialized path for the cloned item.
func (r *WorkspaceTemplateRepository) RestoreClonedHierarchyTx(ctx context.Context, tx database.Tx, newID int64, path string, parentID, relatedWorkItemID *int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE items SET parent_id = ?, path = ? WHERE id = ?`, nullableInt64Arg(parentID), path, newID)
	if err != nil {
		return fmt.Errorf("restore cloned item hierarchy: %w", err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE items SET related_work_item_id = ? WHERE id = ?`, nullableInt64Arg(relatedWorkItemID), newID)
	if err != nil {
		return fmt.Errorf("restore cloned item relation: %w", err)
	}
	return nil
}

func nullableInt64Arg(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// ListItemLabelsTx reads global label associations for every item in the
// source workspace.
func (r *WorkspaceTemplateRepository) ListItemLabelsTx(ctx context.Context, tx database.Tx, workspaceID int) ([]TemplateCloneLabel, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT il.item_id, il.label_id
		FROM item_labels il
		JOIN items i ON i.id = il.item_id
		WHERE i.workspace_id = ?
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item labels for clone: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var labels []TemplateCloneLabel
	for rows.Next() {
		var label TemplateCloneLabel
		if err := rows.Scan(&label.ItemID, &label.LabelID); err != nil {
			return nil, fmt.Errorf("scan item label for clone: %w", err)
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item labels for clone: %w", err)
	}
	return labels, nil
}

// InsertItemLabelTx attaches one global label to a cloned item.
func (r *WorkspaceTemplateRepository) InsertItemLabelTx(ctx context.Context, tx database.Tx, itemID, labelID int, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)
		ON CONFLICT (item_id, label_id) DO NOTHING
	`, itemID, labelID, now)
	if err != nil {
		return fmt.Errorf("insert cloned item label: %w", err)
	}
	return nil
}

// ListItemLinksTx reads item-to-item links whose endpoints are both items of
// the source workspace.
func (r *WorkspaceTemplateRepository) ListItemLinksTx(ctx context.Context, tx database.Tx, workspaceID int) ([]TemplateCloneLink, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT link_type_id, source_id, target_id, custom_field_id
		FROM item_links
		WHERE source_type = 'item' AND target_type = 'item'
		  AND source_id IN (SELECT id FROM items WHERE workspace_id = ?)
		  AND target_id IN (SELECT id FROM items WHERE workspace_id = ?)
	`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item links for clone: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var links []TemplateCloneLink
	for rows.Next() {
		var link TemplateCloneLink
		if err := rows.Scan(&link.LinkTypeID, &link.SourceID, &link.TargetID, &link.CustomFieldID); err != nil {
			return nil, fmt.Errorf("scan item link for clone: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item links for clone: %w", err)
	}
	return links, nil
}

// InsertItemLinkTx writes one remapped item-to-item link.
func (r *WorkspaceTemplateRepository) InsertItemLinkTx(ctx context.Context, tx database.Tx, link TemplateCloneLink, creatorID int, now time.Time) (int, error) {
	var customFieldID any
	if link.CustomFieldID.Valid {
		customFieldID = link.CustomFieldID.Int64
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, custom_field_id, created_at)
		VALUES (?, 'item', ?, 'item', ?, ?, ?, ?)
		ON CONFLICT (link_type_id, source_type, source_id, target_type, target_id) DO NOTHING
		RETURNING id
	`, link.LinkTypeID, link.SourceID, link.TargetID, creatorID, customFieldID, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("insert cloned item link: %w", err)
	}
	return id, nil
}

// LoadCustomFieldDefsTx loads the definitions referenced by cloned custom
// field values so the copy policy can validate against current types and
// options.
func (r *WorkspaceTemplateRepository) LoadCustomFieldDefsTx(ctx context.Context, tx database.Tx, fieldIDs []int) (map[int]TemplateCustomFieldDef, error) {
	out := make(map[int]TemplateCustomFieldDef, len(fieldIDs))
	if len(fieldIDs) == 0 {
		return out, nil
	}

	const chunkSize = 500
	for start := 0; start < len(fieldIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(fieldIDs) {
			end = len(fieldIDs)
		}
		chunk := fieldIDs[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT id, field_type, COALESCE(options, '') FROM custom_field_definitions
			WHERE id IN (`+strings.Join(placeholders, ",")+`)
		`, args...)
		if err != nil {
			return nil, fmt.Errorf("load custom field definitions for clone: %w", err)
		}
		for rows.Next() {
			var id int
			var def TemplateCustomFieldDef
			if err := rows.Scan(&id, &def.FieldType, &def.Options); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan custom field definition for clone: %w", err)
			}
			out[id] = def
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate custom field definitions for clone: %w", err)
		}
		_ = rows.Close()
	}
	return out, nil
}

// ReferencedCatalogIDsMissingTx returns which of the referenced IDs do not
// exist in the given catalog table. Used to reject inconsistent source
// snapshots before any destination row is written.
func (r *WorkspaceTemplateRepository) ReferencedCatalogIDsMissingTx(ctx context.Context, tx database.Tx, table string, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	present := make(map[int]bool, len(ids))
	const chunkSize = 500
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM `+table+` WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("load %s ids for clone validation: %w", table, err)
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan %s id for clone validation: %w", table, err)
			}
			present[id] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate %s ids for clone validation: %w", table, err)
		}
		_ = rows.Close()
	}

	var missing []int
	for _, id := range ids {
		if !present[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
