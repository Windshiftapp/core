package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/services"

	"github.com/google/uuid"
)

// ensureUsers matches or creates users for import.
// Returns:
//   - userMap: Jira accountID → Windshift user ID
//   - usernameMap: Jira accountID → Windshift username (for ADF mention resolution)
//
// Fetches missing emails via the Jira API when needed.
func (h *JiraImportHandler) ensureUsers(ctx context.Context, jobID string, users []JiraUserSummary, client jira.Client) (map[string]int, map[string]string, error) { //nolint:unparam,gocritic // error return kept for API consistency; named returns aren't worth the noise here
	result := make(map[string]int)
	usernames := make(map[string]string)

	// First pass: fetch missing emails via API
	for i := range users {
		if users[i].AccountID == "" {
			continue
		}
		if users[i].Email != "" {
			continue // Already have email
		}

		// Try to fetch email via API
		email, err := client.GetUserEmail(ctx, users[i].AccountID)
		if err != nil {
			slog.Debug("Failed to fetch email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.Any("error", err))
		} else if email != "" {
			users[i].Email = email
			slog.Debug("Fetched email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.String("email", email))
		}
	}

	// Second pass: create/match users
	for _, u := range users {
		// Skip users without account ID
		if u.AccountID == "" {
			continue
		}

		// Synthesize an email for accounts where Jira Cloud's GDPR rules hide
		// the real one. Without this every emailless account would be skipped
		// and downstream fields (reporter, comment author, mentions, custom
		// user-pickers) would silently lose their user reference. The synthetic
		// address is deterministic per accountID so re-imports map to the same
		// inactive user instead of creating a new ghost each run.
		if u.Email == "" {
			u.Email = syntheticEmailForAccount(u.AccountID)
		}

		// Check if we already have a mapping for this user in this job
		var existingUserID int
		var existingUsername string
		err := h.db.QueryRow(`
			SELECT u.id, u.username
			FROM jira_import_user_mappings m
			JOIN users u ON u.id = m.windshift_user_id
			WHERE m.job_id = ? AND m.jira_account_id = ?
		`, jobID, u.AccountID).Scan(&existingUserID, &existingUsername)
		if err == nil {
			result[u.AccountID] = existingUserID
			usernames[u.AccountID] = existingUsername
			continue
		}

		// Try to find existing Windshift user by email
		var userID int
		var existingByEmailUsername string
		if u.Email != "" {
			err = h.db.QueryRow(`SELECT id, username FROM users WHERE email = ?`, u.Email).Scan(&userID, &existingByEmailUsername)
			if err == nil {
				// Found existing user
				result[u.AccountID] = userID
				usernames[u.AccountID] = existingByEmailUsername
				h.recordUserMapping(jobID, u, userID, false)
				continue
			}
		}

		// Create new inactive user
		firstName, lastName := parseDisplayName(u.DisplayName)
		username := generateUsername(u.Email, u.DisplayName)

		var newUserID int64
		err = h.db.QueryRow(`
			INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, created_at, updated_at)
			VALUES (?, ?, ?, ?, false, ?, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, u.Email, username, firstName, lastName, u.AvatarURL).Scan(&newUserID)
		if err != nil {
			slog.Error("Failed to create user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Any("error", err))
			continue
		}

		result[u.AccountID] = int(newUserID)
		usernames[u.AccountID] = username
		h.recordUserMapping(jobID, u, int(newUserID), true)

		slog.Debug("Created user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Int64("userID", newUserID))
	}

	return result, usernames, nil
}

// recordUserMapping stores a Jira user to Windshift user mapping
func (h *JiraImportHandler) recordUserMapping(jobID string, user JiraUserSummary, windshiftUserID int, wasCreated bool) {
	_, err := h.db.ExecWrite(`
		INSERT INTO jira_import_user_mappings (job_id, jira_account_id, jira_email, jira_display_name, windshift_user_id, was_created)
		VALUES (?, ?, ?, ?, ?, ?)
	`, jobID, user.AccountID, user.Email, user.DisplayName, windshiftUserID, wasCreated)
	if err != nil {
		slog.Error("Failed to record user mapping", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// ensureLabel returns the workspace-scoped label ID for name, creating the
// label row if it doesn't exist yet. Color/created_at/updated_at fall back to
// the schema defaults.
func (h *JiraImportHandler) ensureLabel(workspaceID int, name string) (int, error) {
	var id int
	err := h.db.QueryRow(
		`SELECT id FROM labels WHERE workspace_id = ? AND name = ?`,
		workspaceID, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var newID int64
	if err := h.db.QueryRow(
		`INSERT INTO labels (name, workspace_id) VALUES (?, ?) RETURNING id`,
		name, workspaceID,
	).Scan(&newID); err != nil {
		return 0, err
	}
	return int(newID), nil
}

// importLabels ensures each Jira label exists in the workspace and links it to
// the imported item. Duplicates within the input slice are silently collapsed
// so we don't trip the (item_id, label_id) UNIQUE constraint.
func (h *JiraImportHandler) importLabels(workspaceID, itemID int, labels []string) {
	if len(labels) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(labels))
	for _, raw := range labels {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		labelID, err := h.ensureLabel(workspaceID, name)
		if err != nil {
			slog.Error("Failed to ensure label",
				slog.String("component", "jira"),
				slog.String("label", name),
				slog.Int("workspaceID", workspaceID),
				slog.Any("error", err))
			continue
		}
		if _, err := h.db.ExecWrite(
			`INSERT INTO item_labels (item_id, label_id) VALUES (?, ?)`,
			itemID, labelID,
		); err != nil {
			slog.Error("Failed to link label to item",
				slog.String("component", "jira"),
				slog.String("label", name),
				slog.Int("itemID", itemID),
				slog.Int("labelID", labelID),
				slog.Any("error", err))
		}
	}
}

// importedDummyUserEmail is the well-known address used for the shared
// fallback user that owns comments whose Jira author can't be resolved (e.g.
// deleted Jira accounts, comments on Service Desk requests from removed
// portal customers). One row across all imports — re-imports don't pile up
// dummy rows.
const importedDummyUserEmail = "imported-user@jira-import.invalid"

// ensureImportedDummyUser returns the ID of the shared fallback user, creating
// it on first use. The user is inactive and password-locked so the row never
// becomes a real account. Concurrent imports that race on creation are handled
// by re-SELECTing after a UNIQUE-violating INSERT.
func (h *JiraImportHandler) ensureImportedDummyUser() (int, error) {
	var id int
	err := h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, importedDummyUserEmail).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	var newID int64
	err = h.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, requires_password_reset, created_at, updated_at)
		VALUES (?, ?, ?, ?, false, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, importedDummyUserEmail, "jira-imported-user", "Imported", "(Jira)").Scan(&newID)
	if err == nil {
		return int(newID), nil
	}

	// Lost a race with another import — re-fetch.
	if e := h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, importedDummyUserEmail).Scan(&id); e == nil {
		return id, nil
	}
	return 0, err
}

// syntheticEmailForAccount produces a deterministic, RFC-safe email for a Jira
// account whose real email is hidden (GDPR-restricted). Colons that appear in
// Cloud accountIDs aren't legal in the local-part of an address, so we map
// them to hyphens. The `.invalid` TLD is reserved by RFC 2606, guaranteeing
// no collision with real domains.
// intPtrToSlice returns a single-element slice when p is non-nil, or nil
// otherwise. Used to bridge legacy single-milestone callers into the multi
// milestone API.
func intPtrToSlice(p *int) []int {
	if p == nil {
		return nil
	}
	return []int{*p}
}

func syntheticEmailForAccount(accountID string) string {
	safe := strings.ReplaceAll(accountID, ":", "-")
	return safe + "@imported.invalid"
}

// parseDisplayName splits a display name into first and last name
func parseDisplayName(displayName string) (firstName, lastName string) {
	parts := strings.SplitN(strings.TrimSpace(displayName), " ", 2)
	if len(parts) >= 1 {
		firstName = parts[0]
	}
	if len(parts) >= 2 {
		lastName = parts[1]
	}
	if firstName == "" {
		firstName = "Imported"
	}
	if lastName == "" {
		lastName = "User"
	}
	return
}

// generateUsername creates a unique username from email or display name
func generateUsername(email, displayName string) string {
	// Try to use email prefix first
	if email != "" {
		parts := strings.Split(email, "@")
		if len(parts) > 0 && parts[0] != "" {
			return strings.ToLower(parts[0])
		}
	}
	// Fall back to display name
	if displayName != "" {
		return strings.ToLower(strings.ReplaceAll(displayName, " ", "."))
	}
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

// collectUsersFromCustomField extracts users from a custom field value
func collectUsersFromCustomField(value interface{}, fieldType string,
	existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	switch fieldType {
	case "user":
		if userObj, ok := value.(map[string]interface{}); ok {
			addUserFromObject(userObj, existingMap, usersToProcess, seen)
		}
	case "users":
		if users, ok := value.([]interface{}); ok {
			for _, u := range users {
				if userObj, ok := u.(map[string]interface{}); ok {
					addUserFromObject(userObj, existingMap, usersToProcess, seen)
				}
			}
		}
	}
}

// addUserFromObject extracts user data from a Jira user object and adds it to the processing list
func addUserFromObject(userObj map[string]interface{}, existingMap map[string]int,
	usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	accountID, _ := userObj["accountId"].(string)
	if accountID == "" {
		return
	}
	if _, exists := existingMap[accountID]; exists {
		return
	}
	if seen[accountID] {
		return
	}

	email, _ := userObj["emailAddress"].(string)
	displayName, _ := userObj["displayName"].(string)
	avatarURL := ""
	if avatars, ok := userObj["avatarUrls"].(map[string]interface{}); ok {
		avatarURL, _ = avatars["48x48"].(string)
	}

	*usersToProcess = append(*usersToProcess, JiraUserSummary{
		AccountID:   accountID,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	})
	seen[accountID] = true
}

// extractCustomFieldValue resolves a single Jira custom field into the value
// that belongs in the item's custom_field_values JSON bag. Returns (nil, false)
// for any skip path so callers can use the same pattern for all types:
//
//	if v, ok := extractCustomFieldValue(mapping, &issue.Fields, userMap); ok { ... }
//
// Phase 0 only handles user/users mappings (the existing pre-refactor behavior);
// additional WindshiftType cases are added in Phase 1.1 (B01/B07/B25).
func extractCustomFieldValue(mapping CustomFieldMapping, fields *jira.JiraIssueFields, userMap map[string]int) (any, bool) {
	if mapping.Action == "skip" {
		return nil, false
	}
	// Only process user/users types for now
	if mapping.WindshiftType != "user" && mapping.WindshiftType != "users" {
		return nil, false
	}
	if fields == nil || fields.CustomFields == nil {
		return nil, false
	}
	value, exists := fields.CustomFields[mapping.JiraID]
	if !exists || value == nil {
		return nil, false
	}

	switch mapping.WindshiftType {
	case "user":
		// Single user picker
		userObj, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}
		accountID, ok := userObj["accountId"].(string)
		if !ok {
			return nil, false
		}
		uid, ok := userMap[accountID]
		if !ok {
			return nil, false
		}
		return uid, true
	case "users":
		// Multi-user picker (like Approvers)
		users, ok := value.([]interface{})
		if !ok {
			return nil, false
		}
		var userIDs []int
		for _, u := range users {
			userObj, ok := u.(map[string]interface{})
			if !ok {
				continue
			}
			accountID, ok := userObj["accountId"].(string)
			if !ok {
				continue
			}
			if uid, ok := userMap[accountID]; ok {
				userIDs = append(userIDs, uid)
			}
		}
		if len(userIDs) == 0 {
			return nil, false
		}
		return userIDs, true
	}
	return nil, false
}

// importIssue imports a single Jira issue as a Windshift work item
func (h *JiraImportHandler) importIssue(ctx context.Context, jobID string, workspaceID int, issue *jira.JiraIssue, statusMap, itemTypeMap, userMap map[string]int, usernameMap map[string]string, versionMap map[string]int, customFieldMappings []CustomFieldMapping, client jira.Client, progress *ImportProgress) error {
	mentionResolver := jira.MentionResolver(func(accountID string) string {
		return usernameMap[accountID]
	})
	// Get mapped status and item type (use nil instead of 0 for missing mappings)
	var statusID *int
	if issue.Fields.Status != nil {
		if sid, ok := statusMap[issue.Fields.Status.ID]; ok {
			statusID = &sid
		}
	}

	var itemTypeID *int
	if issue.Fields.IssueType != nil {
		if tid, ok := itemTypeMap[issue.Fields.IssueType.ID]; ok {
			itemTypeID = &tid
		}
	}

	// Map assignee and reporter
	var assigneeID *int
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Assignee.GetIdentifier()]; ok {
			assigneeID = &uid
		}
	}

	var reporterID *int
	if issue.Fields.Reporter != nil && issue.Fields.Reporter.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Reporter.GetIdentifier()]; ok {
			reporterID = &uid
		}
	}

	// Creator (immutable in Jira) is distinct from Reporter (mutable). Preserve
	// it on items.creator_id so audit views in Windshift reflect who originated
	// the issue, not who happened to run the import.
	var creatorID *int
	if issue.Fields.Creator != nil && issue.Fields.Creator.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Creator.GetIdentifier()]; ok {
			creatorID = &uid
		}
	}

	// Map fixVersion to milestone (use first version)
	var milestoneID *int
	if len(issue.Fields.FixVersions) > 0 {
		if mid, ok := versionMap[issue.Fields.FixVersions[0].ID]; ok {
			milestoneID = &mid
		}
	}

	// Map priority through the synonym table so Jira-only names (Highest, Lowest,
	// Blocker, Major, Minor, Trivial) land on canonical Windshift priorities
	// instead of falling back to the workspace default.
	var priorityName string
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != "" {
		priorityName = jira.SuggestPriorityMapping(issue.Fields.Priority.Name)
	}

	// Parse due date
	var dueDate *time.Time
	if issue.Fields.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
			dueDate = &parsed
		}
	}

	// Preserve Jira's original timestamps so chronology survives the import.
	// Without this every imported item appears created at import time, which
	// breaks reports, "recent" filters, and the timeline view.
	createdAt := jira.ParseJiraTimestamp(issue.Fields.Created)
	updatedAt := jira.ParseJiraTimestamp(issue.Fields.Updated)

	// Convert description from ADF to markdown, resolving Jira accountIDs to
	// Windshift usernames so MentionService picks up @mentions on import.
	description := ""
	if issue.Fields.Description != nil {
		description = jira.ConvertADFToMarkdownWithUsers(issue.Fields.Description, mentionResolver)
	}

	// Process custom fields (user/users types only for now). Standard top-level
	// fields that have no dedicated Windshift column ride along inside the same
	// JSON bag so reports and exports can still surface them — e.g. Jira's
	// resolutiondate, persisted as `_jira_resolved_at` so the underscore prefix
	// distinguishes it from user-mappable customfield_* keys.
	customFieldValues := make(map[string]interface{})
	if resolved := jira.ParseJiraTimestamp(issue.Fields.Resolved); resolved != nil {
		customFieldValues["_jira_resolved_at"] = resolved.UTC().Format(time.RFC3339)
	}
	for _, mapping := range customFieldMappings {
		if v, ok := extractCustomFieldValue(mapping, &issue.Fields, userMap); ok {
			customFieldValues[mapping.JiraID] = v
		}
	}

	// Serialize custom field values to JSON
	customFieldValuesJSON := ""
	if len(customFieldValues) > 0 {
		if jsonBytes, err := json.Marshal(customFieldValues); err == nil {
			customFieldValuesJSON = string(jsonBytes)
		}
	}

	// Create the work item using centralized service (handles workspace_item_number, frac_index, etc.)
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:           workspaceID,
		Title:                 issue.Fields.Summary,
		Description:           description,
		StatusID:              statusID,
		ItemTypeID:            itemTypeID,
		Priority:              priorityName,
		DueDate:               dueDate,
		AssigneeID:            assigneeID,
		ReporterID:            reporterID,
		CreatorID:             creatorID,
		MilestoneIDs:          intPtrToSlice(milestoneID),
		CustomFieldValuesJSON: customFieldValuesJSON,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Build metadata for the mapping (includes parent key for later linking)
	meta := map[string]interface{}{
		"summary": issue.Fields.Summary,
	}
	// Resolve the parent issue key across the three places Jira encodes it:
	//   1. Fields.Parent — team-managed projects (always populated when present).
	//   2. Fields.Epic   — some Jira Cloud responses surface the epic this way.
	//   3. customfield_* of type gh-epic-link — company-managed projects.
	// Without (3) the importer would lose epic→story relationships on the most
	// common deployment shape; without (2) we'd miss them on Cloud responses.
	parentKey := ""
	switch {
	case issue.Fields.Parent != nil && issue.Fields.Parent.Key != "":
		parentKey = issue.Fields.Parent.Key
	case issue.Fields.Epic != nil && issue.Fields.Epic.Key != "":
		parentKey = issue.Fields.Epic.Key
	default:
		for _, mapping := range customFieldMappings {
			if mapping.JiraType == "com.pyxis.greenhopper.jira:gh-epic-link" {
				if v, ok := issue.Fields.CustomFields[mapping.JiraID].(string); ok && v != "" {
					parentKey = v
				}
				break
			}
		}
	}
	if parentKey != "" {
		meta["parent_key"] = parentKey
	}
	if len(issue.Fields.IssueLinks) > 0 {
		var links []map[string]interface{}
		for _, link := range issue.Fields.IssueLinks {
			entry := map[string]interface{}{}
			if link.Type != nil {
				entry["type_name"] = link.Type.Name
				entry["inward"] = link.Type.Inward
				entry["outward"] = link.Type.Outward
			}
			if link.InwardIssue != nil {
				entry["inward_key"] = link.InwardIssue.Key
			}
			if link.OutwardIssue != nil {
				entry["outward_key"] = link.OutwardIssue.Key
			}
			links = append(links, entry)
		}
		meta["issue_links"] = links
	}

	// Record the mapping
	h.recordMapping(jobID, "item", issue.ID, issue.Key, int(itemID), meta)

	// Attach Jira labels (top-level Fields.Labels, distinct from labels-typed
	// custom fields). Workspace-scoped — same name in different workspaces is
	// independent.
	h.importLabels(workspaceID, int(itemID), issue.Fields.Labels)

	// Import comments for this issue
	h.importComments(jobID, int(itemID), issue, userMap, mentionResolver)

	// Import attachments for this issue
	h.importAttachments(ctx, jobID, int(itemID), issue, userMap, client, progress)

	return nil
}

// ================================================================
// Phase 3: Parent/Hierarchy Linking
// ================================================================

// linkParents sets parent_id on imported items whose Jira issue had a parent field.
// Must be called after all issues for a project are imported so that both
// parent and child exist in jira_import_id_mappings.
func (h *JiraImportHandler) linkParents(jobID string) {
	// Find all item mappings that have a parent_key in metadata
	rows, err := h.db.Query(`
		SELECT windshift_id, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for parent linking", slog.String("component", "jira"), slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type parentLink struct {
		childID   int
		parentKey string
	}
	var links []parentLink

	for rows.Next() {
		var windshiftID int
		var metadataJSON sql.NullString
		if err := rows.Scan(&windshiftID, &metadataJSON); err != nil {
			slog.Warn("failed to scan item mapping row", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}
		if !metadataJSON.Valid {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &meta); err != nil {
			continue
		}
		if parentKey, ok := meta["parent_key"].(string); ok && parentKey != "" {
			links = append(links, parentLink{childID: windshiftID, parentKey: parentKey})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating item mapping rows for parent linking", slog.String("component", "jira"), slog.Any("error", err))
		return
	}

	for _, link := range links {
		// Look up parent's Windshift ID from mappings
		var parentID int
		err := h.db.QueryRow(`
			SELECT windshift_id FROM jira_import_id_mappings
			WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
		`, jobID, link.parentKey).Scan(&parentID)
		if err != nil {
			slog.Debug("Parent not found in import mappings",
				slog.String("component", "jira"),
				slog.String("parentKey", link.parentKey),
				slog.Int("childID", link.childID))
			continue
		}

		// Update the child item's parent_id directly.
		// We cannot use ItemUpdateService here because it requires a valid user ID
		// for history tracking, and the import runs without a user context.
		_, err = h.db.ExecWrite(`UPDATE items SET parent_id = ? WHERE id = ?`, parentID, link.childID)
		if err != nil {
			slog.Error("Failed to set parent_id",
				slog.String("component", "jira"),
				slog.Int("childID", link.childID),
				slog.Int("parentID", parentID),
				slog.Any("error", err))
		}
	}
}

// ================================================================
// Phase 4: Comment Import
// ================================================================

// importComments imports comments from a Jira issue into Windshift
func (h *JiraImportHandler) importComments(jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int, mentionResolver jira.MentionResolver) {
	if issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) == 0 {
		return
	}

	// Create a CommentService without notification/webhook/mention services
	// so bulk import doesn't generate notifications
	commentSvc := services.NewCommentService(h.db)

	// dummyID is fetched lazily — most issues have only resolvable authors,
	// and we don't want to create the row unless we actually need it.
	dummyID := 0
	resolveAuthor := func(c *jira.JiraComment) int {
		if c.Author != nil && c.Author.GetIdentifier() != "" {
			if uid, ok := userMap[c.Author.GetIdentifier()]; ok {
				return uid
			}
		}
		if dummyID == 0 {
			id, err := h.ensureImportedDummyUser()
			if err != nil {
				slog.Error("Failed to ensure imported dummy user",
					slog.String("component", "jira"),
					slog.String("issue", issue.Key),
					slog.Any("error", err))
				return 0
			}
			dummyID = id
		}
		return dummyID
	}

	for _, comment := range issue.Fields.Comment.Comments {
		content := jira.ConvertADFToMarkdownWithUsers(comment.Body, mentionResolver)
		if content == "" {
			continue
		}

		authorID := resolveAuthor(&comment)
		if authorID == 0 {
			// Dummy-user creation failed; skip rather than violate the FK.
			continue
		}

		createdAt := jira.ParseJiraTimestamp(comment.Created)

		result, err := commentSvc.Create(services.CreateCommentParams{
			ItemID:      itemID,
			AuthorID:    authorID,
			Content:     content,
			ActorUserID: authorID,
			CreatedAt:   createdAt,
		})
		if err != nil {
			slog.Error("Failed to import comment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("commentID", comment.ID),
				slog.Any("error", err))
			continue
		}

		h.recordMapping(jobID, "comment", comment.ID, issue.Key, int(result.CommentID), nil)
	}
}

// ================================================================
// Phase 5: Issue Link Import
// ================================================================

// importIssueLinks creates item_links from Jira issue links stored in mapping metadata.
// Must be called after all issues for a project are imported.
func (h *JiraImportHandler) importIssueLinks(jobID string) {
	// Query all item mappings with issue_links metadata
	rows, err := h.db.Query(`
		SELECT windshift_id, jira_key, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for link import", slog.String("component", "jira"), slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type issueLinkInfo struct {
		sourceID  int
		sourceKey string
		links     []map[string]interface{}
	}
	var allLinks []issueLinkInfo

	for rows.Next() {
		var windshiftID int
		var jiraKey string
		var metadataJSON sql.NullString
		if err := rows.Scan(&windshiftID, &jiraKey, &metadataJSON); err != nil {
			slog.Warn("failed to scan item mapping row for link import", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}
		if !metadataJSON.Valid {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &meta); err != nil {
			continue
		}
		linksRaw, ok := meta["issue_links"].([]interface{})
		if !ok || len(linksRaw) == 0 {
			continue
		}
		var links []map[string]interface{}
		for _, l := range linksRaw {
			if m, ok := l.(map[string]interface{}); ok {
				links = append(links, m)
			}
		}
		if len(links) > 0 {
			allLinks = append(allLinks, issueLinkInfo{sourceID: windshiftID, sourceKey: jiraKey, links: links})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating item mapping rows for link import", slog.String("component", "jira"), slog.Any("error", err))
		return
	}

	// Cache link type lookups
	linkTypeCache := make(map[string]int) // link type name -> ID
	linkSvc := services.NewItemLinkService(h.db)

	for _, info := range allLinks {
		for _, link := range info.links {
			typeName, _ := link["type_name"].(string)
			if typeName == "" {
				continue
			}

			// Determine source and target.
			// Outward link: this issue is the source, outward_key is the target.
			// Inward link:  inward_key is the source, this issue is the target.
			// We only build the link from the outward side so an A→B relationship
			// imported from both ends doesn't produce two rows.
			outwardKey, _ := link["outward_key"].(string)
			if outwardKey == "" {
				// Inward-only entry. If the source is in this import we'll catch
				// the same relationship from the source's outward_key in another
				// iteration. If it isn't, we cannot represent the link in
				// Windshift today (no external-reference support yet) — log it
				// so the loss isn't silent.
				inwardKey, _ := link["inward_key"].(string)
				if inwardKey == "" {
					continue
				}
				var sourceID int
				err := h.db.QueryRow(`
					SELECT windshift_id FROM jira_import_id_mappings
					WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
				`, jobID, inwardKey).Scan(&sourceID)
				if errors.Is(err, sql.ErrNoRows) {
					slog.Warn("Dropping inward link from non-imported issue",
						slog.String("component", "jira"),
						slog.String("source", inwardKey),
						slog.String("target", info.sourceKey),
						slog.String("typeName", typeName))
				} else if err != nil {
					slog.Warn("Failed to look up inward link source",
						slog.String("component", "jira"),
						slog.String("source", inwardKey),
						slog.Any("error", err))
				}
				continue
			}

			// Look up target Windshift ID
			var targetID int
			err := h.db.QueryRow(`
				SELECT windshift_id FROM jira_import_id_mappings
				WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
			`, jobID, outwardKey).Scan(&targetID)
			if errors.Is(err, sql.ErrNoRows) {
				// Target wasn't part of this import. Same external-reference
				// limitation as above — log so the drop is observable.
				slog.Warn("Dropping outward link to non-imported issue",
					slog.String("component", "jira"),
					slog.String("source", info.sourceKey),
					slog.String("target", outwardKey),
					slog.String("typeName", typeName))
				continue
			} else if err != nil {
				continue
			}

			// Ensure link type exists
			linkTypeID, ok := linkTypeCache[typeName]
			if !ok {
				linkTypeID, err = h.ensureLinkType(typeName, link)
				if err != nil {
					slog.Error("Failed to ensure link type",
						slog.String("component", "jira"),
						slog.String("typeName", typeName),
						slog.Any("error", err))
					continue
				}
				linkTypeCache[typeName] = linkTypeID
			}

			// Create item link via service (handles duplicate check)
			linkID, err := linkSvc.CreateLink(services.CreateItemLinkParams{
				LinkTypeID: linkTypeID,
				SourceType: "item",
				SourceID:   info.sourceID,
				TargetType: "item",
				TargetID:   targetID,
			})
			if err != nil {
				slog.Error("Failed to create item link",
					slog.String("component", "jira"),
					slog.String("source", info.sourceKey),
					slog.String("target", outwardKey),
					slog.Any("error", err))
				continue
			}

			if linkID > 0 {
				h.recordMapping(jobID, "link", fmt.Sprintf("%s-%s-%s", info.sourceKey, typeName, outwardKey), "", int(linkID), nil)
			}
		}
	}
}

// ensureLinkType finds or creates a link type matching the Jira link type
func (h *JiraImportHandler) ensureLinkType(typeName string, linkData map[string]interface{}) (int, error) {
	// Try to find existing by name
	var existingID int
	err := h.db.QueryRow(`SELECT id FROM link_types WHERE name = ?`, typeName).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	// Create new link type
	forwardLabel, _ := linkData["outward"].(string)
	reverseLabel, _ := linkData["inward"].(string)
	if forwardLabel == "" {
		forwardLabel = typeName
	}
	if reverseLabel == "" {
		reverseLabel = typeName
	}

	linkTypeSvc := services.NewEnumService(h.db, services.NewLinkTypeConfig())
	linkType := &models.LinkType{
		Name:         typeName,
		ForwardLabel: forwardLabel,
		ReverseLabel: reverseLabel,
		Color:        "#6B7280",
		Active:       true,
	}
	entity, err := linkTypeSvc.Create(linkType, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create link type: %w", err)
	}
	return entity.GetID(), nil
}

// ================================================================
// Phase 6: Attachment Import
// ================================================================

// importAttachments downloads and stores attachments from a Jira issue
func (h *JiraImportHandler) importAttachments(ctx context.Context, jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int, client jira.Client, progress *ImportProgress) {
	if len(issue.Fields.Attachment) == 0 {
		return
	}

	// Get attachment storage path from settings
	var attachmentPath string
	err := h.db.QueryRow(`SELECT attachment_path FROM attachment_settings WHERE enabled = true LIMIT 1`).Scan(&attachmentPath)
	if err != nil || attachmentPath == "" {
		slog.Warn("Attachment settings not configured, skipping attachment import",
			slog.String("component", "jira"), slog.String("issue", issue.Key))
		return
	}

	for _, attachment := range issue.Fields.Attachment {
		if attachment.Content == "" {
			continue
		}

		progress.TotalAttachments++

		// Download the attachment
		reader, _, err := client.DownloadAttachment(ctx, attachment.Content)
		if err != nil {
			slog.Error("Failed to download attachment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		// Generate a unique filename to avoid collisions
		storedFilename := fmt.Sprintf("%s_%s", uuid.New().String(), filepath.Base(attachment.Filename))
		filePath := filepath.Join(attachmentPath, storedFilename)

		// Save to disk
		file, err := os.Create(filePath) //nolint:gosec // G304 — filePath from attachmentPath + UUID + filename
		if err != nil {
			_ = reader.Close()
			slog.Error("Failed to create attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		written, err := io.Copy(file, reader)
		_ = file.Close()
		_ = reader.Close()
		if err != nil {
			_ = os.Remove(filePath) //nolint:gosec // G703 — filePath from attachmentPath + UUID + filepath.Base(filename)
			slog.Error("Failed to write attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		// Use actual written size if Jira didn't report one
		fileSize := attachment.Size
		if fileSize == 0 {
			fileSize = written
		}

		// Map uploader
		var uploadedBy *int
		if attachment.Author != nil && attachment.Author.GetIdentifier() != "" {
			if uid, ok := userMap[attachment.Author.GetIdentifier()]; ok {
				uploadedBy = &uid
			}
		}

		mimeType := attachment.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		// Insert attachment record via service
		attachmentSvc := services.NewAttachmentService(h.db)
		attachmentID, err := attachmentSvc.CreateRecord(services.CreateAttachmentParams{
			ItemID:           itemID,
			EntityType:       "item",
			Filename:         storedFilename,
			OriginalFilename: attachment.Filename,
			FilePath:         filePath,
			MimeType:         mimeType,
			FileSize:         fileSize,
			UploadedBy:       uploadedBy,
		})
		if err != nil {
			slog.Error("Failed to insert attachment record",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		h.recordMapping(jobID, "attachment", attachment.ID, issue.Key, int(attachmentID), nil)
		progress.ImportedAttachments++
	}
}
