package aitools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func (e *Env) timePermission() *services.TimePermissionService {
	if e.TimePermService != nil {
		return e.TimePermService
	}
	return services.NewTimePermissionService(e.DB, e.PermService)
}

type worklogRef struct {
	WorklogID int `json:"worklog_id"`
}

func (a worklogRef) read(env *Env) (*models.Worklog, error) {
	permitted, err := env.timePermission().CanViewWorklog(env.UserID, a.WorklogID)
	if err != nil {
		return nil, err
	}
	if !permitted {
		return nil, errors.New("worklog not found")
	}
	return env.timeWorklogService().Get(a.WorklogID)
}

func worklogView(env *Env, log models.Worklog) models.Worklog {
	result := services.RedactInaccessibleWorklogItems([]models.Worklog{log}, func(id int) (bool, error) { return env.HasWorkspaceAccess(id), nil })[0]
	result.ProjectMaxHours, result.ProjectTotalHours = nil, nil
	return result
}

func worklogURL(id int) string {
	return fmt.Sprintf("/api/v2/time/worklogs/%d", id)
}

type rangeWorkspace struct {
	workspaceArgs
	reportDates
}

type notificationsArgs struct {
	UnreadOnly bool `json:"unread_only,omitempty"`
	toolPage
}

type reviewArgs struct {
	reportDates
	toolPage
}

type myWorkArgs struct {
	WorkspaceID int    `json:"workspace_id,omitempty"`
	Days        int    `json:"days,omitempty" jsonschema:"Upcoming window in days, default 7 and maximum 30"`
	Focus       string `json:"focus,omitempty" jsonschema:"Choose overdue or upcoming to page only those assignments; omit for all open assignments"`
	toolPage
}

type itemTemplateArgs struct {
	workspaceArgs
	ItemTypeID *int `json:"item_type_id,omitempty"`
	toolPage
}

type formFieldArgs struct {
	itemRef
	WorkspaceID int  `json:"workspace_id,omitempty" jsonschema:"Workspace for a new item, when no item reference is supplied"`
	ItemTypeID  *int `json:"item_type_id,omitempty"`
}

func dependencyRows(env *Env, items []models.Item) (map[int]services.EntityLinks, error) {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return env.ReadServices.Links.ListLinksForItemsWithChecks(env.UserID, ids)
}

