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
	ColorMode string                    `json:"color_mode"`
	ThemeID   *int                      `json:"theme_id"`
	Theme     *models.Theme             `json:"theme"`
	TUI       models.UserTUIPreferences `json:"tui"`
}

type tuiPreferencesPatch struct {
	Theme           Optional[string]  `json:"theme"`
	SplitRatio      Optional[float64] `json:"split_ratio"`
	LastWorkspaceID Optional[int]     `json:"last_workspace_id"`
}

type preferencesPatchRequest struct {
	ColorMode Optional[string]              `json:"color_mode"`
	ThemeID   Optional[int]                 `json:"theme_id"`
	TUI       Optional[tuiPreferencesPatch] `json:"tui"`
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
	return preferencesDTO{
		ColorMode: snapshot.ColorMode, ThemeID: snapshot.ThemeID,
		Theme: snapshot.Theme, TUI: snapshot.TUI,
	}
}

func preferenceMutationError(err error) error {
	switch {
	case errors.Is(err, services.ErrInvalidColorMode):
		return newError(http.StatusBadRequest, "invalid_request", "color_mode must be light, dark, or system")
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return newError(http.StatusNotFound, "not_found", "Theme was not found")
	default:
		return internalError(err)
	}
}
