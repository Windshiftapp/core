package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ConfigSetExportService walks a ConfigurationSet and everything it
// transitively touches (workflow, condition set, approval set, screens,
// item types, priorities, the custom field definitions referenced anywhere
// inside) and emits a portable JSON envelope suitable for import on another
// instance.
type ConfigSetExportService struct {
	db   database.Database
	repo *repository.ConfigurationSetRepository
}

// NewConfigSetExportService wires the export service to the existing config
// set repository. Read-only; no transactions.
func NewConfigSetExportService(db database.Database, repo *repository.ConfigurationSetRepository) *ConfigSetExportService {
	return &ConfigSetExportService{db: db, repo: repo}
}

// Export builds a fully-resolved template payload for the given configuration
// set. exportedBy is optional provenance metadata stamped into the envelope.
//
// Returns ErrCannotExportDefault when the target row has is_default=true —
// the default config set is not portable; callers (the HTTP handler) should
// surface this as 403 so users know to clone first.
func (s *ConfigSetExportService) Export(ctx context.Context, configSetID int, exportedBy *ConfigSetExportBy) (*ConfigSetTemplate, error) {
	cs, err := s.repo.FindByID(configSetID)
	if err != nil {
		return nil, err
	}
	if cs.IsDefault {
		return nil, ErrCannotExportDefault
	}

	tpl := &ConfigSetTemplate{
		SchemaVersion: ConfigSetTemplateSchemaVersion,
		Kind:          ConfigSetTemplateKind,
		ExportedAt:    time.Now().UTC(),
		ExportedBy:    exportedBy,
	}
	tpl.Payload.ConfigurationSet = ConfigSetTplConfigSet{
		Name:                    cs.Name,
		Description:             cs.Description,
		DifferentiateByItemType: cs.DifferentiateByItemType,
		DefaultItemTypeName:     cs.DefaultItemTypeName,
	}

	customFieldNames := map[int]string{}
	statusNames := map[int]string{}
	statusCategoryNames := map[string]struct{}{}
	transitionRefs := map[int]ConfigSetTplTransitionRef{} // transition_id → from/to ref

	// Workflow + transitions (config-set primary workflow only — overlay
	// workflows from item-type configs are exported separately below).
	primaryWorkflowName := ""
	if cs.WorkflowID != nil {
		wf, err := s.exportWorkflow(ctx, *cs.WorkflowID, statusNames, transitionRefs)
		if err != nil {
			return nil, fmt.Errorf("export primary workflow: %w", err)
		}
		if wf != nil {
			primaryWorkflowName = wf.Name
			tpl.Payload.Workflows = append(tpl.Payload.Workflows, *wf)
		}
	}
	tpl.Payload.Links.WorkflowName = primaryWorkflowName

	// Per-item-type overlay workflows. Skip the sentinel "Default" entry the
	// loader uses to mean "inherit the config-set workflow".
	itemTypeNames := []string{}
	for _, itc := range cs.ItemTypeConfigs {
		itemTypeNames = append(itemTypeNames, itc.ItemTypeName)
		if itc.WorkflowID != nil && itc.WorkflowName != "" && itc.WorkflowName != "Default" {
			if _, ok := findWorkflowByName(tpl.Payload.Workflows, itc.WorkflowName); !ok {
				wf, err := s.exportWorkflow(ctx, *itc.WorkflowID, statusNames, transitionRefs)
				if err != nil {
					return nil, fmt.Errorf("export overlay workflow %q: %w", itc.WorkflowName, err)
				}
				if wf != nil {
					tpl.Payload.Workflows = append(tpl.Payload.Workflows, *wf)
				}
			}
		}
	}

	// Condition set (workflow-bound; export only the one referenced by this
	// configuration set; overlay-set condition references are exported as
	// names in the links section but the sets themselves must already exist
	// either via the primary workflow path or be added explicitly).
	if cs.ConditionSetID != nil {
		set, err := s.exportConditionSet(ctx, *cs.ConditionSetID, transitionRefs, customFieldNames)
		if err != nil {
			return nil, fmt.Errorf("export condition set: %w", err)
		}
		if set != nil {
			tpl.Payload.ConditionSets = append(tpl.Payload.ConditionSets, *set)
			tpl.Payload.Links.ConditionSetName = set.Name
		}
	}
	for _, itc := range cs.ItemTypeConfigs {
		if itc.ConditionSetID != nil && itc.ConditionSetName != "" {
			if _, ok := findConditionSetByName(tpl.Payload.ConditionSets, itc.ConditionSetName); !ok {
				set, err := s.exportConditionSet(ctx, *itc.ConditionSetID, transitionRefs, customFieldNames)
				if err != nil {
					return nil, fmt.Errorf("export overlay condition set %q: %w", itc.ConditionSetName, err)
				}
				if set != nil {
					tpl.Payload.ConditionSets = append(tpl.Payload.ConditionSets, *set)
				}
			}
		}
	}

	// Approval set (same overlay treatment).
	if cs.ApprovalSetID != nil {
		set, err := s.exportApprovalSet(ctx, *cs.ApprovalSetID, transitionRefs, customFieldNames)
		if err != nil {
			return nil, fmt.Errorf("export approval set: %w", err)
		}
		if set != nil {
			tpl.Payload.ApprovalSets = append(tpl.Payload.ApprovalSets, *set)
			tpl.Payload.Links.ApprovalSetName = set.Name
		}
	}
	for _, itc := range cs.ItemTypeConfigs {
		if itc.ApprovalSetID != nil && itc.ApprovalSetName != "" {
			if _, ok := findApprovalSetByName(tpl.Payload.ApprovalSets, itc.ApprovalSetName); !ok {
				set, err := s.exportApprovalSet(ctx, *itc.ApprovalSetID, transitionRefs, customFieldNames)
				if err != nil {
					return nil, fmt.Errorf("export overlay approval set %q: %w", itc.ApprovalSetName, err)
				}
				if set != nil {
					tpl.Payload.ApprovalSets = append(tpl.Payload.ApprovalSets, *set)
				}
			}
		}
	}

	// Item types and priorities used by the configuration set.
	if len(itemTypeNames) > 0 {
		itemTypes, err := s.exportItemTypesByName(ctx, itemTypeNames)
		if err != nil {
			return nil, fmt.Errorf("export item types: %w", err)
		}
		tpl.Payload.ItemTypes = itemTypes
	}
	priorityNames := make([]string, 0, len(cs.PrioritiesDetailed))
	for _, p := range cs.PrioritiesDetailed {
		priorityNames = append(priorityNames, p.Name)
	}
	if len(priorityNames) > 0 {
		priorities, err := s.exportPrioritiesByName(ctx, priorityNames)
		if err != nil {
			return nil, fmt.Errorf("export priorities: %w", err)
		}
		tpl.Payload.Priorities = priorities
		tpl.Payload.Links.PriorityNames = priorityNames
	}

	// Screens — both top-level (create/edit/view) and per-item-type overrides.
	screenIDs := uniqueInts(
		ptrToSlice(cs.CreateScreenID),
		ptrToSlice(cs.EditScreenID),
		ptrToSlice(cs.ViewScreenID),
	)
	for _, itc := range cs.ItemTypeConfigs {
		screenIDs = append(screenIDs, ptrToSlice(itc.CreateScreenID)...)
		screenIDs = append(screenIDs, ptrToSlice(itc.EditScreenID)...)
		screenIDs = append(screenIDs, ptrToSlice(itc.ViewScreenID)...)
	}
	screenIDs = uniqueInts(screenIDs)
	if len(screenIDs) > 0 {
		screens, err := s.exportScreens(ctx, screenIDs, customFieldNames)
		if err != nil {
			return nil, fmt.Errorf("export screens: %w", err)
		}
		tpl.Payload.Screens = screens
	}
	tpl.Payload.Links.CreateScreenName = cs.CreateScreenName
	tpl.Payload.Links.EditScreenName = cs.EditScreenName
	tpl.Payload.Links.ViewScreenName = cs.ViewScreenName

	// Custom fields collected through workflow/condition/approval/screen
	// traversal — emit their full definitions.
	if len(customFieldNames) > 0 {
		fields, err := s.exportCustomFields(ctx, mapValues(customFieldNames))
		if err != nil {
			return nil, fmt.Errorf("export custom fields: %w", err)
		}
		tpl.Payload.CustomFields = fields
	}

	// Statuses collected through workflow traversal — embed full descriptors.
	if len(statusNames) > 0 {
		statuses, err := s.exportStatuses(ctx, mapValues(statusNames))
		if err != nil {
			return nil, fmt.Errorf("export statuses: %w", err)
		}
		tpl.Payload.Statuses = statuses
		for _, st := range statuses {
			statusCategoryNames[st.CategoryName] = struct{}{}
		}
	}

	// Status categories — name-only references.
	if len(statusCategoryNames) > 0 {
		cats := make([]ConfigSetTplStatusCat, 0, len(statusCategoryNames))
		for name := range statusCategoryNames {
			cats = append(cats, ConfigSetTplStatusCat{Name: name})
		}
		sort.Slice(cats, func(i, j int) bool { return cats[i].Name < cats[j].Name })
		tpl.Payload.StatusCategories = cats
	}

	// Item-type config links (per-type overrides, names only).
	for _, itc := range cs.ItemTypeConfigs {
		entry := ConfigSetTplItemTypeConfig{ItemTypeName: itc.ItemTypeName}
		if itc.WorkflowID != nil && itc.WorkflowName != "Default" {
			entry.WorkflowName = itc.WorkflowName
		}
		if itc.ConditionSetID != nil {
			entry.ConditionSetName = itc.ConditionSetName
		}
		if itc.ApprovalSetID != nil {
			entry.ApprovalSetName = itc.ApprovalSetName
		}
		if itc.CreateScreenID != nil && itc.CreateScreenName != "Default" {
			entry.CreateScreenName = itc.CreateScreenName
		}
		if itc.EditScreenID != nil && itc.EditScreenName != "Default" {
			entry.EditScreenName = itc.EditScreenName
		}
		if itc.ViewScreenID != nil && itc.ViewScreenName != "Default" {
			entry.ViewScreenName = itc.ViewScreenName
		}
		tpl.Payload.Links.ItemTypeConfigs = append(tpl.Payload.Links.ItemTypeConfigs, entry)
	}

	return tpl, nil
}

