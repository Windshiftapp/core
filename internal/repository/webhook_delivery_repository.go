package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WebhookDeliveryRepository persists outbound webhook delivery attempts and
// reads them back for the admin Diagnostics page.
type WebhookDeliveryRepository struct {
	db database.Database
}

// NewWebhookDeliveryRepository constructs a new repository.
func NewWebhookDeliveryRepository(db database.Database) *WebhookDeliveryRepository {
	return &WebhookDeliveryRepository{db: db}
}

// Insert records a single delivery attempt. Always uses the write connection.
// Errors are returned but the caller (the webhook sender) typically just logs
// them — recording failure must never block actual webhook dispatch.
func (r *WebhookDeliveryRepository) Insert(ctx context.Context, d *models.WebhookDelivery) error {
	_, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO webhook_deliveries
			(channel_id, item_id, event_type, attempt_type, transport, request_url,
			 requested_at, response_status_code, response_time_ms, success, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		d.ChannelID, d.ItemID, d.EventType, d.AttemptType, d.Transport, nullStringArg(d.RequestURL),
		d.RequestedAt, nullIntArg(d.ResponseStatusCode), nullIntArg(d.ResponseTimeMs),
		d.Success, nullStringArg(d.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("failed to insert webhook delivery: %w", err)
	}
	return nil
}

// RecentDeliveriesOpts controls cross-channel queries against webhook_deliveries.
type RecentDeliveriesOpts struct {
	Status    string    // "" for any; "failed" or "success"
	ChannelID int       // 0 for any
	Since     time.Time // include rows with requested_at >= Since (zero = no lower bound)
	Limit     int       // capped at 200; 25 default
}

// GetRecent returns delivery rows joined with channel name.
func (r *WebhookDeliveryRepository) GetRecent(opts RecentDeliveriesOpts) ([]*models.WebhookDelivery, error) {
	conds := []string{"1=1"}
	args := []any{}
	switch opts.Status {
	case "failed":
		conds = append(conds, "d.success = ?")
		args = append(args, false)
	case "success":
		conds = append(conds, "d.success = ?")
		args = append(args, true)
	}
	if opts.ChannelID > 0 {
		conds = append(conds, "d.channel_id = ?")
		args = append(args, opts.ChannelID)
	}
	if !opts.Since.IsZero() {
		conds = append(conds, "d.requested_at >= ?")
		args = append(args, opts.Since)
	}

	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}

	query := fmt.Sprintf(`
		SELECT d.id, d.channel_id, d.item_id, d.event_type, d.attempt_type, d.transport,
		       d.request_url, d.requested_at, d.response_status_code, d.response_time_ms,
		       d.success, d.error_message, c.name
		FROM webhook_deliveries d
		LEFT JOIN channels c ON d.channel_id = c.id
		WHERE %s
		ORDER BY d.requested_at DESC
		LIMIT ?
	`, strings.Join(conds, " AND "))
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhook deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*models.WebhookDelivery
	for rows.Next() {
		d := &models.WebhookDelivery{}
		var itemID, statusCode, timeMs sql.NullInt64
		var requestURL, errorMessage, channelName sql.NullString

		if err := rows.Scan(
			&d.ID, &d.ChannelID, &itemID, &d.EventType, &d.AttemptType, &d.Transport,
			&requestURL, &d.RequestedAt, &statusCode, &timeMs,
			&d.Success, &errorMessage, &channelName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan webhook delivery: %w", err)
		}

		if itemID.Valid {
			v := int(itemID.Int64)
			d.ItemID = &v
		}
		if statusCode.Valid {
			v := int(statusCode.Int64)
			d.ResponseStatusCode = &v
		}
		if timeMs.Valid {
			v := int(timeMs.Int64)
			d.ResponseTimeMs = &v
		}
		if requestURL.Valid {
			d.RequestURL = requestURL.String
		}
		if errorMessage.Valid {
			d.ErrorMessage = errorMessage.String
		}
		if channelName.Valid {
			d.ChannelName = channelName.String
		}

		result = append(result, d)
	}

	return result, nil
}

// ChannelDeliveryStats aggregates one channel's delivery activity inside a window.
type ChannelDeliveryStats struct {
	ChannelID    int     `json:"channel_id"`
	ChannelName  string  `json:"channel_name"`
	Total        int     `json:"total"`
	Successes    int     `json:"successes"`
	Failures     int     `json:"failures"`
	AvgLatencyMs *int    `json:"avg_latency_ms,omitempty"`
	LastSuccess  *string `json:"last_success_at,omitempty"`
	LastFailure  *string `json:"last_failure_at,omitempty"`
}

// Stats returns per-channel delivery stats for rows since the given time.
// Channels with zero deliveries in the window are not included.
func (r *WebhookDeliveryRepository) Stats(since time.Time) ([]*ChannelDeliveryStats, error) {
	rows, err := r.db.Query(`
		SELECT d.channel_id, c.name,
		       COUNT(*),
		       SUM(CASE WHEN d.success THEN 1 ELSE 0 END),
		       SUM(CASE WHEN d.success THEN 0 ELSE 1 END),
		       AVG(d.response_time_ms),
		       MAX(CASE WHEN d.success THEN d.requested_at END),
		       MAX(CASE WHEN NOT d.success THEN d.requested_at END)
		FROM webhook_deliveries d
		LEFT JOIN channels c ON d.channel_id = c.id
		WHERE d.requested_at >= ?
		GROUP BY d.channel_id, c.name
		ORDER BY COUNT(*) DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhook delivery stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*ChannelDeliveryStats
	for rows.Next() {
		s := &ChannelDeliveryStats{}
		var channelName sql.NullString
		var avgLatency sql.NullFloat64
		var lastSuccess, lastFailure sql.NullTime

		if err := rows.Scan(
			&s.ChannelID, &channelName, &s.Total, &s.Successes, &s.Failures,
			&avgLatency, &lastSuccess, &lastFailure,
		); err != nil {
			return nil, fmt.Errorf("failed to scan webhook delivery stats: %w", err)
		}

		if channelName.Valid {
			s.ChannelName = channelName.String
		}
		if avgLatency.Valid {
			v := int(avgLatency.Float64)
			s.AvgLatencyMs = &v
		}
		if lastSuccess.Valid {
			v := lastSuccess.Time.Format(time.RFC3339)
			s.LastSuccess = &v
		}
		if lastFailure.Valid {
			v := lastFailure.Time.Format(time.RFC3339)
			s.LastFailure = &v
		}
		result = append(result, s)
	}

	return result, nil
}

// Purge deletes delivery rows older than the given cutoff. Returns the number
// of rows deleted. Intended for the admin manual-purge endpoint.
func (r *WebhookDeliveryRepository) Purge(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx,
		`DELETE FROM webhook_deliveries WHERE requested_at < ?`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to purge webhook deliveries: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read purge row count: %w", err)
	}
	return rows, nil
}

func nullStringArg(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIntArg(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
