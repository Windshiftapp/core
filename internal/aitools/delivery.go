package aitools

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerRead[A any](name, description string, group CapabilityGroup, scopes []string, run func(context.Context, *Env, A) (any, error)) {
	Register(Default, Tool[A]{Name: name, Description: description, Group: group, Scopes: scopes,
		Access: AccessRead, Risk: RiskLow, Run: run, Sources: []string{SourceAIChat, SourceMCP}})
}

type itemRef struct {
	ItemID  int    `json:"item_id,omitempty"`
	ItemKey string `json:"item_key,omitempty" jsonschema:"Work-item key such as PROJ-42"`
}

func (ref itemRef) read(ctx context.Context, env *Env) (*models.Item, error) {
	id, err := resolveItemID(env.DB, ref.ItemID, ref.ItemKey)
	if err != nil {
		return nil, repository.ErrNotFound
	}
	item, err := env.itemApplication().Get(ctx, env.UserID, id, false)
	if err != nil {
		return nil, err
	}
	if !env.HasWorkspaceAccess(item.WorkspaceID) {
		return nil, repository.ErrNotFound
	}
	return item, nil
}

type toolPage struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Page size (default 20, maximum 100)"`
	Offset int `json:"offset,omitempty" jsonschema:"Offset returned by the previous page"`
}

func (p toolPage) bounds() (limit, offset int) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	return min(p.Limit, 100), max(p.Offset, 0)
}

func pageResult[T any](items []T, total, offset int) map[string]any {
	out := map[string]any{"items": items, "total": total}
	if len(items) > 0 && offset+len(items) < total {
		out["next_offset"] = offset + len(items)
	}
	return out
}

func slicePage[T any](items []T, p toolPage) map[string]any {
	size, offset := p.bounds()
	start := min(offset, len(items))
	return pageResult(items[start:min(start+size, len(items))], len(items), offset)
}

func itemURL(item *models.Item) string {
	return fmt.Sprintf("/workspaces/%d/items/%d", item.WorkspaceID, item.ID)
}

type itemPageArgs struct {
	itemRef
	toolPage
}

type reportDates struct {
	From string `json:"from,omitempty" jsonschema:"First local date (YYYY-MM-DD)"`
	To   string `json:"to,omitempty" jsonschema:"Last local date, inclusive (YYYY-MM-DD)"`
}

func (dates reportDates) filter(filter *repository.WorklogDetailFilter, location *time.Location, required bool) error {
	if dates.From == "" && dates.To == "" && !required {
		return nil
	}
	start, end, err := services.CivilDateRangeUTC(dates.From, dates.To, location)
	if err != nil {
		return errors.New("from and to must be ordered dates in YYYY-MM-DD format")
	}
	if end.After(start.In(location).AddDate(0, 0, 366)) {
		return errors.New("date range may not exceed 366 days")
	}
	from, to := start.Unix(), end.Unix()
	filter.DateFromUnix, filter.DateToExclusiveUnix = &from, &to
	return nil
}

func (e *Env) worklogReadFilter() (repository.WorklogDetailFilter, error) {
	permission := e.TimePermService
	if permission == nil {
		permission = services.NewTimePermissionService(e.DB, e.PermService)
	}
	projects, err := permission.GetAccessibleProjects(e.UserID)
	if err != nil {
		return repository.WorklogDetailFilter{}, err
	}
	managed, err := permission.GetManagedProjects(e.UserID)
	if err != nil {
		return repository.WorklogDetailFilter{}, err
	}
	filter := repository.WorklogDetailFilter{AccessibleProjectIDs: projects, ManagedProjectIDs: managed}
	if managed != nil {
		filter.ViewerID = &e.UserID
	}
	return filter, nil
}

type itemWorklogArgs struct {
	itemPageArgs
	reportDates
}

type projectWorklogArgs struct {
	ProjectID int  `json:"project_id" jsonschema:"Time project ID from list_time_projects"`
	UserID    *int `json:"user_id,omitempty" jsonschema:"Filter by the person who logged the worklog"`
	reportDates
	toolPage
}

type deliverySummaryArgs struct {
	ItemKeys []string `json:"item_keys,omitempty" jsonschema:"One to twenty work-item keys; alternatively use item_ids"`
	ItemIDs  []int    `json:"item_ids,omitempty" jsonschema:"One to twenty numeric item IDs, when the current item has no key in context"`
	reportDates
}

