# Worklog time model (0.8.8)

Fixes WI-1279, WI-1280, WI-1281, WI-1282 (report: GitHub issue 247).

## Agreed model

A worklog entry is constructed from a selected civil date, optional clock
times, and an explicit entry timezone. The three fields together resolve to
complete UTC start/end timestamps, which are the single source of truth for
display, filtering, and reporting.

- Explicit `start_time`/`end_time` clocks take precedence over a supplied
  `duration`; when both are sent the duration must match or the request is
  rejected with 400 (`duration does not match start_time and end_time`).
- `start_time` plus `duration` computes the end clock on the same civil date.
- An end clock at or before the start clock resolves on the following day, so
  overnight work (23:00-01:00) is one continuous interval. Equal clocks span
  the full 24h daily cap.
- Duration-only input keeps the established convention: the interval starts
  at local midnight on the selected date in the entry timezone.
- Clocks are exact wall-clock values: DST gaps and folds are rejected with an
  explicit error, and the offset of each actual date applies (an overnight
  entry across a DST boundary stores the true elapsed time).
- Moving the selected date on an edit reconstructs both timestamps from the
  new day, preserving the clock times and the day difference.
- A PATCH that touches no time field preserves the stored interval exactly.
  The frontend form sends time fields only when the user changed one, and
  always sends its explicit `timezone` when it does.

## Entry timezone resolution

`input.timezone` wins, then the acting user's profile timezone, then the
stored profile default (UTC). The frontend populates and interprets the form
controls in the profile timezone, so browser/profile differences cannot
silently reinterpret saved work.

## Reporting semantics (WI-1281)

Date labels, date-range inclusion, and daily totals derive from the stored
timestamps in an explicitly selected reporting timezone:

- `GET /time/worklogs`, `GET /time/projects/{id}/worklogs`, and v1
  `GET /rest/api/v1/time/worklogs` accept a `timezone` query parameter for
  their civil date filters; it defaults to the caller's profile timezone.
- Filters are interval overlaps on the timestamps (`start_time < rangeEnd`
  and `end_time > rangeStart`), so an entry crossing local midnight counts
  toward both days. An interval ending exactly at midnight contributes
  nothing to the following day.
- List ordering is `start_time DESC`.
- Desktop views (TimeEntry, Timesheet, TimeReports), exports (CSV/PDF), the
  mobile timer list, and item worklog tabs label and group by the civil date
  of the timestamps in the reporting timezone, splitting minutes at local
  midnight (`frontend/src/lib/utils/worklogTimezone.js`).
- The daily briefing query uses the same overlap semantics against the
  user-timezone day window.

## Stored `date` column inventory (kept, not dropped)

`time_worklogs.date` still stores the UTC-midnight key of the entry date in
the entry timezone, and writers still populate it (worklog create/update,
timer stop, AI `log_time`). Remaining readers after 0.8.8:

- API responses (`date` field on v1 and v2 DTOs) for external consumers.
- No internal grouping, filtering, or ordering reads the column anymore; the
  daily briefing and every report derive from the timestamps.

Dropping the column requires a migration plus an audit of external API
consumers of the `date` field; historical rows keep valid timestamps, so no
backfill is needed before that removal.
