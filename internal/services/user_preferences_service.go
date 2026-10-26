package services

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrInvalidColorMode is returned when a preference update contains an unsupported color mode.
var ErrInvalidColorMode = errors.New("invalid color mode")

// ErrPreferencesTooLarge is returned when a preferences update would store more
// than the bounded per-user JSON blob.
var ErrPreferencesTooLarge = errors.New("user preferences exceed the storage limit")

// maxPreferencesBytes bounds the serialized per-user preferences document.
// Dashboard layout plus view state and saved filters fit comfortably; this
// only rejects pathological growth.
const maxPreferencesBytes = 1 << 20 // 1 MiB

type UserPreferencesSnapshot struct {
	ColorMode    string
	ThemeID      *int
	Theme        *models.Theme
	TUI          models.UserTUIPreferences
	ViewState    map[string]map[string]any
	SavedFilters map[string]any
}

type UserPreferencesPatch struct {
	ColorModeSet       bool
	ColorMode          string
	ThemeIDSet         bool
	ThemeID            *int
	TUIThemeSet        bool
	TUITheme           string
	TUISplitRatioSet   bool
	TUISplitRatio      *float64
	LastWorkspaceIDSet bool
	LastWorkspaceID    *int
	// ViewStateSet applies a per-scope view-state merge. A nil scope value
	// deletes the whole scope; a nil view value deletes that view.
	ViewStateSet bool
	ViewState    map[string]map[string]any
	// SavedFiltersSet replaces the stored filter list per provided scope. A
	// nil scope value deletes that scope.
	SavedFiltersSet bool
	SavedFilters    map[string]any
}

// UserPreferencesService owns user preference use cases.
type UserPreferencesService struct {
	prefsRepo         *repository.UserPreferencesRepository
	themeRepo         *repository.ThemeRepository
	permissionService *PermissionService
}

// NewUserPreferencesService creates a UserPreferencesService.
func NewUserPreferencesService(prefsRepo *repository.UserPreferencesRepository, themeRepo *repository.ThemeRepository, permissionServices ...*PermissionService) *UserPreferencesService {
	service := &UserPreferencesService{prefsRepo: prefsRepo, themeRepo: themeRepo}
	if len(permissionServices) > 0 {
		service.permissionService = permissionServices[0]
	}
	return service
}

func (s *UserPreferencesService) loadData(userID int) (models.UserPreferencesData, error) {
	var prefs models.UserPreferencesData
	prefsJSON, err := s.prefsRepo.GetJSON(userID)
	if errors.Is(err, repository.ErrNotFound) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if prefsJSON == "" {
		return prefs, nil
	}
	if err := json.Unmarshal([]byte(prefsJSON), &prefs); err != nil {
		return prefs, err
	}
	return prefs, nil
}

func (s *UserPreferencesService) saveData(userID int, prefs models.UserPreferencesData) error {
	prefsBytes, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if len(prefsBytes) > maxPreferencesBytes {
		return ErrPreferencesTooLarge
	}
	return s.prefsRepo.UpsertJSON(userID, string(prefsBytes), time.Now())
}

func (s *UserPreferencesService) GetSnapshot(userID int) (UserPreferencesSnapshot, error) {
	prefs, err := s.loadData(userID)
	if err != nil {
		return UserPreferencesSnapshot{}, err
	}

	colorMode := prefs.ColorMode
	if colorMode == "" {
		colorMode = "system"
	}
	var theme *models.Theme
	if prefs.ThemeID != nil {
		if resolved, err := s.themeRepo.GetByID(*prefs.ThemeID); err == nil {
			theme = &resolved
		}
	}

	tui, err := s.GetTUI(userID)
	if err != nil {
		return UserPreferencesSnapshot{}, err
	}
	return UserPreferencesSnapshot{
		ColorMode: colorMode, ThemeID: prefs.ThemeID,
		Theme: theme, TUI: tui,
		ViewState:    prefs.ViewState,
		SavedFilters: prefs.SavedFilters,
	}, nil
}

func (s *UserPreferencesService) UpdateSnapshot(userID int, patch UserPreferencesPatch) (UserPreferencesSnapshot, error) {
	prefs, err := s.loadData(userID)
	if err != nil {
		return UserPreferencesSnapshot{}, err
	}
	if patch.ColorModeSet {
		if patch.ColorMode != "light" && patch.ColorMode != "dark" && patch.ColorMode != "system" {
			return UserPreferencesSnapshot{}, ErrInvalidColorMode
		}
		prefs.ColorMode = patch.ColorMode
	}
	if patch.ThemeIDSet {
		if patch.ThemeID != nil {
			if _, err := s.themeRepo.GetByID(*patch.ThemeID); err != nil {
				return UserPreferencesSnapshot{}, err
			}
		}
		prefs.ThemeID = patch.ThemeID
	}
	if patch.TUIThemeSet || patch.TUISplitRatioSet || patch.LastWorkspaceIDSet {
		tui := models.UserTUIPreferences{}
		if prefs.TUI != nil {
			tui = *prefs.TUI
		}
		if patch.TUIThemeSet {
			tui.Theme = patch.TUITheme
		}
		if patch.TUISplitRatioSet {
			tui.SplitRatio = patch.TUISplitRatio
		}
		if patch.LastWorkspaceIDSet {
			tui.LastWorkspaceID = patch.LastWorkspaceID
		}
		prefs.TUI = &tui
	}
	if prefs.TUI != nil {
		normalizeTUIPreferences(userID, s.permissionService, prefs.TUI)
	}
	if patch.ViewStateSet {
		prefs.ViewState = mergeViewState(prefs.ViewState, patch.ViewState)
	}
	if patch.SavedFiltersSet {
		prefs.SavedFilters = mergeSavedFilters(prefs.SavedFilters, patch.SavedFilters)
	}
	if err := s.saveData(userID, prefs); err != nil {
		return UserPreferencesSnapshot{}, err
	}
	return s.GetSnapshot(userID)
}

