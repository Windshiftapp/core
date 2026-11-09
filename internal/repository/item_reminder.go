package repository

import (
	"fmt"
	"time"
)

// DueReminderRetryInterval is how long a skipped reminder candidate is held
// back before it is re-evaluated. A candidate whose recipient was ineligible
// (inactive, agent, or without workspace access) is recorded as not delivered
// and retried after this interval, so a temporary ineligibility does not lose
// the reminder while still letting the sweep advance past it instead of
// re-selecting the same row every tick.
const DueReminderRetryInterval = 24 * time.Hour

// DueSoonItem is the projection the due-date reminder sweep needs: the
// opted-in recipient to notify and enough item identity to build a deep link
// and delivery key.
type DueSoonItem struct {
	ItemID       int
	WorkspaceID  int
	WorkspaceKey string
	ItemNumber   int
	Title        string
	DueDate      time.Time
	RecipientID  int
}

// RecordDueDateReminder records the outcome of one due-date reminder attempt
// for the (item, user, due date) triple. It is idempotent, and a delivered row
// is never downgraded by a later skipped attempt. attemptedAt is the sweep
// clock, so the retry backoff and the query use the same clock. The ledger
// outlives the notification row so dismissing the notification cannot
// resurrect it.
func (r *ItemRepository) RecordDueDateReminder(itemID, userID int, dueDate string, delivered bool, attemptedAt time.Time) error {
	if _, err := r.db.ExecWrite(`
		INSERT INTO item_due_reminders (item_id, user_id, due_date, delivered, sent_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_id, user_id, due_date) DO UPDATE SET
		    delivered = item_due_reminders.delivered OR excluded.delivered,
		    sent_at = excluded.sent_at
	`, itemID, userID, dueDate, delivered, attemptedAt); err != nil {
		return fmt.Errorf("record due-date reminder: %w", err)
	}
	return nil
}

// ListDueSoonItemsForReminderUsers returns up to limit non-completed items that
// are due within their recipient's configured lead time and have not already
// been reminded. The recipient is the assignee for regular items and the
// workspace owner for personal-workspace items without an assignee. The
// lead-time window, merged/deactivated exclusions, and the sent-reminder
// exclusion all run in SQL, so every returned row is a candidate and a full
// batch always makes progress instead of re-selecting rows that can never be
// claimed. A skipped candidate is held back until retryBefore so a temporary
// ineligibility does not consume a batch slot on every sweep.
func (r *ItemRepository) ListDueSoonItemsForReminderUsers(from, to, retryBefore time.Time, limit int) ([]DueSoonItem, error) {
	if limit <= 0 {
		limit = 500
	}
	// Lead-time comparison and ledger date formatting are the only
	// engine-specific pieces: SQLite uses date() modifiers, PostgreSQL adds an
	// integer to a date. Both normalize to YYYY-MM-DD so the ledger text
	// matches the value the sweep writes.
	leadPredicate := `date(i.due_date) <= date(?, '+' || up.due_date_reminder_days || ' days')`
	ledgerDate := `date(i.due_date)`
	if r.db.GetDriverName() == "postgres" {
		leadPredicate = `i.due_date <= ?::date + up.due_date_reminder_days`
		ledgerDate = `i.due_date::text`
	}
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_id, COALESCE(w.key, ''), i.workspace_item_number,
		       i.title, i.due_date, COALESCE(i.assignee_id, w.owner_id)
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		JOIN user_preferences up ON up.user_id = COALESCE(i.assignee_id, w.owner_id)
		LEFT JOIN statuses st ON st.id = i.status_id
		LEFT JOIN status_categories sc ON sc.id = st.category_id
		WHERE up.due_date_reminders_enabled = TRUE
		  AND i.due_date IS NOT NULL
		  AND i.due_date >= ?
		  AND i.due_date <= ?
		  AND `+leadPredicate+`
		  AND COALESCE(sc.is_completed, FALSE) = FALSE
		  AND i.merged_into_item_id IS NULL
		  AND w.active = TRUE
		  AND (i.assignee_id IS NOT NULL OR w.is_personal = TRUE)
		  AND NOT EXISTS (
		      SELECT 1 FROM item_due_reminders r
		      WHERE r.item_id = i.id
		        AND r.user_id = COALESCE(i.assignee_id, w.owner_id)
		        AND r.due_date = `+ledgerDate+`
		        AND (r.delivered = TRUE OR r.sent_at > ?)
		  )
		ORDER BY i.due_date ASC, i.id ASC
		LIMIT ?
	`, from, to, from, retryBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list due-soon items for reminder users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]DueSoonItem, 0, limit)
	for rows.Next() {
		var item DueSoonItem
		if err := rows.Scan(&item.ItemID, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&item.Title, &item.DueDate, &item.RecipientID); err != nil {
			return nil, fmt.Errorf("scan due-soon reminder item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due-soon reminder items: %w", err)
	}
	return items, nil
}
