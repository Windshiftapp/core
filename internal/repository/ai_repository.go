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
