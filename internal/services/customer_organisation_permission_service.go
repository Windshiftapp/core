package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CustomerOrganisationPermissionService handles per-customer-organisation
// access checks. Visibility model: open by default; any row in
// customer_organisation_members converts the org into a whitelist.
// Managers always have full access and can manage the member list.
type CustomerOrganisationPermissionService struct {
	db                    database.Database
	permissionService     *PermissionService
	timePermissionService *TimePermissionService
}

// NewCustomerOrganisationPermissionService creates the service.
func NewCustomerOrganisationPermissionService(db database.Database, permissionService *PermissionService, timePermissionService *TimePermissionService) *CustomerOrganisationPermissionService {
	return &CustomerOrganisationPermissionService{
		db:                    db,
		permissionService:     permissionService,
		timePermissionService: timePermissionService,
	}
}

// HasCustomersManagePermission returns true for system admins and holders of
// customers.manage (or project.manage). Delegates to TimePermissionService so
// the rule stays in one place.
func (s *CustomerOrganisationPermissionService) HasCustomersManagePermission(userID int) (bool, error) {
	return s.timePermissionService.HasCustomersManagePermission(userID)
}

// HasMembers reports whether any whitelist rows exist for the org.
func (s *CustomerOrganisationPermissionService) HasMembers(customerOrgID int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM customer_organisation_members WHERE customer_organisation_id = ?
	`, customerOrgID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error checking customer organisation members: %w", err)
	}
	return count > 0, nil
}

// HasManagers reports whether any manager rows exist for the org.
func (s *CustomerOrganisationPermissionService) HasManagers(customerOrgID int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM customer_organisation_managers WHERE customer_organisation_id = ?
	`, customerOrgID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error checking customer organisation managers: %w", err)
	}
	return count > 0, nil
}

// IsManager reports whether the user manages this org (direct or via group).
// Holders of customers.manage are always considered managers.
func (s *CustomerOrganisationPermissionService) IsManager(userID, customerOrgID int) (bool, error) {
	hasGlobal, err := s.HasCustomersManagePermission(userID)
	if err != nil {
		return false, err
	}
	if hasGlobal {
		return true, nil
	}

	var directAssigned bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_organisation_managers
			WHERE customer_organisation_id = ? AND manager_type = 'user' AND manager_id = ?
		)
	`, customerOrgID, userID).Scan(&directAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking direct manager assignment: %w", err)
	}
	if directAssigned {
		return true, nil
	}

	var groupAssigned bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_organisation_managers com
			JOIN group_members gm ON com.manager_id = gm.group_id
			WHERE com.customer_organisation_id = ? AND com.manager_type = 'group' AND gm.user_id = ?
		)
	`, customerOrgID, userID).Scan(&groupAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking group manager assignment: %w", err)
	}
	return groupAssigned, nil
}

// isMember reports whether the user is in the whitelist (direct or via group).
func (s *CustomerOrganisationPermissionService) isMember(userID, customerOrgID int) (bool, error) {
	var directAssigned bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_organisation_members
			WHERE customer_organisation_id = ? AND member_type = 'user' AND member_id = ?
		)
	`, customerOrgID, userID).Scan(&directAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking direct member assignment: %w", err)
	}
	if directAssigned {
		return true, nil
	}

	var groupAssigned bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_organisation_members com
			JOIN group_members gm ON com.member_id = gm.group_id
			WHERE com.customer_organisation_id = ? AND com.member_type = 'group' AND gm.user_id = ?
		)
	`, customerOrgID, userID).Scan(&groupAssigned)
	if err != nil {
		return false, fmt.Errorf("error checking group member assignment: %w", err)
	}
	return groupAssigned, nil
}

// CanView resolves the visibility rule:
// manager → yes; no whitelist rows → yes; otherwise must be in the whitelist.
func (s *CustomerOrganisationPermissionService) CanView(userID, customerOrgID int) (bool, error) {
	isManager, err := s.IsManager(userID, customerOrgID)
	if err != nil {
		return false, err
	}
	if isManager {
		return true, nil
	}

	hasMembers, err := s.HasMembers(customerOrgID)
	if err != nil {
		return false, err
	}
	if !hasMembers {
		return true, nil
	}

	return s.isMember(userID, customerOrgID)
}

