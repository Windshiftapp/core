package services

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	analyticsIDBatchSize = 400
	analyticsItemLimit   = 8
	analyticsMaxDays     = 366
)

// AnalyticsItemSummary is a drill-down row used by health, aging, and
// delivery-time reports.
type AnalyticsItemSummary struct {
	ID                  int      `json:"id"`
	WorkspaceItemNumber int      `json:"workspace_item_number"`
	Title               string   `json:"title"`
	Status              string   `json:"status"`
	AgeDays             int      `json:"age_days"`
	LastActiveDate      string   `json:"last_active_date,omitempty"`
	DueDate             string   `json:"due_date,omitempty"`
	CompletedDate       string   `json:"completed_date,omitempty"`
	DeliveryDays        float64  `json:"delivery_days,omitempty"`
	Flags               []string `json:"flags"`
}

// WorkHealthResult summarizes unfinished work that needs attention now.
type WorkHealthResult struct {
	UnfinishedItems int                    `json:"unfinished_items"`
	Overdue         int                    `json:"overdue"`
	Stale           int                    `json:"stale"`
	Unassigned      int                    `json:"unassigned"`
	WithoutPriority int                    `json:"without_priority"`
	WithoutEstimate int                    `json:"without_estimate"`
	StaleAfterDays  int                    `json:"stale_after_days"`
	AttentionItems  []AnalyticsItemSummary `json:"attention_items"`
	DataQuality     DataQuality            `json:"data_quality"`
}

// ThroughputBucket compares arrivals and first completions in a bounded week.
type ThroughputBucket struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Created   int    `json:"created"`
	Completed int    `json:"completed"`
	NetChange int    `json:"net_change"`
}

// ThroughputResult reports weekly item flow without requiring iterations.
type ThroughputResult struct {
	Buckets          []ThroughputBucket `json:"buckets"`
	TotalCreated     int                `json:"total_created"`
	TotalCompleted   int                `json:"total_completed"`
	AverageCompleted float64            `json:"average_completed"`
	Definition       string             `json:"definition"`
	DataQuality      DataQuality        `json:"data_quality"`
}

// AgingBucket groups unfinished items by age since creation.
type AgingBucket struct {
	Key       string `json:"key"`
	MinDays   int    `json:"min_days"`
	MaxDays   *int   `json:"max_days,omitempty"`
	ItemCount int    `json:"item_count"`
}

// StatusAging summarizes the current unfinished inventory for one status.
type StatusAging struct {
	Status     string  `json:"status"`
	ItemCount  int     `json:"item_count"`
	MedianDays float64 `json:"median_days"`
	P85Days    float64 `json:"p85_days"`
}

// AgingWIPResult reports the age distribution of currently unfinished work.
type AgingWIPResult struct {
	TotalItems  int                    `json:"total_items"`
	MedianDays  float64                `json:"median_days"`
	P85Days     float64                `json:"p85_days"`
	Buckets     []AgingBucket          `json:"buckets"`
	ByStatus    []StatusAging          `json:"by_status"`
	OldestItems []AnalyticsItemSummary `json:"oldest_items"`
	DataQuality DataQuality            `json:"data_quality"`
}

// DeliveryTimePoint contains one weekly completion cohort.
type DeliveryTimePoint struct {
	StartDate      string  `json:"start_date"`
	EndDate        string  `json:"end_date"`
	CompletedItems int     `json:"completed_items"`
	MedianDays     float64 `json:"median_days"`
	P85Days        float64 `json:"p85_days"`
}

// DeliveryTimeResult measures creation through first completion. First
// completion is immutable when an item is later reopened.
type DeliveryTimeResult struct {
	TotalItemsAnalyzed  int                    `json:"total_items_analyzed"`
	AverageDays         float64                `json:"average_days"`
	MedianDays          float64                `json:"median_days"`
	P85Days             float64                `json:"p85_days"`
	MissingHistoryItems int                    `json:"missing_history_items"`
	Definition          string                 `json:"definition"`
	Trend               []DeliveryTimePoint    `json:"trend"`
	SlowestItems        []AnalyticsItemSummary `json:"slowest_items"`
	DataQuality         DataQuality            `json:"data_quality"`
}

