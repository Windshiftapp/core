package services

import (
	"errors"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// OptionalPatch distinguishes an absent merge-patch field from one explicitly
// set, including set to null. It mirrors the v2 transport's Optional type so
// the service layer stays free of transport imports.
type OptionalPatch[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// BoardConfigurationPatch is the merge-patch input for a board configuration.
// Unset fields keep their stored value; a set field replaces it.
type BoardConfigurationPatch struct {
	Columns                    OptionalPatch[[]models.BoardColumnRequest]
	BacklogStatusIDs           OptionalPatch[[]int]
	ListColumns                OptionalPatch[[]models.ListColumn]
	CardFields                 OptionalPatch[[]models.ListColumn]
	RoadmapConfig              OptionalPatch[models.RoadmapConfig]
	ShowRightmostColumnLast50  OptionalPatch[bool]
	CompletedItemRetentionDays OptionalPatch[int]
	ViewSettings               OptionalPatch[models.ViewSettings]
}

// PatchBoardConfiguration applies a merge patch over the stored configuration
// and delegates to the full-replace write path, so authorization, validation,
// view-settings merging, auditing, and create-on-missing stay in one place.
// Only the fields the caller set are touched; everything else round-trips.
func (s *CollectionApplicationService) PatchBoardConfiguration(actor AuditActor, scope BoardConfigurationScope, patch BoardConfigurationPatch) (*models.BoardConfiguration, error) {
	if err := s.authorizeBoardWrite(actor.UserID, scope); err != nil {
		return nil, err
	}
	input, err := s.materializeBoardConfiguration(scope, patch)
	if err != nil {
		return nil, err
	}
	return s.PutBoardConfiguration(actor, scope, input)
}

// materializeBoardConfiguration starts from the stored configuration and
// overlays the set patch fields. A missing row starts from zero values so the
// write path creates it.
func (s *CollectionApplicationService) materializeBoardConfiguration(scope BoardConfigurationScope, patch BoardConfigurationPatch) (models.BoardConfigurationRequest, error) {
	input := models.BoardConfigurationRequest{}
	current, err := s.loadBoardConfiguration(scope)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		// No stored row yet: the patch starts from zero values.
	case err != nil:
		return input, err
	default:
		columns, columnErr := s.boards.GetColumnsWithStatuses(current.ID)
		if columnErr != nil {
			return input, columnErr
		}
		input = models.BoardConfigurationRequest{
			Columns:                    boardColumnRequests(columns),
			BacklogStatusIDs:           current.BacklogStatusIDs,
			ListColumns:                current.ListColumns,
			CardFields:                 current.CardFields,
			RoadmapConfig:              current.RoadmapConfig,
			ShowRightmostColumnLast50:  current.ShowRightmostColumnLast50,
			CompletedItemRetentionDays: current.CompletedItemRetentionDays,
			ViewSettings:               current.ViewSettings,
		}
	}
	applyBoardConfigurationPatch(&input, patch)
	return input, nil
}

// boardColumnRequests maps stored columns back to the request shape, dropping
// the server-managed fields (board_configuration_id, created_at, updated_at).
func boardColumnRequests(columns []models.BoardColumn) []models.BoardColumnRequest {
	requests := make([]models.BoardColumnRequest, 0, len(columns))
	for _, column := range columns {
		requests = append(requests, models.BoardColumnRequest{
			ID:           &column.ID,
			Name:         column.Name,
			DisplayOrder: column.DisplayOrder,
			WIPLimit:     column.WIPLimit,
			Color:        column.Color,
			StatusIDs:    column.StatusIDs,
		})
	}
	return requests
}

func applyBoardConfigurationPatch(input *models.BoardConfigurationRequest, patch BoardConfigurationPatch) {
	if patch.Columns.Set {
		input.Columns = patch.Columns.Value
	}
	if patch.BacklogStatusIDs.Set {
		input.BacklogStatusIDs = patch.BacklogStatusIDs.Value
	}
	if patch.ListColumns.Set {
		input.ListColumns = patch.ListColumns.Value
	}
	if patch.CardFields.Set {
		input.CardFields = patch.CardFields.Value
	}
	if patch.RoadmapConfig.Set {
		if patch.RoadmapConfig.Null {
			input.RoadmapConfig = nil
		} else {
			roadmap := patch.RoadmapConfig.Value
			input.RoadmapConfig = &roadmap
		}
	}
	if patch.ShowRightmostColumnLast50.Set {
		input.ShowRightmostColumnLast50 = patch.ShowRightmostColumnLast50.Value
	}
	if patch.CompletedItemRetentionDays.Set {
		if patch.CompletedItemRetentionDays.Null {
			input.CompletedItemRetentionDays = nil
		} else {
			days := patch.CompletedItemRetentionDays.Value
			input.CompletedItemRetentionDays = &days
		}
	}
	if patch.ViewSettings.Set {
		if patch.ViewSettings.Null {
			input.ViewSettings = nil
		} else {
			settings := patch.ViewSettings.Value
			input.ViewSettings = &settings
		}
	}
}
