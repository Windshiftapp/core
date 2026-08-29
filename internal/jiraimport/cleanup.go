package jiraimport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"windshift/internal/repository"
)

var errAttachmentPathOutsideRoot = errors.New("attachment path is outside configured storage root")

type mapping struct {
	entityType   string
	windshiftID  int
	metadataJSON sql.NullString
}

type cleanupResult struct {
	Deleted  map[string]int   `json:"deleted"`
	Retained map[string]int   `json:"retained,omitempty"`
	Failed   []CleanupFailure `json:"failed,omitempty"`
}

type referenceQuery struct {
	statement string
	args      []any
}

// DeleteImportedData enforces the provenance boundary and removes only records
// this job explicitly owns. Unknown or malformed provenance never authorizes a
// destructive operation.
func (s *Service) DeleteImportedData(jobID string, confirmedWorkspaceCount int) (map[string]int, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM jira_import_jobs WHERE id = ?`, jobID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == "queued" || status == "running" {
		return nil, ErrJobActive
	}
	currentWorkspaceCount, _ := s.EntityCounts(jobID)
	if confirmedWorkspaceCount != currentWorkspaceCount {
		return nil, &WorkspaceCountMismatchError{Confirmed: confirmedWorkspaceCount, Current: currentWorkspaceCount}
	}

	mappings, err := s.cleanupMappings(jobID)
	if err != nil {
		return nil, err
	}
	result := cleanupResult{
		Deleted:  make(map[string]int),
		Retained: make(map[string]int),
	}
	for _, item := range mappings {
		retained, err := s.deleteMapping(jobID, item, result.Deleted)
		if err != nil {
			result.Failed = append(result.Failed, CleanupFailure{
				EntityType:  item.entityType,
				WindshiftID: item.windshiftID,
				Error:       err.Error(),
			})
			continue
		}
		if retained {
			result.Retained[item.entityType]++
		}
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(result.Failed) > 0 {
		if _, err := s.db.ExecWrite(`UPDATE jira_import_jobs SET result_json = ? WHERE id = ?`, string(resultJSON), jobID); err != nil {
			return nil, err
		}
		return result.Deleted, &CleanupError{Failures: result.Failed}
	}
	if err := s.finalizeCleanup(jobID, string(resultJSON)); err != nil {
		return nil, err
	}
	return result.Deleted, nil
}

func (s *Service) finalizeCleanup(jobID, resultJSON string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin Jira import cleanup finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecWrite(`DELETE FROM jira_import_id_mappings WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("delete Jira import mappings: %w", err)
	}
	if _, err := tx.ExecWrite(`
		UPDATE jira_import_jobs SET status = 'data_deleted', result_json = ? WHERE id = ?
	`, resultJSON, jobID); err != nil {
		return fmt.Errorf("mark Jira import data deleted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Jira import cleanup finalization: %w", err)
	}
	return nil
}

