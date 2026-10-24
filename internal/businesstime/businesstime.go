// Package businesstime implements pure business-calendar time arithmetic.
//
// A Calendar is compiled once from stored configuration and then drives all
// SLA duration math. The three operations are:
//
//	AddCalendarTime(calendar, from, duration) -> (deadline, ok)
//	ElapsedCalendarTime(calendar, from, to)   -> duration
//	IsWithinCalendar(calendar, instant)       -> bool
//
// Business time counts wall-clock working minutes of the calendar's schedule.
// A weekly interval is a fixed duration regardless of daylight-saving
// transitions: an 09:00-17:00 shift counts eight business hours on every day it
// runs, including the days a DST gap or fold occurs. Interval endpoints are
// resolved to instants with the standard library's civil-time rules, so a
// nonexistent start advances to the first valid instant and an ambiguous start
// uses the earlier occurrence.
package businesstime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// AddHorizon bounds AddCalendarTime's forward search. A calendar that never
// opens returns ok = false instead of looping forever.
const AddHorizon = 5 * 366 * 24 * time.Hour

// ClockInterval is a wall-clock interval on a single weekday, in "HH:MM"
// form. The start is inclusive and the end is exclusive.
type ClockInterval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Holiday is a dated closure. Recurring holidays repeat on the same month and
// day every year, so annual holidays do not need to be expanded into a finite
// list.
type Holiday struct {
	Date      string `json:"date,omitempty"` // YYYY-MM-DD, required unless recurring
	MonthDay  string `json:"month_day,omitempty"`
	Recurring bool   `json:"recurring,omitempty"`
	Name      string `json:"name,omitempty"`
}

// RawCalendar is the persisted shape of a working calendar. WeeklyIntervals
// and Holidays are JSON encoded so the same struct serves both the repository
// row and a stored cycle snapshot.
type RawCalendar struct {
	Timezone        string                     `json:"timezone"`
	WeeklyIntervals map[string][]ClockInterval `json:"weekly_intervals"`
	Holidays        []Holiday                  `json:"holidays,omitempty"`
}

// Calendar is a compiled, immutable working calendar.
type Calendar struct {
	location *time.Location

	// weekly[weekday] holds the day's intervals in minutes from local midnight.
	weekly [7][]minuteInterval

	// weeklyDuration is the nominal working time of one full week. It ignores
	// holidays and is constant, which lets long elapsed ranges use arithmetic.
	weeklyDuration time.Duration

	holidays    map[int]struct{} // concrete closures keyed by civil day number
	recurring   map[[2]int]struct{}
	holidayList []Holiday // retained source entries for snapshots and diagnostics
}

type minuteInterval struct {
	start int // inclusive minutes from local midnight
	end   int // exclusive minutes from local midnight
}

var weekdayNames = [7]string{
	"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday",
}

// Compile validates and compiles a raw calendar. An empty weekly schedule is
// valid and yields a calendar with no working time; AddCalendarTime on it
// returns ok = false.
func Compile(raw RawCalendar) (*Calendar, error) {
	name := strings.TrimSpace(raw.Timezone)
	if name == "" {
		name = "UTC"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid IANA timezone %q", raw.Timezone)
	}

	calendar := &Calendar{
		location:    location,
		holidays:    make(map[int]struct{}),
		recurring:   make(map[[2]int]struct{}),
		holidayList: append([]Holiday(nil), raw.Holidays...),
	}

	seenWeekday := make(map[int]string, len(raw.WeeklyIntervals))
	for day, intervals := range raw.WeeklyIntervals {
		index, ok := weekdayIndex(day)
		if !ok {
			return nil, fmt.Errorf("unknown weekday %q", day)
		}
		if previous, ok := seenWeekday[index]; ok {
			return nil, fmt.Errorf("duplicate weekday %q (already defined as %q)", day, previous)
		}
		seenWeekday[index] = day
		for _, interval := range intervals {
			start, err := parseClock(interval.Start)
			if err != nil {
				return nil, fmt.Errorf("%s interval start: %w", day, err)
			}
			end, err := parseClock(interval.End)
			if err != nil {
				return nil, fmt.Errorf("%s interval end: %w", day, err)
			}
			if end <= start {
				return nil, fmt.Errorf("%s interval %s-%s must end after it starts", day, interval.Start, interval.End)
			}
			calendar.weekly[index] = append(calendar.weekly[index], minuteInterval{start: start, end: end})
		}
		sort.Slice(calendar.weekly[index], func(a, b int) bool {
			return calendar.weekly[index][a].start < calendar.weekly[index][b].start
		})
		calendar.weekly[index] = mergeIntervals(calendar.weekly[index])
	}
	for index := range calendar.weekly {
		calendar.weeklyDuration += weekdaySeconds(calendar.weekly[index])
	}

	for _, holiday := range raw.Holidays {
		if holiday.Recurring {
			month, day, err := parseMonthDay(holiday)
			if err != nil {
				return nil, fmt.Errorf("recurring holiday: %w", err)
			}
			calendar.recurring[[2]int{month, day}] = struct{}{}
			continue
		}
		date, err := time.Parse(time.DateOnly, strings.TrimSpace(holiday.Date))
		if err != nil {
			return nil, fmt.Errorf("holiday %q: use YYYY-MM-DD", holiday.Date)
		}
		calendar.holidays[civilDayNumber(date)] = struct{}{}
	}

	return calendar, nil
}

