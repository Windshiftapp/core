package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"windshift/internal/models"
)

// Conformance checking answers one question: does the live configuration of
// a configuration set still match its canonical template? The canonical
// template is the same portable document the import flow consumes; the live
// side is produced by the export flow, so both sides are compared in one
// name-based coordinate system.
//
// Drift rows name the concrete difference per section. Rows flagged
// repairable can be restored to the template by Repair; "extra" rows (live
// entities the template does not declare) are reported but never
// auto-repaired, because deleting shared registry rows could break item
// data or other configuration sets.

const (
	ConformanceKindMissing   = "missing"
	ConformanceKindDiffering = "differing"
	ConformanceKindExtra     = "extra"
)

// ConfigSetConformanceDrift is one concrete difference. ID is a stable key
// ("<section>/<kind>/<path>") used to select rows for repair.
type ConfigSetConformanceDrift struct {
	ID         string `json:"id"`
	Section    string `json:"section"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Detail     string `json:"detail"`
	Repairable bool   `json:"repairable"`
}

// ConfigSetConformanceReport summarizes one check run.
type ConfigSetConformanceReport struct {
	ConfigSetID     int                         `json:"configuration_set_id"`
	Conformant      bool                        `json:"conformant"`
	DriftCount      int                         `json:"drift_count"`
	RepairableCount int                         `json:"repairable_count"`
	Drifts          []ConfigSetConformanceDrift `json:"drifts"`
	CheckedAt       time.Time                   `json:"checked_at"`
}

// ConfigSetConformanceRepairOutcome records what happened to one selected
// drift row during repair.
type ConfigSetConformanceRepairOutcome struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	Name    string `json:"name"`
	Status  string `json:"status"` // repaired | skipped | failed
	Detail  string `json:"detail,omitempty"`
}

// ConfigSetConformanceRepairResult reports one repair run. PostCheck carries
// the conformance state after the repair transaction committed.
type ConfigSetConformanceRepairResult struct {
	ConfigSetID int                                 `json:"configuration_set_id"`
	Outcomes    []ConfigSetConformanceRepairOutcome `json:"outcomes"`
	Repaired    int                                 `json:"repaired_count"`
	Skipped     int                                 `json:"skipped_count"`
	Failed      int                                 `json:"failed_count"`
	PostCheck   *ConfigSetConformanceReport         `json:"post_check"`
}

// ConfigSetConformanceRepairRequest is the repair endpoint body: the
// canonical template plus the drift IDs to repair. Empty IDs repair every
// repairable row.
type ConfigSetConformanceRepairRequest struct {
	Template ConfigSetTemplate `json:"template"`
	IDs      []string          `json:"ids,omitempty"`
}

// driftID builds the stable row key. Path segments are lowercased so IDs
// survive cosmetic casing changes between runs.
func driftID(section, kind, path string) string {
	return section + "/" + kind + "/" + strings.ToLower(path)
}

func drift(section, kind, name, detail string, repairable bool) ConfigSetConformanceDrift {
	return ConfigSetConformanceDrift{
		ID:         driftID(section, kind, name),
		Section:    section,
		Kind:       kind,
		Name:       name,
		Detail:     detail,
		Repairable: repairable,
	}
}

// DiffConformanceTemplates compares a canonical template against the live
// export of a configuration set and returns every concrete difference.
// Both sides use the same export coordinate system, so sections absent from
// either side are simply empty.
func DiffConformanceTemplates(canonical, live *ConfigSetTemplate) []ConfigSetConformanceDrift {
	c, l := &canonical.Payload, &live.Payload
	drifts := make([]ConfigSetConformanceDrift, 0, 16)

	drifts = append(drifts, diffStatusSections(c.Statuses, l.Statuses)...)
	drifts = append(drifts, diffWorkflowSections(c.Workflows, l.Workflows)...)
	drifts = append(drifts, diffScreenSections(c.Screens, l.Screens)...)
	drifts = append(drifts, diffCustomFieldSections(c.CustomFields, l.CustomFields)...)
	drifts = append(drifts, diffPrioritySections(c.Priorities, l.Priorities)...)
	drifts = append(drifts, diffItemTypeSections(c.ItemTypes, l.ItemTypes)...)
	drifts = append(drifts, diffConditionSetSections(c.ConditionSets, l.ConditionSets)...)
	drifts = append(drifts, diffApprovalSetSections(c.ApprovalSets, l.ApprovalSets)...)
	drifts = append(drifts, diffLinkTypeSections(c.LinkTypes, l.LinkTypes)...)
	drifts = append(drifts, diffLinksSections(&c.Links, &l.Links, c.ConfigurationSet.DefaultItemTypeName, l.ConfigurationSet.DefaultItemTypeName)...)

	return drifts
}

// ---- statuses ---------------------------------------------------------------

func diffStatusSections(canonical, live []ConfigSetTplStatus) []ConfigSetConformanceDrift {
	liveByName := statusIndex(live)
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("statuses", ConformanceKindMissing, want.Name,
				fmt.Sprintf("status %q is missing", want.Name), true))
			continue
		}
		if got.CategoryName != want.CategoryName {
			drifts = append(drifts, drift("statuses", ConformanceKindDiffering, want.Name+"/category",
				fmt.Sprintf("status %q is in category %q, template requires %q", want.Name, got.CategoryName, want.CategoryName), true))
		}
		if got.Description != want.Description {
			drifts = append(drifts, drift("statuses", ConformanceKindDiffering, want.Name+"/description",
				fmt.Sprintf("status %q description differs from the template", want.Name), true))
		}
	}
	for _, got := range live {
		if _, ok := statusIndex(canonical)[lowerStr(got.Name)]; !ok {
			drifts = append(drifts, drift("statuses", ConformanceKindExtra, got.Name,
				fmt.Sprintf("status %q exists but is not in the template (reported only; statuses may hold item data)", got.Name), false))
		}
	}
	return drifts
}

func statusIndex(statuses []ConfigSetTplStatus) map[string]ConfigSetTplStatus {
	out := make(map[string]ConfigSetTplStatus, len(statuses))
	for _, st := range statuses {
		out[lowerStr(st.Name)] = st
	}
	return out
}

// ---- workflows ----------------------------------------------------------------

type transitionKey struct {
	from    string
	to      string
	fromAll bool
}

func workflowTransitionKey(t ConfigSetTplWorkflowTransition) transitionKey {
	return transitionKey{from: lowerStrPtr(t.FromStatusName), to: lowerStr(t.ToStatusName), fromAll: t.FromAllStatuses}
}

func diffWorkflowSections(canonical, live []ConfigSetTplWorkflow) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplWorkflow{}
	for _, wf := range live {
		liveByName[lowerStr(wf.Name)] = wf
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("workflows", ConformanceKindMissing, want.Name,
				fmt.Sprintf("workflow %q is missing", want.Name), true))
			continue
		}
		if got.Description != want.Description {
			drifts = append(drifts, drift("workflows", ConformanceKindDiffering, want.Name+"/description",
				fmt.Sprintf("workflow %q description differs from the template", want.Name), true))
		}
		gotTransitions := map[transitionKey]ConfigSetTplWorkflowTransition{}
		for _, t := range got.Transitions {
			gotTransitions[workflowTransitionKey(t)] = t
		}
		wantTransitions := map[transitionKey]ConfigSetTplWorkflowTransition{}
		for _, t := range want.Transitions {
			wantTransitions[workflowTransitionKey(t)] = t
		}
		for key, wt := range wantTransitions {
			liveT, ok := gotTransitions[key]
			if !ok {
				drifts = append(drifts, drift("workflows", ConformanceKindMissing, want.Name+"/transition/"+transitionPath(wt),
					fmt.Sprintf("workflow %q is missing the %s transition", want.Name, transitionLabel(wt)), true))
				continue
			}
			if liveT.DisplayOrder != wt.DisplayOrder || liveT.SourceHandle != wt.SourceHandle || liveT.TargetHandle != wt.TargetHandle {
				drifts = append(drifts, drift("workflows", ConformanceKindDiffering,
					want.Name+"/transition/"+transitionPath(wt),
					fmt.Sprintf("workflow %q transition %s differs (order/handles changed)", want.Name, transitionLabel(wt)), true))
			}
		}
		for key := range gotTransitions {
			if _, ok := wantTransitions[key]; !ok {
				drifts = append(drifts, drift("workflows", ConformanceKindExtra, want.Name+"/transition/"+key.to,
					fmt.Sprintf("workflow %q has an extra transition into %q (reported only; other configuration may reference it)", want.Name, key.to), false))
			}
		}
	}
	for _, got := range live {
		found := false
		for _, want := range canonical {
			if lowerStr(want.Name) == lowerStr(got.Name) {
				found = true
				break
			}
		}
		if !found {
			drifts = append(drifts, drift("workflows", ConformanceKindExtra, got.Name,
				fmt.Sprintf("workflow %q exists but is not in the template (reported only; other configuration may reference it)", got.Name), false))
		}
	}
	return drifts
}

func transitionLabel(t ConfigSetTplWorkflowTransition) string {
	if t.FromAllStatuses {
		return fmt.Sprintf("from-all → %q", t.ToStatusName)
	}
	if t.FromStatusName == nil {
		return fmt.Sprintf("initial → %q", t.ToStatusName)
	}
	return fmt.Sprintf("%q → %q", *t.FromStatusName, t.ToStatusName)
}

func transitionPath(t ConfigSetTplWorkflowTransition) string {
	if t.FromAllStatuses {
		return "all→" + t.ToStatusName
	}
	if t.FromStatusName == nil {
		return "initial→" + t.ToStatusName
	}
	return *t.FromStatusName + "→" + t.ToStatusName
}

// ---- screens ----------------------------------------------------------------

type screenFieldKey struct {
	kind string
	key  string // field_identifier, or the custom field name for kind=custom
}

func screenFieldKeyOf(f ConfigSetTplScreenField) screenFieldKey {
	if f.FieldKind == "custom" {
		return screenFieldKey{kind: "custom", key: lowerStr(f.CustomFieldName)}
	}
	return screenFieldKey{kind: f.FieldKind, key: lowerStr(f.FieldIdentifier)}
}

func diffScreenSections(canonical, live []ConfigSetTplScreen) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplScreen{}
	for _, sc := range live {
		liveByName[lowerStr(sc.Name)] = sc
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("screens", ConformanceKindMissing, want.Name,
				fmt.Sprintf("screen %q is missing", want.Name), true))
			continue
		}
		gotFields := map[screenFieldKey]ConfigSetTplScreenField{}
		for _, f := range got.Fields {
			gotFields[screenFieldKeyOf(f)] = f
		}
		wantFields := map[screenFieldKey]ConfigSetTplScreenField{}
		for _, f := range want.Fields {
			wantFields[screenFieldKeyOf(f)] = f
		}
		for key, wf := range wantFields {
			liveF, ok := gotFields[key]
			if !ok {
				drifts = append(drifts, drift("screens", ConformanceKindMissing, want.Name+"/field/"+key.kind+":"+key.key,
					fmt.Sprintf("screen %q is missing the %s field %q", want.Name, key.kind, key.key), true))
				continue
			}
			if liveF.DisplayOrder != wf.DisplayOrder || liveF.IsRequired != wf.IsRequired || liveF.FieldWidth != wf.FieldWidth {
				drifts = append(drifts, drift("screens", ConformanceKindDiffering, want.Name+"/field/"+key.kind+":"+key.key,
					fmt.Sprintf("screen %q field %q differs (order/required/width changed)", want.Name, key.key), true))
			}
		}
		for key := range gotFields {
			if _, ok := wantFields[key]; !ok {
				drifts = append(drifts, drift("screens", ConformanceKindExtra, want.Name+"/field/"+key.kind+":"+key.key,
					fmt.Sprintf("screen %q has an extra field %q", want.Name, key.key), true))
			}
		}
		wantSystem := map[string]bool{}
		for _, sf := range want.SystemFields {
			wantSystem[strings.ToLower(sf)] = true
		}
		gotSystem := map[string]bool{}
		for _, sf := range got.SystemFields {
			gotSystem[strings.ToLower(sf)] = true
		}
		for sf := range wantSystem {
			if !gotSystem[sf] {
				drifts = append(drifts, drift("screens", ConformanceKindMissing, want.Name+"/system/"+sf,
					fmt.Sprintf("screen %q is missing system field %q", want.Name, sf), true))
			}
		}
		for sf := range gotSystem {
			if !wantSystem[sf] {
				drifts = append(drifts, drift("screens", ConformanceKindExtra, want.Name+"/system/"+sf,
					fmt.Sprintf("screen %q has an extra system field %q", want.Name, sf), true))
			}
		}
	}
	for _, got := range live {
		found := false
		for _, want := range canonical {
			if lowerStr(want.Name) == lowerStr(got.Name) {
				found = true
				break
			}
		}
		if !found {
			drifts = append(drifts, drift("screens", ConformanceKindExtra, got.Name,
				fmt.Sprintf("screen %q exists but is not in the template (reported only; other configuration may reference it)", got.Name), false))
		}
	}
	return drifts
}

// ---- custom fields ---------------------------------------------------------------

func diffCustomFieldSections(canonical, live []ConfigSetTplCustomField) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplCustomField{}
	for _, cf := range live {
		liveByName[lowerStr(cf.Name)] = cf
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("custom_fields", ConformanceKindMissing, want.Name,
				fmt.Sprintf("custom field %q is missing", want.Name), true))
			continue
		}
		var diffs []string
		if models.CanonicalCustomFieldType(got.FieldType) != models.CanonicalCustomFieldType(want.FieldType) {
			diffs = append(diffs, fmt.Sprintf("type %q → %q", got.FieldType, want.FieldType))
		}
		if got.Description != want.Description {
			diffs = append(diffs, "description")
		}
		if got.Required != want.Required {
			diffs = append(diffs, fmt.Sprintf("required %t → %t", got.Required, want.Required))
		}
		if got.Options != want.Options {
			diffs = append(diffs, "options")
		}
		if got.DisplayOrder != want.DisplayOrder {
			diffs = append(diffs, fmt.Sprintf("display order %d → %d", got.DisplayOrder, want.DisplayOrder))
		}
		if got.AppliesToPortalCustomers != want.AppliesToPortalCustomers || got.AppliesToCustomerOrganisations != want.AppliesToCustomerOrganisations {
			diffs = append(diffs, "portal/organisation visibility")
		}
		if len(diffs) > 0 {
			drifts = append(drifts, drift("custom_fields", ConformanceKindDiffering, want.Name,
				fmt.Sprintf("custom field %q differs: %s", want.Name, strings.Join(diffs, ", ")), true))
		}
	}
	for _, got := range live {
		inCanonical := false
		for _, want := range canonical {
			if lowerStr(want.Name) == lowerStr(got.Name) {
				inCanonical = true
				break
			}
		}
		if !inCanonical {
			drifts = append(drifts, drift("custom_fields", ConformanceKindExtra, got.Name,
				fmt.Sprintf("custom field %q exists but is not in the template (reported only; items may hold values for it)", got.Name), false))
		}
	}
	return drifts
}

// ---- priorities -------------------------------------------------------------

func diffPrioritySections(canonical, live []ConfigSetTplPriority) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplPriority{}
	for _, p := range live {
		liveByName[lowerStr(p.Name)] = p
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("priorities", ConformanceKindMissing, want.Name,
				fmt.Sprintf("priority %q is missing", want.Name), true))
			continue
		}
		var diffs []string
		if got.Description != want.Description {
			diffs = append(diffs, "description")
		}
		if got.Icon != want.Icon || got.Color != want.Color {
			diffs = append(diffs, "icon/color")
		}
		if got.SortOrder != want.SortOrder {
			diffs = append(diffs, fmt.Sprintf("sort order %d → %d", got.SortOrder, want.SortOrder))
		}
		if len(diffs) > 0 {
			drifts = append(drifts, drift("priorities", ConformanceKindDiffering, want.Name,
				fmt.Sprintf("priority %q differs: %s", want.Name, strings.Join(diffs, ", ")), true))
		}
	}
	for _, got := range live {
		if _, ok := liveByName[lowerStr(got.Name)]; !ok {
			drifts = append(drifts, drift("priorities", ConformanceKindExtra, got.Name,
				fmt.Sprintf("priority %q exists but is not in the template (reported only; items may use it)", got.Name), false))
		}
	}
	return drifts
}

// ---- item types -------------------------------------------------------------

func diffItemTypeSections(canonical, live []ConfigSetTplItemType) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplItemType{}
	for _, it := range live {
		liveByName[lowerStr(it.Name)] = it
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("item_types", ConformanceKindMissing, want.Name,
				fmt.Sprintf("item type %q is missing", want.Name), true))
			continue
		}
		var diffs []string
		if got.Description != want.Description {
			diffs = append(diffs, "description")
		}
		if got.Icon != want.Icon || got.Color != want.Color {
			diffs = append(diffs, "icon/color")
		}
		if got.HierarchyLevel != want.HierarchyLevel {
			diffs = append(diffs, fmt.Sprintf("hierarchy level %d → %d", got.HierarchyLevel, want.HierarchyLevel))
		}
		if got.SortOrder != want.SortOrder {
			diffs = append(diffs, fmt.Sprintf("sort order %d → %d", got.SortOrder, want.SortOrder))
		}
		if len(diffs) > 0 {
			drifts = append(drifts, drift("item_types", ConformanceKindDiffering, want.Name,
				fmt.Sprintf("item type %q differs: %s", want.Name, strings.Join(diffs, ", ")), true))
		}
	}
	for _, got := range live {
		if _, ok := liveByName[lowerStr(got.Name)]; !ok {
			drifts = append(drifts, drift("item_types", ConformanceKindExtra, got.Name,
				fmt.Sprintf("item type %q exists but is not in the template (reported only; items may use it)", got.Name), false))
		}
	}
	return drifts
}

// ---- condition sets ---------------------------------------------------------------

type conditionBindingKey struct {
	from    string
	to      string
	fromAll bool
}

func conditionBindingKeyOf(tc ConfigSetTplTransitionCondition) conditionBindingKey {
	return conditionBindingKey{from: lowerStrPtr(tc.FromStatusName), to: lowerStr(tc.ToStatusName), fromAll: tc.FromAllStatuses}
}

func conditionBindingPath(tc ConfigSetTplTransitionCondition) string {
	if tc.FromAllStatuses {
		return "all→" + tc.ToStatusName
	}
	if tc.FromStatusName == nil {
		return "initial→" + tc.ToStatusName
	}
	return *tc.FromStatusName + "→" + tc.ToStatusName
}

// conditionBindingSignature captures a binding's logic mode and conditions so
// structural equality can be tested with one DeepEqual.
func conditionBindingSignature(tc ConfigSetTplTransitionCondition) map[string]any {
	conditions := make([]map[string]any, 0, len(tc.Conditions))
	for _, c := range tc.Conditions {
		conditions = append(conditions, map[string]any{
			"type": c.Type, "mode": c.Mode, "display_order": c.DisplayOrder,
			"error_message": c.ErrorMessage, "config": c.Config,
		})
	}
	return map[string]any{"logic_mode": tc.LogicMode, "conditions": conditions}
}

func diffConditionSetSections(canonical, live []ConfigSetTplConditionSet) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplConditionSet{}
	for _, set := range live {
		liveByName[lowerStr(set.Name)] = set
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("condition_sets", ConformanceKindMissing, want.Name,
				fmt.Sprintf("condition set %q is missing", want.Name), true))
			continue
		}
		if got.WorkflowName != want.WorkflowName {
			drifts = append(drifts, drift("condition_sets", ConformanceKindDiffering, want.Name+"/workflow",
				fmt.Sprintf("condition set %q is bound to workflow %q, template requires %q", want.Name, got.WorkflowName, want.WorkflowName), true))
		}
		gotBindings := map[conditionBindingKey]ConfigSetTplTransitionCondition{}
		for _, tc := range got.TransitionConditions {
			gotBindings[conditionBindingKeyOf(tc)] = tc
		}
		wantBindings := map[conditionBindingKey]ConfigSetTplTransitionCondition{}
		for _, tc := range want.TransitionConditions {
			wantBindings[conditionBindingKeyOf(tc)] = tc
		}
		for key, wt := range wantBindings {
			liveT, ok := gotBindings[key]
			if !ok {
				drifts = append(drifts, drift("condition_sets", ConformanceKindMissing, want.Name+"/binding/"+conditionBindingPath(wt),
					fmt.Sprintf("condition set %q is missing the %s conditions", want.Name, conditionBindingPath(wt)), true))
				continue
			}
			if !deepEqualJSON(conditionBindingSignature(wt), conditionBindingSignature(liveT)) {
				drifts = append(drifts, drift("condition_sets", ConformanceKindDiffering, want.Name+"/binding/"+conditionBindingPath(wt),
					fmt.Sprintf("condition set %q conditions for %s differ from the template", want.Name, conditionBindingPath(wt)), true))
			}
		}
		for key := range gotBindings {
			if _, ok := wantBindings[key]; !ok {
				drifts = append(drifts, drift("condition_sets", ConformanceKindExtra, want.Name+"/binding/"+key.to,
					fmt.Sprintf("condition set %q has extra conditions for %q (repair removes them)", want.Name, key.to), true))
			}
		}
	}
	for _, got := range live {
		if _, ok := liveByName[lowerStr(got.Name)]; !ok {
			drifts = append(drifts, drift("condition_sets", ConformanceKindExtra, got.Name,
				fmt.Sprintf("condition set %q exists but is not in the template (reported only; other configuration may reference it)", got.Name), false))
		}
	}
	return drifts
}

// ---- approval sets ---------------------------------------------------------------

func approvalSetStatusSignature(ss ConfigSetTplApprovalSetStatus) map[string]any {
	steps := make([]map[string]any, 0, len(ss.Steps))
	for _, s := range ss.Steps {
		steps = append(steps, approvalStepSignature(s))
	}
	return map[string]any{
		"step_mode":          ss.StepMode,
		"approve_transition": transitionKeyOf(ss.ApproveTransition),
		"deny_transition":    transitionKeyOf(ss.DenyTransition),
		"steps":              steps,
	}
}

func transitionKeyOf(ref ConfigSetTplTransitionRef) map[string]any {
	return map[string]any{"from": lowerStrPtr(ref.FromStatusName), "to": ref.ToStatusName, "all": ref.FromAllStatuses}
}

func approvalStepSignature(s ConfigSetTplApprovalStep) map[string]any {
	return map[string]any{
		"display_order": s.DisplayOrder, "name": s.Name,
		"quorum_mode": s.QuorumMode, "quorum_count": s.QuorumCount, "quorum_percent": s.QuorumPercent,
		"rejection_policy": s.RejectionPolicy,
		"approver": map[string]any{
			"source": s.ApproverSource, "field": s.ApproverFieldIdentifier, "custom_field": s.ApproverCustomFieldName,
			"role": s.ApproverRoleName, "group": s.ApproverGroupName, "user": s.ApproverUserEmail,
			"self_approval": s.AllowSelfApproval,
		},
		"on_leave": s.OnLeaveStrategy,
		"escalation": map[string]any{
			"after_hours": s.EscalationAfterHours, "action": s.EscalationAction,
			"target_source": s.EscalationTargetSource, "target_field": s.EscalationTargetFieldIdentifier,
			"target_custom_field": s.EscalationTargetCustomFieldName, "target_role": s.EscalationTargetRoleName,
			"target_group": s.EscalationTargetGroupName, "target_user": s.EscalationTargetUserEmail,
			"max_escalations": s.MaxEscalations,
		},
	}
}

func diffApprovalSetSections(canonical, live []ConfigSetTplApprovalSet) []ConfigSetConformanceDrift {
	liveByName := map[string]ConfigSetTplApprovalSet{}
	for _, set := range live {
		liveByName[lowerStr(set.Name)] = set
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("approval_sets", ConformanceKindMissing, want.Name,
				fmt.Sprintf("approval set %q is missing", want.Name), true))
			continue
		}
		if got.WorkflowName != want.WorkflowName {
			drifts = append(drifts, drift("approval_sets", ConformanceKindDiffering, want.Name+"/workflow",
				fmt.Sprintf("approval set %q is bound to workflow %q, template requires %q", want.Name, got.WorkflowName, want.WorkflowName), true))
		}
		gotByStatus := map[string]ConfigSetTplApprovalSetStatus{}
		for _, ss := range got.SetStatuses {
			gotByStatus[lowerStr(ss.StatusName)] = ss
		}
		wantByStatus := map[string]ConfigSetTplApprovalSetStatus{}
		for _, ss := range want.SetStatuses {
			wantByStatus[lowerStr(ss.StatusName)] = ss
		}
		for name, wt := range wantByStatus {
			liveSS, ok := gotByStatus[name]
			if !ok {
				drifts = append(drifts, drift("approval_sets", ConformanceKindMissing, want.Name+"/status/"+name,
					fmt.Sprintf("approval set %q is missing the approval configuration for status %q", want.Name, wt.StatusName), true))
				continue
			}
			if !deepEqualJSON(approvalSetStatusSignature(wt), approvalSetStatusSignature(liveSS)) {
				drifts = append(drifts, drift("approval_sets", ConformanceKindDiffering, want.Name+"/status/"+name,
					fmt.Sprintf("approval set %q approval configuration for status %q differs from the template", want.Name, wt.StatusName), true))
			}
		}
		for name := range gotByStatus {
			if _, ok := wantByStatus[name]; !ok {
				drifts = append(drifts, drift("approval_sets", ConformanceKindExtra, want.Name+"/status/"+name,
					fmt.Sprintf("approval set %q has an extra approval configuration for status %q (repair removes it unless a live approval request depends on it)", want.Name, name), true))
			}
		}
	}
	for _, got := range live {
		if _, ok := liveByName[lowerStr(got.Name)]; !ok {
			drifts = append(drifts, drift("approval_sets", ConformanceKindExtra, got.Name,
				fmt.Sprintf("approval set %q exists but is not in the template (reported only; other configuration may reference it)", got.Name), false))
		}
	}
	return drifts
}

// ---- link types ---------------------------------------------------------------

func diffLinkTypeSections(canonical, live []ConfigSetTplLinkType) []ConfigSetConformanceDrift {
	// Asymmetric on purpose: the template lists the link types a pack
	// requires; the registry may carry additional types without drifting.
	liveByName := map[string]ConfigSetTplLinkType{}
	for _, lt := range live {
		liveByName[lowerStr(lt.Name)] = lt
	}
	var drifts []ConfigSetConformanceDrift
	for _, want := range canonical {
		got, ok := liveByName[lowerStr(want.Name)]
		if !ok {
			drifts = append(drifts, drift("link_types", ConformanceKindMissing, want.Name,
				fmt.Sprintf("link type %q is missing", want.Name), true))
			continue
		}
		// Live rows carry an empty Active flag in the template coordinate
		// system only if the exporter filtered them; the export emits active
		// types, so a missing-but-known type shows up as "missing" above.
		normalizedWant := normalizeTplLinkType(want)
		normalizedGot := normalizeTplLinkType(got)
		var diffs []string
		if normalizedWant.Description != normalizedGot.Description {
			diffs = append(diffs, "description")
		}
		if normalizedWant.ForwardLabel != normalizedGot.ForwardLabel {
			diffs = append(diffs, fmt.Sprintf("forward label %q → %q", normalizedGot.ForwardLabel, normalizedWant.ForwardLabel))
		}
		if normalizedWant.ReverseLabel != normalizedGot.ReverseLabel {
			diffs = append(diffs, fmt.Sprintf("reverse label %q → %q", normalizedGot.ReverseLabel, normalizedWant.ReverseLabel))
		}
		if normalizedWant.Color != normalizedGot.Color {
			diffs = append(diffs, fmt.Sprintf("color %s → %s", normalizedGot.Color, normalizedWant.Color))
		}
		if strings.Join(normalizedWant.AllowedEntityTypes, ",") != strings.Join(normalizedGot.AllowedEntityTypes, ",") {
			diffs = append(diffs, "allowed entity types")
		}
		if len(diffs) > 0 {
			drifts = append(drifts, drift("link_types", ConformanceKindDiffering, want.Name,
				fmt.Sprintf("link type %q differs: %s", want.Name, strings.Join(diffs, ", ")), true))
		}
	}
	return drifts
}

// ---- configuration set glue ---------------------------------------------------------------

func diffLinksSections(canonical, live *ConfigSetTplLinks, canonicalDefaultItemType, liveDefaultItemType string) []ConfigSetConformanceDrift {
	var drifts []ConfigSetConformanceDrift
	compare := func(label, want, got string) {
		if want != got {
			drifts = append(drifts, drift("links", ConformanceKindDiffering, label,
				fmt.Sprintf("%s is %q, template requires %q", label, got, want), true))
		}
	}
	compare("workflow", canonical.WorkflowName, live.WorkflowName)
	compare("condition set", canonical.ConditionSetName, live.ConditionSetName)
	compare("approval set", canonical.ApprovalSetName, live.ApprovalSetName)
	compare("create screen", canonical.CreateScreenName, live.CreateScreenName)
	compare("edit screen", canonical.EditScreenName, live.EditScreenName)
	compare("view screen", canonical.ViewScreenName, live.ViewScreenName)
	compare("default item type", canonicalDefaultItemType, liveDefaultItemType)

	wantPriorities := map[string]bool{}
	for _, p := range canonical.PriorityNames {
		wantPriorities[lowerStr(p)] = true
	}
	gotPriorities := map[string]bool{}
	for _, p := range live.PriorityNames {
		gotPriorities[lowerStr(p)] = true
	}
	for p := range wantPriorities {
		if !gotPriorities[p] {
			drifts = append(drifts, drift("links", ConformanceKindMissing, "priority/"+p,
				fmt.Sprintf("priority %q is not assigned to the configuration set", p), true))
		}
	}
	for p := range gotPriorities {
		if !wantPriorities[p] {
			drifts = append(drifts, drift("links", ConformanceKindExtra, "priority/"+p,
				fmt.Sprintf("priority %q is assigned but not in the template (repair removes the assignment; items keep their priority values)", p), true))
		}
	}

	liveConfigs := map[string]ConfigSetTplItemTypeConfig{}
	for _, itc := range live.ItemTypeConfigs {
		liveConfigs[lowerStr(itc.ItemTypeName)] = itc
	}
	wantConfigs := map[string]ConfigSetTplItemTypeConfig{}
	for _, itc := range canonical.ItemTypeConfigs {
		wantConfigs[lowerStr(itc.ItemTypeName)] = itc
	}
	for name, wt := range wantConfigs {
		got, ok := liveConfigs[name]
		if !ok {
			drifts = append(drifts, drift("links", ConformanceKindMissing, "item_type_config/"+name,
				fmt.Sprintf("item type %q has no per-type configuration", name), true))
			continue
		}
		for _, row := range [][3]string{
			{"workflow", wt.WorkflowName, got.WorkflowName},
			{"condition set", wt.ConditionSetName, got.ConditionSetName},
			{"approval set", wt.ApprovalSetName, got.ApprovalSetName},
			{"create screen", wt.CreateScreenName, got.CreateScreenName},
			{"edit screen", wt.EditScreenName, got.EditScreenName},
			{"view screen", wt.ViewScreenName, got.ViewScreenName},
		} {
			if row[1] != row[2] {
				drifts = append(drifts, drift("links", ConformanceKindDiffering, "item_type_config/"+name+"/"+row[0],
					fmt.Sprintf("item type %q override %s is %q, template requires %q", name, row[0], row[2], row[1]), true))
			}
		}
	}
	for name := range liveConfigs {
		if _, ok := wantConfigs[name]; !ok {
			drifts = append(drifts, drift("links", ConformanceKindExtra, "item_type_config/"+name,
				fmt.Sprintf("item type %q has a per-type configuration that is not in the template (repair removes it)", name), true))
		}
	}
	return drifts
}

// ---- helpers ---------------------------------------------------------------

func deepEqualJSON(a, b any) bool {
	ra, err := json.Marshal(a)
	if err != nil {
		return false
	}
	rb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ra, rb)
}
