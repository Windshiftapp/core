package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type OnCallRepository struct {
	db database.Database
}

func NewOnCallRepository(db database.Database) *OnCallRepository {
	return &OnCallRepository{db: db}
}

// nullIntPtr converts a sql.NullInt64 to an *int, returning nil if not valid.
func nullIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// nullTimePtr converts a sql.NullTime to a *time.Time, returning nil if not valid.
func nullTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	return &n.Time
}

// incidentScanTargets holds nullable scan destinations for on-call incident queries.
// Use scanArgs to get the targets for rows.Scan, then call populate to fill the model.
type incidentScanTargets struct {
	itemID             sql.NullInt64
	acknowledgedAt     sql.NullTime
	acknowledgedBy     sql.NullInt64
	resolvedAt         sql.NullTime
	resolvedBy         sql.NullInt64
	policyName         sql.NullString
	itemTitle          sql.NullString
	acknowledgedByName sql.NullString
	resolvedByName     sql.NullString
}

// scanArgs returns scan destinations for the nullable incident columns.
// The withNames parameter controls whether acknowledged_by_name and resolved_by_name
// are included (used by GetIncidentByID but not GetActiveIncidents).
func (t *incidentScanTargets) scanArgs(inc *models.OnCallIncident, withNames bool) []any {
	args := []any{
		&inc.ID, &inc.EscalationPolicyID, &t.itemID, &inc.Status,
		&inc.TriggeredAt, &t.acknowledgedAt, &t.acknowledgedBy,
		&t.resolvedAt, &t.resolvedBy,
		&inc.CurrentEscalationStep, &inc.EscalationRepeatCount, &inc.CreatedAt,
		&t.policyName, &t.itemTitle,
	}
	if withNames {
		args = append(args, &t.acknowledgedByName, &t.resolvedByName)
	}
	return args
}

// populate assigns the scanned nullable values into the incident model fields.
func (t *incidentScanTargets) populate(inc *models.OnCallIncident) {
	inc.ItemID = nullIntPtr(t.itemID)
	inc.AcknowledgedAt = nullTimePtr(t.acknowledgedAt)
	inc.AcknowledgedBy = nullIntPtr(t.acknowledgedBy)
	inc.ResolvedAt = nullTimePtr(t.resolvedAt)
	inc.ResolvedBy = nullIntPtr(t.resolvedBy)
	inc.PolicyName = t.policyName.String
	inc.ItemTitle = t.itemTitle.String
	inc.AcknowledgedByName = t.acknowledgedByName.String
	inc.ResolvedByName = t.resolvedByName.String
}

// swapRequestScanTargets holds nullable scan destinations for swap request queries.
type swapRequestScanTargets struct {
	respondedAt   sql.NullTime
	requesterName sql.NullString
	targetName    sql.NullString
}

// scanArgs returns scan destinations for swap request rows.
func (t *swapRequestScanTargets) scanArgs(sr *models.OnCallSwapRequest) []any {
	return []any{
		&sr.ID, &sr.ScheduleID, &sr.RequesterUserID, &sr.TargetUserID,
		&sr.SwapStart, &sr.SwapEnd, &sr.Status, &t.respondedAt, &sr.CreatedAt,
		&t.requesterName, &t.targetName,
	}
}

// populate assigns the scanned nullable values into the swap request model fields.
func (t *swapRequestScanTargets) populate(sr *models.OnCallSwapRequest) {
	sr.RespondedAt = nullTimePtr(t.respondedAt)
	sr.RequesterName = t.requesterName.String
	sr.TargetName = t.targetName.String
}

// Schedule CRUD

