import { Calendar } from 'lucide-svelte';

const STATUS_COLORS = {
  active: '#0052CC',
  completed: '#00875A',
  cancelled: '#6B778C',
  planned: '#5243AA',
};

const FALLBACK_STATUS_COLOR = '#6B778C';

export function getIterationStatusColor(status) {
  return STATUS_COLORS[status] || FALLBACK_STATUS_COLOR;
}

function capitalize(str) {
  return str ? str.charAt(0).toUpperCase() + str.slice(1) : '';
}

function defaultStatusBadgeColors({ hex }) {
  return { bgColor: `${hex}15`, textColor: hex };
}

/**
 * Build the iteration/sprint picker config shared across CollectionBoard,
 * ItemDetailSidebar, and IterationDependencies.
 *
 * Variations:
 *   - icon:               omit for "no icon" call sites; pass a descriptor like
 *                         `{ type: 'component', source: (item) => Globe }` otherwise.
 *   - searchFields:       fields the picker searches in. Default ['name'].
 *   - calendarIcon:       icon component used for the start/end date row.
 *   - statusBadgeColors:  ({ status, hex }) => { bgColor, textColor }. Default
 *                         is `${hex}15` background with `hex` text. Override to
 *                         use a different blending scheme (e.g. rgba with an
 *                         accessible-contrast pass).
 */
export function buildIterationPickerConfig({
  icon,
  searchFields = ['name'],
  calendarIcon = Calendar,
  statusBadgeColors = defaultStatusBadgeColors,
} = {}) {
  return {
    ...(icon ? { icon } : {}),
    primary: { text: (item) => item.name },
    badges: [
      {
        text: (item) => (item.is_global ? 'Global' : 'Workspace'),
        bgColor: () => 'var(--ds-background-neutral)',
        textColor: () => 'var(--ds-text-subtle)',
      },
    ],
    metadata: [
      {
        type: 'date-range',
        icon: calendarIcon,
        startDate: (item) => item.start_date,
        endDate: (item) => item.end_date,
      },
      {
        type: 'badge',
        text: (item) => (item.status ? capitalize(item.status) : ''),
        bgColor: (item) => {
          if (!item.status) return 'transparent';
          return statusBadgeColors({
            status: item.status,
            hex: getIterationStatusColor(item.status),
          }).bgColor;
        },
        textColor: (item) => {
          if (!item.status) return 'var(--ds-text)';
          return statusBadgeColors({
            status: item.status,
            hex: getIterationStatusColor(item.status),
          }).textColor;
        },
      },
    ],
    searchFields,
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
  };
}
