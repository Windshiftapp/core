package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/constants"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

type itemCreation struct {
	ctx        context.Context
	db         database.Database
	params     ItemCreationParams
	now        time.Time
	createdAt  time.Time
	updatedAt  time.Time
	statusID   *int
	priorityID *int
}

func prepareItemCreation(ctx context.Context, db database.Database, params ItemCreationParams) (*itemCreation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("item creation requires a context")
	}

	creation := &itemCreation{ctx: ctx, db: db, params: params}
	if err := creation.validateAssignments(); err != nil {
		return nil, err
	}
	if err := creation.resolveItemType(); err != nil {
		return nil, err
	}
	if err := creation.applyMandatoryTemplate(); err != nil {
		return nil, err
	}
	creation.resolveTimestamps()
	if err := creation.resolveStatus(); err != nil {
		return nil, err
	}
	if err := validation.ValidateTaskState(db, creation.params.WorkspaceID, creation.params.ValidatingUserID, creation.params.IsTask, creation.statusID); err != nil {
		return nil, err
	}
	if err := creation.resolvePriority(); err != nil {
		return nil, err
	}
	return creation, nil
}

func (c *itemCreation) validateAssignments() error {
	params := c.params
	if err := validation.ValidatePlanningAssignments(c.db, params.WorkspaceID, params.MilestoneIDs, params.IterationID); err != nil {
		return err
	}
	labels := repository.NewLabelRepository(c.db)
	for _, labelID := range params.LabelIDs {
		if _, err := labels.GetByID(labelID); errors.Is(err, repository.ErrNotFound) {
			return &validation.ValidationError{Field: "label_ids", Message: fmt.Sprintf("Label %d not found", labelID)}
		} else if err != nil {
			return fmt.Errorf("validate label %d: %w", labelID, err)
		}
	}
	if params.ValidatingUserID > 0 && params.AssigneeID != nil {
		actionable, err := c.assigneeCanAct()
		if err != nil {
			return fmt.Errorf("failed to validate assignee: %w", err)
		}
		if !actionable {
			return &validation.ValidationError{Field: "assignee_id", Message: "Assignee user not found"}
		}
	}
	if params.ValidatingUserID > 0 && params.PermService != nil {
		return validateProjectAssignmentAccess(
			c.db,
			params.PermService,
			params.ValidatingUserID,
			params.ProjectID,
			params.TimeProjectID,
		)
	}
	return nil
}

func (c *itemCreation) assigneeCanAct() (bool, error) {
	params := c.params
	if params.PermService != nil {
		return NewWorkspaceUserResolver(c.db, params.PermService).CanActInWorkspace(*params.AssigneeID, params.WorkspaceID)
	}
	return repository.NewUserRepository(c.db).ActiveExists(*params.AssigneeID)
}

func (c *itemCreation) resolveItemType() error {
	itemTypeID, err := resolveItemTypeForCreation(c.db, c.params.WorkspaceID, c.params.ItemTypeID)
	if err != nil {
		return err
	}
	c.params.ItemTypeID = itemTypeID
	return validation.ValidateGenericSubtaskBoundary(
		c.db,
		*itemTypeID,
		c.params.ParentID,
		c.params.AllowUnparentedGenericSubtask,
	)
}

func (c *itemCreation) applyMandatoryTemplate() error {
	params := &c.params
	if params.SkipMandatoryTemplate {
		return nil
	}
	mandatory, err := repository.NewTemplateRepository(c.db).GetMandatoryForType(params.WorkspaceID, *params.ItemTypeID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to resolve mandatory template: %w", err)
	}

	applied := false
	if strings.TrimSpace(params.Description) == "" {
		params.Description = mandatory.DescriptionBody
		applied = true
	}
	if params.MandatoryTemplateOut != nil {
		*params.MandatoryTemplateOut = MandatoryTemplateInfo{
			TemplateID: mandatory.ID,
			Name:       mandatory.Name,
			Applied:    applied,
		}
	}
	return nil
}

func (c *itemCreation) resolveTimestamps() {
	c.now = time.Now()
	c.createdAt = c.now
	if c.params.CreatedAt != nil {
		c.createdAt = *c.params.CreatedAt
	}
	c.updatedAt = c.now
	if c.params.UpdatedAt != nil {
		c.updatedAt = *c.params.UpdatedAt
	}
}