// AnalyticsCapabilities tells the UI which optional planning surfaces have
// enough structural prerequisites to be considered.
type AnalyticsCapabilities struct {
	HasIterations           bool   `json:"has_iterations"`
	TargetForecastAvailable bool   `json:"target_forecast_available"`
	TargetForecastReason    string `json:"target_forecast_reason,omitempty"`
}

type analyticsStatusMeta struct {
	ID        int
	Completed bool
}

type analyticsItem struct {
	ID                  int
	WorkspaceItemNumber int
	Title               string
	StatusID            int
	Status              string
	CurrentCompleted    bool
	DueDate             *time.Time
	HasAssignee         bool
	HasPriority         bool
	HasEstimate         bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastActiveAt        time.Time
}

type analyticsStatusEvent struct {
	ItemID      int
	At          time.Time
	OldStatusID int
	HasOld      bool
	NewStatusID int
	HasNew      bool
}

type analyticsDeliveryData struct {
	Items        []analyticsItem
	StatusByID   map[int]analyticsStatusMeta
	EventsByItem map[int][]analyticsStatusEvent
}

// GetAnalyticsContext is the request-aware analytics entry point.
func (s *AnalyticsService) GetAnalyticsContext(ctx context.Context, params ResolveDatasetParams) (*AnalyticsResult, error) {
	if params.StartDate.IsZero() || params.EndDate.IsZero() {
		return nil, fmt.Errorf("analytics date range is required")
	}
	if params.StartDate.After(params.EndDate) {
		return nil, fmt.Errorf("analytics start date must be on or before end date")
	}
	inclusiveDays := int(analyticsUTCDate(params.EndDate).Sub(analyticsUTCDate(params.StartDate)).Hours()/24) + 1
	if inclusiveDays > analyticsMaxDays {
		return nil, fmt.Errorf("analytics date range cannot exceed %d days", analyticsMaxDays)
	}

	ds, err := s.resolveDataset(ctx, params)
	if err != nil {
		return nil, err
	}

	data, err := s.loadAnalyticsDeliveryData(ctx, ds)
	if err != nil {
		return nil, err
	}
	stalenessSettings, err := s.workItemStaleness.Get()
	if err != nil {
		return nil, err
	}

	now := s.now()
	firstCompletions := analyticsFirstCompletions(data)
	hasIterations := ds.Summary.IterationCount > 0
	forecastReason := "target_required"
	if !hasIterations {
		forecastReason = "no_iterations"
	}

	return &AnalyticsResult{
		SchemaVersion: 2,
		Dataset:       ds.Summary,
		Health:        analyticsWorkHealth(data, now, stalenessSettings.StaleAfterDays),
		Throughput:    analyticsThroughput(data, firstCompletions, params.StartDate, params.EndDate),
		AgingWIP:      analyticsAgingWIP(data, now),
		DeliveryTime:  analyticsDeliveryTime(data, firstCompletions, params.StartDate, params.EndDate, now),
		Capabilities: AnalyticsCapabilities{
			HasIterations:           hasIterations,
			TargetForecastAvailable: false,
			TargetForecastReason:    forecastReason,
		},
	}, nil
}

func analyticsCohortMode(params ResolveDatasetParams) string {
	if params.CollectionID > 0 || strings.TrimSpace(params.QLQuery) != "" {
		return "current_collection"
	}
	return "current_workspace"
}

