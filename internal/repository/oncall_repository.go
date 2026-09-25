package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type OnCallRepository struct {
	db database.Database
}

// IncidentFilter limits incidents to teams and item workspaces visible to
// the caller. AllTeams bypasses only the team filter. ItemID narrows to the
// incidents of one work item.
type IncidentFilter struct {
	PolicyID     *int
	Status       string
	ItemID       *int
	TeamIDs      []int
	WorkspaceIDs []int
	AllTeams     bool
}

func NewOnCallRepository(db database.Database) *OnCallRepository {
	return &OnCallRepository{db: db}
}

// nullTimePtr converts a sql.NullTime to a *time.Time, returning nil if not valid.
func nullTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	return &n.Time
}

// incidentSelectBody is the shared projection for incident reads. It joins the
// owning item so callers get title/key/workspace without a second query.
const incidentSelectBody = `
	SELECT inc.id, inc.item_id, inc.status, inc.urgency, inc.source,
	       inc.escalation_policy_id, inc.triggered_at, inc.acknowledged_at, inc.acknowledged_by,
	       inc.resolved_at, inc.resolved_by, inc.escalation_step, inc.escalation_repeat_count,
	       inc.next_escalation_at, COALESCE(inc.dedup_key, ''), inc.created_at, inc.updated_at,
	       COALESCE(p.name, '') AS policy_name,
	       i.title AS item_title,
	       w.key || '-' || i.workspace_item_number AS item_key,
	       i.workspace_id, i.team_id, COALESCE(t.name, '') AS team_name,
	       COALESCE(ack.first_name || ' ' || ack.last_name, '') AS acknowledged_by_name,
	       COALESCE(res.first_name || ' ' || res.last_name, '') AS resolved_by_name
	FROM incidents inc
	JOIN items i ON i.id = inc.item_id
	JOIN workspaces w ON w.id = i.workspace_id
	LEFT JOIN teams t ON t.id = i.team_id
	LEFT JOIN on_call_escalation_policies p ON p.id = inc.escalation_policy_id
	LEFT JOIN users ack ON ack.id = inc.acknowledged_by
	LEFT JOIN users res ON res.id = inc.resolved_by`

// scanIncident scans one row of incidentSelectBody.
func scanIncident(scanner rowScanner) (models.Incident, error) {
	var inc models.Incident
	var escalationPolicyID, acknowledgedBy, resolvedBy, teamID sql.NullInt64
	var acknowledgedAt, resolvedAt, nextEscalationAt sql.NullTime
	var acknowledgedByName, resolvedByName sql.NullString
	err := scanner.Scan(
		&inc.ID, &inc.ItemID, &inc.Status, &inc.Urgency, &inc.Source,
		&escalationPolicyID, &inc.TriggeredAt, &acknowledgedAt, &acknowledgedBy,
		&resolvedAt, &resolvedBy, &inc.EscalationStep, &inc.EscalationRepeatCount,
		&nextEscalationAt, &inc.DedupKey, &inc.CreatedAt, &inc.UpdatedAt,
		&inc.PolicyName, &inc.ItemTitle, &inc.ItemKey,
		&inc.WorkspaceID, &teamID, &inc.TeamName,
		&acknowledgedByName, &resolvedByName,
	)
	if err != nil {
		return models.Incident{}, err
	}
	inc.EscalationPolicyID = nullIntPtr(escalationPolicyID)
	inc.AcknowledgedAt = nullTimePtr(acknowledgedAt)
	inc.AcknowledgedBy = nullIntPtr(acknowledgedBy)
	inc.ResolvedAt = nullTimePtr(resolvedAt)
	inc.ResolvedBy = nullIntPtr(resolvedBy)
	inc.NextEscalationAt = nullTimePtr(nextEscalationAt)
	inc.TeamID = nullIntPtr(teamID)
	inc.AcknowledgedByName = acknowledgedByName.String
	inc.ResolvedByName = resolvedByName.String
	return inc, nil
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

	// Load current + upcoming overrides (anything not yet ended) so the
	// schedule detail view can render them. Past overrides are omitted.
	overrides, err := r.GetActiveOverrides(s.ID)
	if err != nil {
		return nil, err
	}
	s.Overrides = overrides

	return &s, nil
}

func (r *OnCallRepository) ListSchedulesForTeam(teamID int, includeRoster bool) ([]models.OnCallSchedule, error) {
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
			_ = rows.Close()
			return nil, err
		}

		s.CreatedBy = nullIntPtr(createdBy)
		s.CreatedByName = createdByName.String
		s.Layers = []models.OnCallScheduleLayer{}
		s.Overrides = []models.OnCallScheduleOverride{}
		schedules = append(schedules, s)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(schedules) == 0 {
		return schedules, nil
	}

	return r.hydrateSchedulesForTeam(teamID, schedules, includeRoster)
}

