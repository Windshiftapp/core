package services

import (
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// ItemCreatedEmitter receives the committed, fully hydrated item so every
// user-facing transport can trigger the same notification, automation, and
// webhook pipeline.
type ItemCreatedEmitter interface {
	EmitItemCreated(item *models.Item, actorUserID int, actorUsername ...string)
}

type contextualItemCreatedEmitter interface {
	EmitItemCreatedWithContext(item *models.Item, actorUserID int, actionContext ActionContext, actorUsername ...string)
}

// ItemCreateInput is the transport-neutral user-facing item-create payload.
type ItemCreateInput struct {
	WorkspaceID       int
	Title             string
	Description       string
	StatusID          *int
	PriorityID        *int
	ItemTypeID        *int
	DueDate           *time.Time
	StartDate         *time.Time
	EndDate           *time.Time
	IsTask            bool
	IterationID       *int
	ProjectID         *int
	InheritProject    bool
	TimeProjectID     *int
	AssigneeID        *int
	ParentID          *int
	RelatedWorkItemID *int
	StoryPoints       *float64
	EstimateMinutes   *int
	CustomFieldValues map[string]any
	MilestoneIDs      []int
	LabelIDs          []int
}

// ItemCreateResult contains the committed item and mandatory-template detail
// needed by transports that expose template enforcement metadata.
type ItemCreateResult struct {
	Item              *models.Item
	MandatoryTemplate MandatoryTemplateInfo
}

// ItemCreationValidationError identifies validation performed before the
// low-level creation transaction. Handlers retain their own error envelopes.
type ItemCreationValidationError struct {
	Message string
}

func (e *ItemCreationValidationError) Error() string { return e.Message }

// ItemCreationService owns the shared user-facing item creation pipeline.
type ItemCreationService struct {
	db      database.Database
	perm    *PermissionService
	emitter ItemCreatedEmitter
}

func NewItemCreationService(db database.Database, perm *PermissionService) *ItemCreationService {
	return &ItemCreationService{db: db, perm: perm}
}

func (s *ItemCreationService) SetEmitter(emitter ItemCreatedEmitter) {
	s.emitter = emitter
}

func (s *ItemCreationService) SetPermissionService(perm *PermissionService) {
	s.perm = perm
}

func (s *ItemCreationService) Create(actorUserID int, actorUsername string, input ItemCreateInput) (*ItemCreateResult, error) {
	return s.create(actorUserID, actorUsername, input, nil)
}

// CreateWithContext runs the standard creation pipeline while preserving the
// automation chain that caused the mutation.
func (s *ItemCreationService) CreateWithContext(
	actorUserID int,
	actorUsername string,
	input ItemCreateInput,
	actionContext ActionContext,
) (*ItemCreateResult, error) {
	return s.create(actorUserID, actorUsername, input, &actionContext)
}

func (s *ItemCreationService) create(
	actorUserID int,
	actorUsername string,
	input ItemCreateInput,
	actionContext *ActionContext,
) (*ItemCreateResult, error) {
	var err error
	input.Title, err = validation.NormalizeTitle(input.Title)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateMarkdownSource("description", input.Description, validation.MarkdownMaxBytes, false); err != nil {
		return nil, err
	}

	validationResult := ValidateItemCreation(s.db, ItemValidationParams{
		WorkspaceID:       input.WorkspaceID,
		Title:             input.Title,
		ItemTypeID:        input.ItemTypeID,
		ParentID:          input.ParentID,
		RelatedWorkItemID: input.RelatedWorkItemID,
		UserID:            actorUserID,
		PermService:       s.perm,
	})
	if !validationResult.Valid {
		return nil, &ItemCreationValidationError{Message: validationResult.Error}
	}

	if input.ParentID != nil && *input.ParentID == 0 {
		input.ParentID = nil
	}
	if input.ProjectID == nil && !input.InheritProject && input.ParentID != nil {
		input.InheritProject = true
	}

	var customFieldValuesJSON string
	if input.CustomFieldValues != nil {
		if err := validation.ValidateAndNormalizeCustomFieldValues(s.db, input.CustomFieldValues); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(input.CustomFieldValues)
		if err != nil {
			return nil, &ItemCreationValidationError{Message: "Invalid custom field values"}
		}
		customFieldValuesJSON = string(encoded)
	}

	var mandatoryTemplate MandatoryTemplateInfo
	itemID, err := CreateItem(s.db, ItemCreationParams{
		WorkspaceID:           input.WorkspaceID,
		Title:                 input.Title,
		Description:           input.Description,
		StatusID:              input.StatusID,
		PriorityID:            input.PriorityID,
		ItemTypeID:            input.ItemTypeID,
		IsTask:                input.IsTask,
		ParentID:              input.ParentID,
		MilestoneIDs:          input.MilestoneIDs,
		LabelIDs:              input.LabelIDs,
		IterationID:           input.IterationID,
		ProjectID:             input.ProjectID,
		InheritProject:        input.InheritProject,
		TimeProjectID:         input.TimeProjectID,
		AssigneeID:            input.AssigneeID,
		CreatorID:             &actorUserID,
		DueDate:               input.DueDate,
		StartDate:             input.StartDate,
		EndDate:               input.EndDate,
		RelatedWorkItemID:     input.RelatedWorkItemID,
		StoryPoints:           input.StoryPoints,
		EstimateMinutes:       input.EstimateMinutes,
		CustomFieldValuesJSON: customFieldValuesJSON,
		ValidatingUserID:      actorUserID,
		PermService:           s.perm,
		MandatoryTemplateOut:  &mandatoryTemplate,
		EventMetadata:         itemEventMetadata(actorUserID, "application", actionContext),
	})
	if err != nil {
		return nil, err
	}

	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(int(itemID))
	if err != nil {
		return nil, fmt.Errorf("load created item: %w", err)
	}
	if contextual, ok := s.emitter.(contextualItemCreatedEmitter); ok && actionContext != nil {
		contextual.EmitItemCreatedWithContext(item, actorUserID, *actionContext, actorUsername)
	} else if s.emitter != nil {
		s.emitter.EmitItemCreated(item, actorUserID, actorUsername)
	}
	return &ItemCreateResult{Item: item, MandatoryTemplate: mandatoryTemplate}, nil
}