// mergeViewState applies a per-scope, per-view merge. A nil scope deletes the
// scope; a nil view deletes that view; an empty scope after deletion is dropped.
func mergeViewState(
	current map[string]map[string]any,
	patch map[string]map[string]any,
) map[string]map[string]any {
	if len(patch) == 0 {
		return current
	}
	merged := make(map[string]map[string]any, len(current)+len(patch))
	for scope, views := range current {
		merged[scope] = views
	}
	for scope, views := range patch {
		if views == nil {
			delete(merged, scope)
			continue
		}
		next := make(map[string]any, len(merged[scope])+len(views))
		for view, value := range merged[scope] {
			next[view] = value
		}
		for view, value := range views {
			if value == nil {
				delete(next, view)
				continue
			}
			next[view] = value
		}
		if len(next) == 0 {
			delete(merged, scope)
		} else {
			merged[scope] = next
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// mergeSavedFilters replaces the filter list for every provided scope and
// deletes scopes sent as null.
func mergeSavedFilters(
	current map[string]any,
	patch map[string]any,
) map[string]any {
	if len(patch) == 0 {
		return current
	}
	merged := make(map[string]any, len(current)+len(patch))
	for scope, value := range current {
		merged[scope] = value
	}
	for scope, value := range patch {
		if value == nil {
			delete(merged, scope)
			continue
		}
		merged[scope] = value
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// GetDashboardLayout returns the user's dashboard layout or an empty layout.
func (s *UserPreferencesService) GetDashboardLayout(userID int) (models.UserDashboardLayout, error) {
	prefs, err := s.loadData(userID)
	if err != nil {
		return models.UserDashboardLayout{}, err
	}
	layout := models.UserDashboardLayout{
		Sections: []models.UserDashboardSection{},
		Widgets:  []models.UserDashboardWidget{},
	}
	if prefs.DashboardLayout != nil {
		layout = *prefs.DashboardLayout
		if layout.Sections == nil {
			layout.Sections = []models.UserDashboardSection{}
		}
		if layout.Widgets == nil {
			layout.Widgets = []models.UserDashboardWidget{}
		}
	}
	return layout, nil
}

// GetDashboardLayoutSnapshot returns the dashboard layout together with a
// stable content revision. The revision changes only when the layout changes,
// rather than whenever another field in the shared preferences row is saved.
func (s *UserPreferencesService) GetDashboardLayoutSnapshot(userID int) (models.UserDashboardLayout, string, error) {
	layout, err := s.GetDashboardLayout(userID)
	if err != nil {
		return models.UserDashboardLayout{}, "", err
	}

	raw, err := json.Marshal(layout)
	if err != nil {
		return models.UserDashboardLayout{}, "", err
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	return layout, revision, nil
}

// UpdateDashboardLayout stores the user's dashboard layout without clobbering other preferences.
func (s *UserPreferencesService) UpdateDashboardLayout(userID int, layout models.UserDashboardLayout) error {
	prefs, err := s.loadData(userID)
	if err != nil {
		return err
	}
	prefs.DashboardLayout = &layout
	return s.saveData(userID, prefs)
}

// GetTUI returns the user's SSH TUI preferences (zero value when unset).
func (s *UserPreferencesService) GetTUI(userID int) (models.UserTUIPreferences, error) {
	prefs, err := s.loadData(userID)
	if err != nil {
		return models.UserTUIPreferences{}, err
	}
	if prefs.TUI == nil {
		return models.UserTUIPreferences{}, nil
	}
	tui := *prefs.TUI
	if tui.LastWorkspaceID != nil && s.permissionService != nil {
		allowed, err := s.permissionService.HasWorkspacePermission(userID, *tui.LastWorkspaceID, models.PermissionItemView)
		if err != nil || !allowed {
			tui.LastWorkspaceID = nil
		}
	}
	return tui, nil
}

// UpdateTUI stores the SSH TUI preferences sub-document without clobbering
// other preferences. Values are normalized rather than rejected — a stale
// or odd preference should never break the TUI.
func (s *UserPreferencesService) UpdateTUI(userID int, tui models.UserTUIPreferences) error {
	normalizeTUIPreferences(userID, s.permissionService, &tui)

	prefs, err := s.loadData(userID)
	if err != nil {
		return err
	}
	prefs.TUI = &tui
	return s.saveData(userID, prefs)
}

func normalizeTUIPreferences(userID int, permissions *PermissionService, tui *models.UserTUIPreferences) {
	if len(tui.Theme) > 64 {
		tui.Theme = tui.Theme[:64]
	}
	if tui.SplitRatio != nil {
		ratio := *tui.SplitRatio
		if ratio < 0.1 {
			ratio = 0.1
		}
		if ratio > 0.9 {
			ratio = 0.9
		}
		tui.SplitRatio = &ratio
	}
	if tui.LastWorkspaceID != nil && *tui.LastWorkspaceID <= 0 {
		tui.LastWorkspaceID = nil
	}
	if tui.LastWorkspaceID != nil && permissions != nil {
		allowed, err := permissions.HasWorkspacePermission(userID, *tui.LastWorkspaceID, models.PermissionItemView)
		if err != nil || !allowed {
			tui.LastWorkspaceID = nil
		}
	}
}