// hydrateSchedulesForTeam fills every schedule's layers. Authorized roster
// views also receive members and active overrides in two additional queries.
func (r *OnCallRepository) hydrateSchedulesForTeam(teamID int, schedules []models.OnCallSchedule, includeRoster bool) ([]models.OnCallSchedule, error) {
	scheduleIndexes := make(map[int]int, len(schedules))
	for i := range schedules {
		scheduleIndexes[schedules[i].ID] = i
	}

	layerRows, err := r.db.Query(`
		SELECT l.id, l.schedule_id, l.name, l.priority, l.rotation_type,
		       l.rotation_interval_days, l.handoff_time, l.start_date, l.end_date,
		       l.created_at, l.updated_at
		FROM on_call_schedule_layers l
		JOIN on_call_schedules s ON s.id = l.schedule_id
		WHERE s.team_id = ?
		ORDER BY l.schedule_id, l.priority
	`, teamID)
	if err != nil {
		return nil, err
	}
	type layerLocation struct {
		schedule int
		layer    int
	}
	layerIndexes := make(map[int]layerLocation)
	for layerRows.Next() {
		var layer models.OnCallScheduleLayer
		var endDate sql.NullString
		if err := layerRows.Scan(
			&layer.ID, &layer.ScheduleID, &layer.Name, &layer.Priority, &layer.RotationType,
			&layer.RotationIntervalDays, &layer.HandoffTime, &layer.StartDate, &endDate,
			&layer.CreatedAt, &layer.UpdatedAt,
		); err != nil {
			_ = layerRows.Close()
			return nil, err
		}
		if endDate.Valid {
			layer.EndDate = &endDate.String
		}
		layer.Members = []models.OnCallScheduleLayerMember{}
		scheduleIndex, ok := scheduleIndexes[layer.ScheduleID]
		if !ok {
			continue
		}
		layerIndex := len(schedules[scheduleIndex].Layers)
		schedules[scheduleIndex].Layers = append(schedules[scheduleIndex].Layers, layer)
		layerIndexes[layer.ID] = layerLocation{schedule: scheduleIndex, layer: layerIndex}
	}
	if err := layerRows.Err(); err != nil {
		_ = layerRows.Close()
		return nil, err
	}
	if err := layerRows.Close(); err != nil {
		return nil, err
	}
	if !includeRoster {
		return schedules, nil
	}

	memberRows, err := r.db.Query(`
		SELECT m.id, m.layer_id, m.user_id, m.position, m.created_at,
		       u.first_name || ' ' || u.last_name AS user_name,
		       u.email, COALESCE(u.avatar_url, '') AS avatar_url
		FROM on_call_schedule_layer_members m
		JOIN on_call_schedule_layers l ON l.id = m.layer_id
		JOIN on_call_schedules s ON s.id = l.schedule_id
		JOIN users u ON u.id = m.user_id
		WHERE s.team_id = ?
		ORDER BY l.schedule_id, l.priority, m.position
	`, teamID)
	if err != nil {
		return nil, err
	}
	for memberRows.Next() {
		var member models.OnCallScheduleLayerMember
		if err := memberRows.Scan(
			&member.ID, &member.LayerID, &member.UserID, &member.Position, &member.CreatedAt,
			&member.UserName, &member.UserEmail, &member.UserAvatarURL,
		); err != nil {
			_ = memberRows.Close()
			return nil, err
		}
		location, ok := layerIndexes[member.LayerID]
		if !ok {
			continue
		}
		layer := &schedules[location.schedule].Layers[location.layer]
		layer.Members = append(layer.Members, member)
	}
	if err := memberRows.Err(); err != nil {
		_ = memberRows.Close()
		return nil, err
	}
	if err := memberRows.Close(); err != nil {
		return nil, err
	}

	overrideRows, err := r.db.Query(`
		SELECT o.id, o.schedule_id, o.user_id, o.override_user_id,
		       o.start_time, o.end_time, o.reason, o.created_by, o.created_at,
		       u.first_name || ' ' || u.last_name AS user_name,
		       ou.first_name || ' ' || ou.last_name AS override_user_name,
		       cb.first_name || ' ' || cb.last_name AS created_by_name
		FROM on_call_schedule_overrides o
		JOIN on_call_schedules s ON s.id = o.schedule_id
		JOIN users u ON u.id = o.user_id
		JOIN users ou ON ou.id = o.override_user_id
		LEFT JOIN users cb ON cb.id = o.created_by
		WHERE s.team_id = ? AND o.end_time > ?
		ORDER BY o.schedule_id, o.start_time
	`, teamID, time.Now())
	if err != nil {
		return nil, err
	}
	defer overrideRows.Close()
	for overrideRows.Next() {
		var override models.OnCallScheduleOverride
		var createdBy sql.NullInt64
		var createdByName sql.NullString
		if err := overrideRows.Scan(
			&override.ID, &override.ScheduleID, &override.UserID, &override.OverrideUserID,
			&override.StartTime, &override.EndTime, &override.Reason, &createdBy, &override.CreatedAt,
			&override.UserName, &override.OverrideUserName, &createdByName,
		); err != nil {
			return nil, err
		}
		override.CreatedBy = nullIntPtr(createdBy)
		override.CreatedByName = createdByName.String
		if scheduleIndex, ok := scheduleIndexes[override.ScheduleID]; ok {
			schedules[scheduleIndex].Overrides = append(schedules[scheduleIndex].Overrides, override)
		}
	}
	if err := overrideRows.Err(); err != nil {
		return nil, err
	}

	return schedules, nil
}

