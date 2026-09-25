package services

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// IncidentService owns the paged-incident lifecycle attached to work items.
// An incident is pager state on an item, not a separate work entity.
type IncidentService struct {
	db            database.Database
	incidents     *repository.OnCallRepository
	items         *repository.ItemRepository
	onCall        *OnCallService
	teams         *repository.TeamRepository
	notifications *NotificationService
	webhooks      WebhookDispatcher
}

// SetWebhookDispatcher wires the workspace webhook dispatcher used by
// escalation steps configured with the webhook notification type.
func (s *IncidentService) SetWebhookDispatcher(dispatcher WebhookDispatcher) {
	s.webhooks = dispatcher
}

// NewIncidentService wires the incident lifecycle to the on-call repository.
// The escalation dependencies (onCall, teams, notifications) may be nil in
// unit tests that only exercise the state machine; escalation is then skipped.
func NewIncidentService(
	db database.Database,
	incidents *repository.OnCallRepository,
	items *repository.ItemRepository,
	onCall *OnCallService,
	teams *repository.TeamRepository,
	notifications *NotificationService,
) *IncidentService {
	return &IncidentService{
		db:            db,
		incidents:     incidents,
		items:         items,
		onCall:        onCall,
		teams:         teams,
		notifications: notifications,
	}
}

var (
	// ErrIncidentItemTeamRequired is returned when an item has no assigned team.
	ErrIncidentItemTeamRequired = errors.New("item must have an assigned team to declare an incident")
	// ErrIncidentPolicyUnknown is returned when no usable escalation policy exists.
	ErrIncidentPolicyUnknown = errors.New("no escalation policy is available for the item's team")
	// ErrIncidentAlreadyOpen is returned when the item already has an open incident.
	ErrIncidentAlreadyOpen = errors.New("item already has an open incident")
	// ErrIncidentResolved is returned when mutating a resolved incident.
	ErrIncidentResolved = errors.New("incident is resolved")
)

// Trigger creates a triggered incident for the item and points the item at it.
// policyID overrides the item team's active escalation policy when supplied.
func (s *IncidentService) Trigger(itemID int, policyID *int, urgency, source string) (*models.Incident, error) {
	item, err := s.items.FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}

	policy, err := s.resolvePolicy(item, policyID)
	if err != nil {
		return nil, err
	}

	existing, err := s.incidents.GetIncidentForItem(itemID)
	switch {
	case err == nil && existing.Status != "resolved":
		return nil, ErrIncidentAlreadyOpen
	case err != nil && !errors.Is(err, repository.ErrNotFound):
		return nil, err
	}

	if urgency == "" {
		urgency = "high"
	}
	if source == "" {
		source = "manual"
	}

	incidentID, err := s.incidents.CreateIncident(itemID, &policy.ID, urgency, source)
	if err != nil {
		return nil, err
	}

	// Step 0 pages inline; the first notification must not wait for a tick.
	if err := s.processTriggered(incidentID); err != nil {
		slog.Warn("incident escalation: initial step failed",
			slog.String("component", "oncall"), slog.Int("incident_id", incidentID), slog.Any("error", err))
	}
	return s.incidents.GetIncidentByID(incidentID)
}

// processTriggered notifies the first escalation step and arms the deadline for
// the next one. A policy with no rules leaves the incident without a deadline.
func (s *IncidentService) processTriggered(incidentID int) error {
	incident, err := s.incidents.GetIncidentByID(incidentID)
	if err != nil {
		return err
	}
	if incident.EscalationPolicyID == nil {
		return nil
	}
	rules, err := s.incidents.GetEscalationRules(*incident.EscalationPolicyID)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	if err := s.enterStep(incident, rules[0]); err != nil {
		slog.Warn("incident escalation: notify step 0 failed",
			slog.String("component", "oncall"), slog.Int("incident_id", incidentID), slog.Any("error", err))
	}
	next := time.Now().Add(time.Duration(rules[0].EscalationDelayMinutes) * time.Minute)
	return s.incidents.SetIncidentEscalation(incidentID, 0, 0, &next)
}

