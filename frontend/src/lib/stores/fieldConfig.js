// System field definitions - single source of truth
export const SYSTEM_FIELDS = [
  { identifier: 'title', name: 'Title', type: 'text' },
  { identifier: 'description', name: 'Description', type: 'textarea' },
  { identifier: 'status', name: 'Status', type: 'select' },
  { identifier: 'priority', name: 'Priority', type: 'select' },
  { identifier: 'assignee', name: 'Assignee', type: 'select' },
  { identifier: 'milestone', name: 'Milestone', type: 'select' },
  { identifier: 'iteration', name: 'Iteration', type: 'select' },
  { identifier: 'due_date', name: 'Due Date', type: 'date' },
  { identifier: 'project', name: 'Project', type: 'select' },
];

// Helper to get field by identifier
export function getSystemField(identifier) {
  return SYSTEM_FIELDS.find((f) => f.identifier === identifier);
}

// Helper to get display name for a system field
export function getSystemFieldName(identifier) {
  return getSystemField(identifier)?.name || identifier;
}