func (r *OnCallRepository) CreateSchedule(teamID int, name, description, timezone string, createdBy int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_schedules (team_id, name, description, timezone, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, true, ?, ?, ?) RETURNING id
	`, teamID, name, description, timezone, createdBy, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) UpdateSchedule(id int, name, description, timezone string, isActive bool) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE on_call_schedules SET name = ?, description = ?, timezone = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, name, description, timezone, isActive, now, id)
	return err
}

func (r *OnCallRepository) DeleteSchedule(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM on_call_schedules WHERE id = ?", id)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return layers, nil
}

// GetLayerByID returns a single rotation layer (without members). Used by the
// mutation handlers to verify the layer belongs to the authorized schedule
// before acting on it.
func (r *OnCallRepository) GetLayerByID(id int) (*models.OnCallScheduleLayer, error) {
	var l models.OnCallScheduleLayer
	var endDate sql.NullString
	err := r.db.QueryRow(`
		SELECT id, schedule_id, name, priority, rotation_type, rotation_interval_days,
			handoff_time, start_date, end_date, created_at, updated_at
		FROM on_call_schedule_layers
		WHERE id = ?
	`, id).Scan(
		&l.ID, &l.ScheduleID, &l.Name, &l.Priority, &l.RotationType,
		&l.RotationIntervalDays, &l.HandoffTime, &l.StartDate, &endDate,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if endDate.Valid {
		l.EndDate = &endDate.String
	}
	return &l, nil
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
	_, err := r.db.ExecWrite(`
		UPDATE on_call_schedule_layers
		SET name = ?, priority = ?, rotation_type = ?, rotation_interval_days = ?,
			handoff_time = ?, start_date = ?, end_date = ?, updated_at = ?
		WHERE id = ?
	`, name, priority, rotationType, intervalDays, handoffTime, startDate, endDate, now, id)
	return err
}

func (r *OnCallRepository) DeleteLayer(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM on_call_schedule_layers WHERE id = ?", id)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return members, nil
}

func (r *OnCallRepository) SetLayerMembers(layerID int, userIDs []int) error {
	// Delete existing members
	if _, err := r.db.ExecWrite("DELETE FROM on_call_schedule_layer_members WHERE layer_id = ?", layerID); err != nil {
		return err
	}

	// Insert new members in order
	now := time.Now()
	for i, userID := range userIDs {
		_, err := r.db.ExecWrite(`
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
	_, err := r.db.ExecWrite("DELETE FROM on_call_schedule_overrides WHERE id = ?", id)
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
	if err := rows.Err(); err != nil {
		return nil, err
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

// GetActivePolicyForTeam returns the team's active escalation policy, if any.
func (r *OnCallRepository) GetActivePolicyForTeam(teamID int) (*models.OnCallEscalationPolicy, error) {
	var id int
	err := r.db.QueryRow(`
		SELECT id FROM on_call_escalation_policies
		WHERE team_id = ? AND is_active = true
		ORDER BY id LIMIT 1
	`, teamID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetPolicyByID(id)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return policies, nil
}

func (r *OnCallRepository) CreatePolicy(teamID int, name, description string, repeatCount, createdBy int) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO on_call_escalation_policies (team_id, name, description, repeat_count, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, true, ?, ?, ?) RETURNING id
	`, teamID, name, description, repeatCount, createdBy, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *OnCallRepository) UpdatePolicy(id int, name, description string, repeatCount int, isActive bool) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE on_call_escalation_policies SET name = ?, description = ?, repeat_count = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, name, description, repeatCount, isActive, now, id)
	return err
}

func (r *OnCallRepository) DeletePolicy(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM on_call_escalation_policies WHERE id = ?", id)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rules, nil
}

func (r *OnCallRepository) SetEscalationRules(policyID int, rules []models.EscalationRuleInput) error {
	// Delete existing rules (cascade deletes notification rules)
	if _, err := r.db.ExecWrite("DELETE FROM on_call_escalation_rules WHERE policy_id = ?", policyID); err != nil {
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
			_, err := r.db.ExecWrite(`
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
	if err := rows.Err(); err != nil {
		return nil, err
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
	_, err := r.db.ExecWrite(`
		UPDATE on_call_swap_requests SET status = ?, responded_at = ? WHERE id = ?
	`, status, now, id)
	return err
}

// Incidents

// GetIncidentByID loads one incident with its item and policy context.
func (r *OnCallRepository) GetIncidentByID(id int) (*models.Incident, error) {
	inc, err := scanIncident(r.db.QueryRow(incidentSelectBody+" WHERE inc.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// GetIncidentForItem returns the incident the item currently points at.
func (r *OnCallRepository) GetIncidentForItem(itemID int) (*models.Incident, error) {
	inc, err := scanIncident(r.db.QueryRow(
		incidentSelectBody+" WHERE i.id = ? AND i.incident_id = inc.id", itemID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// CreateIncident inserts the incident and points the item at it atomically.
// A partial unique index rejects a second open incident for the same item.
func (r *OnCallRepository) CreateIncident(itemID int, policyID *int, urgency, source string) (int, error) {
	var incidentID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRow(`
			INSERT INTO incidents (item_id, status, urgency, source, escalation_policy_id, triggered_at, created_at, updated_at)
			VALUES (?, 'triggered', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, itemID, urgency, source, policyID).Scan(&incidentID); err != nil {
			return err
		}
		_, err := tx.Exec(
			`UPDATE items SET incident_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			incidentID, itemID,
		)
		return err
	})
	if err != nil {
		return 0, err
	}
	return incidentID, nil
}

// AcknowledgeIncident records the ack handshake and stops escalation.
func (r *OnCallRepository) AcknowledgeIncident(id, userID int, now time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE incidents
		SET status = 'acknowledged', acknowledged_at = ?, acknowledged_by = ?,
			next_escalation_at = NULL, updated_at = ?
		WHERE id = ?
	`, now, userID, now, id)
	return err
}

// UnacknowledgeIncident returns an incident to the triggered state so
// escalation resumes after an accidental ack.
func (r *OnCallRepository) UnacknowledgeIncident(id int, now time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE incidents
		SET status = 'triggered', acknowledged_at = NULL, acknowledged_by = NULL,
			resolved_at = NULL, resolved_by = NULL, next_escalation_at = NULL, updated_at = ?
		WHERE id = ?
	`, now, id)
	return err
}

// ResolveIncident ends the incident. Resolve is not item completion.
func (r *OnCallRepository) ResolveIncident(id, userID int, now time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE incidents
		SET status = 'resolved', resolved_at = ?, resolved_by = ?,
			next_escalation_at = NULL, updated_at = ?
		WHERE id = ?
	`, now, userID, now, id)
	return err
}

// SetIncidentEscalation persists the engine cursor: the current step, the
// repeat count, and the next escalation deadline (nil when exhausted).
func (r *OnCallRepository) SetIncidentEscalation(id, step, repeatCount int, next *time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE incidents
		SET escalation_step = ?, escalation_repeat_count = ?, next_escalation_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, step, repeatCount, next, id)
	return err
}

// FindDueIncidentIDs returns triggered incidents whose escalation deadline has
// passed, oldest first.
func (r *OnCallRepository) FindDueIncidentIDs(now time.Time, limit int) ([]int, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(`
		SELECT id FROM incidents
		WHERE status = 'triggered' AND next_escalation_at IS NOT NULL AND next_escalation_at <= ?
		ORDER BY next_escalation_at
		LIMIT ?
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int, 0, limit)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteIncidentNotificationStates clears every pending scheduled notification
// for an incident. Called when the incident is acked/resolved or escalates.
func (r *OnCallRepository) DeleteIncidentNotificationStates(incidentID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM incident_notification_state WHERE incident_id = ?`, incidentID)
	return err
}

// CreateIncidentNotificationState schedules one delayed/repeated notification.
func (r *OnCallRepository) CreateIncidentNotificationState(incidentID, escalationRuleID, notificationRuleID, repeatIndex int, nextAt time.Time) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO incident_notification_state
			(incident_id, escalation_rule_id, notification_rule_id, repeat_index, next_notification_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, incidentID, escalationRuleID, notificationRuleID, repeatIndex, nextAt)
	return err
}

// FindDueNotificationStateIDs returns scheduled notification rows due now.
func (r *OnCallRepository) FindDueNotificationStateIDs(now time.Time, limit int) ([]int, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(`
		SELECT id FROM incident_notification_state
		WHERE next_notification_at <= ?
		ORDER BY next_notification_at
		LIMIT ?
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int, 0, limit)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetNotificationState loads a scheduled notification row.
func (r *OnCallRepository) GetNotificationState(id int) (*models.IncidentNotificationState, error) {
	var state models.IncidentNotificationState
	err := r.db.QueryRow(`
		SELECT id, incident_id, escalation_rule_id, notification_rule_id, repeat_index,
		       next_notification_at, created_at, updated_at
		FROM incident_notification_state WHERE id = ?
	`, id).Scan(
		&state.ID, &state.IncidentID, &state.EscalationRuleID, &state.NotificationRuleID,
		&state.RepeatIndex, &state.NextNotificationAt, &state.CreatedAt, &state.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// AdvanceNotificationState moves a scheduled notification to its next repeat,
// or deletes it when the chain is finished.
func (r *OnCallRepository) AdvanceNotificationState(id, nextRepeatIndex int, nextAt *time.Time) error {
	if nextAt == nil {
		return r.DeleteNotificationState(id)
	}
	_, err := r.db.ExecWrite(`
		UPDATE incident_notification_state
		SET repeat_index = ?, next_notification_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, nextRepeatIndex, *nextAt, id)
	return err
}

// DeleteNotificationState removes a completed scheduled notification.
func (r *OnCallRepository) DeleteNotificationState(id int) error {
	_, err := r.db.ExecWrite(`DELETE FROM incident_notification_state WHERE id = ?`, id)
	return err
}

// GetNotificationRuleByID loads one notification rule.
func (r *OnCallRepository) GetNotificationRuleByID(id int) (*models.OnCallNotificationRule, error) {
	var rule models.OnCallNotificationRule
	var repeatInterval sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, escalation_rule_id, notification_type, delay_minutes,
		       repeat_interval_minutes, repeat_count, created_at
		FROM on_call_notification_rules WHERE id = ?
	`, id).Scan(
		&rule.ID, &rule.EscalationRuleID, &rule.NotificationType, &rule.DelayMinutes,
		&repeatInterval, &rule.RepeatCount, &rule.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rule.RepeatIntervalMinutes = nullIntPtr(repeatInterval)
	return &rule, nil
}

// GetEscalationRuleByID loads one escalation rule.
func (r *OnCallRepository) GetEscalationRuleByID(id int) (*models.OnCallEscalationRule, error) {
	var rule models.OnCallEscalationRule
	err := r.db.QueryRow(`
		SELECT id, policy_id, step_order, escalation_delay_minutes, target_type, target_id, created_at
		FROM on_call_escalation_rules WHERE id = ?
	`, id).Scan(
		&rule.ID, &rule.PolicyID, &rule.StepOrder, &rule.EscalationDelayMinutes,
		&rule.TargetType, &rule.TargetID, &rule.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListIncidents returns incidents for the caller's teams/workspaces.
func (r *OnCallRepository) ListIncidents(filter IncidentFilter) ([]models.Incident, error) {
	if !filter.AllTeams && len(filter.TeamIDs) == 0 {
		return []models.Incident{}, nil
	}

	query := incidentSelectBody + " WHERE 1=1"
	args := []any{}

	if filter.PolicyID != nil {
		query += " AND inc.escalation_policy_id = ?"
		args = append(args, *filter.PolicyID)
	}
	if filter.Status != "" {
		query += " AND inc.status = ?"
		args = append(args, filter.Status)
	}
	if filter.ItemID != nil {
		query += " AND inc.item_id = ?"
		args = append(args, *filter.ItemID)
	}
	if !filter.AllTeams {
		query += " AND i.team_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(filter.TeamIDs)), ",") + ")"
		for _, teamID := range filter.TeamIDs {
			args = append(args, teamID)
		}
	}
	if len(filter.WorkspaceIDs) == 0 {
		return []models.Incident{}, nil
	}
	query += " AND i.workspace_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(filter.WorkspaceIDs)), ",") + ")"
	for _, workspaceID := range filter.WorkspaceIDs {
		args = append(args, workspaceID)
	}

	query += " ORDER BY inc.triggered_at DESC, inc.id DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	incidents := []models.Incident{}
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return incidents, nil
}