type deliveryReportArgs struct {
	WorkspaceID  int    `json:"workspace_id,omitempty"`
	CollectionID int    `json:"collection_id,omitempty"`
	IterationID  int    `json:"iteration_id,omitempty"`
	View         string `json:"view,omitempty" jsonschema:"Use effort (default), variance (estimated tasks), or coverage (completed tasks with no visible bookings)"`
	reportDates
	toolPage
}

type bookedContributor struct {
	UserID        int    `json:"user_id,omitempty"`
	Name          string `json:"name,omitempty"`
	BookedMinutes int64  `json:"booked_minutes"`
	Entries       int    `json:"entries"`
}

type itemEffort struct {
	Minutes      int64
	Contributors map[int]bookedContributor
}

func bookedMinutes(start, end int64, duration int, filter repository.WorklogDetailFilter) int64 {
	if filter.DateFromUnix == nil || filter.DateToExclusiveUnix == nil {
		return int64(duration)
	}
	return services.BookedMinutesInRange(start, end, duration, *filter.DateFromUnix, *filter.DateToExclusiveUnix)
}

func readItemEffort(ctx context.Context, env *Env, items []models.Item, filter repository.WorklogDetailFilter) (map[int]*itemEffort, error) {
	totals := make(map[int]*itemEffort, len(items))
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if totals[item.ID] == nil {
			totals[item.ID] = &itemEffort{Contributors: make(map[int]bookedContributor)}
			ids = append(ids, item.ID)
		}
	}
	repo := repository.NewTimeWorklogRepository(env.DB)
	for batch := range slices.Chunk(ids, 200) {
		filter.ItemIDs = batch
		err := repo.StreamAggregateInputs(ctx, filter, func(row *repository.WorklogAggregateInput) error {
			entry := totals[row.ItemID]
			minutes := bookedMinutes(row.StartTimeUnix, row.EndTimeUnix, row.DurationMinutes, filter)
			entry.Minutes += minutes
			contributor := entry.Contributors[row.UserID]
			contributor.UserID, contributor.Name = row.UserID, row.UserName
			contributor.BookedMinutes += minutes
			contributor.Entries++
			entry.Contributors[row.UserID] = contributor
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return totals, nil
}

func orderedContributors(totals map[int]bookedContributor) []bookedContributor {
	rows := make([]bookedContributor, 0, len(totals))
	for _, entry := range totals {
		rows = append(rows, entry)
	}
	slices.SortFunc(rows, func(a, b bookedContributor) int {
		if n := cmp.Compare(b.BookedMinutes, a.BookedMinutes); n != 0 {
			return n
		}
		return cmp.Compare(a.UserID, b.UserID)
	})
	return rows
}

func linkedTestEvidence(ctx context.Context, env *Env, item *models.Item) (map[string]any, error) {
	if !env.allowScope(auth.ScopeTestsRead) {
		return nil, nil
	}
	allowed, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionTestView)
	if err != nil || !allowed {
		return nil, err
	}
	records, err := repository.NewTestQARepository(env.DB).LinkedItemResults(ctx, item.WorkspaceID, item.ID, 6)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, min(5, len(records)))
	for _, record := range records[:min(5, len(records))] {
		out = append(out, map[string]any{"result_id": record.ID, "case": record.CaseTitle, "status": record.Status, "executed_at": record.ExecutedAt,
			"url": fmt.Sprintf("/workspaces/%d/tests/runs/%d", item.WorkspaceID, record.RunID)})
	}
	return map[string]any{"results": out, "has_more": len(records) > 5}, nil
}