// GetAccessible returns the customer-organisation IDs the user can see.
// A nil return means "all accessible" (global customers.manage holder).
func (s *CustomerOrganisationPermissionService) GetAccessible(userID int) ([]int, error) {
	hasGlobal, err := s.HasCustomersManagePermission(userID)
	if err != nil {
		return nil, err
	}
	if hasGlobal {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT DISTINCT c.id FROM customer_organisations c
		WHERE
			NOT EXISTS (SELECT 1 FROM customer_organisation_members WHERE customer_organisation_id = c.id)
			OR EXISTS (SELECT 1 FROM customer_organisation_managers WHERE customer_organisation_id = c.id AND manager_type = 'user' AND manager_id = ?)
			OR EXISTS (
				SELECT 1 FROM customer_organisation_managers com
				JOIN group_members gm ON com.manager_id = gm.group_id
				WHERE com.customer_organisation_id = c.id AND com.manager_type = 'group' AND gm.user_id = ?
			)
			OR EXISTS (SELECT 1 FROM customer_organisation_members WHERE customer_organisation_id = c.id AND member_type = 'user' AND member_id = ?)
			OR EXISTS (
				SELECT 1 FROM customer_organisation_members com
				JOIN group_members gm ON com.member_id = gm.group_id
				WHERE com.customer_organisation_id = c.id AND com.member_type = 'group' AND gm.user_id = ?
			)
	`, userID, userID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("error getting accessible customer organisations: %w", err)
	}
	defer rows.Close()

	// Always return a non-nil slice when we're in filtered mode so callers can
	// distinguish "user has global access (nil)" from "user has access to no
	// orgs (empty slice)".
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning customer organisation ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating customer organisations: %w", err)
	}
	return ids, nil
}

// GetManagers returns all managers for an org, joined with user/group display data.
func (s *CustomerOrganisationPermissionService) GetManagers(customerOrgID int) ([]models.CustomerOrganisationManager, error) {
	rows, err := s.db.Query(`
		SELECT com.id, com.customer_organisation_id, com.manager_type, com.manager_id, com.granted_by, com.granted_at,
		       CASE
		           WHEN com.manager_type = 'user' THEN u.first_name || ' ' || u.last_name
		           WHEN com.manager_type = 'group' THEN g.name
		       END as manager_name,
		       CASE
		           WHEN com.manager_type = 'user' THEN u.email
		           ELSE ''
		       END as manager_email
		FROM customer_organisation_managers com
		LEFT JOIN users u ON com.manager_type = 'user' AND com.manager_id = u.id
		LEFT JOIN groups g ON com.manager_type = 'group' AND com.manager_id = g.id
		WHERE com.customer_organisation_id = ?
		ORDER BY com.granted_at DESC
	`, customerOrgID)
	if err != nil {
		return nil, fmt.Errorf("error getting customer organisation managers: %w", err)
	}
	defer rows.Close()

	var managers []models.CustomerOrganisationManager
	for rows.Next() {
		var m models.CustomerOrganisationManager
		var name, email sql.NullString
		if err := rows.Scan(&m.ID, &m.CustomerOrganisationID, &m.ManagerType, &m.ManagerID, &m.GrantedBy, &m.GrantedAt, &name, &email); err != nil {
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

// GetMembers returns all members for an org, joined with user/group display data.
func (s *CustomerOrganisationPermissionService) GetMembers(customerOrgID int) ([]models.CustomerOrganisationMember, error) {
	rows, err := s.db.Query(`
		SELECT com.id, com.customer_organisation_id, com.member_type, com.member_id, com.granted_by, com.granted_at,
		       CASE
		           WHEN com.member_type = 'user' THEN u.first_name || ' ' || u.last_name
		           WHEN com.member_type = 'group' THEN g.name
		       END as member_name,
		       CASE
		           WHEN com.member_type = 'user' THEN u.email
		           ELSE ''
		       END as member_email
		FROM customer_organisation_members com
		LEFT JOIN users u ON com.member_type = 'user' AND com.member_id = u.id
		LEFT JOIN groups g ON com.member_type = 'group' AND com.member_id = g.id
		WHERE com.customer_organisation_id = ?
		ORDER BY com.granted_at DESC
	`, customerOrgID)
	if err != nil {
		return nil, fmt.Errorf("error getting customer organisation members: %w", err)
	}
	defer rows.Close()

	var members []models.CustomerOrganisationMember
	for rows.Next() {
		var m models.CustomerOrganisationMember
		var name, email sql.NullString
		if err := rows.Scan(&m.ID, &m.CustomerOrganisationID, &m.MemberType, &m.MemberID, &m.GrantedBy, &m.GrantedAt, &name, &email); err != nil {
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

// AddManager inserts a manager row, then returns the joined record.
func (s *CustomerOrganisationPermissionService) AddManager(customerOrgID int, managerType string, managerID, grantedBy int) (*models.CustomerOrganisationManager, error) {
	if managerType != "user" && managerType != "group" {
		return nil, fmt.Errorf("invalid manager_type: must be 'user' or 'group'")
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO customer_organisation_managers (customer_organisation_id, manager_type, manager_id, granted_by)
		VALUES (?, ?, ?, ?) RETURNING id
	`, customerOrgID, managerType, managerID, grantedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("error adding customer organisation manager: %w", err)
	}

	managers, err := s.GetManagers(customerOrgID)
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

// RemoveManager deletes a manager row by id.
func (s *CustomerOrganisationPermissionService) RemoveManager(managerID int) error {
	_, err := s.db.ExecWrite(`DELETE FROM customer_organisation_managers WHERE id = ?`, managerID)
	if err != nil {
		return fmt.Errorf("error removing customer organisation manager: %w", err)
	}
	return nil
}

// AddMember inserts a member row, then returns the joined record.
func (s *CustomerOrganisationPermissionService) AddMember(customerOrgID int, memberType string, memberID, grantedBy int) (*models.CustomerOrganisationMember, error) {
	if memberType != "user" && memberType != "group" {
		return nil, fmt.Errorf("invalid member_type: must be 'user' or 'group'")
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO customer_organisation_members (customer_organisation_id, member_type, member_id, granted_by)
		VALUES (?, ?, ?, ?) RETURNING id
	`, customerOrgID, memberType, memberID, grantedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("error adding customer organisation member: %w", err)
	}

	members, err := s.GetMembers(customerOrgID)
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

// RemoveMember deletes a member row by id.
func (s *CustomerOrganisationPermissionService) RemoveMember(memberID int) error {
	_, err := s.db.ExecWrite(`DELETE FROM customer_organisation_members WHERE id = ?`, memberID)
	if err != nil {
		return fmt.Errorf("error removing customer organisation member: %w", err)
	}
	return nil
}
