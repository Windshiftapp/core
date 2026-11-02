package repository

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Milestone history fields. Only the fields the milestone Activity
// tab shows are recorded. A page_link row records a page linked
// (new_value is the page ID) or unlinked (old_value is the page ID).
const (
	MilestoneHistoryDescription = "description"
	MilestoneHistoryStatus      = "status"
	MilestoneHistoryTargetDate  = "target_date"
	MilestoneHistoryPageLink    = "page_link"
)

// MilestoneHistoryEntry is one recorded change to a milestone field.
// UserID nil means a system actor (automation, imports).
type MilestoneHistoryEntry struct {
	MilestoneID int
	UserID      *int
	FieldName   string
	OldValue    string
	NewValue    string
	ChangedAt   time.Time
}

// RecordMilestoneHistory writes milestone history rows through w, which may be
// a transaction so the rows commit with the change they describe. Empty
// values are stored as NULL.
func RecordMilestoneHistory(w HistoryWriter, entries ...MilestoneHistoryEntry) error {
	for _, entry := range entries {
		changedAt := entry.ChangedAt
		if changedAt.IsZero() {
			changedAt = time.Now().UTC()
		}
		var userID any
		if entry.UserID != nil && *entry.UserID > 0 {
			userID = *entry.UserID
		}
		if _, err := w.ExecWrite(`
			INSERT INTO milestone_history (milestone_id, user_id, field_name, old_value, new_value, changed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, entry.MilestoneID, userID, entry.FieldName, nullableHistoryValue(entry.OldValue), nullableHistoryValue(entry.NewValue), changedAt); err != nil {
			return fmt.Errorf("record milestone %d %s history: %w", entry.MilestoneID, entry.FieldName, err)
		}
		if err := TouchMilestoneActivity(w, changedAt, entry.MilestoneID); err != nil {
			return err
		}
	}
	return nil
}

// TouchMilestoneActivity sets the milestones' last_activity_at to at, the
// time of their newest Activity entry. Every writer of an Activity source
// calls it through the same writer as the row it adds.
func TouchMilestoneActivity(w HistoryWriter, at time.Time, milestoneIDs ...int) error {
	clause, args := inPlaceholders(milestoneIDs)
	if clause == "" {
		return nil
	}
	if _, err := w.ExecWrite(
		"UPDATE milestones SET last_activity_at = ? WHERE id IN ("+clause+")",
		append([]any{at.UTC()}, args...)...,
	); err != nil {
		return fmt.Errorf("touch milestone activity: %w", err)
	}
	return nil
}

// TouchItemMilestoneActivity sets last_activity_at on every milestone the item
// belongs to, for item status changes and item comments.
func TouchItemMilestoneActivity(w HistoryWriter, itemID int, at time.Time) error {
	if _, err := w.ExecWrite(
		"UPDATE milestones SET last_activity_at = ? WHERE id IN (SELECT milestone_id FROM item_milestones WHERE item_id = ?)",
		at.UTC(), itemID,
	); err != nil {
		return fmt.Errorf("touch milestone activity for item %d: %w", itemID, err)
	}
	return nil
}

// touchItemHistoryMilestoneActivity bumps the milestones an item history row
// shows up under in the Activity feed: the milestones an item joined or left
// for a milestones row, and the item's milestones for a status change.
// Creation status rows (no old status) are not Activity entries.
func touchItemHistoryMilestoneActivity(w HistoryWriter, entry HistoryEntry) error {
	oldValue := entry.OldValue
	if entry.OldValueNull {
		oldValue = ""
	}
	switch entry.FieldName {
	case "milestones":
		return TouchMilestoneActivity(w, entry.ChangedAt, changedMilestoneIDs(oldValue, entry.NewValue)...)
	case "status_id":
		if oldValue == "" {
			return nil
		}
		return TouchItemMilestoneActivity(w, entry.ItemID, entry.ChangedAt)
	}
	return nil
}

// changedMilestoneIDs returns the milestone IDs in exactly one of two
// milestones history values ("3,50").
func changedMilestoneIDs(oldList, newList string) []int {
	counts := map[int]int{}
	for _, list := range []string{oldList, newList} {
		seen := map[int]bool{}
		for _, part := range strings.Split(list, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || seen[id] {
				continue
			}
			seen[id] = true
			counts[id]++
		}
	}
	changed := []int{}
	for id, count := range counts {
		if count == 1 {
			changed = append(changed, id)
		}
	}
	return changed
}

func nullableHistoryValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}
