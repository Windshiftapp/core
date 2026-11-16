package services

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

func (s *CollectionApplicationService) requirePortalManager(ctx context.Context, userID, channelID int) error {
	repo := repository.NewChannelRepository(s.db)
	channel, err := repo.FindByID(ctx, channelID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCollectionNotFound
	}
	if err != nil {
		return err
	}
	if channel.Type != "portal" {
		return collectionValidation("board placements require a portal channel")
	}
	admin, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return err
	}
	if admin {
		return nil
	}
	allowed, err := repo.UserCanManage(ctx, userID, channelID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCollectionForbidden
	}
	return nil
}

func (s *CollectionApplicationService) ListBoardPlacements(ctx context.Context, userID, channelID int) ([]models.BoardPlacement, error) {
	if err := s.requirePortalManager(ctx, userID, channelID); err != nil {
		return nil, err
	}
	items, err := repository.NewBoardPlacementRepository(s.db).List(ctx, channelID)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.CollectionID)
	}
	enabled, err := s.boardsEnabled()
	if err != nil {
		return nil, err
	}
	boards, err := s.boardSharingRows(ids, false, enabled)
	if err != nil {
		return nil, err
	}
	policy, err := repository.NewCustomerAccessRepository(s.db).Policy(ctx, "portal", channelID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		for _, board := range boards {
			if board.CollectionID != items[i].CollectionID {
				continue
			}
			baseline := board.Policy
			effective := intersectBoardPolicies(baseline, policy, items[i].Policy)
			items[i].SourceName, items[i].BaselinePolicy, items[i].EffectivePolicy, items[i].StandaloneEnabled = board.Name, &baseline, &effective, board.StandaloneEnabled
			break
		}
	}
	return items, nil
}

func (s *CollectionApplicationService) SaveBoardPlacement(ctx context.Context, actor AuditActor, channelID int, id string, input models.BoardPlacement) (*models.BoardPlacement, error) {
	// Management summaries are read-only projections, not caller-controlled publication fields.
	input.SourceName, input.BaselinePolicy, input.EffectivePolicy, input.StandaloneEnabled = "", nil, nil, false
	if err := s.requirePortalManager(ctx, actor.UserID, channelID); err != nil {
		return nil, err
	}
	input.Label = sanitize.PlainTextField.Sanitize(strings.TrimSpace(input.Label))
	if input.Label == "" || len(input.Label) > 200 {
		return nil, collectionValidation("placement label must be 1-200 characters")
	}
	policy, err := repository.NewCustomerAccessRepository(s.db).ValidatePolicy(ctx, input.Policy)
	if err != nil {
		return nil, collectionValidation(err.Error())
	}
	input.Policy = policy
	repo := repository.NewBoardPlacementRepository(s.db)
	var before *models.BoardPlacement
	if id == "" {
		if input.Revision != 0 {
			return nil, collectionValidation("new placements must omit revision")
		}
		if _, _, err := s.sharingCollection(actor.UserID, input.CollectionID); err != nil {
			return nil, err
		}
		if err := s.validatePublicPermission(actor.UserID, true); err != nil {
			return nil, err
		}
		id = uuid.NewString()
	} else {
		before, err = repo.Get(ctx, channelID, id)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrCollectionNotFound
		}
		if err != nil {
			return nil, err
		}
		if before.CollectionID != input.CollectionID {
			return nil, collectionValidation("a placement cannot change its publication")
		}
		if before.Revision != input.Revision {
			return nil, ErrCollectionConflict
		}
		_, _, boardErr := s.sharingCollection(actor.UserID, input.CollectionID)
		if boardErr != nil && !errors.Is(boardErr, ErrCollectionForbidden) {
			return nil, boardErr
		}
		if boardErr != nil && !policyNarrows(before.Policy, policy) {
			return nil, ErrCollectionForbidden
		}
	}
	input.ID, input.ChannelID, input.Revision = id, channelID, input.Revision+1
	raw, err := json.Marshal(policy.OrganisationIDs)
	if err != nil {
		return nil, err
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		if before == nil {
			_, err = tx.Exec(`INSERT INTO board_placements (id, collection_id, channel_id, label, display_order, enabled, access_mode, organisation_ids, revision) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, input.CollectionID, channelID, input.Label, input.DisplayOrder, input.Enabled, policy.Mode, string(raw), input.Revision)
		} else {
			result, writeErr := tx.Exec(`UPDATE board_placements SET label = ?, display_order = ?, enabled = ?, access_mode = ?, organisation_ids = ?, revision = revision + 1 WHERE id = ? AND channel_id = ? AND revision = ?`,
				input.Label, input.DisplayOrder, input.Enabled, policy.Mode, string(raw), id, channelID, before.Revision)
			if writeErr != nil {
				return writeErr
			}
			count, writeErr := result.RowsAffected()
			if writeErr != nil {
				return writeErr
			}
			if count != 1 {
				return ErrCollectionConflict
			}
		}
		if err != nil {
			return err
		}
		return logger.LogAuditTx(ctx, tx, logger.AuditEvent{UserID: actor.UserID, Username: actor.Username, IPAddress: actor.IPAddress, UserAgent: actor.UserAgent,
			Source: actor.Source, ActionType: logger.ActionCollectionPlacementUpdate, ResourceType: logger.ResourceCollection, ResourceID: &input.CollectionID, Success: true,
			Details: map[string]any{"before": before, "after": input, "portal_id": channelID, "placement_id": id}})
	})
	if database.IsUniqueConstraintError(err) {
		return nil, ErrCollectionConflict
	}
	if err != nil {
		return nil, err
	}
	return &input, nil
}

func policyNarrows(before, after models.CustomerAccessPolicy) bool {
	if before.Mode == "public" {
		return true
	}
	if after.Mode != "organisations" {
		return false
	}
	return !slices.ContainsFunc(after.OrganisationIDs, func(id int) bool { return !slices.Contains(before.OrganisationIDs, id) })
}

func (s *CollectionApplicationService) DeleteBoardPlacement(ctx context.Context, actor AuditActor, channelID int, id string, revision int) error {
	if err := s.requirePortalManager(ctx, actor.UserID, channelID); err != nil {
		return err
	}
	before, err := repository.NewBoardPlacementRepository(s.db).Get(ctx, channelID, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCollectionNotFound
	}
	if err != nil {
		return err
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		result, err := tx.Exec(`DELETE FROM board_placements WHERE id = ? AND channel_id = ? AND revision = ?`, id, channelID, revision)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrCollectionConflict
		}
		return logger.LogAuditTx(ctx, tx, logger.AuditEvent{UserID: actor.UserID, Username: actor.Username, IPAddress: actor.IPAddress, UserAgent: actor.UserAgent,
			Source: actor.Source, ActionType: logger.ActionCollectionPlacementRemove, ResourceType: logger.ResourceCollection, ResourceID: &before.CollectionID, Success: true,
			Details: map[string]any{"before": before, "portal_id": channelID, "placement_id": id}})
	})
}