func (r *OnCallRepository) GetScheduleByID(id int) (*models.OnCallSchedule, error) {
	var s models.OnCallSchedule
	var createdBy sql.NullInt64
	var createdByName sql.NullString
	var teamName sql.NullString

	err := r.db.QueryRow(`
		SELECT s.id, s.team_id, s.name, s.description, s.timezone, s.is_active,
			s.created_by, s.created_at, s.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			t.name as team_name
		FROM on_call_schedules s
		LEFT JOIN users u ON u.id = s.created_by
		LEFT JOIN teams t ON t.id = s.team_id
		WHERE s.id = ?
	`, id).Scan(
		&s.ID, &s.TeamID, &s.Name, &s.Description, &s.Timezone, &s.IsActive,
		&createdBy, &s.CreatedAt, &s.UpdatedAt,
		&createdByName, &teamName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.CreatedBy = nullIntPtr(createdBy)
	s.CreatedByName = createdByName.String
	s.TeamName = teamName.String

	// Load layers
	layers, err := r.GetLayersForSchedule(s.ID)
	if err != nil {
		return nil, err
	}
	s.Layers = layers

	return &s, nil
}

func (r *OnCallRepository) ListSchedulesForTeam(teamID int) ([]models.OnCallSchedule, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.team_id, s.name, s.description, s.timezone, s.is_active,
			s.created_by, s.created_at, s.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM on_call_schedules s
		LEFT JOIN users u ON u.id = s.created_by
		WHERE s.team_id = ?
		ORDER BY s.name
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.OnCallSchedule
	for rows.Next() {
		var s models.OnCallSchedule
		var createdBy sql.NullInt64
		var createdByName sql.NullString

		err := rows.Scan(
			&s.ID, &s.TeamID, &s.Name, &s.Description, &s.Timezone, &s.IsActive,
			&createdBy, &s.CreatedAt, &s.UpdatedAt,
			&createdByName,
		)
		if err != nil {
			return nil, err
		}

		s.CreatedBy = nullIntPtr(createdBy)
		s.CreatedByName = createdByName.String
		schedules = append(schedules, s)
	}

	return schedules, nil
}

func (r *OnCallRepository) CreateSchedule(teamID int, name, description, timezone string, createdBy int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_schedules (team_id, name, description, timezone, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?) RETURNING id
	`, teamID, name, description, timezone, createdBy, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) UpdateSchedule(id int, name, description, timezone string, isActive bool) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE on_call_schedules SET name = ?, description = ?, timezone = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, name, description, timezone, isActive, now, id)
	return err
}

func (r *OnCallRepository) DeleteSchedule(id int) error {
	_, err := r.db.Exec("DELETE FROM on_call_schedules WHERE id = ?", id)
	return err
}

// Layer Management

func (r *OnCallRepository) GetLayersForSchedule(scheduleID int) ([]models.OnCallScheduleLayer, error) {
	rows, err := r.db.Query(`
		SELECT id, schedule_id, name, priority, rotation_type, rotation_interval_days,
			handoff_time, start_date, end_date, created_at, updated_at
		FROM on_call_schedule_layers
		WHERE schedule_id = ?
		ORDER BY priority
	`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var layers []models.OnCallScheduleLayer
	for rows.Next() {
		var l models.OnCallScheduleLayer
		var endDate sql.NullString

		err := rows.Scan(
			&l.ID, &l.ScheduleID, &l.Name, &l.Priority, &l.RotationType,
			&l.RotationIntervalDays, &l.HandoffTime, &l.StartDate, &endDate,
			&l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if endDate.Valid {
			l.EndDate = &endDate.String
		}

		// Load members for this layer
		members, err := r.GetLayerMembers(l.ID)
		if err != nil {
			return nil, err
		}
		l.Members = members

		layers = append(layers, l)
	}

	return layers, nil
}

func (r *OnCallRepository) AddLayer(scheduleID int, name string, priority int, rotationType string, intervalDays int, handoffTime, startDate string, endDate *string) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_schedule_layers (schedule_id, name, priority, rotation_type,
			rotation_interval_days, handoff_time, start_date, end_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, scheduleID, name, priority, rotationType, intervalDays, handoffTime, startDate, endDate, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) UpdateLayer(id int, name string, priority int, rotationType string, intervalDays int, handoffTime, startDate string, endDate *string) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE on_call_schedule_layers
		SET name = ?, priority = ?, rotation_type = ?, rotation_interval_days = ?,
			handoff_time = ?, start_date = ?, end_date = ?, updated_at = ?
		WHERE id = ?
	`, name, priority, rotationType, intervalDays, handoffTime, startDate, endDate, now, id)
	return err
}

