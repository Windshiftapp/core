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
// REST handlers and the AI tools go through this service so that workspace/item
// access validation and worklog creation live in exactly one place.
//
// A timer may run without a project. On stop the service decides whether the
// entry can be booked directly (active, bookable project plus a description) or
// must be persisted as a recoverable draft worklog.
type TimerService struct {
	repo        *repository.ActiveTimerRepository
	draftRepo   *repository.TimeWorklogDraftRepository
	itemRepo    *repository.ItemRepository
	worklogs    *TimeWorklogService
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
	draftRepo *repository.TimeWorklogDraftRepository,
	itemRepo *repository.ItemRepository,
	worklogs *TimeWorklogService,
	timePerm *TimePermissionService,
	permService *PermissionService,
) *TimerService {
	service := &TimerService{
		repo:        repo,
		draftRepo:   draftRepo,
		itemRepo:    itemRepo,
		worklogs:    worklogs,
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
	ErrTimerDraftNotFound   = errors.New("timer: draft not found")
	ErrTimerDraftConflict   = errors.New("timer: draft conflict")
)

// StopResult is the data returned to callers when an active timer is stopped.
// Exactly one of Booked/WorklogID or DraftID is set: a stop either books a
// worklog directly or produces a recoverable draft.
type StopResult struct {
	TimerID         int
	WorkspaceID     int
	ProjectID       *int
	Description     string
	StartTimeUTC    int64
	EndTimeUTC      int64
	DurationSeconds int64
	DurationMinutes int
	Booked          bool
	WorklogID       *int
	DraftID         *int
	ProjectName     string
	ItemTitle       string
	WorkspaceName   string
}

// StartTimer creates a new active timer for userID after validating all access
// invariants. Project is optional; description may be empty.
//
// Order matters: a 404-style result (ErrTimerNotFound) is returned for
// workspace/item permission failures so callers can't probe existence by
// observing 403 vs 404 (see MEMORY.md, "Security Policy").
func (s *TimerService) StartTimer(
	userID, workspaceID int,
	projectID *int,
	itemID *int,
	description string,
) (*models.ActiveTimer, error) {
	sanitize.Apply(&description, sanitize.RichText)

	// A contextual item can supply the workspace when the caller did not.
	if workspaceID <= 0 && itemID != nil && *itemID > 0 {
		wsID, err := s.itemRepo.GetWorkspaceID(*itemID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: item", ErrTimerNotFound)
		}
		if err != nil {
			return nil, err
		}
		workspaceID = wsID
	}
	if workspaceID <= 0 {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrTimerValidation)
	}

	// Workspace access: 404 (not 403) on failure per project policy.
	canViewWS, err := s.permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return nil, err
	}
	if !canViewWS {
		return nil, fmt.Errorf("%w: workspace", ErrTimerNotFound)
	}

	// Item access: must exist, belong to the supplied workspace, and be
	// viewable. Re-checked defensively in case items gain their own scope.
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

	// Optional project: when supplied it must be active and bookable. A
	// projectless timer is allowed and stops into a draft.
	if projectID != nil && *projectID > 0 {
		canBook, err := s.timePerm.CanBookTimeOnProject(userID, *projectID)
		if err != nil {
			return nil, err
		}
		if !canBook {
			return nil, ErrTimerForbidden
		}
		projectStatus, err := s.repo.GetProjectStatus(*projectID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: project", ErrTimerNotFound)
		}
		if err != nil {
			return nil, err
		}
		if projectStatus != "Active" {
			return nil, ErrTimerProjectInactive
		}
	} else {
		projectID = nil
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

// StartTimerLegacy preserves the v1 contract: a project and a description are
// required. v1 is frozen and must not gain the projectless start behavior.
func (s *TimerService) StartTimerLegacy(userID, workspaceID, projectID int, itemID *int, description string) (*models.ActiveTimer, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("%w: project_id is required", ErrTimerValidation)
	}
	if description == "" {
		return nil, fmt.Errorf("%w: description is required", ErrTimerValidation)
	}
	return s.StartTimer(userID, workspaceID, &projectID, itemID, description)
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

// StopActiveForUser stops whichever timer the user currently has running using
// the legacy (v1) contract: the timer must be complete or the stop is rejected
// and the timer retained. New callers use StopActiveTimer.
func (s *TimerService) StopActiveForUser(userID int) (*StopResult, error) {
	timer, err := s.repo.GetTimerForUser(userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTimerNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.stopTimer(userID, timer, false)
}

// StopTimerByID stops the specified timer after verifying ownership using the
// legacy (v1) complete-only contract.
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
	return s.stopTimer(userID, timer, false)
}

// StopActiveTimer stops the intended timer and books or drafts it. When timerID
// is nil the user's active timer is used. A retry for a timer that was already
// stopped recovers the original result (booked worklog or draft) instead of
// stopping a newer timer.
func (s *TimerService) StopActiveTimer(userID int, timerID *int) (*StopResult, error) {
	var timer *models.ActiveTimer
	var err error
	if timerID != nil && *timerID > 0 {
		timer, err = s.repo.GetTimerByID(*timerID)
		if errors.Is(err, repository.ErrNotFound) {
			return s.recoverStoppedTimer(userID, *timerID)
		}
	} else {
		timer, err = s.repo.GetTimerForUser(userID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerNotFound
		}
	}
	if err != nil {
		return nil, err
	}
	if timer.UserID != userID {
		return nil, ErrTimerForbidden
	}
	return s.stopTimer(userID, timer, true)
}

// recoverStoppedTimer resolves a retried stop for a timer that is already gone.
// It returns the booked worklog or the draft produced by the original stop.
func (s *TimerService) recoverStoppedTimer(userID, timerID int) (*StopResult, error) {
	key := timerBookingKey(timerID)
	if booking, err := s.draftRepo.GetBooking(userID, key); err == nil {
		worklogID := booking.WorklogID
		return &StopResult{TimerID: timerID, Booked: true, WorklogID: &worklogID}, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if draft, err := s.draftRepo.GetDraftBySourceTimer(userID, timerID); err == nil {
		draftID := draft.ID
		return &StopResult{TimerID: timerID, Booked: false, DraftID: &draftID}, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	return nil, ErrTimerNotFound
}

// stopTimer performs the shared stop work. When allowDraft is false (legacy v1)
// an incomplete timer is rejected and retained.
func (s *TimerService) stopTimer(userID int, timer *models.ActiveTimer, allowDraft bool) (*StopResult, error) {
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

	bookableProject := s.bookableProject(userID, timer)
	if bookableProject != nil && timer.Description != "" {
		worklogID, err := s.repo.FinalizeTimerBooking(timer.ID, repository.CreateWorklogInput{
			ProjectID:       *bookableProject,
			UserID:          userID,
			ItemID:          itemID,
			Description:     timer.Description,
			DateUnix:        dateInt,
			StartTimeUnix:   int(timer.StartTimeUTC),
			EndTimeUnix:     int(endTimeUTC),
			DurationMinutes: durationMinutes,
			NowUnix:         nowUnix,
		}, timerBookingKey(timer.ID))
		if errors.Is(err, repository.ErrDuplicateEntry) {
			// A concurrent or retried stop already booked this timer.
			booking, lookupErr := s.draftRepo.GetBooking(userID, timerBookingKey(timer.ID))
			if lookupErr != nil {
				return nil, lookupErr
			}
			bookedID := booking.WorklogID
			return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, true, &bookedID, nil, metadataVisible), nil
		}
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerNotFound
		}
		if err != nil {
			return nil, err
		}
		return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, true, &worklogID, nil, metadataVisible), nil
	}

	if !allowDraft {
		return nil, fmt.Errorf("%w: the timer is missing booking information", ErrTimerValidation)
	}

	// Incomplete: persist a recoverable draft. Keep the project when it still
	// exists so the completion modal can offer it, but never invent a link.
	draftProject := timer.ProjectID
	if draftProject != nil {
		if _, err := s.repo.GetProjectStatus(*draftProject); errors.Is(err, repository.ErrNotFound) {
			draftProject = nil
		}
	}
	var draftWorkspace *int
	if metadataVisible {
		ws := timer.WorkspaceID
		draftWorkspace = &ws
	}
	draftID, err := s.repo.FinalizeTimerDraft(timer.ID, repository.CreateDraftInput{
		UserID:          userID,
		ProjectID:       draftProject,
		ItemID:          itemID,
		WorkspaceID:     draftWorkspace,
		Description:     timer.Description,
		DateUnix:        int64Ptr(int64(dateInt)),
		StartTimeUnix:   int64Ptr(timer.StartTimeUTC),
		EndTimeUnix:     int64Ptr(endTimeUTC),
		DurationMinutes: intPtr(durationMinutes),
		Source:          "timer",
		SourceTimerID:   intPtr(timer.ID),
		NowUnix:         nowUnix,
	})
	if errors.Is(err, repository.ErrDuplicateEntry) {
		draft, lookupErr := s.draftRepo.GetDraftBySourceTimer(userID, timer.ID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, false, nil, &draft.ID, metadataVisible), nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTimerNotFound
	}
	if err != nil {
		return nil, err
	}
	return buildStopResult(timer, endTimeUTC, durationSeconds, durationMinutes, false, nil, &draftID, metadataVisible), nil
}

