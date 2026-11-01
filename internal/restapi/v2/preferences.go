package v2

import (
	"database/sql"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPreferenceRoutes(builder *routeBuilder, preferences preferencesApplication) {
	builder.Read("/users/me/preferences", AuthAuthenticated, []string{"user-preferences:read"}, getPreferences(preferences))
	builder.JSON(http.MethodPatch, "/users/me/preferences", http.StatusOK, true, AuthAuthenticated, []string{"user-preferences:write"}, updatePreferences(preferences))
}

type preferencesDTO struct {
	ColorMode    string                    `json:"color_mode"`
	ThemeID      *int                      `json:"theme_id"`
	Theme        *models.Theme             `json:"theme"`
	TUI          models.UserTUIPreferences `json:"tui"`
	ViewState    map[string]map[string]any `json:"view_state"`
	SavedFilters map[string]any            `json:"saved_filters"`
	// DueDateRemindersEnabled and DueDateReminderDays drive the per-user
	// automatic reminders for upcoming due items.
	DueDateRemindersEnabled bool `json:"due_date_reminders_enabled"`
	DueDateReminderDays     int  `json:"due_date_reminder_days"`
	// CommentSortOrder is the user's preferred comment thread order.
	CommentSortOrder string `json:"comment_sort_order"`
}

type tuiPreferencesPatch struct {
	Theme           Optional[string]  `json:"theme"`
	SplitRatio      Optional[float64] `json:"split_ratio"`
	LastWorkspaceID Optional[int]     `json:"last_workspace_id"`
}

type preferencesPatchRequest struct {
	ColorMode    Optional[string]                    `json:"color_mode"`
	ThemeID      Optional[int]                       `json:"theme_id"`
	TUI          Optional[tuiPreferencesPatch]       `json:"tui"`
	ViewState    Optional[map[string]map[string]any] `json:"view_state"`
	SavedFilters Optional[map[string]any]            `json:"saved_filters"`

	DueDateRemindersEnabled Optional[bool] `json:"due_date_reminders_enabled"`
	DueDateReminderDays     Optional[int]  `json:"due_date_reminder_days"`

	CommentSortOrder Optional[string] `json:"comment_sort_order"`
}

func getPreferences(preferences preferencesApplication) readOperation[preferencesDTO] {
	return func(r *http.Request) (preferencesDTO, error) {
		user, err := principal(r)
		if err != nil {
			return preferencesDTO{}, err
		}
		snapshot, err := preferences.GetSnapshot(user.ID)
		if err != nil {
			return preferencesDTO{}, internalError(err)
		}
		return preferencesFromSnapshot(snapshot), nil
	}
}

func updatePreferences(preferences preferencesApplication) jsonOperation[preferencesPatchRequest, preferencesDTO] {
	return func(r *http.Request, input preferencesPatchRequest) (preferencesDTO, error) {
		user, err := principal(r)
		if err != nil {
			return preferencesDTO{}, err
		}
		if input.ColorMode.Null {
			return preferencesDTO{}, newError(http.StatusBadRequest, "invalid_request", "color_mode cannot be null")
		}
		patch := services.UserPreferencesPatch{
			ColorModeSet: input.ColorMode.Set, ColorMode: input.ColorMode.Value,
			ThemeIDSet: input.ThemeID.Set,
		}
		if input.ThemeID.Set && !input.ThemeID.Null {
			patch.ThemeID = &input.ThemeID.Value
		}
		if input.ViewState.Set {
			patch.ViewStateSet = true
			if !input.ViewState.Null {
				patch.ViewState = input.ViewState.Value
			}
		}
		if input.SavedFilters.Set {
			patch.SavedFiltersSet = true
			if !input.SavedFilters.Null {
				patch.SavedFilters = input.SavedFilters.Value
			}
		}
		if input.DueDateRemindersEnabled.Set {
			patch.DueDateRemindersEnabledSet = true
			patch.DueDateRemindersEnabled = input.DueDateRemindersEnabled.Value
		}
		if input.DueDateReminderDays.Set {
			patch.DueDateReminderDaysSet = true
			patch.DueDateReminderDays = input.DueDateReminderDays.Value
		}
		if input.CommentSortOrder.Set {
			patch.CommentSortOrderSet = true
			patch.CommentSortOrder = input.CommentSortOrder.Value
		}
		if input.TUI.Set {
			if input.TUI.Null {
				patch.TUIThemeSet = true
				patch.TUISplitRatioSet = true
				patch.LastWorkspaceIDSet = true
			} else {
				patch.TUIThemeSet = input.TUI.Value.Theme.Set
				if !input.TUI.Value.Theme.Null {
					patch.TUITheme = input.TUI.Value.Theme.Value
				}
				patch.TUISplitRatioSet = input.TUI.Value.SplitRatio.Set
				if input.TUI.Value.SplitRatio.Set && !input.TUI.Value.SplitRatio.Null {
					patch.TUISplitRatio = &input.TUI.Value.SplitRatio.Value
				}
				patch.LastWorkspaceIDSet = input.TUI.Value.LastWorkspaceID.Set
				if input.TUI.Value.LastWorkspaceID.Set && !input.TUI.Value.LastWorkspaceID.Null {
					patch.LastWorkspaceID = &input.TUI.Value.LastWorkspaceID.Value
				}
			}
		}
		snapshot, err := preferences.UpdateSnapshot(user.ID, patch)
		if err != nil {
			return preferencesDTO{}, preferenceMutationError(err)
		}
		return preferencesFromSnapshot(snapshot), nil
	}
}

func preferencesFromSnapshot(snapshot services.UserPreferencesSnapshot) preferencesDTO {
	viewState := snapshot.ViewState
	if viewState == nil {
		viewState = map[string]map[string]any{}
	}
	savedFilters := snapshot.SavedFilters
	if savedFilters == nil {
		savedFilters = map[string]any{}
	}
	return preferencesDTO{
		ColorMode: snapshot.ColorMode, ThemeID: snapshot.ThemeID,
		Theme: snapshot.Theme, TUI: snapshot.TUI,
		ViewState: viewState, SavedFilters: savedFilters,
		DueDateRemindersEnabled: snapshot.DueDateRemindersEnabled,
		DueDateReminderDays:     snapshot.DueDateReminderDays,
		CommentSortOrder:        snapshot.CommentSortOrder,
	}
}

func preferenceMutationError(err error) error {
	switch {
	case errors.Is(err, services.ErrInvalidColorMode):
		return newError(http.StatusBadRequest, "invalid_request", "color_mode must be light, dark, or system")
	case errors.Is(err, services.ErrInvalidReminderDays):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrInvalidCommentSortOrder):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrPreferencesTooLarge):
		return newError(http.StatusRequestEntityTooLarge, "payload_too_large", "Preferences exceed the storage limit")
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return newError(http.StatusNotFound, "not_found", "Theme was not found")
	default:
		return internalError(err)
	}
}