// ---- per-section exporters ------------------------------------------------

func (s *ConfigSetExportService) exportWorkflow(ctx context.Context, workflowID int, statusNames map[int]string, transitionRefs map[int]ConfigSetTplTransitionRef) (*ConfigSetTplWorkflow, error) {
	var wf ConfigSetTplWorkflow
	var description sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT name, COALESCE(description, '') FROM workflows WHERE id = ?
	`, workflowID).Scan(&wf.Name, &description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	wf.Description = description.String

	rows, err := s.db.QueryContext(ctx, `
		SELECT wt.id, wt.from_status_id, wt.to_status_id, wt.display_order,
		       COALESCE(wt.source_handle, ''), COALESCE(wt.target_handle, ''),
		       fs.name AS from_status_name, ts.name AS to_status_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		WHERE wt.workflow_id = ?
		ORDER BY wt.display_order, wt.id
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var transitionID int
		var fromID sql.NullInt64
		var toID int
		var displayOrder int
		var sourceHandle, targetHandle string
		var fromName sql.NullString
		var toName string
		if scanErr := rows.Scan(&transitionID, &fromID, &toID, &displayOrder, &sourceHandle, &targetHandle, &fromName, &toName); scanErr != nil {
			return nil, scanErr
		}

		t := ConfigSetTplWorkflowTransition{
			ToStatusName: toName,
			DisplayOrder: displayOrder,
			SourceHandle: sourceHandle,
			TargetHandle: targetHandle,
		}
		if fromName.Valid {
			f := fromName.String
			t.FromStatusName = &f
			statusNames[int(fromID.Int64)] = f
		}
		statusNames[toID] = toName

		ref := ConfigSetTplTransitionRef{ToStatusName: toName}
		if t.FromStatusName != nil {
			ref.FromStatusName = t.FromStatusName
		}
		transitionRefs[transitionID] = ref

		wf.Transitions = append(wf.Transitions, t)
	}
	return &wf, rows.Err()
}

