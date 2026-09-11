package services

import (
	"errors"
	"fmt"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// TimerService owns the start/stop lifecycle for active timers. Both the
// REST handler (handlers/active_timers.go) and the AI tools
// (aitools/time.go) go through this service so that workspace/item access
// validation and worklog creation live in exactly one place.
type TimerService struct {
	repo        *repository.ActiveTimerRepository
	itemRepo    *repository.ItemRepository
	timePerm    *TimePermissionService
	permService *PermissionService
	// workspaceAccess is kept as a function so every timer response uses the
	// same fail-closed gate and focused tests can exercise revocation behavior.
	workspaceAccess func(userID, workspaceID int) (bool, error)
}

// NewTimerService wires the dependencies the service needs to enforce all
// of the start/stop invariants.
func NewTimerService(
	repo *repository.ActiveTimerRepository,
	itemRepo *repository.ItemRepository,
	timePerm *TimePermissionService,
	permService *PermissionService,
) *TimerService {
	service := &TimerService{
		repo:        repo,
		itemRepo:    itemRepo,
		timePerm:    timePerm,
		permService: permService,
	}
	if permService != nil {
		service.workspaceAccess = func(userID, workspaceID int) (bool, error) {
			return permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		}
	}
	return service
}

// Typed error sentinels — callers (HTTP handler, AI tool) map these to
// their protocol's response shape.
var (
	ErrTimerValidation      = errors.New("timer: validation failed")
	ErrTimerNotFound        = errors.New("timer: not found")
	ErrTimerForbidden       = errors.New("timer: forbidden")
	ErrTimerProjectInactive = errors.New("timer: project not active")
	ErrTimerAlreadyRunning  = errors.New("timer: a timer is already running")
)

// StopResult is the data returned to callers when an active timer is stopped.
type StopResult struct {
	TimerID         int
	WorkspaceID     int
	ProjectID       int
	Description     string
	StartTimeUTC    int64
	EndTimeUTC      int64
	DurationSeconds int64
	DurationMinutes int
	WorklogCreated  bool
	ProjectName     string
	ItemTitle       string
	WorkspaceName   string
}

// StartTimer creates a new active timer for userID after validating all
// access invariants.
//
// Order matters: a 404-style result (ErrTimerNotFound) is returned for
// workspace/item permission failures so callers can't probe existence by
// observing 403 vs 404 (see MEMORY.md, "Security Policy").
func (s *TimerService) StartTimer(
	userID, workspaceID, projectID int,
	itemID *int,
	description string,
) (*models.ActiveTimer, error) {
	sanitize.Apply(&description, sanitize.RichText)
	if description == "" {
		return nil, fmt.Errorf("%w: description is required", ErrTimerValidation)
	}
	if workspaceID <= 0 {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrTimerValidation)
	}
	if projectID <= 0 {
		return nil, fmt.Errorf("%w: project_id is required", ErrTimerValidation)
	}

	canBook, err := s.timePerm.CanBookTimeOnProject(userID, projectID)
	if err != nil {
		return nil, err
	}
	if !canBook {
		return nil, ErrTimerForbidden
	}

	projectStatus, err := s.repo.GetProjectStatus(projectID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("%w: project", ErrTimerNotFound)
	}
	if err != nil {
		return nil, err
	}
	if projectStatus != "Active" {
		return nil, ErrTimerProjectInactive
	}
	projectCustomerID, err := s.repo.GetProjectCustomerID(projectID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("%w: project", ErrTimerNotFound)
	}
	if err != nil {
		return nil, err
	}
	if projectCustomerID == nil {
		return nil, fmt.Errorf("%w: project has no customer assigned", ErrTimerValidation)
	}

	// Workspace access: 404 (not 403) on failure per project policy.
	canViewWS, err := s.permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return nil, err
	}
	if !canViewWS {
		return nil, fmt.Errorf("%w: workspace", ErrTimerNotFound)
	}

	// Item access: must exist, must belong to the supplied workspace,
	// and the user must be able to view it (workspace check above
	// already covers the workspace, but we re-check defensively in
	// case items table moves to its own permission scope later).
	if itemID != nil && *itemID > 0 {
		wsID, err := s.itemRepo.GetWorkspaceID(*itemID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: item", ErrTimerNotFound)
		}
		if err != nil {
			return nil, err
		}
		if wsID != workspaceID {
			return nil, fmt.Errorf("%w: item", ErrTimerNotFound)
		}
		canViewItemWS, err := s.permService.HasWorkspacePermission(userID, wsID, models.PermissionItemView)
		if err != nil {
			return nil, err
		}
		if !canViewItemWS {
			return nil, fmt.Errorf("%w: item", ErrTimerNotFound)
		}
	}

	// Fast-path pre-check; the UNIQUE(user_id) index on active_timers is the
	// authoritative backstop for the race two concurrent starts would otherwise
	// win (the duplicate-entry mapping below).
	hasActive, err := s.repo.HasActiveTimerForUser(userID)
	if err != nil {
		return nil, err
	}
	if hasActive {
		return nil, ErrTimerAlreadyRunning
	}

	now := time.Now().UTC().Unix()
	id, err := s.repo.CreateTimer(repository.CreateTimerInput{
		WorkspaceID:  workspaceID,
		ItemID:       itemID,
		ProjectID:    projectID,
		UserID:       userID,
		Description:  description,
		StartTimeUTC: now,
	})
	if errors.Is(err, repository.ErrDuplicateEntry) {
		// Lost the start/start race: another timer was inserted between our
		// pre-check and this insert. The UNIQUE index rejected the duplicate.
		return nil, ErrTimerAlreadyRunning
	}
	if err != nil {
		return nil, err
	}

	timer, err := s.repo.GetTimerByID(id)
	if err != nil {
		return nil, err
	}
	return timer, nil
}

