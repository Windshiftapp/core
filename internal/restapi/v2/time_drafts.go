package v2

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
)

// draftDTO is the wire shape of a user-owned draft worklog. Absolute unix
// timestamps are exposed alongside the civil fields the completion modal edits.
type draftDTO struct {
	ID                 int    `json:"id"`
	UserID             int    `json:"user_id"`
	ProjectID          *int   `json:"project_id"`
	ItemID             *int   `json:"item_id"`
	WorkspaceID        *int   `json:"workspace_id"`
	Description        string `json:"description"`
	Date               *int64 `json:"date"`
	StartTime          *int64 `json:"start_time"`
	EndTime            *int64 `json:"end_time"`
	DurationMinutes    *int   `json:"duration_minutes"`
	Source             string `json:"source"`
	SourceTimerID      *int   `json:"source_timer_id"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
	Version            int    `json:"version"`
	ProjectName        string `json:"project_name"`
	CustomerName       string `json:"customer_name"`
	CustomerBuiltinKey string `json:"customer_builtin_key,omitempty"`
	ItemTitle          string `json:"item_title"`
	WorkspaceKey       string `json:"workspace_key"`
	WorkspaceName      string `json:"workspace_name"`
}

// draftPatchRequest is a merge patch: omitted fields keep their stored value.
type draftPatchRequest struct {
	Version         int              `json:"version"`
	ProjectID       Optional[int]    `json:"project_id"`
	ItemID          Optional[int]    `json:"item_id"`
	Description     Optional[string] `json:"description"`
	Date            Optional[string] `json:"date"`
	Duration        Optional[string] `json:"duration"`
	DurationMinutes Optional[int]    `json:"duration_minutes"`
	StartTime       Optional[string] `json:"start_time"`
	EndTime         Optional[string] `json:"end_time"`
	Timezone        Optional[string] `json:"timezone"`
}

// draftBookRequest carries the completion form for booking a draft.
type draftBookRequest struct {
	Version         int    `json:"version"`
	ProjectID       *int   `json:"project_id"`
	ItemID          *int   `json:"item_id"`
	Description     string `json:"description"`
	Date            string `json:"date"`
	Duration        string `json:"duration"`
	DurationMinutes int    `json:"duration_minutes"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	Timezone        string `json:"timezone"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func listDrafts(timers timerApplication) readOperation[[]draftDTO] {
	return func(r *http.Request) ([]draftDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		drafts, err := timers.ListDrafts(user.ID)
		if err != nil {
			return nil, internalError(err)
		}
		return mapDrafts(drafts), nil
	}
}

func getDraft(timers timerApplication) readOperation[draftDTO] {
	return func(r *http.Request) (draftDTO, error) {
		user, err := principal(r)
		if err != nil {
			return draftDTO{}, err
		}
		id, err := pathID(r, "draft_id")
		if err != nil {
			return draftDTO{}, err
		}
		draft, err := timers.GetDraft(user.ID, id)
		if err != nil {
			return draftDTO{}, draftError(err)
		}
		return draftFromModel(draft), nil
	}
}

func updateDraft(timers timerApplication) jsonOperation[draftPatchRequest, draftDTO] {
	return func(r *http.Request, patch draftPatchRequest) (draftDTO, error) {
		user, err := principal(r)
		if err != nil {
			return draftDTO{}, err
		}
		id, err := pathID(r, "draft_id")
		if err != nil {
			return draftDTO{}, err
		}
		current, err := timers.GetDraft(user.ID, id)
		if err != nil {
			return draftDTO{}, draftError(err)
		}
		input := mergeDraftPatch(current, patch, user.Timezone)
		draft, err := timers.UpdateDraft(user.ID, id, patch.Version, input)
		if err != nil {
			return draftDTO{}, draftError(err)
		}
		return draftFromModel(draft), nil
	}
}

func deleteDraft(timers timerApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "draft_id")
		if err != nil {
			return err
		}
		return draftError(timers.DeleteDraft(user.ID, id))
	}
}

func bookDraft(timers timerApplication) jsonOperation[draftBookRequest, worklogDTO] {
	return func(r *http.Request, input draftBookRequest) (worklogDTO, error) {
		user, err := principal(r)
		if err != nil {
			return worklogDTO{}, err
		}
		id, err := pathID(r, "draft_id")
		if err != nil {
			return worklogDTO{}, err
		}
		worklog, err := timers.BookDraft(user.ID, id, input.Version, input.IdempotencyKey, draftBookInput(input, user.Timezone))
		if err != nil {
			return worklogDTO{}, draftError(err)
		}
		return worklogFromModel(worklog, nil), nil
	}
}

func draftBookInput(input draftBookRequest, userTimezone string) services.WorklogMutationInput {
	projectID := 0
	if input.ProjectID != nil {
		projectID = *input.ProjectID
	}
	return services.WorklogMutationInput{
		ProjectID: projectID, ItemID: input.ItemID, Description: input.Description,
		Date: input.Date, Duration: input.Duration, DurationMinutes: input.DurationMinutes,
		StartTime: input.StartTime, EndTime: input.EndTime, Timezone: input.Timezone,
		UserTimezone: userTimezone,
	}
}

// mergeDraftPatch overlays the supplied patch fields on the stored draft so an
// omitted field keeps its value. It mirrors the worklog PATCH merge: a supplied
// duration restarts the interval from the date, a supplied clock or date
// reconstructs it, and an untouched interval is preserved exactly.
func mergeDraftPatch(current *models.TimeWorklogDraft, patch draftPatchRequest, userTimezone string) services.WorklogMutationInput {
	input := services.WorklogMutationInput{
		ItemID:       current.ItemID,
		Description:  current.Description,
		UserTimezone: userTimezone,
	}
	if current.ProjectID != nil {
		input.ProjectID = *current.ProjectID
	}
	if current.Date != nil {
		input.Date = time.Unix(*current.Date, 0).UTC().Format("2006-01-02")
	}
	if current.StartTime != nil {
		input.StoredStartUnix = *current.StartTime
	}
	if current.EndTime != nil {
		input.StoredEndUnix = *current.EndTime
	}
	if current.DurationMins != nil {
		input.DurationMinutes = *current.DurationMins
	}

	if patch.ProjectID.Set {
		input.ProjectID = patch.ProjectID.Value
	}
	if patch.ItemID.Set {
		input.ItemID = optionalNullableValue(patch.ItemID)
	}
	if patch.Description.Set {
		input.Description = patch.Description.Value
	}
	if patch.Date.Set {
		input.Date = patch.Date.Value
	}
	if patch.Timezone.Set {
		input.Timezone = patch.Timezone.Value
	}
	if patch.StartTime.Set {
		input.StartTime = patch.StartTime.Value
	}
	if patch.EndTime.Set {
		input.EndTime = patch.EndTime.Value
	}
	switch {
	case patch.Duration.Set || patch.DurationMinutes.Set:
		// A duration-only edit restarts from local midnight on the entry date.
		if !patch.StartTime.Set && !patch.EndTime.Set {
			input.StoredStartUnix = 0
			input.StoredEndUnix = 0
		}
		input.DurationMinutes = 0
		if patch.Duration.Set {
			input.Duration = patch.Duration.Value
		} else {
			input.DurationMinutes = patch.DurationMinutes.Value
		}
	case patch.Date.Set || patch.StartTime.Set || patch.EndTime.Set:
		input.DurationMinutes = 0
	default:
		input.PreserveStored = true
	}
	return input
}

func mapDrafts(drafts []models.TimeWorklogDraft) []draftDTO {
	out := make([]draftDTO, len(drafts))
	for i := range drafts {
		out[i] = draftFromModel(&drafts[i])
	}
	return out
}

func draftFromModel(draft *models.TimeWorklogDraft) draftDTO {
	return draftDTO{
		ID: draft.ID, UserID: draft.UserID, ProjectID: draft.ProjectID, ItemID: draft.ItemID,
		WorkspaceID: draft.WorkspaceID, Description: draft.Description, Date: draft.Date,
		StartTime: draft.StartTime, EndTime: draft.EndTime, DurationMinutes: draft.DurationMins,
		Source: draft.Source, SourceTimerID: draft.SourceTimerID, CreatedAt: draft.CreatedAt,
		UpdatedAt: draft.UpdatedAt, Version: draft.Version, ProjectName: draft.ProjectName,
		CustomerName: draft.CustomerName, CustomerBuiltinKey: draft.CustomerBuiltin,
		ItemTitle: draft.ItemTitle, WorkspaceKey: draft.WorkspaceKey, WorkspaceName: draft.WorkspaceName,
	}
}

func draftError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrTimerDraftNotFound):
		return newError(http.StatusNotFound, "not_found", "Draft was not found")
	case errors.Is(err, services.ErrTimerDraftConflict):
		return newError(http.StatusConflict, "conflict", "Draft was modified; reload and try again")
	case errors.Is(err, services.ErrTimerForbidden),
		errors.Is(err, services.ErrWorklogProjectNotFound):
		return newError(http.StatusNotFound, "not_found", "Time project was not found")
	case errors.Is(err, services.ErrWorklogProjectInactive),
		errors.Is(err, services.ErrWorklogInvalidInput):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