func (r *OnCallRepository) DeleteLayer(id int) error {
	_, err := r.db.Exec("DELETE FROM on_call_schedule_layers WHERE id = ?", id)
	return err
}

// Layer Members

func (r *OnCallRepository) GetLayerMembers(layerID int) ([]models.OnCallScheduleLayerMember, error) {
	rows, err := r.db.Query(`
		SELECT m.id, m.layer_id, m.user_id, m.position, m.created_at,
			u.first_name || ' ' || u.last_name as user_name,
			u.email, COALESCE(u.avatar_url, '') as avatar_url
		FROM on_call_schedule_layer_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.layer_id = ?
		ORDER BY m.position
	`, layerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.OnCallScheduleLayerMember
	for rows.Next() {
		var m models.OnCallScheduleLayerMember
		err := rows.Scan(&m.ID, &m.LayerID, &m.UserID, &m.Position, &m.CreatedAt,
			&m.UserName, &m.UserEmail, &m.UserAvatarURL)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}

	return members, nil
}

func (r *OnCallRepository) SetLayerMembers(layerID int, userIDs []int) error {
	// Delete existing members
	if _, err := r.db.Exec("DELETE FROM on_call_schedule_layer_members WHERE layer_id = ?", layerID); err != nil {
		return err
	}

	// Insert new members in order
	now := time.Now()
	for i, userID := range userIDs {
		_, err := r.db.Exec(`
			INSERT INTO on_call_schedule_layer_members (layer_id, user_id, position, created_at)
			VALUES (?, ?, ?, ?)
		`, layerID, userID, i+1, now)
		if err != nil {
			return fmt.Errorf("failed to add member %d to layer: %w", userID, err)
		}
	}

	return nil
}

// Overrides

func (r *OnCallRepository) CreateOverride(scheduleID, userID, overrideUserID int, startTime, endTime time.Time, reason string, createdBy int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_schedule_overrides (schedule_id, user_id, override_user_id,
			start_time, end_time, reason, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, scheduleID, userID, overrideUserID, startTime, endTime, reason, createdBy, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) DeleteOverride(id int) error {
	_, err := r.db.Exec("DELETE FROM on_call_schedule_overrides WHERE id = ?", id)
	return err
}

// GetOverrideByID returns the override row, used by the handler to walk
// override → schedule → team for a permission check before deletion.
func (r *OnCallRepository) GetOverrideByID(id int) (*models.OnCallScheduleOverride, error) {
	var o models.OnCallScheduleOverride
	var createdBy sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, schedule_id, user_id, override_user_id,
		       start_time, end_time, reason, created_by, created_at
		FROM on_call_schedule_overrides
		WHERE id = ?
	`, id).Scan(
		&o.ID, &o.ScheduleID, &o.UserID, &o.OverrideUserID,
		&o.StartTime, &o.EndTime, &o.Reason, &createdBy, &o.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	o.CreatedBy = nullIntPtr(createdBy)
	return &o, nil
}

func (r *OnCallRepository) GetActiveOverrides(scheduleID int) ([]models.OnCallScheduleOverride, error) {
	now := time.Now()
	rows, err := r.db.Query(`
		SELECT o.id, o.schedule_id, o.user_id, o.override_user_id,
			o.start_time, o.end_time, o.reason, o.created_by, o.created_at,
			u.first_name || ' ' || u.last_name as user_name,
			ou.first_name || ' ' || ou.last_name as override_user_name,
			cb.first_name || ' ' || cb.last_name as created_by_name
		FROM on_call_schedule_overrides o
		JOIN users u ON u.id = o.user_id
		JOIN users ou ON ou.id = o.override_user_id
		LEFT JOIN users cb ON cb.id = o.created_by
		WHERE o.schedule_id = ? AND o.end_time > ?
		ORDER BY o.start_time
	`, scheduleID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []models.OnCallScheduleOverride
	for rows.Next() {
		var o models.OnCallScheduleOverride
		var createdBy sql.NullInt64
		var createdByName sql.NullString

		err := rows.Scan(
			&o.ID, &o.ScheduleID, &o.UserID, &o.OverrideUserID,
			&o.StartTime, &o.EndTime, &o.Reason, &createdBy, &o.CreatedAt,
			&o.UserName, &o.OverrideUserName, &createdByName,
		)
		if err != nil {
			return nil, err
		}
		o.CreatedBy = nullIntPtr(createdBy)
		o.CreatedByName = createdByName.String
		overrides = append(overrides, o)
	}

	return overrides, nil
}

// Escalation Policy CRUD

func (r *OnCallRepository) GetPolicyByID(id int) (*models.OnCallEscalationPolicy, error) {
	var p models.OnCallEscalationPolicy
	var createdBy sql.NullInt64
	var createdByName sql.NullString
	var teamName sql.NullString

	err := r.db.QueryRow(`
		SELECT p.id, p.team_id, p.name, p.description, p.repeat_count, p.is_active,
			p.created_by, p.created_at, p.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			t.name as team_name
		FROM on_call_escalation_policies p
		LEFT JOIN users u ON u.id = p.created_by
		LEFT JOIN teams t ON t.id = p.team_id
		WHERE p.id = ?
	`, id).Scan(
		&p.ID, &p.TeamID, &p.Name, &p.Description, &p.RepeatCount, &p.IsActive,
		&createdBy, &p.CreatedAt, &p.UpdatedAt,
		&createdByName, &teamName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	p.CreatedBy = nullIntPtr(createdBy)
	p.CreatedByName = createdByName.String
	p.TeamName = teamName.String

	// Load rules
	rules, err := r.GetEscalationRules(p.ID)
	if err != nil {
		return nil, err
	}
	p.Rules = rules

	return &p, nil
}

func (r *OnCallRepository) ListPoliciesForTeam(teamID int) ([]models.OnCallEscalationPolicy, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.team_id, p.name, p.description, p.repeat_count, p.is_active,
			p.created_by, p.created_at, p.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM on_call_escalation_policies p
		LEFT JOIN users u ON u.id = p.created_by
		WHERE p.team_id = ?
		ORDER BY p.name
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []models.OnCallEscalationPolicy
	for rows.Next() {
		var p models.OnCallEscalationPolicy
		var createdBy sql.NullInt64
		var createdByName sql.NullString

		err := rows.Scan(
			&p.ID, &p.TeamID, &p.Name, &p.Description, &p.RepeatCount, &p.IsActive,
			&createdBy, &p.CreatedAt, &p.UpdatedAt,
			&createdByName,
		)
		if err != nil {
			return nil, err
		}
		p.CreatedBy = nullIntPtr(createdBy)
		p.CreatedByName = createdByName.String
		policies = append(policies, p)
	}

	return policies, nil
}

func (r *OnCallRepository) CreatePolicy(teamID int, name, description string, repeatCount, createdBy int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_escalation_policies (team_id, name, description, repeat_count, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?) RETURNING id
	`, teamID, name, description, repeatCount, createdBy, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) UpdatePolicy(id int, name, description string, repeatCount int, isActive bool) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE on_call_escalation_policies SET name = ?, description = ?, repeat_count = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, name, description, repeatCount, isActive, now, id)
	return err
}