// CompileJSON compiles a calendar from its stored JSON encoding.
func CompileJSON(encoded string) (*Calendar, error) {
	raw, err := ParseRaw(encoded)
	if err != nil {
		return nil, err
	}
	return Compile(raw)
}

// FromStored builds a RawCalendar from the working_calendars columns: timezone
// is stored separately, weekly_intervals is a bare weekday-to-intervals map,
// and holidays is a bare array. This is distinct from the full RawCalendar
// envelope used for cycle snapshots.
func FromStored(timezone string, weeklyIntervals, holidays json.RawMessage) (RawCalendar, error) {
	raw := RawCalendar{Timezone: timezone}
	if len(weeklyIntervals) > 0 {
		if err := json.Unmarshal(weeklyIntervals, &raw.WeeklyIntervals); err != nil {
			return RawCalendar{}, fmt.Errorf("decode weekly_intervals: %w", err)
		}
	}
	if len(holidays) > 0 {
		if err := json.Unmarshal(holidays, &raw.Holidays); err != nil {
			return RawCalendar{}, fmt.Errorf("decode holidays: %w", err)
		}
	}
	return raw, nil
}

// AlwaysOpenRaw returns a 24/7 schedule in the given zone. It is the default
// when no calendar is selected and the seed for a new workspace.
func AlwaysOpenRaw(timezone string) RawCalendar {
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}
	allDay := []ClockInterval{{Start: "00:00", End: "24:00"}}
	weekly := make(map[string][]ClockInterval, 7)
	for _, day := range weekdayNames {
		weekly[day] = allDay
	}
	return RawCalendar{Timezone: timezone, WeeklyIntervals: weekly}
}

// ParseRaw decodes the stored calendar JSON. A blank value is a 24/7 UTC
// calendar, the default when no calendar is selected.
func ParseRaw(encoded string) (RawCalendar, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return AlwaysOpenRaw("UTC"), nil
	}
	var raw RawCalendar
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return RawCalendar{}, fmt.Errorf("decode calendar: %w", err)
	}
	return raw, nil
}

// Location reports the calendar's time zone.
func (c *Calendar) Location() *time.Location { return c.location }

// WithinCalendarHours reports whether instant falls inside a working interval.
func (c *Calendar) WithinCalendarHours(instant time.Time) bool {
	local := instant.In(c.location)
	if c.isHoliday(local) {
		return false
	}
	for _, interval := range c.weekly[int(local.Weekday())] {
		start, end := c.intervalInstants(local, interval)
		if !instant.Before(start) && instant.Before(end) {
			return true
		}
	}
	return false
}

// AddCalendarTime advances from by exactly duration of business time and
// returns the resulting instant. It returns ok = false when the calendar never
// opens within the search horizon. A non-positive duration returns from.
func (c *Calendar) AddCalendarTime(from time.Time, duration time.Duration) (time.Time, bool) {
	if duration <= 0 {
		return from, true
	}
	remaining := duration
	cursor := from
	horizon := from.Add(AddHorizon)
	for cursor.Before(horizon) {
		local := cursor.In(c.location)
		day := local
		advanced := false
		if !c.isHoliday(day) {
			for _, interval := range c.weekly[int(day.Weekday())] {
				start, end := c.intervalInstants(day, interval)
				if !end.After(cursor) {
					continue
				}
				segmentStart := start
				if segmentStart.Before(cursor) {
					segmentStart = cursor
				}
				if !end.After(segmentStart) {
					continue
				}
				segment := end.Sub(segmentStart)
				if remaining <= segment {
					return segmentStart.Add(remaining), true
				}
				remaining -= segment
				cursor = end
				advanced = true
			}
		}
		// Always move off the current local day, even when it had no working
		// time, so an empty calendar still terminates at the horizon.
		next := startOfNextDay(day.In(c.location))
		if !next.After(cursor) {
			if advanced {
				continue
			}
			next = cursor.Add(time.Second)
		}
		cursor = next
	}
	return time.Time{}, false
}

