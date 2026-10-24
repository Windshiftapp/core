package sla

import (
	"windshift/internal/itemevents"
	"windshift/internal/models"
)

// matches reports whether a condition holds for a recorded fact.
func (c *compiledCondition) matches(fact itemevents.RecordedFact, config *compiledConfig) bool {
	switch c.typ {
	case conditionCreated:
		return fact.Type == itemevents.Created
	case conditionStatusEntered:
		return c.statusEntered(fact, config)
	case conditionStatusExited:
		return fact.Type == itemevents.StatusChanged && c.statusInSet(fact.OldStatusID, c.statusIDs)
	case conditionStatusCurrent:
		return c.statusInSet(fact.Snapshot.StatusID, c.statusIDs)
	case conditionStatusCategoryEntered:
		return c.categoryInSet(categoryOfStatus(fact.NewStatusID, fact, true, config), c.categoryIDs)
	case conditionStatusCategoryExited:
		return fact.Type == itemevents.StatusChanged && c.categoryInSet(categoryOfStatus(fact.OldStatusID, fact, false, config), c.categoryIDs)
	case conditionStatusCategoryCurrent:
		return c.categoryInSet(categoryOfStatus(fact.Snapshot.StatusID, fact, false, config), c.categoryIDs)
	case conditionResolutionSet:
		statusID := fact.NewStatusID
		if statusID == nil {
			statusID = fact.Snapshot.StatusID
		}
		if statusID == nil {
			return false
		}
		return config.statuses[*statusID].completed
	case conditionAssigneeSet:
		for _, change := range fact.Changes {
			if change.Field == "assignee_id" && change.NewValue != nil {
				return true
			}
		}
		return false
	case conditionCommentByCustomer:
		return fact.Type == itemevents.CommentCreated && c.commentKind == "customer" && fact.CommentAuthorKind == "portal_customer" && !fact.CommentIsPrivate
	case conditionCommentByAgent:
		return fact.Type == itemevents.CommentCreated && c.commentKind == "agent" && fact.CommentAuthorKind != "" && fact.CommentAuthorKind != "portal_customer" && !fact.CommentIsPrivate
	default:
		return false
	}
}

func (c *compiledCondition) statusEntered(fact itemevents.RecordedFact, _ *compiledConfig) bool {
	if len(c.statusIDs) == 0 {
		return false
	}
	statusID := fact.NewStatusID
	if statusID == nil && fact.Type == itemevents.Created {
		statusID = fact.Snapshot.StatusID
	}
	return c.statusInSet(statusID, c.statusIDs)
}

func (c *compiledCondition) statusInSet(statusID *int, set map[int]struct{}) bool {
	if statusID == nil {
		return false
	}
	_, ok := set[*statusID]
	return ok
}

func (c *compiledCondition) categoryInSet(categoryID *int, set map[int]struct{}) bool {
	if categoryID == nil {
		return false
	}
	_, ok := set[*categoryID]
	return ok
}

// categoryOfStatus resolves the category for a status ID. The preferNew flag
// is unused today but documents that entered conditions read the new status
// while exited conditions read the old one.
func categoryOfStatus(statusID *int, _ itemevents.RecordedFact, _ bool, config *compiledConfig) *int {
	if statusID == nil {
		return nil
	}
	info, ok := config.statuses[*statusID]
	if !ok {
		return nil
	}
	categoryID := info.categoryID
	return &categoryID
}

// matchPhase returns true when any condition in the phase holds.
func (m *compiledMetric) matchPhase(phase string, fact itemevents.RecordedFact, config *compiledConfig) bool {
	for _, condition := range m.conditions {
		if condition.phase != phase {
			continue
		}
		if condition.matches(fact, config) {
			return true
		}
	}
	return false
}

// matchesCurrent reports whether a condition holds for an item's current
// persisted state. Event-only conditions (comments) do not match; historical
// conditions use the current status as the entered status.
func (c *compiledCondition) matchesCurrent(snapshot itemevents.ItemSnapshot, config *compiledConfig) bool {
	switch c.typ {
	case conditionCreated:
		return true
	case conditionStatusEntered, conditionStatusCurrent:
		return c.statusInSet(snapshot.StatusID, c.statusIDs)
	case conditionStatusCategoryEntered, conditionStatusCategoryCurrent:
		if snapshot.StatusID == nil {
			return false
		}
		info, ok := config.statuses[*snapshot.StatusID]
		if !ok {
			return false
		}
		_, ok = c.categoryIDs[info.categoryID]
		return ok
	case conditionResolutionSet:
		if snapshot.StatusID == nil {
			return false
		}
		return config.statuses[*snapshot.StatusID].completed
	case conditionAssigneeSet:
		return snapshot.AssigneeID != nil
	default:
		return false
	}
}

// pauseRelevant reports whether a fact can change whether a pause condition
// holds. A paused cycle is only resumed by a fact that touches a pause input;
// unrelated edits leave it paused.
func (m *compiledMetric) pauseRelevant(fact itemevents.RecordedFact) bool {
	for _, condition := range m.conditions {
		if condition.phase != models.SLAPhasePause {
			continue
		}
		if condition.relevantToFact(fact) {
			return true
		}
	}
	return false
}

func (c *compiledCondition) relevantToFact(fact itemevents.RecordedFact) bool {
	for _, field := range conditionInputFields(c) {
		switch field {
		case "__created":
			if fact.Type == itemevents.Created {
				return true
			}
		case "__comment":
			if fact.Type == itemevents.CommentCreated {
				return true
			}
		default:
			if field == "status_id" && fact.Type == itemevents.StatusChanged {
				return true
			}
			for _, change := range fact.Changes {
				if change.Field == field {
					return true
				}
			}
		}
	}
	return false
}

// matchPhaseCurrent reports whether any condition in the phase holds for an
// item's current state.
func (m *compiledMetric) matchPhaseCurrent(phase string, snapshot itemevents.ItemSnapshot, config *compiledConfig) bool {
	for _, condition := range m.conditions {
		if condition.phase != phase {
			continue
		}
		if condition.matchesCurrent(snapshot, config) {
			return true
		}
	}
	return false
}
