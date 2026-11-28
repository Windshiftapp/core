package aitools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/redact"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type ReadServices struct {
	Planning          *services.PlanningApplicationService
	Actions           *services.ActionApplicationService
	Attachments       *services.ItemAttachmentService
	Assets            *services.AssetApplicationService
	Governance        *services.GovernanceApplicationService
	Runs              *services.AgentRunReadService
	MilestoneActivity *services.MilestoneActivityService
	TimeProjects      *services.TimeProjectApplicationService
	Participants      *services.ItemParticipantService
	ItemDetail        *services.ItemDetailApplicationService
	Catalog           *services.CatalogReadService
	Links             *services.ItemLinkService
	LoadScreen        func(int) (*models.Screen, error)
	ItemSLA           func(context.Context, int, int) ([]models.ItemSLA, error)
	Notifications     func(context.Context, int, bool, int, int) ([]models.Notification, error)
}

func registerApplicationRead[A any](name, description string, group CapabilityGroup, scopes []string, run func(context.Context, *Env, A) (any, error)) {
	registerRead(name, description, group, scopes, func(ctx context.Context, env *Env, args A) (any, error) {
		if env.ReadServices == nil {
			return nil, errors.New("application service is unavailable")
		}
		return run(ctx, env, args)
	})
}

type workspaceArgs struct {
	WorkspaceID int `json:"workspace_id"`
}

func (args workspaceArgs) check(env *Env) error {
	if !env.HasWorkspaceAccess(args.WorkspaceID) {
		return errors.New("workspace not found")
	}
	return nil
}

type collectionArgs struct {
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	Query       string `json:"query,omitempty"`
	toolPage
}

type collectionItemsArgs struct {
	CollectionID int `json:"collection_id"`
	toolPage
}

type iterationArgs struct {
	IterationID int `json:"iteration_id"`
}

type milestoneArgs struct {
	MilestoneID int `json:"milestone_id"`
	toolPage
}

type queueArgs struct {
	WorkspaceID  int  `json:"workspace_id,omitempty"`
	CollectionID *int `json:"collection_id,omitempty"`
}

func (args queueArgs) read(ctx context.Context, env *Env) ([]services.QueueView, error) {
	if args.CollectionID != nil && (*args.CollectionID <= 0 || !env.allowScope(auth.ScopeCollectionsRead)) {
		return nil, errors.New("collection scope is unavailable")
	}
	if args.CollectionID == nil && !env.HasWorkspaceAccess(args.WorkspaceID) || args.WorkspaceID > 0 && !env.HasWorkspaceAccess(args.WorkspaceID) {
		return nil, errors.New("queue scope not found")
	}
	return env.itemApplication().ListQueues(ctx, env.UserID, args.WorkspaceID, args.CollectionID)
}

type queueItemsArgs struct {
	queueArgs
	QueueID  int    `json:"queue_id,omitempty"`
	QueueKey string `json:"queue_key,omitempty" jsonschema:"Built-in key from list_queues"`
	toolPage
}

type attachmentArgs struct {
	itemRef
	AttachmentID int `json:"attachment_id"`
}

type approvalsArgs struct {
	Status string `json:"status,omitempty" jsonschema:"Approval status; defaults to pending"`
	toolPage
}

type actionLogsArgs struct {
	workspaceArgs
	ActionID int `json:"action_id"`
	toolPage
}

type assetListArgs struct {
	AssetSetID  int    `json:"asset_set_id" jsonschema:"Asset set ID; use 0 to discover accessible sets"`
	AssetTypeID int    `json:"asset_type_id,omitempty"`
	Query       string `json:"query,omitempty"`
	Filter      string `json:"filter,omitempty" jsonschema:"Optional asset query-language filter for configured fields"`
	toolPage
}

type assetArgs struct {
	AssetID int `json:"asset_id"`
}

type pageHistoryArgs struct {
	workspaceArgs
	PageID int `json:"page_id"`
	toolPage
}

type worklogTotalsArgs struct {
	reportDates
	ProjectID  *int `json:"project_id,omitempty"`
	CustomerID *int `json:"customer_id,omitempty"`
	MineOnly   bool `json:"mine_only,omitempty" jsonschema:"Set true when the user asks for their own time"`
	toolPage
}

