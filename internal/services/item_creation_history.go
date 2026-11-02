package services

import (
	"fmt"
	"sort"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// RecordItemCreationHistory writes the initial item_history rows for a newly
// created item. Every creation path calls it so any valid item has a history
// feed, whether a person, an integration, or a scheduler created it. The actor
// is the internal user when one acted, otherwise the portal customer, otherwise
// the system. metadata carries the creation timestamp and any agent provenance.
func RecordItemCreationHistory(
	db database.Database,
	tx database.Tx,
	item *models.Item,
	actorUserID int,
	portalCustomerID *int,
	metadata itemevents.Metadata,
) error {
	history := stampHistorySource(creationHistoryEntries(*item, actorUserID, portalCustomerID, metadata.OccurredAt), metadata)
	return repository.NewItemRepository(db).RecordHistoryBatch(tx, history)
}

// creationHistoryEntries builds the initial history rows for a new item. The
// acting internal user wins when present; the portal customer is used only for
// submissions with no internal creator, and a missing actor falls back to the
// system so creation is never silently absent from the feed.
func creationHistoryEntries(item models.Item, userID int, portalCustomerID *int, changedAt time.Time) []HistoryEntry {
	var history []HistoryEntry
	if changedAt.IsZero() {
		changedAt = time.Now()
	}

	// Helper to add history entry (old_value is always empty for creation)
	addHistory := func(fieldName, newValue string) {
		if newValue == "" {
			return
		}
		entry := HistoryEntry{
			ItemID:    item.ID,
			FieldName: fieldName,
			OldValue:  "",
			NewValue:  newValue,
			ChangedAt: changedAt,
		}
		setCreationHistoryActor(&entry, userID, portalCustomerID)
		history = append(history, entry)
	}

	// Add entries for all fields
	addHistory("title", item.Title)
	addHistory("description", item.Description)
	addHistory("item_type_id", intPtrToString(item.ItemTypeID))
	addHistory("status_id", intPtrToString(item.StatusID))
	addHistory("priority_id", intPtrToString(item.PriorityID))
	addHistory("iteration_id", intPtrToString(item.IterationID))
	addHistory("project_id", intPtrToString(item.ProjectID))
	addHistory("time_project_id", intPtrToString(item.TimeProjectID))
	addHistory("assignee_id", intPtrToString(item.AssigneeID))
	addHistory("team_id", intPtrToString(item.TeamID))
	addHistory("creator_id", intPtrToString(item.CreatorID))
	addHistory("parent_id", intPtrToString(item.ParentID))
	addHistory("due_date", timePtrToString(item.DueDate))
	addHistory("start_date", timePtrToString(item.StartDate))
	addHistory("end_date", timePtrToString(item.EndDate))
	addHistory("workspace_id", fmt.Sprintf("%d", item.WorkspaceID))
	addHistory("story_points", float64PtrToString(item.StoryPoints))
	addHistory("estimate_minutes", intPtrToString(item.EstimateMinutes))
	// Only record inherit_project when set; the default (false) on root items
	// would otherwise add a no-op entry to every creation event.
	if item.InheritProject {
		addHistory("inherit_project", "true")
	}

	return history
}

// setCreationHistoryActor sets a creation row's actor: the internal user
// when one acted, otherwise the portal customer, otherwise the system.
func setCreationHistoryActor(entry *HistoryEntry, userID int, portalCustomerID *int) {
	switch {
	case userID > 0:
		entry.ActorKind = repository.HistoryActorUser
		entry.UserID = userID
	case portalCustomerID != nil:
		entry.ActorKind = repository.HistoryActorPortalCustomer
		entry.ActorPortalCustomerID = portalCustomerID
	default:
		entry.ActorKind = repository.HistoryActorSystem
	}
}

// RecordItemCreationMilestonesHistory writes the milestones row for an item
// created directly in milestones, in the old/new CSV form the update path
// writes, so the milestone Activity feed shows "item added" from it. Items
// created without milestones get no row. The actor follows
// RecordItemCreationHistory.
func RecordItemCreationMilestonesHistory(
	db database.Database,
	tx database.Tx,
	itemID int,
	milestoneIDs []int,
	actorUserID int,
	portalCustomerID *int,
	metadata itemevents.Metadata,
) error {
	if len(milestoneIDs) == 0 {
		return nil
	}
	changedAt := metadata.OccurredAt
	if changedAt.IsZero() {
		changedAt = time.Now()
	}
	ids := append([]int(nil), milestoneIDs...)
	sort.Ints(ids)
	entry := HistoryEntry{ItemID: itemID, FieldName: "milestones", OldValue: "", NewValue: joinIntsCSV(ids), ChangedAt: changedAt}
	setCreationHistoryActor(&entry, actorUserID, portalCustomerID)
	return repository.NewItemRepository(db).RecordHistoryBatch(tx, stampHistorySource([]HistoryEntry{entry}, metadata))
}
