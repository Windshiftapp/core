package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

var ErrOverviewChartRange = errors.New("from and to must be UTC dates spanning 1 to 366 days")

// OverviewChartsQuery selects only the series needed by the active layout.
type OverviewChartsQuery struct {
	UserID, WorkspaceID int
	CollectionID        *int
	From, To            time.Time
	Created, Completed  bool
}

func (s *ItemApplicationService) OverviewCharts(ctx context.Context, q OverviewChartsQuery) (*models.OverviewCharts, error) {
	q.From, q.To = q.From.UTC(), q.To.UTC()
	if q.From.IsZero() || q.To.IsZero() || !q.From.Before(q.To) || q.From.AddDate(0, 0, 366).Before(q.To) ||
		!q.From.Equal(q.From.UTC().Truncate(24*time.Hour)) || !q.To.Equal(q.To.UTC().Truncate(24*time.Hour)) {
		return nil, ErrOverviewChartRange
	}
	if err := s.authorizeQueueRead(q.UserID, QueueScope{WorkspaceID: q.WorkspaceID}); err != nil {
		return nil, err
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM workspaces WHERE id = ? AND active = true", q.WorkspaceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	var resolved resolvedItemListQL
	if q.CollectionID != nil {
		collection, err := repository.NewCollectionRepository(s.db).GetByID(*q.CollectionID)
		if err != nil {
			return nil, err
		}
		if collection.WorkspaceID == nil || *collection.WorkspaceID != q.WorkspaceID {
			return nil, repository.ErrNotFound
		}
		if err := s.authorizeQueueRead(q.UserID, QueueScope{WorkspaceID: q.WorkspaceID, CollectionID: q.CollectionID}); err != nil {
			return nil, err
		}
		resolved, err = s.crud.resolveItemListQLContext(ctx, collection.QLQuery, 0, "", q.UserID)
		if err != nil {
			return nil, err
		}
	}
	buckets, err := s.items.OverviewChartBuckets(ctx, repository.OverviewChartQuery{
		WorkspaceID: q.WorkspaceID, FilterSQL: resolved.sql, FilterArgs: resolved.args,
		From: q.From, To: q.To, Created: q.Created, Completed: q.Completed,
	})
	if err != nil {
		return nil, err
	}
	return &models.OverviewCharts{
		From: q.From.Format(time.DateOnly), To: q.To.AddDate(0, 0, -1).Format(time.DateOnly),
		Timezone: "UTC", CompletionDefinition: "first_recorded_completion", Buckets: buckets,
	}, nil
}