func itemDependencies(ctx context.Context, env *Env, a itemPageArgs) (any, error) {
	item, err := a.read(ctx, env)
	if err != nil {
		return nil, err
	}
	links, err := dependencyRows(env, []models.Item{*item})
	if err != nil {
		return nil, err
	}
	open, completed, downstream := []itemSummaryDTO{}, []itemSummaryDTO{}, []itemSummaryDTO{}
	for _, entry := range []struct {
		rows     []models.ItemLink
		outgoing bool
	}{{links[item.ID].Outgoing, true}, {links[item.ID].Incoming, false}} {
		for _, link := range entry.rows {
			if !strings.EqualFold(link.LinkTypeName, "Depends On") || link.TargetType != "item" || link.SourceType != "item" {
				continue
			}
			id := link.SourceID
			if entry.outgoing {
				id = link.TargetID
			}
			target, err := (itemRef{ItemID: id}).read(ctx, env)
			if errors.Is(err, services.ErrItemForbidden) || errors.Is(err, repository.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			row := itemToSummary(target)
			if !entry.outgoing {
				downstream = append(downstream, row)
			} else {
				facts, err := repository.NewItemRepository(env.DB).CompletionFacts(ctx, []models.Item{*target})
				if err != nil {
					return nil, err
				}
				if facts[target.ID].Completed {
					completed = append(completed, row)
				} else {
					open = append(open, row)
				}
			}
		}
	}
	return map[string]any{"url": itemURL(item), "open_blockers": slicePage(open, a.toolPage), "completed_prerequisites": slicePage(completed, a.toolPage), "downstream": slicePage(downstream, a.toolPage)}, nil
}

func reviewRange(env *Env, dates reportDates) (repository.WorklogDetailFilter, error) {
	_, loc := services.ResolveTimezoneOrUTC(env.Timezone)
	var filter repository.WorklogDetailFilter
	err := dates.filter(&filter, loc, true)
	return filter, err
}

func init() {
	items := []string{auth.ScopeItemsRead}
	timeRead := []string{auth.ScopeTimeRead}
	registerRead("get_worklog", "Get one visible saved worklog before explaining or correcting it.", CapabilityTime, timeRead,
		func(_ context.Context, env *Env, a worklogRef) (any, error) {
			log, err := a.read(env)
			if err != nil {
				return nil, err
			}
			return map[string]any{"worklog": worklogView(env, *log), "url": worklogURL(log.ID)}, nil
		})
	registerRead("get_active_timer", "Read the authenticated user's active timer and linked task.", CapabilityTime, timeRead,
		func(_ context.Context, env *Env, _ struct{}) (any, error) {
			if env.TimerService == nil {
				return nil, errors.New("timer service is unavailable")
			}
			timer, err := env.TimerService.GetActiveForUser(env.UserID)
			return map[string]any{"active": timer != nil, "timer": timer}, err
		})
	registerRead("list_time_drafts", "List the caller's unbooked time drafts and the metadata still needed to book them.", CapabilityTime, timeRead,
		func(_ context.Context, env *Env, a toolPage) (any, error) {
			if env.TimerService == nil {
				return nil, errors.New("timer service is unavailable")
			}
			drafts, err := env.TimerService.ListDrafts(env.UserID)
			if err != nil {
				return nil, err
			}
			type draftStatus struct {
				models.TimeWorklogDraft
				MissingFields []string `json:"missing_fields"`
			}
			rows := make([]draftStatus, 0, len(drafts))
			for _, draft := range drafts {
				missing := []string{}
				if draft.ProjectID == nil {
					missing = append(missing, "project_id")
				}
				if draft.Date == nil {
					missing = append(missing, "date")
				}
				if draft.DurationMins == nil || *draft.DurationMins <= 0 {
					missing = append(missing, "duration_minutes")
				}
				rows = append(rows, draftStatus{TimeWorklogDraft: draft, MissingFields: missing})
			}
			return slicePage(rows, a), nil
		})
	registerApplicationRead("get_time_project_budget", "Read booked and remaining project hours when the caller can manage the time project.", CapabilityTime, timeRead,
		func(_ context.Context, env *Env, a struct {
			ProjectID int `json:"project_id"`
		}) (any, error) {
			manager, err := env.timePermission().IsTimeProjectManager(env.UserID, a.ProjectID)
			if err != nil {
				return nil, err
			}
			if !manager {
				return nil, errors.New("project budget is unavailable")
			}
			project, err := env.ReadServices.TimeProjects.Get(env.UserID, a.ProjectID)
			if err != nil {
				return nil, err
			}
			result := map[string]any{"project_id": project.ID, "name": project.Name, "booked_hours": project.TotalHours}
			if limit, ok := project.Settings["max_hours"].(float64); ok && limit > 0 {
				result["budget_hours"] = limit
				if project.TotalHours != nil {
					result["remaining_hours"] = limit - *project.TotalHours
				}
			}
			return result, nil
		})
	registerApplicationRead("get_item_dependencies", "Find open prerequisites, completed prerequisites, and downstream work blocked by this item.", CapabilityPlanningActivity, items, itemDependencies)
	registerRead("get_item_hierarchy", "Read the visible ancestor path and the item's direct subtasks.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a itemPageArgs) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			children, err := env.itemApplication().Children(ctx, env.UserID, item.ID)
			if err != nil {
				return nil, err
			}
			ancestors := []itemSummaryDTO{}
			seen := map[int]bool{item.ID: true}
			parent := item.ParentID
			for parent != nil && !seen[*parent] && len(ancestors) < 20 {
				ancestor, err := (itemRef{ItemID: *parent}).read(ctx, env)
				if errors.Is(err, repository.ErrNotFound) {
					break
				}
				if err != nil {
					return nil, err
				}
				seen[ancestor.ID] = true
				ancestors = append(ancestors, itemToSummary(ancestor))
				parent = ancestor.ParentID
			}
			return map[string]any{"url": itemURL(item), "ancestors": ancestors, "children": slicePage(summarizeItems(children), a.toolPage)}, nil
		})
	registerRead("get_delivery_flow_metrics", "Read the workspace analytics for delivery time, throughput, and aging work over a date range.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a rangeWorkspace) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			start, err := time.Parse(time.DateOnly, a.From)
			if err != nil {
				return nil, errors.New("from must be a date in YYYY-MM-DD format")
			}
			end, err := time.Parse(time.DateOnly, a.To)
			if err != nil {
				return nil, errors.New("to must be a date in YYYY-MM-DD format")
			}
			return services.NewAnalyticsService(env.DB).GetAnalyticsContext(ctx, services.ResolveDatasetParams{
				UserID: env.UserID, WorkspaceID: a.WorkspaceID, StartDate: start, EndDate: end})
		})
	registerApplicationRead("get_my_work_overview", "Find overdue, upcoming and blocked work assigned to the caller.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a myWorkArgs) (any, error) {
			if len(env.AccessibleWorkspaceIDs) == 0 || (a.WorkspaceID > 0 && !env.HasWorkspaceAccess(a.WorkspaceID)) {
				return nil, errors.New("workspace not found")
			}
			if a.Days == 0 {
				a.Days = 7
			}
			if a.Days < 1 || a.Days > 30 {
				return nil, errors.New("days must be between 1 and 30")
			}
			_, loc := services.ResolveTimezoneOrUTC(env.Timezone)
			now := time.Now().In(loc)
			today := now.Format(time.DateOnly)
			until := now.AddDate(0, 0, a.Days).Format(time.DateOnly)
			ql := "(status_completed = false OR status_id IS EMPTY)"
			sortBy := ""
			switch a.Focus {
			case "", "all":
			case "overdue":
				ql += fmt.Sprintf(" AND dueDate < '%s'", today)
				sortBy = "due_date"
			case "upcoming":
				ql += fmt.Sprintf(" AND dueDate >= '%s' AND dueDate <= '%s'", today, until)
				sortBy = "due_date"
			default:
				return nil, errors.New("focus must be all, overdue, or upcoming")
			}
			limit, offset := a.bounds()
			page, err := env.itemApplication().List(ctx, services.ItemListRequest{UserID: env.UserID, WorkspaceIDs: env.AccessibleWorkspaceIDs, WorkspaceID: a.WorkspaceID,
				Filters: services.ItemFilters{AssigneeID: &env.UserID}, QL: ql, OmitDescriptions: true, SortBy: sortBy, SortAsc: sortBy != "",
				Pagination: services.PaginationParams{Limit: limit, Offset: offset}})
			if err != nil {
				return nil, err
			}
			allLinks, err := dependencyRows(env, page.Items)
			if err != nil {
				return nil, err
			}
			overdue, upcoming, blocked := []itemSummaryDTO{}, []itemSummaryDTO{}, []itemSummaryDTO{}
			for _, item := range page.Items {
				dto := itemToSummary(&item)
				if dto.DueDate != "" {
					if dto.DueDate < today {
						overdue = append(overdue, dto)
					} else if dto.DueDate <= until {
						upcoming = append(upcoming, dto)
					}
				}
				for _, link := range allLinks[item.ID].Outgoing {
					if !strings.EqualFold(link.LinkTypeName, "Depends On") || link.TargetType != "item" {
						continue
					}
					prerequisite, err := (itemRef{ItemID: link.TargetID}).read(ctx, env)
					if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrItemForbidden) {
						continue
					}
					if err != nil {
						return nil, err
					}
					facts, err := repository.NewItemRepository(env.DB).CompletionFacts(ctx, []models.Item{*prerequisite})
					if err != nil {
						return nil, err
					}
					if !facts[prerequisite.ID].Completed {
						blocked = append(blocked, dto)
						break
					}
				}
			}
			out := pageResult(summarizeItems(page.Items), page.Total, offset)
			out["overdue_on_page"], out["upcoming_on_page"], out["blocked_on_page"] = overdue, upcoming, blocked
			out["as_of"], out["upcoming_through"] = today, until
			return out, nil
		})
	registerRead("get_assignment_load", "Summarize open and overdue assignments per person and for unassigned work.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a workspaceArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			page, err := env.itemApplication().List(ctx, services.ItemListRequest{UserID: env.UserID, WorkspaceIDs: []int{a.WorkspaceID}, WorkspaceID: a.WorkspaceID,
				QL: "(status_completed = false OR status_id IS EMPTY)", OmitDescriptions: true, Pagination: services.PaginationParams{Limit: 1000}})
			if err != nil {
				return nil, err
			}
			if page.Total > len(page.Items) {
				return nil, errors.New("workspace exceeds the assignment report limit")
			}
			type load struct {
				Name        string  `json:"name"`
				Open        int     `json:"open"`
				Overdue     int     `json:"overdue"`
				StoryPoints float64 `json:"story_points"`
			}
			groups := map[int]load{}
			_, loc := services.ResolveTimezoneOrUTC(env.Timezone)
			today := time.Now().In(loc).Format(time.DateOnly)
			for _, item := range page.Items {
				id := 0
				if item.AssigneeID != nil {
					id = *item.AssigneeID
				}
				row := groups[id]
				row.Name = item.AssigneeName
				row.Open++
				if item.DueDate != nil && item.DueDate.Format(time.DateOnly) < today {
					row.Overdue++
				}
				if item.StoryPoints != nil {
					row.StoryPoints += *item.StoryPoints
				}
				groups[id] = row
			}
			return map[string]any{"by_assignee_id": groups, "unassigned_key": 0, "total": page.Total}, nil
		})
	registerApplicationRead("list_my_notifications", "Read the caller's inbox. Set unread_only to page through unread notices.", CapabilityUsersApprovals, []string{auth.ScopeNotificationsRead},
		func(ctx context.Context, env *Env, a notificationsArgs) (any, error) {
			limit, offset := a.bounds()
			rows, err := env.ReadServices.Notifications(ctx, env.UserID, a.UnreadOnly, limit+1, offset)
			if err != nil {
				return nil, err
			}
			out := []map[string]any{}
			for _, row := range rows[:min(len(rows), limit)] {
				out = append(out, map[string]any{"id": row.ID, "title": row.Title, "message": row.Message, "type": row.Type, "read": row.Read, "timestamp": row.Timestamp, "url": row.ActionURL})
			}
			result := map[string]any{"items": out}
			if len(rows) > limit {
				result["next_offset"] = offset + limit
			}
			return result, nil
		})
	registerRead("get_item_watchers", "Show who follows a task and whether you are watching it.", CapabilityUsersApprovals, items,
		func(ctx context.Context, env *Env, a itemPageArgs) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			status, err := env.itemApplication().WatchStatus(ctx, env.UserID, item.ID)
			if err != nil {
				return nil, err
			}
			rows, err := env.itemApplication().Watchers(ctx, env.UserID, item.ID)
			if err != nil {
				return nil, err
			}
			result := slicePage(rows, a.toolPage)
			result["caller_status"] = status
			return result, nil
		})
	registerApplicationRead("list_item_request_participants", "Show the external participants already involved in a visible support request.", CapabilityUsersApprovals, items,
		func(ctx context.Context, env *Env, a itemPageArgs) (any, error) {
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			rows, err := env.ReadServices.Participants.List(item.ID)
			if err != nil {
				return nil, err
			}
			return slicePage(rows, a.toolPage), nil
		})
	registerApplicationRead("get_item_form_fields", "Read effective fields and options for editing an existing item, or creating a new item in a workspace.", CapabilityIssueManagement, []string{auth.ScopeItemsRead, auth.ScopeScreensRead},
		func(ctx context.Context, env *Env, a formFieldArgs) (any, error) {
			if a.ItemID == 0 && a.ItemKey == "" {
				if !env.HasWorkspaceAccess(a.WorkspaceID) {
					return nil, errors.New("workspace not found")
				}
				allowed, err := env.PermService.HasWorkspacePermission(env.UserID, a.WorkspaceID, models.PermissionItemCreate)
				if err != nil {
					return nil, err
				}
				if !allowed {
					return nil, errors.New("the caller cannot create items in this workspace")
				}
				config, err := env.ReadServices.Catalog.GetEffectiveConfig(env.UserID, a.WorkspaceID, a.ItemTypeID)
				if err != nil {
					return nil, err
				}
				screen, err := env.ReadServices.LoadScreen(config.CreateScreenID)
				if err != nil {
					return nil, err
				}
				template, err := env.ReadServices.Catalog.ListItemTemplates(env.UserID, a.WorkspaceID, a.ItemTypeID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"create_screen": screen, "valid_item_type_ids": config.ItemTypeIDs, "default_item_type_id": config.DefaultItemTypeID,
					"priorities": config.Priorities, "mandatory_template_id": template.MandatoryTemplateID}, nil
			}
			item, err := a.read(ctx, env)
			if err != nil {
				return nil, err
			}
			detail, err := env.ReadServices.ItemDetail.Get(ctx, env.UserID, item.ID, "", false)
			if err != nil {
				return nil, err
			}
			return map[string]any{"screens": detail.ScreenContext, "request_type_fields": detail.RequestTypeFields, "section_errors": detail.SectionErrors}, nil
		})
	registerApplicationRead("list_item_templates", "Discover workspace work-item templates for the requested item type.", CapabilityIssueManagement, []string{auth.ScopeItemTemplatesRead},
		func(_ context.Context, env *Env, a itemTemplateArgs) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			result, err := env.ReadServices.Catalog.ListItemTemplates(env.UserID, a.WorkspaceID, a.ItemTypeID)
			if err != nil {
				return nil, err
			}
			out := slicePage(result.Items, a.toolPage)
			out["mandatory_template_id"] = result.MandatoryTemplateID
			return out, nil
		})
	registerRead("get_support_metrics", "Read the workspace support intake, backlog, resolution, and SLA report for a date range.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a rangeWorkspace) (any, error) {
			if err := a.check(env); err != nil {
				return nil, err
			}
			filter, err := reviewRange(env, a.reportDates)
			if err != nil {
				return nil, err
			}
			_, loc := services.ResolveTimezoneOrUTC(env.Timezone)
			return env.itemApplication().AggregateSupportMetrics(ctx, services.SupportMetricsQuery{UserID: env.UserID, WorkspaceIDs: []int{a.WorkspaceID},
				From: time.Unix(*filter.DateFromUnix, 0), To: time.Unix(*filter.DateToExclusiveUnix, 0), Location: loc, Bucket: services.SupportBucketWeek, Segment: services.SupportSegmentNone})
		})
	registerRead("list_my_reviews", "Read your own written daily and weekly review notes for a date range.", CapabilityPlanningActivity, []string{auth.ScopeReviewsRead},
		func(ctx context.Context, env *Env, a reviewArgs) (any, error) {
			if _, err := reviewRange(env, a.reportDates); err != nil {
				return nil, err
			}
			limit, offset := a.bounds()
			limit = min(limit, 25)
			out, err := repository.NewReviewRepository(env.DB).ListPreviews(ctx, env.UserID, a.From, a.To, limit+1, offset)
			if err != nil {
				return nil, err
			}
			result := map[string]any{"items": out[:min(limit, len(out))]}
			if len(out) > limit {
				result["next_offset"] = offset + limit
			}
			return result, nil
		})
	registerRead("list_my_completed_items", "List assigned items with a recorded completion date in the supplied period.", CapabilityPlanningActivity, items,
		func(ctx context.Context, env *Env, a reviewArgs) (any, error) {
			filter, err := reviewRange(env, a.reportDates)
			if err != nil {
				return nil, err
			}
			if len(env.AccessibleWorkspaceIDs) == 0 {
				return pageResult([]itemSummaryDTO{}, 0, 0), nil
			}
			limit, offset := a.bounds()
			items, err := repository.NewItemRepository(env.DB).ListRecentlyCompletedForUser(ctx, env.UserID, env.AccessibleWorkspaceIDs,
				time.Unix(*filter.DateFromUnix, 0), time.Unix(*filter.DateToExclusiveUnix, 0), limit+1, offset)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(items))
			for i := range items {
				item := &items[i]
				out = append(out, map[string]any{"item": itemToSummary(item), "url": itemURL(item), "completed_at": item.CompletedAt})
			}
			result := map[string]any{"items": out[:min(limit, len(out))]}
			if len(out) > limit {
				result["next_offset"] = offset + limit
			}
			return result, nil
		})
}