func analyticsIDChunks(ids []int) [][]int {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]int, 0, (len(ids)+analyticsIDBatchSize-1)/analyticsIDBatchSize)
	for start := 0; start < len(ids); start += analyticsIDBatchSize {
		end := start + analyticsIDBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

func analyticsIDArgs(ids []int) (placeholders string, args []any) {
	placeholderList := make([]string, len(ids))
	args = make([]any, len(ids))
	for i, id := range ids {
		placeholderList[i] = "?"
		args[i] = id
	}
	return strings.Join(placeholderList, ","), args
}

func (s *AnalyticsService) loadAnalyticsDeliveryData(ctx context.Context, ds *dataset) (*analyticsDeliveryData, error) {
	data := &analyticsDeliveryData{
		Items:        []analyticsItem{},
		StatusByID:   make(map[int]analyticsStatusMeta),
		EventsByItem: make(map[int][]analyticsStatusEvent),
	}

	statusRows, err := s.db.QueryContext(ctx, `
		SELECT st.id, sc.is_completed
		FROM statuses st
		JOIN status_categories sc ON sc.id = st.category_id
	`)
	if err != nil {
		return nil, fmt.Errorf("load analytics statuses: %w", err)
	}
	for statusRows.Next() {
		var meta analyticsStatusMeta
		if err := statusRows.Scan(&meta.ID, &meta.Completed); err != nil {
			statusRows.Close()
			return nil, fmt.Errorf("scan analytics status: %w", err)
		}
		data.StatusByID[meta.ID] = meta
	}
	if err := statusRows.Err(); err != nil {
		statusRows.Close()
		return nil, fmt.Errorf("iterate analytics statuses: %w", err)
	}
	statusRows.Close()

	for _, chunk := range analyticsIDChunks(ds.ItemIDs) {
		placeholders, args := analyticsIDArgs(chunk)
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
			SELECT i.id, i.workspace_item_number, i.title, i.status_id,
			       i.due_date, i.assignee_id, i.priority_id, i.story_points,
			       i.estimate_minutes, i.created_at, i.updated_at, i.last_active_at,
			       COALESCE(st.name, ''), COALESCE(sc.is_completed, false)
			FROM items i
			LEFT JOIN statuses st ON st.id = i.status_id
			LEFT JOIN status_categories sc ON sc.id = st.category_id
			WHERE i.id IN (%s)
		`, placeholders), args...)
		if err != nil {
			return nil, fmt.Errorf("load analytics items: %w", err)
		}

		for rows.Next() {
			var item analyticsItem
			var statusID, assigneeID, priorityID, estimateMinutes sql.NullInt64
			var storyPoints sql.NullFloat64
			var dueDate, lastActiveAt sql.NullTime
			if err := rows.Scan(
				&item.ID, &item.WorkspaceItemNumber, &item.Title, &statusID,
				&dueDate, &assigneeID, &priorityID, &storyPoints,
				&estimateMinutes, &item.CreatedAt, &item.UpdatedAt, &lastActiveAt,
				&item.Status, &item.CurrentCompleted,
			); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan analytics item: %w", err)
			}
			if statusID.Valid {
				item.StatusID = int(statusID.Int64)
			}
			if dueDate.Valid {
				due := dueDate.Time
				item.DueDate = &due
			}
			item.HasAssignee = assigneeID.Valid
			item.HasPriority = priorityID.Valid
			item.HasEstimate = (storyPoints.Valid && storyPoints.Float64 > 0) ||
				(estimateMinutes.Valid && estimateMinutes.Int64 > 0)
			switch {
			case lastActiveAt.Valid:
				item.LastActiveAt = lastActiveAt.Time
			case !item.UpdatedAt.IsZero():
				item.LastActiveAt = item.UpdatedAt
			default:
				item.LastActiveAt = item.CreatedAt
			}
			data.Items = append(data.Items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate analytics items: %w", err)
		}
		rows.Close()
	}

	for _, chunk := range analyticsIDChunks(ds.ItemIDs) {
		placeholders, args := analyticsIDArgs(chunk)
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
			SELECT ih.item_id, ih.changed_at, ih.old_value, ih.new_value
			FROM item_history ih
			JOIN statuses completed_status
			  ON ih.new_value = CAST(completed_status.id AS TEXT)
			JOIN status_categories completed_category
			  ON completed_category.id = completed_status.category_id
			WHERE ih.item_id IN (%s)
			  AND ih.field_name = 'status_id'
			  AND completed_category.is_completed = true
			ORDER BY ih.item_id, ih.changed_at, ih.id
		`, placeholders), args...)
		if err != nil {
			return nil, fmt.Errorf("load analytics status history: %w", err)
		}

		for rows.Next() {
			var event analyticsStatusEvent
			var changedAt any
			var oldValue, newValue sql.NullString
			if err := rows.Scan(&event.ItemID, &changedAt, &oldValue, &newValue); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan analytics status history: %w", err)
			}
			at, ok := analyticsDBTime(changedAt)
			if !ok {
				continue
			}
			event.At = at
			event.OldStatusID, event.HasOld = analyticsStatusID(oldValue)
			event.NewStatusID, event.HasNew = analyticsStatusID(newValue)
			data.EventsByItem[event.ItemID] = append(data.EventsByItem[event.ItemID], event)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate analytics status history: %w", err)
		}
		rows.Close()
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].ID < data.Items[j].ID
	})
	return data, nil
}