// GetActiveForUser returns the user's active timer with item/workspace
// metadata removed when access has been revoked since the timer started.
func (s *TimerService) GetActiveForUser(userID int) (*models.ActiveTimer, error) {
	timer, err := s.repo.GetTimerForUser(userID)
	if err != nil {
		return nil, err
	}
	if !s.canViewTimerMetadata(userID, timer) {
		redactActiveTimerMetadata(timer)
	}
	return timer, nil
}

// StopActiveForUser stops whichever timer the user currently has running.
// The AI tool (stop_timer) calls this — it does not pass a timer ID.
func (s *TimerService) StopActiveForUser(userID int) (*StopResult, error) {
	timer, err := s.repo.GetTimerForUser(userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTimerNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.StopTimerByID(userID, timer.ID)
}

// StopTimerByID stops the specified timer after verifying ownership.
// The REST handler calls this — it parses the ID from the URL.
//
// If the timer carries an item link that the caller can no longer view
// (the workspace was revoked between start and stop), the worklog is
// still recorded against the project but the item link is dropped. This
// is defense-in-depth for finding 2 in bughunt8: pre-existing rows from
// before StartTimer's validation tightening could otherwise be flushed
// into worklogs with a forged item association.
func (s *TimerService) StopTimerByID(userID, timerID int) (*StopResult, error) {
	timer, err := s.repo.GetTimerByID(timerID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTimerNotFound
	}
	if err != nil {
		return nil, err
	}
	if timer.UserID != userID {
		return nil, ErrTimerForbidden
	}

	metadataVisible := s.canViewTimerMetadata(userID, timer)

	// Drop the item link if access has since been revoked or the item no
	// longer exists. Permission lookup failures fail closed so stopping a timer
	// can never become a metadata side channel.
	itemID := timer.ItemID
	if !metadataVisible {
		itemID = nil
	} else if itemID != nil && *itemID > 0 {
		wsID, err := s.itemRepo.GetWorkspaceID(*itemID)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			itemID = nil
		case err != nil:
			itemID = nil
		default:
			canViewWS, permErr := s.permService.HasWorkspacePermission(userID, wsID, models.PermissionItemView)
			if permErr != nil || !canViewWS || wsID != timer.WorkspaceID {
				itemID = nil
			}
		}
	}

	endTimeUTC := time.Now().UTC().Unix()
	durationSeconds := endTimeUTC - timer.StartTimeUTC
	durationMinutes := int(durationSeconds / 60)

	customerID, err := s.repo.GetProjectCustomerID(timer.ProjectID)
	if err != nil {
		return nil, err
	}
	if customerID == nil {
		// The project may have lost its customer after this timer started, or
		// the row may predate start-time validation. No valid worklog can be
		// created, but deleting the timer prevents the user from being wedged.
		if err := s.repo.DeleteTimer(timer.ID); err != nil {
			return nil, err
		}
		return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, false, metadataVisible), nil
	}

	timezone, err := s.repo.GetUserTimezone(userID)
	if err != nil {
		return nil, err
	}
	_, location, timezoneErr := ResolveTimezone(timezone)
	if timezoneErr != nil {
		// Do not wedge a running timer because of a legacy invalid user setting;
		// UTC matches the pre-0.8.5 attribution behavior.
		location = time.UTC
	}
	startTime := time.Unix(timer.StartTimeUTC, 0).In(location)
	dateInt := int(WorklogDateUnix(startTime))
	nowUnix := time.Now().UTC().Unix()

	if err := s.repo.FinalizeTimer(timer.ID, repository.CreateWorklogInput{
		ProjectID:       timer.ProjectID,
		CustomerID:      *customerID,
		UserID:          userID,
		ItemID:          itemID,
		Description:     timer.Description,
		DateUnix:        dateInt,
		StartTimeUnix:   int(timer.StartTimeUTC),
		EndTimeUnix:     int(endTimeUTC),
		DurationMinutes: durationMinutes,
		NowUnix:         nowUnix,
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerNotFound
		}
		return nil, err
	}

	return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, true, metadataVisible), nil
}

