package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// RecurrenceRepository provides data access methods for recurrence rules and instances
type RecurrenceRepository struct {
	db database.Database
}

// NewRecurrenceRepository creates a new recurrence repository
func NewRecurrenceRepository(db database.Database) *RecurrenceRepository {
	return &RecurrenceRepository{db: db}
}

// GetByID retrieves a recurrence rule by ID
func (r *RecurrenceRepository) GetByID(id int) (*models.RecurrenceRule, error) {
	var rule models.RecurrenceRule
	var dtend, lastGenUntil, nextGenCheck sql.NullTime
	var statusOnCreate, createdBy sql.NullInt64

	err := r.db.QueryRow(`
		SELECT rr.id, rr.template_item_id, rr.workspace_id, rr.rrule, rr.dtstart, rr.dtend,
		       rr.timezone, rr.lead_time_days, rr.last_generated_until, rr.next_generation_check,
		       rr.copy_assignee, rr.copy_priority, rr.copy_custom_fields, rr.copy_description,
		       rr.status_on_create, rr.is_active, rr.created_by, rr.created_at, rr.updated_at,
		       i.title, w.name, w.key, u.name
		FROM recurrence_rules rr
		LEFT JOIN items i ON rr.template_item_id = i.id
		LEFT JOIN workspaces w ON rr.workspace_id = w.id
		LEFT JOIN users u ON rr.created_by = u.id
		WHERE rr.id = ?
	`, id).Scan(
		&rule.ID, &rule.TemplateItemID, &rule.WorkspaceID, &rule.RRule, &rule.DtStart, &dtend,
		&rule.Timezone, &rule.LeadTimeDays, &lastGenUntil, &nextGenCheck,
		&rule.CopyAssignee, &rule.CopyPriority, &rule.CopyCustomFields, &rule.CopyDescription,
		&statusOnCreate, &rule.IsActive, &createdBy, &rule.CreatedAt, &rule.UpdatedAt,
		&rule.TemplateTitle, &rule.WorkspaceName, &rule.WorkspaceKey, &rule.CreatorName,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find recurrence rule: %w", err)
	}

	// Handle nullable fields
	if dtend.Valid {
		rule.DtEnd = &dtend.Time
	}
	if lastGenUntil.Valid {
		rule.LastGeneratedUntil = &lastGenUntil.Time
	}
	if nextGenCheck.Valid {
		rule.NextGenerationCheck = &nextGenCheck.Time
	}
	if statusOnCreate.Valid {
		val := int(statusOnCreate.Int64)
		rule.StatusOnCreate = &val
	}
	if createdBy.Valid {
		val := int(createdBy.Int64)
		rule.CreatedBy = &val
	}

	return &rule, nil
}

// GetByTemplateItemID retrieves a recurrence rule by its template item ID
func (r *RecurrenceRepository) GetByTemplateItemID(templateItemID int) (*models.RecurrenceRule, error) {
	var ruleID int
	err := r.db.QueryRow(`SELECT id FROM recurrence_rules WHERE template_item_id = ?`, templateItemID).Scan(&ruleID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find recurrence rule by template: %w", err)
	}
	return r.GetByID(ruleID)
}