func (s *Service) cleanupMappings(jobID string) ([]mapping, error) {
	rows, err := s.db.Query(`
		SELECT entity_type, windshift_id, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ?
		ORDER BY CASE entity_type
			WHEN 'link' THEN 1 WHEN 'external_issue_link' THEN 2 WHEN 'watch' THEN 3
			WHEN 'worklog' THEN 4 WHEN 'comment' THEN 5 WHEN 'attachment' THEN 6
			WHEN 'test_case' THEN 7 WHEN 'item' THEN 8
			WHEN 'portal_customer_channel' THEN 9 WHEN 'portal_customer_role' THEN 10
			WHEN 'request_type' THEN 11 WHEN 'portal_customer' THEN 12
			WHEN 'customer_organisation' THEN 13 WHEN 'portal' THEN 14
			WHEN 'asset' THEN 15 WHEN 'asset_category' THEN 16 WHEN 'asset_status' THEN 17
			WHEN 'asset_type' THEN 18 WHEN 'asset_set' THEN 19
			WHEN 'board_configuration' THEN 20 WHEN 'collection' THEN 21
			WHEN 'iteration' THEN 22 WHEN 'milestone' THEN 23
			WHEN 'configuration_set' THEN 24 WHEN 'screen' THEN 25 WHEN 'workflow' THEN 26
			WHEN 'custom_field' THEN 27 WHEN 'status' THEN 28 WHEN 'item_type' THEN 29
			WHEN 'time_project' THEN 30 WHEN 'workspace' THEN 31 ELSE 32 END,
			id
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []mapping
	for rows.Next() {
		var item mapping
		if err := rows.Scan(&item.entityType, &item.windshiftID, &item.metadataJSON); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) deleteMapping(jobID string, item mapping, deleted map[string]int) (bool, error) {
	if item.entityType != "portal_customer" && !MappingWasCreated(item.metadataJSON) {
		return false, nil
	}
	if isSharedGlobalEntity(item.entityType) {
		referenced, err := s.sharedGlobalEntityIsReferenced(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if referenced {
			return true, nil
		}
	}
	var tableName string
	switch item.entityType {
	case "item":
		tableName = "items"
	case "test_case":
		tableName = "test_cases"
	case "workspace":
		tableName = "workspaces"
	case "request_type":
		reused, err := s.reusedEntity(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		tableName = "request_types"
	case "portal_customer_channel":
		channelID, ok := mappingMetadataInt(item.metadataJSON, "channel_id")
		if !ok {
			return false, errors.New("portal customer channel mapping is missing channel_id")
		}
		return false, s.deletePair("portal_customer_channels", "portal_customer_id", item.windshiftID, "channel_id", channelID, item.entityType, deleted)
	case "portal_customer_role":
		roleID, ok := mappingMetadataInt(item.metadataJSON, "contact_role_id")
		if !ok {
			return false, errors.New("portal customer role mapping is missing contact_role_id")
		}
		return false, s.deletePair("portal_customer_roles", "portal_customer_id", item.windshiftID, "contact_role_id", roleID, item.entityType, deleted)
	case "portal_customer":
		if !MappingWasCreated(item.metadataJSON) {
			return false, s.restorePortalCustomerOrganisation(item)
		}
		tableName = "portal_customers"
	case "customer_organisation":
		reused, err := s.reusedEntity(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		tableName = "customer_organisations"
	case "portal":
		reused, err := s.reusedEntity(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		tableName = "channels"
	case "asset":
		tableName = "assets"
	case "asset_category":
		tableName = "asset_categories"
	case "asset_status":
		tableName = "asset_statuses"
	case "asset_type":
		reused, err := s.reusedEntity(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		tableName = "asset_types"
	case "asset_set":
		reused, err := s.reusedEntity(jobID, item.entityType, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		tableName = "asset_management_sets"
	case "status":
		tableName = "statuses"
	case "item_type":
		tableName = "item_types"
	case "milestone":
		tableName = "milestones"
	case "custom_field":
		tableName = "custom_field_definitions"
	case "board_configuration":
		tableName = "board_configurations"
	case "collection":
		tableName = "collections"
	case "attachment":
		removed, err := s.deleteAttachment(item.windshiftID)
		if err != nil {
			return false, err
		}
		if removed {
			deleted[item.entityType]++
		}
		return false, nil
	case "comment":
		tableName = "comments"
	case "link":
		tableName = "item_links"
	case "external_issue_link":
		linkID, ok := mappingMetadataString(item.metadataJSON, "integration_link_id")
		if !ok {
			return false, errors.New("external issue link mapping is missing integration_link_id")
		}
		result, err := s.db.ExecWrite(`DELETE FROM item_integration_links WHERE id = ?`, linkID)
		if err != nil {
			return false, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return false, err
		} else if affected > 0 {
			deleted[item.entityType]++
		}
		return false, nil
	case "watch":
		userID, ok := mappingMetadataInt(item.metadataJSON, "user_id")
		if !ok {
			return false, errors.New("watch mapping is missing user_id")
		}
		result, err := s.db.ExecWrite(`
			UPDATE item_watches SET is_active = false, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = ? AND item_id = ?
		`, userID, item.windshiftID)
		if err != nil {
			return false, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return false, err
		} else if affected > 0 {
			deleted[item.entityType]++
		}
		return false, nil
	case "worklog":
		tableName = "time_worklogs"
	case "iteration":
		tableName = "iterations"
	case "time_project":
		reused, err := s.reusedWorkspaceTimeProject(jobID, item.windshiftID)
		if err != nil {
			return false, err
		}
		if reused {
			return false, nil
		}
		if _, err := s.db.ExecWrite("UPDATE workspaces SET time_project_id = NULL WHERE time_project_id = ?", item.windshiftID); err != nil {
			return false, err
		}
		tableName = "time_projects"
	case "configuration_set":
		for _, query := range []string{
			"DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?",
			"DELETE FROM configuration_set_item_types WHERE configuration_set_id = ?",
			"DELETE FROM configuration_set_screens WHERE configuration_set_id = ?",
			"DELETE FROM configuration_set_priorities WHERE configuration_set_id = ?",
		} {
			if _, err := s.db.ExecWrite(query, item.windshiftID); err != nil {
				return false, err
			}
		}
		tableName = "configuration_sets"
	case "screen":
		tableName = "screens"
	case "workflow":
		if err := s.deleteWorkflowTransitions(item.windshiftID); err != nil {
			return false, err
		}
		tableName = "workflows"
	default:
		slog.Warn("unknown Jira import mapping entity type", slog.String("entity_type", item.entityType))
		return false, fmt.Errorf("unsupported Jira import mapping entity type %q", item.entityType)
	}
	result, err := s.db.ExecWrite(fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName), item.windshiftID) //nolint:gosec // tableName is selected from the fixed whitelist above.
	if err != nil {
		slog.Error("failed to delete imported Jira entity", slog.String("entity_type", item.entityType), slog.Int("windshift_id", item.windshiftID), slog.Any("error", err))
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		deleted[item.entityType]++
	}
	return false, nil
}

func (s *Service) deletePair(table, firstColumn string, firstID int, secondColumn string, secondID int, entityType string, deleted map[string]int) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ? AND %s = ?", table, firstColumn, secondColumn) //nolint:gosec // all identifiers are fixed callers.
	result, err := s.db.ExecWrite(query, firstID, secondID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		deleted[entityType]++
	}
	return nil
}

func (s *Service) restorePortalCustomerOrganisation(item mapping) error {
	assigned, _ := mappingMetadataBool(item.metadataJSON, "organization_was_assigned")
	if !assigned {
		return nil
	}
	previousID, _ := mappingMetadataInt(item.metadataJSON, "previous_customer_organisation_id")
	if previousID > 0 {
		_, err := s.db.ExecWrite(`
			UPDATE portal_customers SET customer_organisation_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, previousID, item.windshiftID)
		return err
	}
	_, err := s.db.ExecWrite(`
		UPDATE portal_customers SET customer_organisation_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, item.windshiftID)
	return err
}

func isSharedGlobalEntity(entityType string) bool {
	switch entityType {
	case "status", "item_type", "screen", "custom_field":
		return true
	default:
		return false
	}
}

func (s *Service) sharedGlobalEntityIsReferenced(jobID, entityType string, windshiftID int) (bool, error) {
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM jira_import_id_mappings
		WHERE job_id <> ? AND entity_type = ? AND windshift_id = ?
	`, jobID, entityType, windshiftID).Scan(&count); err != nil {
		return false, fmt.Errorf("check cross-job %s references: %w", entityType, err)
	}
	if count > 0 {
		return true, nil
	}

	switch entityType {
	case "status":
		return s.statusIsReferenced(windshiftID)
	case "item_type":
		return s.itemTypeIsReferenced(windshiftID)
	case "screen":
		return s.screenIsReferenced(windshiftID)
	case "custom_field":
		return s.customFieldIsReferenced(windshiftID)
	default:
		return false, nil
	}
}

