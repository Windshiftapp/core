package repository

import (
	"context"

	"windshift/internal/database"
)

type ReviewPreview struct {
	ID        int    `json:"id"`
	Date      string `json:"review_date"`
	Type      string `json:"review_type"`
	Content   string `json:"review_data"`
	Truncated bool   `json:"truncated"`
}

type ReviewRepository struct{ db database.Database }

func NewReviewRepository(db database.Database) *ReviewRepository { return &ReviewRepository{db: db} }

func (r *ReviewRepository) ListPreviews(ctx context.Context, userID int, from, to string, limit, offset int) ([]ReviewPreview, error) {
	const excerptLimit = 2000
	rows, err := r.db.QueryContext(ctx, `SELECT id, CAST(review_date AS TEXT), review_type, SUBSTR(CAST(review_data AS TEXT), 1, ?), LENGTH(CAST(review_data AS TEXT)) > ?
		FROM reviews WHERE user_id = ? AND review_date >= ? AND review_date <= ?
		ORDER BY review_date DESC, id DESC LIMIT ? OFFSET ?`, excerptLimit, excerptLimit, userID, from, to, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ReviewPreview{}
	for rows.Next() {
		var review ReviewPreview
		if err := rows.Scan(&review.ID, &review.Date, &review.Type, &review.Content, &review.Truncated); err != nil {
			return nil, err
		}
		result = append(result, review)
	}
	return result, rows.Err()
}
