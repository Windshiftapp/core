package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

const MaxWorklogDurationMinutes = 24 * 60

var ErrWorklogItemNotFound = errors.New("item not found")

type WorklogTimeInput struct {
	Duration        string
	DurationMinutes int
	StartTime       string
	EndTime         string
}

// ParseWorklogTimes validates the supported time-input forms and returns
// normalized Unix timestamps. The civil date plus clock values are exact
// wall-clock values in date's location; a supplied end clock at or before the
// start clock resolves on the following day so overnight work stays one entry.
// Explicit start/end clocks take precedence over duration; a duration supplied
// alongside them must agree or the input is rejected. Duration-only input
// keeps the established convention of starting at local midnight on date.
// A single worklog is capped at one day; longer work belongs in separate
// dated entries.
func ParseWorklogTimes(date time.Time, input WorklogTimeInput) (durationMinutes int, startUnix, endUnix int64, err error) {
	var duration time.Duration
	var start, end time.Time

	suppliedDuration := input.Duration != "" || input.DurationMinutes > 0
	switch {
	case input.StartTime != "" && input.EndTime != "":
		start, err = ResolveCivilClock(date, input.StartTime)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid start_time: %w", err)
		}
		end, err = resolveWorklogEndClock(date, input.EndTime, start)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid end_time: %w", err)
		}
		duration = end.Sub(start)
		if suppliedDuration {
			supplied, err := parseSuppliedDuration(input)
			if err != nil {
				return 0, 0, 0, err
			}
			if supplied != duration {
				return 0, 0, 0, fmt.Errorf("duration does not match start_time and end_time")
			}
		}
	case input.StartTime != "" && suppliedDuration:
		start, err = ResolveCivilClock(date, input.StartTime)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid start_time: %w", err)
		}
		duration, err = parseSuppliedDuration(input)
		if err != nil {
			return 0, 0, 0, err
		}
		end = start.Add(duration)
	case input.StartTime != "" || input.EndTime != "":
		return 0, 0, 0, fmt.Errorf("provide both start_time and end_time")
	default:
		duration, err = parseSuppliedDuration(input)
		if err != nil {
			return 0, 0, 0, err
		}
		start = date
		end = date.Add(duration)
	}

	durationMinutes = int(duration / time.Minute)
	if durationMinutes <= 0 {
		return 0, 0, 0, fmt.Errorf("duration must be positive")
	}
	if duration > time.Duration(MaxWorklogDurationMinutes)*time.Minute {
		return 0, 0, 0, fmt.Errorf("duration must not exceed %d minutes", MaxWorklogDurationMinutes)
	}
	if !end.After(start) {
		return 0, 0, 0, fmt.Errorf("end_time must be after start_time")
	}

	return durationMinutes, start.Unix(), end.Unix(), nil
}

// parseSuppliedDuration reads whichever duration field the caller sent,
// applying the shared positive and one-day limits.
func parseSuppliedDuration(input WorklogTimeInput) (time.Duration, error) {
	if input.Duration != "" {
		parsed, err := utils.ParseDuration(input.Duration)
		if err != nil {
			return 0, fmt.Errorf("invalid duration: %w", err)
		}
		return parsed, nil
	}
	return ValidateWorklogDurationMinutes(input.DurationMinutes)
}

// resolveWorklogEndClock resolves the end clock on the selected day; a value
// at or before the resolved start carries to the next day so entries like
// 23:00-01:00 store one continuous interval.
func resolveWorklogEndClock(date time.Time, clock string, start time.Time) (time.Time, error) {
	sameDay, err := ResolveCivilClock(date, clock)
	if err != nil {
		return time.Time{}, err
	}
	if sameDay.After(start) {
		return sameDay, nil
	}
	return ResolveCivilClock(date.AddDate(0, 0, 1), clock)
}

func ValidateWorklogDurationMinutes(minutes int) (time.Duration, error) {
	if minutes <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	if minutes > MaxWorklogDurationMinutes {
		return 0, fmt.Errorf("duration must not exceed %d minutes", MaxWorklogDurationMinutes)
	}
	return time.Duration(minutes) * time.Minute, nil
}

// ResolveAccessibleWorklogItem normalizes numeric and key references through
// one permission-aware lookup. Every failure deliberately collapses to
// ErrWorklogItemNotFound so callers do not disclose item existence.
func ResolveAccessibleWorklogItem(
	db database.Database,
	itemID int,
	itemKey string,
	canViewWorkspace func(workspaceID int) (bool, error),
) (int, error) {
	resolvedID := itemID
	itemRepo := repository.NewItemRepository(db)
	if itemKey != "" {
		parts := strings.SplitN(itemKey, "-", 2)
		if len(parts) != 2 {
			return 0, ErrWorklogItemNotFound
		}
		itemNumber, err := strconv.Atoi(parts[1])
		if err != nil || itemNumber <= 0 {
			return 0, ErrWorklogItemNotFound
		}
		resolvedID, err = itemRepo.FindIDByKeyAndNumber(parts[0], itemNumber)
		if err != nil {
			return 0, ErrWorklogItemNotFound
		}
	}
	if resolvedID <= 0 {
		return 0, ErrWorklogItemNotFound
	}

	workspaceID, err := itemRepo.GetWorkspaceID(resolvedID)
	if err != nil {
		return 0, ErrWorklogItemNotFound
	}
	allowed, err := canViewWorkspace(workspaceID)
	if err != nil || !allowed {
		return 0, ErrWorklogItemNotFound
	}
	return resolvedID, nil
}

// RedactInaccessibleWorklogItems returns a copy with all item-identifying
// fields removed when the caller cannot view the linked workspace.
func RedactInaccessibleWorklogItems(
	worklogs []models.Worklog,
	canViewWorkspace func(workspaceID int) (bool, error),
) []models.Worklog {
	redacted := append([]models.Worklog(nil), worklogs...)
	access := make(map[int]bool)
	checked := make(map[int]bool)
	for i := range redacted {
		worklog := &redacted[i]
		if worklog.ItemID == nil || worklog.WorkspaceID == nil {
			continue
		}

		workspaceID := *worklog.WorkspaceID
		if !checked[workspaceID] {
			allowed, err := canViewWorkspace(workspaceID)
			access[workspaceID] = err == nil && allowed
			checked[workspaceID] = true
		}
		if access[workspaceID] {
			continue
		}

		worklog.ItemID = nil
		worklog.ItemTitle = ""
		worklog.WorkspaceID = nil
		worklog.WorkspaceKey = ""
		worklog.WorkspaceItemNumber = 0
	}
	return redacted
}
