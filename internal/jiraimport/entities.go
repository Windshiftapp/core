package jiraimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/validation"

	"uuid"
)

type UserMapping struct {
	UserID   int
	Username string
}

func (s *Service) UserMapping(jobID, accountID string) (UserMapping, bool) {
	var mapping UserMapping
	err := s.db.QueryRow(`
		SELECT u.id, u.username
		FROM jira_import_user_mappings m
		JOIN users u ON u.id = m.windshift_user_id
		WHERE m.job_id = ? AND m.jira_account_id = ?
	`, jobID, accountID).Scan(&mapping.UserID, &mapping.Username)
	return mapping, err == nil
}

func (s *Service) RecordUserMapping(
	jobID, accountID, email, displayName string,
	userID int,
	wasCreated bool,
) error {
	_, err := s.db.ExecWrite(`
		INSERT INTO jira_import_user_mappings
			(job_id, jira_account_id, jira_email, jira_display_name, windshift_user_id, was_created)
		VALUES (?, ?, ?, ?, ?, ?)
	`, jobID, accountID, email, displayName, userID, wasCreated)
	return err
}

func (s *Service) FindUserByEmail(email string) (UserMapping, error) {
	id, username, err := s.users.FindByEmailCaseInsensitive(email)
	return UserMapping{UserID: id, Username: username}, err
}

func (s *Service) UniqueImportedUsername(base string) (string, error) {
	const maxUsernameLength = 32
	base = strings.TrimSpace(strings.ToLower(base))
	if base == "" {
		base = "jira-user"
	}
	if len(base) > maxUsernameLength {
		base = base[:maxUsernameLength]
	}
	for suffix := 0; ; suffix++ {
		candidate := base
		if suffix > 0 {
			tail := fmt.Sprintf("-%d", suffix+1)
			prefixLength := maxUsernameLength - len(tail)
			if prefixLength < 1 {
				return "", fmt.Errorf("cannot allocate username suffix")
			}
			if len(candidate) > prefixLength {
				candidate = candidate[:prefixLength]
			}
			candidate += tail
		}
		exists, err := s.users.UsernameExistsCaseInsensitive(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
}

func (s *Service) CreateImportedUser(
	email, username, firstName, lastName, avatarURL string,
) (int, error) {
	id, err := s.users.Create(repository.CreateUserParams{
		Email:                 email,
		Username:              username,
		FirstName:             firstName,
		LastName:              lastName,
		AvatarURL:             avatarURL,
		RequiresPasswordReset: true,
		IsActive:              false,
	})
	return int(id), err
}

func (s *Service) EnsureImportedDummyUser(email string) (int, error) {
	id, err := s.users.GetIDByEmail(email)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, err
	}
	id64, createErr := s.users.Create(repository.CreateUserParams{
		Email:                 email,
		Username:              "jira-imported-user",
		FirstName:             "Imported",
		LastName:              "(Jira)",
		RequiresPasswordReset: true,
		IsActive:              false,
	})
	if createErr == nil {
		return int(id64), nil
	}
	if errors.Is(createErr, repository.ErrDuplicateEntry) {
		if id, err = s.users.GetIDByEmail(email); err == nil {
			return id, nil
		}
	}
	return 0, createErr
}

func (s *Service) EnsureLabel(name string) (int, error) {
	id, err := s.labels.FindIDByName(name)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, err
	}
	id64, _, err := s.labels.Create(name, "")
	return int(id64), err
}

func (s *Service) AddItemLabel(itemID, labelID int) error {
	_, err := s.db.ExecWrite(
		"INSERT INTO item_labels (item_id, label_id, created_at) VALUES (?, ?, ?)",
		itemID, labelID, time.Now(),
	)
	if database.IsUniqueConstraintError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("attach imported label %d to item %d: %w", labelID, itemID, err)
	}
	return nil
}

func (s *Service) CustomFieldOptions(fieldID int) string {
	options, err := s.customFields.FindOptions(fieldID)
	if err != nil {
		return ""
	}
	return options
}