// ElapsedCalendarTime returns the business time between from and to. It
// returns zero when to is not after from.
func (c *Calendar) ElapsedCalendarTime(from, to time.Time) time.Duration {
	if !to.After(from) {
		return 0
	}
	localFrom := from.In(c.location)
	localTo := to.In(c.location)

	// Walk the first partial day, then use whole-week arithmetic for the
	// interior before walking the trailing days. Only the edges cost per-day
	// work, so a multi-month range is bounded by a constant.
	cursor := localFrom
	firstDayEnd := startOfNextDay(cursor)
	total := time.Duration(0)
	if firstDayEnd.Before(localTo) {
		total += c.businessOnDay(cursor, firstDayEnd)
		cursor = firstDayEnd
	}

	lastDayStart := dayAtMidnight(localTo)
	days := civilDayNumber(lastDayStart) - civilDayNumber(cursor)
	if days > 0 {
		fullWeeks := days / 7
		if fullWeeks > 0 {
			interiorEnd := addCalendarDays(cursor, fullWeeks*7)
			total += time.Duration(fullWeeks) * c.weeklyDuration
			total -= c.holidaySeconds(cursor, interiorEnd)
			total += c.dstAdjustment(cursor, interiorEnd)
			cursor = interiorEnd
		}
		for cursor.Before(lastDayStart) {
			next := startOfNextDay(cursor)
			total += c.businessOnDay(cursor, next)
			cursor = next
		}
	}
	if cursor.Before(localTo) {
		total += c.businessOnDay(cursor, localTo)
	}
	return total
}

// NextOpening returns the first instant at or after from that lies inside a
// working interval, and whether one exists within the search horizon. It is
// used to anchor backfilled cycles to real business time.
func (c *Calendar) NextOpening(from time.Time) (time.Time, bool) {
	if c.WithinCalendarHours(from) {
		return from, true
	}
	cursor := dayAtMidnight(from.In(c.location))
	horizon := from.Add(AddHorizon)
	for cursor.Before(horizon) {
		local := cursor.In(c.location)
		if !c.isHoliday(local) {
			for _, interval := range c.weekly[int(local.Weekday())] {
				start, end := c.intervalInstants(local, interval)
				if start.After(from) {
					return start, true
				}
				if from.Before(end) {
					return from, true
				}
			}
		}
		cursor = startOfNextDay(cursor)
	}
	return time.Time{}, false
}

// businessOnDay returns the working time on the local day containing windowStart,
// clipped to [windowStart, windowEnd). windowEnd is exclusive and both bounds
// must fall on the same local day.
func (c *Calendar) businessOnDay(windowStart, windowEnd time.Time) time.Duration {
	local := dayAtMidnight(windowStart.In(c.location))
	if c.isHoliday(local) {
		return 0
	}
	var total time.Duration
	for _, interval := range c.weekly[int(local.Weekday())] {
		start, end := c.intervalInstants(local, interval)
		if start.Before(windowStart) {
			start = windowStart
		}
		if end.After(windowEnd) {
			end = windowEnd
		}
		if end.After(start) {
			total += end.Sub(start)
		}
	}
	return total
}