// AdvanceDue moves an incident to its next escalation step when its deadline
// has passed. It is a no-op when the incident was acked/resolved in the
// meantime, or when the chain is exhausted (the deadline is then cleared).
func (s *IncidentService) AdvanceDue(incidentID int) error {
	incident, err := s.incidents.GetIncidentByID(incidentID)
	if err != nil {
		return err
	}
	if incident.Status != "triggered" || incident.NextEscalationAt == nil || incident.NextEscalationAt.After(time.Now()) {
		return nil
	}
	if incident.EscalationPolicyID == nil {
		return s.incidents.SetIncidentEscalation(incidentID, incident.EscalationStep, incident.EscalationRepeatCount, nil)
	}
	rules, err := s.incidents.GetEscalationRules(*incident.EscalationPolicyID)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return s.incidents.SetIncidentEscalation(incidentID, incident.EscalationStep, incident.EscalationRepeatCount, nil)
	}

	policy, err := s.incidents.GetPolicyByID(*incident.EscalationPolicyID)
	if err != nil {
		return err
	}

	step, repeat := incident.EscalationStep, incident.EscalationRepeatCount
	nextIndex := step + 1
	if nextIndex >= len(rules) {
		// RepeatCount is the maximum number of full passes over the chain.
		if repeat+1 >= policy.RepeatCount {
			return s.incidents.SetIncidentEscalation(incidentID, step, repeat, nil)
		}
		repeat++
		nextIndex = 0
	}

	rule := rules[nextIndex]
	if err := s.enterStep(incident, rule); err != nil {
		slog.Warn("incident escalation: notify step failed",
			slog.String("component", "oncall"), slog.Int("incident_id", incidentID), slog.Any("error", err))
	}
	next := time.Now().Add(time.Duration(rule.EscalationDelayMinutes) * time.Minute)
	return s.incidents.SetIncidentEscalation(incidentID, nextIndex, repeat, &next)
}

// enterStep clears the previous step's pending notifications, then delivers or
// schedules the current step's notification rules. Recipients are chosen by the
// policy, so they are intentionally not filtered by item workspace permission:
// the responder may have no rights in the item's workspace.
func (s *IncidentService) enterStep(incident *models.Incident, rule models.OnCallEscalationRule) error {
	// Notifications scheduled for a lower step are stale once we escalate.
	if err := s.incidents.DeleteIncidentNotificationStates(incident.ID); err != nil {
		return err
	}

	userIDs, err := s.resolveTargets(rule)
	if err != nil {
		return err
	}

	if len(rule.NotificationRules) == 0 {
		// No explicit rules: page in-app once so the step is never silent.
		return s.dispatchNotification(incident, rule, models.OnCallNotificationRule{NotificationType: "in_app", RepeatCount: 1}, userIDs)
	}

	now := time.Now()
	for _, nr := range rule.NotificationRules {
		if err := s.scheduleNotification(incident, rule, nr, userIDs, now); err != nil {
			slog.Warn("incident escalation: schedule notification failed",
				slog.String("component", "oncall"), slog.Int("incident_id", incident.ID), slog.Any("error", err))
		}
	}
	return nil
}

