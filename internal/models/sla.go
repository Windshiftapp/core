package models

import (
	"encoding/json"
	"time"
)

// SLA condition phases, ordered by evaluation precedence: stop, pause, start.
const (
	SLAPhaseStart = "start"
	SLAPhasePause = "pause"
	SLAPhaseStop  = "stop"
)

// SLA cycle statuses and origins.
const (
	SLACycleOngoing   = "ongoing"
	SLACycleCompleted = "completed"
	SLACycleAbandoned = "abandoned"

	SLAOriginNative   = "native"
	SLAOriginBackfill = "backfill"
	SLAOriginImport   = "import"
)

// SLA job kinds.
const (
	SLAJobBreach         = "breach"
	SLAJobWarning        = "warning"
	SLAJobRecalcItem     = "recalc_item"
	SLAJobRecalcMetric   = "recalc_metric"
	SLAJobRecalcCalendar = "recalc_calendar"
)

// TeamWorkspaceBinding authorizes a workspace to reference a team's service
// hours. It is the consent boundary for team calendar reuse.
type TeamWorkspaceBinding struct {
	ID          int       `json:"id"`
	TeamID      int       `json:"team_id"`
	WorkspaceID int       `json:"workspace_id"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses.
	TeamName      string `json:"team_name,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

// WorkingCalendar is a workspace- or team-owned business-hours calendar.
// Exactly one of WorkspaceID or TeamID is set.
type WorkingCalendar struct {
	ID              int             `json:"id"`
	WorkspaceID     *int            `json:"workspace_id,omitempty"`
	TeamID          *int            `json:"team_id,omitempty"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Timezone        string          `json:"timezone"`
	WeeklyIntervals json.RawMessage `json:"weekly_intervals"`
	Holidays        json.RawMessage `json:"holidays"`
	IsDefault       bool            `json:"is_default"`
	Source          *string         `json:"source,omitempty"`
	SourceID        *string         `json:"source_id,omitempty"`
	SourcePayload   *string         `json:"source_payload,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// SLAMetric is a Jira-compatible SLA definition owned by a workspace.
type SLAMetric struct {
	ID            int            `json:"id"`
	WorkspaceID   int            `json:"workspace_id"`
	Name          string         `json:"name"`
	DisplayFormat string         `json:"display_format"`
	Position      int            `json:"position"`
	IsActive      bool           `json:"is_active"`
	ImportStatus  string         `json:"import_status"`
	Source        *string        `json:"source,omitempty"`
	SourceID      *string        `json:"source_id,omitempty"`
	SourcePayload *string        `json:"source_payload,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Conditions    []SLACondition `json:"conditions,omitempty"`
	Goals         []SLAGoal      `json:"goals,omitempty"`
}

// SLACondition is one typed start, pause, or stop condition in an OR group.
type SLACondition struct {
	ID            int             `json:"id"`
	MetricID      int             `json:"metric_id"`
	Phase         string          `json:"phase"`
	Position      int             `json:"position"`
	ConditionType string          `json:"condition_type"`
	Config        json.RawMessage `json:"config"`
	SourcePayload *string         `json:"source_payload,omitempty"`
}

// SLAGoal is one ordered, QL-matched goal within a metric.
type SLAGoal struct {
	ID            int             `json:"id"`
	MetricID      int             `json:"metric_id"`
	Position      int             `json:"position"`
	QLQuery       string          `json:"ql_query"`
	OriginalJQL   *string         `json:"original_jql,omitempty"`
	ImportStatus  string          `json:"import_status"`
	SourceID      *string         `json:"source_id,omitempty"`
	SourcePayload *string         `json:"source_payload,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Targets       []SLAGoalTarget `json:"targets,omitempty"`
}

// SLAGoalTarget is the duration target for a goal. A goal has exactly one
// target; priority-specific durations are expressed as separate goals.
type SLAGoalTarget struct {
	ID               int     `json:"id"`
	GoalID           int     `json:"goal_id"`
	Position         int     `json:"position"`
	TargetMs         int64   `json:"target_ms"`
	CalendarID       int     `json:"calendar_id"`
	SourceID         *string `json:"source_id,omitempty"`
	SourcePayload    *string `json:"source_payload,omitempty"`
	CalendarSourceID string  `json:"-"`
}

// SLAWorkspaceState tracks the configuration generation for a workspace.
type SLAWorkspaceState struct {
	WorkspaceID      int `json:"workspace_id"`
	ConfigGeneration int `json:"config_generation"`
}

// ItemSLACycle is one recorded SLA cycle for an item and metric.
type ItemSLACycle struct {
	ID                  int64           `json:"id"`
	ItemID              int             `json:"item_id"`
	MetricID            int             `json:"metric_id"`
	GoalID              *int            `json:"goal_id,omitempty"`
	CalendarID          *int            `json:"calendar_id,omitempty"`
	CycleNo             int             `json:"cycle_no"`
	Status              string          `json:"status"`
	StartedAt           time.Time       `json:"started_at"`
	StoppedAt           *time.Time      `json:"stopped_at,omitempty"`
	BreachTime          *time.Time      `json:"breach_time,omitempty"`
	GoalDurationMs      int64           `json:"goal_duration_ms"`
	ElapsedMs           int64           `json:"elapsed_ms"`
	RemainingMs         int64           `json:"remaining_ms"`
	Paused              bool            `json:"paused"`
	WithinCalendarHours bool            `json:"within_calendar_hours"`
	Breached            bool            `json:"breached"`
	PauseStartedAt      *time.Time      `json:"pause_started_at,omitempty"`
	NextDeadlineAt      *time.Time      `json:"next_deadline_at,omitempty"`
	LastCalculatedAt    time.Time       `json:"last_calculated_at"`
	RemainingAtPauseMs  *int64          `json:"remaining_at_pause_ms,omitempty"`
	BreachedAt          *time.Time      `json:"breached_at,omitempty"`
	Origin              string          `json:"origin"`
	AbandonReason       *string         `json:"abandon_reason,omitempty"`
	CalendarSnapshot    json.RawMessage `json:"calendar_snapshot"`
	GoalQuerySnapshot   string          `json:"goal_query_snapshot"`
	SourceID            *string         `json:"source_id,omitempty"`
	SourcePayload       *string         `json:"source_payload,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// SLAJob is a durable unit of pending SLA work.
type SLAJob struct {
	ID           int64      `json:"id"`
	Kind         string     `json:"kind"`
	ThresholdKey string     `json:"threshold_key"`
	CycleID      *int64     `json:"cycle_id,omitempty"`
	ItemID       *int       `json:"item_id,omitempty"`
	MetricID     *int       `json:"metric_id,omitempty"`
	DueAt        time.Time  `json:"due_at"`
	DeadlineAt   *time.Time `json:"deadline_at,omitempty"`
	Cursor       *string    `json:"cursor,omitempty"`
	Attempts     int        `json:"attempts"`
	LeaseOwner   *string    `json:"lease_owner,omitempty"`
	LastError    *string    `json:"last_error,omitempty"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"created_at"`
}

// DerivedSLACycle is the read-time, Jira-shaped view of a cycle. Elapsed and
// remaining are exact as of the derivation instant; the stored columns remain
// the scheduling truth.
type DerivedSLACycle struct {
	CycleNo             int          `json:"cycle_no"`
	Status              string       `json:"status"`
	GoalID              *int         `json:"goal_id,omitempty"`
	StartedAt           time.Time    `json:"started_at"`
	StoppedAt           *time.Time   `json:"stopped_at,omitempty"`
	BreachTime          *time.Time   `json:"breach_time,omitempty"`
	ElapsedMs           int64        `json:"elapsed_ms"`
	RemainingMs         *int64       `json:"remaining_ms,omitempty"`
	GoalDurationMs      int64        `json:"goal_duration_ms"`
	Paused              bool         `json:"paused"`
	Breached            bool         `json:"breached"`
	WithinCalendarHours bool         `json:"within_calendar_hours"`
	PauseStartedAt      *time.Time   `json:"pause_started_at,omitempty"`
	NextDeadlineAt      *time.Time   `json:"next_deadline_at,omitempty"`
	CalendarTimezone    string       `json:"calendar_timezone,omitempty"`
	Coverage            *SLACoverage `json:"coverage,omitempty"`
}

// SLAReportMetric aggregates completed cycles for a metric. Deltas are stored
// values, never recomputed, so later configuration edits cannot rewrite
// history. Current-state segmentation is labeled as such.
type SLAReportMetric struct {
	MetricID          int    `json:"metric_id"`
	MetricName        string `json:"metric_name"`
	Ongoing           int    `json:"ongoing"`
	CurrentlyBreached int    `json:"currently_breached"`
	Completed         int    `json:"completed"`
	Breached          int    `json:"breached"`
	AvgElapsedMs      *int64 `json:"avg_elapsed_ms,omitempty"`
	AvgGoalMs         *int64 `json:"avg_goal_ms,omitempty"`
	MaxElapsedMs      *int64 `json:"max_elapsed_ms,omitempty"`
}

// SLAReportBreach is one completed, breached cycle.
type SLAReportBreach struct {
	CycleID        int64      `json:"cycle_id"`
	ItemID         int        `json:"item_id"`
	ItemKey        string     `json:"item_key"`
	Title          string     `json:"title"`
	MetricName     string     `json:"metric_name"`
	StatusName     string     `json:"status_name,omitempty"`
	StoppedAt      *time.Time `json:"stopped_at,omitempty"`
	BreachedAt     *time.Time `json:"breached_at,omitempty"`
	ElapsedMs      int64      `json:"elapsed_ms"`
	GoalDurationMs int64      `json:"goal_duration_ms"`
}

// SLAReport is the compliance view over completed cycles.
type SLAReport struct {
	From          *time.Time         `json:"from,omitempty"`
	To            *time.Time         `json:"to,omitempty"`
	Metrics       []SLAReportMetric  `json:"metrics"`
	BreachedItems []SLAReportBreach  `json:"breached_items"`
	Coverage      *SLAReportCoverage `json:"coverage,omitempty"`
}

// SLACoverage is the informative SLA-vs-team service-hours comparison attached
// to an item's SLA cycle. It never changes SLA semantics.
type SLACoverage struct {
	Reference              string `json:"reference"`
	TeamIDs                []int  `json:"team_ids,omitempty"`
	WithinTeamServiceHours bool   `json:"within_team_service_hours"`
	SLAWeeklyMs            int64  `json:"sla_weekly_ms,omitempty"`
	TeamWeeklyMs           int64  `json:"team_weekly_ms,omitempty"`
	OverlapWeeklyMs        int64  `json:"overlap_weekly_ms,omitempty"`
	SLAOutsideTeamWeeklyMs int64  `json:"sla_outside_team_weekly_ms,omitempty"`
	TeamOutsideSLAWeeklyMs int64  `json:"team_outside_sla_weekly_ms,omitempty"`
	Discrepancy            string `json:"discrepancy,omitempty"`
	Note                   string `json:"note,omitempty"`
}

// SLAReportCoverage aggregates the same comparison over a report window using
// each completed cycle's stored calendar snapshot.
type SLAReportCoverage struct {
	Reference                   string `json:"reference"`
	TeamIDs                     []int  `json:"team_ids,omitempty"`
	SLACountedMs                int64  `json:"sla_counted_ms,omitempty"`
	TeamServiceMs               int64  `json:"team_service_ms,omitempty"`
	UncoveredMs                 int64  `json:"uncovered_ms,omitempty"`
	ExcludedMs                  int64  `json:"excluded_ms,omitempty"`
	BreachedOutsideServiceHours int    `json:"breached_outside_service_hours"`
	CurrentStateReference       bool   `json:"current_state_reference"`
}

// ItemSLA is the Jira-shaped SLA state for one item.
type ItemSLA struct {
	ItemID        int               `json:"item_id"`
	MetricID      int               `json:"metric_id"`
	MetricName    string            `json:"metric_name"`
	DisplayFormat string            `json:"display_format"`
	Recalculating bool              `json:"recalculating"`
	Ongoing       *DerivedSLACycle  `json:"ongoing,omitempty"`
	Completed     []DerivedSLACycle `json:"completed,omitempty"`
}

// SLAWarningThreshold fires an sla.warning when a percentage of the goal has
// elapsed. A nil MetricID applies the threshold to every metric in the
// workspace. Thresholds are configuration, not goal fields, so they never
// round-trip through Jira SLA imports.
type SLAWarningThreshold struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	MetricID    *int      `json:"metric_id,omitempty"`
	Label       string    `json:"label"`
	Percent     int       `json:"percent"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses.
	MetricName string `json:"metric_name,omitempty"`
}

// SLACalendarImpact describes what a calendar edit would recalculate. It is a
// read-only preview: it lists the bound workspaces, the goal targets that
// reference the calendar, and the ongoing cycle count per workspace.
type SLACalendarImpact struct {
	CalendarID int                          `json:"calendar_id"`
	Workspaces []SLACalendarImpactWorkspace `json:"workspaces"`
}

// SLACalendarImpactWorkspace is one workspace affected by a calendar edit.
type SLACalendarImpactWorkspace struct {
	WorkspaceID   int                       `json:"workspace_id"`
	WorkspaceName string                    `json:"workspace_name"`
	GoalTargets   []SLACalendarImpactTarget `json:"goal_targets"`
	OngoingCycles int                       `json:"ongoing_cycles"`
}

// SLACalendarImpactTarget is one goal target referencing the calendar.
type SLACalendarImpactTarget struct {
	TargetID   int    `json:"target_id"`
	GoalID     int    `json:"goal_id"`
	MetricID   int    `json:"metric_id"`
	MetricName string `json:"metric_name"`
	TargetMs   int64  `json:"target_ms"`
}
