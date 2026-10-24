package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"windshift/internal/database"
)

// AIRepository contains small data lookups used by AI handlers.
type AIRepository struct {
	db database.Database
}

func NewAIRepository(db database.Database) *AIRepository {
	return &AIRepository{db: db}
}

// DailyBriefingSummary is the latest successful daily briefing for a user.
type DailyBriefingSummary struct {
	ID                 int
	Content            string
	Date               string
	UpdatedAt          string
	GeneratedAt        string
	SourceWorkspaceIDs []int
	ScopeRecorded      bool
}

// FailedBriefing is one row from daily_briefings where the LLM call failed.
// The classifier in package llm buckets the Error string for diagnostics.
type FailedBriefing struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Date      string `json:"date"`
	Error     string `json:"error"`
	CreatedAt string `json:"created_at"`
}

// ListFailedBriefings returns recent failed daily_briefings rows ordered by
// created_at desc. Both bucket counts and the sample-message display in the
// diagnostics widget pull from this same query.
func (r *AIRepository) ListFailedBriefings(since time.Time, limit int) ([]FailedBriefing, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT id, user_id, date, COALESCE(error, ''), created_at
		 FROM daily_briefings
		 WHERE error IS NOT NULL AND error <> '' AND created_at >= ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FailedBriefing
	for rows.Next() {
		var b FailedBriefing
		if err := rows.Scan(&b.ID, &b.UserID, &b.Date, &b.Error, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BriefingHealthRow is the newest daily_briefings row for one user inside the
// diagnostic window. The scheduler can write several rows per user and day
// (a failure followed by a retry), so only the newest row describes the user's
// current state.
type BriefingHealthRow struct {
	ID            int
	UserID        int
	Date          string
	ContentLength int
	Error         string
	// HasProvenance mirrors the read-path ScopeRecorded flag: it is true when
	// source_workspace_ids is non-NULL. Legacy rows are hidden regardless of
	// their content, so this distinguishes "hidden" from "empty".
	HasProvenance bool
	// LockHeld is true while an unexpired generation lease is held. A held lock
	// with no content means a tick is running or a holder crashed recently.
	LockHeld  bool
	UpdatedAt string
}

// ListLatestBriefingHealthRows returns the newest daily_briefings row per user
// updated at or after since, newest first, capped at limit. Users with no row
// in the window are absent, which is how the caller detects "never generated".
// now classifies a held generation lease; it is a bound value rather than
// CURRENT_TIMESTAMP because SQLite stores a Go time.Time lease in a different
// textual shape than CURRENT_TIMESTAMP.
func (r *AIRepository) ListLatestBriefingHealthRows(since, now time.Time, limit int) ([]BriefingHealthRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	rows, err := r.db.Query(`
		SELECT id, user_id, date, LENGTH(content), COALESCE(error, ''),
		       source_workspace_ids,
		       CASE WHEN lock_until IS NOT NULL AND lock_until > ? THEN 1 ELSE 0 END,
		       updated_at
		FROM (
			SELECT id, user_id, date, content, error, source_workspace_ids, lock_until, updated_at,
			       ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY updated_at DESC, id DESC) AS rn
			FROM daily_briefings
			WHERE updated_at >= ?
		) latest
		WHERE rn = 1
		ORDER BY updated_at DESC
		LIMIT ?
	`, now, since, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []BriefingHealthRow
	for rows.Next() {
		var row BriefingHealthRow
		var source sql.NullString
		var lockHeld int
		if err := rows.Scan(&row.ID, &row.UserID, &row.Date, &row.ContentLength, &row.Error, &source, &lockHeld, &row.UpdatedAt); err != nil {
			return nil, err
		}
		row.HasProvenance = source.Valid
		row.LockHeld = lockHeld == 1
		out = append(out, row)
	}
	return out, rows.Err()
}

// CountActiveUsers returns the number of active users, matching the set the
// briefing scheduler iterates when it fans out generation.
func (r *AIRepository) CountActiveUsers() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_active = true`).Scan(&count)
	return count, err
}

// GetLatestSuccessfulDailyBriefing returns the latest non-error briefing for a user.
func (r *AIRepository) GetLatestSuccessfulDailyBriefing(userID int) (*DailyBriefingSummary, error) {
	var b DailyBriefingSummary
	var sourceWorkspaceIDs sql.NullString
	err := r.db.QueryRow(
		`SELECT id, content, date, updated_at, source_workspace_ids FROM daily_briefings WHERE user_id = ? AND error IS NULL ORDER BY date DESC LIMIT 1`,
		userID,
	).Scan(&b.ID, &b.Content, &b.Date, &b.UpdatedAt, &sourceWorkspaceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sourceWorkspaceIDs.Valid {
		if err := json.Unmarshal([]byte(sourceWorkspaceIDs.String), &b.SourceWorkspaceIDs); err != nil {
			return nil, err
		}
		b.ScopeRecorded = true
	}
	b.GeneratedAt = b.UpdatedAt
	if t, parseErr := time.Parse("2006-01-02 15:04:05", b.UpdatedAt); parseErr == nil {
		b.GeneratedAt = t.Format(time.RFC3339)
	}
	return &b, nil
}