func (r *OnCallRepository) DeletePolicy(id int) error {
	_, err := r.db.Exec("DELETE FROM on_call_escalation_policies WHERE id = ?", id)
	return err
}

// Escalation Rules

func (r *OnCallRepository) GetEscalationRules(policyID int) ([]models.OnCallEscalationRule, error) {
	rows, err := r.db.Query(`
		SELECT id, policy_id, step_order, escalation_delay_minutes, target_type, target_id, created_at
		FROM on_call_escalation_rules
		WHERE policy_id = ?
		ORDER BY step_order
	`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.OnCallEscalationRule
	for rows.Next() {
		var rule models.OnCallEscalationRule
		err := rows.Scan(&rule.ID, &rule.PolicyID, &rule.StepOrder,
			&rule.EscalationDelayMinutes, &rule.TargetType, &rule.TargetID, &rule.CreatedAt)
		if err != nil {
			return nil, err
		}

		// Load notification rules for this escalation rule
		notifRules, err := r.GetNotificationRulesForStep(rule.ID)
		if err != nil {
			return nil, err
		}
		rule.NotificationRules = notifRules

		rules = append(rules, rule)
	}

	return rules, nil
}

func (r *OnCallRepository) SetEscalationRules(policyID int, rules []models.EscalationRuleInput) error {
	// Delete existing rules (cascade deletes notification rules)
	if _, err := r.db.Exec("DELETE FROM on_call_escalation_rules WHERE policy_id = ?", policyID); err != nil {
		return err
	}

	now := time.Now()
	for _, rule := range rules {
		var ruleID int64
		err := r.db.QueryRow(`
			INSERT INTO on_call_escalation_rules (policy_id, step_order, escalation_delay_minutes, target_type, target_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, policyID, rule.StepOrder, rule.EscalationDelayMinutes, rule.TargetType, rule.TargetID, now).Scan(&ruleID)
		if err != nil {
			return fmt.Errorf("failed to create escalation rule: %w", err)
		}

		// Create notification rules for this step
		for _, nr := range rule.NotificationRules {
			_, err := r.db.Exec(`
				INSERT INTO on_call_notification_rules (escalation_rule_id, notification_type, delay_minutes, repeat_interval_minutes, repeat_count, created_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, ruleID, nr.NotificationType, nr.DelayMinutes, nr.RepeatIntervalMinutes, nr.RepeatCount, now)
			if err != nil {
				return fmt.Errorf("failed to create notification rule: %w", err)
			}
		}
	}

	return nil
}

func (r *OnCallRepository) GetNotificationRulesForStep(escalationRuleID int) ([]models.OnCallNotificationRule, error) {
	rows, err := r.db.Query(`
		SELECT id, escalation_rule_id, notification_type, delay_minutes,
			repeat_interval_minutes, repeat_count, created_at
		FROM on_call_notification_rules
		WHERE escalation_rule_id = ?
		ORDER BY delay_minutes
	`, escalationRuleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.OnCallNotificationRule
	for rows.Next() {
		var nr models.OnCallNotificationRule
		var repeatInterval sql.NullInt64

		err := rows.Scan(&nr.ID, &nr.EscalationRuleID, &nr.NotificationType,
			&nr.DelayMinutes, &repeatInterval, &nr.RepeatCount, &nr.CreatedAt)
		if err != nil {
			return nil, err
		}
		nr.RepeatIntervalMinutes = nullIntPtr(repeatInterval)
		rules = append(rules, nr)
	}

	return rules, nil
}

// Swap Requests

func (r *OnCallRepository) CreateSwapRequest(scheduleID, requesterUserID, targetUserID int, swapStart, swapEnd time.Time) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_swap_requests (schedule_id, requester_user_id, target_user_id,
			swap_start, swap_end, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?) RETURNING id
	`, scheduleID, requesterUserID, targetUserID, swapStart, swapEnd, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) GetSwapRequestByID(id int) (*models.OnCallSwapRequest, error) {
	var sr models.OnCallSwapRequest
	var t swapRequestScanTargets

	err := r.db.QueryRow(`
		SELECT sr.id, sr.schedule_id, sr.requester_user_id, sr.target_user_id,
			sr.swap_start, sr.swap_end, sr.status, sr.responded_at, sr.created_at,
			req.first_name || ' ' || req.last_name as requester_name,
			tgt.first_name || ' ' || tgt.last_name as target_name
		FROM on_call_swap_requests sr
		JOIN users req ON req.id = sr.requester_user_id
		JOIN users tgt ON tgt.id = sr.target_user_id
		WHERE sr.id = ?
	`, id).Scan(t.scanArgs(&sr)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.populate(&sr)
	return &sr, nil
}

func (r *OnCallRepository) UpdateSwapRequestStatus(id int, status string) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE on_call_swap_requests SET status = ?, responded_at = ? WHERE id = ?
	`, status, now, id)
	return err
}

