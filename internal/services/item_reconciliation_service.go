package services

import (
	"context"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
	"windshift/internal/validation"
)

// ExternalItemSource identifies the system that owns an inbound mutation.
type ExternalItemSource string

const (
	ExternalItemSourceGitHubIssueSync ExternalItemSource = "github_issue_sync"
)

// ExternalItemReconciliationPolicy makes non-user side effects explicit.
// This path never emits notifications, actions, webhooks, or mentions.
type ExternalItemReconciliationPolicy struct {
	Source                  ExternalItemSource
	ActorUserID             int
	RecordHistory           bool
	ApplyMandatoryTemplates bool
	TriggerAssignee         bool
	PublishLiveUpdates      bool
}

// GitHubIssueSyncReconciliationPolicy returns the inbound issue-sync policy.
func GitHubIssueSyncReconciliationPolicy() ExternalItemReconciliationPolicy {
	return ExternalItemReconciliationPolicy{
		Source:             ExternalItemSourceGitHubIssueSync,
		PublishLiveUpdates: true,
	}
}

// ExternalItemCreateRequest describes one source-owned item creation.
type ExternalItemCreateRequest struct {
	Policy      ExternalItemReconciliationPolicy
	Input       ItemCreateInput
	AfterCreate ItemCreateTransactionHook
}

// ExternalItemUpdateRequest describes one source-owned item update.
type ExternalItemUpdateRequest struct {
	Policy      ExternalItemReconciliationPolicy
	ItemID      int
	UpdateData  map[string]any
	AfterUpdate ItemUpdateTransactionHook
}

// ExternalItemReconciliationService applies inbound changes through the
// canonical item validation and transaction paths without user-facing events.
type ExternalItemReconciliationService struct {
	db database.Database
}

func NewExternalItemReconciliationService(db database.Database) *ExternalItemReconciliationService {
	return &ExternalItemReconciliationService{db: db}
}

// Create validates and commits an externally owned item and source metadata.
func (s *ExternalItemReconciliationService) Create(ctx context.Context, req ExternalItemCreateRequest) (*models.Item, error) {
	if err := validateExternalReconciliationRequest(ctx, s.db, req.Policy); err != nil {
		return nil, err
	}

	var err error
	req.Input.Title, err = validation.NormalizeTitle(req.Input.Title)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateMarkdownSource("description", req.Input.Description, validation.MarkdownMaxBytes, false); err != nil {
		return nil, err
	}
	req.Input.ItemTypeID, err = resolveItemTypeForCreation(s.db, req.Input.WorkspaceID, req.Input.ItemTypeID)
	if err != nil {
		return nil, err
	}

	validationResult := ValidateItemCreation(s.db, ItemValidationParams{
		WorkspaceID:       req.Input.WorkspaceID,
		Title:             req.Input.Title,
		ItemTypeID:        req.Input.ItemTypeID,
		ParentID:          req.Input.ParentID,
		RelatedWorkItemID: req.Input.RelatedWorkItemID,
	})
	if !validationResult.Valid {
		return nil, &ItemCreationValidationError{Message: validationResult.Error}
	}
	if req.Input.StatusID == nil {
		return nil, &validation.ValidationError{Field: "status_id", Message: "external reconciliation requires a mapped status"}
	}
	if err := validateExternalStatus(s.db, req.Input.WorkspaceID, req.Input.ItemTypeID, *req.Input.StatusID); err != nil {
		return nil, err
	}
	if err := validateExternalAssignee(s.db, req.Input.AssigneeID); err != nil {
		return nil, err
	}

	var creatorID *int
	if req.Policy.RecordHistory {
		creatorID = &req.Policy.ActorUserID
	}
	itemID, err := createItem(ctx, s.db, ItemCreationParams{
		WorkspaceID:           req.Input.WorkspaceID,
		Title:                 req.Input.Title,
		Description:           req.Input.Description,
		StatusID:              req.Input.StatusID,
		PriorityID:            req.Input.PriorityID,
		ItemTypeID:            req.Input.ItemTypeID,
		IsTask:                req.Input.IsTask,
		ParentID:              req.Input.ParentID,
		MilestoneIDs:          req.Input.MilestoneIDs,
		IterationID:           req.Input.IterationID,
		ProjectID:             req.Input.ProjectID,
		InheritProject:        req.Input.InheritProject,
		TimeProjectID:         req.Input.TimeProjectID,
		AssigneeID:            req.Input.AssigneeID,
		CreatorID:             creatorID,
		ValidatingUserID:      req.Policy.ActorUserID,
		DueDate:               req.Input.DueDate,
		StartDate:             req.Input.StartDate,
		EndDate:               req.Input.EndDate,
		RelatedWorkItemID:     req.Input.RelatedWorkItemID,
		StoryPoints:           req.Input.StoryPoints,
		EstimateMinutes:       req.Input.EstimateMinutes,
		SkipAssigneeTrigger:   !req.Policy.TriggerAssignee,
		SkipPublish:           !req.Policy.PublishLiveUpdates,
		SkipMandatoryTemplate: !req.Policy.ApplyMandatoryTemplates,
		AfterCreate:           req.AfterCreate,
	})
	if err != nil {
		return nil, err
	}

	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(int(itemID))
	if err != nil {
		return nil, fmt.Errorf("load reconciled item: %w", err)
	}
	return item, nil
}

