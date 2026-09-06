package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

var (
	ErrItemWorkspaceMoveSameWorkspace   = errors.New("item is already in the destination workspace")
	ErrItemWorkspaceMoveInvalidType     = errors.New("item type is not available in the destination workspace")
	ErrItemWorkspaceMoveInvalidStatus   = errors.New("status is not available for the destination item type")
	ErrItemWorkspaceMoveInvalidPriority = errors.New("priority is not available in the destination workspace")
)

type ItemWorkspaceMoveInput struct {
	DestinationWorkspaceID int  `json:"destination_workspace_id"`
	TargetItemTypeID       int  `json:"target_item_type_id,omitempty"`
	TargetStatusID         int  `json:"target_status_id,omitempty"`
	TargetPriorityID       *int `json:"target_priority_id"`
}

type ItemWorkspaceMoveOption struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
	Color     string `json:"color,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type ItemWorkspaceMoveField struct {
	Field  string `json:"field"`
	Action string `json:"action"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}

type ItemWorkspaceMovePreview struct {
	SourceWorkspaceID        int                       `json:"source_workspace_id"`
	SourceWorkspaceName      string                    `json:"source_workspace_name"`
	SourceKey                string                    `json:"source_key"`
	DestinationWorkspaceID   int                       `json:"destination_workspace_id"`
	DestinationWorkspaceName string                    `json:"destination_workspace_name"`
	DestinationWorkspaceKey  string                    `json:"destination_workspace_key"`
	TargetItemTypeID         int                       `json:"target_item_type_id"`
	TargetStatusID           int                       `json:"target_status_id"`
	TargetPriorityID         *int                      `json:"target_priority_id"`
	ItemTypes                []ItemWorkspaceMoveOption `json:"item_types"`
	Statuses                 []ItemWorkspaceMoveOption `json:"statuses"`
	Priorities               []ItemWorkspaceMoveOption `json:"priorities"`
	Fields                   []ItemWorkspaceMoveField  `json:"fields"`
	LabelsKept               []string                  `json:"labels_kept"`
	LabelsDropped            []string                  `json:"labels_dropped"`
	CustomFieldsKept         []string                  `json:"custom_fields_kept"`
	CustomFieldsDropped      []string                  `json:"custom_fields_dropped"`
	ChildrenDetached         int                       `json:"children_detached"`
}

type ItemWorkspaceMoveResult struct {
	Item             *models.Item              `json:"item"`
	OldKey           string                    `json:"old_key"`
	NewKey           string                    `json:"new_key"`
	Preview          *ItemWorkspaceMovePreview `json:"preview"`
	DetachedChildIDs []int                     `json:"-"`
}

type itemMoveSnapshot struct {
	ID                  int
	WorkspaceID         int
	WorkspaceItemNumber int
	WorkspaceName       string
	WorkspaceKey        string
	ItemTypeID          *int
	ItemTypeName        string
	StatusID            *int
	StatusName          string
	PriorityID          *int
	PriorityName        string
	IterationID         *int
	ProjectID           *int
	TimeProjectID       *int
	ParentID            *int
	ChannelID           *int
	RequestTypeID       *int
	CustomFieldValues   map[string]any
	Path                string
	IsTask              bool
}

type ItemWorkspaceMoveService struct {
	db database.Database
}

func NewItemWorkspaceMoveService(db database.Database) *ItemWorkspaceMoveService {
	return &ItemWorkspaceMoveService{db: db}
}

