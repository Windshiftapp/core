package jiraimport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type mappingStore interface {
	QueryRow(query string, args ...any) *sql.Row
	ExecWrite(query string, args ...any) (sql.Result, error)
}

type PreviousMapping struct {
	ID          int
	JobID       string
	WindshiftID int
	Metadata    sql.NullString
}

type MappingRecord struct {
	EntityType  string
	JiraID      string
	JiraKey     string
	WindshiftID int
	Metadata    map[string]any
}

func (s *Service) ConfigurationRecords(jobID string) ([]MappingRecord, error) {
	rows, err := s.db.Query(`
		SELECT entity_type, jira_id, jira_key, windshift_id, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type IN ('custom_field', 'workflow', 'screen')
		ORDER BY entity_type, jira_id
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var records []MappingRecord
	for rows.Next() {
		var record MappingRecord
		var metadata sql.NullString
		if err := rows.Scan(&record.EntityType, &record.JiraID, &record.JiraKey, &record.WindshiftID, &metadata); err != nil {
			return nil, err
		}
		record.Metadata = Metadata(metadata)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Service) FidelityRecords(jobID string) ([]MappingRecord, error) {
	rows, err := s.db.Query(`
		SELECT entity_type, jira_key, metadata_json
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type IN ('custom_field', 'fidelity_finding')
		ORDER BY entity_type, jira_id
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var records []MappingRecord
	for rows.Next() {
		var record MappingRecord
		var metadata sql.NullString
		if err := rows.Scan(&record.EntityType, &record.JiraKey, &metadata); err != nil {
			return nil, err
		}
		record.Metadata = Metadata(metadata)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Service) ItemCustomFieldValues(jobID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT COALESCE(i.custom_field_values, '{}')
		FROM jira_import_id_mappings m
		JOIN items i ON i.id = m.windshift_id
		WHERE m.job_id = ? AND m.entity_type = 'item'
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		values = append(values, raw)
	}
	return values, rows.Err()
}

func MappingWasCreated(metadata sql.NullString) bool {
	if !metadata.Valid || strings.TrimSpace(metadata.String) == "" {
		return false
	}
	var values map[string]any
	if json.Unmarshal([]byte(metadata.String), &values) != nil {
		return false
	}
	created, ok := values["was_created"].(bool)
	return ok && created
}

func (s *Service) RecordMapping(jobID, entityType, jiraID, jiraKey string, windshiftID int, metadata map[string]any) error {
	return recordMappingInStore(s.db, jobID, entityType, jiraID, jiraKey, windshiftID, metadata)
}

func recordMappingInStore(store mappingStore, jobID, entityType, jiraID, jiraKey string, windshiftID int, metadata map[string]any) error {
	mappingMetadata := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		mappingMetadata[key] = value
	}
	if _, ok := mappingMetadata["was_created"]; !ok {
		mappingMetadata["was_created"] = mappingActionWasCreated(mappingMetadata)
	}
	var existingMetadataJSON string
	err := store.QueryRow(`
		SELECT metadata_json FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = ? AND jira_id = ?
	`, jobID, entityType, jiraID).Scan(&existingMetadataJSON)
	if err == nil && MappingWasCreated(sql.NullString{String: existingMetadataJSON, Valid: true}) {
		mappingMetadata["was_created"] = true
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load existing mapping provenance: %w", err)
	}
	metadataJSON, err := json.Marshal(mappingMetadata)
	if err != nil {
		return fmt.Errorf("encode mapping provenance: %w", err)
	}
	if _, err := store.ExecWrite(`
		INSERT INTO jira_import_id_mappings (job_id, entity_type, jira_id, jira_key, windshift_id, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (job_id, entity_type, jira_id) DO UPDATE SET
			jira_key = excluded.jira_key,
			windshift_id = excluded.windshift_id,
			metadata_json = excluded.metadata_json
	`, jobID, entityType, jiraID, jiraKey, windshiftID, string(metadataJSON)); err != nil {
		return fmt.Errorf("persist mapping provenance: %w", err)
	}
	return nil
}