func taskDelivery(ctx context.Context, env *Env, item *models.Item, effort *itemEffort, completion repository.ItemCompletionFact) (map[string]any, error) {
	links, err := repository.NewItemRepository(env.DB).ListSCMLinkSummaries(ctx, item.ID, 7, 0)
	if err != nil {
		return nil, err
	}
	elapsed, err := env.itemApplication().StatusDurations(ctx, env.UserID, item.ID)
	if err != nil {
		return nil, err
	}
	contributors := orderedContributors(effort.Contributors)
	result := map[string]any{
		"item": itemToSummary(item), "url": itemURL(item), "created_at": item.CreatedAt, "completion": completion,
		"estimate_minutes": item.EstimateMinutes, "visible_booked_minutes": effort.Minutes,
		"contributor_count": len(contributors), "contributors": contributors[:min(len(contributors), 20)],
		"recorded_scm_links": links[:min(len(links), 6)], "more_scm_links": len(links) > 6, "status_durations": elapsed,
	}
	if len(contributors) == 0 {
		result["worklog_evidence"] = "no visible worklogs"
	}
	tests, err := linkedTestEvidence(ctx, env, item)
	if err != nil {
		return nil, err
	}
	if tests != nil {
		result["recorded_test_results"] = tests
	}
	if env.CommentService != nil {
		page, err := env.CommentService.GetFeedByItemID(item.ID, false, services.CommentFeedOptions{Limit: 3})
		if err != nil {
			return nil, err
		}
		comments := mapCommentsDTO(page.Comments)
		for i := range comments {
			content := []rune(comments[i].Content)
			if len(content) > 700 {
				comments[i].Content = string(content[:700]) + "…"
			}
		}
		result["recent_comments"], result["more_comments"] = comments, page.HasMore
	}
	return result, nil
}

func deliverySummary(ctx context.Context, env *Env, args deliverySummaryArgs) (any, error) {
	if len(args.ItemKeys)+len(args.ItemIDs) < 1 || len(args.ItemKeys)+len(args.ItemIDs) > 20 || len(args.ItemKeys) > 0 && len(args.ItemIDs) > 0 {
		return nil, errors.New("provide one to twenty item keys or numeric IDs")
	}
	refs := make([]itemRef, 0, len(args.ItemKeys)+len(args.ItemIDs))
	for _, key := range args.ItemKeys {
		refs = append(refs, itemRef{ItemKey: key})
	}
	for _, id := range args.ItemIDs {
		refs = append(refs, itemRef{ItemID: id})
	}
	items := make([]models.Item, 0, len(refs))
	seen := map[int]bool{}
	for index, ref := range refs {
		item, err := ref.read(ctx, env)
		if err != nil {
			return nil, fmt.Errorf("could not read item at position %d", index+1)
		}
		if !seen[item.ID] {
			seen[item.ID] = true
			items = append(items, *item)
		}
	}
	filter, err := env.worklogReadFilter()
	if err != nil {
		return nil, err
	}
	zone, location := services.ResolveTimezoneOrUTC(env.Timezone)
	if err := args.filter(&filter, location, false); err != nil {
		return nil, err
	}
	totals, err := readItemEffort(ctx, env, items, filter)
	if err != nil {
		return nil, err
	}
	completed, err := repository.NewItemRepository(env.DB).CompletionFacts(ctx, items)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, err := taskDelivery(ctx, env, &item, totals[item.ID], completed[item.ID])
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return map[string]any{"items": result, "from": args.From, "to": args.To, "timezone": zone}, nil
}

