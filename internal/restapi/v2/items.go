package v2

import (
	"cmp"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/validation"
)

const maxItemBatch = 500

type idBatchRequest struct {
	IDs []int `json:"ids"`
}

type itemListMeta struct {
	NextCursor     string   `json:"next_cursor,omitempty"`
	Watermark      int64    `json:"watermark,omitempty"`
	SortableFields []string `json:"sortable_fields"`
}

type transitionMatrixEntry struct {
	ItemTypeID  int                             `json:"item_type_id"`
	StatusID    int                             `json:"status_id"`
	Transitions []services.ItemTransitionOption `json:"transitions"`
}

type itemCreateRequest struct {
	WorkspaceID       int            `json:"workspace_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	StatusID          *int           `json:"status_id"`
	PriorityID        *int           `json:"priority_id"`
	ItemTypeID        *int           `json:"item_type_id"`
	DueDate           *time.Time     `json:"due_date"`
	StartDate         *time.Time     `json:"start_date"`
	EndDate           *time.Time     `json:"end_date"`
	IsTask            bool           `json:"is_task"`
	IterationID       *int           `json:"iteration_id"`
	ProjectID         *int           `json:"project_id"`
	InheritProject    bool           `json:"inherit_project"`
	TimeProjectID     *int           `json:"time_project_id"`
	AssigneeID        *int           `json:"assignee_id"`
	ParentID          *int           `json:"parent_id"`
	RelatedWorkItemID *int           `json:"related_work_item_id"`
	StoryPoints       *float64       `json:"story_points"`
	EstimateMinutes   *int           `json:"estimate_minutes"`
	CustomFieldValues map[string]any `json:"custom_field_values"`
	MilestoneIDs      []int          `json:"milestone_ids"`
	LabelIDs          []int          `json:"label_ids"`
}

type itemPatchRequest struct {
	Title             Optional[string]         `json:"title"`
	Description       Optional[string]         `json:"description"`
	PriorityID        Optional[int]            `json:"priority_id"`
	AssigneeID        Optional[int]            `json:"assignee_id"`
	ParentID          Optional[int]            `json:"parent_id"`
	IterationID       Optional[int]            `json:"iteration_id"`
	ProjectID         Optional[int]            `json:"project_id"`
	MilestoneIDs      Optional[[]int]          `json:"milestone_ids"`
	DueDate           Optional[time.Time]      `json:"due_date"`
	StartDate         Optional[time.Time]      `json:"start_date"`
	EndDate           Optional[time.Time]      `json:"end_date"`
	IsTask            Optional[bool]           `json:"is_task"`
	CustomFieldValues Optional[map[string]any] `json:"custom_field_values"`
}

type itemWatchRequest struct {
	Reason string `json:"reason"`
}

type itemReparentRequest struct {
	ParentID *int `json:"parent_id"`
}

type itemRankRequest struct {
	PreviousItemID *int `json:"previous_item_id"`
	NextItemID     *int `json:"next_item_id"`
}

type itemTransitionRequest struct {
	ToStatusID int `json:"to_status_id"`
}

type itemTypeChangeRequest struct {
	TargetItemTypeID int  `json:"target_item_type_id"`
	TargetStatusID   *int `json:"target_status_id"`
}

type itemBulkUpdateRequest struct {
	ItemIDs []int          `json:"item_ids"`
	Set     map[string]any `json:"set"`
}

type itemBulkPatchRequest struct {
	Patches []struct {
		ItemID int            `json:"item_id"`
		Set    map[string]any `json:"set"`
	} `json:"patches"`
}

type roadmapHierarchyDatesRequest struct {
	RootIDs []int `json:"root_ids"`
}

func registerItemRoutes(builder *routeBuilder, app *services.ItemApplicationService, detail *services.ItemDetailApplicationService) {
	collection := "/items"
	builder.PageMetadata(collection, AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, Pagination, int, itemListMeta, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		page, request, err := parseItemList(r, user.ID)
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		result, err := app.List(r.Context(), request)
		return result.Items, page, result.Total, itemListMeta{
			NextCursor: result.NextCursor, Watermark: result.Watermark, SortableFields: result.SortableFields,
		}, itemError(err)
	})
	builder.JSON(http.MethodPost, collection, http.StatusCreated, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemCreateRequest) (*models.Item, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		result, err := app.Create(r.Context(), auditActor(r, user), services.ItemCreateInput{
			WorkspaceID: input.WorkspaceID, Title: input.Title, Description: input.Description,
			StatusID: input.StatusID, PriorityID: input.PriorityID, ItemTypeID: input.ItemTypeID,
			DueDate: input.DueDate, StartDate: input.StartDate, EndDate: input.EndDate,
			IsTask: input.IsTask, IterationID: input.IterationID, ProjectID: input.ProjectID,
			InheritProject: input.InheritProject, TimeProjectID: input.TimeProjectID,
			AssigneeID: input.AssigneeID, ParentID: input.ParentID,
			RelatedWorkItemID: input.RelatedWorkItemID, StoryPoints: input.StoryPoints,
			EstimateMinutes: input.EstimateMinutes, CustomFieldValues: input.CustomFieldValues,
			MilestoneIDs: input.MilestoneIDs, LabelIDs: input.LabelIDs,
		})
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, collection+"/batch", http.StatusOK, false, AuthAuthenticated, []string{"items:read"}, func(r *http.Request, input idBatchRequest) ([]models.Item, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		ids, err := normalizeBatchIDs(input.IDs)
		if err != nil {
			return nil, err
		}
		result, err := app.Batch(r.Context(), user.ID, ids)
		return result, itemError(err)
	})
	builder.Read("/workspaces/{workspace_key}/items/{item_number}", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (*models.Item, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		number, err := pathID(r, "item_number")
		if err != nil {
			return nil, err
		}
		result, err := app.GetByKey(r.Context(), user.ID, strings.TrimSpace(r.PathValue("workspace_key")), number)
		return result, itemError(err)
	})
	builder.Read("/workspaces/{workspace_key}/items/{item_number}/detail-summary", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemDetailSummary, error) {
		user, err := principal(r)
		if err != nil {
			return services.ItemDetailSummary{}, err
		}
		number, err := pathID(r, "item_number")
		if err != nil {
			return services.ItemDetailSummary{}, err
		}
		result, err := detail.GetByKey(r.Context(), user.ID, strings.TrimSpace(r.PathValue("workspace_key")), number, r.URL.Query().Get("surface"))
		return result, itemError(err)
	})
	builder.Read(collection+"/{item_id}/detail-summary", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemDetailSummary, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemDetailSummary{}, err
		}
		result, err := detail.Get(r.Context(), user.ID, id, r.URL.Query().Get("surface"))
		return result, itemError(err)
	})
	builder.Read(collection+"/{item_id}", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (*models.Item, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		reference, err := parseItemReference(r.PathValue("item_id"))
		if err != nil {
			return nil, err
		}
		var result *models.Item
		if reference.ID > 0 {
			result, err = app.Get(r.Context(), user.ID, reference.ID, true)
		} else {
			result, err = app.GetByKey(r.Context(), user.ID, reference.WorkspaceKey, reference.ItemNumber)
		}
		return result, itemError(err)
	})
	builder.JSON(http.MethodPatch, collection+"/{item_id}", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemPatchRequest) (*models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.Patch(r.Context(), auditActor(r, user), id, itemPatchFields(input))
		return result, itemError(err)
	})
	builder.Command(http.MethodDelete, collection+"/{item_id}", AuthAuthenticated, []string{"items:delete"}, func(r *http.Request) error {
		user, id, err := itemTarget(r)
		if err != nil {
			return err
		}
		return itemError(app.Delete(auditActor(r, user), id))
	})
	registerItemReadRoutes(builder, app)
	registerItemSetRoutes(builder, app)
}

func registerItemSetRoutes(builder *routeBuilder, app *services.ItemApplicationService) {
	builder.PageMetadata("/items/backlog", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, Pagination, int, itemListMeta, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		workspaceID, err := optionalItemQueryID(r, "workspace_id")
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		collectionID, err := optionalItemQueryID(r, "collection_id")
		if err != nil {
			return nil, Pagination{}, 0, itemListMeta{}, err
		}
		if workspaceID == 0 && collectionID == 0 {
			return nil, Pagination{}, 0, itemListMeta{}, invalidQuery("workspace_id")
		}
		result, err := app.Backlog(r.Context(), services.ItemBacklogRequest{
			UserID: user.ID, WorkspaceID: workspaceID, CollectionID: collectionID,
			QL: r.URL.Query().Get("ql"), SubQL: r.URL.Query().Get("sub_ql"),
			Pagination:       services.PaginationParams{Page: page.Page, Limit: page.PageSize, Offset: page.Offset},
			OmitDescriptions: r.URL.Query().Get("fields") == "summary", IncludeWatermark: r.URL.Query().Get("include_watermark") == "true",
		})
		return result.Items, page, result.Total, itemListMeta{Watermark: result.Watermark, SortableFields: result.SortableFields}, itemError(err)
	})
	builder.Read("/items/changes", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemChangesResult, error) {
		user, err := principal(r)
		if err != nil {
			return services.ItemChangesResult{}, err
		}
		workspaceID, err := optionalItemQueryID(r, "workspace_id")
		if err != nil {
			return services.ItemChangesResult{}, err
		}
		collectionID, err := optionalItemQueryID(r, "collection_id")
		if err != nil {
			return services.ItemChangesResult{}, err
		}
		sinceRaw := strings.TrimSpace(r.URL.Query().Get("since"))
		var since int64
		if sinceRaw != "" {
			since, err = strconv.ParseInt(sinceRaw, 10, 64)
			if err != nil || since < 0 {
				return services.ItemChangesResult{}, invalidQuery("since")
			}
		}
		result, err := app.Changes(r.Context(), services.ItemChangesRequest{
			UserID: user.ID, WorkspaceID: workspaceID, CollectionID: collectionID,
			Since: since, SinceProvided: sinceRaw != "", SubQL: r.URL.Query().Get("sub_ql"),
		})
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, "/items/bulk-update", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemBulkUpdateRequest) (services.ItemBulkResult, error) {
		user, err := principal(r)
		if err != nil {
			return services.ItemBulkResult{}, err
		}
		result, err := app.BulkUpdate(r.Context(), auditActor(r, user), input.ItemIDs, input.Set)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, "/items/bulk-patch", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemBulkPatchRequest) (services.ItemBulkResult, error) {
		user, err := principal(r)
		if err != nil {
			return services.ItemBulkResult{}, err
		}
		patches := make([]services.BulkItemPatch, len(input.Patches))
		for i := range input.Patches {
			patches[i] = services.BulkItemPatch{ItemID: input.Patches[i].ItemID, Fields: input.Patches[i].Set}
		}
		result, err := app.BulkPatch(r.Context(), auditActor(r, user), patches)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, "/items/roadmap-hierarchy-dates", http.StatusOK, false, AuthAuthenticated, []string{"items:read"}, func(r *http.Request, input roadmapHierarchyDatesRequest) (services.RoadmapHierarchyDatesResult, error) {
		user, err := principal(r)
		if err != nil {
			return services.RoadmapHierarchyDatesResult{}, err
		}
		result, err := app.RoadmapHierarchyDates(r.Context(), user.ID, input.RootIDs)
		return result, itemError(err)
	})
}

func registerItemReadRoutes(builder *routeBuilder, app *services.ItemApplicationService) {
	path := "/items/{item_id}"
	builder.Read(path+"/children", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.Children(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Read(path+"/ancestors", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.Ancestors(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Read(path+"/descendants", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		maxDepth, err := parseNonNegativeQueryInt(r, "max_depth", 0)
		if err != nil {
			return nil, err
		}
		result, err := app.Descendants(r.Context(), user.ID, id, maxDepth)
		return result, itemError(err)
	})
	builder.Read(path+"/time-rollup", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (*models.TimeRollup, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		maxDepth, err := parsePositiveInt(r, "max_depth", 10, 30)
		if err != nil {
			return nil, err
		}
		result, err := app.TimeRollup(r.Context(), user.ID, id, maxDepth)
		return result, itemError(err)
	})
	builder.Read(path+"/history", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.ItemHistory, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.History(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Read(path+"/status-durations", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (*models.ItemStatusDurations, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.StatusDurations(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Read(path+"/watch", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemWatchStatus, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemWatchStatus{}, err
		}
		result, err := app.WatchStatus(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPut, path+"/watch", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemWatchRequest) (services.ItemWatchStatus, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemWatchStatus{}, err
		}
		result, err := app.Watch(r.Context(), user.ID, id, input.Reason)
		return result, itemError(err)
	})
	builder.Action(http.MethodDelete, path+"/watch", http.StatusOK, AuthAuthenticated, []string{"items:write"}, func(r *http.Request) (services.ItemWatchStatus, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemWatchStatus{}, err
		}
		result, err := app.Unwatch(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Read(path+"/personal-tasks", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.PersonalTasks(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.Command(http.MethodDelete, path+"/related-work-item", AuthAuthenticated, []string{"items:write"}, func(r *http.Request) error {
		user, id, err := itemTarget(r)
		if err != nil {
			return err
		}
		return itemError(app.UnlinkPersonalTask(user.ID, id))
	})
	builder.Read(path+"/delete-info", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemDeleteInfo, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemDeleteInfo{}, err
		}
		result, err := app.DeleteInfo(user.ID, id)
		return result, itemError(err)
	})
	builder.Action(http.MethodPost, path+"/cascade-deletion", http.StatusOK, AuthAuthenticated, []string{"items:delete"}, func(r *http.Request) (services.ItemMutationCount, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemMutationCount{}, err
		}
		result, err := app.DeleteCascade(auditActor(r, user), id)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, path+"/reparent-children", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemReparentRequest) (services.ItemMutationCount, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemMutationCount{}, err
		}
		result, err := app.ReparentChildren(r.Context(), auditActor(r, user), id, input.ParentID)
		return result, itemError(err)
	})
	builder.Action(http.MethodPost, path+"/copy", http.StatusCreated, AuthAuthenticated, []string{"items:write"}, func(r *http.Request) (*models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.Copy(r.Context(), auditActor(r, user), id)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, path+"/move-workspace/preview", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input services.ItemWorkspaceMoveInput) (*services.ItemWorkspaceMovePreview, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.MovePreview(user.ID, id, input)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, path+"/move-workspace", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input services.ItemWorkspaceMoveInput) (*services.ItemWorkspaceMoveResult, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.MoveWorkspace(r.Context(), auditActor(r, user), id, input)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPatch, path+"/rank", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemRankRequest) (*models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateRank(r.Context(), auditActor(r, user), id, services.ItemRankInput{PreviousItemID: input.PreviousItemID, NextItemID: input.NextItemID})
		return result, itemError(err)
	})
	builder.Read(path+"/available-transitions", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (services.ItemTransitionSummary, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return services.ItemTransitionSummary{}, err
		}
		result, err := app.AvailableTransitions(r.Context(), user.ID, id)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, path+"/transition", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemTransitionRequest) (*services.ItemTransitionResult, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		if input.ToStatusID < 1 {
			return nil, invalidQuery("to_status_id")
		}
		result, err := app.Transition(r.Context(), auditActor(r, user), id, input.ToStatusID)
		return result, itemError(err)
	})
	builder.Read(path+"/type-change-analysis", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) (*services.ItemTypeChangeAnalysis, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		if r.URL.Query().Get("target_item_type_id") == "" {
			return nil, invalidQuery("target_item_type_id")
		}
		targetTypeID, err := parsePositiveInt(r, "target_item_type_id", 1, int(^uint(0)>>1))
		if err != nil {
			return nil, err
		}
		result, err := app.AnalyzeTypeChange(user.ID, id, targetTypeID)
		return result, itemError(err)
	})
	builder.JSON(http.MethodPost, path+"/change-type", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input itemTypeChangeRequest) (*models.Item, error) {
		user, id, err := itemTarget(r)
		if err != nil {
			return nil, err
		}
		if input.TargetItemTypeID < 1 {
			return nil, invalidQuery("target_item_type_id")
		}
		result, err := app.ChangeType(r.Context(), auditActor(r, user), id, services.ItemTypeChangeInput{TargetItemTypeID: input.TargetItemTypeID, TargetStatusID: input.TargetStatusID})
		return result, itemError(err)
	})
	builder.Read("/workspaces/{workspace_id}/transition-matrix", AuthAuthenticated, []string{"items:read"}, func(r *http.Request) ([]transitionMatrixEntry, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		result, err := app.TransitionMatrix(r.Context(), user.ID, workspaceID)
		if err != nil {
			return nil, itemError(err)
		}
		entries := make([]transitionMatrixEntry, 0, len(result))
		for key, transitions := range result {
			left, right, ok := strings.Cut(key, ":")
			if !ok {
				return nil, internalError(errors.New("invalid transition matrix key"))
			}
			itemTypeID, itemTypeErr := strconv.Atoi(left)
			statusID, statusErr := strconv.Atoi(right)
			if itemTypeErr != nil || statusErr != nil {
				return nil, internalError(errors.New("invalid transition matrix key"))
			}
			entries = append(entries, transitionMatrixEntry{ItemTypeID: itemTypeID, StatusID: statusID, Transitions: transitions})
		}
		slices.SortFunc(entries, func(a, b transitionMatrixEntry) int {
			if byType := cmp.Compare(a.ItemTypeID, b.ItemTypeID); byType != 0 {
				return byType
			}
			return cmp.Compare(a.StatusID, b.StatusID)
		})
		return entries, nil
	})
}

func itemTarget(r *http.Request) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, "item_id")
	return user, id, err
}

type itemLookupReference struct {
	ID           int
	WorkspaceKey string
	ItemNumber   int
}

func parseItemReference(raw string) (itemLookupReference, error) {
	if looksLikeInteger(raw) {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 1 {
			return itemLookupReference{}, invalidItemReference("item_id must be a positive integer")
		}
		return itemLookupReference{ID: id}, nil
	}

	workspaceKey, itemNumberText, found := strings.Cut(raw, "-")
	itemNumber, err := strconv.Atoi(itemNumberText)
	if !found || !validWorkspaceKeyReference(workspaceKey) || err != nil || itemNumber < 1 {
		return itemLookupReference{}, invalidItemReference("item_id must be a positive integer or an item key in KEY-NUMBER format")
	}
	return itemLookupReference{WorkspaceKey: workspaceKey, ItemNumber: itemNumber}, nil
}

func looksLikeInteger(value string) bool {
	if value == "" {
		return false
	}
	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return false
	}
	for i := start; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func validWorkspaceKeyReference(value string) bool {
	if len(value) < 2 || len(value) > 10 {
		return false
	}
	for i := range len(value) {
		char := value[i]
		isDigit := char >= '0' && char <= '9'
		isUpper := char >= 'A' && char <= 'Z'
		isLower := char >= 'a' && char <= 'z'
		if !isDigit && !isUpper && !isLower {
			return false
		}
	}
	return true
}

func invalidItemReference(message string) error {
	err := newError(http.StatusBadRequest, "invalid_request", message)
	err.Details = map[string]any{"field": "item_id"}
	return err
}

func parseItemList(r *http.Request, userID int) (Pagination, services.ItemListRequest, error) {
	page, err := ParsePage(r)
	if err != nil {
		return Pagination{}, services.ItemListRequest{}, err
	}
	query := r.URL.Query()
	request := services.ItemListRequest{
		UserID: userID, QL: query.Get("ql"), SubQL: query.Get("sub_ql"),
		Pagination:       services.PaginationParams{Limit: page.PageSize, Offset: page.Offset, Cursor: query.Get("cursor"), CursorMode: query.Get("cursor") != ""},
		OmitDescriptions: query.Get("fields") == "summary",
		IncludeWatermark: query.Get("include_watermark") == "true",
	}
	for name, target := range map[string]*int{"workspace_id": &request.WorkspaceID, "collection_id": &request.CollectionID} {
		if query.Get(name) == "" {
			continue
		}
		value, parseErr := strconv.Atoi(query.Get(name))
		if parseErr != nil || value < 1 {
			return Pagination{}, services.ItemListRequest{}, invalidQuery(name)
		}
		*target = value
	}
	if request.CollectionID > 0 {
		request.WorkspaceID = 0
	}
	if request.QL == "" && request.CollectionID == 0 {
		if err := parseItemFilters(r, &request.Filters); err != nil {
			return Pagination{}, services.ItemListRequest{}, err
		}
	}
	request.Filters.TextQuery = strings.TrimSpace(query.Get("search"))
	if request.Filters.TextQuery == "" {
		request.Filters.TextQuery = ""
	}
	request.Filters.StatusIDs, err = itemIDList(query.Get("status_id"))
	if err != nil {
		return Pagination{}, services.ItemListRequest{}, invalidQuery("status_id")
	}
	request.Filters.StatusIDsNot, err = itemIDList(query.Get("status_id_not"))
	if err != nil {
		return Pagination{}, services.ItemListRequest{}, invalidQuery("status_id_not")
	}
	request.Filters.CompletedSince = stringPointer(query.Get("completed_since"))

	sort := query.Get("sort")
	if strings.HasPrefix(sort, "-") {
		request.SortBy, request.SortAsc = strings.TrimPrefix(sort, "-"), false
	} else if sort != "" {
		request.SortBy, request.SortAsc = sort, true
	}
	if request.SortBy != "" && !validItemSort(request.SortBy) {
		return Pagination{}, services.ItemListRequest{}, invalidQuery("sort")
	}
	return page, request, nil
}

func parseItemFilters(r *http.Request, filters *services.ItemFilters) error {
	query := r.URL.Query()
	for name, target := range map[string]**int{
		"status_id": &filters.StatusID, "priority_id": &filters.PriorityID,
		"assignee_id": &filters.AssigneeID, "item_type_id": &filters.ItemTypeID,
		"iteration_id": &filters.IterationID, "milestone_id": &filters.MilestoneID,
		"id": &filters.ItemID, "level": &filters.Level, "max_level": &filters.MaxLevel,
	} {
		if query.Get(name) == "" || strings.Contains(query.Get(name), ",") {
			continue
		}
		value, err := strconv.Atoi(query.Get(name))
		if err != nil || value < 0 {
			return invalidQuery(name)
		}
		*target = &value
	}
	if raw := query.Get("parent_id"); raw != "" {
		value := 0
		if raw != "null" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 {
				return invalidQuery("parent_id")
			}
			value = parsed
		}
		filters.ParentID, filters.ParentIDIsSet = &value, true
	}
	filters.CreatedSince = stringPointer(query.Get("created_since"))
	return nil
}

func itemPatchFields(input itemPatchRequest) map[string]json.RawMessage {
	fields := make(map[string]json.RawMessage)
	putOptional(fields, "title", input.Title)
	putOptional(fields, "description", input.Description)
	putOptional(fields, "priority_id", input.PriorityID)
	putOptional(fields, "assignee_id", input.AssigneeID)
	putOptional(fields, "parent_id", input.ParentID)
	putOptional(fields, "iteration_id", input.IterationID)
	putOptional(fields, "project_id", input.ProjectID)
	putOptional(fields, "milestone_ids", input.MilestoneIDs)
	putOptional(fields, "due_date", input.DueDate)
	putOptional(fields, "start_date", input.StartDate)
	putOptional(fields, "end_date", input.EndDate)
	putOptional(fields, "is_task", input.IsTask)
	putOptional(fields, "custom_fields", input.CustomFieldValues)
	return fields
}

func putOptional[T any](target map[string]json.RawMessage, name string, value Optional[T]) {
	if !value.Set {
		return
	}
	if value.Null {
		target[name] = json.RawMessage("null")
		return
	}
	encoded, _ := json.Marshal(value.Value)
	target[name] = encoded
}

func itemIDList(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{}, nil
	}
	seen := make(map[int]struct{})
	ids := make([]int, 0, strings.Count(raw, ",")+1)
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id < 1 {
			return nil, invalidQuery("ids")
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func normalizeBatchIDs(input []int) ([]int, error) {
	if len(input) > maxItemBatch {
		err := newError(http.StatusBadRequest, "invalid_request", "ids supports at most 500 values")
		err.Details = map[string]string{"field": "ids"}
		return nil, err
	}
	seen := make(map[int]struct{}, len(input))
	ids := make([]int, 0, len(input))
	for _, id := range input {
		if id < 1 {
			err := newError(http.StatusBadRequest, "invalid_request", "ids is invalid")
			err.Details = map[string]string{"field": "ids"}
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func validItemSort(value string) bool {
	if id, err := strconv.Atoi(value); err == nil && id > 0 {
		return true
	}
	for _, field := range repository.SystemSortableFieldKeys() {
		if value == field {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func invalidQuery(field string) error {
	err := newError(http.StatusBadRequest, "invalid_request", field+" is invalid")
	err.Details = map[string]string{"field": field}
	return err
}

func optionalItemQueryID(r *http.Request, field string) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(field))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, invalidQuery(field)
	}
	return value, nil
}

func itemError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrItemForbidden) || errors.Is(err, services.ErrItemDeletionForbidden) {
		return newError(http.StatusNotFound, "not_found", "Item not found")
	}
	if errors.Is(err, services.ErrQLQuery) || errors.Is(err, services.ErrCollectionNotFound) || errors.Is(err, repository.ErrInvalidItemListCursor) {
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, services.ErrItemConflict) {
		return newError(http.StatusConflict, "conflict", "Item order changed; refresh and try again")
	}
	if errors.Is(err, services.ErrItemTypeMigrationRequired) {
		return newError(http.StatusConflict, "migration_required", err.Error())
	}
	if errors.Is(err, services.ErrItemHasProtectedIntegrationLinks) {
		return newError(http.StatusConflict, "conflict", "Remove all protected integration links from the affected items before deleting them.")
	}
	if errors.Is(err, services.ErrBulkItemNotFound) {
		return newError(http.StatusNotFound, "not_found", "Item not found")
	}
	if errors.Is(err, services.ErrBulkItemForbidden) {
		return newError(http.StatusForbidden, "forbidden", "Item update is not permitted")
	}
	var validationError *validation.ValidationError
	if errors.As(err, &validationError) {
		apiErr := newError(http.StatusBadRequest, "validation_failed", err.Error())
		apiErr.Details = map[string]string{"field": validationError.Field}
		return apiErr
	}
	if errors.Is(err, services.ErrBulkPatchLimit) || errors.Is(err, services.ErrBulkItemLimit) ||
		errors.Is(err, services.ErrBulkFieldsRequired) || errors.Is(err, services.ErrBulkDuplicateItem) ||
		services.IsBulkItemFieldError(err) || services.IsBulkItemValidationError(err) || errors.Is(err, repository.ErrRoadmapHierarchyRootLimit) {
		return newError(http.StatusBadRequest, "validation_failed", err.Error())
	}
	if errors.Is(err, services.ErrItemWorkspaceMoveSameWorkspace) || errors.Is(err, services.ErrItemWorkspaceMoveInvalidType) ||
		errors.Is(err, services.ErrItemWorkspaceMoveInvalidStatus) || errors.Is(err, services.ErrItemWorkspaceMoveInvalidPriority) {
		return newError(http.StatusBadRequest, "validation_failed", err.Error())
	}
	var creation *services.ItemCreationValidationError
	var transition *services.TransitionRejection
	if errors.As(err, &creation) || errors.As(err, &transition) ||
		errors.Is(err, services.ErrMissingItemType) || errors.Is(err, services.ErrInvalidItemType) || errors.Is(err, services.ErrProjectNotFound) {
		return newError(http.StatusBadRequest, "validation_failed", err.Error())
	}
	return internalError(err)
}
