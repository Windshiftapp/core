package services

import (
	"encoding/json"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/sanitize"
)

// SanitizeConfigSetTemplate applies the same field policies as live CRUD to
// an uploaded template document. JSON option blobs are size-validated, not
// scrubbed, to preserve their shape. A returned error means the document is
// structurally invalid (missing required labels, oversized option payload)
// and import must not proceed.
func SanitizeConfigSetTemplate(tpl *ConfigSetTemplate) error {
	if tpl.ExportedBy != nil {
		SanitizeConfigSetExportBy(tpl.ExportedBy)
	}
	p := &tpl.Payload
	sanitize.ApplyAll(
		sanitize.Pair{Target: &p.ConfigurationSet.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &p.ConfigurationSet.Description, Policy: sanitize.RichText},
		sanitize.Pair{Target: &p.ConfigurationSet.DefaultItemTypeName, Policy: sanitize.PlainTextField},
	)
	for i := range p.StatusCategories {
		sanitize.Apply(&p.StatusCategories[i].Name, sanitize.PlainTextField)
	}
	for i := range p.CustomFields {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &p.CustomFields[i].Name, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &p.CustomFields[i].FieldType, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &p.CustomFields[i].Description, Policy: sanitize.Comment},
		)
		if err := sanitize.ValidateJSONPayload(
			fmt.Sprintf("custom_fields[%d].options", i), p.CustomFields[i].Options,
		); err != nil {
			return err
		}
	}
	for i := range p.Statuses {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &p.Statuses[i].Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.Statuses[i].Description, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.Statuses[i].CategoryName, Policy: sanitize.PlainTextField},
		)
	}
	for i := range p.ItemTypes {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &p.ItemTypes[i].Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.ItemTypes[i].Description, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.ItemTypes[i].Icon, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &p.ItemTypes[i].Color, Policy: sanitize.ShortIdentifier},
		)
	}
	for i := range p.Priorities {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &p.Priorities[i].Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.Priorities[i].Description, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &p.Priorities[i].Icon, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &p.Priorities[i].Color, Policy: sanitize.ShortIdentifier},
		)
	}
	for i := range p.LinkTypes {
		lt := &p.LinkTypes[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &lt.Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &lt.Description, Policy: sanitize.RichText},
			sanitize.Pair{Target: &lt.ForwardLabel, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &lt.ReverseLabel, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &lt.Color, Policy: sanitize.ShortIdentifier},
		)
		for j := range lt.AllowedEntityTypes {
			sanitize.Apply(&lt.AllowedEntityTypes[j], sanitize.ShortIdentifier)
		}
		// A value made entirely of stripped markup has no meaning left; keep
		// the vocabulary clean instead of persisting empty entries.
		entityTypes := lt.AllowedEntityTypes[:0]
		for _, et := range lt.AllowedEntityTypes {
			if et != "" {
				entityTypes = append(entityTypes, et)
			}
		}
		lt.AllowedEntityTypes = entityTypes
		// Mirror the live validateLinkType contract: an imported row the link
		// type editor would reject must not reach the registry.
		if lt.Name == "" || lt.ForwardLabel == "" || lt.ReverseLabel == "" {
			return fmt.Errorf("link_types[%d]: name, forward_label, and reverse_label are required", i)
		}
	}
	for i := range p.Screens {
		sc := &p.Screens[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &sc.Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &sc.Description, Policy: sanitize.RichText},
		)
		for j := range sc.Fields {
			sanitize.ApplyAll(
				sanitize.Pair{Target: &sc.Fields[j].FieldKind, Policy: sanitize.ShortIdentifier},
				sanitize.Pair{Target: &sc.Fields[j].FieldIdentifier, Policy: sanitize.ShortIdentifier},
				sanitize.Pair{Target: &sc.Fields[j].CustomFieldName, Policy: sanitize.ShortIdentifier},
				sanitize.Pair{Target: &sc.Fields[j].FieldWidth, Policy: sanitize.ShortIdentifier},
			)
		}
		for j := range sc.SystemFields {
			sanitize.Apply(&sc.SystemFields[j], sanitize.ShortIdentifier)
		}
	}
	for i := range p.Workflows {
		wf := &p.Workflows[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &wf.Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &wf.Description, Policy: sanitize.RichText},
		)
		for j := range wf.Transitions {
			t := &wf.Transitions[j]
			sanitize.ApplyAll(
				sanitize.Pair{Target: t.FromStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &t.ToStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &t.SourceHandle, Policy: sanitize.ShortIdentifier},
				sanitize.Pair{Target: &t.TargetHandle, Policy: sanitize.ShortIdentifier},
			)
		}
	}
	for i := range p.ConditionSets {
		cs := &p.ConditionSets[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &cs.Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &cs.Description, Policy: sanitize.RichText},
			sanitize.Pair{Target: &cs.WorkflowName, Policy: sanitize.PlainTextField},
		)
		for j := range cs.TransitionConditions {
			tc := &cs.TransitionConditions[j]
			sanitize.ApplyAll(
				sanitize.Pair{Target: tc.FromStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &tc.ToStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &tc.LogicMode, Policy: sanitize.ShortIdentifier},
			)
			for k := range tc.Conditions {
				sanitizeConfigSetCondition(&tc.Conditions[k])
			}
		}
	}
	for i := range p.ApprovalSets {
		as := &p.ApprovalSets[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &as.Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &as.Description, Policy: sanitize.RichText},
			sanitize.Pair{Target: &as.WorkflowName, Policy: sanitize.PlainTextField},
		)
		for j := range as.SetStatuses {
			ss := &as.SetStatuses[j]
			sanitize.ApplyAll(
				sanitize.Pair{Target: &ss.StatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &ss.StepMode, Policy: sanitize.ShortIdentifier},
				sanitize.Pair{Target: ss.ApproveTransition.FromStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &ss.ApproveTransition.ToStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: ss.DenyTransition.FromStatusName, Policy: sanitize.PlainTextField},
				sanitize.Pair{Target: &ss.DenyTransition.ToStatusName, Policy: sanitize.PlainTextField},
			)
			for k := range ss.Steps {
				sanitizeConfigSetApprovalStep(&ss.Steps[k])
			}
		}
	}
	sanitizeConfigSetLinks(&p.Links)
	return nil
}