func (s *Service) statusIsReferenced(id int) (bool, error) {
	referenced, err := s.anyReference([]referenceQuery{
		{statement: "SELECT COUNT(*) FROM items WHERE status_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", args: []any{id, id}},
		{statement: "SELECT COUNT(*) FROM board_column_statuses WHERE status_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM approval_set_statuses WHERE status_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM approval_requests WHERE status_id = ? OR from_status_id = ?", args: []any{id, id}},
	})
	if err != nil || referenced {
		return referenced, err
	}
	return s.anyJSONReference(id, []string{
		"SELECT backlog_status_ids FROM board_configurations WHERE backlog_status_ids IS NOT NULL",
		"SELECT status_mapping FROM issue_sync_configs WHERE status_mapping IS NOT NULL",
		"SELECT reverse_status_mapping FROM issue_sync_configs WHERE reverse_status_mapping IS NOT NULL",
	})
}

func (s *Service) itemTypeIsReferenced(id int) (bool, error) {
	referenced, err := s.anyReference([]referenceQuery{
		{statement: "SELECT COUNT(*) FROM items WHERE item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM configuration_sets WHERE default_item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM configuration_set_item_types WHERE item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM request_types WHERE item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM asset_reports WHERE item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM item_template_item_types WHERE item_type_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM issue_sync_configs WHERE default_item_type_id = ?", args: []any{id}},
	})
	if err != nil || referenced {
		return referenced, err
	}
	return s.anyJSONReference(id, []string{
		"SELECT requirement_item_type_ids FROM test_coverage_configurations WHERE requirement_item_type_ids IS NOT NULL",
	})
}

