package services

import (
	"errors"
	"fmt"
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
	durationMinutes, startUnix, endUnix, err := ParseWorklogTimes(date, WorklogTimeInput{
		Duration: input.Duration, DurationMinutes: input.DurationMinutes,
		StartTime: input.StartTime, EndTime: input.EndTime,
	})
	if err != nil {
		return preparedWorklog{}, worklogInputError{err}
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
