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

// ComputeRotationForLayer determines which member is on call for the given layer
// at the specified time. Returns nil if no member is on call (e.g. outside the
// layer's active window or no members configured).
func (s *OnCallService) ComputeRotationForLayer(layer *models.OnCallScheduleLayer, t time.Time) *int {
	startDate, err := time.Parse("2006-01-02", layer.StartDate)
	if err != nil {
		return nil
	}
	if t.Before(startDate) {
		return nil
	}

	if layer.EndDate != nil {
		endDate, err := time.Parse("2006-01-02", *layer.EndDate)
		if err != nil {
			return nil
		}
		// End date is inclusive, so the layer is active through the end of that day.
		endOfDay := endDate.Add(24 * time.Hour)
		if t.After(endOfDay) || t.Equal(endOfDay) {
			return nil
		}
	}

	handoff, err := time.Parse("15:04", layer.HandoffTime)
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
	daysSinceStart := int(t.Sub(startDate).Hours() / 24)

	// If we have not yet reached the handoff time today, the previous rotation
	// slot is still active, so we shift back by one period.
	handoffHour, handoffMin := handoff.Hour(), handoff.Minute()
	currentHour, currentMin := t.Hour(), t.Minute()
	beforeHandoff := currentHour < handoffHour || (currentHour == handoffHour && currentMin < handoffMin)

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

// GetCurrentOnCall resolves who is currently on call for the given schedule,
// taking overrides and layer priorities into account.
func (s *OnCallService) GetCurrentOnCall(scheduleID int) (*models.CurrentOnCallResponse, error) {
	schedule, err := s.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	overrides, err := s.onCallRepo.GetActiveOverrides(scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get overrides: %w", err)
	}

	now := time.Now()

	resp := &models.CurrentOnCallResponse{
		ScheduleID: scheduleID,
		OnCall:     []models.OnCallUserEntry{},
	}

	// Check overrides first. An override replaces the original user with the
	// override user for the duration of the override window.
	for _, o := range overrides {
		if now.After(o.StartTime) && now.Before(o.EndTime) {
			resp.OnCall = append(resp.OnCall, models.OnCallUserEntry{
				UserID:     o.OverrideUserID,
				UserName:   o.OverrideUserName,
				IsOverride: true,
			})
		}
	}

	// Build a set of user IDs already covered by overrides so we don't
	// duplicate entries from layer resolution.
	overrideUserIDs := make(map[int]bool)
	for _, entry := range resp.OnCall {
		overrideUserIDs[entry.UserID] = true
	}

	// Process layers by priority (lowest priority number = highest importance).
	layers := make([]models.OnCallScheduleLayer, len(schedule.Layers))
	copy(layers, schedule.Layers)
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority < layers[j].Priority
	})

	for _, layer := range layers {
		userID := s.ComputeRotationForLayer(&layer, now)
		if userID == nil {
			continue
		}
		if overrideUserIDs[*userID] {
			continue
		}
		resp.OnCall = append(resp.OnCall, models.OnCallUserEntry{
			UserID:    *userID,
			LayerName: layer.Name,
		})
	}

	return resp, nil
}

// AcknowledgeIncident marks an incident as acknowledged by the given user.
func (s *OnCallService) AcknowledgeIncident(incidentID, userID int) error {
	incident, err := s.onCallRepo.GetIncidentByID(incidentID)
	if err != nil {
		return fmt.Errorf("failed to get incident: %w", err)
	}

	now := time.Now()
	err = s.onCallRepo.UpdateIncident(
		incident.ID,
		"acknowledged",
		&now,
		&userID,
		incident.ResolvedAt,
		incident.ResolvedBy,
		incident.CurrentEscalationStep,
		incident.EscalationRepeatCount,
	)
	if err != nil {
		return fmt.Errorf("failed to acknowledge incident: %w", err)
	}

	return nil
}

// ResolveIncident marks an incident as resolved by the given user.
func (s *OnCallService) ResolveIncident(incidentID, userID int) error {
	incident, err := s.onCallRepo.GetIncidentByID(incidentID)
	if err != nil {
		return fmt.Errorf("failed to get incident: %w", err)
	}

	now := time.Now()
	err = s.onCallRepo.UpdateIncident(
		incident.ID,
		"resolved",
		incident.AcknowledgedAt,
		incident.AcknowledgedBy,
		&now,
		&userID,
		incident.CurrentEscalationStep,
		incident.EscalationRepeatCount,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve incident: %w", err)
	}

	return nil
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
