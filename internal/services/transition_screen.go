package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// transitionScreenSystemFieldKeys maps a screen system-field identifier to the
// item update keys that satisfy it. The screen editor uses short identifiers
// (priority, assignee) while the item API uses _id keys.
var transitionScreenSystemFieldKeys = map[string][]string{
	"title":            {"title"},
	"description":      {"description"},
	"priority":         {"priority_id"},
	"assignee":         {"assignee_id"},
	"team":             {"team_id"},
	"iteration":        {"iteration_id"},
	"project":          {"project_id"},
	"due_date":         {"due_date"},
	"start_date":       {"start_date"},
	"end_date":         {"end_date"},
	"story_points":     {"story_points"},
	"estimate":         {"estimate_minutes"},
	"estimate_minutes": {"estimate_minutes"},
	"milestone":        {"milestone_ids"},
}

// resolveGoverningTransitionID returns the transition row that governs a move:
// a direct edge wins over a from-all row. Returns 0 when the item has no
// workflow or no matching edge (personal workspaces).
func (s *WorkflowService) resolveGoverningTransitionID(workspaceID int, itemTypeID *int, fromStatusID, toStatusID int64) (int, error) {
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return 0, err
	}
	if workflowID == nil {
		return 0, nil
	}
	var transitionID int
	err = s.db.QueryRow(`
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ?
		  AND to_status_id = ?
		  AND (from_status_id = ? OR from_all_statuses = TRUE)
		ORDER BY CASE WHEN from_status_id IS NULL THEN 1 ELSE 0 END, display_order
		LIMIT 1
	`, *workflowID, toStatusID, fromStatusID).Scan(&transitionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve governing transition: %w", err)
	}
	return transitionID, nil
}

// validateTransitionScreenFields returns the required screen fields that the
// merged state (stored item + submitted values) does not satisfy.
func (s *WorkflowService) validateTransitionScreenFields(item *models.Item, fields []repository.TransitionScreenField, submitted map[string]any) ([]map[string]any, error) {
	var missing []map[string]any
	for _, field := range fields {
		if !field.IsRequired {
			continue
		}
		satisfied, err := s.transitionScreenFieldSatisfied(item, field, submitted)
		if err != nil {
			return nil, err
		}
		if !satisfied {
			missing = append(missing, map[string]any{
				"identifier": field.FieldIdentifier,
				"field_type": field.FieldType,
			})
		}
	}
	return missing, nil
}

func (s *WorkflowService) transitionScreenFieldSatisfied(item *models.Item, field repository.TransitionScreenField, submitted map[string]any) (bool, error) {
	if field.FieldType == "custom" {
		return transitionCustomFieldValuePresent(item, field.FieldIdentifier, submitted), nil
	}
	// A submitted value always wins over the stored row.
	if keys, ok := transitionScreenSystemFieldKeys[field.FieldIdentifier]; ok && submitted != nil {
		for _, key := range keys {
			if value, ok := submitted[key]; ok && !screenValueEmpty(value) {
				return true, nil
			}
		}
	}
	switch field.FieldIdentifier {
	case "milestone":
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM item_milestones WHERE item_id = ?", item.ID).Scan(&count); err != nil {
			return false, fmt.Errorf("check item milestones: %w", err)
		}
		return count > 0, nil
	case "labels":
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM item_labels WHERE item_id = ?", item.ID).Scan(&count); err != nil {
			return false, fmt.Errorf("check item labels: %w", err)
		}
		return count > 0, nil
	case "comment":
		if submitted != nil {
			if value, ok := submitted["comment"].(string); ok && strings.TrimSpace(value) != "" {
				return true, nil
			}
		}
		return false, nil
	}
	return screenItemFieldPresent(item, field.FieldIdentifier), nil
}

// overlayTransitionFields merges submitted transition-screen values onto the
// condition context so validators see the values being submitted, not the
// stale row.
func overlayTransitionFields(ctx, fields map[string]any) {
	if len(fields) == 0 {
		return
	}
	for key, value := range fields {
		if key == "custom_field_values" || key == "comment" {
			continue
		}
		ctx[key] = value
	}
	submittedCustom, ok := fields["custom_field_values"].(map[string]any)
	if !ok || len(submittedCustom) == 0 {
		return
	}
	merged := map[string]any{}
	if existing, ok := ctx["custom_fields"].(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	for key, value := range submittedCustom {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	ctx["custom_fields"] = merged
}

// screenUpdateData strips transition-only keys (comment) from the submitted
// fields so the item update validator never sees a key it does not own.
func screenUpdateData(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		if key == "comment" {
			continue
		}
		out[key] = value
	}
	return out
}

func screenValueEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case []int:
		return len(v) == 0
	default:
		return false
	}
}

func screenItemFieldPresent(item *models.Item, identifier string) bool {
	switch identifier {
	case "title":
		return strings.TrimSpace(item.Title) != ""
	case "description":
		return strings.TrimSpace(item.Description) != ""
	case "status":
		return item.StatusID != nil
	case "priority":
		return item.PriorityID != nil
	case "assignee":
		return item.AssigneeID != nil
	case "team":
		return item.TeamID != nil
	case "iteration":
		return item.IterationID != nil
	case "project":
		return item.ProjectID != nil
	case "due_date":
		return item.DueDate != nil
	case "start_date":
		return item.StartDate != nil
	case "end_date":
		return item.EndDate != nil
	case "story_points":
		return item.StoryPoints != nil
	case "estimate", "estimate_minutes":
		return item.EstimateMinutes != nil
	}
	// Unknown identifiers are not gates: a screen can reference a field this
	// slice does not model without blocking every transition.
	return true
}

func transitionCustomFieldValuePresent(item *models.Item, identifier string, submitted map[string]any) bool {
	if submitted != nil {
		if cfv, ok := submitted["custom_field_values"].(map[string]any); ok {
			if value, ok := cfv[identifier]; ok && !screenValueEmpty(value) {
				return true
			}
		}
	}
	if item.CustomFieldValues != nil {
		if value, ok := item.CustomFieldValues[identifier]; ok && !screenValueEmpty(value) {
			return true
		}
	}
	return false
}