// holidaySeconds subtracts the real working time of the holidays whose nominal
// time was included in the whole-week arithmetic of [start, end). A day listed
// as both a concrete and a recurring holiday is subtracted once. A recurring
// date that does not exist in a year (for example February 29) is skipped.
func (c *Calendar) holidaySeconds(start, end time.Time) time.Duration {
	startDay := civilDayNumber(dayAtMidnight(start.In(c.location)))
	endDay := civilDayNumber(dayAtMidnight(end.In(c.location)))
	var total time.Duration
	seen := make(map[int]struct{})
	addHoliday := func(midnight time.Time) {
		day := civilDayNumber(midnight)
		if day < startDay || day >= endDay {
			return
		}
		if _, ok := seen[day]; ok {
			return
		}
		seen[day] = struct{}{}
		// Subtract the nominal weekday time included in weeklyDuration.
		total += weekdaySeconds(c.weekly[int(midnight.Weekday())])
	}
	for day := range c.holidays {
		startDayTime := time.Unix(int64(day)*86400, 0).UTC()
		if midnight, ok := civilDayStart(startDayTime.Year(), startDayTime.Month(), startDayTime.Day(), c.location); ok {
			addHoliday(midnight)
		}
	}
	startYear := start.In(c.location).Year()
	endYear := end.In(c.location).Year()
	for monthDay := range c.recurring {
		for year := startYear; year <= endYear; year++ {
			midnight, ok := civilDayStart(year, time.Month(monthDay[0]), monthDay[1], c.location)
			if !ok {
				continue
			}
			addHoliday(midnight)
		}
	}
	return total
}

// dstAdjustment corrects the nominal whole-week arithmetic for clock
// transitions in [start, end). For each transition it replaces the nominal
// duration of the transition day's weekday with the real duration of that
// day's working intervals, so an interval endpoint that falls inside a DST gap
// is corrected per interval. Transitions are found by sampling every seven
// days and binary searching; modern zones have at most two transitions a year.
// The final sample is clamped to end so a transition in the trailing partial
// week is not missed.
func (c *Calendar) dstAdjustment(start, end time.Time) time.Duration {
	const step = 7 * 24 * time.Hour
	loc := c.location
	var total time.Duration
	apply := func(instant time.Time) {
		day := dayAtMidnight(instant.In(loc))
		if c.isHoliday(day) {
			return
		}
		nominal := weekdaySeconds(c.weekly[int(day.Weekday())])
		total += c.businessOnDay(day, startOfNextDay(day)) - nominal
	}
	findTransition := func(lo, hi time.Time, previousOffset int) time.Time {
		for hi.Sub(lo) > time.Second {
			mid := lo.Add(hi.Sub(lo) / 2)
			if _, midOffset := mid.In(loc).Zone(); midOffset == previousOffset {
				lo = mid
			} else {
				hi = mid
			}
		}
		return hi
	}

	_, startOffset := start.In(loc).Zone()
	// A transition exactly at start lands on the first interior day, which the
	// weekly sampling below cannot observe because the offset is already new.
	if _, beforeOffset := start.Add(-time.Second).In(loc).Zone(); beforeOffset != startOffset {
		apply(start)
	}
	cursor := start
	previousOffset := startOffset
	for cursor.Before(end) {
		next := cursor.Add(step)
		if next.After(end) {
			next = end
		}
		_, offset := next.In(loc).Zone()
		if offset != previousOffset {
			hi := findTransition(cursor, next, previousOffset)
			if hi.Before(end) {
				apply(hi)
			}
			previousOffset = offset
		}
		cursor = next
	}
	return total
}

func (c *Calendar) isHoliday(local time.Time) bool {
	if _, ok := c.holidays[civilDayNumber(local)]; ok {
		return true
	}
	if len(c.recurring) == 0 {
		return false
	}
	_, ok := c.recurring[[2]int{int(local.Month()), local.Day()}]
	return ok
}

// intervalInstants resolves a wall-clock interval on a given day to absolute
// instants. A nonexistent endpoint (inside a DST gap) advances to the first
// valid instant after the gap; an ambiguous endpoint uses the earlier
// occurrence. An end of 24:00 is the start of the next local day.
func (c *Calendar) intervalInstants(day time.Time, interval minuteInterval) (start, end time.Time) {
	year, month, date := day.In(c.location).Date()
	start = resolveWallClock(year, month, date, interval.start, c.location)
	if interval.end >= 24*60 {
		end = startOfNextDay(start)
	} else {
		end = resolveWallClock(year, month, date, interval.end, c.location)
	}
	return start, end
}

// resolveWallClock returns the instant for a wall-clock time on the given
// date. The standard library resolves a nonexistent time to an unspecified
// side of the gap; this normalizes it to the first valid instant after the
// gap so interval endpoints inside a gap are consistent. An ambiguous time
// resolves to the earlier occurrence.
func resolveWallClock(year int, month time.Month, day, minutes int, loc *time.Location) time.Time {
	hour, minute := minutes/60, minutes%60
	candidate := time.Date(year, month, day, hour, minute, 0, 0, loc)
	local := candidate.In(loc)
	if local.Year() == year && local.Month() == month && local.Day() == day && local.Hour() == hour && local.Minute() == minute {
		return candidate
	}
	// Nonexistent: advance from the first valid instant of the date until the
	// wall clock reaches the requested time (a gap is at most a few hours).
	cursor := dayStart(year, month, day, loc)
	for i := 0; i < 24*60; i++ {
		local := cursor.In(loc)
		if local.Year() == year && local.Month() == month && local.Day() == day && local.Hour()*60+local.Minute() >= minutes {
			return cursor
		}
		cursor = cursor.Add(time.Minute)
	}
	return candidate
}

