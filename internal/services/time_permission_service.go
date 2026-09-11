package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TimePermissionService handles permission checks for time tracking
type TimePermissionService struct {
	db                database.Database
	permissionService *PermissionService
}

// NewTimePermissionService creates a new TimePermissionService
func NewTimePermissionService(db database.Database, permissionService *PermissionService) *TimePermissionService {
	return &TimePermissionService{
		db:                db,
		permissionService: permissionService,
	}
}

// HasProjectManagePermission checks if user has the global project.manage permission
// Returns true if: system.admin OR project.manage
func (s *TimePermissionService) HasProjectManagePermission(userID int) (bool, error) {
	// Check system admin first
	isAdmin, err := s.permissionService.IsSystemAdmin(userID)
	if err != nil {
		return false, fmt.Errorf("error checking system admin: %w", err)
	}
	if isAdmin {
		return true, nil
	}

	// Check global project.manage permission
	hasPermission, err := s.permissionService.HasGlobalPermission(userID, models.PermissionProjectManage)
	if err != nil {
		return false, fmt.Errorf("error checking project.manage permission: %w", err)
	}

	return hasPermission, nil
}

// HasProjectManagePermissionContext is the request-aware form used while
// enriching item-list responses.
func (s *TimePermissionService) HasProjectManagePermissionContext(ctx context.Context, userID int) (bool, error) {
	isAdmin, err := s.permissionService.IsSystemAdminContext(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("error checking system admin: %w", err)
	}
	if isAdmin {
		return true, nil
	}
	hasPermission, err := s.permissionService.HasGlobalPermissionContext(ctx, userID, models.PermissionProjectManage)
	if err != nil {
		return false, fmt.Errorf("error checking project.manage permission: %w", err)
	}
	return hasPermission, nil
}

// HasCustomersManagePermission checks if user has the global customers.manage permission
// Returns true if: system.admin OR customers.manage OR project.manage
func (s *TimePermissionService) HasCustomersManagePermission(userID int) (bool, error) {
	// Check system admin first
	isAdmin, err := s.permissionService.IsSystemAdmin(userID)
	if err != nil {
		return false, fmt.Errorf("error checking system admin: %w", err)
	}
	if isAdmin {
		return true, nil
	}

	// Check global project.manage permission (project managers can also manage customers)
	hasProjectManage, err := s.permissionService.HasGlobalPermission(userID, models.PermissionProjectManage)
	if err != nil {
		return false, fmt.Errorf("error checking project.manage permission: %w", err)
	}
	if hasProjectManage {
		return true, nil
	}

	// Check global customers.manage permission
	hasPermission, err := s.permissionService.HasGlobalPermission(userID, models.PermissionCustomersManage)
	if err != nil {
		return false, fmt.Errorf("error checking customers.manage permission: %w", err)
	}

	return hasPermission, nil
}

// IsTimeProjectManager requires an assignment or global management authority.
func (s *TimePermissionService) IsTimeProjectManager(userID, projectID int) (bool, error) {
	hasFullAccess, err := s.HasProjectManagePermission(userID)
	if err != nil || hasFullAccess {
		return hasFullAccess, err
	}
	return s.isProjectManager(userID, projectID)
}

// CanGrantProjectAccess uses the same authority as project management.
func (s *TimePermissionService) CanGrantProjectAccess(userID, projectID int) (bool, error) {
	return s.IsTimeProjectManager(userID, projectID)
}

// CanBookTimeOnProject checks if user can create worklogs on a specific project
// True if: IsTimeProjectManager OR assigned as member (user/group) OR no members configured
func (s *TimePermissionService) CanBookTimeOnProject(userID, projectID int) (bool, error) {
	// 1. Managers can always book on their projects
	isManager, err := s.IsTimeProjectManager(userID, projectID)
	if err != nil {
		return false, err
	}
	if isManager {
		return true, nil
	}

	// 2. Check if project has MEMBER restrictions configured
	hasMembers, err := s.HasProjectMembers(projectID)
	if err != nil {
		return false, err
	}
	if !hasMembers {
		return true, nil // No member restrictions - booking open to all
	}

	// 3. Members exist - check if user is assigned as member
	return s.isProjectMember(userID, projectID)
}

// CanViewProject checks if user can view a specific project
// True if: IsTimeProjectManager OR CanBookTimeOnProject
func (s *TimePermissionService) CanViewProject(userID, projectID int) (bool, error) {
	// Same as booking permission for now
	return s.CanBookTimeOnProject(userID, projectID)
}

