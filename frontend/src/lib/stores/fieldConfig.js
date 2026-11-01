// System field definitions - single source of truth
export const SYSTEM_FIELDS = [
  {
    identifier: 'key',
    name: 'Key',
    type: 'text',
    cardSelectable: false,
    listColumn: { required: true },
  },
  {
    identifier: 'title',
    name: 'Title',
    type: 'text',
    cardSelectable: false,
    listColumn: { required: true },
  },
  {
    identifier: 'description',
    name: 'Description',
    type: 'textarea',
    cardSelectable: false,
    listColumn: null,
  },
  {
    identifier: 'status',
    name: 'Status',
    type: 'select',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'priority',
    name: 'Priority',
    type: 'select',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'assignee',
    name: 'Assignee',
    type: 'select',
    cardSelectable: false,
    listColumn: { required: false },
  },
  // Manually-added system field: available in the screen editor but not a
  // default board card or list column.
  {
    identifier: 'team',
    name: 'Team',
    type: 'select',
    cardSelectable: false,
    listColumn: null,
  },
  {
    identifier: 'milestone',
    name: 'Milestone',
    type: 'select',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'iteration',
    name: 'Iteration',
    type: 'select',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'due_date',
    name: 'Due Date',
    type: 'date',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'start_date',
    name: 'Start Date',
    type: 'date',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'end_date',
    name: 'End Date',
    type: 'date',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'labels',
    name: 'Labels',
    type: 'multi-select',
    cardSelectable: true,
    listColumn: null,
  },
  {
    identifier: 'created_at',
    name: 'Created',
    type: 'date',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'project',
    name: 'Project',
    type: 'select',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'parent',
    name: 'Parent',
    type: 'text',
    cardSelectable: true,
    listColumn: null,
  },
  {
    identifier: 'time_in_status',
    name: 'Time in Status',
    type: 'text',
    cardSelectable: true,
    listColumn: null,
  },
  {
    identifier: 'story_points',
    name: 'Story Points',
    type: 'number',
    cardSelectable: true,
    listColumn: { required: false },
  },
  {
    identifier: 'estimate',
    name: 'Estimate',
    type: 'duration',
    cardSelectable: true,
    listColumn: { required: false },
  },
];

// Board-only card fields. They can be added to a board card but never appear in
// an item create/edit screen, so they stay out of SYSTEM_FIELDS. The SLA badge
// is only mounted (and its read only issued) when this field is configured.
export const BOARD_ONLY_CARD_FIELDS = [
  {
    identifier: 'sla',
    name: 'SLA',
    type: 'text',
    cardSelectable: true,
    listColumn: null,
  },
];

// Read-only fields that can appear as list columns but are not part of the
// item create/edit screen model. Keeping them out of SYSTEM_FIELDS avoids
// exposing them to the screen editor.
export const LIST_ONLY_FIELDS = [
  {
    identifier: 'updated_at',
    name: 'Updated',
    type: 'date',
    cardSelectable: false,
    listColumn: { required: false },
  },
];

// Derived lists for specific contexts
export const CARD_SELECTABLE_FIELDS = [
  ...SYSTEM_FIELDS.filter((f) => f.cardSelectable),
  ...BOARD_ONLY_CARD_FIELDS,
];
export const LIST_COLUMN_FIELDS = [
  ...SYSTEM_FIELDS.filter((f) => f.listColumn !== null),
  ...LIST_ONLY_FIELDS,
];

// Helper to get field by identifier
function getSystemField(identifier) {
  return (
    SYSTEM_FIELDS.find((f) => f.identifier === identifier) ||
    LIST_ONLY_FIELDS.find((f) => f.identifier === identifier)
  );
}

// Helper to get display name for a system field
export function getSystemFieldName(identifier) {
  return getSystemField(identifier)?.name || identifier;
}
