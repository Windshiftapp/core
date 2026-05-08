package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TeamRepository provides data access methods for teams
type TeamRepository struct {
	db database.Database
}

// NewTeamRepository creates a new team repository
func NewTeamRepository(db database.Database) *TeamRepository {
	return &TeamRepository{db: db}
}

// GetByID retrieves a team by ID with created_by_name, direct_member_count, and group_count
func (r *TeamRepository) GetByID(id int) (*models.Team, error) {
	var team models.Team
	var createdBy sql.NullInt64
	var createdByName sql.NullString

	var icon, color, avatarURL sql.NullString
	err := r.db.QueryRow(`
		SELECT t.id, t.name, t.description, t.is_active,
		       COALESCE(t.icon, ''), COALESCE(t.color, ''), COALESCE(t.avatar_url, ''),
		       t.created_by, t.created_at, t.updated_at,
		       u.first_name || ' ' || u.last_name as created_by_name,
		       (SELECT COUNT(*) FROM team_members WHERE team_id = t.id) as direct_member_count,
		       (SELECT COUNT(*) FROM team_groups WHERE team_id = t.id) as group_count
		FROM teams t
		LEFT JOIN users u ON t.created_by = u.id
		WHERE t.id = ?
	`, id).Scan(
		&team.ID, &team.Name, &team.Description, &team.IsActive,
		&icon, &color, &avatarURL,
		&createdBy, &team.CreatedAt, &team.UpdatedAt, &createdByName,
		&team.DirectMemberCount, &team.GroupCount,
	)
	team.Icon = icon.String
	team.Color = color.String
	team.AvatarURL = avatarURL.String

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find team: %w", err)
	}

	if createdBy.Valid {
		val := int(createdBy.Int64)
		team.CreatedBy = &val
	}
	if createdByName.Valid {
		team.CreatedByName = createdByName.String
	}

	return &team, nil
}

// List returns all teams with member counts, ordered by name
func (r *TeamRepository) List() ([]models.Team, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.description, t.is_active,
		       COALESCE(t.icon, ''), COALESCE(t.color, ''), COALESCE(t.avatar_url, ''),
		       t.created_by, t.created_at, t.updated_at,
		       u.first_name || ' ' || u.last_name as created_by_name,
		       (SELECT COUNT(*) FROM team_members WHERE team_id = t.id) as direct_member_count,
		       (SELECT COUNT(*) FROM team_groups WHERE team_id = t.id) as group_count
		FROM teams t
		LEFT JOIN users u ON t.created_by = u.id
		ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var teams []models.Team
	for rows.Next() {
		var team models.Team
		var createdBy sql.NullInt64
		var createdByName sql.NullString

		var icon, color, avatarURL sql.NullString
		if err := rows.Scan(
			&team.ID, &team.Name, &team.Description, &team.IsActive,
			&icon, &color, &avatarURL,
			&createdBy, &team.CreatedAt, &team.UpdatedAt, &createdByName,
			&team.DirectMemberCount, &team.GroupCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}
		team.Icon = icon.String
		team.Color = color.String
		team.AvatarURL = avatarURL.String

		if createdBy.Valid {
			val := int(createdBy.Int64)
			team.CreatedBy = &val
		}
		if createdByName.Valid {
			team.CreatedByName = createdByName.String
		}

		teams = append(teams, team)
	}

	return teams, rows.Err()
}