// AccessibleTimeProjectIDs resolves the projects whose worklogs a user may view.
func (s *TimePermissionService) AccessibleTimeProjectIDs(userID int) ([]int, error) {
	rows, err := s.db.Query("SELECT id FROM time_projects ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list time projects for access: %w", err)
	}
	var candidates []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan time project for access: %w", err)
		}
		candidates = append(candidates, id)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close time project access rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read time projects for access: %w", err)
	}
	result := make([]int, 0, len(candidates))
	for _, id := range candidates {
		allowed, err := s.CanViewProject(userID, id)
		if err != nil {
			return nil, err
		}
		if allowed {
			result = append(result, id)
		}
	}
	return result, nil
}

// HasProjectManagers checks if a project has any manager restrictions configured
func (s *TimePermissionService) HasProjectManagers(projectID int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM time_project_managers WHERE project_id = ?
	`, projectID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error checking project managers: %w", err)
	}
	return count > 0, nil
}

// HasProjectMembers checks if a project has any member restrictions configured
func (s *TimePermissionService) HasProjectMembers(projectID int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM time_project_members WHERE project_id = ?
	`, projectID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error checking project members: %w", err)
	}
	return count > 0, nil
}

// isProjectManager checks if user is directly or via group assigned as manager
func (s *TimePermissionService) isProjectManager(userID, projectID int) (bool, error) {
	// Check direct user assignment
	var directAssigned bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM time_project_managers
			WHERE project_id = ? AND manager_type = 'user' AND manager_id = ?
		)
	`, projectID, userID).Scan(&directAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking direct manager assignment: %w", err)
	}
	if directAssigned {
		return true, nil
	}

	// Check group assignment
	var groupAssigned bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM time_project_managers tpm
			JOIN group_members gm ON tpm.manager_id = gm.group_id
			WHERE tpm.project_id = ? AND tpm.manager_type = 'group' AND gm.user_id = ?
		)
	`, projectID, userID).Scan(&groupAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking group manager assignment: %w", err)
	}

	return groupAssigned, nil
}

// isProjectMember checks if user is directly or via group assigned as member
func (s *TimePermissionService) isProjectMember(userID, projectID int) (bool, error) {
	// Check direct user assignment
	var directAssigned bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM time_project_members
			WHERE project_id = ? AND member_type = 'user' AND member_id = ?
		)
	`, projectID, userID).Scan(&directAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking direct member assignment: %w", err)
	}
	if directAssigned {
		return true, nil
	}

	// Check group assignment
	var groupAssigned bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM time_project_members tpm
			JOIN group_members gm ON tpm.member_id = gm.group_id
			WHERE tpm.project_id = ? AND tpm.member_type = 'group' AND gm.user_id = ?
		)
	`, projectID, userID).Scan(&groupAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking group member assignment: %w", err)
	}

	return groupAssigned, nil
}

// GetAccessibleProjects returns project IDs user can access (nil = all accessible)
// For users with project.manage, returns nil (all projects)
// For other users, returns projects where they are manager, member, or no restrictions exist
func (s *TimePermissionService) GetAccessibleProjects(userID int) ([]int, error) {
	return s.GetAccessibleProjectsContext(context.Background(), userID)
}

