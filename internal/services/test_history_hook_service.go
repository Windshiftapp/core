package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
)

// HistoryBackdateEntry moves the changed_at timestamp of an item's
// existing history rows. FieldName "*" matches every history row of the
// item; otherwise FieldName plus OldValue and NewValue must match the
// row exactly. Entries apply in order, so a later entry can override an
// earlier wildcard.
type HistoryBackdateEntry struct {
	FieldName string    `json:"field_name"`
	OldValue  *string   `json:"old_value,omitempty"`
	NewValue  *string   `json:"new_value,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
}

// HistoryBackdateItem optionally shifts an item's created_at alongside
// its history rows.
type HistoryBackdateItem struct {
	ItemID    int                    `json:"item_id"`
	CreatedAt *time.Time             `json:"created_at,omitempty"`
	Set       []HistoryBackdateEntry `json:"set"`
}

type HistoryBackdateRequest struct {
	Items []HistoryBackdateItem `json:"items"`
}

// TestHistoryHookService backs the WINDSHIFT_E2E_TEST_HOOKS-only
// history backdate route. It can only move timestamps of rows the
// production API already wrote — it never inserts, deletes, or rewrites
// field values — so browser tests can exercise time-based features
// (burndown history, status durations) that would otherwise require the
// API to grow time-travel parameters.
type TestHistoryHookService struct {
	db database.Database
}

func NewTestHistoryHookService(db database.Database) *TestHistoryHookService {
	return &TestHistoryHookService{db: db}
}

// Backdate applies the requested timestamp shifts and returns the number
// of history rows whose changed_at changed. Items whose IDs do not exist
// update zero rows; callers assert on the count they expect.
func (s *TestHistoryHookService) Backdate(ctx context.Context, req HistoryBackdateRequest) (int, error) {
	updated := 0
	for _, item := range req.Items {
		if item.CreatedAt != nil {
			res, err := s.db.ExecContext(ctx,
				`UPDATE items SET created_at = ? WHERE id = ?`,
				item.CreatedAt.UTC(), item.ItemID)
			if err != nil {
				return updated, fmt.Errorf("backdate created_at for item %d: %w", item.ItemID, err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return updated, fmt.Errorf("backdate created_at: item %d not found", item.ItemID)
			}
		}
		for _, entry := range item.Set {
			var (
				res sql.Result
				err error
			)
			if entry.FieldName == "*" {
				res, err = s.db.ExecContext(ctx,
					`UPDATE item_history SET changed_at = ? WHERE item_id = ?`,
					entry.ChangedAt.UTC(), item.ItemID)
			} else {
				oldValue, newValue := "", ""
				if entry.OldValue != nil {
					oldValue = *entry.OldValue
				}
				if entry.NewValue != nil {
					newValue = *entry.NewValue
				}
				res, err = s.db.ExecContext(ctx,
					`UPDATE item_history SET changed_at = ? WHERE item_id = ? AND field_name = ? AND old_value = ? AND new_value = ?`,
					entry.ChangedAt.UTC(), item.ItemID, entry.FieldName, oldValue, newValue)
			}
			if err != nil {
				return updated, fmt.Errorf("backdate history for item %d (%s): %w", item.ItemID, entry.FieldName, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				updated += int(n)
			}
		}
	}
	return updated, nil
}