// Create inserts a new team and returns its ID
func (r *TeamRepository) Create(name, description, icon, color, avatarURL string, createdBy int) (int, error) {
	now := time.Now()
	var id int64

	err := r.db.QueryRow(`
		INSERT INTO teams (name, description, is_active, icon, color, avatar_url, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, name, description, true, icon, color, avatarURL, createdBy, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("failed to create team: %w", err)
	}

	return int(id), nil
}

// Update updates an existing team
func (r *TeamRepository) Update(id int, name, description string, isActive bool, icon, color, avatarURL string) error {
	now := time.Now()
	result, err := r.db.Exec(`
		UPDATE teams
		SET name = ?, description = ?, is_active = ?, icon = ?, color = ?, avatar_url = ?, updated_at = ?
		WHERE id = ?
	`, name, description, isActive, icon, color, avatarURL, now, id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("failed to update team: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a team by ID
func (r *TeamRepository) Delete(id int) error {
	result, err := r.db.Exec("DELETE FROM teams WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// GetDirectMembers returns direct members of a team with joined user fields
func (r *TeamRepository) GetDirectMembers(teamID int) ([]models.TeamMember, error) {
	rows, err := r.db.Query(`
		SELECT tm.id, tm.team_id, tm.user_id, tm.role, tm.added_by, tm.added_at, tm.created_at,
		       u.first_name || ' ' || u.last_name as user_name,
		       u.email as user_email,
		       u.username as user_username,
		       COALESCE(u.avatar_url, '') as user_avatar_url,
		       COALESCE(adder.first_name || ' ' || adder.last_name, '') as added_by_name
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id
		LEFT JOIN users adder ON adder.id = tm.added_by
		WHERE tm.team_id = ?
		ORDER BY u.last_name, u.first_name
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []models.TeamMember
	for rows.Next() {
		var member models.TeamMember
		var addedBy sql.NullInt64

		if err := rows.Scan(
			&member.ID, &member.TeamID, &member.UserID, &member.Role, &addedBy,
			&member.AddedAt, &member.CreatedAt,
			&member.UserName, &member.UserEmail, &member.UserUsername,
			&member.UserAvatarURL, &member.AddedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}

		if addedBy.Valid {
			val := int(addedBy.Int64)
			member.AddedBy = &val
		}

		members = append(members, member)
	}

	return members, rows.Err()
}

// AddDirectMember adds a user as a direct member of a team (ignores if already exists)
func (r *TeamRepository) AddDirectMember(teamID, userID int, role string, addedBy int) error {
	now := time.Now()
	_, err := r.db.Exec(`
		INSERT INTO team_members (team_id, user_id, role, added_by, added_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, teamID, userID, role, addedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}
	return nil
}

// RemoveDirectMember removes a user from a team's direct members
func (r *TeamRepository) RemoveDirectMember(teamID, userID int) error {
	_, err := r.db.Exec(`
		DELETE FROM team_members WHERE team_id = ? AND user_id = ?
	`, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}
	return nil
}

// UpdateMemberRole updates the role of a direct team member
func (r *TeamRepository) UpdateMemberRole(teamID, userID int, role string) error {
	result, err := r.db.Exec(`
		UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?
	`, role, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// IsTeamAdmin returns true if the user has role "admin" in the team's direct members
func (r *TeamRepository) IsTeamAdmin(teamID, userID int) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM team_members
		WHERE team_id = ? AND user_id = ? AND role = 'admin'
	`, teamID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check team admin: %w", err)
	}
	return count > 0, nil
}

// IsTeamMember returns true if the user belongs to the team — either directly
// (any role) or transitively through a mapped group. Used by view-side
// permission gates (e.g. who can read the current on-call roster).
func (r *TeamRepository) IsTeamMember(teamID, userID int) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?
			UNION ALL
			SELECT 1 FROM team_groups tg
			JOIN group_members gm ON gm.group_id = tg.group_id
			WHERE tg.team_id = ? AND gm.user_id = ?
		)
	`, teamID, userID, teamID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check team membership: %w", err)
	}
	return count > 0, nil
}

// GetMappedGroups returns groups mapped to a team with joined group name and member count
func (r *TeamRepository) GetMappedGroups(teamID int) ([]models.TeamGroupMapping, error) {
	rows, err := r.db.Query(`
		SELECT tg.id, tg.team_id, tg.group_id, tg.added_by, tg.added_at,
		       g.name as group_name,
		       (SELECT COUNT(*) FROM group_members WHERE group_id = tg.group_id) as member_count,
		       COALESCE(adder.first_name || ' ' || adder.last_name, '') as added_by_name
		FROM team_groups tg
		JOIN groups g ON g.id = tg.group_id
		LEFT JOIN users adder ON adder.id = tg.added_by
		WHERE tg.team_id = ?
		ORDER BY g.name
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mappings []models.TeamGroupMapping
	for rows.Next() {
		var mapping models.TeamGroupMapping
		var addedBy sql.NullInt64

		if err := rows.Scan(
			&mapping.ID, &mapping.TeamID, &mapping.GroupID, &addedBy, &mapping.AddedAt,
			&mapping.GroupName, &mapping.MemberCount, &mapping.AddedByName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan group mapping: %w", err)
		}

		if addedBy.Valid {
			val := int(addedBy.Int64)
			mapping.AddedBy = &val
		}

		mappings = append(mappings, mapping)
	}

	return mappings, rows.Err()
}