// Update validates and commits an externally owned update and source metadata.
func (s *ExternalItemReconciliationService) Update(ctx context.Context, req ExternalItemUpdateRequest) (*UpdateItemResult, error) {
	if err := validateExternalReconciliationRequest(ctx, s.db, req.Policy); err != nil {
		return nil, err
	}
	if req.ItemID <= 0 {
		return nil, &validation.ValidationError{Field: "item_id", Message: "must be positive"}
	}

	var mappedStatusID *int
	if rawStatus, changed := req.UpdateData["status_id"]; changed {
		if rawStatus == nil {
			return nil, &validation.ValidationError{Field: "status_id", Message: "external reconciliation requires a mapped status"}
		}
		statusID, ok := utils.CoerceInt(rawStatus)
		if !ok {
			return nil, &validation.ValidationError{Field: "status_id", Message: "invalid mapped status"}
		}
		mappedStatusID = &statusID
	}
	var beforeUpdate ItemUpdateTransactionHook
	if mappedStatusID != nil {
		beforeUpdate = func(_ context.Context, _ database.Tx, _, updated *models.Item) error {
			return validateExternalStatus(s.db, updated.WorkspaceID, updated.ItemTypeID, *mappedStatusID)
		}
	}

	return NewItemUpdateService(s.db).updateItem(ctx, UpdateItemRequest{
		ItemID:        req.ItemID,
		UpdateData:    req.UpdateData,
		UserID:        req.Policy.ActorUserID,
		EventMetadata: itemEventMetadata(req.Policy.ActorUserID, string(req.Policy.Source), nil),
	}, itemUpdateOptions{
		allowStatus:             true,
		recordHistory:           req.Policy.RecordHistory,
		triggerAssignee:         req.Policy.TriggerAssignee,
		publish:                 req.Policy.PublishLiveUpdates,
		beforeUpdateTransaction: beforeUpdate,
		afterUpdateTransaction:  req.AfterUpdate,
	})
}

func validateExternalReconciliationRequest(ctx context.Context, db database.Database, policy ExternalItemReconciliationPolicy) error {
	if ctx == nil {
		return fmt.Errorf("external item reconciliation requires a context")
	}
	if db == nil {
		return fmt.Errorf("external item reconciliation requires a database")
	}
	switch policy.Source {
	case ExternalItemSourceGitHubIssueSync:
	case "":
		return fmt.Errorf("external item reconciliation requires a source")
	default:
		return fmt.Errorf("unsupported external item reconciliation source %q", policy.Source)
	}
	if policy.RecordHistory && policy.ActorUserID <= 0 {
		return fmt.Errorf("external item reconciliation history requires an actor")
	}
	return nil
}

func validateExternalStatus(db database.Database, workspaceID int, itemTypeID *int, statusID int) error {
	workflowID, err := NewWorkflowService(db).GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return fmt.Errorf("resolve external reconciliation workflow: %w", err)
	}
	if workflowID == nil {
		return &validation.ValidationError{Field: "status_id", Message: "workspace item type has no workflow"}
	}

	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE workflow_id = ? AND (from_status_id = ? OR to_status_id = ?)
		)
	`, *workflowID, statusID, statusID).Scan(&exists); err != nil {
		return fmt.Errorf("validate external reconciliation status: %w", err)
	}
	if !exists {
		return &validation.ValidationError{Field: "status_id", Message: "status is not part of the workspace item type workflow"}
	}
	return nil
}

func validateExternalAssignee(db database.Database, assigneeID *int) error {
	if assigneeID == nil {
		return nil
	}
	active, err := repository.NewUserRepository(db).ActiveExists(*assigneeID)
	if err != nil {
		return fmt.Errorf("validate external reconciliation assignee: %w", err)
	}
	if !active {
		return &validation.ValidationError{Field: "assignee_id", Message: "Assignee user not found"}
	}
	return nil
}
