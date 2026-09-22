package services

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrWorklogProjectNotFound = errors.New("worklog project not found")
	ErrWorklogProjectInactive = errors.New("worklog project is not active")
	ErrWorklogCustomerMissing = errors.New("worklog project has no customer")
	ErrWorklogInvalidInput    = errors.New("invalid worklog input")
)

type worklogInputError struct{ err error }

func (e worklogInputError) Error() string { return e.err.Error() }
func (e worklogInputError) Unwrap() error { return ErrWorklogInvalidInput }

// WorklogMutationInput is the common create/full-update worklog contract.
type WorklogMutationInput struct {
	ProjectID       int
	ItemID          *int
	Description     string
	Date            string
	Duration        string
	DurationMinutes int
	StartTime       string
	EndTime         string
	Timezone        string
	UserTimezone    string
	StoredStartUnix int64
	StoredEndUnix   int64
	PreserveStored  bool
}

// WorklogMutationResult contains the canonical row and normalized clock data.
type WorklogMutationResult struct {
	Worklog         *models.Worklog
	Warnings        []string
	Timezone        string
	Location        *time.Location
	StartTimeUnix   int64
	EndTimeUnix     int64
	DurationMinutes int
}

// TimeWorklogService owns worklog validation, normalization, and persistence.
type TimeWorklogService struct {
	db       database.Database
	worklogs *repository.TimeWorklogRepository
	projects *repository.TimeProjectRepository
}

// NewTimeWorklogService creates the shared worklog application boundary.
func NewTimeWorklogService(db database.Database) *TimeWorklogService {
	return &TimeWorklogService{
		db: db, worklogs: repository.NewTimeWorklogRepository(db), projects: repository.NewTimeProjectRepository(db),
	}
}

// Create validates, creates, and reloads a worklog.
func (s *TimeWorklogService) Create(userID int, input WorklogMutationInput) (*WorklogMutationResult, error) {
	prepared, err := s.prepare(userID, input)
	if err != nil {
		return nil, err
	}
	id, err := s.worklogs.Create(repository.NewWorklog{
		ProjectID: input.ProjectID, CustomerID: prepared.customerID, UserID: userID,
		ItemID: input.ItemID, Description: prepared.description, DateUnix: WorklogDateUnix(prepared.date),
		StartTimeUnix: prepared.startUnix, EndTimeUnix: prepared.endUnix, DurationMinutes: prepared.durationMinutes,
	})
	if err != nil {
		return nil, err
	}
	return s.result(int(id), prepared)
}

// Update validates, fully updates, and reloads a worklog.
func (s *TimeWorklogService) Update(userID, worklogID int, input WorklogMutationInput) (*WorklogMutationResult, error) {
	prepared, err := s.prepare(userID, input)
	if err != nil {
		return nil, err
	}
	if err := s.worklogs.Update(repository.UpdateWorklog{
		ID: worklogID, ProjectID: input.ProjectID, CustomerID: int(prepared.customerID),
		ItemID: input.ItemID, Description: prepared.description, DateUnix: WorklogDateUnix(prepared.date),
		StartTimeUnix: prepared.startUnix, EndTimeUnix: prepared.endUnix, DurationMinutes: prepared.durationMinutes,
	}); err != nil {
		return nil, err
	}
	return s.result(worklogID, prepared)
}

// UpdateDescription sanitizes and updates the bearer description-only contract.
func (s *TimeWorklogService) UpdateDescription(worklogID int, description string) ([]string, error) {
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &description, Policy: sanitize.Comment, Label: "Description"},
	)
	return warnings, s.worklogs.UpdateDescription(worklogID, description)
}

// Delete removes a worklog.
func (s *TimeWorklogService) Delete(worklogID int) error {
	return s.worklogs.Delete(worklogID)
}

// Get returns one joined worklog.
func (s *TimeWorklogService) Get(worklogID int) (*models.Worklog, error) {
	return s.worklogs.GetDetail(worklogID)
}

// ListMine returns one page of a user's joined worklogs.
func (s *TimeWorklogService) ListMine(filter repository.WorklogListFilter) ([]models.Worklog, int, error) {
	return s.worklogs.ListForUser(filter)
}

// List returns joined worklogs matching a project or item filter.
func (s *TimeWorklogService) List(filter repository.WorklogDetailFilter) ([]models.Worklog, error) {
	return s.worklogs.ListDetails(filter)
}

// ListPage returns a repository-bounded joined worklog page.
func (s *TimeWorklogService) ListPage(filter repository.WorklogDetailFilter) ([]models.Worklog, int, error) {
	return s.worklogs.ListDetailsPage(filter)
}