// mergeIntervals coalesces sorted intervals that overlap or touch, so a
// schedule listing 09:00-12:00 and 10:00-13:00 counts three hours, not five.
func mergeIntervals(intervals []minuteInterval) []minuteInterval {
	if len(intervals) < 2 {
		return intervals
	}
	merged := make([]minuteInterval, 0, len(intervals))
	for _, interval := range intervals {
		if len(merged) == 0 || interval.start > merged[len(merged)-1].end {
			merged = append(merged, interval)
			continue
		}
		last := &merged[len(merged)-1]
		if interval.end > last.end {
			last.end = interval.end
		}
	}
	return merged
}

func weekdaySeconds(intervals []minuteInterval) time.Duration {
	var total time.Duration
	for _, interval := range intervals {
		total += time.Duration(interval.end-interval.start) * time.Minute
	}
	return total
}

func weekdayIndex(name string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for index, candidate := range weekdayNames {
		if normalized == candidate || normalized == candidate[:3] {
			return index, true
		}
	}
	return 0, false
}

func parseClock(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "24:00" {
		return 24 * 60, nil
	}
	parsed, err := time.Parse("15:04", trimmed)
	if err != nil {
		return 0, fmt.Errorf("time %q must be HH:MM", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func parseMonthDay(holiday Holiday) (month, day int, err error) {
	value := strings.TrimSpace(holiday.MonthDay)
	if value == "" {
		// A recurring holiday may still carry a concrete date; reuse its
		// month and day.
		date, err := time.Parse(time.DateOnly, strings.TrimSpace(holiday.Date))
		if err != nil {
			return 0, 0, fmt.Errorf("recurring holiday needs month_day or a date")
		}
		return int(date.Month()), date.Day(), nil
	}
	parsed, err := time.Parse("01-02", value)
	if err != nil {
		return 0, 0, fmt.Errorf("month_day %q must be MM-DD", value)
	}
	return int(parsed.Month()), parsed.Day(), nil
}

// dayAtMidnight returns the first instant of the local day containing t.
func dayAtMidnight(t time.Time) time.Time {
	year, month, day := t.Date()
	return dayStart(year, month, day, t.Location())
}

// dayStart returns the first instant of the given local date. Zones that skip
// midnight on a DST transition make time.Date(...,0,0,0,0) render the previous
// day, so step forward until the local date matches.
func dayStart(year int, month time.Month, day int, loc *time.Location) time.Time {
	start := time.Date(year, month, day, 0, 0, 0, 0, loc)
	for i := 0; i < 4; i++ {
		y, m, d := start.In(loc).Date()
		if y == year && m == month && d == day {
			break
		}
		start = start.Add(time.Hour)
	}
	return start
}

// civilDayStart returns the first instant of a civil date and whether that
// date exists. February 29 in a non-leap year does not exist.
func civilDayStart(year int, month time.Month, day int, loc *time.Location) (time.Time, bool) {
	noon := time.Date(year, month, day, 12, 0, 0, 0, loc)
	y, m, d := noon.In(loc).Date()
	if y != year || m != month || d != day {
		return time.Time{}, false
	}
	return dayStart(year, month, day, loc), true
}

// addCalendarDays returns the first instant of the local day that is days
// civil days after t's date.
func addCalendarDays(t time.Time, days int) time.Time {
	year, month, day := t.Date()
	noon := time.Date(year, month, day, 12, 0, 0, 0, t.Location()).AddDate(0, 0, days)
	y, m, d := noon.Date()
	return dayStart(y, m, d, t.Location())
}

// startOfNextDay returns the first instant of the local day after t's date.
func startOfNextDay(t time.Time) time.Time {
	return addCalendarDays(t, 1)
}

// civilDayNumber is a DST-independent day counter. It lets date differences
// avoid the 23/25-hour days produced by clock transitions.
func civilDayNumber(t time.Time) int {
	year, month, day := t.Date()
	return int(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}
