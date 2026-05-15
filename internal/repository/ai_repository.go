package repository

import (
	"database/sql"
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
	ID          int
	Content     string
	Date        string
	UpdatedAt   string
	GeneratedAt string
}

// GetLatestSuccessfulDailyBriefing returns the latest non-error briefing for a user.
func (r *AIRepository) GetLatestSuccessfulDailyBriefing(userID int) (*DailyBriefingSummary, error) {
	var b DailyBriefingSummary
	err := r.db.QueryRow(
		`SELECT id, content, date, updated_at FROM daily_briefings WHERE user_id = ? AND error IS NULL ORDER BY date DESC LIMIT 1`,
		userID,
	).Scan(&b.ID, &b.Content, &b.Date, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.GeneratedAt = b.UpdatedAt
	if t, parseErr := time.Parse("2006-01-02 15:04:05", b.UpdatedAt); parseErr == nil {
		b.GeneratedAt = t.Format(time.RFC3339)
	}
	return &b, nil
}