// WorklogDailyAggregate splits one worklog's elapsed minutes across the civil
// days it touches, in the reporting timezone.
type WorklogDailyAggregate struct {
	Day          string `json:"day"`
	UserID       int    `json:"user_id"`
	UserName     string `json:"user_name"`
	ProjectID    int    `json:"project_id"`
	ProjectName  string `json:"project_name"`
	CustomerID   int    `json:"customer_id"`
	CustomerName string `json:"customer_name"`
	Minutes      int64  `json:"minutes"`
}

// WorklogTotalAggregate sums booked duration and entry counts per
// (user, project, customer) — not day-partitioned, matching the stored
// duration_minutes contract.
type WorklogTotalAggregate struct {
	UserID          int    `json:"user_id"`
	UserName        string `json:"user_name"`
	ProjectID       int    `json:"project_id"`
	ProjectName     string `json:"project_name"`
	CustomerID      int    `json:"customer_id"`
	CustomerName    string `json:"customer_name"`
	DurationMinutes int64  `json:"duration_minutes"`
	Entries         int64  `json:"entries"`
}

// WorklogAggregate is the report document: day-split minutes for charts and
// per-member averages, plus duration/entry totals for summaries and top lists.
type WorklogAggregate struct {
	Daily  []WorklogDailyAggregate `json:"daily"`
	Totals []WorklogTotalAggregate `json:"totals"`
}

// SplitWorklogMinutesByDay slices [start, end) into minutes per civil date
// key (YYYY-MM-DD) in the location. It mirrors the frontend utility
// (splitWorklogMinutesByDay) exactly — including midnight splitting and
// half-open boundaries — so server aggregates and legacy client math agree.
func SplitWorklogMinutesByDay(start, end int64, location *time.Location) []struct {
	Day     string
	Minutes int64
} {
	type daySlice = struct {
		Day     string
		Minutes int64
	}
	slices := []daySlice{}
	if end <= start {
		return slices
	}
	cursor := time.Unix(start, 0).In(location)
	endT := time.Unix(end, 0).In(location)
	for cursor.Before(endT) {
		year, month, day := cursor.Date()
		boundary := time.Date(year, month, day+1, 0, 0, 0, 0, location)
		if !boundary.After(cursor) {
			boundary = time.Date(year, month, day+2, 0, 0, 0, 0, location)
		}
		sliceEnd := boundary
		if endT.Before(sliceEnd) {
			sliceEnd = endT
		}
		minutes := int64(sliceEnd.Sub(cursor).Minutes() + 0.5)
		if minutes > 0 {
			slices = append(slices, daySlice{Day: cursor.Format("2006-01-02"), Minutes: minutes})
		}
		cursor = sliceEnd
	}
	return slices
}

// Aggregate reduces the filtered worklogs to (day, user, project, customer)
// split-minute groups and (user, project, customer) duration/entry totals so
// reports render without fetching every raw row.
func (s *TimeWorklogService) Aggregate(filter repository.WorklogDetailFilter, timezone string) (*WorklogAggregate, error) {
	_, location, err := ResolveTimezone(timezone)
	if err != nil {
		return nil, err
	}
	inputs, err := s.worklogs.ListAggregateInputs(filter)
	if err != nil {
		return nil, err
	}

	dailyIndex := make(map[WorklogDailyAggregate]int)
	daily := make([]WorklogDailyAggregate, 0)
	totalIndex := make(map[WorklogTotalAggregate]int)
	totals := make([]WorklogTotalAggregate, 0)

	for i := range inputs {
		input := &inputs[i]
		for _, slice := range SplitWorklogMinutesByDay(input.StartTimeUnix, input.EndTimeUnix, location) {
			key := WorklogDailyAggregate{
				Day: slice.Day, UserID: input.UserID, UserName: input.UserName,
				ProjectID: input.ProjectID, ProjectName: input.ProjectName,
				CustomerID: input.CustomerID, CustomerName: input.CustomerName,
			}
			idx, ok := dailyIndex[key]
			if !ok {
				dailyIndex[key] = len(daily)
				key.Minutes = slice.Minutes
				daily = append(daily, key)
			} else {
				daily[idx].Minutes += slice.Minutes
			}
		}

		key := WorklogTotalAggregate{
			UserID: input.UserID, UserName: input.UserName,
			ProjectID: input.ProjectID, ProjectName: input.ProjectName,
			CustomerID: input.CustomerID, CustomerName: input.CustomerName,
		}
		idx, ok := totalIndex[key]
		if !ok {
			totalIndex[key] = len(totals)
			key.DurationMinutes = int64(input.DurationMinutes)
			key.Entries = 1
			totals = append(totals, key)
		} else {
			totals[idx].DurationMinutes += int64(input.DurationMinutes)
			totals[idx].Entries++
		}
	}

	sort.Slice(daily, func(i, j int) bool {
		if daily[i].Day != daily[j].Day {
			return daily[i].Day < daily[j].Day
		}
		if daily[i].UserID != daily[j].UserID {
			return daily[i].UserID < daily[j].UserID
		}
		if daily[i].ProjectID != daily[j].ProjectID {
			return daily[i].ProjectID < daily[j].ProjectID
		}
		return daily[i].CustomerID < daily[j].CustomerID
	})
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].UserID != totals[j].UserID {
			return totals[i].UserID < totals[j].UserID
		}
		if totals[i].ProjectID != totals[j].ProjectID {
			return totals[i].ProjectID < totals[j].ProjectID
		}
		return totals[i].CustomerID < totals[j].CustomerID
	})
	return &WorklogAggregate{Daily: daily, Totals: totals}, nil
}