// AddGroupMapping adds a group mapping to a team (ignores if already exists)
func (r *TeamRepository) AddGroupMapping(teamID, groupID, addedBy int) error {
	now := time.Now()
	_, err := r.db.Exec(`
		INSERT INTO team_groups (team_id, group_id, added_by, added_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, teamID, groupID, addedBy, now)
	if err != nil {
		return fmt.Errorf("failed to add group mapping: %w", err)
	}
	return nil
}

// RemoveGroupMapping removes a group mapping from a team
func (r *TeamRepository) RemoveGroupMapping(teamID, groupID int) error {
	_, err := r.db.Exec(`
		DELETE FROM team_groups WHERE team_id = ? AND group_id = ?
	`, teamID, groupID)
	if err != nil {
		return fmt.Errorf("failed to remove group mapping: %w", err)
	}
	return nil
}

// GetResolvedMembers returns the union of direct members and group members,
// deduplicated with direct membership taking precedence, annotated with leave status
func (r *TeamRepository) GetResolvedMembers(teamID int) ([]models.ResolvedTeamMember, error) {
	rows, err := r.db.Query(`
		WITH all_members AS (
			SELECT user_id, 1 as is_direct FROM team_members WHERE team_id = ?
			UNION ALL
			SELECT gm.user_id, 0 as is_direct FROM team_groups tg
			JOIN group_members gm ON gm.group_id = tg.group_id
			WHERE tg.team_id = ?
		),
		deduped AS (
			SELECT user_id, MAX(is_direct) as is_direct FROM all_members GROUP BY user_id
		)
		SELECT u.id, u.first_name || ' ' || u.last_name as name, u.email, u.username,
			COALESCE(u.avatar_url, '') as avatar_url,
			CASE WHEN d.is_direct = 1 THEN 'direct' ELSE 'group' END as source,
			CASE WHEN d.is_direct = 1 THEN 'Direct'
				 ELSE COALESCE((
					 SELECT g.name FROM team_groups tg
					 JOIN group_members gm ON gm.group_id = tg.group_id AND gm.user_id = u.id
					 JOIN groups g ON g.id = tg.group_id
					 WHERE tg.team_id = ?
					 LIMIT 1
				 ), '') END as source_name,
			CASE WHEN ulp.id IS NOT NULL THEN 1 ELSE 0 END as is_on_leave,
			ulp.substitute_user_id,
			COALESCE(sub.first_name || ' ' || sub.last_name, '') as substitute_name
		FROM deduped d
		JOIN users u ON u.id = d.user_id
		LEFT JOIN user_leave_periods ulp ON ulp.user_id = u.id AND ulp.is_active = true
			AND ulp.start_date <= date('now') AND ulp.end_date >= date('now')
		LEFT JOIN users sub ON sub.id = ulp.substitute_user_id
		WHERE u.is_active = true
		ORDER BY u.last_name, u.first_name
	`, teamID, teamID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resolved members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []models.ResolvedTeamMember
	for rows.Next() {
		var member models.ResolvedTeamMember
		var isOnLeave int
		var substituteID sql.NullInt64

		if err := rows.Scan(
			&member.UserID, &member.UserName, &member.UserEmail, &member.UserUsername,
			&member.UserAvatarURL, &member.Source, &member.SourceName,
			&isOnLeave, &substituteID, &member.SubstituteName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan resolved member: %w", err)
		}

		member.IsOnLeave = isOnLeave == 1
		if substituteID.Valid {
			val := int(substituteID.Int64)
			member.SubstituteID = &val
		}

		members = append(members, member)
	}

	return members, rows.Err()
}

// GetTeamsForUser returns teams where the user is a direct member or in a mapped group
func (r *TeamRepository) GetTeamsForUser(userID int) ([]models.Team, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT t.id, t.name, t.description, t.is_active, t.created_by, t.created_at, t.updated_at
		FROM teams t
		WHERE t.id IN (
			SELECT team_id FROM team_members WHERE user_id = ?
			UNION
			SELECT tg.team_id FROM team_groups tg
			JOIN group_members gm ON gm.group_id = tg.group_id
			WHERE gm.user_id = ?
		)
		ORDER BY t.name
	`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get teams for user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var teams []models.Team
	for rows.Next() {
		var team models.Team
		var createdBy sql.NullInt64

		if err := rows.Scan(
			&team.ID, &team.Name, &team.Description, &team.IsActive, &createdBy,
			&team.CreatedAt, &team.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}

		if createdBy.Valid {
			val := int(createdBy.Int64)
			team.CreatedBy = &val
		}

		teams = append(teams, team)
	}

	return teams, rows.Err()
}

// GetRoundRobinState retrieves the round-robin assignment state for a given action node and team
func (r *TeamRepository) GetRoundRobinState(actionNodeID, teamID int) (*models.RoundRobinState, error) {
	var state models.RoundRobinState
	var lastAssignedUserID sql.NullInt64
	var lastAssignedAt sql.NullTime

	err := r.db.QueryRow(`
		SELECT id, action_node_id, team_id, last_assigned_user_id, last_assigned_at,
		       assignment_count, updated_at
		FROM team_round_robin_state
		WHERE action_node_id = ? AND team_id = ?
	`, actionNodeID, teamID).Scan(
		&state.ID, &state.ActionNodeID, &state.TeamID, &lastAssignedUserID,
		&lastAssignedAt, &state.AssignmentCount, &state.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get round robin state: %w", err)
	}

	if lastAssignedUserID.Valid {
		val := int(lastAssignedUserID.Int64)
		state.LastAssignedUserID = &val
	}
	if lastAssignedAt.Valid {
		state.LastAssignedAt = &lastAssignedAt.Time
	}

	return &state, nil
}

// UpdateRoundRobinState upserts the round-robin assignment state for a given action node and team
func (r *TeamRepository) UpdateRoundRobinState(actionNodeID, teamID, userID int) error {
	now := time.Now()
	_, err := r.db.Exec(`
		INSERT INTO team_round_robin_state (action_node_id, team_id, last_assigned_user_id, last_assigned_at, assignment_count, updated_at)
		VALUES (?, ?, ?, ?, 1, ?)
		ON CONFLICT (action_node_id, team_id) DO UPDATE SET
			last_assigned_user_id = excluded.last_assigned_user_id,
			last_assigned_at = excluded.last_assigned_at,
			assignment_count = team_round_robin_state.assignment_count + 1,
			updated_at = excluded.updated_at
	`, actionNodeID, teamID, userID, now, now)
	if err != nil {
		return fmt.Errorf("failed to update round robin state: %w", err)
	}
	return nil
}