func (c *itemCreation) resolveStatus() error {
	params := c.params
	workflowService := NewWorkflowService(c.db)
	if params.StatusID != nil {
		if params.ValidatingUserID > 0 {
			if err := workflowService.ValidateCreateStatusOverride(c.ctx, params.WorkspaceID, params.ItemTypeID, *params.StatusID); err != nil {
				return err
			}
		}
		c.statusID = params.StatusID
	} else if params.Status != "" {
		c.statusID = mapTextStatusToID(params.Status)
	}
	if c.statusID == nil {
		c.statusID, _ = workflowService.GetInitialStatusIDCached(params.WorkspaceID, params.ItemTypeID)
	}
	if c.statusID != nil {
		return nil
	}

	isPersonal, err := repository.IsPersonalWorkspace(c.db, params.WorkspaceID)
	if err != nil {
		return fmt.Errorf("resolve personal workspace status: %w", err)
	}
	if isPersonal {
		c.statusID = intPtr(constants.StatusIDOpen)
	}
	return nil
}

func (c *itemCreation) resolvePriority() error {
	params := c.params
	if params.PriorityID != nil {
		c.priorityID = params.PriorityID
	} else if params.Priority != "" {
		c.priorityID = mapTextPriorityToID(params.Priority)
	}
	if c.priorityID == nil {
		c.priorityID = c.defaultPriorityID()
	}
	if c.priorityID == nil {
		return nil
	}

	allowed, err := validation.IsPriorityAllowedInWorkspace(c.db, params.WorkspaceID, *c.priorityID)
	if err != nil {
		return fmt.Errorf("failed to validate workspace priority: %w", err)
	}
	if !allowed {
		return &validation.ValidationError{Field: "priority_id", Message: "Priority is not allowed in this workspace"}
	}
	return nil
}

func (c *itemCreation) defaultPriorityID() *int {
	var priorityID int
	err := c.db.QueryRow(`
		SELECT p.id FROM priorities p
		INNER JOIN configuration_set_priorities csp ON p.id = csp.priority_id
		INNER JOIN workspace_configuration_sets wcs ON csp.configuration_set_id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ?
		ORDER BY p.is_default DESC, p.sort_order, p.id
		LIMIT 1
	`, c.params.WorkspaceID).Scan(&priorityID)
	if err != nil {
		err = c.db.QueryRow("SELECT id FROM priorities WHERE is_default = true LIMIT 1").Scan(&priorityID)
	}
	if err != nil {
		return nil
	}
	return &priorityID
}

func (c *itemCreation) insert() (int, error) {
	return repository.WithItemCreateTransaction(c.ctx, c.db, func(tx database.Tx) (int, error) {
		fracIndex, err := repository.GenerateFracIndexForNewItem(tx, c.db.GetDriverName())
		if err != nil {
			return 0, fmt.Errorf("failed to generate frac_index: %w", err)
		}
		itemNumber, err := repository.NewItemRepository(c.db).GetNextWorkspaceItemNumber(tx, c.params.WorkspaceID)
		if err != nil {
			return 0, fmt.Errorf("failed to generate workspace item number: %w", err)
		}
		itemID, err := c.insertRow(tx, itemNumber, fracIndex)
		if err != nil {
			return 0, err
		}
		if err := c.extendTransaction(tx, itemID); err != nil {
			return 0, err
		}
		if err := c.recordCreation(tx, itemID, itemNumber); err != nil {
			return 0, err
		}
		return itemID, nil
	})
}

func (c *itemCreation) insertRow(tx database.Tx, itemNumber int, fracIndex string) (int, error) {
	p := c.params
	query := `
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description, status_id, priority_id, is_task,
			iteration_id, project_id, inherit_project, time_project_id, assignee_id, reporter_id, creator_id, creator_portal_customer_id,
			channel_id, request_type_id, due_date, start_date, end_date, related_work_item_id,
			story_points, estimate_minutes, custom_field_values, virtual_field_data, parent_id,
			frac_index, created_at, updated_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`

	var itemID int
	err := tx.QueryRow(query,
		p.WorkspaceID, itemNumber, p.ItemTypeID, p.Title, p.Description, c.statusID, c.priorityID, p.IsTask,
		p.IterationID, p.ProjectID, p.InheritProject, p.TimeProjectID, p.AssigneeID, p.ReporterID, p.CreatorID,
		p.CreatorPortalCustomerID, p.ChannelID, p.RequestTypeID, p.DueDate, p.StartDate, p.EndDate,
		p.RelatedWorkItemID, p.StoryPoints, p.EstimateMinutes, nullString(p.CustomFieldValuesJSON),
		nullString(p.VirtualFieldDataJSON), p.ParentID, fracIndex, c.createdAt, c.updatedAt, c.updatedAt,
	).Scan(&itemID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert item: %w", err)
	}
	return itemID, nil
}