func deliveryReport(ctx context.Context, env *Env, args deliveryReportArgs) (any, error) {
	scopes := 0
	for _, id := range []int{args.WorkspaceID, args.CollectionID, args.IterationID} {
		if id < 0 {
			return nil, errors.New("scope IDs must be positive")
		}
		if id > 0 {
			scopes++
		}
	}
	if scopes != 1 {
		return nil, errors.New("provide exactly one workspace_id, collection_id, or iteration_id")
	}
	if args.View == "" {
		args.View = "effort"
	}
	if args.View != "effort" && args.View != "variance" && args.View != "coverage" {
		return nil, errors.New("view must be effort, variance, or coverage")
	}
	if args.CollectionID > 0 && !env.allowScope(auth.ScopeCollectionsRead) {
		return nil, errors.New("collection scope is unavailable")
	}
	if args.IterationID > 0 {
		if !env.allowScope(auth.ScopeIterationsRead) {
			return nil, errors.New("iteration scope is unavailable")
		}
		if env.ReadServices == nil || env.ReadServices.Planning == nil {
			return nil, errors.New("planning service is unavailable")
		}
		iteration, err := env.ReadServices.Planning.GetIteration(env.UserID, args.IterationID)
		if err != nil {
			return nil, err
		}
		if args.From == "" && args.To == "" {
			args.From, args.To = iteration.StartDate, iteration.EndDate
		}
	}
	if len(env.AccessibleWorkspaceIDs) == 0 || (args.WorkspaceID > 0 && !env.HasWorkspaceAccess(args.WorkspaceID)) {
		return nil, errors.New("report scope not found")
	}
	filter, err := env.worklogReadFilter()
	if err != nil {
		return nil, err
	}
	zone, location := services.ResolveTimezoneOrUTC(env.Timezone)
	if err := args.filter(&filter, location, true); err != nil {
		return nil, err
	}
	itemFilter := services.ItemFilters{}
	if args.IterationID > 0 {
		itemFilter.IterationID = &args.IterationID
	}
	request := services.ItemListRequest{UserID: env.UserID, WorkspaceIDs: env.AccessibleWorkspaceIDs,
		WorkspaceID: args.WorkspaceID, CollectionID: args.CollectionID, Filters: itemFilter, OmitDescriptions: true,
		Pagination: services.PaginationParams{Limit: 1000}, SortBy: "created_at", SortAsc: true}
	const maxReportItems = 5000
	var items []models.Item
	for {
		page, err := env.itemApplication().List(ctx, request)
		if err != nil {
			return nil, err
		}
		if page.Total > maxReportItems || len(items)+len(page.Items) > maxReportItems {
			return nil, errors.New("report scope exceeds 5000 items; select a smaller collection or iteration")
		}
		items = append(items, page.Items...)
		if len(items) >= page.Total || len(page.Items) < request.Pagination.Limit {
			break
		}
		request.Pagination.Offset += len(page.Items)
	}
	totals, err := readItemEffort(ctx, env, items, filter)
	if err != nil {
		return nil, err
	}
	itemRepo := repository.NewItemRepository(env.DB)
	completedInPeriod, err := itemRepo.CompletedDuring(ctx, items,
		time.Unix(*filter.DateFromUnix, 0), time.Unix(*filter.DateToExclusiveUnix, 0))
	if err != nil {
		return nil, err
	}
	worklogs := repository.NewTimeWorklogRepository(env.DB)
	lifetime := make(map[int]int64, len(items))
	for batch := range slices.Chunk(items, 200) {
		lifetimeFilter := filter
		lifetimeFilter.ItemIDs = make([]int, 0, len(batch))
		for _, item := range batch {
			lifetimeFilter.ItemIDs = append(lifetimeFilter.ItemIDs, item.ID)
		}
		group, err := worklogs.ItemLifetimeMinutes(ctx, lifetimeFilter)
		if err != nil {
			return nil, err
		}
		for id, minutes := range group {
			lifetime[id] = minutes
		}
	}
	slices.SortFunc(items, func(a, b models.Item) int {
		if n := cmp.Compare(totals[b.ID].Minutes, totals[a.ID].Minutes); n != 0 {
			return n
		}
		return cmp.Compare(a.ID, b.ID)
	})
	people := map[int]bookedContributor{}
	coverageItems := []models.Item{}
	varianceItems := []models.Item{}
	var booked int64
	var unestimated, completed, completedWithoutTime int
	for _, item := range items {
		entry := totals[item.ID]
		booked += entry.Minutes
		if item.EstimateMinutes == nil {
			unestimated++
		} else if args.View == "variance" {
			varianceItems = append(varianceItems, item)
		}
		if completedInPeriod[item.ID] {
			completed++
			if _, hasWorklogs := lifetime[item.ID]; !hasWorklogs {
				completedWithoutTime++
				coverageItems = append(coverageItems, item)
			}
		}
		for id, work := range entry.Contributors {
			sum := people[id]
			sum.UserID, sum.Name = id, work.Name
			sum.BookedMinutes += work.BookedMinutes
			sum.Entries += work.Entries
			people[id] = sum
		}
	}
	visibleItems := items
	switch args.View {
	case "coverage":
		visibleItems = coverageItems
	case "variance":
		visibleItems = varianceItems
		slices.SortStableFunc(visibleItems, func(a, b models.Item) int {
			return cmp.Compare(lifetime[b.ID]-int64(*b.EstimateMinutes), lifetime[a.ID]-int64(*a.EstimateMinutes))
		})
	}
	size, offset := args.bounds()
	start := min(offset, len(visibleItems))
	selected := visibleItems[start:min(start+min(size, 20), len(visibleItems))]
	completion, err := itemRepo.CompletionFacts(ctx, selected)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(selected))
	for _, item := range selected {
		row, err := taskDelivery(ctx, env, &item, totals[item.ID], completion[item.ID])
		if err != nil {
			return nil, err
		}
		if item.EstimateMinutes != nil {
			row["lifetime_visible_booked_minutes_minus_estimate"] = lifetime[item.ID] - int64(*item.EstimateMinutes)
		}
		rows = append(rows, row)
	}
	result := pageResult(rows, len(visibleItems), offset)
	result["view"], result["scope_item_count"] = args.View, len(items)
	contributors := orderedContributors(people)
	result["contributors"], result["contributor_count"] = contributors[:min(len(contributors), 50)], len(contributors)
	result["visible_booked_minutes"], result["without_estimate"] = booked, unestimated
	result["completed_in_period"], result["completed_without_visible_time"] = completed, completedWithoutTime
	result["from"], result["to"], result["timezone"] = args.From, args.To, zone
	return result, nil
}