func (s *ItemWorkspaceMoveService) Preview(itemID, actorUserID int, input ItemWorkspaceMoveInput) (*ItemWorkspaceMovePreview, error) {
	item, err := s.loadSnapshot(itemID)
	if err != nil {
		return nil, err
	}
	if input.DestinationWorkspaceID == item.WorkspaceID {
		return nil, ErrItemWorkspaceMoveSameWorkspace
	}

	destinationName, destinationKey, err := s.loadDestination(input.DestinationWorkspaceID)
	if err != nil {
		return nil, err
	}
	itemTypes, err := s.listDestinationItemTypes(input.DestinationWorkspaceID)
	if err != nil {
		return nil, err
	}
	if len(itemTypes) == 0 {
		return nil, ErrItemWorkspaceMoveInvalidType
	}
	targetTypeID := pickMoveOption(input.TargetItemTypeID, item.ItemTypeID, itemTypes)
	if targetTypeID == 0 {
		return nil, ErrItemWorkspaceMoveInvalidType
	}

	statuses, err := s.listDestinationStatuses(input.DestinationWorkspaceID, targetTypeID)
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, ErrItemWorkspaceMoveInvalidStatus
	}
	targetStatusID := pickMoveOption(input.TargetStatusID, item.StatusID, statuses)
	if targetStatusID == 0 {
		return nil, ErrItemWorkspaceMoveInvalidStatus
	}
	if err := validation.ValidateTaskState(s.db, input.DestinationWorkspaceID, actorUserID, item.IsTask, &targetStatusID); err != nil {
		return nil, err
	}

	priorities, err := s.listDestinationPriorities(input.DestinationWorkspaceID)
	if err != nil {
		return nil, err
	}
	targetPriorityID := input.TargetPriorityID
	if input.TargetItemTypeID == 0 && input.TargetStatusID == 0 && input.TargetPriorityID == nil {
		targetPriorityID = pickOptionalMoveOption(item.PriorityID, priorities)
	}
	if targetPriorityID != nil && !containsMoveOption(priorities, *targetPriorityID) {
		return nil, ErrItemWorkspaceMoveInvalidPriority
	}

	preview := &ItemWorkspaceMovePreview{
		SourceWorkspaceID: item.WorkspaceID, SourceWorkspaceName: item.WorkspaceName,
		SourceKey:                fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
		DestinationWorkspaceID:   input.DestinationWorkspaceID,
		DestinationWorkspaceName: destinationName, DestinationWorkspaceKey: destinationKey,
		TargetItemTypeID: targetTypeID, TargetStatusID: targetStatusID, TargetPriorityID: targetPriorityID,
		ItemTypes: itemTypes, Statuses: statuses, Priorities: priorities,
	}
	if err := s.populatePreviewMappings(preview, item); err != nil {
		return nil, err
	}
	return preview, nil
}

func pickMoveOption(requested int, current *int, options []ItemWorkspaceMoveOption) int {
	if requested > 0 {
		if containsMoveOption(options, requested) {
			return requested
		}
		return 0
	}
	if current != nil && containsMoveOption(options, *current) {
		return *current
	}
	for _, option := range options {
		if option.IsDefault {
			return option.ID
		}
	}
	return options[0].ID
}

func pickOptionalMoveOption(current *int, options []ItemWorkspaceMoveOption) *int {
	if current != nil && containsMoveOption(options, *current) {
		value := *current
		return &value
	}
	for _, option := range options {
		if option.IsDefault {
			value := option.ID
			return &value
		}
	}
	return nil
}

func containsMoveOption(options []ItemWorkspaceMoveOption, id int) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

