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
    listColumn: null,
  },
  {
    identifier: 'end_date',
    name: 'End Date',
    type: 'date',
    cardSelectable: true,
    listColumn: null,
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
    identifier: 'story_points',
    name: 'Story Points',
    type: 'number',
    cardSelectable: true,
    listColumn: { required: false },
  },
];

// Derived lists for specific contexts
export const CARD_SELECTABLE_FIELDS = SYSTEM_FIELDS.filter((f) => f.cardSelectable);
export const LIST_COLUMN_FIELDS = SYSTEM_FIELDS.filter((f) => f.listColumn !== null);

// Helper to get field by identifier
export function getSystemField(identifier) {
  return SYSTEM_FIELDS.find((f) => f.identifier === identifier);
}

// Helper to get display name for a system field
export function getSystemFieldName(identifier) {
  return getSystemField(identifier)?.name || identifier;
}