// Incidents

func (r *OnCallRepository) GetIncidentByID(id int) (*models.OnCallIncident, error) {
	var inc models.OnCallIncident
	var t incidentScanTargets

	err := r.db.QueryRow(`
		SELECT i.id, i.escalation_policy_id, i.item_id, i.status,
			i.triggered_at, i.acknowledged_at, i.acknowledged_by,
			i.resolved_at, i.resolved_by,
			i.current_escalation_step, i.escalation_repeat_count, i.created_at,
			p.name as policy_name,
			it.title as item_title,
			ack.first_name || ' ' || ack.last_name as acknowledged_by_name,
			res.first_name || ' ' || res.last_name as resolved_by_name
		FROM on_call_incidents i
		LEFT JOIN on_call_escalation_policies p ON p.id = i.escalation_policy_id
		LEFT JOIN items it ON it.id = i.item_id
		LEFT JOIN users ack ON ack.id = i.acknowledged_by
		LEFT JOIN users res ON res.id = i.resolved_by
		WHERE i.id = ?
	`, id).Scan(t.scanArgs(&inc, true)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.populate(&inc)
	return &inc, nil
}

func (r *OnCallRepository) UpdateIncident(id int, status string, acknowledgedAt *time.Time, acknowledgedBy *int, resolvedAt *time.Time, resolvedBy *int, step, repeatCount int) error {
	_, err := r.db.Exec(`
		UPDATE on_call_incidents
		SET status = ?, acknowledged_at = ?, acknowledged_by = ?,
			resolved_at = ?, resolved_by = ?,
			current_escalation_step = ?, escalation_repeat_count = ?
		WHERE id = ?
	`, status, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, step, repeatCount, id)
	return err
}

func (r *OnCallRepository) GetActiveIncidents(policyID *int, status string) ([]models.OnCallIncident, error) {
	query := `
		SELECT i.id, i.escalation_policy_id, i.item_id, i.status,
			i.triggered_at, i.acknowledged_at, i.acknowledged_by,
			i.resolved_at, i.resolved_by,
			i.current_escalation_step, i.escalation_repeat_count, i.created_at,
			p.name as policy_name,
			it.title as item_title
		FROM on_call_incidents i
		LEFT JOIN on_call_escalation_policies p ON p.id = i.escalation_policy_id
		LEFT JOIN items it ON it.id = i.item_id
		WHERE 1=1
	`
	args := []interface{}{}

	if policyID != nil {
		query += " AND i.escalation_policy_id = ?"
		args = append(args, *policyID)
	}
	if status != "" {
		query += " AND i.status = ?"
		args = append(args, status)
	}

	query += " ORDER BY i.triggered_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []models.OnCallIncident
	for rows.Next() {
		var inc models.OnCallIncident
		var t incidentScanTargets

		err := rows.Scan(t.scanArgs(&inc, false)...)
		if err != nil {
			return nil, err
		}

		t.populate(&inc)
		incidents = append(incidents, inc)
	}

	return incidents, nil
}