// GetRulesNeedingGeneration returns active rules where next_generation_check <= now
func (r *RecurrenceRepository) GetRulesNeedingGeneration(limit int) ([]*models.RecurrenceRule, error) {
	rows, err := r.db.Query(`
		SELECT rr.id, rr.template_item_id, rr.workspace_id, rr.rrule, rr.dtstart, rr.dtend,
		       rr.timezone, rr.lead_time_days, rr.last_generated_until, rr.next_generation_check,
		       rr.copy_assignee, rr.copy_priority, rr.copy_custom_fields, rr.copy_description,
		       rr.status_on_create, rr.is_active, rr.created_by, rr.created_at, rr.updated_at
		FROM recurrence_rules rr
		WHERE rr.is_active = 1
		  AND (rr.next_generation_check IS NULL OR rr.next_generation_check <= ?)
		ORDER BY rr.next_generation_check ASC
		LIMIT ?
	`, time.Now(), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recurrence rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []*models.RecurrenceRule
	for rows.Next() {
		rule := &models.RecurrenceRule{}
		var dtend, lastGenUntil, nextGenCheck sql.NullTime
		var statusOnCreate, createdBy sql.NullInt64

		err := rows.Scan(
			&rule.ID, &rule.TemplateItemID, &rule.WorkspaceID, &rule.RRule, &rule.DtStart, &dtend,
			&rule.Timezone, &rule.LeadTimeDays, &lastGenUntil, &nextGenCheck,
			&rule.CopyAssignee, &rule.CopyPriority, &rule.CopyCustomFields, &rule.CopyDescription,
			&statusOnCreate, &rule.IsActive, &createdBy, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recurrence rule: %w", err)
		}

		// Handle nullable fields
		if dtend.Valid {
			rule.DtEnd = &dtend.Time
		}
		if lastGenUntil.Valid {
			rule.LastGeneratedUntil = &lastGenUntil.Time
		}
		if nextGenCheck.Valid {
			rule.NextGenerationCheck = &nextGenCheck.Time
		}
		if statusOnCreate.Valid {
			val := int(statusOnCreate.Int64)
			rule.StatusOnCreate = &val
		}
		if createdBy.Valid {
			val := int(createdBy.Int64)
			rule.CreatedBy = &val
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// Create creates a new recurrence rule
func (r *RecurrenceRepository) Create(rule *models.RecurrenceRule) (int, error) {
	result, err := r.db.Exec(`
		INSERT INTO recurrence_rules (
			template_item_id, workspace_id, rrule, dtstart, dtend, timezone,
			lead_time_days, copy_assignee, copy_priority, copy_custom_fields,
			copy_description, status_on_create, is_active, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rule.TemplateItemID, rule.WorkspaceID, rule.RRule, rule.DtStart, rule.DtEnd, rule.Timezone,
		rule.LeadTimeDays, rule.CopyAssignee, rule.CopyPriority, rule.CopyCustomFields,
		rule.CopyDescription, rule.StatusOnCreate, rule.IsActive, rule.CreatedBy,
		time.Now(), time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create recurrence rule: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return int(id), nil
}

// Update updates a recurrence rule
func (r *RecurrenceRepository) Update(rule *models.RecurrenceRule) error {
	_, err := r.db.Exec(`
		UPDATE recurrence_rules SET
			rrule = ?, dtstart = ?, dtend = ?, timezone = ?, lead_time_days = ?,
			copy_assignee = ?, copy_priority = ?, copy_custom_fields = ?,
			copy_description = ?, status_on_create = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`,
		rule.RRule, rule.DtStart, rule.DtEnd, rule.Timezone, rule.LeadTimeDays,
		rule.CopyAssignee, rule.CopyPriority, rule.CopyCustomFields,
		rule.CopyDescription, rule.StatusOnCreate, rule.IsActive, time.Now(),
		rule.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update recurrence rule: %w", err)
	}
	return nil
}

// Delete deletes a recurrence rule
func (r *RecurrenceRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM recurrence_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete recurrence rule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateGenerationProgress updates the last_generated_until and next_generation_check fields
func (r *RecurrenceRepository) UpdateGenerationProgress(id int, lastGenUntil, nextCheck time.Time) error {
	_, err := r.db.Exec(`
		UPDATE recurrence_rules SET
			last_generated_until = ?, next_generation_check = ?, updated_at = ?
		WHERE id = ?
	`, lastGenUntil, nextCheck, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update generation progress: %w", err)
	}
	return nil
}

// UpdateNextCheck updates only the next_generation_check field
func (r *RecurrenceRepository) UpdateNextCheck(id int, nextCheck time.Time) error {
	_, err := r.db.Exec(`
		UPDATE recurrence_rules SET next_generation_check = ?, updated_at = ?
		WHERE id = ?
	`, nextCheck, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update next check: %w", err)
	}
	return nil
}

// GetExistingInstanceDates returns a map of dates that already have instances for a rule
func (r *RecurrenceRepository) GetExistingInstanceDates(ruleID int) (map[string]bool, error) {
	rows, err := r.db.Query(`
		SELECT scheduled_date FROM recurrence_instances WHERE recurrence_rule_id = ?
	`, ruleID)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing instance dates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	dates := make(map[string]bool)
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("failed to scan date: %w", err)
		}
		dates[date.Format("2006-01-02")] = true
	}

	return dates, nil
}

// GetNextSequenceNumber returns the next sequence number for a rule
func (r *RecurrenceRepository) GetNextSequenceNumber(tx database.Tx, ruleID int) (int, error) {
	var maxSeq sql.NullInt64
	err := tx.QueryRow(`
		SELECT MAX(sequence_number) FROM recurrence_instances WHERE recurrence_rule_id = ?
	`, ruleID).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("failed to get max sequence number: %w", err)
	}

	if maxSeq.Valid {
		return int(maxSeq.Int64) + 1, nil
	}
	return 1, nil
}

// CreateInstance creates a new recurrence instance record
func (r *RecurrenceRepository) CreateInstance(tx database.Tx, instance *models.RecurrenceInstance) error {
	_, err := tx.Exec(`
		INSERT INTO recurrence_instances (
			recurrence_rule_id, instance_item_id, scheduled_date, sequence_number, created_at
		) VALUES (?, ?, ?, ?, ?)
	`,
		instance.RecurrenceRuleID, instance.InstanceItemID, instance.ScheduledDate,
		instance.SequenceNumber, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to create recurrence instance: %w", err)
	}
	return nil
}

// GetInstancesByRuleID retrieves all instances for a rule
func (r *RecurrenceRepository) GetInstancesByRuleID(ruleID, limit, offset int) ([]*models.RecurrenceInstance, error) {
	rows, err := r.db.Query(`
		SELECT ri.id, ri.recurrence_rule_id, ri.instance_item_id, ri.scheduled_date,
		       ri.sequence_number, ri.created_at, i.title, s.name
		FROM recurrence_instances ri
		LEFT JOIN items i ON ri.instance_item_id = i.id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE ri.recurrence_rule_id = ?
		ORDER BY ri.scheduled_date DESC
		LIMIT ? OFFSET ?
	`, ruleID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var instances []*models.RecurrenceInstance
	for rows.Next() {
		instance := &models.RecurrenceInstance{}
		var itemTitle, itemStatus sql.NullString

		err := rows.Scan(
			&instance.ID, &instance.RecurrenceRuleID, &instance.InstanceItemID,
			&instance.ScheduledDate, &instance.SequenceNumber, &instance.CreatedAt,
			&itemTitle, &itemStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}

		if itemTitle.Valid {
			instance.ItemTitle = itemTitle.String
		}
		if itemStatus.Valid {
			instance.ItemStatus = itemStatus.String
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// CountInstancesByRuleID returns the count of instances for a rule
func (r *RecurrenceRepository) CountInstancesByRuleID(ruleID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM recurrence_instances WHERE recurrence_rule_id = ?
	`, ruleID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count instances: %w", err)
	}
	return count, nil
}

// ListByWorkspace lists all recurrence rules for a workspace
func (r *RecurrenceRepository) ListByWorkspace(workspaceID int) ([]*models.RecurrenceRule, error) {
	rows, err := r.db.Query(`
		SELECT rr.id, rr.template_item_id, rr.workspace_id, rr.rrule, rr.dtstart, rr.dtend,
		       rr.timezone, rr.lead_time_days, rr.last_generated_until, rr.next_generation_check,
		       rr.copy_assignee, rr.copy_priority, rr.copy_custom_fields, rr.copy_description,
		       rr.status_on_create, rr.is_active, rr.created_by, rr.created_at, rr.updated_at,
		       i.title, w.name, w.key,
		       (SELECT COUNT(*) FROM recurrence_instances ri WHERE ri.recurrence_rule_id = rr.id)
		FROM recurrence_rules rr
		LEFT JOIN items i ON rr.template_item_id = i.id
		LEFT JOIN workspaces w ON rr.workspace_id = w.id
		WHERE rr.workspace_id = ?
		ORDER BY rr.created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query recurrence rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []*models.RecurrenceRule
	for rows.Next() {
		rule := &models.RecurrenceRule{}
		var dtend, lastGenUntil, nextGenCheck sql.NullTime
		var statusOnCreate, createdBy sql.NullInt64

		err := rows.Scan(
			&rule.ID, &rule.TemplateItemID, &rule.WorkspaceID, &rule.RRule, &rule.DtStart, &dtend,
			&rule.Timezone, &rule.LeadTimeDays, &lastGenUntil, &nextGenCheck,
			&rule.CopyAssignee, &rule.CopyPriority, &rule.CopyCustomFields, &rule.CopyDescription,
			&statusOnCreate, &rule.IsActive, &createdBy, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.TemplateTitle, &rule.WorkspaceName, &rule.WorkspaceKey, &rule.InstanceCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recurrence rule: %w", err)
		}

		// Handle nullable fields
		if dtend.Valid {
			rule.DtEnd = &dtend.Time
		}
		if lastGenUntil.Valid {
			rule.LastGeneratedUntil = &lastGenUntil.Time
		}
		if nextGenCheck.Valid {
			rule.NextGenerationCheck = &nextGenCheck.Time
		}
		if statusOnCreate.Valid {
			val := int(statusOnCreate.Int64)
			rule.StatusOnCreate = &val
		}
		if createdBy.Valid {
			val := int(createdBy.Int64)
			rule.CreatedBy = &val
		}

		rules = append(rules, rule)
	}

	return rules, nil
}