func itemWorklogs(ctx context.Context, env *Env, args itemWorklogArgs) (any, error) {
	item, err := args.read(ctx, env)
	if err != nil {
		return nil, err
	}
	filter, err := env.worklogReadFilter()
	if err != nil {
		return nil, err
	}
	filter.ItemID = &item.ID
	result, err := worklogPage(env, filter, args.reportDates, args.toolPage)
	if err != nil {
		return nil, err
	}
	result["item_url"] = itemURL(item)
	return result, nil
}

func projectWorklogs(_ context.Context, env *Env, args projectWorklogArgs) (any, error) {
	if args.ProjectID <= 0 || (args.UserID != nil && *args.UserID <= 0) {
		return nil, errors.New("project_id and any supplied user_id must be positive")
	}
	filter, err := env.worklogReadFilter()
	if err != nil {
		return nil, err
	}
	if filter.AccessibleProjectIDs != nil && !slices.Contains(filter.AccessibleProjectIDs, args.ProjectID) {
		return nil, errors.New("time project not found")
	}
	project, err := repository.NewTimeProjectRepository(env.DB).GetBookingInfo(args.ProjectID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errors.New("time project not found")
	}
	if err != nil {
		return nil, err
	}
	filter.ProjectID, filter.UserID = &args.ProjectID, args.UserID
	result, err := worklogPage(env, filter, args.reportDates, args.toolPage)
	if err != nil {
		return nil, err
	}
	result["project_id"], result["project_name"] = args.ProjectID, project.Name
	return result, nil
}

func worklogPage(env *Env, filter repository.WorklogDetailFilter, dates reportDates, page toolPage) (map[string]any, error) {
	zone, location := services.ResolveTimezoneOrUTC(env.Timezone)
	if err := dates.filter(&filter, location, false); err != nil {
		return nil, err
	}
	filter.Limit, filter.Offset = page.bounds()
	rows, total, err := env.timeWorklogService().ListPage(filter)
	if err != nil {
		return nil, err
	}
	canReadItems := env.allowScope(auth.ScopeItemsRead)
	rows = services.RedactInaccessibleWorklogItems(rows, func(workspaceID int) (bool, error) {
		return canReadItems && env.HasWorkspaceAccess(workspaceID), nil
	})
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		row.ProjectMaxHours, row.ProjectTotalHours = nil, nil
		periodMinutes := bookedMinutes(row.StartTime, row.EndTime, row.DurationMins, filter)
		out = append(out, map[string]any{"worklog": row, "booked_minutes_in_range": periodMinutes, "url": worklogURL(row.ID)})
	}
	result := pageResult(out, total, filter.Offset)
	result["worklogs_url"] = "/time/worklogs"
	result["from"], result["to"], result["timezone"] = dates.From, dates.To, zone
	return result, nil
}

func itemTimeRollup(ctx context.Context, env *Env, args itemRef) (any, error) {
	root, err := args.read(ctx, env)
	if err != nil {
		return nil, err
	}
	items := []models.Item{*root}
	seen := map[int]bool{root.ID: true}
	const maxItems = 250
	truncated := false
	for pos := 0; pos < len(items) && !truncated; pos++ {
		children, err := env.itemApplication().Children(ctx, env.UserID, items[pos].ID)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if seen[child.ID] || !env.HasWorkspaceAccess(child.WorkspaceID) {
				continue
			}
			if len(items) == maxItems {
				truncated = true
				break
			}
			items = append(items, child)
			seen[child.ID] = true
		}
	}
	filter, err := env.worklogReadFilter()
	if err != nil {
		return nil, err
	}
	totals, err := readItemEffort(ctx, env, items, filter)
	if err != nil {
		return nil, err
	}
	var childMinutes, childEstimate int64
	for _, child := range items[1:] {
		childMinutes += totals[child.ID].Minutes
		if child.EstimateMinutes != nil {
			childEstimate += int64(*child.EstimateMinutes)
		}
	}
	return map[string]any{"item_url": itemURL(root), "item_count": len(items), "truncated": truncated,
		"direct_estimate_minutes": root.EstimateMinutes, "descendant_estimate_minutes": childEstimate,
		"direct_visible_booked_minutes": totals[root.ID].Minutes, "descendant_visible_booked_minutes": childMinutes}, nil
}