func (s *ItemWorkspaceMoveService) loadSnapshot(itemID int) (*itemMoveSnapshot, error) {
	var out itemMoveSnapshot
	var itemTypeID, statusID, priorityID, iterationID, projectID, timeProjectID sql.NullInt64
	var parentID, channelID, requestTypeID sql.NullInt64
	var itemTypeName, statusName, priorityName, customJSON sql.NullString
	err := s.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, w.name, w.key,
		       i.item_type_id, it.name, i.status_id, st.name, i.priority_id, p.name,
		       i.iteration_id, i.project_id, i.time_project_id, i.parent_id,
		       i.channel_id, i.request_type_id, i.custom_field_values, COALESCE(i.path, '/'), i.is_task
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		LEFT JOIN item_types it ON it.id = i.item_type_id
		LEFT JOIN statuses st ON st.id = i.status_id
		LEFT JOIN priorities p ON p.id = i.priority_id
		WHERE i.id = ?
	`, itemID).Scan(&out.ID, &out.WorkspaceID, &out.WorkspaceItemNumber, &out.WorkspaceName, &out.WorkspaceKey,
		&itemTypeID, &itemTypeName, &statusID, &statusName, &priorityID, &priorityName,
		&iterationID, &projectID, &timeProjectID, &parentID, &channelID, &requestTypeID, &customJSON, &out.Path, &out.IsTask)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load item move snapshot: %w", err)
	}
	out.ItemTypeID, out.StatusID, out.PriorityID = nullableMoveInt(itemTypeID), nullableMoveInt(statusID), nullableMoveInt(priorityID)
	out.IterationID, out.ProjectID, out.TimeProjectID = nullableMoveInt(iterationID), nullableMoveInt(projectID), nullableMoveInt(timeProjectID)
	out.ParentID, out.ChannelID, out.RequestTypeID = nullableMoveInt(parentID), nullableMoveInt(channelID), nullableMoveInt(requestTypeID)
	out.ItemTypeName, out.StatusName, out.PriorityName = itemTypeName.String, statusName.String, priorityName.String
	out.CustomFieldValues = map[string]any{}
	if customJSON.Valid && strings.TrimSpace(customJSON.String) != "" {
		if err := json.Unmarshal([]byte(customJSON.String), &out.CustomFieldValues); err != nil {
			return nil, fmt.Errorf("decode item custom fields: %w", err)
		}
	}
	return &out, nil
}

func nullableMoveInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func (s *ItemWorkspaceMoveService) loadDestination(workspaceID int) (name, key string, err error) {
	err = s.db.QueryRow(`SELECT name, key FROM workspaces WHERE id = ? AND active = true`, workspaceID).Scan(&name, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", repository.ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("load destination workspace: %w", err)
	}
	return name, key, nil
}

func (s *ItemWorkspaceMoveService) listDestinationItemTypes(workspaceID int) ([]ItemWorkspaceMoveOption, error) {
	itemTypes, err := repository.NewItemTypeRepository(s.db).ListForWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list destination item types: %w", err)
	}
	sort.SliceStable(itemTypes, func(i, j int) bool {
		if itemTypes[i].IsDefault != itemTypes[j].IsDefault {
			return itemTypes[i].IsDefault
		}
		if itemTypes[i].HierarchyLevel != itemTypes[j].HierarchyLevel {
			return itemTypes[i].HierarchyLevel < itemTypes[j].HierarchyLevel
		}
		if itemTypes[i].SortOrder != itemTypes[j].SortOrder {
			return itemTypes[i].SortOrder < itemTypes[j].SortOrder
		}
		return itemTypes[i].Name < itemTypes[j].Name
	})
	options := []ItemWorkspaceMoveOption{}
	for _, itemType := range itemTypes {
		if itemType.HierarchyLevel == -1 {
			continue
		}
		options = append(options, ItemWorkspaceMoveOption{
			ID: itemType.ID, Name: itemType.Name, Icon: itemType.Icon,
			Color: itemType.Color, IsDefault: itemType.IsDefault,
		})
	}
	return options, nil
}

func (s *ItemWorkspaceMoveService) listDestinationStatuses(workspaceID, itemTypeID int) ([]ItemWorkspaceMoveOption, error) {
	workflowID, mapped, err := repository.NewConfigurationSetRepository(s.db).MappedItemTypeWorkflow(workspaceID, itemTypeID)
	if err != nil {
		return nil, fmt.Errorf("resolve destination workflow: %w", err)
	}
	if !mapped || workflowID == nil {
		return s.listAllStatuses()
	}
	statuses, err := repository.NewStatusRepository(s.db).ListForWorkflow(*workflowID)
	if err != nil {
		return nil, fmt.Errorf("list destination statuses: %w", err)
	}
	return moveOptionsFromStatuses(statuses), nil
}

func (s *ItemWorkspaceMoveService) listAllStatuses() ([]ItemWorkspaceMoveOption, error) {
	statuses, err := repository.NewStatusRepository(s.db).List()
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	options := moveOptionsFromStatuses(statuses)
	sortMoveOptionsDefaultName(options)
	return options, nil
}

func (s *ItemWorkspaceMoveService) listDestinationPriorities(workspaceID int) ([]ItemWorkspaceMoveOption, error) {
	priorities, err := repository.NewPriorityRepository(s.db).ListForWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list destination priorities: %w", err)
	}
	sort.SliceStable(priorities, func(i, j int) bool {
		if priorities[i].IsDefault != priorities[j].IsDefault {
			return priorities[i].IsDefault
		}
		if priorities[i].SortOrder != priorities[j].SortOrder {
			return priorities[i].SortOrder < priorities[j].SortOrder
		}
		return priorities[i].Name < priorities[j].Name
	})
	options := make([]ItemWorkspaceMoveOption, 0, len(priorities))
	for _, priority := range priorities {
		options = append(options, ItemWorkspaceMoveOption{
			ID: priority.ID, Name: priority.Name, Icon: priority.Icon,
			Color: priority.Color, IsDefault: priority.IsDefault,
		})
	}
	return options, nil
}

func moveOptionsFromStatuses(statuses []models.Status) []ItemWorkspaceMoveOption {
	options := make([]ItemWorkspaceMoveOption, 0, len(statuses))
	for _, status := range statuses {
		options = append(options, ItemWorkspaceMoveOption{
			ID: status.ID, Name: status.Name, Color: status.CategoryColor,
			IsDefault: status.IsDefault,
		})
	}
	return options
}

func sortMoveOptionsDefaultName(options []ItemWorkspaceMoveOption) {
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].IsDefault != options[j].IsDefault {
			return options[i].IsDefault
		}
		return options[i].Name < options[j].Name
	})
}

func optionName(options []ItemWorkspaceMoveOption, id int) string {
	for _, option := range options {
		if option.ID == id {
			return option.Name
		}
	}
	return ""
}

func displayMoveValue(id *int, name string) string {
	if id == nil {
		return "None"
	}
	if name != "" {
		return name
	}
	return strconv.Itoa(*id)
}

func (s *ItemWorkspaceMoveService) populatePreviewMappings(preview *ItemWorkspaceMovePreview, item *itemMoveSnapshot) error {
	keptLabels, droppedLabels, err := s.previewLabels(item.ID)
	if err != nil {
		return err
	}
	preview.LabelsKept, preview.LabelsDropped = keptLabels, droppedLabels

	_, keptCustom, droppedCustom, err := s.destinationCustomFields(item.CustomFieldValues, preview.DestinationWorkspaceID, preview.TargetItemTypeID)
	if err != nil {
		return err
	}
	preview.CustomFieldsKept, preview.CustomFieldsDropped = keptCustom, droppedCustom

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id = ?`, item.ID).Scan(&preview.ChildrenDetached); err != nil {
		return fmt.Errorf("count children for move preview: %w", err)
	}

	priorityName := "None"
	if preview.TargetPriorityID != nil {
		priorityName = optionName(preview.Priorities, *preview.TargetPriorityID)
	}
	preview.Fields = []ItemWorkspaceMoveField{
		{Field: "workspace", Action: "map", From: item.WorkspaceName, To: preview.DestinationWorkspaceName},
		{Field: "key", Action: "map", From: preview.SourceKey, To: preview.DestinationWorkspaceKey + "-(assigned on confirmation)"},
		{Field: "item_type", Action: "map", From: displayMoveValue(item.ItemTypeID, item.ItemTypeName), To: optionName(preview.ItemTypes, preview.TargetItemTypeID)},
		{Field: "status", Action: "map", From: displayMoveValue(item.StatusID, item.StatusName), To: optionName(preview.Statuses, preview.TargetStatusID)},
		{Field: "priority", Action: moveAction(item.PriorityID != nil, preview.TargetPriorityID != nil), From: displayMoveValue(item.PriorityID, item.PriorityName), To: priorityName},
		{Field: "parent", Action: "drop", From: presenceMoveValue(item.ParentID), To: "Workspace root"},
		{Field: "children", Action: childrenMoveAction(preview.ChildrenDetached), From: strconv.Itoa(preview.ChildrenDetached), To: "Detached to source workspace root"},
		{Field: "labels", Action: collectionMoveAction(len(keptLabels), len(droppedLabels)), From: strings.Join(append(append([]string{}, keptLabels...), droppedLabels...), ", "), To: strings.Join(keptLabels, ", ")},
		{Field: "custom_fields", Action: collectionMoveAction(len(keptCustom), len(droppedCustom)), From: strings.Join(append(append([]string{}, keptCustom...), droppedCustom...), ", "), To: strings.Join(keptCustom, ", ")},
		{Field: "iteration", Action: "drop", From: presenceMoveValue(item.IterationID), To: "None"},
		{Field: "milestones", Action: "drop", From: "Current assignments", To: "None"},
		{Field: "project", Action: "drop", From: presenceMoveValue(item.ProjectID), To: "None"},
		{Field: "time_project", Action: "drop", From: presenceMoveValue(item.TimeProjectID), To: "None"},
		{Field: "channel", Action: "drop", From: presenceMoveValue(item.ChannelID), To: "None"},
		{Field: "request_type", Action: "drop", From: presenceMoveValue(item.RequestTypeID), To: "None"},
		{Field: "calendar_schedule", Action: "drop", From: "Current schedule", To: "None"},
		{Field: "approvals", Action: "drop", From: "Current requests", To: "None"},
		{Field: "recurrence", Action: "map", From: item.WorkspaceKey, To: preview.DestinationWorkspaceKey},
		{Field: "comments_attachments_worklogs_links_watches", Action: "keep", From: "Item-scoped", To: "Unchanged"},
		{Field: "assignee_creator_reporter_relations", Action: "keep", From: "Item-scoped", To: "Unchanged"},
	}
	return nil
}

