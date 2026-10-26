export function formatUtcTime(value) {
  if (!value) return '—';
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date
    .toISOString()
    .replace('T', ' ')
    .replace(/\..*Z$/, ' UTC');
}

export function formatDurationMs(ms) {
  if (ms == null) return '—';
  if (ms < 1000) return `${ms} ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(sec < 10 ? 2 : 1)}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${Math.round(sec - min * 60)}s`;
}

export function formatLatencyMs(ms) {
  if (ms == null) return '—';
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatValue(value) {
  if (value === null || value === undefined || value === '') return '—';
  return String(value);
}

export function formatRelativeTime(value, emptyLabel = 'Never refreshed') {
  if (!value) return emptyLabel;
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return emptyLabel;
  const diffMs = Date.now() - date.getTime();
  if (diffMs < 0) return 'just now';
  const mins = Math.round(diffMs / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

export function formatAgeSeconds(seconds) {
  if (!seconds) return '—';
  if (seconds < 90) return `${seconds}s`;
  const mins = Math.round(seconds / 60);
  if (mins < 90) return `${mins}m`;
  return `${Math.round(mins / 60)}h`;
}

export function truncateText(value, maxLength) {
  if (!value) return '';
  return value.length > maxLength ? `${value.slice(0, maxLength - 1)}…` : value;
}

export function successSummary(row) {
  const successes = row.successes ?? 0;
  const total = row.total ?? 0;
  const rate = total > 0 ? Math.round((successes / total) * 100) : null;
  return `${successes}${rate != null ? ` (${rate}%)` : ''}`;
}

/**
 * Aggregate the per-row diagnostics stats into the headline tile totals.
 * `avgField` selects the row field to weight-average (`avg_duration_ms` for
 * scheduler runs, `avg_latency_ms` for webhook deliveries).
 *
 * @param {Array<any>} stats
 * @param {string} avgField
 */
export function diagnosticsTotals(stats, avgField) {
  let total = 0;
  let success = 0;
  let failed = 0;
  let weighted = 0;
  let count = 0;
  for (const s of stats) {
    total += s.total ?? 0;
    success += s.successes ?? 0;
    failed += s.failures ?? 0;
    if (s[avgField] != null && s.total) {
      weighted += s[avgField] * s.total;
      count += s.total;
    }
  }
  return {
    total,
    success,
    failed,
    successRate: total > 0 ? Math.round((success / total) * 100) : null,
    avg: count > 0 ? Math.round(weighted / count) : null,
  };
}

export async function runDiagnosticsPurge({
  olderThan,
  confirmMessage,
  execute,
  successMessage,
  reload,
  setPurging,
  errorToast,
  successToast,
}) {
  if (!olderThan || !/^\d+[dhm]$/.test(olderThan)) {
    errorToast('Use a duration like 30d, 168h, or 60m');
    return;
  }
  if (!confirm(confirmMessage)) {
    return;
  }
  setPurging(true);
  try {
    const result = await execute();
    successToast(successMessage(result));
    await reload();
  } catch (err) {
    errorToast(err?.message ?? 'Purge failed');
  } finally {
    setPurging(false);
  }
}
