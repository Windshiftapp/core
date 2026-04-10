package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
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

// ensureUsers matches or creates users for import
// Returns a map from Jira account ID to Windshift user ID
// Fetches missing emails via the Jira API when needed
func (h *JiraImportHandler) ensureUsers(ctx context.Context, jobID string, users []JiraUserSummary, client jira.Client) (map[string]int, error) { //nolint:unparam // error return kept for API consistency
	result := make(map[string]int)

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

		// Skip users without email - they can't be matched later anyway
		// and empty emails cause UNIQUE constraint violations
		if u.Email == "" {
			slog.Debug("Skipping user without email", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("accountID", u.AccountID))
			continue
		}

		// Check if we already have a mapping for this user in this job
		var existingUserID int
		err := h.db.QueryRow(`
			SELECT windshift_user_id FROM jira_import_user_mappings
			WHERE job_id = ? AND jira_account_id = ?
		`, jobID, u.AccountID).Scan(&existingUserID)
		if err == nil {
			result[u.AccountID] = existingUserID
			continue
		}

		// Try to find existing Windshift user by email
		var userID int
		if u.Email != "" {
			err = h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, u.Email).Scan(&userID)
			if err == nil {
				// Found existing user
				result[u.AccountID] = userID
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
		h.recordUserMapping(jobID, u, int(newUserID), true)

		slog.Debug("Created user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Int64("userID", newUserID))
	}

	return result, nil
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

// importIssue imports a single Jira issue as a Windshift work item
func (h *JiraImportHandler) importIssue(ctx context.Context, jobID string, workspaceID int, issue *jira.JiraIssue, statusMap, itemTypeMap, userMap, versionMap map[string]int, customFieldMappings []CustomFieldMapping, client jira.Client, progress *ImportProgress) error {
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

	// Map fixVersion to milestone (use first version)
	var milestoneID *int
	if len(issue.Fields.FixVersions) > 0 {
		if mid, ok := versionMap[issue.Fields.FixVersions[0].ID]; ok {
			milestoneID = &mid
		}
	}

	// Map priority
	var priorityName string
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != "" {
		priorityName = issue.Fields.Priority.Name
	}

	// Parse due date
	var dueDate *time.Time
	if issue.Fields.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
			dueDate = &parsed
		}
	}

	// Convert description from ADF to markdown
	description := ""
	if issue.Fields.Description != nil {
		description = jira.ConvertADFToMarkdown(issue.Fields.Description)
	}

	// Process custom fields (user/users types only for now)
	customFieldValues := make(map[string]interface{})
	for _, mapping := range customFieldMappings {
		if mapping.Action == "skip" {
			continue
		}

		// Only process user/users types for now
		if mapping.WindshiftType != "user" && mapping.WindshiftType != "users" {
			continue
		}

		value, exists := issue.Fields.CustomFields[mapping.JiraID]
		if !exists || value == nil {
			continue
		}

		switch mapping.WindshiftType {
		case "user":
			// Single user picker
			if userObj, ok := value.(map[string]interface{}); ok {
				if accountID, ok := userObj["accountId"].(string); ok {
					if uid, ok := userMap[accountID]; ok {
						customFieldValues[mapping.JiraID] = uid
					}
				}
			}
		case "users":
			// Multi-user picker (like Approvers)
			if users, ok := value.([]interface{}); ok {
				var userIDs []int
				for _, u := range users {
					if userObj, ok := u.(map[string]interface{}); ok {
						if accountID, ok := userObj["accountId"].(string); ok {
							if uid, ok := userMap[accountID]; ok {
								userIDs = append(userIDs, uid)
							}
						}
					}
				}
				if len(userIDs) > 0 {
					customFieldValues[mapping.JiraID] = userIDs
				}
			}
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
		MilestoneID:           milestoneID,
		CustomFieldValuesJSON: customFieldValuesJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Build metadata for the mapping (includes parent key for later linking)
	meta := map[string]interface{}{
		"summary": issue.Fields.Summary,
	}
	if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		meta["parent_key"] = issue.Fields.Parent.Key
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

	// Import comments for this issue
	h.importComments(jobID, int(itemID), issue, userMap)

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
func (h *JiraImportHandler) importComments(jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int) {
	if issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) == 0 {
		return
	}

	// Create a CommentService without notification/webhook/mention services
	// so bulk import doesn't generate notifications
	commentSvc := services.NewCommentService(h.db)

	for _, comment := range issue.Fields.Comment.Comments {
		content := jira.ConvertADFToMarkdown(comment.Body)
		if content == "" {
			continue
		}

		authorID := 0
		if comment.Author != nil && comment.Author.GetIdentifier() != "" {
			if uid, ok := userMap[comment.Author.GetIdentifier()]; ok {
				authorID = uid
			}
		}

		// Parse created timestamp
		var createdAt *time.Time
		if comment.Created != "" {
			if parsed, err := time.Parse("2006-01-02T15:04:05.000-0700", comment.Created); err == nil {
				createdAt = &parsed
			} else if parsed, err := time.Parse("2006-01-02T15:04:05.000Z0700", comment.Created); err == nil {
				createdAt = &parsed
			}
		}

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

			// Determine source and target
			// For outward links: this issue is the source, outward_key is target
			// For inward links: inward_key is the source, this issue is target
			// We only process outward links to avoid duplicates
			outwardKey, _ := link["outward_key"].(string)
			if outwardKey == "" {
				continue
			}

			// Look up target Windshift ID
			var targetID int
			err := h.db.QueryRow(`
				SELECT windshift_id FROM jira_import_id_mappings
				WHERE job_id = ? AND entity_type = 'item' AND jira_key = ?
			`, jobID, outwardKey).Scan(&targetID)
			if err != nil {
				// Target issue not imported (different project or not selected)
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
			_ = os.Remove(filePath)
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