func presenceMoveValue(value *int) string {
	if value == nil {
		return "None"
	}
	return strconv.Itoa(*value)
}

func moveAction(from, to bool) string {
	if from && !to {
		return "drop"
	}
	if !from && !to {
		return "keep"
	}
	return "map"
}

func childrenMoveAction(count int) string {
	if count == 0 {
		return "keep"
	}
	return "detach"
}

func collectionMoveAction(kept, dropped int) string {
	if dropped > 0 && kept > 0 {
		return "partial"
	}
	if dropped > 0 {
		return "drop"
	}
	return "keep"
}

func (s *ItemWorkspaceMoveService) previewLabels(itemID int) (kept, dropped []string, err error) {
	rows, err := s.db.Query(`
		SELECT labels.name
		FROM item_labels il
		JOIN labels ON labels.id = il.label_id
		WHERE il.item_id = ?
		ORDER BY labels.name
	`, itemID)
	if err != nil {
		return nil, nil, fmt.Errorf("preview item labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	kept = []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, nil, err
		}
		kept = append(kept, name)
	}
	return kept, []string{}, rows.Err()
}

func (s *ItemWorkspaceMoveService) destinationCustomFields(values map[string]any, workspaceID, itemTypeID int) (keptValues map[string]any, kept, dropped []string, err error) {
	if len(values) == 0 {
		return map[string]any{}, []string{}, []string{}, nil
	}
	screenRepo := repository.NewScreenRepository(s.db)
	screenID, err := screenRepo.GetEffectiveCreateScreenID(workspaceID, itemTypeID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve destination screen: %w", err)
	}

	allowed := map[string]string{}
	if screenID != nil {
		fields, err := screenRepo.ListFields(*screenID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list destination custom fields: %w", err)
		}
		for _, field := range fields {
			if field.FieldType == "custom" {
				allowed[field.FieldIdentifier] = field.FieldName
			}
		}
	}

	names := map[string]string{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var name string
		if err := s.db.QueryRow(`SELECT name FROM custom_field_definitions WHERE CAST(id AS TEXT) = ?`, key).Scan(&name); err == nil {
			names[key] = name
		} else {
			names[key] = key
		}
	}

	keptValues = map[string]any{}
	kept, dropped = []string{}, []string{}
	for _, key := range keys {
		if name, ok := allowed[key]; ok {
			keptValues[key] = values[key]
			kept = append(kept, name)
		} else {
			dropped = append(dropped, names[key])
		}
	}
	return keptValues, kept, dropped, nil
}

