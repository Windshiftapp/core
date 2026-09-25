package services

import (
	"fmt"
	"sort"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type OnCallService struct {
	db         database.Database
	onCallRepo *repository.OnCallRepository
	leaveRepo  *repository.LeaveRepository
}

func NewOnCallService(db database.Database, onCallRepo *repository.OnCallRepository, leaveRepo *repository.LeaveRepository) *OnCallService {
	return &OnCallService{
		db:         db,
		onCallRepo: onCallRepo,
		leaveRepo:  leaveRepo,
	}
}

func (s *OnCallService) computeRotationForLayer(layer *models.OnCallScheduleLayer, instant time.Time, location *time.Location) *int {
	startDate, err := parseOnCallDate(layer.StartDate)
	if err != nil {
		return nil
	}
	local := instant.In(location)
	currentDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	if currentDate.Before(startDate) {
		return nil
	}

	if layer.EndDate != nil {
		endDate, err := parseOnCallDate(*layer.EndDate)
		if err != nil {
			return nil
		}
		endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.UTC)
		if currentDate.After(endDate) {
			return nil
		}
	}

	handoff, err := onCallHandoffBoundary(currentDate, layer.HandoffTime, location)
	if err != nil {
		return nil
	}

	members := make([]models.OnCallScheduleLayerMember, len(layer.Members))
	copy(members, layer.Members)
	if len(members) == 0 {
		return nil
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Position < members[j].Position
	})

	// Calculate the number of full days since the start date.
	daysSinceStart := int(currentDate.Sub(startDate) / (24 * time.Hour))

	// If we have not yet reached the handoff time today, the previous rotation
	// slot is still active, so we shift back by one period.
	beforeHandoff := instant.Before(handoff)

	var rotationIndex int
	switch layer.RotationType {
	case "daily":
		rotationIndex = daysSinceStart
		if beforeHandoff {
			rotationIndex--
		}
	case "weekly":
		rotationIndex = daysSinceStart / 7
		// For weekly rotation, shift back only if we are on the handoff day
		// boundary (first day of the new week) and before the handoff time.
		if daysSinceStart%7 == 0 && beforeHandoff {
			rotationIndex--
		}
	case "custom":
		interval := layer.RotationIntervalDays
		if interval <= 0 {
			interval = 1
		}
		rotationIndex = daysSinceStart / interval
		if daysSinceStart%interval == 0 && beforeHandoff {
			rotationIndex--
		}
	default:
		return nil
	}

	// Ensure a non-negative index before taking modulo.
	memberCount := len(members)
	rotationIndex = ((rotationIndex % memberCount) + memberCount) % memberCount

	userID := members[rotationIndex].UserID
	return &userID
}

func parseOnCallDate(value string) (time.Time, error) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

// GetCurrentOnCall resolves who is currently on call for the given schedule,
// taking overrides and layer priorities into account.
func (s *OnCallService) GetCurrentOnCall(scheduleID int) (*models.CurrentOnCallResponse, error) {
	schedule, err := s.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	return s.CurrentOnCallForSchedule(schedule, time.Now()), nil
}

// CurrentOnCallForSchedule resolves a fully hydrated schedule without database
// reads. It is shared by the single-schedule endpoint and the team overview.
func (s *OnCallService) CurrentOnCallForSchedule(schedule *models.OnCallSchedule, now time.Time) *models.CurrentOnCallResponse {
	resp := &models.CurrentOnCallResponse{
		ScheduleID: schedule.ID,
		OnCall:     []models.OnCallUserEntry{},
	}

	_, location, err := ResolveTimezone(schedule.Timezone)
	if err != nil {
		// Existing invalid rows predate request validation. UTC preserves the old
		// behavior without making every rotation disappear.
		location = time.UTC
	}

	// Check overrides first. An override replaces the original user with the
	// override user for the duration of the override window.
	replacedUserIDs := make(map[int]bool)
	overrideUserIDs := make(map[int]bool)
	for _, o := range schedule.Overrides {
		if now.After(o.StartTime) && now.Before(o.EndTime) {
			replacedUserIDs[o.UserID] = true
			if !overrideUserIDs[o.OverrideUserID] {
				resp.OnCall = append(resp.OnCall, models.OnCallUserEntry{
					UserID:     o.OverrideUserID,
					UserName:   o.OverrideUserName,
					IsOverride: true,
				})
				overrideUserIDs[o.OverrideUserID] = true
			}
		}
	}

	// Process layers by priority (lowest priority number = highest importance).
	layers := make([]models.OnCallScheduleLayer, len(schedule.Layers))
	copy(layers, schedule.Layers)
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority < layers[j].Priority
	})

	for _, layer := range layers {
		userID := s.computeRotationForLayer(&layer, now, location)
		if userID == nil {
			continue
		}
		if replacedUserIDs[*userID] || overrideUserIDs[*userID] {
			continue
		}
		var userName, userEmail string
		for _, member := range layer.Members {
			if member.UserID == *userID {
				userName = member.UserName
				userEmail = member.UserEmail
				break
			}
		}
		resp.OnCall = append(resp.OnCall, models.OnCallUserEntry{
			UserID:    *userID,
			UserName:  userName,
			UserEmail: userEmail,
			LayerName: layer.Name,
		})
	}

	return resp
}

// CreateSwapOverride converts an approved swap request into a schedule override,
// replacing the requester with the target user for the swap window.
func (s *OnCallService) CreateSwapOverride(swapRequestID int) error {
	swap, err := s.onCallRepo.GetSwapRequestByID(swapRequestID)
	if err != nil {
		return fmt.Errorf("failed to get swap request: %w", err)
	}

	if swap.Status != "approved" {
		return fmt.Errorf("swap request is not approved (status: %s)", swap.Status)
	}

	_, err = s.onCallRepo.CreateOverride(
		swap.ScheduleID,
		swap.RequesterUserID,
		swap.TargetUserID,
		swap.SwapStart,
		swap.SwapEnd,
		fmt.Sprintf("Swap request #%d", swap.ID),
		swap.TargetUserID,
	)
	if err != nil {
		return fmt.Errorf("failed to create override from swap: %w", err)
	}

	return nil
}