func (s *Service) screenIsReferenced(id int) (bool, error) {
	return s.anyReference([]referenceQuery{
		{statement: "SELECT COUNT(*) FROM configuration_set_screens WHERE screen_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM configuration_set_item_types WHERE create_screen_id = ? OR edit_screen_id = ? OR view_screen_id = ?", args: []any{id, id, id}},
	})
}

func (s *Service) customFieldIsReferenced(id int) (bool, error) {
	idText := strconv.Itoa(id)
	identifiers := []any{idText, "custom_field_" + idText, "cf_" + idText}
	referenced, err := s.anyReference([]referenceQuery{
		{statement: "SELECT COUNT(*) FROM screen_fields WHERE field_type = 'custom' AND field_identifier IN (?, ?, ?)", args: identifiers},
		{statement: "SELECT COUNT(*) FROM request_type_fields WHERE field_type = 'custom' AND field_identifier IN (?, ?, ?)", args: identifiers},
		{statement: "SELECT COUNT(*) FROM asset_report_fields WHERE field_type = 'custom' AND field_identifier IN (?, ?, ?)", args: identifiers},
		{statement: "SELECT COUNT(*) FROM asset_type_fields WHERE custom_field_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM item_links WHERE custom_field_id = ?", args: []any{id}},
		{statement: "SELECT COUNT(*) FROM approval_steps WHERE (approver_source = 'custom_field' AND approver_field_id = ?) OR (escalation_target_source = 'custom_field' AND escalation_target_field_id = ?)", args: []any{id, id}},
	})
	if err != nil || referenced {
		return referenced, err
	}
	rowsUsingField, err := s.customFields.CountRowsUsingField(id)
	if err != nil {
		return false, fmt.Errorf("check custom field values: %w", err)
	}
	if rowsUsingField > 0 {
		return true, nil
	}
	return s.anyJSONReference(id, []string{
		"SELECT list_columns FROM board_configurations WHERE list_columns IS NOT NULL",
		"SELECT card_fields FROM board_configurations WHERE card_fields IS NOT NULL",
		"SELECT roadmap_config FROM board_configurations WHERE roadmap_config IS NOT NULL",
		"SELECT column_config FROM asset_reports WHERE column_config IS NOT NULL",
		"SELECT custom_field_values FROM portal_customers WHERE custom_field_values IS NOT NULL",
		"SELECT custom_field_values FROM customer_organisations WHERE custom_field_values IS NOT NULL",
		"SELECT custom_field_values FROM portal_request_drafts WHERE custom_field_values IS NOT NULL",
		"SELECT config FROM conditions WHERE config IS NOT NULL",
	})
}

