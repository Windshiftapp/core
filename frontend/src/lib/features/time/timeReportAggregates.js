// Pure reducers over the server-side worklog aggregate document
// (GET /api/v2/time/worklogs/aggregate, WI-1449). Reports derive every
// statistic — summary, member breakdown, daily chart — from the aggregate
// instead of fetching all raw rows. Minutes in `daily` rows are split across
// civil days by the server; `totals` carry booked duration and entry counts.

/**
 * @param {Array<{user_id: number, user_name: string, project_id: number, project_name: string, customer_id: number, customer_name: string, duration_minutes: number, entries: number}>} totals
 * @param {{ dateFrom?: string, dateTo?: string }} [range]
 */
export function buildSummary(totals, range = {}) {
  const dateFrom = range?.dateFrom;
  const dateTo = range?.dateTo;
  const safeTotals = Array.isArray(totals) ? totals : [];
  const totalMinutes = safeTotals.reduce((sum, t) => sum + t.duration_minutes, 0);
  const totalEntries = safeTotals.reduce((sum, t) => sum + t.entries, 0);
  const summary = {
    totalHours: Math.round((totalMinutes / 60) * 100) / 100,
    totalEntries,
    averageHoursPerDay: 0,
    topProject: null,
    topCustomer: null,
  };

  if (totalEntries === 0) return summary;

  if (dateFrom && dateTo) {
    const daysDiff =
      Math.ceil(
        (new Date(dateTo).getTime() - new Date(dateFrom).getTime()) / (1000 * 60 * 60 * 24)
      ) + 1;
    summary.averageHoursPerDay = Math.round((summary.totalHours / daysDiff) * 100) / 100;
  }
  const projectMinutes = new Map();
  const customerMinutes = new Map();
  for (const t of safeTotals) {
    projectMinutes.set(
      t.project_name,
      (projectMinutes.get(t.project_name) || 0) + t.duration_minutes
    );
    customerMinutes.set(
      t.customer_name,
      (customerMinutes.get(t.customer_name) || 0) + t.duration_minutes
    );
  }
  summary.topProject = topOf(projectMinutes);
  summary.topCustomer = topOf(customerMinutes);
  return summary;
}

function topOf(minutesByName) {
  let bestName = null;
  let bestMinutes = -1;
  for (const [name, minutes] of minutesByName) {
    if (minutes > bestMinutes) {
      bestName = name;
      bestMinutes = minutes;
    }
  }
  if (bestName === null) return null;
  return { name: bestName, hours: Math.round((bestMinutes / 60) * 100) / 100 };
}

/**
 * Per-member hours/entries/avgPerDay for the project report.
 * @param {Array<{day: string, user_id: number, user_name: string, minutes: number}>} daily
 * @param {Array<{user_id: number, user_name: string, project_id: number, project_name: string, customer_id: number, customer_name: string, duration_minutes: number, entries: number}>} totals
 */
export function buildMemberBreakdown(daily, totals) {
  // daily rows: {day, user_id, user_name, minutes}; totals rows:
  // {user_id, user_name, duration_minutes, entries} plus id/name columns.
  const safeDaily = Array.isArray(daily) ? daily : [];
  const safeTotals = Array.isArray(totals) ? totals : [];

  const names = new Map();
  const activeDays = new Map();
  for (const row of safeDaily) {
    names.set(row.user_id, row.user_name);
    if (!activeDays.has(row.user_id)) activeDays.set(row.user_id, new Set());
    activeDays.get(row.user_id).add(row.day);
  }

  const members = new Map();
  for (const t of safeTotals) {
    const existing = members.get(t.user_id);
    if (existing) {
      existing.durationMinutes += t.duration_minutes;
      existing.entries += t.entries;
    } else {
      members.set(t.user_id, {
        user_name: names.get(t.user_id) || t.user_name || 'Unknown',
        durationMinutes: t.duration_minutes,
        entries: t.entries,
      });
    }
  }
  for (const [userId, member] of members) {
    const days = activeDays.get(userId);
    member.hours = Math.round((member.durationMinutes / 60) * 100) / 100;
    member.avgPerDay =
      days && days.size > 0 ? Math.round((member.durationMinutes / 60 / days.size) * 100) / 100 : 0;
    delete member.durationMinutes;
  }
  return [...members.values()].sort((a, b) => b.hours - a.hours);
}

/**
 * Daily chart series from the day-split groups.
 * @param {Array<{day: string, minutes: number}>} daily
 */
export function buildDailyChartData(daily) {
  const safeDaily = Array.isArray(daily) ? daily : [];
  const byDay = new Map();
  for (const row of safeDaily) {
    byDay.set(row.day, (byDay.get(row.day) || 0) + row.minutes);
  }
  return [...byDay.keys()].sort().map((date) => ({
    date: new Date(date),
    count: Math.round(((byDay.get(date) || 0) / 60) * 100) / 100,
    label: date,
  }));
}