func (s *ConfigSetExportService) exportConditionSet(ctx context.Context, conditionSetID int, transitionRefs map[int]ConfigSetTplTransitionRef, customFieldNames map[int]string) (*ConfigSetTplConditionSet, error) {
	var cs ConfigSetTplConditionSet
	var description sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT cs.name, COALESCE(cs.description, ''), w.name
		FROM condition_sets cs
		JOIN workflows w ON cs.workflow_id = w.id
		WHERE cs.id = ?
	`, conditionSetID).Scan(&cs.Name, &description, &cs.WorkflowName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cs.Description = description.String

	tcRows, err := s.db.QueryContext(ctx, `
		SELECT cst.id, cst.transition_id, cst.logic_mode
		FROM condition_set_transitions cst
		WHERE cst.condition_set_id = ?
		ORDER BY cst.id
	`, conditionSetID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tcRows.Close() }()

	type tcRow struct {
		ID           int
		TransitionID int
		LogicMode    string
	}
	var rows []tcRow
	for tcRows.Next() {
		var r tcRow
		if scanErr := tcRows.Scan(&r.ID, &r.TransitionID, &r.LogicMode); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, r)
	}
	if err := tcRows.Err(); err != nil {
		return nil, err
	}

	for _, r := range rows {
		ref, ok := transitionRefs[r.TransitionID]
		if !ok {
			// Transition not part of any exported workflow — skip rather than
			// emit a dangling reference.
			continue
		}
		tc := ConfigSetTplTransitionCondition{
			FromStatusName: ref.FromStatusName,
			ToStatusName:   ref.ToStatusName,
			LogicMode:      r.LogicMode,
		}

		condRows, err := s.db.QueryContext(ctx, `
			SELECT condition_type, mode, COALESCE(error_message, ''), display_order, config
			FROM conditions
			WHERE condition_set_transition_id = ?
			ORDER BY display_order, id
		`, r.ID)
		if err != nil {
			return nil, err
		}
		for condRows.Next() {
			var c ConfigSetTplCondition
			var configRaw string
			if scanErr := condRows.Scan(&c.Type, &c.Mode, &c.ErrorMessage, &c.DisplayOrder, &configRaw); scanErr != nil {
				_ = condRows.Close()
				return nil, scanErr
			}
			cfg := map[string]interface{}{}
			if configRaw != "" {
				if err := json.Unmarshal([]byte(configRaw), &cfg); err != nil {
					_ = condRows.Close()
					return nil, fmt.Errorf("invalid condition config json: %w", err)
				}
			}
			if err := s.rewriteConditionConfigForExport(ctx, c.Type, cfg, customFieldNames); err != nil {
				_ = condRows.Close()
				return nil, err
			}
			c.Config = cfg
			tc.Conditions = append(tc.Conditions, c)
		}
		_ = condRows.Close()
		cs.TransitionConditions = append(cs.TransitionConditions, tc)
	}
	return &cs, nil
}

// rewriteConditionConfigForExport replaces ID-based references with names so
// the bundle is portable. role_id → role_name; group_id → group_name; for
// custom_field references, field_id → custom_field_name.
func (s *ConfigSetExportService) rewriteConditionConfigForExport(ctx context.Context, condType string, cfg map[string]interface{}, customFieldNames map[int]string) error {
	switch condType {
	case models.ConditionTypeUserInRole:
		if rawID, ok := cfg["role_id"]; ok {
			if id, ok := parseIntish(rawID); ok && id > 0 {
				name, err := s.lookupRoleName(ctx, id)
				if err != nil {
					return err
				}
				cfg["role_name"] = name
				delete(cfg, "role_id")
			}
		}
		s.rewriteFieldRef(cfg, customFieldNames, ctx)
	case models.ConditionTypeUserInGroup:
		if rawID, ok := cfg["group_id"]; ok {
			if id, ok := parseIntish(rawID); ok && id > 0 {
				name, err := s.lookupGroupName(ctx, id)
				if err != nil {
					return err
				}
				cfg["group_name"] = name
				delete(cfg, "group_id")
			}
		}
		s.rewriteFieldRef(cfg, customFieldNames, ctx)
	case models.ConditionTypeFieldValue:
		// field_identifier may be either a numeric custom-field id or a
		// regular column name; only rewrite if it parses as an integer that
		// matches a custom field row.
		if rawID, ok := cfg["field_id"]; ok {
			if id, ok := parseIntish(rawID); ok && id > 0 {
				name, err := s.lookupCustomFieldName(ctx, id)
				if err != nil {
					return err
				}
				cfg["custom_field_name"] = name
				customFieldNames[id] = name
				delete(cfg, "field_id")
			}
		}
	case models.ConditionTypeScript:
		// Script body is opaque; leave alone.
	}
	return nil
}

// rewriteFieldRef handles the embedded FieldRef inside user_in_role / user_in_group
// configs (when source=='custom_field', it carries a field_id).
func (s *ConfigSetExportService) rewriteFieldRef(cfg map[string]interface{}, customFieldNames map[int]string, ctx context.Context) {
	source, _ := cfg["source"].(string)
	if source != "custom_field" {
		return
	}
	if rawID, ok := cfg["field_id"]; ok {
		if id, ok := parseIntish(rawID); ok && id > 0 {
			name, err := s.lookupCustomFieldName(ctx, id)
			if err == nil && name != "" {
				cfg["custom_field_name"] = name
				customFieldNames[id] = name
				delete(cfg, "field_id")
			}
		}
	}
}

func (s *ConfigSetExportService) exportApprovalSet(ctx context.Context, approvalSetID int, transitionRefs map[int]ConfigSetTplTransitionRef, customFieldNames map[int]string) (*ConfigSetTplApprovalSet, error) {
	var as ConfigSetTplApprovalSet
	var description sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT a.name, COALESCE(a.description, ''), w.name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id
		WHERE a.id = ?
	`, approvalSetID).Scan(&as.Name, &description, &as.WorkflowName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	as.Description = description.String

	statusRows, err := s.db.QueryContext(ctx, `
		SELECT ass.id, ass.status_id, ass.approve_transition_id, ass.deny_transition_id, ass.step_mode, st.name
		FROM approval_set_statuses ass
		JOIN statuses st ON st.id = ass.status_id
		WHERE ass.approval_set_id = ? AND ass.is_active = true
		ORDER BY ass.id
	`, approvalSetID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = statusRows.Close() }()

	type statusRow struct {
		ID                            int
		StatusID, ApproveTID, DenyTID int
		StepMode, StatusName          string
	}
	var rows []statusRow
	for statusRows.Next() {
		var r statusRow
		if scanErr := statusRows.Scan(&r.ID, &r.StatusID, &r.ApproveTID, &r.DenyTID, &r.StepMode, &r.StatusName); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, r)
	}
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	for _, r := range rows {
		appRef := transitionRefs[r.ApproveTID]
		denyRef := transitionRefs[r.DenyTID]
		setStatus := ConfigSetTplApprovalSetStatus{
			StatusName:        r.StatusName,
			StepMode:          r.StepMode,
			ApproveTransition: appRef,
			DenyTransition:    denyRef,
		}

		stepRows, err := s.db.QueryContext(ctx, `
			SELECT id, display_order, name,
			       quorum_mode, quorum_count, quorum_percent, rejection_policy,
			       approver_source, approver_field_identifier, approver_field_id,
			       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
			       on_leave_strategy,
			       escalation_after_hours, escalation_action, escalation_target_source,
			       escalation_target_field_identifier, escalation_target_field_id,
			       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
			       max_escalations
			FROM approval_steps WHERE approval_set_status_id = ? ORDER BY display_order, id
		`, r.ID)
		if err != nil {
			return nil, err
		}
		for stepRows.Next() {
			var (
				stepID                                                               int
				step                                                                 ConfigSetTplApprovalStep
				approverFieldIdent                                                   sql.NullString
				approverFieldID, approverRoleID, approverGroupID, approverUserID     sql.NullInt64
				escAfterHours                                                        sql.NullInt64
				escAction, escTargetSource, escTargetFieldIdent                      sql.NullString
				escTargetFieldID, escTargetRoleID, escTargetGroupID, escTargetUserID sql.NullInt64
				maxEscalations                                                       sql.NullInt64
			)
			if scanErr := stepRows.Scan(
				&stepID, &step.DisplayOrder, &step.Name,
				&step.QuorumMode, &step.QuorumCount, &step.QuorumPercent, &step.RejectionPolicy,
				&step.ApproverSource, &approverFieldIdent, &approverFieldID,
				&approverRoleID, &approverGroupID, &approverUserID, &step.AllowSelfApproval,
				&step.OnLeaveStrategy,
				&escAfterHours, &escAction, &escTargetSource,
				&escTargetFieldIdent, &escTargetFieldID,
				&escTargetRoleID, &escTargetGroupID, &escTargetUserID,
				&maxEscalations,
			); scanErr != nil {
				_ = stepRows.Close()
				return nil, scanErr
			}
			step.ApproverFieldIdentifier = approverFieldIdent.String
			step.EscalationAction = escAction.String
			step.EscalationTargetSource = escTargetSource.String
			step.EscalationTargetFieldIdentifier = escTargetFieldIdent.String
			if escAfterHours.Valid {
				v := int(escAfterHours.Int64)
				step.EscalationAfterHours = &v
			}
			if maxEscalations.Valid {
				v := int(maxEscalations.Int64)
				step.MaxEscalations = &v
			}

			// Approver ref-by-name.
			if approverFieldID.Valid {
				name, err := s.lookupCustomFieldName(ctx, int(approverFieldID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.ApproverCustomFieldName = name
				customFieldNames[int(approverFieldID.Int64)] = name
			}
			if approverRoleID.Valid {
				name, err := s.lookupRoleName(ctx, int(approverRoleID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.ApproverRoleName = name
			}
			if approverGroupID.Valid {
				name, err := s.lookupGroupName(ctx, int(approverGroupID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.ApproverGroupName = name
			}
			if approverUserID.Valid {
				email, err := s.lookupUserEmail(ctx, int(approverUserID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.ApproverUserEmail = email
			}

			// Escalation target ref-by-name (mirrors approver).
			if escTargetFieldID.Valid {
				name, err := s.lookupCustomFieldName(ctx, int(escTargetFieldID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.EscalationTargetCustomFieldName = name
				customFieldNames[int(escTargetFieldID.Int64)] = name
			}
			if escTargetRoleID.Valid {
				name, err := s.lookupRoleName(ctx, int(escTargetRoleID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.EscalationTargetRoleName = name
			}
			if escTargetGroupID.Valid {
				name, err := s.lookupGroupName(ctx, int(escTargetGroupID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.EscalationTargetGroupName = name
			}
			if escTargetUserID.Valid {
				email, err := s.lookupUserEmail(ctx, int(escTargetUserID.Int64))
				if err != nil {
					_ = stepRows.Close()
					return nil, err
				}
				step.EscalationTargetUserEmail = email
			}

			setStatus.Steps = append(setStatus.Steps, step)
		}
		_ = stepRows.Close()
		as.SetStatuses = append(as.SetStatuses, setStatus)
	}
	return &as, nil
}

func (s *ConfigSetExportService) exportItemTypesByName(ctx context.Context, names []string) ([]ConfigSetTplItemType, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders, args := inArgs(names)
	q := fmt.Sprintf(`
		SELECT name, COALESCE(description, ''), COALESCE(icon, ''), COALESCE(color, ''),
		       COALESCE(hierarchy_level, 0), COALESCE(sort_order, 0)
		FROM item_types
		WHERE name IN (%s)
		ORDER BY hierarchy_level, sort_order, name
	`, placeholders)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ConfigSetTplItemType
	for rows.Next() {
		var it ConfigSetTplItemType
		if err := rows.Scan(&it.Name, &it.Description, &it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *ConfigSetExportService) exportPrioritiesByName(ctx context.Context, names []string) ([]ConfigSetTplPriority, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders, args := inArgs(names)
	q := fmt.Sprintf(`
		SELECT name, COALESCE(description, ''), COALESCE(icon, ''), COALESCE(color, ''), COALESCE(sort_order, 0)
		FROM priorities
		WHERE name IN (%s)
		ORDER BY sort_order, name
	`, placeholders)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ConfigSetTplPriority
	for rows.Next() {
		var p ConfigSetTplPriority
		if err := rows.Scan(&p.Name, &p.Description, &p.Icon, &p.Color, &p.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ConfigSetExportService) exportScreens(ctx context.Context, screenIDs []int, customFieldNames map[int]string) ([]ConfigSetTplScreen, error) {
	if len(screenIDs) == 0 {
		return nil, nil
	}
	placeholders, args := inArgsInt(screenIDs)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, name, COALESCE(description, '')
		FROM screens
		WHERE id IN (%s)
		ORDER BY id
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		ID         int
		Name, Desc string
	}
	var screens []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name, &r.Desc); err != nil {
			return nil, err
		}
		screens = append(screens, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ConfigSetTplScreen, 0, len(screens))
	for _, sr := range screens {
		s2 := ConfigSetTplScreen{Name: sr.Name, Description: sr.Desc}
		fieldRows, err := s.db.QueryContext(ctx, `
			SELECT field_type, field_identifier, display_order, COALESCE(is_required, false), COALESCE(field_width, '')
			FROM screen_fields
			WHERE screen_id = ?
			ORDER BY display_order, id
		`, sr.ID)
		if err != nil {
			return nil, err
		}
		for fieldRows.Next() {
			var f ConfigSetTplScreenField
			var ft, ident string
			if scanErr := fieldRows.Scan(&ft, &ident, &f.DisplayOrder, &f.IsRequired, &f.FieldWidth); scanErr != nil {
				_ = fieldRows.Close()
				return nil, scanErr
			}
			f.FieldKind = ft
			if ft == "custom" {
				if id, ok := parseIntish(ident); ok && id > 0 {
					name, err := s.lookupCustomFieldName(ctx, id)
					if err != nil {
						_ = fieldRows.Close()
						return nil, err
					}
					f.CustomFieldName = name
					customFieldNames[id] = name
				} else {
					f.FieldIdentifier = ident
				}
			} else {
				f.FieldIdentifier = ident
			}
			s2.Fields = append(s2.Fields, f)
		}
		_ = fieldRows.Close()

		sysRows, err := s.db.QueryContext(ctx, `SELECT field_name FROM screen_system_fields WHERE screen_id = ? ORDER BY field_name`, sr.ID)
		if err != nil {
			return nil, err
		}
		for sysRows.Next() {
			var name string
			if err := sysRows.Scan(&name); err != nil {
				_ = sysRows.Close()
				return nil, err
			}
			s2.SystemFields = append(s2.SystemFields, name)
		}
		_ = sysRows.Close()
		out = append(out, s2)
	}
	return out, nil
}

func (s *ConfigSetExportService) exportCustomFields(ctx context.Context, names []string) ([]ConfigSetTplCustomField, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders, args := inArgs(names)
	q := fmt.Sprintf(`
		SELECT name, field_type, COALESCE(description, ''), COALESCE(required, false), COALESCE(options, ''),
		       COALESCE(display_order, 0), COALESCE(applies_to_portal_customers, false), COALESCE(applies_to_customer_organisations, false)
		FROM custom_field_definitions
		WHERE name IN (%s)
		ORDER BY display_order, name
	`, placeholders)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ConfigSetTplCustomField
	for rows.Next() {
		var f ConfigSetTplCustomField
		if err := rows.Scan(&f.Name, &f.FieldType, &f.Description, &f.Required, &f.Options,
			&f.DisplayOrder, &f.AppliesToPortalCustomers, &f.AppliesToCustomerOrganisations); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *ConfigSetExportService) exportStatuses(ctx context.Context, names []string) ([]ConfigSetTplStatus, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders, args := inArgs(names)
	q := fmt.Sprintf(`
		SELECT s.name, COALESCE(s.description, ''), c.name
		FROM statuses s
		JOIN status_categories c ON s.category_id = c.id
		WHERE s.name IN (%s)
		ORDER BY s.name
	`, placeholders)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ConfigSetTplStatus
	for rows.Next() {
		var st ConfigSetTplStatus
		if err := rows.Scan(&st.Name, &st.Description, &st.CategoryName); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ---- name lookups ---------------------------------------------------------

func (s *ConfigSetExportService) lookupCustomFieldName(ctx context.Context, id int) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM custom_field_definitions WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("custom field id=%d not found", id)
	}
	return name, err
}

func (s *ConfigSetExportService) lookupRoleName(ctx context.Context, id int) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM workspace_roles WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("workspace_role id=%d not found", id)
	}
	return name, err
}

func (s *ConfigSetExportService) lookupGroupName(ctx context.Context, id int) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT group_name FROM groups WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("group id=%d not found", id)
	}
	return name, err
}

func (s *ConfigSetExportService) lookupUserEmail(ctx context.Context, id int) (string, error) {
	var email string
	err := s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, id).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("user id=%d not found", id)
	}
	return email, err
}

// ---- helpers --------------------------------------------------------------

func ptrToSlice(p *int) []int {
	if p == nil {
		return nil
	}
	return []int{*p}
}

func uniqueInts(slices ...[]int) []int {
	seen := map[int]struct{}{}
	var out []int
	for _, s := range slices {
		for _, v := range s {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func mapValues(m map[int]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func inArgs(names []string) (placeholders string, args []interface{}) {
	args = make([]interface{}, 0, len(names))
	for i, n := range names {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, n)
	}
	return placeholders, args
}

func inArgsInt(ids []int) (placeholders string, args []interface{}) {
	args = make([]interface{}, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	return placeholders, args
}

// parseIntish coerces a JSON-decoded value (or a TEXT column scanned as
// string) into an int. Numbers come back as float64 from json.Unmarshal;
// SQLite TEXT columns scan as string. Returns false when no sensible int
// is extractable so callers can leave the original identifier alone.
func parseIntish(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		var i int
		if _, err := fmt.Sscanf(x, "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}

func findWorkflowByName(ws []ConfigSetTplWorkflow, name string) (int, bool) {
	for i, w := range ws {
		if w.Name == name {
			return i, true
		}
	}
	return -1, false
}

func findConditionSetByName(sets []ConfigSetTplConditionSet, name string) (int, bool) {
	for i, s := range sets {
		if s.Name == name {
			return i, true
		}
	}
	return -1, false
}

func findApprovalSetByName(sets []ConfigSetTplApprovalSet, name string) (int, bool) {
	for i, s := range sets {
		if s.Name == name {
			return i, true
		}
	}
	return -1, false
}
