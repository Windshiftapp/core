/**
 * Shared SVG progress-chart helpers for IterationDetail / MilestoneDetail.
 *
 * Both detail pages render an identical donut chart (status breakdown) and
 * status-category toggle list. Constants and pure helpers live here; the
 * progress data fetch + the surrounding template stay per-page.
 */

export const PROGRESS_CHART_RADIUS = 48;
export const PROGRESS_CHART_CIRCUMFERENCE = 2 * Math.PI * PROGRESS_CHART_RADIUS;

// Used when the backend hasn't supplied a category color.
export const PROGRESS_CHART_FALLBACK_COLORS = [
  '#22c55e',
  '#3b82f6',
  '#d1d5db',
  '#f97316',
  '#ec4899',
  '#8b5cf6',
];

/**
 * Clamp `value` to [0, 100] and round. Returns 0 for non-numeric input.
 */
export function formatPercent(value) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.min(100, Math.max(0, Math.round(value)));
  }
  return 0;
}

/**
 * Turn a status_breakdown array (`[{ category_name, category_color, item_count }, ...]`)
 * into SVG arc segments laid out around the donut.
 */
export function buildProgressSegments(breakdown, totalItems) {
  if (!breakdown || !totalItems || totalItems <= 0) return [];
  let offset = 0;
  return breakdown
    .filter((segment) => segment.item_count > 0)
    .map((segment, index) => {
      const fraction = segment.item_count / totalItems;
      const arcLength = Math.max(fraction * PROGRESS_CHART_CIRCUMFERENCE, 0);
      const dasharray = `${arcLength} ${PROGRESS_CHART_CIRCUMFERENCE}`;
      const segmentData = {
        ...segment,
        dasharray,
        offset,
        color:
          segment.category_color ||
          PROGRESS_CHART_FALLBACK_COLORS[index % PROGRESS_CHART_FALLBACK_COLORS.length],
      };
      offset -= arcLength;
      return segmentData;
    });
}