func (s *TimerService) canViewTimerWorkspace(userID, workspaceID int) bool {
	if s.workspaceAccess == nil {
		return false
	}
	allowed, err := s.workspaceAccess(userID, workspaceID)
	return err == nil && allowed
}

func (s *TimerService) canViewTimerMetadata(userID int, timer *models.ActiveTimer) bool {
	if timer == nil || !s.canViewTimerWorkspace(userID, timer.WorkspaceID) {
		return false
	}
	if timer.ItemID == nil || *timer.ItemID <= 0 {
		return true
	}
	itemWorkspaceID, err := s.itemRepo.GetWorkspaceID(*timer.ItemID)
	if err != nil || itemWorkspaceID != timer.WorkspaceID {
		return false
	}
	return s.canViewTimerWorkspace(userID, itemWorkspaceID)
}

func redactActiveTimerMetadata(timer *models.ActiveTimer) {
	timer.WorkspaceID = 0
	timer.ItemID = nil
	timer.ItemTitle = nil
	timer.WorkspaceName = nil
	timer.WorkspaceKey = nil
	timer.WorkspaceItemNumber = nil
}

func buildStopResult(
	timer *models.ActiveTimer,
	endTimeUTC, durationSeconds int64,
	durationMinutes int,
	worklogCreated, metadataVisible bool,
) *StopResult {
	res := &StopResult{
		TimerID:         timer.ID,
		ProjectID:       timer.ProjectID,
		Description:     timer.Description,
		StartTimeUTC:    timer.StartTimeUTC,
		EndTimeUTC:      endTimeUTC,
		DurationSeconds: durationSeconds,
		DurationMinutes: durationMinutes,
		WorklogCreated:  worklogCreated,
	}
	if metadataVisible {
		res.WorkspaceID = timer.WorkspaceID
	}
	if timer.ProjectName != nil {
		res.ProjectName = *timer.ProjectName
	}
	if metadataVisible && timer.ItemTitle != nil {
		res.ItemTitle = *timer.ItemTitle
	}
	if metadataVisible && timer.WorkspaceName != nil {
		res.WorkspaceName = *timer.WorkspaceName
	}
	return res
}