type preparedWorklog struct {
	customerID      int64
	description     string
	date            time.Time
	startUnix       int64
	endUnix         int64
	durationMinutes int
	timezone        string
	location        *time.Location
	warnings        []string
}

func (s *TimeWorklogService) prepare(userID int, input WorklogMutationInput) (preparedWorklog, error) {
	project, err := s.projects.GetBookingInfo(input.ProjectID)
	if errors.Is(err, repository.ErrNotFound) {
		return preparedWorklog{}, ErrWorklogProjectNotFound
	}
	if err != nil {
		return preparedWorklog{}, err
	}
	if project.CustomerID == nil {
		return preparedWorklog{}, ErrWorklogCustomerMissing
	}
	if project.Status != "Active" {
		return preparedWorklog{}, fmt.Errorf("%w: %s", ErrWorklogProjectInactive, project.Status)
	}

	timezone := input.Timezone
	if timezone == "" {
		timezone = input.UserTimezone
	}
	if timezone == "" {
		timezone, err = LookupUserTimezone(s.db, userID)
		if err != nil {
			return preparedWorklog{}, err
		}
	}
	resolvedTimezone, location, err := ResolveTimezone(timezone)
	if err != nil {
		return preparedWorklog{}, worklogInputError{err}
	}
	date, err := ParseCivilDate(input.Date, location)
	if err != nil {
		return preparedWorklog{}, worklogInputError{err}
	}
	durationMinutes := input.DurationMinutes
	startUnix := input.StoredStartUnix
	endUnix := input.StoredEndUnix
	if input.PreserveStored {
		if _, err := ValidateWorklogDurationMinutes(durationMinutes); err != nil {
			return preparedWorklog{}, worklogInputError{err}
		}
		if startUnix <= 0 || endUnix <= startUnix {
			return preparedWorklog{}, worklogInputError{fmt.Errorf("stored worklog interval is invalid")}
		}
	} else {
		startTime := input.StartTime
		if startTime == "" && startUnix > 0 {
			startTime = time.Unix(startUnix, 0).In(location).Format("15:04")
		}
		endTime := input.EndTime
		if endTime == "" && endUnix > 0 {
			endTime = time.Unix(endUnix, 0).In(location).Format("15:04")
		}
		durationMinutes, startUnix, endUnix, err = ParseWorklogTimes(date, WorklogTimeInput{
			Duration: input.Duration, DurationMinutes: input.DurationMinutes,
			StartTime: startTime, EndTime: endTime,
		})
		if err != nil {
			return preparedWorklog{}, worklogInputError{err}
		}
	}
	description := input.Description
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &description, Policy: sanitize.Comment, Label: "Description"},
	)
	return preparedWorklog{
		customerID: *project.CustomerID, description: description, date: date,
		startUnix: startUnix, endUnix: endUnix, durationMinutes: durationMinutes,
		timezone: resolvedTimezone, location: location, warnings: warnings,
	}, nil
}

func (s *TimeWorklogService) result(worklogID int, prepared preparedWorklog) (*WorklogMutationResult, error) {
	worklog, err := s.worklogs.GetDetail(worklogID)
	if err != nil {
		return nil, err
	}
	return &WorklogMutationResult{
		Worklog: worklog, Warnings: prepared.warnings, Timezone: prepared.timezone,
		Location: prepared.location, StartTimeUnix: prepared.startUnix,
		EndTimeUnix: prepared.endUnix, DurationMinutes: prepared.durationMinutes,
	}, nil
}
