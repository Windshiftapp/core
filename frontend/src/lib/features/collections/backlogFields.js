export const BACKLOG_FIELD_OPTIONS = [
  { id: 'parent', labelKey: 'items.parent' },
  { id: 'milestone', labelKey: 'common.milestone' },
  { id: 'status', labelKey: 'common.status' },
  { id: 'priority', labelKey: 'common.priority' },
  { id: 'assignee', labelKey: 'common.assignee' },
  { id: 'due_date', labelKey: 'common.dueDate' },
  { id: 'story_points', labelKey: 'items.storyPoints' },
];

export const DEFAULT_BACKLOG_FIELDS = ['parent', 'status', 'story_points'];

const backlogFieldIds = new Set(BACKLOG_FIELD_OPTIONS.map((field) => field.id));

export function normalizeBacklogFields(fields) {
  if (!Array.isArray(fields)) return [...DEFAULT_BACKLOG_FIELDS];
  return [...new Set(fields.filter((field) => backlogFieldIds.has(field)))];
}