func (s *Service) FindPreviousMapping(currentJobID, entityType, jiraID string) (*PreviousMapping, error) {
	var mapping PreviousMapping
	err := s.db.QueryRow(`
		SELECT m.id, m.job_id, m.windshift_id, m.metadata_json
		FROM jira_import_id_mappings m
		JOIN jira_import_jobs previous_job ON previous_job.id = m.job_id
		JOIN jira_import_jobs current_job ON current_job.id = ?
		WHERE m.job_id <> current_job.id
		  AND previous_job.connection_id = current_job.connection_id
		  AND previous_job.status <> 'data_deleted'
		  AND m.entity_type = ?
		  AND m.jira_id = ?
		ORDER BY COALESCE(previous_job.completed_at, previous_job.created_at) DESC, m.id DESC
		LIMIT 1
	`, currentJobID, entityType, jiraID).Scan(
		&mapping.ID,
		&mapping.JobID,
		&mapping.WindshiftID,
		&mapping.Metadata,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (s *Service) RecordMappingAndTransferOwnership(
	jobID, entityType, jiraID, jiraKey string,
	windshiftID int,
	metadata map[string]any,
	previous *PreviousMapping,
) error {
	if previous == nil {
		return s.RecordMapping(jobID, entityType, jiraID, jiraKey, windshiftID, metadata)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin mapping ownership transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := recordMappingInStore(tx, jobID, entityType, jiraID, jiraKey, windshiftID, metadata); err != nil {
		return err
	}
	previousMetadata := Metadata(previous.Metadata)
	if previousMetadata == nil {
		previousMetadata = make(map[string]any)
	}
	previousMetadata["was_created"] = false
	previousMetadata["superseded_by_job_id"] = jobID
	encoded, err := json.Marshal(previousMetadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecWrite(`
		UPDATE jira_import_id_mappings SET metadata_json = ? WHERE id = ?
	`, string(encoded), previous.ID); err != nil {
		return fmt.Errorf("transfer mapping ownership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mapping ownership transaction: %w", err)
	}
	return nil
}

func (s *Service) MappedEntity(jobID, entityType, jiraID string) (int, bool) {
	var id int
	err := s.db.QueryRow(`
		SELECT windshift_id FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = ? AND jira_id = ?
		ORDER BY id LIMIT 1
	`, jobID, entityType, jiraID).Scan(&id)
	return id, err == nil && id > 0
}

func (s *Service) MappedEntityByKey(jobID, entityType, jiraKey string) (int, bool) {
	var id int
	err := s.db.QueryRow(`
		SELECT windshift_id FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = ? AND jira_key = ?
		ORDER BY id LIMIT 1
	`, jobID, entityType, jiraKey).Scan(&id)
	return id, err == nil && id > 0
}

func (s *Service) LookupMappedEntityByKey(jobID, entityType, jiraKey string) (int, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT windshift_id FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = ? AND jira_key = ?
		ORDER BY id LIMIT 1
	`, jobID, entityType, jiraKey).Scan(&id)
	return id, err
}

func mappingActionWasCreated(metadata map[string]any) bool {
	action, _ := metadata["action"].(string)
	switch action {
	case "map", "reuse_existing", "reuse_existing_mapping", "reuse_workspace_default", "update_existing":
		return false
	default:
		return true
	}
}

func Metadata(metadata sql.NullString) map[string]any {
	if !metadata.Valid || strings.TrimSpace(metadata.String) == "" {
		return nil
	}
	var values map[string]any
	if json.Unmarshal([]byte(metadata.String), &values) != nil {
		return nil
	}
	return values
}

func mappingMetadata(metadata sql.NullString) map[string]any {
	return Metadata(metadata)
}

func mappingMetadataInt(metadata sql.NullString, key string) (int, bool) {
	value, ok := mappingMetadata(metadata)[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func mappingMetadataBool(metadata sql.NullString, key string) (enabled, found bool) {
	value, exists := mappingMetadata(metadata)[key]
	if !exists {
		return false, false
	}
	result, ok := value.(bool)
	return result, ok
}

func mappingMetadataString(metadata sql.NullString, key string) (string, bool) {
	value, ok := mappingMetadata(metadata)[key]
	if !ok {
		return "", false
	}
	result, ok := value.(string)
	return result, ok && strings.TrimSpace(result) != ""
}
