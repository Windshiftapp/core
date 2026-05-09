package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ConfigSetImportService applies a ConfigSetTemplate bundle to the current
// instance, creating a fresh configuration set (and any missing global
// shared entities) under one DB transaction.
//
// Resolution policy:
//   - Statuses, item types, priorities, custom fields, status categories,
//     screens: match by name (case-insensitive) and create if missing.
//   - Workflow, condition set, approval set, configuration set: always
//     created fresh with the bundle's names (DB does not enforce uniqueness
//     on these except screens, where reuse-by-name is intentional).
//   - Status categories, roles, groups, users: must already exist on the
//     target. Missing references surface as ErrUnresolvedReferences before
//     any write happens.
type ConfigSetImportService struct {
	db   database.Database
	repo *repository.ConfigurationSetRepository
}

func NewConfigSetImportService(db database.Database, repo *repository.ConfigurationSetRepository) *ConfigSetImportService {
	return &ConfigSetImportService{db: db, repo: repo}
}

// Import applies the given template. Returns the new configuration set ID,
// any non-fatal warnings, and an error. On ErrUnresolvedReferences /
// ErrDefaultEntityConflict the transaction has not been started; on any
// other error it has been rolled back.
func (s *ConfigSetImportService) Import(ctx context.Context, tpl *ConfigSetTemplate) (configSetID int, warnings []string, err error) {
	if err := s.validateEnvelope(tpl); err != nil {
		return 0, nil, err
	}

	// Default-entity conflicts are a structural rejection — surface them
	// before walking identity refs so the user gets the simpler "this can't
	// land here at all" signal first instead of mixing two failure modes.
	if err := s.validateNoDefaultCollisions(ctx, tpl); err != nil {
		return 0, nil, err
	}

	if err := s.validateReferences(ctx, tpl); err != nil {
		return 0, nil, err
	}

	err = database.WithTx(s.db, func(tx database.Tx) error {
		id, w, err := s.apply(ctx, tx, tpl)
		if err != nil {
			return err
		}
		configSetID = id
		warnings = w
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return configSetID, warnings, nil
}

func (s *ConfigSetImportService) validateEnvelope(tpl *ConfigSetTemplate) error {
	if tpl == nil {
		return errors.New("import: empty template")
	}
	if tpl.Kind != ConfigSetTemplateKind {
		return fmt.Errorf("import: unsupported kind %q (want %q)", tpl.Kind, ConfigSetTemplateKind)
	}
	if tpl.SchemaVersion != ConfigSetTemplateSchemaVersion {
		return fmt.Errorf("import: unsupported schema_version %d (want %d)", tpl.SchemaVersion, ConfigSetTemplateSchemaVersion)
	}
	if tpl.Payload.ConfigurationSet.Name == "" {
		return errors.New("import: configuration_set.name is required")
	}
	return nil
}

// validateNoDefaultCollisions refuses imports whose top-level config set
// or any embedded workflow shares a name (case-insensitive) with an existing
// `is_default = true` row. Statuses / item types / priorities / custom
// fields are excluded — those reuse-by-name and never duplicate, so a
// collision there is benign.
func (s *ConfigSetImportService) validateNoDefaultCollisions(ctx context.Context, tpl *ConfigSetTemplate) error {
	var conflicts []DefaultConflict

	if name := tpl.Payload.ConfigurationSet.Name; name != "" {
		isDefault, err := s.nameMatchesDefault(ctx, "configuration_sets", "name", name)
		if err != nil {
			return err
		}
		if isDefault {
			conflicts = append(conflicts, DefaultConflict{Kind: DefaultConflictConfigurationSet, Name: name})
		}
	}
	for _, wf := range tpl.Payload.Workflows {
		if wf.Name == "" {
			continue
		}
		isDefault, err := s.nameMatchesDefault(ctx, "workflows", "name", wf.Name)
		if err != nil {
			return err
		}
		if isDefault {
			conflicts = append(conflicts, DefaultConflict{Kind: DefaultConflictWorkflow, Name: wf.Name})
		}
	}

	if len(conflicts) == 0 {
		return nil
	}
	return &ErrDefaultEntityConflict{Conflicts: conflicts}
}

// nameMatchesDefault returns true when at least one row in `table` has
// `nameCol` equal (case-insensitive) to the supplied name AND `is_default = true`.
// Restricted to caller-supplied tables: configuration_sets, workflows.
func (s *ConfigSetImportService) nameMatchesDefault(ctx context.Context, table, nameCol, name string) (bool, error) {
	switch table {
	case "configuration_sets", "workflows":
		// allow-list — must match a literal switch case to avoid SQL injection
		// even though the values are not user-controlled.
	default:
		return false, fmt.Errorf("nameMatchesDefault: refusing untrusted table %q", table)
	}
	q := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE LOWER(%s) = LOWER(?) AND is_default = true)`, table, nameCol)
	var exists bool
	if err := s.db.QueryRowContext(ctx, q, name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// validateReferences walks the template and confirms every identity ref
// (status category, role, group, user) is resolvable on the target. Returns
// ErrUnresolvedReferences if any are missing. No writes.
func (s *ConfigSetImportService) validateReferences(ctx context.Context, tpl *ConfigSetTemplate) error {
	var missing []UnresolvedRef

	// Status categories — must be present.
	for _, cat := range tpl.Payload.StatusCategories {
		if id, _ := s.lookupStatusCategoryID(ctx, cat.Name); id == 0 {
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindStatusCategory, Name: cat.Name,
				Path: "status_categories",
			})
		}
	}
	// Statuses also reference categories — collect any not already in the
	// status_categories block.
	seenCatNames := map[string]struct{}{}
	for _, c := range tpl.Payload.StatusCategories {
		seenCatNames[lowerStr(c.Name)] = struct{}{}
	}
	for _, st := range tpl.Payload.Statuses {
		if st.CategoryName == "" {
			continue
		}
		if _, ok := seenCatNames[lowerStr(st.CategoryName)]; ok {
			continue
		}
		if id, _ := s.lookupStatusCategoryID(ctx, st.CategoryName); id == 0 {
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindStatusCategory, Name: st.CategoryName,
				Path: fmt.Sprintf("statuses/%s", st.Name),
			})
		}
	}

	// Conditions: roles / groups (custom fields are auto-created from the
	// embedded definitions; users do not appear here).
	for _, set := range tpl.Payload.ConditionSets {
		for ti, tc := range set.TransitionConditions {
			for ci, c := range tc.Conditions {
				switch c.Type {
				case models.ConditionTypeUserInRole:
					if name, _ := c.Config["role_name"].(string); name != "" {
						if id, _ := s.lookupRoleID(ctx, name); id == 0 {
							missing = append(missing, UnresolvedRef{
								Kind: UnresolvedKindRole, Name: name,
								Path: fmt.Sprintf("condition_sets/%s/transition_conditions/%d/conditions/%d", set.Name, ti, ci),
							})
						}
					}
				case models.ConditionTypeUserInGroup:
					if name, _ := c.Config["group_name"].(string); name != "" {
						if id, _ := s.lookupGroupID(ctx, name); id == 0 {
							missing = append(missing, UnresolvedRef{
								Kind: UnresolvedKindGroup, Name: name,
								Path: fmt.Sprintf("condition_sets/%s/transition_conditions/%d/conditions/%d", set.Name, ti, ci),
							})
						}
					}
				}
			}
		}
	}

	// Approval steps: roles / groups / users.
	for _, set := range tpl.Payload.ApprovalSets {
		for si, ss := range set.SetStatuses {
			for stepi, step := range ss.Steps {
				path := fmt.Sprintf("approval_sets/%s/set_statuses/%d/steps/%d", set.Name, si, stepi)
				if step.ApproverRoleName != "" {
					if id, _ := s.lookupRoleID(ctx, step.ApproverRoleName); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindRole, Name: step.ApproverRoleName,
							Path: path + ".approver",
						})
					}
				}
				if step.ApproverGroupName != "" {
					if id, _ := s.lookupGroupID(ctx, step.ApproverGroupName); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindGroup, Name: step.ApproverGroupName,
							Path: path + ".approver",
						})
					}
				}
				if step.ApproverUserEmail != "" {
					if id, _ := s.lookupUserID(ctx, step.ApproverUserEmail); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindUser, Email: step.ApproverUserEmail,
							Path: path + ".approver",
						})
					}
				}
				if step.EscalationTargetRoleName != "" {
					if id, _ := s.lookupRoleID(ctx, step.EscalationTargetRoleName); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindRole, Name: step.EscalationTargetRoleName,
							Path: path + ".escalation_target",
						})
					}
				}
				if step.EscalationTargetGroupName != "" {
					if id, _ := s.lookupGroupID(ctx, step.EscalationTargetGroupName); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindGroup, Name: step.EscalationTargetGroupName,
							Path: path + ".escalation_target",
						})
					}
				}
				if step.EscalationTargetUserEmail != "" {
					if id, _ := s.lookupUserID(ctx, step.EscalationTargetUserEmail); id == 0 {
						missing = append(missing, UnresolvedRef{
							Kind: UnresolvedKindUser, Email: step.EscalationTargetUserEmail,
							Path: path + ".escalation_target",
						})
					}
				}
			}
		}
	}

	if len(missing) == 0 {
		return nil
	}
	return &ErrUnresolvedReferences{Items: missing}
}

// apply runs the strict-order creation flow inside a single transaction.
// Helper maps are populated as each section completes so later sections can
// rewrite name references back to the IDs minted earlier.
func (s *ConfigSetImportService) apply(ctx context.Context, tx database.Tx, tpl *ConfigSetTemplate) (configSetID int, warnings []string, err error) {
	now := time.Now()

	// 1. Custom fields (must lead — referenced by screens, conditions, approvals).
	customFieldNameToID := map[string]int{}
	for _, cf := range tpl.Payload.CustomFields {
		id, err := s.findOrCreateCustomField(ctx, tx, cf, now)
		if err != nil {
			return 0, nil, fmt.Errorf("custom_field %q: %w", cf.Name, err)
		}
		customFieldNameToID[lowerStr(cf.Name)] = id
	}

	// 2. Status categories — already validated to exist; build name → id map.
	categoryNameToID := map[string]int{}
	for _, cat := range tpl.Payload.StatusCategories {
		id, err := s.lookupStatusCategoryID(ctx, cat.Name)
		if err != nil || id == 0 {
			return 0, nil, fmt.Errorf("status_category %q: not found (validation race)", cat.Name)
		}
		categoryNameToID[lowerStr(cat.Name)] = id
	}

	// 3. Statuses — match by name, create if missing. Resolve category by name.
	statusNameToID := map[string]int{}
	for _, st := range tpl.Payload.Statuses {
		id, err := s.findOrCreateStatus(ctx, tx, st, categoryNameToID, now)
		if err != nil {
			return 0, nil, fmt.Errorf("status %q: %w", st.Name, err)
		}
		statusNameToID[lowerStr(st.Name)] = id
	}

	// 4. Item types — match by name, create if missing.
	itemTypeNameToID := map[string]int{}
	for _, it := range tpl.Payload.ItemTypes {
		id, err := s.findOrCreateItemType(ctx, tx, it, now)
		if err != nil {
			return 0, nil, fmt.Errorf("item_type %q: %w", it.Name, err)
		}
		itemTypeNameToID[lowerStr(it.Name)] = id
	}

	// 5. Priorities — match by name, create if missing.
	priorityNameToID := map[string]int{}
	for _, p := range tpl.Payload.Priorities {
		id, err := s.findOrCreatePriority(ctx, tx, p, now)
		if err != nil {
			return 0, nil, fmt.Errorf("priority %q: %w", p.Name, err)
		}
		priorityNameToID[lowerStr(p.Name)] = id
	}

	// 6. Screens — match by name (DB has UNIQUE(name)). Reuse if found;
	//    otherwise create with rewritten custom-field identifiers.
	screenNameToID := map[string]int{}
	for _, sc := range tpl.Payload.Screens {
		id, reused, err := s.findOrCreateScreen(ctx, tx, sc, customFieldNameToID, now)
		if err != nil {
			return 0, nil, fmt.Errorf("screen %q: %w", sc.Name, err)
		}
		screenNameToID[lowerStr(sc.Name)] = id
		if reused {
			warnings = append(warnings, fmt.Sprintf("screen %q already exists by name; reusing existing definition", sc.Name))
		}
	}

	// 7. Workflows — always created fresh. Build a (workflowName, fromName, toName)
	//    → transition_id map for later condition/approval references.
	type transitionKey struct {
		workflowName string
		fromName     string // empty for initial transitions
		toName       string
	}
	workflowNameToID := map[string]int{}
	transitionKeyToID := map[transitionKey]int{}
	for _, wf := range tpl.Payload.Workflows {
		wfID, err := s.createWorkflow(ctx, tx, wf, now)
		if err != nil {
			return 0, nil, fmt.Errorf("workflow %q: %w", wf.Name, err)
		}
		workflowNameToID[lowerStr(wf.Name)] = wfID
		for _, t := range wf.Transitions {
			toID, ok := statusNameToID[lowerStr(t.ToStatusName)]
			if !ok {
				return 0, nil, fmt.Errorf("workflow %q: transition references unknown to_status %q", wf.Name, t.ToStatusName)
			}
			var fromID *int
			fromName := ""
			if t.FromStatusName != nil {
				fid, ok := statusNameToID[lowerStr(*t.FromStatusName)]
				if !ok {
					return 0, nil, fmt.Errorf("workflow %q: transition references unknown from_status %q", wf.Name, *t.FromStatusName)
				}
				fromID = &fid
				fromName = *t.FromStatusName
			}
			tID, err := s.createWorkflowTransition(ctx, tx, wfID, fromID, toID, t, now)
			if err != nil {
				return 0, nil, fmt.Errorf("workflow %q: insert transition: %w", wf.Name, err)
			}
			transitionKeyToID[transitionKey{lowerStr(wf.Name), lowerStr(fromName), lowerStr(t.ToStatusName)}] = tID
		}
	}

	// 8. Condition sets — always created fresh; resolve transition IDs and
	//    rewrite role/group/custom-field references.
	conditionSetNameToID := map[string]int{}
	for _, set := range tpl.Payload.ConditionSets {
		csID, err := s.createConditionSet(ctx, tx, set, workflowNameToID, now)
		if err != nil {
			return 0, nil, fmt.Errorf("condition_set %q: %w", set.Name, err)
		}
		conditionSetNameToID[lowerStr(set.Name)] = csID

		for _, tc := range set.TransitionConditions {
			fromName := ""
			if tc.FromStatusName != nil {
				fromName = *tc.FromStatusName
			}
			tID, ok := transitionKeyToID[transitionKey{lowerStr(set.WorkflowName), lowerStr(fromName), lowerStr(tc.ToStatusName)}]
			if !ok {
				return 0, nil, fmt.Errorf("condition_set %q: transition (%s → %s) not found in workflow %q", set.Name, displayFrom(tc.FromStatusName), tc.ToStatusName, set.WorkflowName)
			}
			cstID, err := s.createConditionSetTransition(ctx, tx, csID, tID, tc.LogicMode, now)
			if err != nil {
				return 0, nil, fmt.Errorf("condition_set %q: insert transition: %w", set.Name, err)
			}
			for _, c := range tc.Conditions {
				cfg := s.rewriteConditionConfigForImport(c.Type, c.Config, customFieldNameToID, ctx)
				if err := s.createCondition(ctx, tx, cstID, c, cfg, now); err != nil {
					return 0, nil, fmt.Errorf("condition_set %q: insert condition: %w", set.Name, err)
				}
			}
		}
	}

	// 9. Approval sets — always created fresh.
	approvalSetNameToID := map[string]int{}
	for _, set := range tpl.Payload.ApprovalSets {
		asID, err := s.createApprovalSet(ctx, tx, set, workflowNameToID, now)
		if err != nil {
			return 0, nil, fmt.Errorf("approval_set %q: %w", set.Name, err)
		}
		approvalSetNameToID[lowerStr(set.Name)] = asID

		for _, ss := range set.SetStatuses {
			statusID, ok := statusNameToID[lowerStr(ss.StatusName)]
			if !ok {
				return 0, nil, fmt.Errorf("approval_set %q: unknown status %q", set.Name, ss.StatusName)
			}
			approveTID, ok := transitionKeyToID[transitionKey{lowerStr(set.WorkflowName), lowerStrPtr(ss.ApproveTransition.FromStatusName), lowerStr(ss.ApproveTransition.ToStatusName)}]
			if !ok {
				return 0, nil, fmt.Errorf("approval_set %q: approve_transition not found", set.Name)
			}
			denyTID, ok := transitionKeyToID[transitionKey{lowerStr(set.WorkflowName), lowerStrPtr(ss.DenyTransition.FromStatusName), lowerStr(ss.DenyTransition.ToStatusName)}]
			if !ok {
				return 0, nil, fmt.Errorf("approval_set %q: deny_transition not found", set.Name)
			}
			assID, err := s.createApprovalSetStatus(ctx, tx, asID, statusID, approveTID, denyTID, ss.StepMode)
			if err != nil {
				return 0, nil, fmt.Errorf("approval_set %q: insert status: %w", set.Name, err)
			}
			for _, step := range ss.Steps {
				if err := s.createApprovalStep(ctx, tx, assID, step, customFieldNameToID); err != nil {
					return 0, nil, fmt.Errorf("approval_set %q: insert step: %w", set.Name, err)
				}
			}
		}
	}

	// 10. Configuration set — always created fresh, with resolved FK refs.
	csName := tpl.Payload.ConfigurationSet.Name
	primaryWorkflowID := lookupOrNil(workflowNameToID, tpl.Payload.Links.WorkflowName)
	primaryConditionSetID := lookupOrNil(conditionSetNameToID, tpl.Payload.Links.ConditionSetName)
	primaryApprovalSetID := lookupOrNil(approvalSetNameToID, tpl.Payload.Links.ApprovalSetName)
	defaultItemTypeID := lookupOrNil(itemTypeNameToID, tpl.Payload.ConfigurationSet.DefaultItemTypeName)

	cs := &models.ConfigurationSet{
		Name:                    csName,
		Description:             tpl.Payload.ConfigurationSet.Description,
		IsDefault:               false,
		DifferentiateByItemType: tpl.Payload.ConfigurationSet.DifferentiateByItemType,
		WorkflowID:              primaryWorkflowID,
		ConditionSetID:          primaryConditionSetID,
		ApprovalSetID:           primaryApprovalSetID,
		DefaultItemTypeID:       defaultItemTypeID,
	}
	id64, err := s.repo.Create(tx, cs)
	if err != nil {
		return 0, nil, fmt.Errorf("create configuration_set: %w", err)
	}
	configSetID = int(id64)

	// Notification setting is intentionally not exported in v1.
	if err := s.repo.SaveNotificationSetting(tx, configSetID, nil); err != nil {
		return 0, nil, fmt.Errorf("save notification setting: %w", err)
	}

	createScreenID := lookupOrNil(screenNameToID, tpl.Payload.Links.CreateScreenName)
	editScreenID := lookupOrNil(screenNameToID, tpl.Payload.Links.EditScreenName)
	viewScreenID := lookupOrNil(screenNameToID, tpl.Payload.Links.ViewScreenName)
	if err := s.repo.SaveScreenAssignments(tx, configSetID, createScreenID, editScreenID, viewScreenID); err != nil {
		return 0, nil, fmt.Errorf("save screen assignments: %w", err)
	}

	priorityIDs := make([]int, 0, len(tpl.Payload.Links.PriorityNames))
	for _, name := range tpl.Payload.Links.PriorityNames {
		if id, ok := priorityNameToID[lowerStr(name)]; ok {
			priorityIDs = append(priorityIDs, id)
		}
	}
	if err := s.repo.SavePriorityAssignments(tx, configSetID, priorityIDs); err != nil {
		return 0, nil, fmt.Errorf("save priority assignments: %w", err)
	}

	// Item-type configs (per-type overrides).
	itConfigs := make([]models.ItemTypeConfig, 0, len(tpl.Payload.Links.ItemTypeConfigs))
	for _, itc := range tpl.Payload.Links.ItemTypeConfigs {
		typeID, ok := itemTypeNameToID[lowerStr(itc.ItemTypeName)]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("item_type_config: skipping unknown item type %q", itc.ItemTypeName))
			continue
		}
		entry := models.ItemTypeConfig{ItemTypeID: typeID}
		entry.WorkflowID = lookupOrNil(workflowNameToID, itc.WorkflowName)
		entry.ConditionSetID = lookupOrNil(conditionSetNameToID, itc.ConditionSetName)
		entry.ApprovalSetID = lookupOrNil(approvalSetNameToID, itc.ApprovalSetName)
		entry.CreateScreenID = lookupOrNil(screenNameToID, itc.CreateScreenName)
		entry.EditScreenID = lookupOrNil(screenNameToID, itc.EditScreenName)
		entry.ViewScreenID = lookupOrNil(screenNameToID, itc.ViewScreenName)
		itConfigs = append(itConfigs, entry)
	}
	if err := s.repo.SaveItemTypeConfigs(tx, configSetID, itConfigs); err != nil {
		return 0, nil, fmt.Errorf("save item type configs: %w", err)
	}

	return configSetID, warnings, nil
}

// ---- find-or-create helpers ----------------------------------------------

func (s *ConfigSetImportService) findOrCreateCustomField(ctx context.Context, tx database.Tx, cf ConfigSetTplCustomField, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM custom_field_definitions WHERE LOWER(name) = LOWER(?)`, cf.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	//nolint:misspell // database uses British spelling (applies_to_customer_organisations)
	err = tx.QueryRowContext(ctx, `
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
		                                       applies_to_portal_customers, applies_to_customer_organisations,
		                                       system_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, false, ?, ?) RETURNING id
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
		cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *ConfigSetImportService) findOrCreateStatus(ctx context.Context, tx database.Tx, st ConfigSetTplStatus, categoryNameToID map[string]int, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM statuses WHERE LOWER(name) = LOWER(?)`, st.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	categoryID, ok := categoryNameToID[lowerStr(st.CategoryName)]
	if !ok {
		// Not in the precomputed map (e.g. status block referenced a category
		// that wasn't listed in status_categories[]); look up directly.
		direct, lookupErr := s.lookupStatusCategoryID(ctx, st.CategoryName)
		if lookupErr != nil || direct == 0 {
			return 0, fmt.Errorf("category %q not resolved", st.CategoryName)
		}
		categoryID = direct
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES (?, ?, ?, false, ?, ?) RETURNING id
	`, st.Name, st.Description, categoryID, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *ConfigSetImportService) findOrCreateItemType(ctx context.Context, tx database.Tx, it ConfigSetTplItemType, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM item_types WHERE LOWER(name) = LOWER(?)`, it.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO item_types (name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at)
		VALUES (?, ?, false, ?, ?, ?, ?, ?, ?) RETURNING id
	`, it.Name, it.Description, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *ConfigSetImportService) findOrCreatePriority(ctx context.Context, tx database.Tx, p ConfigSetTplPriority, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)`, p.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO priorities (name, description, is_default, icon, color, sort_order, created_at, updated_at)
		VALUES (?, ?, false, ?, ?, ?, ?, ?) RETURNING id
	`, p.Name, p.Description, p.Icon, p.Color, p.SortOrder, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// findOrCreateScreen reuses a screen by name if one exists (DB enforces
// UNIQUE(name)); otherwise creates the screen, screen_fields, and
// screen_system_fields rows.
func (s *ConfigSetImportService) findOrCreateScreen(ctx context.Context, tx database.Tx, sc ConfigSetTplScreen, customFieldNameToID map[string]int, now time.Time) (id int, reused bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM screens WHERE LOWER(name) = LOWER(?)`, sc.Name).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO screens (name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, sc.Name, sc.Description, now, now).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	for _, f := range sc.Fields {
		ident := f.FieldIdentifier
		if f.FieldKind == "custom" {
			cfID, ok := customFieldNameToID[lowerStr(f.CustomFieldName)]
			if !ok {
				return 0, false, fmt.Errorf("screen %q: custom field %q not found", sc.Name, f.CustomFieldName)
			}
			ident = strconv.Itoa(cfID)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id, f.FieldKind, ident, f.DisplayOrder, f.IsRequired, f.FieldWidth)
		if err != nil {
			return 0, false, err
		}
	}
	for _, name := range sc.SystemFields {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO screen_system_fields (screen_id, field_name) VALUES (?, ?)
		`, id, name)
		if err != nil {
			return 0, false, err
		}
	}
	return id, false, nil
}

func (s *ConfigSetImportService) createWorkflow(ctx context.Context, tx database.Tx, wf ConfigSetTplWorkflow, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES (?, ?, false, ?, ?) RETURNING id
	`, wf.Name, wf.Description, now, now).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createWorkflowTransition(ctx context.Context, tx database.Tx, workflowID int, fromStatusID *int, toStatusID int, t ConfigSetTplWorkflowTransition, now time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, workflowID, fromStatusID, toStatusID, t.DisplayOrder, t.SourceHandle, t.TargetHandle, now).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createConditionSet(ctx context.Context, tx database.Tx, set ConfigSetTplConditionSet, workflowNameToID map[string]int, now time.Time) (int, error) {
	wfID, ok := workflowNameToID[lowerStr(set.WorkflowName)]
	if !ok {
		return 0, fmt.Errorf("workflow %q not found", set.WorkflowName)
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO condition_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, set.Name, set.Description, wfID, now, now).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createConditionSetTransition(ctx context.Context, tx database.Tx, conditionSetID, transitionID int, logicMode string, now time.Time) (int, error) {
	mode := logicMode
	if mode == "" {
		mode = "and"
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode, created_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, conditionSetID, transitionID, mode, now).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createCondition(ctx context.Context, tx database.Tx, transitionID int, c ConfigSetTplCondition, cfg map[string]interface{}, now time.Time) error {
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal condition config: %w", err)
	}
	mode := c.Mode
	if mode == "" {
		mode = models.ConditionModeCondition
	}
	var errMsg interface{}
	if c.ErrorMessage != "" {
		errMsg = c.ErrorMessage
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO conditions (condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, transitionID, c.Type, string(cfgBytes), c.DisplayOrder, mode, errMsg, now)
	return err
}

// rewriteConditionConfigForImport reverses the export rewrites: name fields
// → integer IDs. Returns a fresh map (does not mutate the input).
func (s *ConfigSetImportService) rewriteConditionConfigForImport(condType string, cfg map[string]interface{}, customFieldNameToID map[string]int, ctx context.Context) map[string]interface{} {
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	switch condType {
	case models.ConditionTypeUserInRole:
		if name, ok := out["role_name"].(string); ok && name != "" {
			if id, _ := s.lookupRoleID(ctx, name); id > 0 {
				out["role_id"] = id
			}
			delete(out, "role_name")
		}
		s.rewriteFieldRefForImport(out, customFieldNameToID)
	case models.ConditionTypeUserInGroup:
		if name, ok := out["group_name"].(string); ok && name != "" {
			if id, _ := s.lookupGroupID(ctx, name); id > 0 {
				out["group_id"] = id
			}
			delete(out, "group_name")
		}
		s.rewriteFieldRefForImport(out, customFieldNameToID)
	case models.ConditionTypeFieldValue:
		if name, ok := out["custom_field_name"].(string); ok && name != "" {
			if id, ok := customFieldNameToID[lowerStr(name)]; ok {
				out["field_id"] = id
			}
			delete(out, "custom_field_name")
		}
	}
	return out
}

func (s *ConfigSetImportService) rewriteFieldRefForImport(cfg map[string]interface{}, customFieldNameToID map[string]int) {
	source, _ := cfg["source"].(string)
	if source != "custom_field" {
		return
	}
	if name, ok := cfg["custom_field_name"].(string); ok && name != "" {
		if id, ok := customFieldNameToID[lowerStr(name)]; ok {
			cfg["field_id"] = id
		}
		delete(cfg, "custom_field_name")
	}
}

func (s *ConfigSetImportService) createApprovalSet(ctx context.Context, tx database.Tx, set ConfigSetTplApprovalSet, workflowNameToID map[string]int, now time.Time) (int, error) {
	wfID, ok := workflowNameToID[lowerStr(set.WorkflowName)]
	if !ok {
		return 0, fmt.Errorf("workflow %q not found", set.WorkflowName)
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, set.Name, set.Description, wfID, now, now).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createApprovalSetStatus(ctx context.Context, tx database.Tx, approvalSetID, statusID, approveTID, denyTID int, stepMode string) (int, error) {
	mode := stepMode
	if mode == "" {
		mode = models.ApprovalStepModeSequential
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_set_statuses (approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, approvalSetID, statusID, approveTID, denyTID, mode, time.Now()).Scan(&id)
	return id, err
}

func (s *ConfigSetImportService) createApprovalStep(ctx context.Context, tx database.Tx, approvalSetStatusID int, step ConfigSetTplApprovalStep, customFieldNameToID map[string]int) error {
	// Resolve identity refs (already verified to exist in phase 1).
	approverFieldID := resolveCustomFieldID(step.ApproverCustomFieldName, customFieldNameToID)
	approverRoleID, _ := s.lookupRoleIDIfNotEmpty(ctx, step.ApproverRoleName)
	approverGroupID, _ := s.lookupGroupIDIfNotEmpty(ctx, step.ApproverGroupName)
	approverUserID, _ := s.lookupUserIDIfNotEmpty(ctx, step.ApproverUserEmail)
	escTargetFieldID := resolveCustomFieldID(step.EscalationTargetCustomFieldName, customFieldNameToID)
	escTargetRoleID, _ := s.lookupRoleIDIfNotEmpty(ctx, step.EscalationTargetRoleName)
	escTargetGroupID, _ := s.lookupGroupIDIfNotEmpty(ctx, step.EscalationTargetGroupName)
	escTargetUserID, _ := s.lookupUserIDIfNotEmpty(ctx, step.EscalationTargetUserEmail)

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

	_, err := tx.ExecContext(ctx, `
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
		approvalSetStatusID, step.DisplayOrder, step.Name,
		quorumMode, step.QuorumCount, step.QuorumPercent, rejectionPolicy,
		step.ApproverSource, nullStringIfEmptyImport(step.ApproverFieldIdentifier), approverFieldID,
		approverRoleID, approverGroupID, approverUserID, step.AllowSelfApproval,
		onLeave,
		step.EscalationAfterHours, nullStringIfEmptyImport(step.EscalationAction), nullStringIfEmptyImport(step.EscalationTargetSource),
		nullStringIfEmptyImport(step.EscalationTargetFieldIdentifier), escTargetFieldID,
		escTargetRoleID, escTargetGroupID, escTargetUserID,
		step.MaxEscalations, time.Now(),
	)
	return err
}

// ---- name → id lookups ---------------------------------------------------

func (s *ConfigSetImportService) lookupStatusCategoryID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM status_categories WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *ConfigSetImportService) lookupRoleID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM workspace_roles WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *ConfigSetImportService) lookupGroupID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM groups WHERE LOWER(group_name) = LOWER(?)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *ConfigSetImportService) lookupUserID(ctx context.Context, email string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER(?)`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *ConfigSetImportService) lookupRoleIDIfNotEmpty(ctx context.Context, name string) (*int, error) {
	if name == "" {
		return nil, nil
	}
	id, err := s.lookupRoleID(ctx, name)
	if err != nil || id == 0 {
		return nil, err
	}
	return &id, nil
}

func (s *ConfigSetImportService) lookupGroupIDIfNotEmpty(ctx context.Context, name string) (*int, error) {
	if name == "" {
		return nil, nil
	}
	id, err := s.lookupGroupID(ctx, name)
	if err != nil || id == 0 {
		return nil, err
	}
	return &id, nil
}

func (s *ConfigSetImportService) lookupUserIDIfNotEmpty(ctx context.Context, email string) (*int, error) {
	if email == "" {
		return nil, nil
	}
	id, err := s.lookupUserID(ctx, email)
	if err != nil || id == 0 {
		return nil, err
	}
	return &id, nil
}

// ---- helpers -------------------------------------------------------------

func lowerStr(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func lowerStrPtr(p *string) string {
	if p == nil {
		return ""
	}
	return lowerStr(*p)
}

func displayFrom(p *string) string {
	if p == nil {
		return "<initial>"
	}
	return *p
}

func lookupOrNil(m map[string]int, name string) *int {
	if name == "" {
		return nil
	}
	if id, ok := m[lowerStr(name)]; ok {
		return &id
	}
	return nil
}

func resolveCustomFieldID(name string, m map[string]int) *int {
	if name == "" {
		return nil
	}
	if id, ok := m[lowerStr(name)]; ok {
		return &id
	}
	return nil
}

// nullStringIfEmptyImport mirrors the unexported helper in the approval_set
// repository — duplicated rather than exported because the repo package keeps
// it private. Empty strings need to scan/insert as SQL NULL on these columns,
// not as the empty literal.
func nullStringIfEmptyImport(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