// scheduleNotification dispatches a zero-delay first send inline and records a
// state row for every delayed or repeated send. Repeats without an interval are
// collapsed to a single send so a step can never loop on itself.
func (s *IncidentService) scheduleNotification(incident *models.Incident, rule models.OnCallEscalationRule, nr models.OnCallNotificationRule, userIDs []int, now time.Time) error {
	sends := nr.RepeatCount
	if sends < 1 {
		sends = 1
	}
	interval := 0
	if nr.RepeatIntervalMinutes != nil {
		interval = *nr.RepeatIntervalMinutes
	}
	if sends > 1 && interval <= 0 {
		sends = 1
	}

	// A zero-delay first send goes out inline; any repeats are armed as one
	// state row that advances in place. A delayed first send is armed and left
	// for the sweeper.
	if nr.DelayMinutes == 0 {
		if err := s.dispatchNotification(incident, rule, nr, userIDs); err != nil {
			slog.Warn("incident escalation: notify failed",
				slog.String("component", "oncall"), slog.Int("incident_id", incident.ID), slog.Any("error", err))
		}
		if sends > 1 {
			nextAt := now.Add(time.Duration(interval) * time.Minute)
			return s.incidents.CreateIncidentNotificationState(incident.ID, rule.ID, nr.ID, 1, nextAt)
		}
		return nil
	}
	sendAt := now.Add(time.Duration(nr.DelayMinutes) * time.Minute)
	return s.incidents.CreateIncidentNotificationState(incident.ID, rule.ID, nr.ID, 0, sendAt)
}

// dispatchNotification delivers one notification-rule send.
func (s *IncidentService) dispatchNotification(incident *models.Incident, rule models.OnCallEscalationRule, nr models.OnCallNotificationRule, userIDs []int) error {
	if nr.NotificationType == "webhook" {
		return s.dispatchIncidentWebhook(incident.ItemID)
	}
	if s.notifications == nil {
		return nil
	}
	if len(userIDs) == 0 {
		slog.Warn("incident escalation: step resolved to no users",
			slog.String("component", "oncall"), slog.Int("incident_id", incident.ID),
			slog.String("target_type", rule.TargetType), slog.Int("target_id", rule.TargetID))
		return nil
	}
	title := fmt.Sprintf("Incident: %s", incident.ItemTitle)
	message := fmt.Sprintf("%s is triggered and needs an acknowledgement", incident.ItemKey)
	// System scope so the page is visible to responders who have no workspace
	// role; recipients are resolved from the policy, not workspace membership.
	// System scope forbids workspace provenance, so the item link travels in
	// the action URL only.
	_, err := s.notifications.notifyUsersAtURL(
		userIDs, 0, "incident", title, message,
		itemActionURL(incident.WorkspaceID, incident.ItemID),
		models.NotificationScopeSystem, nil, nil,
		"incident", &incident.ItemID, nil,
	)
	return err
}

// dispatchIncidentWebhook emits an incident event through the workspace's
// configured outbound webhooks.
func (s *IncidentService) dispatchIncidentWebhook(itemID int) error {
	if s.webhooks == nil {
		return nil
	}
	item, err := s.items.FindByIDWithDetails(itemID)
	if err != nil {
		return err
	}
	s.webhooks.DispatchEvent("incident.triggered", item)
	return nil
}

// DispatchDueNotification delivers a scheduled notification whose deadline has
// passed, then arms the next repeat or clears the state row.
func (s *IncidentService) DispatchDueNotification(stateID int) error {
	state, err := s.incidents.GetNotificationState(stateID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	incident, err := s.incidents.GetIncidentByID(state.IncidentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return s.incidents.DeleteNotificationState(stateID)
		}
		return err
	}
	if incident.Status != "triggered" {
		return s.incidents.DeleteNotificationState(stateID)
	}

	nr, err := s.incidents.GetNotificationRuleByID(state.NotificationRuleID)
	if err != nil {
		return s.incidents.DeleteNotificationState(stateID)
	}
	rule, err := s.incidents.GetEscalationRuleByID(state.EscalationRuleID)
	if err != nil {
		return s.incidents.DeleteNotificationState(stateID)
	}

	userIDs, err := s.resolveTargets(*rule)
	if err != nil {
		return err
	}
	if err := s.dispatchNotification(incident, *rule, *nr, userIDs); err != nil {
		slog.Warn("incident escalation: scheduled notify failed",
			slog.String("component", "oncall"), slog.Int("incident_id", incident.ID), slog.Any("error", err))
	}

	sends := nr.RepeatCount
	if sends < 1 {
		sends = 1
	}
	interval := 0
	if nr.RepeatIntervalMinutes != nil {
		interval = *nr.RepeatIntervalMinutes
	}
	nextIndex := state.RepeatIndex + 1
	if nextIndex < sends && interval > 0 {
		nextAt := time.Now().Add(time.Duration(interval) * time.Minute)
		return s.incidents.AdvanceNotificationState(stateID, nextIndex, &nextAt)
	}
	return s.incidents.DeleteNotificationState(stateID)
}

