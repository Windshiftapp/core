package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// LoadPersonalLabelsForItems attaches labels visible to viewingUserID.
func LoadPersonalLabelsForItems(ctx context.Context, db database.Database, items []models.Item, viewingUserID int) error {
	if len(items) == 0 {
		return nil
	}

	args := make([]any, 0, len(items)+1)
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		args = append(args, item.ID)
		placeholders = append(placeholders, "?")
	}
	args = append(args, viewingUserID)

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT pil.item_id, pl.id, pl.name, pl.color, pl.user_id, pl.created_at, pl.updated_at
		FROM personal_item_labels pil
		JOIN personal_labels pl ON pil.personal_label_id = pl.id
		WHERE pil.item_id IN (%s)
		  AND (pl.user_id IS NULL OR pl.user_id = ?)
		ORDER BY pl.name
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return fmt.Errorf("load personal labels for items: %w", err)
	}
	defer rows.Close()

	labelsByItem := make(map[int][]models.PersonalLabel)
	for rows.Next() {
		var itemID int
		var label models.PersonalLabel
		var userID sql.NullInt64
		if err := rows.Scan(&itemID, &label.ID, &label.Name, &label.Color, &userID, &label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("scan personal label: %w", err)
		}
		if userID.Valid {
			value := int(userID.Int64)
			label.UserID = &value
		}
		labelsByItem[itemID] = append(labelsByItem[itemID], label)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate personal labels: %w", err)
	}

	for i := range items {
		items[i].PersonalLabels = labelsByItem[items[i].ID]
	}
	return nil
}
