package services

import (
	"errors"
	"fmt"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
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
}

// NewTimerService wires the dependencies the service needs to enforce all
// of the start/stop invariants.
func NewTimerService(
	repo *repository.ActiveTimerRepository,
	itemRepo *repository.ItemRepository,
	timePerm *TimePermissionService,
	permService *PermissionService,
) *TimerService {
	return &TimerService{
		repo:        repo,
		itemRepo:    itemRepo,
		timePerm:    timePerm,
		permService: permService,
	}
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
	if err != nil {
		return nil, err
	}

	timer, err := s.repo.GetTimerByID(id)
	if err != nil {
		return nil, err
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

	// Drop the item link if access has since been revoked or the item
	// no longer exists.
	itemID := timer.ItemID
	if itemID != nil && *itemID > 0 {
		wsID, err := s.itemRepo.GetWorkspaceID(*itemID)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			itemID = nil
		case err != nil:
			return nil, err
		default:
			canViewWS, permErr := s.permService.HasWorkspacePermission(userID, wsID, models.PermissionItemView)
			if permErr != nil {
				return nil, permErr
			}
			if !canViewWS {
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

	startTime := time.Unix(timer.StartTimeUTC, 0).UTC()
	dateInt := int(startTime.Truncate(24 * time.Hour).Unix())
	nowUnix := time.Now().UTC().Unix()

	if err := s.repo.CreateWorklog(repository.CreateWorklogInput{
		ProjectID:       timer.ProjectID,
		CustomerID:      customerID,
		UserID:          userID,
		ItemID:          itemID,
		Description:     timer.Description,
		DateUnix:        dateInt,
		StartTimeUnix:   int(timer.StartTimeUTC),
		EndTimeUnix:     int(endTimeUTC),
		DurationMinutes: durationMinutes,
		NowUnix:         nowUnix,
	}); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteTimer(timer.ID); err != nil {
		return nil, err
	}

	res := &StopResult{
		TimerID:         timer.ID,
		WorkspaceID:     timer.WorkspaceID,
		ProjectID:       timer.ProjectID,
		Description:     timer.Description,
		StartTimeUTC:    timer.StartTimeUTC,
		EndTimeUTC:      endTimeUTC,
		DurationSeconds: durationSeconds,
		DurationMinutes: durationMinutes,
		WorklogCreated:  true,
	}
	if timer.ProjectName != nil {
		res.ProjectName = *timer.ProjectName
	}
	if timer.ItemTitle != nil {
		res.ItemTitle = *timer.ItemTitle
	}
	if timer.WorkspaceName != nil {
		res.WorkspaceName = *timer.WorkspaceName
	}
	return res, nil
}