// GetAccessibleProjectsContext is the request-aware form of GetAccessibleProjects.
func (s *TimePermissionService) GetAccessibleProjectsContext(ctx context.Context, userID int) ([]int, error) {
	// Check if user has full access
	hasFullAccess, err := s.HasProjectManagePermissionContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if hasFullAccess {
		return nil, nil // nil means all projects are accessible
	}

	// Get all projects with no restrictions or where user has access
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.id FROM time_projects p
		WHERE
			-- Membership restrictions make a project private.
			NOT EXISTS (SELECT 1 FROM time_project_members WHERE project_id = p.id)
			-- OR user is direct manager
			OR EXISTS (SELECT 1 FROM time_project_managers WHERE project_id = p.id AND manager_type = 'user' AND manager_id = ?)
			-- OR user is in a manager group
			OR EXISTS (
				SELECT 1 FROM time_project_managers tpm
				JOIN group_members gm ON tpm.manager_id = gm.group_id
				WHERE tpm.project_id = p.id AND tpm.manager_type = 'group' AND gm.user_id = ?
			)
			-- OR user is direct member (when member restrictions exist)
			OR EXISTS (SELECT 1 FROM time_project_members WHERE project_id = p.id AND member_type = 'user' AND member_id = ?)
			-- OR user is in a member group (when member restrictions exist)
			OR EXISTS (
				SELECT 1 FROM time_project_members tpm
				JOIN group_members gm ON tpm.member_id = gm.group_id
				WHERE tpm.project_id = p.id AND tpm.member_type = 'group' AND gm.user_id = ?
			)
	`, userID, userID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("error getting accessible projects: %w", err)
	}
	defer rows.Close()

	// Initialize as a non-nil slice: nil is reserved for the full-access
	// sentinel above, so a user with zero accessible projects must return
	// an empty slice rather than accidentally signal full access.
	projectIDs := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning project ID: %w", err)
		}
		projectIDs = append(projectIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating projects: %w", err)
	}

	return projectIDs, nil
}

// MaskInaccessibleProjectNames blanks the human-readable project name fields on
// items whose project / time-project / effective-project the user is not allowed
// to view, so a restricted time project's name isn't disclosed to item viewers
// who lack time-project access. Project *IDs* are left intact (they carry no
// name); only the names are stripped. A user with full project access (the
// GetAccessibleProjects nil sentinel) is never masked.
func (s *TimePermissionService) MaskInaccessibleProjectNames(userID int, items []models.Item) {
	s.MaskInaccessibleProjectNamesContext(context.Background(), userID, items)
}

// MaskInaccessibleProjectNamesContext is the request-aware form used by item
// list/search/backlog responses.
func (s *TimePermissionService) MaskInaccessibleProjectNamesContext(ctx context.Context, userID int, items []models.Item) {
	accessible, err := s.GetAccessibleProjectsContext(ctx, userID)
	if err != nil {
		slog.Warn("failed to load accessible projects for masking", slog.Int("user_id", userID), slog.Any("error", err))
		// Fail closed: if we can't determine access, strip names rather than leak.
		for i := range items {
			items[i].ProjectName = ""
			items[i].TimeProjectName = ""
			items[i].EffectiveProjectName = ""
		}
		return
	}
	if accessible == nil {
		return // full access: nothing to mask
	}
	allowed := make(map[int]struct{}, len(accessible))
	for _, id := range accessible {
		allowed[id] = struct{}{}
	}
	canSee := func(p *int) bool {
		if p == nil {
			return true // no project assigned → no name to hide
		}
		_, ok := allowed[*p]
		return ok
	}
	for i := range items {
		if !canSee(items[i].ProjectID) {
			items[i].ProjectName = ""
		}
		if !canSee(items[i].TimeProjectID) {
			items[i].TimeProjectName = ""
		}
		if !canSee(items[i].EffectiveProjectID) {
			items[i].EffectiveProjectName = ""
		}
	}
}

// CanViewWorklog grants managers all entries and other project viewers their own.
func (s *TimePermissionService) CanViewWorklog(userID, worklogID int) (bool, error) {
	return s.canAccessWorklog(userID, worklogID)
}

// CanEditWorklog requires project visibility and ownership or management authority.
func (s *TimePermissionService) CanEditWorklog(userID, worklogID int) (bool, error) {
	return s.canAccessWorklog(userID, worklogID)
}

func (s *TimePermissionService) canAccessWorklog(userID, worklogID int) (bool, error) {
	var projectID int
	var owner sql.NullInt64
	err := s.db.QueryRow("SELECT project_id, user_id FROM time_worklogs WHERE id = ?", worklogID).Scan(&projectID, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get worklog access: %w", err)
	}
	visible, err := s.CanViewProject(userID, projectID)
	if err != nil || !visible {
		return false, err
	}
	if owner.Valid && int(owner.Int64) == userID {
		return true, nil
	}
	return s.IsTimeProjectManager(userID, projectID)
}

// GetManagedProjects returns assigned projects, or nil for global managers.
func (s *TimePermissionService) GetManagedProjects(userID int) ([]int, error) {
	full, err := s.HasProjectManagePermission(userID)
	if err != nil || full {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT DISTINCT project_id FROM time_project_managers
 WHERE (manager_type = 'user' AND manager_id = ?)
 OR (manager_type = 'group' AND manager_id IN (SELECT group_id FROM group_members WHERE user_id = ?))
 ORDER BY project_id`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("list managed projects: %w", err)
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetProjectManagers returns all managers for a project
func (s *TimePermissionService) GetProjectManagers(projectID int) ([]models.TimeProjectManager, error) {
	rows, err := s.db.Query(`
		SELECT tpm.id, tpm.project_id, tpm.manager_type, tpm.manager_id, tpm.granted_by, tpm.granted_at,
		       CASE
		           WHEN tpm.manager_type = 'user' THEN u.first_name || ' ' || u.last_name
		           WHEN tpm.manager_type = 'group' THEN g.name
		       END as manager_name,
		       CASE
		           WHEN tpm.manager_type = 'user' THEN u.email
		           ELSE ''
		       END as manager_email
		FROM time_project_managers tpm
		LEFT JOIN users u ON tpm.manager_type = 'user' AND tpm.manager_id = u.id
		LEFT JOIN groups g ON tpm.manager_type = 'group' AND tpm.manager_id = g.id
		WHERE tpm.project_id = ?
		ORDER BY tpm.granted_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("error getting project managers: %w", err)
	}
	defer rows.Close()

	var managers []models.TimeProjectManager
	for rows.Next() {
		var m models.TimeProjectManager
		var name, email sql.NullString
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.ManagerType, &m.ManagerID, &m.GrantedBy, &m.GrantedAt, &name, &email); err != nil {
			return nil, fmt.Errorf("error scanning manager: %w", err)
		}
		m.ManagerName = name.String
		m.ManagerEmail = email.String
		managers = append(managers, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating managers: %w", err)
	}

	return managers, nil
}

// GetProjectMembers returns all members for a project
func (s *TimePermissionService) GetProjectMembers(projectID int) ([]models.TimeProjectMember, error) {
	rows, err := s.db.Query(`
		SELECT tpm.id, tpm.project_id, tpm.member_type, tpm.member_id, tpm.granted_by, tpm.granted_at,
		       CASE
		           WHEN tpm.member_type = 'user' THEN u.first_name || ' ' || u.last_name
		           WHEN tpm.member_type = 'group' THEN g.name
		       END as member_name,
		       CASE
		           WHEN tpm.member_type = 'user' THEN u.email
		           ELSE ''
		       END as member_email
		FROM time_project_members tpm
		LEFT JOIN users u ON tpm.member_type = 'user' AND tpm.member_id = u.id
		LEFT JOIN groups g ON tpm.member_type = 'group' AND tpm.member_id = g.id
		WHERE tpm.project_id = ?
		ORDER BY tpm.granted_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("error getting project members: %w", err)
	}
	defer rows.Close()

	var members []models.TimeProjectMember
	for rows.Next() {
		var m models.TimeProjectMember
		var name, email sql.NullString
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.MemberType, &m.MemberID, &m.GrantedBy, &m.GrantedAt, &name, &email); err != nil {
			return nil, fmt.Errorf("error scanning member: %w", err)
		}
		m.MemberName = name.String
		m.MemberEmail = email.String
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating members: %w", err)
	}

	return members, nil
}