func (s *ItemWorkspaceMoveService) Move(itemID, actorUserID int, input ItemWorkspaceMoveInput) (*ItemWorkspaceMoveResult, error) {
	return s.MoveContext(context.Background(), itemID, actorUserID, input)
}

// MoveContext moves an item while honoring cancellation from the caller.
func (s *ItemWorkspaceMoveService) MoveContext(ctx context.Context, itemID, actorUserID int, input ItemWorkspaceMoveInput) (*ItemWorkspaceMoveResult, error) {
	if input.TargetItemTypeID <= 0 {
		return nil, ErrItemWorkspaceMoveInvalidType
	}
	if input.TargetStatusID <= 0 {
		return nil, ErrItemWorkspaceMoveInvalidStatus
	}
	preview, err := s.Preview(itemID, actorUserID, input)
	if err != nil {
		return nil, err
	}
	item, err := s.loadSnapshot(itemID)
	if err != nil {
		return nil, err
	}
	if err := s.validateWorkflowGuards(ctx, item, preview); err != nil {
		return nil, err
	}
	customValues, _, _, err := s.destinationCustomFields(item.CustomFieldValues, input.DestinationWorkspaceID, input.TargetItemTypeID)
	if err != nil {
		return nil, err
	}
	customJSON, err := json.Marshal(customValues)
	if err != nil {
		return nil, fmt.Errorf("encode destination custom fields: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin item workspace move: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	itemRepo := repository.NewItemRepository(s.db)
	originalInTx, err := itemRepo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return nil, err
	}
	integrationLinkUnavailable, err := NewIntegrationLinkGuards(s.db).HasLinkUnavailableInWorkspaceTx(tx, itemID, input.DestinationWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("validate integration links for workspace move: %w", err)
	}
	if integrationLinkUnavailable {
		return nil, &validation.ValidationError{
			Field:   "destination_workspace_id",
			Message: "Destination workspace is not allowed by every provider-managed integration link",
		}
	}

	newNumber, err := itemRepo.GetNextWorkspaceItemNumber(tx, input.DestinationWorkspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if _, err := tx.Exec(`
		INSERT INTO item_key_reservations (
			workspace_id, workspace_item_number, moved_item_id,
			destination_workspace_id, destination_workspace_item_number, moved_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, item.WorkspaceID, item.WorkspaceItemNumber, itemID, input.DestinationWorkspaceID, newNumber, actorUserID, now); err != nil {
		return nil, fmt.Errorf("reserve old item key: %w", err)
	}

	childIDs, err := detachMoveChildren(tx, itemID, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM item_milestones WHERE item_id = ?`, itemID); err != nil {
		return nil, fmt.Errorf("clear milestones: %w", err)
	}
	if _, err := tx.Exec(`UPDATE recurrence_rules SET workspace_id = ?, status_on_create = NULL, updated_at = ? WHERE template_item_id = ?`, input.DestinationWorkspaceID, now, itemID); err != nil {
		return nil, fmt.Errorf("remap recurrence: %w", err)
	}

	priorityValue := any(nil)
	if input.TargetPriorityID != nil {
		priorityValue = *input.TargetPriorityID
	}
	result, err := tx.Exec(`
		UPDATE items
		SET workspace_id = ?, workspace_item_number = ?, item_type_id = ?, status_id = ?, priority_id = ?,
		    iteration_id = NULL, project_id = NULL, time_project_id = NULL, inherit_project = false,
		    custom_field_values = ?, parent_id = NULL, path = '/', channel_id = NULL,
		    request_type_id = NULL, calendar_data = NULL, updated_at = ?, last_active_at = ?
		WHERE id = ? AND workspace_id = ? AND workspace_item_number = ?
	`, input.DestinationWorkspaceID, newNumber, input.TargetItemTypeID, input.TargetStatusID, priorityValue,
		string(customJSON), now, now, itemID, item.WorkspaceID, item.WorkspaceItemNumber)
	if err != nil {
		return nil, fmt.Errorf("move item to workspace: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected != 1 {
		return nil, fmt.Errorf("item changed while move was being confirmed")
	}

	newKey := fmt.Sprintf("%s-%d", preview.DestinationWorkspaceKey, newNumber)
	historyJSON, err := json.Marshal(map[string]any{
		"old_key": preview.SourceKey, "new_key": newKey, "fields": preview.Fields,
		"labels_kept": preview.LabelsKept, "labels_dropped": preview.LabelsDropped,
		"custom_fields_kept": preview.CustomFieldsKept, "custom_fields_dropped": preview.CustomFieldsDropped,
	})
	if err != nil {
		return nil, fmt.Errorf("encode move history: %w", err)
	}
	history := repository.HistoryEntry{
		ItemID: itemID, UserID: actorUserID, FieldName: "workspace_move",
		OldValue: preview.SourceKey, NewValue: string(historyJSON), ChangedAt: now,
	}
	if err := itemRepo.RecordHistory(tx, history); err != nil {
		return nil, err
	}
	updatedInTx, err := itemRepo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return nil, err
	}
	metadata := itemevents.User(actorUserID, "application")
	metadata.OccurredAt = now
	recorder := itemevents.NewRecorder(s.db)
	for _, childID := range childIDs {
		child, err := itemRepo.FindByIDForUpdate(tx, childID)
		if err != nil {
			return nil, err
		}
		if _, err := recorder.Updated(ctx, tx, child, []itemevents.FieldChange{{
			Field: "parent_id", OldValue: itemID, NewValue: nil,
		}}, metadata); err != nil {
			return nil, err
		}
	}
	changes := itemevents.Changes(originalInTx, updatedInTx)
	if originalInTx.StatusID != nil && *originalInTx.StatusID != input.TargetStatusID {
		newStatusID := input.TargetStatusID
		if _, err := recorder.StatusChanged(ctx, tx, updatedInTx, originalInTx.StatusID, &newStatusID, changes, metadata); err != nil {
			return nil, err
		}
	} else if _, err := recorder.Updated(ctx, tx, updatedInTx, changes, metadata); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit item workspace move: %w", err)
	}
	repository.InvalidateItemListCountCache(s.db, item.WorkspaceID)
	repository.InvalidateItemListCountCache(s.db, input.DestinationWorkspaceID)

	updated, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	items := []models.Item{*updated}
	if err := repository.NewLabelRepository(s.db).LoadForItems(items); err != nil {
		return nil, err
	}
	updated = &items[0]
	PublishItemChange(itemID, ItemChangeUpdated)
	for _, childID := range childIDs {
		PublishItemChange(childID, ItemChangeUpdated)
	}
	return &ItemWorkspaceMoveResult{
		Item:             updated,
		OldKey:           preview.SourceKey,
		NewKey:           newKey,
		Preview:          preview,
		DetachedChildIDs: childIDs,
	}, nil
}

func (s *ItemWorkspaceMoveService) validateWorkflowGuards(ctx context.Context, item *itemMoveSnapshot, preview *ItemWorkspaceMovePreview) error {
	guard := NewItemTypeChangeService(s.db)
	pending, err := guard.itemHasPendingApproval(item.ID)
	if err != nil {
		return fmt.Errorf("check pending approval before workspace move: %w", err)
	}
	if pending {
		return &validation.ValidationError{
			Field:   "destination_workspace_id",
			Message: "Cannot move an item while an approval is pending",
		}
	}
	if item.StatusID == nil || preview.TargetStatusID == *item.StatusID {
		return nil
	}

	targetTypeID := preview.TargetItemTypeID
	targetWorkflowID, err := guard.workflowService.GetWorkflowIDForItem(preview.DestinationWorkspaceID, &targetTypeID)
	if err != nil {
		return fmt.Errorf("resolve destination workflow: %w", err)
	}

	approvalBound, err := guard.statusIsApprovalBound(ctx, preview.DestinationWorkspaceID, targetTypeID, preview.TargetStatusID)
	if err != nil {
		return err
	}
	if approvalBound {
		return &validation.ValidationError{
			Field:   "target_status_id",
			Message: "Target status requires approval in the destination workspace",
		}
	}
	if targetWorkflowID == nil {
		return nil
	}

	initialStatusID, err := guard.workflowService.GetInitialStatusID(*targetWorkflowID)
	if err != nil {
		return err
	}
	if initialStatusID != nil && *initialStatusID == preview.TargetStatusID {
		return nil
	}

	transitionID, err := guard.findWorkflowTransitionID(*targetWorkflowID, *item.StatusID, preview.TargetStatusID)
	if err != nil {
		return err
	}
	if transitionID == nil {
		return &validation.ValidationError{
			Field:   "target_status_id",
			Message: "Target status is not reachable by a direct transition in the destination workflow",
		}
	}

	conditionSetID, err := guard.resolveConditionSetIDForItemType(preview.DestinationWorkspaceID, targetTypeID)
	if err != nil {
		return err
	}
	if conditionSetID == nil {
		return nil
	}
	hasConditions, err := guard.transitionHasConditions(*conditionSetID, *transitionID)
	if err != nil {
		return err
	}
	if hasConditions {
		return &validation.ValidationError{
			Field:   "target_status_id",
			Message: "Target status transition has conditions in the destination workspace",
		}
	}
	return nil
}

func detachMoveChildren(tx database.Tx, itemID int, changedAt time.Time) ([]int, error) {
	rows, err := tx.Query(`SELECT id, COALESCE(path, '/') FROM items WHERE parent_id = ? ORDER BY id`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list children to detach: %w", err)
	}
	type childPath struct {
		id   int
		path string
	}
	children := []childPath{}
	for rows.Next() {
		var child childPath
		if err := rows.Scan(&child.id, &child.path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(children))
	for _, child := range children {
		oldPrefix := child.path + strconv.Itoa(child.id) + "/"
		newPrefix := "/" + strconv.Itoa(child.id) + "/"
		if _, err := tx.Exec(`UPDATE items SET path = ? || SUBSTR(path, ?) WHERE path LIKE ?`, newPrefix, len(oldPrefix)+1, oldPrefix+"%"); err != nil {
			return nil, fmt.Errorf("rewrite detached child descendants: %w", err)
		}
		if _, err := tx.Exec(`UPDATE items SET parent_id = NULL, path = '/', updated_at = ? WHERE id = ?`, changedAt, child.id); err != nil {
			return nil, fmt.Errorf("detach child: %w", err)
		}
		ids = append(ids, child.id)
	}
	return ids, nil
}
