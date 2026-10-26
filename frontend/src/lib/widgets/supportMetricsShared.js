/**
 * Shared derivations for the support metrics widgets (SupportMetricsWidget,
 * SupportOverviewWidget). Pure helpers so the two widgets stay in lockstep on
 * the window, activity, SLA, and chart contracts.
 */

/**
 * Metrics window in days from a widget config (`28d` default).
 * @param {{ window?: string } | null | undefined} config
 */
export function supportWindowDays(config) {
  if (config?.window === '7d') return 7;
  if (config?.window === '84d') return 84;
  return 28;
}

/**
 * ISO `{ from, to }` range covering the last `windowDays` days, inclusive.
 * @param {number} windowDays
 */
export function supportDateRange(windowDays) {
  const to = new Date();
  const from = new Date(to);
  from.setDate(from.getDate() - (windowDays - 1));
  return { from: toISOStringDate(from), to: toISOStringDate(to) };
}

/** ISO date (YYYY-MM-DD). */
export function toISOStringDate(date) {
  return date.toISOString().slice(0, 10);
}

/**
 * True when the metrics document reports any created/resolved/backlog activity.
 * @param {any} doc
 */
export function supportHasActivity(doc) {
  return !!doc && (doc.summary.created > 0 || doc.summary.resolved > 0 || doc.summary.backlog > 0);
}

/**
 * SLA compliance label: rounded percentage, or the "no data" label.
 * @param {any} summary
 * @param {(key: string) => string} t
 */
export function supportSlaLabel(summary, t) {
  return summary?.sla && summary.sla.completed > 0
    ? `${Math.round(summary.sla.compliant_pct)}%`
    : t('dashboard.states.supportSlaNone');
}

/**
 * Chart series/categories for the created-vs-resolved bar chart.
 * @param {any} doc
 * @param {(key: string) => string} t
 */
export function supportChartData(doc, t) {
  if (!doc) return { categories: [], series: [] };
  const buckets = doc.buckets ?? [];
  return {
    categories: buckets.map((bucket) => bucket.bucket_start),
    series: [
      {
        key: 'created',
        label: t('dashboard.states.supportCreated'),
        color: '#6366f1',
        values: buckets.map((b) => b.created),
      },
      {
        key: 'resolved',
        label: t('dashboard.states.supportResolved'),
        color: '#22c55e',
        values: buckets.map((b) => b.resolved),
      },
    ],
  };
}