func (c *itemCreation) extendTransaction(tx database.Tx, itemID int) error {
	for _, milestoneID := range c.params.MilestoneIDs {
		if _, err := tx.Exec(
			"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
			itemID,
			milestoneID,
			c.now,
		); err != nil {
			return fmt.Errorf("failed to attach milestone %d to new item: %w", milestoneID, err)
		}
	}
	if len(c.params.LabelIDs) > 0 {
		if err := repository.NewLabelRepository(c.db).ReplaceItemLabelsTx(c.ctx, tx, itemID, c.params.LabelIDs); err != nil {
			return err
		}
	}
	if c.params.AfterCreate != nil {
		return c.params.AfterCreate(c.ctx, tx, itemID)
	}
	return nil
}

func (c *itemCreation) recordCreation(tx database.Tx, itemID, itemNumber int) error {
	item, err := c.eventItem(itemID, itemNumber)
	if err != nil {
		return err
	}
	metadata := itemCreateEventMetadata(c.params, c.createdAt)
	if c.params.CreatorID != nil {
		history := creationHistoryEntries(*item, *c.params.CreatorID, metadata.OccurredAt)
		if err := repository.NewItemRepository(c.db).RecordHistoryBatch(tx, history); err != nil {
			return fmt.Errorf("record item creation history: %w", err)
		}
	}
	_, err = itemevents.NewRecorder(c.db).Created(c.ctx, tx, item, c.params.MilestoneIDs, metadata)
	return err
}

func (c *itemCreation) eventItem(itemID, itemNumber int) (*models.Item, error) {
	customFields, err := decodeItemEventFields(c.params.CustomFieldValuesJSON, "custom fields")
	if err != nil {
		return nil, err
	}
	virtualFields, err := decodeItemEventFields(c.params.VirtualFieldDataJSON, "virtual fields")
	if err != nil {
		return nil, err
	}
	p := c.params
	return &models.Item{
		ID: itemID, WorkspaceID: p.WorkspaceID, WorkspaceItemNumber: itemNumber,
		ItemTypeID: p.ItemTypeID, Title: p.Title, Description: p.Description,
		StatusID: c.statusID, PriorityID: c.priorityID, IsTask: p.IsTask,
		AssigneeID: p.AssigneeID, ReporterID: p.ReporterID, CreatorID: p.CreatorID,
		CreatorPortalCustomerID: p.CreatorPortalCustomerID, ChannelID: p.ChannelID,
		RequestTypeID: p.RequestTypeID, ParentID: p.ParentID, IterationID: p.IterationID,
		ProjectID: p.ProjectID, InheritProject: p.InheritProject, TimeProjectID: p.TimeProjectID,
		RelatedWorkItemID: p.RelatedWorkItemID, DueDate: p.DueDate, StartDate: p.StartDate,
		EndDate: p.EndDate, StoryPoints: p.StoryPoints, EstimateMinutes: p.EstimateMinutes,
		CustomFieldValues: customFields, VirtualFieldData: virtualFields,
		CreatedAt: c.createdAt, UpdatedAt: c.updatedAt,
	}, nil
}

func decodeItemEventFields(raw, name string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, fmt.Errorf("decode item event %s: %w", name, err)
	}
	return fields, nil
}

func (c *itemCreation) finish(itemID int) {
	p := c.params
	if !p.SkipAssigneeTrigger && p.AssigneeID != nil {
		triggeredBy := p.ValidatingUserID
		if triggeredBy == 0 && p.CreatorID != nil {
			triggeredBy = *p.CreatorID
		}
		maybeTriggerAssigneeRun(p.WorkspaceID, itemID, nil, p.AssigneeID, triggeredBy)
	}
	repository.InvalidateItemListCountCache(c.db, p.WorkspaceID)
	if p.SkipPublish {
		return
	}
	PublishItemChange(itemID, ItemChangeCreated)
	if p.ParentID != nil {
		PublishItemChange(*p.ParentID, ItemChangeUpdated)
	}
}
