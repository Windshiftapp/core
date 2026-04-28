import { formatDate } from './dateFormatter.js';
import { escapeHtml } from './sanitize.ts';
import { getStatusInlineStyle } from './statusColors.js';

/**
 * Build the standard work-item table column set used by Collections and
 * SearchPage. The columns are otherwise identical; the differences are:
 *
 *   - itemUrl(item):    URL for the Key/Title links. Pages compose this
 *                       differently (one uses itemUrl util, one inlines).
 *   - lastColumn:       trailing column before the row-actions cell
 *                       (Collections shows "Created", Search shows "Updated").
 *   - allStatuses,
 *     statusCategories: passed to getStatusInlineStyle for the Status cell.
 *
 * The functions called inside `render` (escapeHtml/formatDate/getStatusInlineStyle)
 * are re-imported from the helper file's location, so callers don't need to.
 */
export function buildWorkItemColumns({ itemUrl, lastColumn, allStatuses, statusCategories }) {
  return [
    {
      key: 'display_key',
      label: 'Key',
      width: 'w-28',
      html: true,
      render: (item) =>
        `<a href="${itemUrl(item)}" class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap no-underline" style="color: var(--ds-text-subtle); background-color: var(--ds-interactive-subtle);">${escapeHtml(item.display_key)}</a>`,
    },
    {
      key: 'title',
      label: 'Title',
      html: true,
      render: (item) =>
        `<a href="${itemUrl(item)}" class="block truncate text-sm no-underline" style="color: inherit;" title="${escapeHtml(item.title)}">${escapeHtml(item.title) || '—'}</a>`,
    },
    {
      key: 'workspace_name',
      label: 'Workspace',
      width: 'w-36',
      html: true,
      render: (item) =>
        `<span class="block truncate" title="${escapeHtml(item.workspace_name)}">${escapeHtml(item.workspace_name) || '—'}</span>`,
    },
    {
      key: 'status_name',
      label: 'Status',
      width: 'w-28',
      html: true,
      render: (item) =>
        item.status_name
          ? `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium whitespace-nowrap" style="${getStatusInlineStyle(item.status_name, allStatuses, statusCategories)}">${escapeHtml(item.status_name)}</span>`
          : '—',
    },
    {
      key: 'priority_name',
      label: 'Priority',
      width: 'w-24',
      html: true,
      render: (item) =>
        item.priority_name
          ? `<span class="text-sm font-medium capitalize whitespace-nowrap" style="color: ${escapeHtml(item.priority_color) || 'var(--ds-text-subtle)'}">${escapeHtml(item.priority_name)}</span>`
          : '—',
    },
    lastColumn,
    { key: 'actions', label: '', width: 'w-12' },
  ];
}

/**
 * Default "Created" trailing column. Collections uses this.
 */
export function createdAtColumn() {
  return {
    key: 'created_at',
    label: 'Created',
    width: 'w-28',
    html: true,
    render: (item) =>
      `<span class="whitespace-nowrap">${formatDate(item.created_at) || '—'}</span>`,
  };
}

/**
 * "Updated" trailing column with a translatable label. SearchPage uses this.
 */
export function updatedAtColumn(label) {
  return {
    key: 'updated_at',
    label,
    width: 'w-28',
    html: true,
    render: (item) =>
      `<span class="whitespace-nowrap">${formatDate(item.updated_at) || '—'}</span>`,
  };
}