func (s *Service) UpdateItemDescription(itemID int, description string, metadata itemevents.Metadata) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	original, err := s.items.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	if err := s.items.UpdateFields(tx, itemID, map[string]any{"description": description}); err != nil {
		return err
	}
	updated, err := s.items.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	if metadata.ActorKind == "" {
		metadata = itemevents.Import("jira")
	}
	if _, err := itemevents.NewRecorder(s.db).Updated(context.Background(), tx, updated, itemevents.Changes(original, updated), metadata); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ItemWorkspaceID(itemID int) (int, error) {
	var workspaceID int
	err := s.db.QueryRow(`SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID)
	return workspaceID, err
}

func (s *Service) UpdateImportedItem(itemID int, params services.ItemCreationParams) (int64, error) {
	if err := validation.ValidatePlanningAssignments(s.db, params.WorkspaceID, params.MilestoneIDs, params.IterationID); err != nil {
		return 0, err
	}
	var priorityID any
	if strings.TrimSpace(params.Priority) != "" {
		var resolvedPriorityID int
		if err := s.db.QueryRow(`
			SELECT id FROM priorities WHERE LOWER(name) = LOWER(?) ORDER BY id LIMIT 1
		`, params.Priority).Scan(&resolvedPriorityID); err == nil {
			priorityID = resolvedPriorityID
		}
	}
	updatedAt := time.Now()
	if params.UpdatedAt != nil {
		updatedAt = *params.UpdatedAt
	}
	var createdAt any
	if params.CreatedAt != nil {
		createdAt = *params.CreatedAt
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin Jira item upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	repo := repository.NewItemRepository(s.db)
	original, err := repo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return 0, fmt.Errorf("load imported Jira item: %w", err)
	}
	oldMilestoneIDs, err := loadItemMilestoneIDs(tx, itemID)
	if err != nil {
		return 0, err
	}
	fracIndex, err := repository.GenerateFracIndexForNewItem(tx, s.db.GetDriverName())
	if err != nil {
		return 0, fmt.Errorf("generate Jira re-import fractional index: %w", err)
	}
	customFieldValues := any(nil)
	if params.CustomFieldValuesJSON != "" {
		customFieldValues = params.CustomFieldValuesJSON
	}
	result, err := tx.ExecWrite(`
		UPDATE items
		SET title = ?, description = ?, status_id = COALESCE(?, status_id),
		    item_type_id = COALESCE(?, item_type_id), priority_id = COALESCE(?, priority_id),
		    iteration_id = ?, time_project_id = ?, assignee_id = ?, reporter_id = ?,
		    creator_id = ?, creator_portal_customer_id = ?, channel_id = ?, request_type_id = ?,
		    due_date = ?, story_points = ?, estimate_minutes = ?, custom_field_values = ?,
		    frac_index = ?, created_at = COALESCE(?, created_at), updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, params.Title, params.Description, params.StatusID, params.ItemTypeID, priorityID,
		params.IterationID, params.TimeProjectID, params.AssigneeID, params.ReporterID,
		params.CreatorID, params.CreatorPortalCustomerID, params.ChannelID, params.RequestTypeID,
		params.DueDate, params.StoryPoints, params.EstimateMinutes, customFieldValues,
		fracIndex, createdAt, updatedAt, itemID, params.WorkspaceID)
	if err != nil {
		return 0, fmt.Errorf("update imported Jira item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return 0, fmt.Errorf("imported Jira item %d no longer exists in workspace %d", itemID, params.WorkspaceID)
	}
	if _, err := tx.ExecWrite(`DELETE FROM item_milestones WHERE item_id = ?`, itemID); err != nil {
		return 0, fmt.Errorf("replace Jira item milestones: %w", err)
	}
	for _, milestoneID := range params.MilestoneIDs {
		if _, err := tx.ExecWrite(`
			INSERT INTO item_milestones (item_id, milestone_id, created_at)
			VALUES (?, ?, CURRENT_TIMESTAMP)
		`, itemID, milestoneID); err != nil {
			return 0, fmt.Errorf("attach Jira milestone %d: %w", milestoneID, err)
		}
	}
	updated, err := repo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return 0, fmt.Errorf("load updated imported Jira item: %w", err)
	}
	metadata := params.EventMetadata
	if metadata.ActorKind == "" {
		metadata = itemevents.Import("jira")
	}
	metadata.OccurredAt = updatedAt
	changes := itemevents.Changes(original, updated)
	if !equalIntIDs(oldMilestoneIDs, params.MilestoneIDs) {
		changes = append(changes, itemevents.FieldChange{
			Field: "milestones", OldValue: oldMilestoneIDs, NewValue: params.MilestoneIDs,
		})
	}
	recorder := itemevents.NewRecorder(s.db)
	if !equalIntPointers(original.StatusID, updated.StatusID) {
		if _, err := recorder.StatusChanged(context.Background(), tx, updated, original.StatusID, updated.StatusID, changes, metadata); err != nil {
			return 0, err
		}
	} else if _, err := recorder.Updated(context.Background(), tx, updated, changes, metadata); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit Jira item upsert: %w", err)
	}
	services.PublishItemChange(itemID)
	services.PublishWorkspaceChange(params.WorkspaceID)
	return int64(itemID), nil
}

