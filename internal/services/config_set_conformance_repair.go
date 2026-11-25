package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ConfigSetConformanceService checks a configuration set against a canonical
// template and repairs drifted configuration back to it. Repair restores
// configuration only — item data is never written.
type ConfigSetConformanceService struct {
	db   database.Database
	repo *repository.ConfigurationSetRepository
	// enqueueOptionRemoval schedules item-value scrubbing for removed select
	// options; it runs inside the repair transaction. Wired at boot.
	enqueueOptionRemoval func(tx database.Tx, fieldID int, fieldType string, removedIDs []int) error
}

func NewConfigSetConformanceService(db database.Database, repo *repository.ConfigurationSetRepository) *ConfigSetConformanceService {
	return &ConfigSetConformanceService{db: db, repo: repo}
}

// SetOptionRemovalEnqueuer wires the custom-field cleanup scheduler so a
// repair that shrinks an option set commits the scrubbing job atomically.
func (s *ConfigSetConformanceService) SetOptionRemovalEnqueuer(fn func(tx database.Tx, fieldID int, fieldType string, removedIDs []int) error) {
	s.enqueueOptionRemoval = fn
}

// Check exports the configuration set's live state and diffs it against the
// canonical template. Read-only.
func (s *ConfigSetConformanceService) Check(ctx context.Context, configSetID int, canonical *ConfigSetTemplate) (*ConfigSetConformanceReport, error) {
	live, err := s.exportLive(ctx, configSetID)
	if err != nil {
		return nil, err
	}
	drifts := DiffConformanceTemplates(canonical, live)
	report := &ConfigSetConformanceReport{
		ConfigSetID: configSetID,
		Conformant:  len(drifts) == 0,
		DriftCount:  len(drifts),
		Drifts:      drifts,
		CheckedAt:   time.Now().UTC(),
	}
	for _, d := range drifts {
		if d.Repairable {
			report.RepairableCount++
		}
	}
	return report, nil
}

func (s *ConfigSetConformanceService) exportLive(ctx context.Context, configSetID int) (*ConfigSetTemplate, error) {
	exporter := NewConfigSetExportService(s.db, s.repo)
	return exporter.Export(ctx, configSetID, nil)
}

// AuditCheck records a conformance check run so reads of configuration
// state stay visible in the security log alongside repairs.
func (s *ConfigSetConformanceService) AuditCheck(actor AuditActor, configSetID, driftCount int) {
	emitServiceAudit(s.db, actor, logger.ActionConfigSetConformanceCheck, logger.ResourceConfigurationSet, &configSetID, "", map[string]any{
		"drift_count": driftCount,
	})
}

// AuditRepair records a conformance repair run with its per-row outcome.
func (s *ConfigSetConformanceService) AuditRepair(actor AuditActor, configSetID, repaired, failed, skipped int) {
	emitServiceAudit(s.db, actor, logger.ActionConfigSetConformanceRepair, logger.ResourceConfigurationSet, &configSetID, "", map[string]any{
		"repaired_count": repaired,
		"failed_count":   failed,
		"skipped_count":  skipped,
	})
}