// AddProjectManager adds a manager to a project
func (s *TimePermissionService) AddProjectManager(projectID int, managerType string, managerID, grantedBy int) (*models.TimeProjectManager, error) {
	if managerType != "user" && managerType != "group" {
		return nil, fmt.Errorf("invalid manager_type: must be 'user' or 'group'")
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO time_project_managers (project_id, manager_type, manager_id, granted_by)
		VALUES (?, ?, ?, ?) RETURNING id
	`, projectID, managerType, managerID, grantedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("error adding project manager: %w", err)
	}

	// Return the created manager with joined data
	managers, err := s.GetProjectManagers(projectID)
	if err != nil {
		return nil, err
	}
	for _, m := range managers {
		if m.ID == int(id) {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("manager not found after insert")
}

// RemoveProjectManager removes a manager from a project
func (s *TimePermissionService) RemoveProjectManager(managerID int) error {
	_, err := s.db.ExecWrite(`DELETE FROM time_project_managers WHERE id = ?`, managerID)
	if err != nil {
		return fmt.Errorf("error removing project manager: %w", err)
	}
	return nil
}

// AddProjectMember adds a member to a project
func (s *TimePermissionService) AddProjectMember(projectID int, memberType string, memberID, grantedBy int) (*models.TimeProjectMember, error) {
	if memberType != "user" && memberType != "group" {
		return nil, fmt.Errorf("invalid member_type: must be 'user' or 'group'")
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO time_project_members (project_id, member_type, member_id, granted_by)
		VALUES (?, ?, ?, ?) RETURNING id
	`, projectID, memberType, memberID, grantedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("error adding project member: %w", err)
	}

	// Return the created member with joined data
	members, err := s.GetProjectMembers(projectID)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if m.ID == int(id) {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("member not found after insert")
}

// RemoveProjectMember removes a member from a project
func (s *TimePermissionService) RemoveProjectMember(memberID int) error {
	_, err := s.db.ExecWrite(`DELETE FROM time_project_members WHERE id = ?`, memberID)
	if err != nil {
		return fmt.Errorf("error removing project member: %w", err)
	}
	return nil
}