func loadItemMilestoneIDs(tx database.Tx, itemID int) ([]int, error) {
	rows, err := tx.Query("SELECT milestone_id FROM item_milestones WHERE item_id = ? ORDER BY milestone_id", itemID)
	if err != nil {
		return nil, fmt.Errorf("load imported Jira item milestones: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func equalIntIDs(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalIntPointers(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func (s *Service) CommentExists(commentID int) bool {
	var exists bool
	return s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM comments WHERE id = ?)`, commentID).Scan(&exists) == nil && exists
}

func (s *Service) UpsertExternalIssueLink(
	jobID string,
	itemID int,
	itemKey, externalKey, typeName, direction string,
	sourceMetadata map[string]any,
) error {
	externalKey = strings.TrimSpace(externalKey)
	if itemID <= 0 || externalKey == "" {
		return nil
	}
	var connectionID, instanceURL string
	var instanceName sql.NullString
	var createdBy sql.NullInt64
	if err := s.db.QueryRow(`
		SELECT j.connection_id, c.instance_url, c.instance_name, j.created_by
		FROM jira_import_jobs j
		JOIN jira_import_connections c ON c.id = j.connection_id
		WHERE j.id = ?
	`, jobID).Scan(&connectionID, &instanceURL, &instanceName, &createdBy); err != nil {
		return fmt.Errorf("load Jira external-link provenance: %w", err)
	}
	providerID := "jira-import-" + connectionID
	providerName := strings.TrimSpace(instanceName.String)
	if providerName == "" {
		providerName = "Jira"
	}
	providerConfig, err := json.Marshal(map[string]any{
		"base_url": strings.TrimRight(instanceURL, "/"), "connection_id": connectionID, "managed_by": "jira_import",
	})
	if err != nil {
		return fmt.Errorf("encode Jira provider configuration: %w", err)
	}
	if _, err := s.db.ExecWrite(`
		INSERT INTO integration_providers
			(id, slug, name, provider_type, enabled, provider_config, created_at, updated_at)
		VALUES (?, ?, ?, 'jira', true, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, enabled = true,
			provider_config = excluded.provider_config, updated_at = CURRENT_TIMESTAMP
	`, providerID, providerID, providerName, string(providerConfig)); err != nil {
		return fmt.Errorf("ensure Jira integration provider: %w", err)
	}

	externalURL := strings.TrimRight(instanceURL, "/") + "/browse/" + url.PathEscape(externalKey)
	relation := strings.TrimSpace(typeName)
	if direction == "outward" {
		if phrase, _ := sourceMetadata["outward"].(string); strings.TrimSpace(phrase) != "" {
			relation = strings.TrimSpace(phrase)
		}
	} else if phrase, _ := sourceMetadata["inward"].(string); strings.TrimSpace(phrase) != "" {
		relation = strings.TrimSpace(phrase)
	}
	title := externalKey
	if relation != "" {
		title = relation + ": " + externalKey
	}
	linkMetadata, err := json.Marshal(map[string]any{
		"jira_issue_key": externalKey, "local_issue_key": itemKey, "jira_link_type": typeName,
		"direction": direction, "source": "jira_import",
	})
	if err != nil {
		return fmt.Errorf("encode Jira external link metadata: %w", err)
	}
	var existingID string
	queryErr := s.db.QueryRow(`
		SELECT id FROM item_integration_links
		WHERE item_id = ? AND integration_provider_id = ? AND external_id = ?
	`, strconv.Itoa(itemID), providerID, externalKey).Scan(&existingID)
	wasCreated := errors.Is(queryErr, sql.ErrNoRows)
	if queryErr != nil && !wasCreated {
		return fmt.Errorf("find existing Jira external link: %w", queryErr)
	}
	linkID := existingID
	if linkID == "" {
		linkID = uuid.New().String()
	}
	linkedBy := "jira-import"
	if createdBy.Valid {
		linkedBy = strconv.FormatInt(createdBy.Int64, 10)
	}
	if _, err := s.db.ExecWrite(`
		INSERT INTO item_integration_links
			(id, item_id, integration_provider_id, external_id, external_url,
			 title, icon, link_type, link_metadata, linked_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'ExternalLink', 'jira_issue', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(item_id, integration_provider_id, external_id) DO UPDATE SET
			external_url = excluded.external_url, title = excluded.title,
			link_metadata = excluded.link_metadata, updated_at = CURRENT_TIMESTAMP
	`, linkID, strconv.Itoa(itemID), providerID, externalKey, externalURL,
		sanitize.PlainTextField.Sanitize(title), string(linkMetadata), linkedBy); err != nil {
		return fmt.Errorf("upsert Jira external issue link: %w", err)
	}
	return s.RecordMapping(jobID, "external_issue_link",
		itemKey+":"+direction+":"+typeName+":"+externalKey,
		externalKey, itemID, map[string]any{
			"integration_link_id": linkID, "provider_id": providerID, "was_created": wasCreated,
		})
}

// SubresourceBelongsToItem reports whether a previously imported comment,
// attachment, or worklog is currently owned by the destination item. A partial
// fork retry must not reuse a mapping owned by the original workspace's item,
// because the update would move that resource instead of copying it.
func (s *Service) SubresourceBelongsToItem(entityType string, windshiftID, itemID int) (bool, error) {
	var query string
	switch entityType {
	case "comment":
		query = `SELECT item_id FROM comments WHERE id = ?`
	case "attachment":
		query = `SELECT item_id FROM attachments WHERE id = ?`
	case "worklog":
		query = `SELECT item_id FROM time_worklogs WHERE id = ?`
	default:
		return false, nil
	}
	var owner sql.NullInt64
	err := s.db.QueryRow(query, windshiftID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check %s ownership: %w", entityType, err)
	}
	return owner.Valid && int(owner.Int64) == itemID, nil
}

// ReassignAttachment moves a previously imported attachment onto a new item
// and refreshes its portal visibility, so a re-import can both relocate and
// reclassify the file.
func (s *Service) ReassignAttachment(attachmentID, itemID int, isInternal bool) (mimeType, originalFilename string, ok bool) {
	if err := s.db.QueryRow(`
		SELECT mime_type, original_filename FROM attachments WHERE id = ?
	`, attachmentID).Scan(&mimeType, &originalFilename); err != nil {
		return "", "", false
	}
	if _, err := s.db.ExecWrite(`
		UPDATE attachments SET item_id = ?, entity_type = 'item', is_internal = ? WHERE id = ?
	`, itemID, isInternal, attachmentID); err != nil {
		return "", "", false
	}
	return mimeType, originalFilename, true
}

type ParentLink struct {
	ChildID   int
	ParentKey string
}

func (s *Service) ValidateParentLink(childID, parentID int) error {
	var childItemTypeID, childHierarchyLevel int
	if err := s.db.QueryRow(`
		SELECT it.id, it.hierarchy_level
		FROM items child
		JOIN item_types it ON child.item_type_id = it.id
		WHERE child.id = ?
	`, childID).Scan(&childItemTypeID, &childHierarchyLevel); err != nil {
		return err
	}
	if childHierarchyLevel != models.HierarchyLevelGenericSubtask {
		return nil
	}
	if err := validation.ValidateParentForItemType(s.db, childItemTypeID, &parentID); err != nil {
		return err
	}
	wouldCycle, err := services.NewHierarchyService(s.db).WouldCreateCycle(childID, parentID)
	if err != nil {
		return err
	}
	if wouldCycle {
		return fmt.Errorf("parent link would create a cycle")
	}
	return nil
}

func (s *Service) ParentLinks(jobID string) ([]ParentLink, error) {
	rows, err := s.db.Query(`
		SELECT windshift_id, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var links []ParentLink
	for rows.Next() {
		var childID int
		var metadata sql.NullString
		if err := rows.Scan(&childID, &metadata); err != nil {
			return nil, err
		}
		var values map[string]any
		if !metadata.Valid || json.Unmarshal([]byte(metadata.String), &values) != nil {
			continue
		}
		if parentKey, ok := values["parent_key"].(string); ok && parentKey != "" {
			links = append(links, ParentLink{ChildID: childID, ParentKey: parentKey})
		}
	}
	return links, rows.Err()
}

type IssueLinks struct {
	SourceID  int
	SourceKey string
	Links     []map[string]any
}

func (s *Service) IssueLinks(jobID string) ([]IssueLinks, error) {
	rows, err := s.db.Query(`
		SELECT windshift_id, jira_key, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var results []IssueLinks
	for rows.Next() {
		var result IssueLinks
		var metadata sql.NullString
		if err := rows.Scan(&result.SourceID, &result.SourceKey, &metadata); err != nil {
			return nil, err
		}
		var values map[string]any
		if !metadata.Valid || json.Unmarshal([]byte(metadata.String), &values) != nil {
			continue
		}
		raw, ok := values["issue_links"].([]any)
		if !ok {
			continue
		}
		for _, entry := range raw {
			if link, ok := entry.(map[string]any); ok {
				result.Links = append(result.Links, link)
			}
		}
		if len(result.Links) > 0 {
			results = append(results, result)
		}
	}
	return results, rows.Err()
}

func (s *Service) EnsureLinkType(name, forwardLabel, reverseLabel string) (int, error) {
	id, err := s.linkTypes.FindIDByName(name)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, err
	}
	entity, err := services.NewEnumService(s.db, services.NewLinkTypeConfig()).Create(&models.LinkType{
		Name:         name,
		ForwardLabel: forwardLabel,
		ReverseLabel: reverseLabel,
		Color:        "#6B7280",
		Active:       true,
	}, nil)
	if err != nil {
		return 0, err
	}
	return entity.GetID(), nil
}

func (s *Service) TimeProjectCustomerID(projectID int) (int64, error) {
	info, err := s.timeProjects.GetBookingInfo(projectID)
	if err != nil {
		return 0, err
	}
	if info.CustomerID == nil {
		return 0, repository.ErrNotFound
	}
	return *info.CustomerID, nil
}

func (s *Service) UpsertImportedWorklog(input repository.ImportedWorklog, previous *PreviousMapping) (int64, error) {
	if previous != nil {
		result, err := s.db.ExecWrite(`
			UPDATE time_worklogs
			SET project_id = ?, customer_id = ?, user_id = ?, item_id = ?,
			    description = ?, date = ?, start_time = ?, end_time = ?,
			    duration_minutes = ?, created_at = ?, updated_at = ?
			WHERE id = ?
		`, input.ProjectID, input.CustomerID, input.UserID, input.ItemID,
			input.Description, input.DateUnix, input.StartTimeUnix, input.EndTimeUnix,
			input.DurationMinutes, input.CreatedAtUnix, input.UpdatedAtUnix, previous.WindshiftID)
		if err != nil {
			return 0, err
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			return int64(previous.WindshiftID), nil
		}
	}
	return s.worklogs.CreateImported(input)
}

func (s *Service) ItemLinkExists(linkID int) bool {
	var exists bool
	return s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM item_links WHERE id = ?)`, linkID).Scan(&exists) == nil && exists
}

func (s *Service) AttachmentPath() (string, bool) {
	var attachmentPath string
	err := s.db.QueryRow(`
		SELECT attachment_path
		FROM attachment_settings
		WHERE enabled = true
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&attachmentPath)
	if err != nil || strings.TrimSpace(attachmentPath) == "" {
		return "", false
	}
	return attachmentPath, true
}

func (s *Service) PreserveAttachmentCreatedAt(attachmentID int64, createdAt time.Time) error {
	return s.attachments.UpdateCreatedAt(attachmentID, createdAt)
}
