package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
)

type BoardPlacementRepository struct{ db database.Database }

func NewBoardPlacementRepository(db database.Database) *BoardPlacementRepository {
	return &BoardPlacementRepository{db: db}
}

const boardPlacementSelect = `SELECT id, collection_id, channel_id, label, display_order, enabled, access_mode, organisation_ids, revision FROM board_placements`

func scanBoardPlacement(row interface{ Scan(...any) error }) (*models.BoardPlacement, error) {
	var placement models.BoardPlacement
	var ids string
	err := row.Scan(&placement.ID, &placement.CollectionID, &placement.ChannelID, &placement.Label, &placement.DisplayOrder, &placement.Enabled, &placement.Policy.Mode, &ids, &placement.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(ids), &placement.Policy.OrganisationIDs); err != nil {
		return nil, err
	}
	return &placement, nil
}

func (r *BoardPlacementRepository) Get(ctx context.Context, channelID int, id string) (*models.BoardPlacement, error) {
	return scanBoardPlacement(r.db.QueryRowContext(ctx, boardPlacementSelect+` WHERE channel_id = ? AND id = ?`, channelID, id))
}

func (r *BoardPlacementRepository) List(ctx context.Context, channelID int) ([]models.BoardPlacement, error) {
	rows, err := r.db.QueryContext(ctx, boardPlacementSelect+` WHERE channel_id = ? ORDER BY display_order, id`, channelID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []models.BoardPlacement{}
	for rows.Next() {
		item, err := scanBoardPlacement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