func init() {
	itemScope := []string{auth.ScopeItemsRead}
	effortScopes := []string{auth.ScopeItemsRead, auth.ScopeTimeRead}
	registerRead("get_task_delivery_summary", "Start here for what happened on specific tickets and who logged how much time. Returns visible booked minutes, workflow durations, recorded completion, discussion and linked SCM evidence.", CapabilityPlanningActivity, effortScopes, deliverySummary)
	registerRead("list_item_worklogs", "List visible booked entries behind a task's effort, including logger, project, dates, and notes.", CapabilityTime, effortScopes, itemWorklogs)
	registerRead("list_project_worklogs", "Drill into individual bookings behind a time project's totals, including unlinked entries. Optionally filter by logger and local dates. Project managers can read other loggers' entries.", CapabilityTime, []string{auth.ScopeTimeRead}, projectWorklogs)
	registerRead("get_delivery_effort_report", "Summarize booked effort and outcomes for one workspace, collection, or iteration over a local date range. Includes task and contributor totals and estimate and coverage signals.", CapabilityPlanningActivity, effortScopes, deliveryReport)
	registerRead("get_item_time_rollup", "Compare direct estimate and visible booked minutes with totals on visible descendants.", CapabilityTime, effortScopes, itemTimeRollup)
	registerRead("list_item_history", "See changes to an item, including actor, timestamp and old and new field values.", CapabilityPlanningActivity, itemScope, func(ctx context.Context, env *Env, args itemPageArgs) (any, error) {
		item, err := args.read(ctx, env)
		if err != nil {
			return nil, err
		}
		rows, err := env.itemApplication().History(ctx, env.UserID, item.ID)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].UserEmail, rows[i].PortalCustomerEmail = "", ""
		}
		result := slicePage(rows, args.toolPage)
		result["url"] = itemURL(item)
		return result, nil
	})
	registerRead("list_item_transitions", "Show the available status options and associated screen or approval requirements before moving an item.", CapabilityPlanningActivity, itemScope, func(ctx context.Context, env *Env, args itemRef) (any, error) {
		item, err := args.read(ctx, env)
		if err != nil {
			return nil, err
		}
		value, err := env.itemApplication().AvailableTransitions(ctx, env.UserID, item.ID)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"transitions": value, "url": itemURL(item)}
		if env.ReadServices != nil && env.ReadServices.LoadScreen != nil && env.allowScope(auth.ScopeScreensRead) {
			screens := map[int]*models.Screen{}
			for _, transition := range value.AvailableTransitions {
				if transition.Screen == nil || screens[transition.Screen.ID] != nil {
					continue
				}
				screen, err := env.ReadServices.LoadScreen(transition.Screen.ID)
				if err != nil {
					return nil, err
				}
				screens[transition.Screen.ID] = screen
			}
			out["screens"] = screens
		}
		return out, nil
	})
	registerRead("get_item_status_durations", "Get elapsed time in each workflow state from status history. Use worklog tools for booked effort.", CapabilityPlanningActivity, itemScope, func(ctx context.Context, env *Env, args itemRef) (any, error) {
		item, err := args.read(ctx, env)
		if err != nil {
			return nil, err
		}
		return env.itemApplication().StatusDurations(ctx, env.UserID, item.ID)
	})
	registerRead("list_item_scm_links", "List linked branches, commits, and pull requests with their last recorded state and URL.", CapabilityPlanningActivity, itemScope, func(ctx context.Context, env *Env, args itemPageArgs) (any, error) {
		item, err := args.read(ctx, env)
		if err != nil {
			return nil, err
		}
		limit, offset := args.bounds()
		rows, err := repository.NewItemRepository(env.DB).ListSCMLinkSummaries(ctx, item.ID, limit+1, offset)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"links": rows[:min(limit, len(rows))], "url": itemURL(item)}
		if len(rows) > limit {
			out["next_offset"] = offset + limit
		}
		return out, nil
	})
}
