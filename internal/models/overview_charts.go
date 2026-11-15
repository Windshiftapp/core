package models

// OverviewChartBucket contains requested daily series; unrequested series are omitted.
type OverviewChartBucket struct {
	Date      string `json:"date"`
	Created   *int   `json:"created,omitempty"`
	Completed *int   `json:"completed,omitempty"`
}

// OverviewCharts reports current-cohort activity in inclusive UTC calendar dates.
type OverviewCharts struct {
	From                 string                `json:"from"`
	To                   string                `json:"to"`
	Timezone             string                `json:"timezone"`
	CompletionDefinition string                `json:"completion_definition"`
	Buckets              []OverviewChartBucket `json:"buckets"`
}