// configSetConditionScriptMaxBytes mirrors the live validateCondition cap on
// script-condition bodies (condition_sets.go) so an imported bundle cannot
// plant a script larger than the condition editor would ever accept.
const configSetConditionScriptMaxBytes = 10240

// configSetConditionConfigMaxBytes bounds the serialized size of one
// condition's free-form Config blob. Legitimate configs are tiny (a couple
// of IDs / a field ref + pattern / a <=10KB script), so 64 KiB is generous
// headroom while keeping a hostile bundle from parking megabytes in
// conditions.config.
const configSetConditionConfigMaxBytes = 64 * 1024

// sanitizeConfigSetCondition bounds one workflow condition: the type / mode
// machine tokens (live path allowlists these; import must at least bound
// them), the user-facing error message, and the free-form Config blob. A
// Config that still exceeds the byte cap after the script trim is dropped
// wholesale — there is no legitimate config that large, and an empty config
// fails closed in the condition engine rather than persisting megabytes.
func sanitizeConfigSetCondition(c *ConfigSetTplCondition) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &c.Type, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &c.Mode, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &c.ErrorMessage, Policy: sanitize.PlainTextField},
	)
	if c.Type == models.ConditionTypeScript {
		if script, ok := c.Config["script"].(string); ok && len(script) > configSetConditionScriptMaxBytes {
			c.Config["script"] = script[:configSetConditionScriptMaxBytes]
		}
	}
	if raw, err := json.Marshal(c.Config); err != nil || len(raw) > configSetConditionConfigMaxBytes {
		c.Config = map[string]any{}
	}
}

// sanitizeConfigSetApprovalStep scrubs the name / identifier / email refs on
// one approval step. Role + group names render in the approval chain editor
// and echo back via UnresolvedRef on a 422; field identifiers + emails are
// identifier-shaped. The quorum / policy / source machine tokens get
// ShortIdentifier because the import path bypasses the ApprovalSetService
// allowlists that bound them on the live path.
func sanitizeConfigSetApprovalStep(st *ConfigSetTplApprovalStep) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &st.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &st.QuorumMode, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.RejectionPolicy, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.ApproverSource, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.OnLeaveStrategy, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.EscalationAction, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.EscalationTargetSource, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.ApproverFieldIdentifier, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.ApproverCustomFieldName, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.ApproverRoleName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &st.ApproverGroupName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &st.ApproverUserEmail, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.EscalationTargetFieldIdentifier, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.EscalationTargetCustomFieldName, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &st.EscalationTargetRoleName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &st.EscalationTargetGroupName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &st.EscalationTargetUserEmail, Policy: sanitize.ShortIdentifier},
	)
}

// sanitizeConfigSetLinks scrubs the by-name glue section so its references
// keep matching the (equally sanitized) defining entities above.
func sanitizeConfigSetLinks(links *ConfigSetTplLinks) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &links.WorkflowName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &links.ConditionSetName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &links.ApprovalSetName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &links.CreateScreenName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &links.EditScreenName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &links.ViewScreenName, Policy: sanitize.PlainTextField},
	)
	for i := range links.PriorityNames {
		sanitize.Apply(&links.PriorityNames[i], sanitize.PlainTextField)
	}
	for i := range links.ItemTypeConfigs {
		itc := &links.ItemTypeConfigs[i]
		sanitize.ApplyAll(
			sanitize.Pair{Target: &itc.ItemTypeName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.WorkflowName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.ConditionSetName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.ApprovalSetName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.CreateScreenName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.EditScreenName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &itc.ViewScreenName, Policy: sanitize.PlainTextField},
		)
	}
}

// SanitizeConfigSetExportBy scrubs the provenance stamp. On export, Instance
// derives from the request Host header; on import the whole struct arrives
// inside the uploaded bundle.
func SanitizeConfigSetExportBy(by *ConfigSetExportBy) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &by.Username, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &by.Instance, Policy: sanitize.PlainTextField},
	)
}