type testReportsArgs struct {
	workspaceArgs
	MilestoneID *int `json:"milestone_id,omitempty"`
	Days        int  `json:"days,omitempty"`
}

func summarizeItems(items []models.Item) []itemSummaryDTO {
	rows := make([]itemSummaryDTO, 0, len(items))
	for i := range items {
		rows = append(rows, itemToSummary(&items[i]))
	}
	return rows
}

func (e *Env) listScopeItems(ctx context.Context, workspaceID, collectionID int, ql string, p toolPage) (any, error) {
	if len(e.AccessibleWorkspaceIDs) == 0 || (workspaceID > 0 && !e.HasWorkspaceAccess(workspaceID)) {
		return nil, errors.New("scope not found")
	}
	limit, offset := p.bounds()
	result, err := e.itemApplication().List(ctx, services.ItemListRequest{UserID: e.UserID, WorkspaceIDs: e.AccessibleWorkspaceIDs,
		WorkspaceID: workspaceID, CollectionID: collectionID, SubQL: ql, OmitDescriptions: true,
		Pagination: services.PaginationParams{Limit: limit, Offset: offset}})
	if err != nil {
		return nil, err
	}
	return pageResult(summarizeItems(result.Items), result.Total, offset), nil
}

func init() {
	items := []string{auth.ScopeItemsRead}
	collections := []string{auth.ScopeCollectionsRead}
	timeRead := []string{auth.ScopeTimeRead}
	milestones := []string{auth.ScopeMilestonesRead}
	iterations := []string{auth.ScopeIterationsRead}
	actions := []string{auth.ScopeActionsRead}
	assets := []string{auth.ScopeAssetsRead}
	registerRead("list_collections", "Find readable saved boards and collections by name or workspace.", CapabilityPlanningActivity, collections,
		func(_ context.Context, env *Env, a collectionArgs) (any, error) {
			if a.WorkspaceID != nil && !env.HasWorkspaceAccess(*a.WorkspaceID) {
				return nil, errors.New("workspace not found")
			}
			limit, offset := a.bounds()
			rows, total, err := services.NewCollectionApplicationService(env.DB, env.PermService).List(services.CollectionListParams{
				UserID: env.UserID, Query: a.Query, WorkspaceID: a.WorkspaceID, Limit: limit, Offset: offset})
			if err != nil {
				return nil, err
			}
			return pageResult(rows, total, offset), nil
		})
	registerRead("list_collection_items", "List items in a saved collection using its saved query and the actor's workspace visibility.", CapabilityPlanningActivity, []string{auth.ScopeItemsRead, auth.ScopeCollectionsRead},
		func(ctx context.Context, env *Env, a collectionItemsArgs) (any, error) {
			if a.CollectionID <= 0 {
				return nil, errors.New("collection_id must be positive")
			}
			return env.listScopeItems(ctx, 0, a.CollectionID, "", a.toolPage)
		})
	registerApplicationRead("get_iteration_progress", "Read the app's sprint/iteration completion counts, dates, and status breakdown.", CapabilityPlanningActivity, iterations,
		func(_ context.Context, env *Env, a iterationArgs) (any, error) {
			value, err := env.ReadServices.Planning.GetIterationProgress(env.UserID, a.IterationID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"id": a.IterationID, "name": value.IterationName, "start_date": value.StartDate, "end_date": value.EndDate,
				"status": value.Status, "total_items": value.TotalItems, "completed_items": value.CompletedItems, "percent_complete": value.PercentComplete,
				"status_breakdown": value.StatusBreakdown, "url": fmt.Sprintf("/iterations/%d", a.IterationID)}, nil
		})
	registerApplicationRead("get_milestone_progress", "Read the app's milestone due date, completion counts and a bounded list of remaining items.", CapabilityPlanningActivity, milestones,
		func(_ context.Context, env *Env, a milestoneArgs) (any, error) {
			value, err := env.ReadServices.Planning.GetMilestoneProgress(env.UserID, a.MilestoneID)
			if err != nil {
				return nil, err
			}
			groups := map[string]any{}
			for key, rows := range value.ItemsByCategory {
				groups[key] = slicePage(rows, a.toolPage)
			}
			return map[string]any{"id": a.MilestoneID, "name": value.MilestoneName, "target_date": value.TargetDate, "total_items": value.TotalItems,
				"completed_items": value.CompletedItems, "status_breakdown": value.StatusBreakdown, "items_by_category": groups, "url": fmt.Sprintf("/milestones/%d", a.MilestoneID)}, nil
		})
	registerApplicationRead("list_item_attachments", "List visible attachment metadata for a work item.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a itemPageArgs) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			limit, offset := a.bounds()
			rows, total, err := env.ReadServices.Attachments.ListItemAttachments(env.UserID, item.ID, limit, offset)
			if err != nil {
				return nil, err
			}
			data := make([]map[string]any, 0, len(rows))
			for _, attachment := range rows {
				data = append(data, map[string]any{"id": attachment.ID, "name": attachment.OriginalFilename,
					"mime_type": attachment.MimeType, "size": attachment.FileSize, "created_at": attachment.CreatedAt,
					"url": fmt.Sprintf("/api/v2/attachments/%d/download", attachment.ID)})
			}
			return pageResult(data, total, offset), nil
		})
	registerApplicationRead("read_item_attachment_text", "Read up to 16 KiB of a visible plain-text attachment belonging to the supplied item. Shows whether the preview was truncated.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a attachmentArgs) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			meta, err := env.ReadServices.Attachments.GetItemAttachment(env.UserID, a.AttachmentID)
			if err != nil {
				return nil, err
			}
			if meta.ItemID == nil || *meta.ItemID != item.ID {
				return nil, errors.New("attachment not found on this item")
			}
			attachment, err := env.ReadServices.Attachments.OpenItemAttachment(ctx, env.UserID, a.AttachmentID, false)
			if err != nil {
				return nil, err
			}
			defer attachment.File.Close()
			media, _, err := mime.ParseMediaType(attachment.MimeType)
			if err != nil || (media != "text/plain" && media != "text/markdown" && media != "text/csv" && media != "application/json") {
				return nil, errors.New("attachment does not have a supported text type")
			}
			const limit = 16 * 1024
			body, err := io.ReadAll(io.LimitReader(attachment.File, limit+1))
			if err != nil {
				return nil, err
			}
			truncated := len(body) > limit
			body = body[:min(len(body), limit)]
			if truncated {
				for len(body) > 0 && !utf8.Valid(body) && len(body) > limit-utf8.UTFMax {
					body = body[:len(body)-1]
				}
			}
			if !utf8.Valid(body) || strings.ContainsRune(string(body), 0) {
				return nil, errors.New("attachment is not UTF-8 text")
			}
			return map[string]any{"attachment_id": a.AttachmentID, "name": attachment.OriginalFilename, "text": string(body), "truncated": truncated, "item_url": itemURL(item)}, nil
		})
	registerRead("list_queues", "List the visible support queue names and counts for a workspace or saved collection.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a queueArgs) (any, error) {
			return a.read(ctx, env)
		})
	registerRead("list_queue_items", "List items matching the selected saved support queue.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a queueItemsArgs) (any, error) {
			if (a.QueueID > 0) == (a.QueueKey != "") {
				return nil, errors.New("provide one queue_id or queue_key")
			}
			queues, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			for _, q := range queues {
				if (a.QueueID > 0 && a.QueueID == q.ID) || (a.QueueKey != "" && a.QueueKey == q.Key) {
					id := 0
					if a.CollectionID != nil {
						id = *a.CollectionID
					}
					return env.listScopeItems(ctx, a.WorkspaceID, id, q.QL, a.toolPage)
				}
			}
			return nil, errors.New("queue not found")
		})
	registerApplicationRead("get_item_sla", "Read the deadlines and current SLA-cycle states for a ticket.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a itemRef) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			if env.ReadServices.ItemSLA == nil {
				return nil, errors.New("SLA information is unavailable")
			}
			return env.ReadServices.ItemSLA(ctx, item.ID, item.WorkspaceID)
		})
	registerApplicationRead("list_my_approvals", "List the authenticated user's approval inbox across items.", CapabilityUsersApprovals, []string{auth.ScopeApprovalsRead},
		func(ctx context.Context, env *Env, a approvalsArgs) (any, error) {
			if a.Status == "" {
				a.Status = "pending"
			}
			limit, offset := a.bounds()
			rows, total, err := env.ReadServices.Governance.ListMyApprovals(ctx, env.UserID, a.Status, limit, offset)
			if err != nil {
				return nil, err
			}
			return pageResult(rows, total, offset), nil
		})
	registerApplicationRead("list_actions", "List workspace automation names, trigger types and enabled state.", CapabilityActions, actions,
		func(_ context.Context, env *Env, a workspaceArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			rows, err := env.ReadServices.Actions.List(env.UserID, a.WorkspaceID)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, min(len(rows), 100))
			for _, action := range rows[:min(len(rows), 100)] {
				out = append(out, map[string]any{"id": action.ID, "name": action.Name, "trigger_type": action.TriggerType, "is_enabled": action.IsEnabled})
			}
			return map[string]any{"actions": out, "total": len(rows), "truncated": len(rows) > 100}, nil
		})
	registerApplicationRead("list_action_logs", "List recorded workspace automation runs and their statuses. Detailed execution payloads are omitted.", CapabilityActions, actions,
		func(_ context.Context, env *Env, a actionLogsArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			limit, offset := a.bounds()
			rows, total, err := env.ReadServices.Actions.Logs(env.UserID, a.WorkspaceID, a.ActionID, limit, offset)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(rows))
			for _, log := range rows {
				summary := []rune(redact.String(log.ErrorMessage))
				out = append(out, map[string]any{"id": log.ID, "status": log.Status, "started_at": log.StartedAt, "completed_at": log.CompletedAt,
					"error_summary": string(summary[:min(len(summary), 300)])})
			}
			return pageResult(out, total, offset), nil
		})
	registerApplicationRead("list_item_agent_runs", "Read a ticket's recent agent run statuses and summaries. Page back using the last run ID.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a struct {
			itemRef
			BeforeID int `json:"before_id,omitempty"`
			Limit    int `json:"limit,omitempty"`
		}) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			limit, _ := (toolPage{Limit: a.Limit}).bounds()
			rows, err := env.ReadServices.Runs.ListForItem(ctx, env.UserID, item.ID, limit+1, a.BeforeID)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, min(limit, len(rows)))
			for _, run := range rows[:min(limit, len(rows))] {
				row := map[string]any{"id": run.ID, "status": run.Status, "queued_at": run.QueuedAt, "started_at": run.StartedAt, "ended_at": run.EndedAt,
					"url": fmt.Sprintf("/api/v2/agent-runs/%d", run.ID)}
				if run.Error != "" {
					summary := []rune(redact.String(run.Error))
					row["error_summary"] = string(summary[:min(len(summary), 300)])
				}
				out = append(out, row)
			}
			result := map[string]any{"runs": out}
			if len(rows) > limit {
				result["next_before_id"] = rows[limit-1].ID
			}
			return result, nil
		})
	registerApplicationRead("search_assets", "Search a permitted asset set by text or by a configured asset filter. Set asset_set_id to 0 to discover accessible sets.", CapabilityPlanningActivity, assets,
		func(_ context.Context, env *Env, a assetListArgs) (any, error) {
			if a.AssetSetID == 0 {
				sets, err := env.ReadServices.Assets.ListSets(env.UserID)
				if err != nil {
					return nil, err
				}
				return slicePage(sets, a.toolPage), nil
			}
			limit, offset := a.bounds()
			filter := repository.AssetListFilter{Search: a.Query, Limit: limit, Offset: offset}
			if a.AssetTypeID > 0 {
				filter.AssetTypeID = fmt.Sprint(a.AssetTypeID)
			}
			rows, total, err := env.ReadServices.Assets.ListAssets(env.UserID, a.AssetSetID, filter, a.Filter)
			if err != nil {
				return nil, err
			}
			return pageResult(rows, total, offset), nil
		})
	registerApplicationRead("get_asset", "Get an accessible asset's fields and visible relationships.", CapabilityPlanningActivity, assets,
		func(_ context.Context, env *Env, a assetArgs) (any, error) {
			item, err := env.ReadServices.Assets.GetAsset(env.UserID, a.AssetID)
			if err != nil {
				return nil, err
			}
			links, err := env.ReadServices.Assets.AssetLinks(env.UserID, a.AssetID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"asset": item, "links": links, "url": fmt.Sprintf("/assets/%d", a.AssetID)}, nil
		})
	registerRead("get_worklog_totals", "Report visible booked time totals grouped by project, person and civil day over a bounded local date range.", CapabilityTime, timeRead,
		func(ctx context.Context, env *Env, a worklogTotalsArgs) (any, error) {
			filter, err := env.worklogReadFilter()
			if err != nil {
				return nil, err
			}
			zone, loc := services.ResolveTimezoneOrUTC(env.Timezone)
			if err := a.filter(&filter, loc, true); err != nil {
				return nil, err
			}
			filter.ProjectID = a.ProjectID
			filter.CustomerID = a.CustomerID
			filter.ClipToDateRange = true
			if a.MineOnly {
				filter.ViewerID, filter.ManagedProjectIDs = &env.UserID, nil
			}
			result, err := env.timeWorklogService().Aggregate(ctx, filter, zone)
			if err != nil {
				return nil, err
			}
			var minutes, entries int64
			for _, group := range result.Totals {
				minutes += group.DurationMinutes
				entries += group.Entries
			}
			return map[string]any{"from": a.From, "to": a.To, "timezone": zone, "daily": slicePage(result.Daily, a.toolPage),
				"totals": slicePage(result.Totals, a.toolPage), "visible_booked_minutes": minutes, "visible_entries": entries}, nil
		})
	registerRead("list_page_revisions", "List visible page revision authors and timestamps without loading historical bodies.", CapabilityKnowledgeDiagrams, []string{auth.ScopePagesRead},
		func(_ context.Context, env *Env, a pageHistoryArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			limit, offset := a.bounds()
			rows, total, err := pageApplicationService(env).ListHistory(env.UserID, a.WorkspaceID, a.PageID, limit, offset)
			if err != nil {
				return nil, err
			}
			result := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				result = append(result, map[string]any{"id": row.ID, "revision": row.RevisionNumber, "title": row.Title,
					"created_at": row.CreatedAt, "author": row.Author, "change_summary": row.ChangeSummary, "change_type": row.ChangeType})
			}
			return pageResult(result, total, offset), nil
		})
	registerApplicationRead("list_milestone_activity", "Read the milestone activity timeline, including visible member changes.", CapabilityPlanningActivity, milestones,
		func(_ context.Context, env *Env, a milestoneArgs) (any, error) {
			limit, offset := a.bounds()
			rows, more, err := env.ReadServices.MilestoneActivity.List(env.UserID, a.MilestoneID, services.MilestoneActivityListParams{Limit: limit, Offset: offset})
			if err != nil {
				return nil, err
			}
			out := map[string]any{"items": rows, "has_more": more}
			if more {
				out["next_offset"] = offset + len(rows)
			}
			return out, nil
		})
	registerApplicationRead("get_milestone_test_statistics", "Read recorded milestone test statistics.", CapabilityTests, []string{auth.ScopeMilestonesRead, auth.ScopeTestsRead},
		func(_ context.Context, env *Env, a milestoneArgs) (any, error) {
			return env.ReadServices.Planning.GetMilestoneTestStatistics(env.UserID, a.MilestoneID)
		})
	registerApplicationRead("get_iteration_burndown", "Read how remaining work changed over an iteration.", CapabilityPlanningActivity, iterations,
		func(_ context.Context, env *Env, a iterationArgs) (any, error) {
			return env.ReadServices.Planning.GetIterationBurndown(env.UserID, a.IterationID)
		})
	registerRead("get_test_reports_summary", "Read workspace QA results and failure trends over a bounded period.", CapabilityTests, []string{auth.ScopeTestsRead},
		func(_ context.Context, env *Env, a testReportsArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			if a.Days == 0 {
				a.Days = 30
			}
			if a.Days < 1 || a.Days > 365 {
				return nil, errors.New("days must be between 1 and 365")
			}
			return testApplication(env).ReportsSummary(env.UserID, a.WorkspaceID, a.MilestoneID, a.Days)
		})
}
