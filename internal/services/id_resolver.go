package services

import (
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// IDResolverService provides ID-to-name resolution for various entities
type IDResolverService struct {
	db database.Database
}

// NewIDResolverService creates a new ID resolver service
func NewIDResolverService(db database.Database) *IDResolverService {
	return &IDResolverService{db: db}
}

// ResolveUserName returns the full name (or username) for a user ID
func (s *IDResolverService) ResolveUserName(id int) string {
	var name string
	err := s.db.QueryRow(`
		SELECT COALESCE(first_name || ' ' || last_name, username)
		FROM users
		WHERE id = ?
	`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveTeamName returns the name for a team ID
func (s *IDResolverService) ResolveTeamName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM teams WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolvePriorityName returns the name for a priority ID
func (s *IDResolverService) ResolvePriorityName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM priorities WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveStatusName returns the name for a status ID
func (s *IDResolverService) ResolveStatusName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM statuses WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveMilestoneName returns the name for a milestone ID
func (s *IDResolverService) ResolveMilestoneName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM milestones WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveProjectName returns the name for a project ID.
// items.project_id FKs to time_projects(id); the legacy `projects` table is
// unused in production.
func (s *IDResolverService) ResolveProjectName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM time_projects WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveItemTypeName returns the name for an item type ID
func (s *IDResolverService) ResolveItemTypeName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM item_types WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// ResolveItemKey returns the item key in "WORKSPACE-ID" format for an item ID
func (s *IDResolverService) ResolveItemKey(id int) string {
	key, err := repository.NewItemRepository(s.db).GetItemKey(id)
	if err != nil {
		return ""
	}
	return key
}

// ResolveUserNames maps user IDs to display names in one read, preferring the
// trimmed full name, then username, then email. IDs without a matching row are
// absent from the map.
func (s *IDResolverService) ResolveUserNames(ids map[int]struct{}) map[int]string {
	names := map[int]string{}
	if len(ids) == 0 {
		return names
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT id, COALESCE(NULLIF(TRIM(first_name || ' ' || last_name), ''), username, email, '') FROM users WHERE id IN (%s)`,
		strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return names
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name string
		if rows.Scan(&id, &name) == nil {
			names[id] = name
		}
	}
	if err := rows.Err(); err != nil {
		return names
	}
	return names
}

// ResolvePriorityIDByName returns the priority ID whose name matches
// case-insensitively, or nil when there is no match.
func (s *IDResolverService) ResolvePriorityIDByName(name string) (*int, error) {
	var id int
	err := s.db.QueryRow("SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)", name).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("priority %q not found", name)
	}
	return &id, nil
}
