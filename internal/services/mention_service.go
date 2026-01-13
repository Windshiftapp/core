package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// MentionPattern matches @username or @"Display Name" patterns
// Supports: @username, @"John Doe"
var MentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)|@"([^"]+)"`)

// ProcessMentionsParams contains parameters for processing mentions
type ProcessMentionsParams struct {
	SourceType  string // "comment" or "item_description"
	SourceID    int
	Content     string
	ItemID      int
	WorkspaceID int
	ActorUserID int
}

// MentionService handles mention extraction, storage, and notification
type MentionService struct {
	db                  database.Database
	notificationService *NotificationService
}

// NewMentionService creates a new mention service
func NewMentionService(db database.Database, notificationService *NotificationService) *MentionService {
	return &MentionService{
		db:                  db,
		notificationService: notificationService,
	}
}

// ExtractMentionIdentifiers parses content and returns list of mention identifiers
func (s *MentionService) ExtractMentionIdentifiers(content string) []string {
	matches := MentionPattern.FindAllStringSubmatch(content, -1)
	identifiers := make([]string, 0)
	seen := make(map[string]bool)

	for _, match := range matches {
		identifier := match[1] // username pattern
		if identifier == "" {
			identifier = match[2] // "Display Name" pattern
		}

		if identifier != "" && !seen[identifier] {
			seen[identifier] = true
			identifiers = append(identifiers, identifier)
		}
	}

	return identifiers
}

// resolveUserIdentifier looks up a user by username or display name
// Returns userID, displayName, error
func (s *MentionService) resolveUserIdentifier(identifier string) (int, string, error) {
	var userID int
	var firstName, lastName string

	// Try username first (exact match, case insensitive)
	err := s.db.QueryRow(`
		SELECT id, first_name, last_name
		FROM users
		WHERE LOWER(username) = LOWER(?) AND is_active = true
	`, identifier).Scan(&userID, &firstName, &lastName)

	if err == nil {
		displayName := strings.TrimSpace(firstName + " " + lastName)
		return userID, displayName, nil
	}

	if err != sql.ErrNoRows {
		return 0, "", err
	}

	// Try display name match (case insensitive)
	err = s.db.QueryRow(`
		SELECT id, first_name, last_name
		FROM users
		WHERE LOWER(first_name || ' ' || last_name) = LOWER(?) AND is_active = true
	`, identifier).Scan(&userID, &firstName, &lastName)

	if err == nil {
		displayName := strings.TrimSpace(firstName + " " + lastName)
		return userID, displayName, nil
	}

	if err == sql.ErrNoRows {
		return 0, "", nil // User not found, not an error
	}

	return 0, "", err
}

// ProcessMentions handles mention diff and creates/removes mention records
func (s *MentionService) ProcessMentions(params ProcessMentionsParams) error {
	slog.Debug("Processing mentions", slog.String("component", "mentions"), slog.String("source_type", params.SourceType), slog.Int("source_id", params.SourceID), slog.Int("item_id", params.ItemID))

	// Extract current mentions from content
	identifiers := s.ExtractMentionIdentifiers(params.Content)

	// Resolve identifiers to user IDs
	currentMentions := make(map[int]string) // userID -> displayName
	for _, identifier := range identifiers {
		userID, displayName, err := s.resolveUserIdentifier(identifier)
		if err != nil {
			slog.Error("Error resolving identifier", slog.String("component", "mentions"), slog.String("identifier", identifier), slog.Any("error", err))
			continue
		}
		if userID == 0 {
			continue // Unknown user, skip
		}
		// Skip self-mentions
		if userID == params.ActorUserID {
			continue
		}
		currentMentions[userID] = displayName
	}

	// Get existing mentions for this source
	existingMentions, err := s.getExistingMentions(params.SourceType, params.SourceID)
	if err != nil {
		return fmt.Errorf("failed to get existing mentions: %w", err)
	}

	existingIDs := make(map[int]bool)
	for _, m := range existingMentions {
		existingIDs[m.MentionedUserID] = true
	}

	// Track new mentions for notifications
	newMentions := make([]struct {
		userID      int
		displayName string
	}, 0)

	// New mentions (in current but not in existing)
	for userID, displayName := range currentMentions {
		if !existingIDs[userID] {
			slog.Debug("Creating new mention", slog.String("component", "mentions"), slog.Int("user_id", userID), slog.String("source_type", params.SourceType), slog.Int("source_id", params.SourceID))

			err := s.createMention(&models.Mention{
				SourceType:               params.SourceType,
				SourceID:                 params.SourceID,
				MentionedUserID:          userID,
				ItemID:                   params.ItemID,
				WorkspaceID:              params.WorkspaceID,
				CreatedBy:                params.ActorUserID,
				MentionedUserDisplayName: displayName,
			})
			if err != nil {
				slog.Error("Error creating mention", slog.String("component", "mentions"), slog.Int("user_id", userID), slog.Any("error", err))
				continue
			}

			newMentions = append(newMentions, struct {
				userID      int
				displayName string
			}{userID, displayName})
		}
	}

	// Removed mentions (in existing but not in current)
	for _, existingMention := range existingMentions {
		if _, exists := currentMentions[existingMention.MentionedUserID]; !exists {
			slog.Debug("Removing mention", slog.String("component", "mentions"), slog.Int("mention_id", existingMention.ID), slog.Int("user_id", existingMention.MentionedUserID))

			_, err := s.db.ExecWrite(`DELETE FROM mentions WHERE id = ?`, existingMention.ID)
			if err != nil {
				slog.Error("Error deleting mention", slog.String("component", "mentions"), slog.Int("mention_id", existingMention.ID), slog.Any("error", err))
			}
		}
	}

	// Emit notifications for new mentions (skip for personal workspaces)
	isPersonal, err := s.isPersonalWorkspace(params.WorkspaceID)
	if err != nil {
		slog.Error("Error checking if workspace is personal", slog.String("component", "mentions"), slog.Any("error", err))
	}

	if !isPersonal {
		for _, mention := range newMentions {
			s.emitMentionNotification(params, mention.userID, mention.displayName)
		}
	} else {
		slog.Debug("Skipping notifications for personal workspace", slog.String("component", "mentions"), slog.Int("workspace_id", params.WorkspaceID))
	}

	slog.Debug("ProcessMentions completed", slog.String("component", "mentions"), slog.Int("created", len(newMentions)), slog.Int("removed", countRemovedMentions(existingMentions, currentMentions)))

	return nil
}

