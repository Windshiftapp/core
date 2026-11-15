package repository

import (
	"context"
	"fmt"
	"time"

	"windshift/internal/models"
)

// OverviewChartQuery uses half-open UTC day bounds and an already authorized scope.
type OverviewChartQuery struct {
	WorkspaceID int
	FilterSQL   string
	FilterArgs  []any
	From, To    time.Time
	Created     bool
	Completed   bool
}

// OverviewChartBuckets aggregates in SQL, independently of item-list pagination.
func (r *ItemRepository) OverviewChartBuckets(ctx context.Context, q OverviewChartQuery) ([]models.OverviewChartBucket, error) {
	where := " WHERE i.workspace_id = ? AND i.merged_into_item_id IS NULL"
	args := []any{q.WorkspaceID}
	if q.FilterSQL != "" {
		where += " AND (" + q.FilterSQL + ")"
		args = append(args, q.FilterArgs...)
	}
	postgres := r.db.GetDriverName() == "postgres"
	instant := func(column string) string {
		if postgres {
			return column
		}
		return "julianday(" + column + ")"
	}
	day := func(column string) string {
		if postgres {
			return "TO_CHAR(" + column + " AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
		}
		return "strftime('%Y-%m-%d', " + column + ")"
	}
	buckets := make([]models.OverviewChartBucket, 0)
	indices := make(map[string]int)
	for date := q.From; date.Before(q.To); date = date.AddDate(0, 0, 1) {
		bucket := models.OverviewChartBucket{Date: date.Format(time.DateOnly)}
		if q.Created {
			bucket.Created = new(int)
		}
		if q.Completed {
			bucket.Completed = new(int)
		}
		indices[bucket.Date] = len(buckets)
		buckets = append(buckets, bucket)
	}
	read := func(query string, queryArgs []any, completed bool) error {
		rows, err := r.db.QueryContext(ctx, query, queryArgs...)
		if err != nil {
			return fmt.Errorf("aggregate overview charts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var date string
			var count int
			if err := rows.Scan(&date, &count); err != nil {
				return fmt.Errorf("scan overview chart bucket: %w", err)
			}
			if index, ok := indices[date]; ok {
				if completed {
					*buckets[index].Completed = count
				} else {
					*buckets[index].Created = count
				}
			}
		}
		return rows.Err()
	}
	if q.Created {
		query := "SELECT " + day("i.created_at") + ", COUNT(*) " + ItemListFilterFromClause() + where +
			" AND " + instant("i.created_at") + " >= " + instant("?") +
			" AND " + instant("i.created_at") + " < " + instant("?") + " GROUP BY 1"
		if err := read(query, append(append([]any{}, args...), q.From, q.To), false); err != nil {
			return nil, err
		}
	}
	if q.Completed {
		// Find the lifetime first completion before applying the lower bound.
		// Otherwise a recompletion inside the window would become a new completion.
		query := `WITH first_completions AS (
			SELECT i.id, MIN(` + instant("ih.changed_at") + `) AS completed_at ` + ItemListFilterFromClause() + `
			JOIN item_history ih ON ih.item_id = i.id AND ih.field_name = 'status_id'
			JOIN statuses completed_status ON ih.new_value = CAST(completed_status.id AS TEXT)
			JOIN status_categories completed_category ON completed_category.id = completed_status.category_id
			JOIN statuses old_status ON ih.old_value = CAST(old_status.id AS TEXT)
			JOIN status_categories old_category ON old_category.id = old_status.category_id ` + where + `
			AND completed_category.is_completed = true AND old_category.is_completed = false
			AND ` + instant("ih.changed_at") + " < " + instant("?") + ` GROUP BY i.id)
			SELECT ` + day("completed_at") + `, COUNT(*) FROM first_completions
			WHERE completed_at >= ` + instant("?") + " GROUP BY 1"
		if err := read(query, append(append([]any{}, args...), q.To, q.From), true); err != nil {
			return nil, err
		}
	}
	return buckets, nil
}