// resolveTargets maps an escalation step target to the users to page.
func (s *IncidentService) resolveTargets(rule models.OnCallEscalationRule) ([]int, error) {
	switch rule.TargetType {
	case "user":
		return []int{rule.TargetID}, nil
	case "on_call_schedule":
		if s.onCall == nil {
			return nil, nil
		}
		current, err := s.onCall.GetCurrentOnCall(rule.TargetID)
		if err != nil {
			return nil, err
		}
		ids := make([]int, 0, len(current.OnCall))
		for _, entry := range current.OnCall {
			ids = append(ids, entry.UserID)
		}
		return ids, nil
	case "team":
		if s.teams == nil {
			return nil, nil
		}
		members, err := s.teams.GetResolvedMembers(rule.TargetID)
		if err != nil {
			return nil, err
		}
		ids := make([]int, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.UserID)
		}
		return ids, nil
	default:
		return nil, nil
	}
}

// resolvePolicy enforces that the policy belongs to the item's team, and
// falls back to the team's active policy when none is named.
func (s *IncidentService) resolvePolicy(item *models.Item, policyID *int) (*models.OnCallEscalationPolicy, error) {
	if item.TeamID == nil {
		return nil, ErrIncidentItemTeamRequired
	}
	if policyID != nil {
		policy, err := s.incidents.GetPolicyByID(*policyID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrIncidentPolicyUnknown
			}
			return nil, err
		}
		if policy.TeamID != *item.TeamID {
			return nil, fmt.Errorf("%w: policy belongs to another team", ErrIncidentPolicyUnknown)
		}
		return policy, nil
	}

	policy, err := s.incidents.GetActivePolicyForTeam(*item.TeamID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrIncidentPolicyUnknown
		}
		return nil, err
	}
	return policy, nil
}

// Acknowledge records the ack handshake and stops escalation.
func (s *IncidentService) Acknowledge(incidentID, userID int) error {
	incident, err := s.incidents.GetIncidentByID(incidentID)
	if err != nil {
		return err
	}
	if incident.Status == "resolved" {
		return ErrIncidentResolved
	}
	if err := s.incidents.AcknowledgeIncident(incidentID, userID, time.Now()); err != nil {
		return err
	}
	// Ack stops the page: drop any scheduled repeats.
	return s.incidents.DeleteIncidentNotificationStates(incidentID)
}

// Unacknowledge returns an acknowledged incident to the triggered state.
func (s *IncidentService) Unacknowledge(incidentID int) error {
	incident, err := s.incidents.GetIncidentByID(incidentID)
	if err != nil {
		return err
	}
	if incident.Status == "resolved" {
		return ErrIncidentResolved
	}
	return s.incidents.UnacknowledgeIncident(incidentID, time.Now())
}

// Resolve ends the incident. It does not change item status, and resolving an
// already-resolved incident is a no-op.
func (s *IncidentService) Resolve(incidentID, userID int) error {
	incident, err := s.incidents.GetIncidentByID(incidentID)
	if err != nil {
		return err
	}
	if incident.Status == "resolved" {
		return nil
	}
	if err := s.incidents.ResolveIncident(incidentID, userID, time.Now()); err != nil {
		return err
	}
	return s.incidents.DeleteIncidentNotificationStates(incidentID)
}