// countRemovedMentions counts how many existing mentions were removed
func countRemovedMentions(existingMentions []models.Mention, currentMentions map[int]string) int {
	count := 0
	for _, m := range existingMentions {
		if _, exists := currentMentions[m.MentionedUserID]; !exists {
			count++
		}
	}
	return count
}

// isPersonalWorkspace checks if the workspace is a personal workspace
func (s *MentionService) isPersonalWorkspace(workspaceID int) (bool, error) {
	var isPersonal bool
	err := s.db.QueryRow(`SELECT is_personal FROM workspaces WHERE id = ?`, workspaceID).Scan(&isPersonal)
	if err != nil {
		return false, err
	}
	return isPersonal, nil
}

// getExistingMentions retrieves existing mentions for a source
func (s *MentionService) getExistingMentions(sourceType string, sourceID int) ([]models.Mention, error) {
	rows, err := s.db.Query(`
		SELECT id, mentioned_user_id, mentioned_user_display_name
		FROM mentions
		WHERE source_type = ? AND source_id = ?
	`, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mentions []models.Mention
	for rows.Next() {
		var m models.Mention
		if err := rows.Scan(&m.ID, &m.MentionedUserID, &m.MentionedUserDisplayName); err != nil {
			return nil, err
		}
		mentions = append(mentions, m)
	}

	return mentions, rows.Err()
}

// createMention inserts a new mention record
func (s *MentionService) createMention(m *models.Mention) error {
	_, err := s.db.ExecWrite(`
		INSERT INTO mentions (
			source_type, source_id, mentioned_user_id, item_id, workspace_id,
			created_by, mentioned_user_display_name, notification_sent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, false, ?)
	`, m.SourceType, m.SourceID, m.MentionedUserID, m.ItemID, m.WorkspaceID,
		m.CreatedBy, m.MentionedUserDisplayName, time.Now())

	return err
}

// emitMentionNotification sends a notification for a new mention
func (s *MentionService) emitMentionNotification(params ProcessMentionsParams, mentionedUserID int, displayName string) {
	if s.notificationService == nil {
		return
	}

	// Get item details for rich notification
	var itemTitle, workspaceKey string
	var workspaceItemNumber int
	err := s.db.QueryRow(`
		SELECT i.title, w.key, i.workspace_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, params.ItemID).Scan(&itemTitle, &workspaceKey, &workspaceItemNumber)
	if err != nil {
		slog.Error("Error fetching item details", slog.String("component", "mentions"), slog.Any("error", err))
		return
	}

	// Get actor name
	var actorFirstName, actorLastName string
	err = s.db.QueryRow(`
		SELECT first_name, last_name FROM users WHERE id = ?
	`, params.ActorUserID).Scan(&actorFirstName, &actorLastName)
	if err != nil {
		slog.Error("Error fetching actor name", slog.String("component", "mentions"), slog.Any("error", err))
		actorFirstName = "Someone"
	}
	actorName := strings.TrimSpace(actorFirstName + " " + actorLastName)

	itemKey := fmt.Sprintf("%s-%d", workspaceKey, workspaceItemNumber)

	// Determine source type description
	sourceTypeDesc := "content"
	if params.SourceType == "comment" {
		sourceTypeDesc = "a comment"
	} else if params.SourceType == "item_description" {
		sourceTypeDesc = "the description"
	}

	// Use AssigneeID to target the mentioned user
	s.notificationService.EmitEvent(&NotificationEvent{
		EventType:   models.EventMention,
		WorkspaceID: params.WorkspaceID,
		ActorUserID: params.ActorUserID,
		ItemID:      params.ItemID,
		AssigneeID:  &mentionedUserID, // Target the mentioned user
		Title:       "You were mentioned",
		TemplateData: map[string]interface{}{
			"item.title":  itemTitle,
			"item.key":    itemKey,
			"actor.name":  actorName,
			"source.type": sourceTypeDesc,
		},
	})

	// Mark notification as sent
	_, err = s.db.ExecWrite(`
		UPDATE mentions
		SET notification_sent = true
		WHERE source_type = ? AND source_id = ? AND mentioned_user_id = ?
	`, params.SourceType, params.SourceID, mentionedUserID)
	if err != nil {
		slog.Error("Error marking notification as sent", slog.String("component", "mentions"), slog.Any("error", err))
	}
}

// DeleteMentionsForSource removes all mentions for a source (called when comment/item is deleted)
func (s *MentionService) DeleteMentionsForSource(sourceType string, sourceID int) error {
	_, err := s.db.ExecWrite(`
		DELETE FROM mentions WHERE source_type = ? AND source_id = ?
	`, sourceType, sourceID)
	return err
}