func analyticsStatusID(value sql.NullString) (int, bool) {
	if !value.Valid {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(value.String))
	if err != nil {
		return 0, false
	}
	return id, true
}

func analyticsDBTime(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, !v.IsZero()
	case string:
		return analyticsParseTime(v)
	case []byte:
		return analyticsParseTime(string(v))
	default:
		return time.Time{}, false
	}
}

func analyticsParseTime(value string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func analyticsFirstCompletions(data *analyticsDeliveryData) map[int]time.Time {
	completions := make(map[int]time.Time)
	for itemID, events := range data.EventsByItem {
		for _, event := range events {
			// Creation writes the item's starting status with no prior value.
			// That is not a transition into a completed state, so it cannot be
			// the completion event delivery time measures from.
			if !event.HasNew || !event.HasOld {
				continue
			}
			newStatus, ok := data.StatusByID[event.NewStatusID]
			if !ok || !newStatus.Completed {
				continue
			}
			if oldStatus, ok := data.StatusByID[event.OldStatusID]; ok && oldStatus.Completed {
				continue
			}
			if previous, exists := completions[itemID]; !exists || event.At.Before(previous) {
				completions[itemID] = event.At
			}
		}
	}
	return completions
}

func analyticsWorkHealth(data *analyticsDeliveryData, now time.Time, staleAfterDays int) WorkHealthResult {
	result := WorkHealthResult{
		StaleAfterDays: staleAfterDays,
		AttentionItems: []AnalyticsItemSummary{},
		DataQuality:    DataQuality{Sufficient: len(data.Items) > 0},
	}
	if len(data.Items) == 0 {
		result.DataQuality.Reason = "no_items"
		return result
	}

	nowDate := analyticsUTCDate(now)
	staleBefore := nowDate.AddDate(0, 0, -staleAfterDays)
	type rankedItem struct {
		item  analyticsItem
		flags []string
	}
	ranked := make([]rankedItem, 0)

	for _, item := range data.Items {
		if item.CurrentCompleted {
			continue
		}
		result.UnfinishedItems++
		flags := make([]string, 0, 5)
		if item.DueDate != nil && analyticsUTCDate(*item.DueDate).Before(nowDate) {
			result.Overdue++
			flags = append(flags, "overdue")
		}
		if analyticsUTCDate(item.LastActiveAt).Before(staleBefore) {
			result.Stale++
			flags = append(flags, "stale")
		}
		if !item.HasAssignee {
			result.Unassigned++
			flags = append(flags, "unassigned")
		}
		if !item.HasPriority {
			result.WithoutPriority++
			flags = append(flags, "without_priority")
		}
		if !item.HasEstimate {
			result.WithoutEstimate++
			flags = append(flags, "without_estimate")
		}
		if len(flags) > 0 {
			ranked = append(ranked, rankedItem{item: item, flags: flags})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		iOverdue := containsAnalyticsFlag(ranked[i].flags, "overdue")
		jOverdue := containsAnalyticsFlag(ranked[j].flags, "overdue")
		if iOverdue != jOverdue {
			return iOverdue
		}
		if len(ranked[i].flags) != len(ranked[j].flags) {
			return len(ranked[i].flags) > len(ranked[j].flags)
		}
		return ranked[i].item.CreatedAt.Before(ranked[j].item.CreatedAt)
	})
	for i := 0; i < len(ranked) && i < analyticsItemLimit; i++ {
		result.AttentionItems = append(result.AttentionItems, analyticsSummary(ranked[i].item, now, ranked[i].flags))
	}
	return result
}

func analyticsAgingWIP(data *analyticsDeliveryData, now time.Time) AgingWIPResult {
	max7, max14, max30, max60 := 7, 14, 30, 60
	result := AgingWIPResult{
		Buckets: []AgingBucket{
			{Key: "0_7", MinDays: 0, MaxDays: &max7},
			{Key: "8_14", MinDays: 8, MaxDays: &max14},
			{Key: "15_30", MinDays: 15, MaxDays: &max30},
			{Key: "31_60", MinDays: 31, MaxDays: &max60},
			{Key: "61_plus", MinDays: 61},
		},
		ByStatus:    []StatusAging{},
		OldestItems: []AnalyticsItemSummary{},
	}

	ages := make([]float64, 0)
	statusAges := make(map[string][]float64)
	active := make([]analyticsItem, 0)
	for _, item := range data.Items {
		if item.CurrentCompleted {
			continue
		}
		age := float64(analyticsAgeDays(item.CreatedAt, now))
		ages = append(ages, age)
		status := item.Status
		statusAges[status] = append(statusAges[status], age)
		active = append(active, item)

		switch {
		case age <= 7:
			result.Buckets[0].ItemCount++
		case age <= 14:
			result.Buckets[1].ItemCount++
		case age <= 30:
			result.Buckets[2].ItemCount++
		case age <= 60:
			result.Buckets[3].ItemCount++
		default:
			result.Buckets[4].ItemCount++
		}
	}

	result.TotalItems = len(active)
	result.MedianDays = analyticsPercentile(ages, 50)
	result.P85Days = analyticsPercentile(ages, 85)
	result.DataQuality = DataQuality{Sufficient: len(active) > 0}
	if len(active) == 0 {
		result.DataQuality.Reason = "no_active_items"
		return result
	}

	for status, values := range statusAges {
		result.ByStatus = append(result.ByStatus, StatusAging{
			Status: status, ItemCount: len(values),
			MedianDays: analyticsPercentile(values, 50),
			P85Days:    analyticsPercentile(values, 85),
		})
	}
	sort.Slice(result.ByStatus, func(i, j int) bool {
		if result.ByStatus[i].ItemCount != result.ByStatus[j].ItemCount {
			return result.ByStatus[i].ItemCount > result.ByStatus[j].ItemCount
		}
		return result.ByStatus[i].Status < result.ByStatus[j].Status
	})

	sort.Slice(active, func(i, j int) bool {
		return active[i].CreatedAt.Before(active[j].CreatedAt)
	})
	for i := 0; i < len(active) && i < analyticsItemLimit; i++ {
		result.OldestItems = append(result.OldestItems, analyticsSummary(active[i], now, nil))
	}
	return result
}

func analyticsThroughput(
	data *analyticsDeliveryData,
	firstCompletions map[int]time.Time,
	startDate, endDate time.Time,
) ThroughputResult {
	buckets := analyticsThroughputBuckets(startDate, endDate)
	result := ThroughputResult{
		Buckets: buckets, Definition: "first_completion_event",
		DataQuality: DataQuality{Sufficient: len(data.Items) > 0},
	}
	if len(data.Items) == 0 {
		result.DataQuality.Reason = "no_items"
		return result
	}

	for _, item := range data.Items {
		if index, ok := analyticsBucketIndex(item.CreatedAt, startDate, endDate); ok {
			result.Buckets[index].Created++
			result.TotalCreated++
		}
		if completedAt, ok := firstCompletions[item.ID]; ok {
			if index, ok := analyticsBucketIndex(completedAt, startDate, endDate); ok {
				result.Buckets[index].Completed++
				result.TotalCompleted++
			}
		}
	}
	for i := range result.Buckets {
		result.Buckets[i].NetChange = result.Buckets[i].Created - result.Buckets[i].Completed
	}
	if len(result.Buckets) > 0 {
		result.AverageCompleted = float64(result.TotalCompleted) / float64(len(result.Buckets))
	}
	return result
}

func analyticsDeliveryTime(
	data *analyticsDeliveryData,
	firstCompletions map[int]time.Time,
	startDate, endDate, now time.Time,
) DeliveryTimeResult {
	type sample struct {
		item        analyticsItem
		completedAt time.Time
		days        float64
	}
	samples := make([]sample, 0)
	values := make([]float64, 0)
	missingHistory := 0
	for _, item := range data.Items {
		completedAt, ok := firstCompletions[item.ID]
		if item.CurrentCompleted && !ok {
			missingHistory++
		}
		if !ok || !analyticsInDateRange(completedAt, startDate, endDate) {
			continue
		}
		days := completedAt.Sub(item.CreatedAt).Hours() / 24
		if days < 0 {
			days = 0
		}
		samples = append(samples, sample{item: item, completedAt: completedAt, days: days})
		values = append(values, days)
	}

	trendBuckets := analyticsDeliveryBuckets(startDate, endDate)
	bucketValues := make([][]float64, len(trendBuckets))
	for _, entry := range samples {
		if index, ok := analyticsBucketIndex(entry.completedAt, startDate, endDate); ok {
			bucketValues[index] = append(bucketValues[index], entry.days)
		}
	}
	for i := range trendBuckets {
		trendBuckets[i].CompletedItems = len(bucketValues[i])
		trendBuckets[i].MedianDays = analyticsPercentile(bucketValues[i], 50)
		trendBuckets[i].P85Days = analyticsPercentile(bucketValues[i], 85)
	}

	result := DeliveryTimeResult{
		TotalItemsAnalyzed:  len(samples),
		AverageDays:         analyticsAverage(values),
		MedianDays:          analyticsPercentile(values, 50),
		P85Days:             analyticsPercentile(values, 85),
		MissingHistoryItems: missingHistory,
		Definition:          "creation_to_first_completion",
		Trend:               trendBuckets,
		SlowestItems:        []AnalyticsItemSummary{},
		DataQuality:         DataQuality{Sufficient: len(samples) >= 3},
	}
	switch len(samples) {
	case 0:
		result.DataQuality.Reason = "no_completed_items"
	case 1, 2:
		result.DataQuality.Reason = "few_completed_items"
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].days > samples[j].days
	})
	for i := 0; i < len(samples) && i < analyticsItemLimit; i++ {
		summary := analyticsSummary(samples[i].item, now, nil)
		summary.CompletedDate = analyticsUTCDate(samples[i].completedAt).Format("2006-01-02")
		summary.DeliveryDays = samples[i].days
		result.SlowestItems = append(result.SlowestItems, summary)
	}
	return result
}