// Repair brings the configuration set back to the canonical template. Rows
// are selected by drift ID; an empty selection repairs every repairable row.
//
// The repair runs in one transaction. Each entity (one workflow, one screen,
// the glue section, …) is wrapped in a SAVEPOINT so a single unfixable
// entity — a live approval request blocking a rewrite, a missing identity —
// fails its own rows without rolling back the rest.
func (s *ConfigSetConformanceService) Repair(ctx context.Context, configSetID int, canonical *ConfigSetTemplate, selectedIDs []string) (*ConfigSetConformanceRepairResult, error) {
	live, err := s.exportLive(ctx, configSetID)
	if err != nil {
		return nil, err
	}
	all := DiffConformanceTemplates(canonical, live)
	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}

	// Group repairable drift rows by entity. The entity name is resolved
	// against the canonical template's entity names so names that contain the
	// drift-path separator ("/") stay unambiguous; the links section repairs
	// as one unit.
	entities := map[string]*conformanceEntityPlan{}
	var order []string
	for _, d := range all {
		if !d.Repairable {
			continue
		}
		if len(selected) > 0 && !selected[d.ID] {
			continue
		}
		entityName := "*"
		if d.Section != "links" {
			entityName = resolveConformanceEntityName(canonical, d.Section, d.Name)
		}
		key := d.Section + "|" + strings.ToLower(entityName)
		if _, ok := entities[key]; !ok {
			entities[key] = &conformanceEntityPlan{section: d.Section, name: entityName}
			order = append(order, key)
		}
		entities[key].rows = append(entities[key].rows, d)
	}

	result := &ConfigSetConformanceRepairResult{ConfigSetID: configSetID}
	if len(order) == 0 {
		return s.finishRepairResult(ctx, configSetID, canonical, result)
	}

	workflowIDsByName, err := s.referencedWorkflowIDsByName(ctx, configSetID)
	if err != nil {
		return nil, err
	}

	r := &conformanceRepairer{
		svc:               s,
		canonical:         canonical,
		now:               time.Now(),
		ctx:               ctx,
		workflowIDsByName: workflowIDsByName,
	}

	// One transaction for the whole repair; each entity is wrapped in a
	// SAVEPOINT so one unfixable entity fails alone.
	err = database.WithTx(s.db, func(tx database.Tx) error {
		r.tx = tx

		// Repair order respects creation dependencies: definitions first, then
		// workflows, then the sets that bind to workflows, then the glue.
		repairSections(r, entities, order, "custom_fields", func(entity *conformanceEntityPlan) error {
			return r.repairCustomFields(entity)
		})
		repairSections(r, entities, order, "statuses", func(entity *conformanceEntityPlan) error {
			return r.repairStatuses(entity)
		})
		repairSections(r, entities, order, "item_types", func(entity *conformanceEntityPlan) error {
			return r.repairItemTypes(entity)
		})
		repairSections(r, entities, order, "priorities", func(entity *conformanceEntityPlan) error {
			return r.repairPriorities(entity)
		})
		repairSections(r, entities, order, "link_types", func(entity *conformanceEntityPlan) error {
			return r.repairLinkTypes(entity)
		})
		repairSections(r, entities, order, "screens", func(entity *conformanceEntityPlan) error {
			return r.repairScreens(entity)
		})
		repairSections(r, entities, order, "workflows", func(entity *conformanceEntityPlan) error {
			return r.repairWorkflows(entity)
		})
		repairSections(r, entities, order, "condition_sets", func(entity *conformanceEntityPlan) error {
			return r.repairConditionSets(entity)
		})
		repairSections(r, entities, order, "approval_sets", func(entity *conformanceEntityPlan) error {
			return r.repairApprovalSets(entity)
		})
		repairSections(r, entities, order, "links", func(entity *conformanceEntityPlan) error {
			return r.repairLinks(configSetID)
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, o := range r.outcomes {
		switch o.Status {
		case "repaired":
			result.Repaired++
		case "skipped":
			result.Skipped++
		case "failed":
			result.Failed++
		}
	}
	result.Outcomes = r.outcomes
	return s.finishRepairResult(ctx, configSetID, canonical, result)
}

func repairSections(r *conformanceRepairer, entities map[string]*conformanceEntityPlan, order []string, section string, fn func(*conformanceEntityPlan) error) {
	for _, key := range order {
		entity := entities[key]
		if entity.section != section {
			continue
		}
		r.current = entity
		r.savepointN++
		savepoint := fmt.Sprintf("conformance_repair_%d", r.savepointN)
		if err := r.exec(`SAVEPOINT ` + savepoint); err != nil {
			r.failEntity(entity, fmt.Sprintf("repair could not start: %v", err))
			continue
		}
		if err := fn(entity); err != nil {
			_ = r.exec(`ROLLBACK TO SAVEPOINT ` + savepoint)
			_ = r.exec(`RELEASE SAVEPOINT ` + savepoint)
			r.failEntity(entity, err.Error())
			continue
		}
		if err := r.exec(`RELEASE SAVEPOINT ` + savepoint); err != nil {
			r.failEntity(entity, fmt.Sprintf("repair could not commit: %v", err))
			continue
		}
		// The repairer may have recorded its own per-entity outcomes (e.g. a
		// manual-attention skip); only then is the blanket success skipped.
		if !entity.resolved {
			r.succeedEntity(entity)
		}
	}
}

func (s *ConfigSetConformanceService) finishRepairResult(ctx context.Context, configSetID int, canonical *ConfigSetTemplate, result *ConfigSetConformanceRepairResult) (*ConfigSetConformanceRepairResult, error) {
	report, err := s.Check(ctx, configSetID, canonical)
	if err != nil {
		return nil, err
	}
	result.PostCheck = report
	return result, nil
}

// ---- repair internals -------------------------------------------------------

type conformanceRepairer struct {
	svc        *ConfigSetConformanceService
	tx         database.Tx
	ctx        context.Context
	canonical  *ConfigSetTemplate
	now        time.Time
	savepointN int
	current    *conformanceEntityPlan
	outcomes   []ConfigSetConformanceRepairOutcome
	// workflowIDsByName maps a lowercased workflow name to the id of a workflow
	// the addressed configuration set actually references. Repair mutates only
	// these rows, never a same-named workflow owned by another set.
	workflowIDsByName map[string]int
}

// conformanceEntityPlan groups the drift rows repair touches for one entity
// (one workflow, one screen, the glue section, …).
type conformanceEntityPlan struct {
	section string
	name    string
	rows    []ConfigSetConformanceDrift
	// resolved marks that the repairer already recorded outcomes for this
	// entity, so the generic success pass must not overwrite them.
	resolved bool
}

func (r *conformanceRepairer) succeedEntity(entity *conformanceEntityPlan) {
	for _, row := range entity.rows {
		r.outcome(row, "repaired", "")
	}
}

func (r *conformanceRepairer) failEntity(entity *conformanceEntityPlan, detail string) {
	for _, row := range entity.rows {
		r.outcome(row, "failed", detail)
	}
}

// outcomeRows records one outcome per drift row addressed by an entity.
func (r *conformanceRepairer) outcomeRows(rows []ConfigSetConformanceDrift, status, detail string) {
	for _, row := range rows {
		r.outcome(row, status, detail)
	}
}

func (r *conformanceRepairer) outcome(row ConfigSetConformanceDrift, status, detail string) {
	entry := ConfigSetConformanceRepairOutcome{
		ID:      row.ID,
		Section: row.Section,
		Name:    row.Name,
		Status:  status,
	}
	if detail == "" {
		entry.Detail = row.Detail
	} else {
		entry.Detail = detail
	}
	r.outcomes = append(r.outcomes, entry)
}

func (r *conformanceRepairer) exec(query string, args ...any) error {
	_, err := r.tx.ExecContext(r.ctx, query, args...)
	return err
}

// lookupID resolves a name to an id inside the repair transaction.
func (r *conformanceRepairer) lookupID(table, column, name string) (int, error) {
	var id int
	err := r.tx.QueryRowContext(r.ctx, fmt.Sprintf(`SELECT id FROM %s WHERE LOWER(%s) = LOWER(?)`, table, column), name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *conformanceRepairer) repairCustomFields(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.CustomFields {
		if !entityCovers(entity, "custom_fields", want.Name) {
			continue
		}
		id, err := r.lookupID("custom_field_definitions", "name", want.Name)
		if err != nil {
			return err
		}
		if id == 0 {
			if err = r.exec(`
				INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
				                                       applies_to_portal_customers, applies_to_customer_organisations,
				                                       system_default, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, false, ?, ?)
			`, want.Name, models.CanonicalCustomFieldType(want.FieldType), want.Description, want.Required, want.Options, want.DisplayOrder,
				want.AppliesToPortalCustomers, want.AppliesToCustomerOrganisations, r.now, r.now); err != nil {
				return fmt.Errorf("create custom field: %w", err)
			}
			continue
		}
		if err := r.repairExistingCustomField(entity, id, want); err != nil {
			return err
		}
	}
	return nil
}

// repairExistingCustomField restores an existing custom field through the
// provisioning semantics: the field type never changes after creation, and a
// shrinking select option set enqueues item-value scrubbing instead of
// leaving dangling option ids in item data (WI-1529).
func (r *conformanceRepairer) repairExistingCustomField(entity *conformanceEntityPlan, fieldID int, want ConfigSetTplCustomField) error {
	var liveType string
	var liveOptions string
	if err := r.tx.QueryRowContext(r.ctx,
		`SELECT field_type, COALESCE(options, '') FROM custom_field_definitions WHERE id = ?`, fieldID,
	).Scan(&liveType, &liveOptions); err != nil {
		return fmt.Errorf("load custom field %q: %w", want.Name, err)
	}

	wantType := models.CanonicalCustomFieldType(want.FieldType)
	typeDrift := liveType != wantType

	// Option removals on select-like fields enqueue a scrubbing job in the
	// repair transaction; on a type change the stored values no longer match
	// either shape, so a type-drifted field is left for manual attention.
	removedOptions := removedSelectOptionIDs(liveOptions, want.Options, wantType)
	if typeDrift && len(removedOptions) > 0 {
		entity.resolved = true
		r.outcomeRows(entity.rows, "skipped", fmt.Sprintf(
			"field type %q cannot be restored to %q after creation; options left untouched so no dangling option ids are introduced",
			liveType, wantType))
		return nil
	}

	if err := r.exec(`
		UPDATE custom_field_definitions
		SET description = ?, required = ?, options = ?, display_order = ?,
		    applies_to_portal_customers = ?, applies_to_customer_organisations = ?, updated_at = ?
		WHERE id = ?
	`, want.Description, want.Required, want.Options, want.DisplayOrder,
		want.AppliesToPortalCustomers, want.AppliesToCustomerOrganisations, r.now, fieldID); err != nil {
		return fmt.Errorf("restore custom field: %w", err)
	}

	if len(removedOptions) > 0 {
		if r.svc.enqueueOptionRemoval == nil {
			return fmt.Errorf("custom field %q: option set shrank but no cleanup scheduler is wired", want.Name)
		}
		if err := r.svc.enqueueOptionRemoval(r.tx, fieldID, wantType, removedOptions); err != nil {
			return fmt.Errorf("enqueue option cleanup for custom field %q: %w", want.Name, err)
		}
	}

	detail := ""
	if typeDrift {
		entity.resolved = true
		detail = fmt.Sprintf("field type %q cannot be changed after creation; resolve the type manually", liveType)
	}
	r.outcomeRows(entity.rows, "repaired", detail)
	return nil
}

// removedSelectOptionIDs returns the option ids present in oldOptionsJSON but
// missing from newOptionsJSON. Non-select fields and unparseable payloads
// return no removals: repair never enqueues scrubbing it cannot reason about.
func removedSelectOptionIDs(oldOptionsJSON, newOptionsJSON, fieldType string) []int {
	if fieldType != "select" && fieldType != "multiselect" {
		return nil
	}
	oldOpts, err := models.ParseSelectOptions(oldOptionsJSON)
	if err != nil {
		return nil
	}
	newOpts, err := models.ParseSelectOptions(newOptionsJSON)
	if err != nil {
		return nil
	}
	kept := make(map[int]bool, len(newOpts.Items))
	for _, item := range newOpts.Items {
		kept[item.ID] = true
	}
	var removed []int
	for _, item := range oldOpts.Items {
		if !kept[item.ID] {
			removed = append(removed, item.ID)
		}
	}
	return removed
}

func (r *conformanceRepairer) repairStatuses(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.Statuses {
		if !entityCovers(entity, "statuses", want.Name) {
			continue
		}
		categoryID, err := r.lookupID("status_categories", "name", want.CategoryName)
		if err != nil {
			return err
		}
		if categoryID == 0 {
			return fmt.Errorf("status category %q does not exist on this instance", want.CategoryName)
		}
		id, err := r.lookupID("statuses", "name", want.Name)
		if err != nil {
			return err
		}
		if id == 0 {
			if err = r.exec(`
				INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
				VALUES (?, ?, ?, false, ?, ?)
			`, want.Name, want.Description, categoryID, r.now, r.now); err != nil {
				return fmt.Errorf("create status: %w", err)
			}
			continue
		}
		if err = r.exec(`UPDATE statuses SET description = ?, category_id = ?, updated_at = ? WHERE id = ?`,
			want.Description, categoryID, r.now, id); err != nil {
			return fmt.Errorf("restore status: %w", err)
		}
	}
	return nil
}

func (r *conformanceRepairer) repairItemTypes(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.ItemTypes {
		if !entityCovers(entity, "item_types", want.Name) {
			continue
		}
		id, err := r.lookupID("item_types", "name", want.Name)
		if err != nil {
			return err
		}
		if id == 0 {
			if err = r.exec(`
				INSERT INTO item_types (name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at)
				VALUES (?, ?, false, ?, ?, ?, ?, ?, ?)
			`, want.Name, want.Description, want.Icon, want.Color, want.HierarchyLevel, want.SortOrder, r.now, r.now); err != nil {
				return fmt.Errorf("create item type: %w", err)
			}
			continue
		}
		if err = r.exec(`
			UPDATE item_types SET description = ?, icon = ?, color = ?, hierarchy_level = ?, sort_order = ?, updated_at = ?
			WHERE id = ?
		`, want.Description, want.Icon, want.Color, want.HierarchyLevel, want.SortOrder, r.now, id); err != nil {
			return fmt.Errorf("restore item type: %w", err)
		}
	}
	return nil
}

func (r *conformanceRepairer) repairPriorities(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.Priorities {
		if !entityCovers(entity, "priorities", want.Name) {
			continue
		}
		id, err := r.lookupID("priorities", "name", want.Name)
		if err != nil {
			return err
		}
		if id == 0 {
			if err = r.exec(`
				INSERT INTO priorities (name, description, is_default, icon, color, sort_order, created_at, updated_at)
				VALUES (?, ?, false, ?, ?, ?, ?, ?)
			`, want.Name, want.Description, want.Icon, want.Color, want.SortOrder, r.now, r.now); err != nil {
				return fmt.Errorf("create priority: %w", err)
			}
			continue
		}
		if err = r.exec(`
			UPDATE priorities SET description = ?, icon = ?, color = ?, sort_order = ?, updated_at = ?
			WHERE id = ?
		`, want.Description, want.Icon, want.Color, want.SortOrder, r.now, id); err != nil {
			return fmt.Errorf("restore priority: %w", err)
		}
	}
	return nil
}

func (r *conformanceRepairer) repairLinkTypes(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.LinkTypes {
		if !entityCovers(entity, "link_types", want.Name) {
			continue
		}
		normalized := normalizeTplLinkType(want)
		id, err := r.lookupID("link_types", "name", want.Name)
		if err != nil {
			return err
		}
		if id == 0 {
			if err = r.exec(`
				INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, false, true, ?, ?, ?)
			`, want.Name, want.Description, want.ForwardLabel, want.ReverseLabel, normalized.Color,
				encodeTplEntityTypes(normalized.AllowedEntityTypes), r.now, r.now); err != nil {
				return fmt.Errorf("create link type: %w", err)
			}
			continue
		}
		var isSystem bool
		if err := r.tx.QueryRowContext(r.ctx, `SELECT is_system FROM link_types WHERE id = ?`, id).Scan(&isSystem); err != nil {
			return err
		}
		if isSystem {
			return fmt.Errorf("link type %q is a system row and cannot be modified by repair", want.Name)
		}
		if err = r.exec(`
			UPDATE link_types
			SET description = ?, forward_label = ?, reverse_label = ?, color = ?, active = true,
			    allowed_entity_types = ?, updated_at = ?
			WHERE id = ?
		`, want.Description, want.ForwardLabel, want.ReverseLabel, normalized.Color,
			encodeTplEntityTypes(normalized.AllowedEntityTypes), r.now, id); err != nil {
			return fmt.Errorf("restore link type: %w", err)
		}
	}
	return nil
}

func (r *conformanceRepairer) repairScreens(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.Screens {
		if !entityCovers(entity, "screens", want.Name) {
			continue
		}
		var screenID int
		err := r.tx.QueryRowContext(r.ctx, `
			INSERT INTO screens (name, description, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, updated_at = EXCLUDED.updated_at
			RETURNING id
		`, want.Name, want.Description, r.now, r.now).Scan(&screenID)
		if err != nil {
			return fmt.Errorf("restore screen: %w", err)
		}
		// Replace the field surface wholesale — screen rows are presentation
		// configuration with no item data behind them.
		if err := r.exec(`DELETE FROM screen_fields WHERE screen_id = ?`, screenID); err != nil {
			return fmt.Errorf("restore screen fields: %w", err)
		}
		for _, f := range want.Fields {
			ident := f.FieldIdentifier
			if f.FieldKind == "custom" {
				cfID, err := r.lookupID("custom_field_definitions", "name", f.CustomFieldName)
				if err != nil {
					return err
				}
				if cfID == 0 {
					return fmt.Errorf("custom field %q not found", f.CustomFieldName)
				}
				ident = strconv.Itoa(cfID)
			}
			if err := r.exec(`
				INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width, placement)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, screenID, f.FieldKind, ident, f.DisplayOrder, f.IsRequired, f.FieldWidth, models.NormalizeScreenFieldPlacement(f.Placement)); err != nil {
				return fmt.Errorf("restore screen fields: %w", err)
			}
		}
		if err := r.exec(`DELETE FROM screen_system_fields WHERE screen_id = ?`, screenID); err != nil {
			return fmt.Errorf("restore screen system fields: %w", err)
		}
		for _, sf := range want.SystemFields {
			if err := r.exec(`INSERT INTO screen_system_fields (screen_id, field_name) VALUES (?, ?)`, screenID, sf); err != nil {
				return fmt.Errorf("restore screen system fields: %w", err)
			}
		}
	}
	return nil
}

// repairWorkflows restores workflow definitions and transitions. Existing
// transitions are updated IN PLACE (keyed by from/to/from_all) — deleting a
// transition would cascade away the condition bindings and approval statuses
// that reference it, so extra transitions are left in place.
func (r *conformanceRepairer) repairWorkflows(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.Workflows {
		if !entityCovers(entity, "workflows", want.Name) {
			continue
		}
		workflowID := r.workflowIDsByName[lowerStr(want.Name)]
		if workflowID == 0 {
			if err := r.tx.QueryRowContext(r.ctx, `
				INSERT INTO workflows (name, description, is_default, created_at, updated_at)
				VALUES (?, ?, false, ?, ?) RETURNING id
			`, want.Name, want.Description, r.now, r.now).Scan(&workflowID); err != nil {
				return fmt.Errorf("restore workflow: %w", err)
			}
		} else if err := r.exec(`UPDATE workflows SET description = ?, updated_at = ? WHERE id = ?`,
			want.Description, r.now, workflowID); err != nil {
			return fmt.Errorf("restore workflow: %w", err)
		}
		for _, t := range want.Transitions {
			toID, err := r.lookupID("statuses", "name", t.ToStatusName)
			if err != nil {
				return err
			}
			if toID == 0 {
				return fmt.Errorf("transition target status %q does not exist", t.ToStatusName)
			}
			var fromID any
			if t.FromStatusName != nil && !t.FromAllStatuses {
				id, err := r.lookupID("statuses", "name", *t.FromStatusName)
				if err != nil {
					return err
				}
				if id == 0 {
					return fmt.Errorf("transition source status %q does not exist", *t.FromStatusName)
				}
				fromID = id
			}
			res, err := r.tx.ExecContext(r.ctx, `
				UPDATE workflow_transitions
				SET from_all_statuses = ?, display_order = ?, source_handle = ?, target_handle = ?
				WHERE workflow_id = ? AND COALESCE(from_status_id, -1) = COALESCE(?, -1) AND to_status_id = ?
			`, t.FromAllStatuses, t.DisplayOrder, t.SourceHandle, t.TargetHandle, workflowID, fromID, toID)
			if err != nil {
				return fmt.Errorf("restore workflow transitions: %w", err)
			}
			if rows, _ := res.RowsAffected(); rows == 0 {
				if err := r.exec(`
					INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, from_all_statuses, display_order, source_handle, target_handle, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, workflowID, fromID, toID, t.FromAllStatuses, t.DisplayOrder, t.SourceHandle, t.TargetHandle, r.now); err != nil {
					return fmt.Errorf("restore workflow transitions: %w", err)
				}
			}
		}
	}
	return nil
}

func (r *conformanceRepairer) repairConditionSets(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.ConditionSets {
		if !entityCovers(entity, "condition_sets", want.Name) {
			continue
		}
		workflowID, err := r.lookupID("workflows", "name", want.WorkflowName)
		if err != nil {
			return err
		}
		if workflowID == 0 {
			return fmt.Errorf("workflow %q does not exist", want.WorkflowName)
		}
		var setID int
		setID, err = r.lookupID("condition_sets", "name", want.Name)
		if err != nil {
			return err
		}
		if setID == 0 {
			if err := r.tx.QueryRowContext(r.ctx, `
				INSERT INTO condition_sets (name, description, workflow_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?) RETURNING id
			`, want.Name, want.Description, workflowID, r.now, r.now).Scan(&setID); err != nil {
				return fmt.Errorf("restore condition set: %w", err)
			}
		} else if err := r.exec(`UPDATE condition_sets SET description = ?, workflow_id = ?, updated_at = ? WHERE id = ?`,
			want.Description, workflowID, r.now, setID); err != nil {
			return fmt.Errorf("restore condition set: %w", err)
		}
		// Replace all bindings wholesale; conditions cascade with their
		// binding. Bindings are condition configuration only.
		if err := r.exec(`DELETE FROM condition_set_transitions WHERE condition_set_id = ?`, setID); err != nil {
			return fmt.Errorf("restore condition set bindings: %w", err)
		}
		for _, tc := range want.TransitionConditions {
			transitionID, err := r.resolveTransitionID(workflowID, tc)
			if err != nil {
				return err
			}
			if transitionID == 0 {
				return fmt.Errorf("transition %s does not exist in workflow %q", conditionBindingPath(tc), want.WorkflowName)
			}
			mode := tc.LogicMode
			if mode == "" {
				mode = "and"
			}
			var cstID int
			if err := r.tx.QueryRowContext(r.ctx, `
				INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode, created_at)
				VALUES (?, ?, ?, ?) RETURNING id
			`, setID, transitionID, mode, r.now).Scan(&cstID); err != nil {
				return fmt.Errorf("restore condition set bindings: %w", err)
			}
			for _, c := range tc.Conditions {
				cfg := r.rewriteConditionConfig(c)
				if err := validateConditionMap(c.Type, c.Mode, cfg); err != nil {
					return fmt.Errorf("restore conditions: %w", err)
				}
				cfgBytes, err := json.Marshal(cfg)
				if err != nil {
					return fmt.Errorf("restore conditions: %w", err)
				}
				condMode := c.Mode
				if condMode == "" {
					condMode = models.ConditionModeCondition
				}
				var errMsg any
				if c.ErrorMessage != "" {
					errMsg = c.ErrorMessage
				}
				if err := r.exec(`
					INSERT INTO conditions (condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`, cstID, c.Type, string(cfgBytes), c.DisplayOrder, condMode, errMsg, r.now); err != nil {
					return fmt.Errorf("restore conditions: %w", err)
				}
			}
		}
	}
	return nil
}

// resolveTransitionID finds a workflow transition by (from, to, from_all).
func (r *conformanceRepairer) resolveTransitionID(workflowID int, tc ConfigSetTplTransitionCondition) (int, error) {
	ref := ConfigSetTplTransitionRef{FromStatusName: tc.FromStatusName, ToStatusName: tc.ToStatusName, FromAllStatuses: tc.FromAllStatuses}
	return r.transitionIDForRef(workflowID, ref)
}

func (r *conformanceRepairer) transitionIDForRef(workflowID int, ref ConfigSetTplTransitionRef) (int, error) {
	toID, err := r.lookupID("statuses", "name", ref.ToStatusName)
	if err != nil || toID == 0 {
		return 0, err
	}
	var fromID any
	if ref.FromStatusName != nil && !ref.FromAllStatuses {
		id, err := r.lookupID("statuses", "name", *ref.FromStatusName)
		if err != nil || id == 0 {
			return 0, err
		}
		fromID = id
	}
	var transitionID int
	err = r.tx.QueryRowContext(r.ctx, `
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ? AND COALESCE(from_status_id, -1) = COALESCE(?, -1) AND to_status_id = ? AND from_all_statuses = ?
	`, workflowID, fromID, toID, ref.FromAllStatuses).Scan(&transitionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return transitionID, err
}

// rewriteConditionConfig turns template name references back into ids, the
// same substitution the import flow performs.
func (r *conformanceRepairer) rewriteConditionConfig(c ConfigSetTplCondition) map[string]any {
	out := make(map[string]any, len(c.Config))
	for k, v := range c.Config {
		out[k] = v
	}
	rewrite := func(nameKey, idKey, table, column string) {
		if name, ok := out[nameKey].(string); ok && name != "" {
			if id, _ := r.lookupID(table, column, name); id > 0 {
				out[idKey] = id
			}
			delete(out, nameKey)
		}
	}
	// Mirrors rewriteFieldRefForImport: a custom_field user reference carries
	// custom_field_name in the template and field_id in the live config.
	rewriteCustomFieldRef := func() {
		if source, _ := out["source"].(string); source != "custom_field" {
			return
		}
		if name, ok := out["custom_field_name"].(string); ok && name != "" {
			if id, _ := r.lookupID("custom_field_definitions", "name", name); id > 0 {
				out["field_id"] = id
			}
			delete(out, "custom_field_name")
		}
	}
	switch c.Type {
	case models.ConditionTypeUserInRole:
		rewrite("role_name", "role_id", "workspace_roles", "name")
		rewriteCustomFieldRef()
	case models.ConditionTypeUserInGroup:
		rewrite("group_name", "group_id", "groups", "group_name")
		rewriteCustomFieldRef()
	case models.ConditionTypeUserIs:
		rewriteCustomFieldRef()
	case models.ConditionTypeFieldValue:
		if name, ok := out["custom_field_name"].(string); ok && name != "" {
			if id, _ := r.lookupID("custom_field_definitions", "name", name); id > 0 {
				out["field_identifier"] = strconv.Itoa(id)
			}
			delete(out, "custom_field_name")
		}
	}
	return out
}

func (r *conformanceRepairer) repairApprovalSets(entity *conformanceEntityPlan) error {
	for _, want := range r.canonical.Payload.ApprovalSets {
		if !entityCovers(entity, "approval_sets", want.Name) {
			continue
		}
		workflowID, err := r.lookupID("workflows", "name", want.WorkflowName)
		if err != nil {
			return err
		}
		if workflowID == 0 {
			return fmt.Errorf("workflow %q does not exist", want.WorkflowName)
		}
		var setID int
		setID, err = r.lookupID("approval_sets", "name", want.Name)
		if err != nil {
			return err
		}
		if setID == 0 {
			if err := r.tx.QueryRowContext(r.ctx, `
				INSERT INTO approval_sets (name, description, workflow_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?) RETURNING id
			`, want.Name, want.Description, workflowID, r.now, r.now).Scan(&setID); err != nil {
				return fmt.Errorf("restore approval set: %w", err)
			}
		} else if err := r.exec(`UPDATE approval_sets SET description = ?, workflow_id = ?, updated_at = ? WHERE id = ?`,
			want.Description, workflowID, r.now, setID); err != nil {
			return fmt.Errorf("restore approval set: %w", err)
		}
		// Replace all status bindings wholesale; steps cascade with their
		// binding. A live pending approval request (RESTRICT) fails the whole
		// entity via its savepoint and is reported, not hidden.
		if err := r.exec(`DELETE FROM approval_set_statuses WHERE approval_set_id = ?`, setID); err != nil {
			return fmt.Errorf("restore approval statuses: %w", err)
		}
		for _, ss := range want.SetStatuses {
			statusID, err := r.lookupID("statuses", "name", ss.StatusName)
			if err != nil {
				return err
			}
			if statusID == 0 {
				return fmt.Errorf("status %q does not exist", ss.StatusName)
			}
			approveTID, err := r.transitionIDForRef(workflowID, ss.ApproveTransition)
			if err != nil {
				return err
			}
			denyTID, err := r.transitionIDForRef(workflowID, ss.DenyTransition)
			if err != nil {
				return err
			}
			if approveTID == 0 || denyTID == 0 {
				return fmt.Errorf("approve/deny transition for status %q does not exist in workflow %q", ss.StatusName, want.WorkflowName)
			}
			stepMode := ss.StepMode
			if stepMode == "" {
				stepMode = models.ApprovalStepModeSequential
			}
			var assID int
			if err := r.tx.QueryRowContext(r.ctx, `
				INSERT INTO approval_set_statuses (approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at)
				VALUES (?, ?, ?, ?, ?, ?) RETURNING id
			`, setID, statusID, approveTID, denyTID, stepMode, r.now).Scan(&assID); err != nil {
				return fmt.Errorf("restore approval statuses: %w", err)
			}
			for _, step := range ss.Steps {
				if err := r.insertApprovalStep(assID, step); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *conformanceRepairer) insertApprovalStep(assID int, step ConfigSetTplApprovalStep) error {
	resolve := func(name, table, column string) any {
		if name == "" {
			return nil
		}
		id, _ := r.lookupID(table, column, name)
		return nullIfZero(id)
	}
	fieldID := func(name string) any {
		if name == "" {
			return nil
		}
		id, _ := r.lookupID("custom_field_definitions", "name", name)
		return nullIfZero(id)
	}
	quorumMode := step.QuorumMode
	if quorumMode == "" {
		quorumMode = models.ApprovalQuorumModeAny
	}
	rejectionPolicy := step.RejectionPolicy
	if rejectionPolicy == "" {
		rejectionPolicy = models.ApprovalRejectionPolicyAnyFails
	}
	onLeave := step.OnLeaveStrategy
	if onLeave == "" {
		onLeave = models.ApprovalOnLeaveUseSubstitute
	}
	strOrNull := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	err := r.exec(`
		INSERT INTO approval_steps
			(approval_set_status_id, display_order, name,
			 quorum_mode, quorum_count, quorum_percent, rejection_policy,
			 approver_source, approver_field_identifier, approver_field_id,
			 approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
			 on_leave_strategy,
			 escalation_after_hours, escalation_action, escalation_target_source,
			 escalation_target_field_identifier, escalation_target_field_id,
			 escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
			 max_escalations, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		assID, step.DisplayOrder, step.Name,
		quorumMode, step.QuorumCount, step.QuorumPercent, rejectionPolicy,
		step.ApproverSource, strOrNull(step.ApproverFieldIdentifier), fieldID(step.ApproverCustomFieldName),
		resolve(step.ApproverRoleName, "workspace_roles", "name"),
		resolve(step.ApproverGroupName, "groups", "group_name"),
		resolve(step.ApproverUserEmail, "users", "email"),
		step.AllowSelfApproval,
		onLeave,
		step.EscalationAfterHours, strOrNull(step.EscalationAction), strOrNull(step.EscalationTargetSource),
		strOrNull(step.EscalationTargetFieldIdentifier), fieldID(step.EscalationTargetCustomFieldName),
		resolve(step.EscalationTargetRoleName, "workspace_roles", "name"),
		resolve(step.EscalationTargetGroupName, "groups", "group_name"),
		resolve(step.EscalationTargetUserEmail, "users", "email"),
		step.MaxEscalations, r.now,
	)
	return err
}

func (r *conformanceRepairer) repairLinks(configSetID int) error {
	c := r.canonical.Payload
	workflowID, err := r.lookupID("workflows", "name", c.Links.WorkflowName)
	if err != nil {
		return err
	}
	conditionSetID, err := r.lookupID("condition_sets", "name", c.Links.ConditionSetName)
	if err != nil {
		return err
	}
	approvalSetID, err := r.lookupID("approval_sets", "name", c.Links.ApprovalSetName)
	if err != nil {
		return err
	}
	defaultItemTypeID, err := r.lookupID("item_types", "name", c.ConfigurationSet.DefaultItemTypeName)
	if err != nil {
		return err
	}
	createScreenID, err := r.lookupID("screens", "name", c.Links.CreateScreenName)
	if err != nil {
		return err
	}
	editScreenID, err := r.lookupID("screens", "name", c.Links.EditScreenName)
	if err != nil {
		return err
	}
	viewScreenID, err := r.lookupID("screens", "name", c.Links.ViewScreenName)
	if err != nil {
		return err
	}
	if err := r.exec(`
		UPDATE configuration_sets
		SET workflow_id = ?, condition_set_id = ?, approval_set_id = ?, default_item_type_id = ?,
		    create_screen_id = ?, edit_screen_id = ?, view_screen_id = ?
		WHERE id = ?
	`, nullIfZero(workflowID), nullIfZero(conditionSetID), nullIfZero(approvalSetID), nullIfZero(defaultItemTypeID),
		nullIfZero(createScreenID), nullIfZero(editScreenID), nullIfZero(viewScreenID), configSetID); err != nil {
		return fmt.Errorf("restore configuration set wiring: %w", err)
	}

	// Replace priority assignments to match the template exactly. Assignments
	// are junction rows; items keep their priority values.
	if err := r.exec(`DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?`, configSetID); err != nil {
		return fmt.Errorf("restore priority assignments: %w", err)
	}
	for _, name := range c.Links.PriorityNames {
		priorityID, err := r.lookupID("priorities", "name", name)
		if err != nil {
			return err
		}
		if priorityID == 0 {
			return fmt.Errorf("priority %q does not exist", name)
		}
		if err := r.exec(`INSERT INTO configuration_set_priorities (configuration_set_id, priority_id) VALUES (?, ?)`, configSetID, priorityID); err != nil {
			return fmt.Errorf("restore priority assignments: %w", err)
		}
	}

	// Replace per-item-type configs to match the template exactly.
	if err := r.exec(`DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?`, configSetID); err != nil {
		return fmt.Errorf("restore item type configs: %w", err)
	}
	for _, itc := range c.Links.ItemTypeConfigs {
		typeID, err := r.lookupID("item_types", "name", itc.ItemTypeName)
		if err != nil {
			return err
		}
		if typeID == 0 {
			return fmt.Errorf("item type %q does not exist", itc.ItemTypeName)
		}
		icWorkflowID, _ := r.lookupID("workflows", "name", itc.WorkflowName)
		icConditionSetID, _ := r.lookupID("condition_sets", "name", itc.ConditionSetName)
		icApprovalSetID, _ := r.lookupID("approval_sets", "name", itc.ApprovalSetName)
		icCreateScreenID, _ := r.lookupID("screens", "name", itc.CreateScreenName)
		icEditScreenID, _ := r.lookupID("screens", "name", itc.EditScreenName)
		icViewScreenID, _ := r.lookupID("screens", "name", itc.ViewScreenName)
		if err := r.exec(`
			INSERT INTO configuration_set_item_types
				(configuration_set_id, item_type_id, workflow_id, condition_set_id, approval_set_id,
				 create_screen_id, edit_screen_id, view_screen_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, configSetID, typeID, nullIfZero(icWorkflowID), nullIfZero(icConditionSetID), nullIfZero(icApprovalSetID),
			nullIfZero(icCreateScreenID), nullIfZero(icEditScreenID), nullIfZero(icViewScreenID)); err != nil {
			return fmt.Errorf("restore item type configs: %w", err)
		}
	}
	return nil
}

// entityCovers reports whether the plan for this entity includes the given
// canonical entity name. Entity names are compared whole: a name may itself
// contain the drift-path separator, so no path stripping happens here.
func entityCovers(entity *conformanceEntityPlan, section, entityName string) bool {
	if entity.section != section {
		return false
	}
	return strings.EqualFold(entityName, entity.name)
}

// resolveConformanceEntityName maps a drift row name ("<entity>/<path...>") to
// the canonical entity it belongs to. Entity names may contain the drift-path
// separator, so the longest canonical name that is either an exact match or a
// path prefix wins. Rows whose entity only exists live (not in the canonical
// template) fall back to their first path segment.
func resolveConformanceEntityName(canonical *ConfigSetTemplate, section, driftName string) string {
	best := ""
	for _, name := range conformanceEntityNames(canonical, section) {
		if driftName != name && !strings.HasPrefix(driftName, name+"/") {
			continue
		}
		if len(name) > len(best) {
			best = name
		}
	}
	if best != "" {
		return best
	}
	if idx := strings.Index(driftName, "/"); idx >= 0 {
		return driftName[:idx]
	}
	return driftName
}

// conformanceEntityNames lists the canonical entity names for one section.
func conformanceEntityNames(canonical *ConfigSetTemplate, section string) []string {
	if canonical == nil {
		return nil
	}
	p := &canonical.Payload
	switch section {
	case "custom_fields":
		names := make([]string, 0, len(p.CustomFields))
		for _, e := range p.CustomFields {
			names = append(names, e.Name)
		}
		return names
	case "statuses":
		names := make([]string, 0, len(p.Statuses))
		for _, e := range p.Statuses {
			names = append(names, e.Name)
		}
		return names
	case "item_types":
		names := make([]string, 0, len(p.ItemTypes))
		for _, e := range p.ItemTypes {
			names = append(names, e.Name)
		}
		return names
	case "priorities":
		names := make([]string, 0, len(p.Priorities))
		for _, e := range p.Priorities {
			names = append(names, e.Name)
		}
		return names
	case "link_types":
		names := make([]string, 0, len(p.LinkTypes))
		for _, e := range p.LinkTypes {
			names = append(names, e.Name)
		}
		return names
	case "screens":
		names := make([]string, 0, len(p.Screens))
		for _, e := range p.Screens {
			names = append(names, e.Name)
		}
		return names
	case "workflows":
		names := make([]string, 0, len(p.Workflows))
		for _, e := range p.Workflows {
			names = append(names, e.Name)
		}
		return names
	case "condition_sets":
		names := make([]string, 0, len(p.ConditionSets))
		for _, e := range p.ConditionSets {
			names = append(names, e.Name)
		}
		return names
	case "approval_sets":
		names := make([]string, 0, len(p.ApprovalSets))
		for _, e := range p.ApprovalSets {
			names = append(names, e.Name)
		}
		return names
	}
	return nil
}

// referencedWorkflowIDsByName returns the workflows the addressed configuration
// set references (its primary workflow and every item-type overlay workflow),
// keyed by lowercased name. Repair uses this instead of a global name lookup so
// a same-named workflow owned by another set is never modified.
func (s *ConfigSetConformanceService) referencedWorkflowIDsByName(ctx context.Context, configSetID int) (map[string]int, error) {
	out := map[string]int{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT wf.id, wf.name
		FROM workflows wf
		WHERE wf.id IN (
			SELECT workflow_id FROM configuration_sets WHERE id = ? AND workflow_id IS NOT NULL
			UNION
			SELECT workflow_id FROM configuration_set_item_types WHERE configuration_set_id = ? AND workflow_id IS NOT NULL
		)
	`, configSetID, configSetID)
	if err != nil {
		return nil, fmt.Errorf("load referenced workflows for set %d: %w", configSetID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan referenced workflow: %w", err)
		}
		out[lowerStr(name)] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate referenced workflows: %w", err)
	}
	return out, nil
}

func nullIfZero(id int) any {
	if id == 0 {
		return nil
	}
	return id
}
