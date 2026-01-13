package services

import (
	"fmt"
	"windshift/internal/database"
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

// ResolveProjectName returns the name for a project ID
func (s *IDResolverService) ResolveProjectName(id int) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM projects WHERE id = ?`, id).Scan(&name)
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
	var workspaceKey string
	var itemNumber int
	err := s.db.QueryRow(`
		SELECT w.key, i.workspace_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&workspaceKey, &itemNumber)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s-%d", workspaceKey, itemNumber)
}