// bookableProject returns the timer's project only when it still exists, is
// active, and the user may book time on it.
func (s *TimerService) bookableProject(userID int, timer *models.ActiveTimer) *int {
	if timer.ProjectID == nil || *timer.ProjectID <= 0 {
		return nil
	}
	status, err := s.repo.GetProjectStatus(*timer.ProjectID)
	if err != nil || status != "Active" {
		return nil
	}
	// Focused tests construct the service without a permission service; the
	// production wiring always supplies one.
	if s.timePerm == nil {
		return timer.ProjectID
	}
	canBook, err := s.timePerm.CanBookTimeOnProject(userID, *timer.ProjectID)
	if err != nil || !canBook {
		return nil
	}
	return timer.ProjectID
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

func timerBookingKey(timerID int) string {
	return fmt.Sprintf("timer:%d", timerID)
}

func buildStopResult(
	timer *models.ActiveTimer,
	endTimeUTC, durationSeconds int64,
	durationMinutes int,
	booked bool,
	worklogID, draftID *int,
	metadataVisible bool,
) *StopResult {
	res := &StopResult{
		TimerID:         timer.ID,
		ProjectID:       timer.ProjectID,
		Description:     timer.Description,
		StartTimeUTC:    timer.StartTimeUTC,
		EndTimeUTC:      endTimeUTC,
		DurationSeconds: durationSeconds,
		DurationMinutes: durationMinutes,
		Booked:          booked,
		WorklogID:       worklogID,
		DraftID:         draftID,
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

func int64Ptr(v int64) *int64 { return &v }

// --- Draft worklog operations -------------------------------------------
//
// Drafts are user-owned incomplete entries. Every lookup and mutation is
// scoped by the authenticated user id, so another user's draft is
// indistinguishable from a missing one.

// ListDrafts returns the caller's incomplete entries.
func (s *TimerService) ListDrafts(userID int) ([]models.TimeWorklogDraft, error) {
	return s.draftRepo.ListDrafts(userID)
}

// GetDraft returns one draft owned by the caller.
func (s *TimerService) GetDraft(userID, id int) (*models.TimeWorklogDraft, error) {
	draft, err := s.draftRepo.GetDraft(id, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTimerDraftNotFound
	}
	return draft, err
}

// UpdateDraft saves an incomplete entry, applying the same civil-time parsing
// as a worklog create but without requiring an active project.
func (s *TimerService) UpdateDraft(userID, id, expectedVersion int, input WorklogMutationInput) (*models.TimeWorklogDraft, error) {
	if _, err := s.draftRepo.GetDraft(id, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerDraftNotFound
		}
		return nil, err
	}
	prepared, err := s.worklogs.PrepareDraft(userID, input)
	if err != nil {
		return nil, err
	}
	var projectID *int
	if input.ProjectID > 0 {
		projectID = &input.ProjectID
	}
	if err := s.draftRepo.UpdateDraft(id, userID, expectedVersion, repository.UpdateDraftInput{
		ProjectID:       projectID,
		ItemID:          s.visibleDraftItem(userID, input.ItemID),
		Description:     prepared.Description,
		DateUnix:        &prepared.DateUnix,
		StartTimeUnix:   &prepared.StartUnix,
		EndTimeUnix:     &prepared.EndUnix,
		DurationMinutes: &prepared.DurationMinutes,
		NowUnix:         time.Now().UTC().Unix(),
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerDraftNotFound
		}
		if errors.Is(err, repository.ErrConflict) {
			return nil, ErrTimerDraftConflict
		}
		return nil, err
	}
	return s.GetDraft(userID, id)
}

// DeleteDraft discards an owned draft.
func (s *TimerService) DeleteDraft(userID, id int) error {
	err := s.draftRepo.DeleteDraft(id, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrTimerDraftNotFound
	}
	return err
}

// BookDraft completes a draft into a booked worklog. It revalidates the
// project and item, consumes exactly one owned draft atomically, and records
// the idempotency key so a retry replays the same worklog.
func (s *TimerService) BookDraft(userID, id, expectedVersion int, idempotencyKey string, input WorklogMutationInput) (*models.Worklog, error) {
	// A replay of a completed booking must resolve before the draft-existence
	// check: the draft is gone once booked, but the recorded key still maps to
	// the original worklog.
	if idempotencyKey != "" {
		booking, err := s.draftRepo.GetBooking(userID, idempotencyKey)
		switch {
		case err == nil:
			if booking.DraftID != nil && *booking.DraftID == id {
				return s.worklogs.Get(booking.WorklogID)
			}
			return nil, ErrTimerDraftConflict
		case !errors.Is(err, repository.ErrNotFound):
			return nil, err
		}
	}
	if _, err := s.draftRepo.GetDraft(id, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrTimerDraftNotFound
		}
		return nil, err
	}
	prepared, err := s.worklogs.Prepare(userID, input)
	if err != nil {
		return nil, err
	}
	worklogID, err := s.draftRepo.BookDraft(id, userID, expectedVersion, repository.CreateWorklogInput{
		ProjectID:       input.ProjectID,
		UserID:          userID,
		ItemID:          s.visibleDraftItem(userID, input.ItemID),
		Description:     prepared.Description,
		DateUnix:        int(prepared.DateUnix),
		StartTimeUnix:   int(prepared.StartUnix),
		EndTimeUnix:     int(prepared.EndUnix),
		DurationMinutes: prepared.DurationMinutes,
		NowUnix:         time.Now().UTC().Unix(),
	}, idempotencyKey)
	if errors.Is(err, repository.ErrDuplicateEntry) {
		booking, lookupErr := s.draftRepo.GetBooking(userID, idempotencyKey)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return s.worklogs.Get(booking.WorklogID)
	}
	if errors.Is(err, repository.ErrConflict) {
		if _, gerr := s.draftRepo.GetDraft(id, userID); errors.Is(gerr, repository.ErrNotFound) {
			return nil, ErrTimerDraftNotFound
		}
		return nil, ErrTimerDraftConflict
	}
	if err != nil {
		return nil, err
	}
	return s.worklogs.Get(worklogID)
}

// visibleDraftItem drops an item link the caller can no longer view, mirroring
// the timer-stop redaction policy.
func (s *TimerService) visibleDraftItem(userID int, itemID *int) *int {
	if itemID == nil || *itemID <= 0 {
		return nil
	}
	wsID, err := s.itemRepo.GetWorkspaceID(*itemID)
	if err != nil {
		return nil
	}
	canView, err := s.permService.HasWorkspacePermission(userID, wsID, models.PermissionItemView)
	if err != nil || !canView {
		return nil
	}
	return itemID
}