func (s *Service) anyReference(queries []referenceQuery) (bool, error) {
	for _, query := range queries {
		var count int
		if err := s.db.QueryRow(query.statement, query.args...).Scan(&count); err != nil {
			return false, fmt.Errorf("check imported entity references: %w", err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) anyJSONReference(id int, queries []string) (bool, error) {
	for _, query := range queries {
		rows, err := s.db.Query(query)
		if err != nil {
			return false, fmt.Errorf("list structured entity references: %w", err)
		}
		for rows.Next() {
			var raw sql.NullString
			if err := rows.Scan(&raw); err != nil {
				_ = rows.Close()
				return false, fmt.Errorf("scan structured entity reference: %w", err)
			}
			if raw.Valid && jsonReferencesID(raw.String, id) {
				_ = rows.Close()
				return true, nil
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("iterate structured entity references: %w", err)
		}
		if err := rows.Close(); err != nil {
			return false, fmt.Errorf("close structured entity references: %w", err)
		}
	}
	return false, nil
}

func jsonReferencesID(raw string, id int) bool {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return true
	}
	idText := strconv.Itoa(id)
	var references func(any) bool
	references = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, nested := range typed {
				if key == idText || key == "custom_field_"+idText || key == "cf_"+idText || references(nested) {
					return true
				}
			}
		case []any:
			for _, nested := range typed {
				if references(nested) {
					return true
				}
			}
		case json.Number:
			parsed, err := strconv.Atoi(typed.String())
			return err == nil && parsed == id
		case string:
			return typed == idText || typed == "custom_field_"+idText || typed == "cf_"+idText
		}
		return false
	}
	return references(value)
}

func (s *Service) reusedEntity(jobID, entityType string, windshiftID int) (bool, error) {
	var metadata sql.NullString
	err := s.db.QueryRow(`
		SELECT metadata_json FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = ? AND windshift_id = ? LIMIT 1
	`, jobID, entityType, windshiftID).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	action, _ := mappingMetadata(metadata)["action"].(string)
	return action == "reuse_existing", nil
}

func (s *Service) reusedWorkspaceTimeProject(jobID string, windshiftID int) (bool, error) {
	var metadata sql.NullString
	err := s.db.QueryRow(`
		SELECT metadata_json FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'time_project' AND windshift_id = ? LIMIT 1
	`, jobID, windshiftID).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	action, _ := mappingMetadata(metadata)["action"].(string)
	return action == "reuse_workspace_default", nil
}

func (s *Service) deleteWorkflowTransitions(workflowID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin Jira workflow cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query("SELECT id FROM workflow_transitions WHERE workflow_id = ?", workflowID)
	if err != nil {
		return fmt.Errorf("list Jira workflow transitions: %w", err)
	}
	var transitionIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Jira workflow transition: %w", err)
		}
		transitionIDs = append(transitionIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate Jira workflow transitions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Jira workflow transitions: %w", err)
	}
	if _, err := repository.CancelApprovalRequestsForTransitions(tx, transitionIDs); err != nil {
		return fmt.Errorf("cancel Jira workflow approval requests: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM workflow_transitions WHERE workflow_id = ?", workflowID); err != nil {
		return fmt.Errorf("delete Jira workflow transitions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Jira workflow cleanup: %w", err)
	}
	return nil
}

func (s *Service) deleteAttachment(attachmentID int) (bool, error) {
	var filePath string
	var thumbnailPath sql.NullString
	err := s.db.QueryRow(`
		SELECT file_path, thumbnail_path FROM attachments WHERE id = ?
	`, attachmentID).Scan(&filePath, &thumbnailPath)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecWrite(`DELETE FROM attachments WHERE id = ?`, attachmentID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	s.removeAttachmentFile(filePath)
	if thumbnailPath.Valid && strings.TrimSpace(thumbnailPath.String) != "" {
		s.removeAttachmentFile(thumbnailPath.String)
	}
	return true, nil
}

func (s *Service) removeAttachmentFile(storedPath string) {
	var root string
	if err := s.db.QueryRow(`
		SELECT attachment_path FROM attachment_settings WHERE enabled = true LIMIT 1
	`).Scan(&root); err != nil {
		return
	}
	path, err := resolvePathWithinRoot(root, storedPath)
	if err != nil {
		slog.Warn("refusing to delete imported attachment outside storage root", slog.String("path", storedPath), slog.Any("error", err))
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) { //nolint:gosec // path is confined to the configured attachment root.
		slog.Warn("failed to delete imported attachment file", slog.String("path", path), slog.Any("error", err))
	}
}

func resolvePathWithinRoot(root, storedPath string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errAttachmentPathOutsideRoot
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	inside := func(candidate string) (string, bool, error) {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			return "", false, err
		}
		return absPath, absPath == absRoot || strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)), nil
	}
	if filepath.IsAbs(storedPath) {
		path, ok, err := inside(storedPath)
		if err != nil || !ok {
			return "", errAttachmentPathOutsideRoot
		}
		return path, nil
	}
	if path, ok, err := inside(storedPath); err == nil && ok {
		return path, nil
	}
	path, ok, err := inside(filepath.Join(root, storedPath))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errAttachmentPathOutsideRoot
	}
	return path, nil
}