func analyticsThroughputBuckets(startDate, endDate time.Time) []ThroughputBucket {
	start := analyticsUTCDate(startDate)
	end := analyticsUTCDate(endDate)
	buckets := make([]ThroughputBucket, 0)
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 7) {
		bucketEnd := cursor.AddDate(0, 0, 6)
		if bucketEnd.After(end) {
			bucketEnd = end
		}
		buckets = append(buckets, ThroughputBucket{
			StartDate: cursor.Format("2006-01-02"),
			EndDate:   bucketEnd.Format("2006-01-02"),
		})
	}
	return buckets
}

func analyticsDeliveryBuckets(startDate, endDate time.Time) []DeliveryTimePoint {
	throughput := analyticsThroughputBuckets(startDate, endDate)
	result := make([]DeliveryTimePoint, len(throughput))
	for i, bucket := range throughput {
		result[i] = DeliveryTimePoint{StartDate: bucket.StartDate, EndDate: bucket.EndDate}
	}
	return result
}

func analyticsBucketIndex(value, startDate, endDate time.Time) (int, bool) {
	if !analyticsInDateRange(value, startDate, endDate) {
		return 0, false
	}
	start := analyticsUTCDate(startDate)
	valueDate := analyticsUTCDate(value)
	days := int(valueDate.Sub(start).Hours() / 24)
	return days / 7, true
}

func analyticsInDateRange(value, startDate, endDate time.Time) bool {
	start := analyticsUTCDate(startDate)
	endExclusive := analyticsUTCDate(endDate).AddDate(0, 0, 1)
	value = value.UTC()
	return !value.Before(start) && value.Before(endExclusive)
}

func analyticsUTCDate(value time.Time) time.Time {
	value = value.UTC()
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func analyticsAgeDays(createdAt, now time.Time) int {
	days := int(analyticsUTCDate(now).Sub(analyticsUTCDate(createdAt)).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func analyticsSummary(item analyticsItem, now time.Time, flags []string) AnalyticsItemSummary {
	summary := AnalyticsItemSummary{
		ID: item.ID, WorkspaceItemNumber: item.WorkspaceItemNumber,
		Title: item.Title, Status: item.Status,
		AgeDays: analyticsAgeDays(item.CreatedAt, now),
		Flags:   append([]string{}, flags...),
	}
	if !item.LastActiveAt.IsZero() {
		summary.LastActiveDate = analyticsUTCDate(item.LastActiveAt).Format("2006-01-02")
	}
	if item.DueDate != nil {
		summary.DueDate = analyticsUTCDate(*item.DueDate).Format("2006-01-02")
	}
	return summary
}

func containsAnalyticsFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if flag == target {
			return true
		}
	}
	return false
}

func analyticsAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func analyticsPercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(percentile/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
