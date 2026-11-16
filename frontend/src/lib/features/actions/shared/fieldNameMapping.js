import { t } from '../../../stores/i18n.svelte.js';

// Mapping from FieldSelector IDs to backend column names. Shared between the
// action editor's set_field / condition / trigger-field controls and the asset
// FieldMappingsEditor so item-field sources resolve to the same backend keys.
export const fieldIdToBackendName = {
  title: 'title',
  description: 'description',
  status: 'status_id',
  priority: 'priority_id',
  assignee: 'assignee_id',
  reporter: 'creator_id',
  milestone: 'milestone_id',
  iteration: 'iteration_id',
  dueDate: 'due_date',
  startDate: 'start_date',
  storyPoints: 'story_points',
  parent: 'parent_id',
  project: 'project_id',
  itemType: 'item_type_id',
};

export const backendNameToFieldId = {
  ...Object.fromEntries(Object.entries(fieldIdToBackendName).map(([key, value]) => [value, key])),
  milestone_ids: 'milestone',
};

export const standardFieldTypes = {
  title: 'text',
  description: 'text',
  status: 'enum',
  priority: 'enum',
  assignee: 'user',
  reporter: 'user',
  milestone: 'enum',
  iteration: 'enum',
  dueDate: 'date',
  startDate: 'date',
  storyPoints: 'number',
  parent: 'reference',
  project: 'enum',
  itemType: 'enum',
};

// FieldSelector ids → the i18n keys it uses for friendly field labels, so a
// hydrated standard field shows "Assignee" rather than the bare id "assignee".
const fieldIdToI18nKey = {
  title: 'title',
  description: 'description',
  status: 'status',
  priority: 'priority',
  itemType: 'type',
  assignee: 'assignee',
  reporter: 'reporter',
  milestone: 'milestone',
  iteration: 'iteration',
  dueDate: 'dueDate',
  startDate: 'startDate',
  storyPoints: 'storyPoints',
  parent: 'parent',
  project: 'project',
  open_child_count: 'openChildCount',
  open_descendant_count: 'openDescendantCount',
};

export function friendlyStandardFieldName(fieldId, translate = t) {
  const key = Object.hasOwn(fieldIdToI18nKey, fieldId) ? fieldIdToI18nKey[fieldId] : null;
  if (!key) return fieldId;
  const tr = /** @type {any} */ (translate(`pickers.fields.${key}`));
  if (typeof tr === 'object' && tr) return tr.name || fieldId;
  return tr || fieldId;
}

// Convert a stored config (field_name / custom_field_id) into the object shape
// FieldSelector expects, hydrating raw backend keys back to friendly labels.
export function getFieldSelectorValue(config, translate = t) {
  if (config?.target === 'custom_field' && config?.custom_field_id) {
    return {
      id: `cf_${config.custom_field_id}`,
      customFieldId: config.custom_field_id,
      name: config.field_display_name || `Custom field ${config.custom_field_id}`,
      type: config.field_type || '',
      isCustom: true,
    };
  }
  const backendName = config?.field_name;
  if (!backendName) return null;
  if (backendName.startsWith('custom_field_')) {
    const customFieldId = parseInt(backendName.slice('custom_field_'.length), 10);
    return {
      id: `cf_${customFieldId}`,
      customFieldId,
      name: config.field_display_name || backendName,
      type: config.field_type || '',
      isCustom: true,
    };
  }
  if (backendName.startsWith('cf_')) {
    return { id: backendName, name: backendName.slice(3), isCustom: true };
  }
  const fieldId = Object.hasOwn(backendNameToFieldId, backendName)
    ? backendNameToFieldId[backendName]
    : Object.hasOwn(fieldIdToI18nKey, backendName)
      ? backendName
      : null;
  if (fieldId === 'milestone') {
    return { id: fieldId, name: translate('common.milestone'), type: 'enum' };
  }
  return fieldId
    ? {
        id: fieldId,
        name: friendlyStandardFieldName(fieldId, translate),
        type: standardFieldTypes[fieldId] || '',
      }
    : { id: backendName, name: backendName, type: config?.field_type || '' };
}

// Inverse of getFieldSelectorValue: resolve a FieldSelector selection back to
// the backend column / custom-field key persisted in node config.
export function backendFieldName(field) {
  if (!field) return '';
  if (field.customFieldId) return `custom_field_${field.customFieldId}`;
  return fieldIdToBackendName[field.id] || field.id;
}

// Collect every output_field name produced by nodes in the flow. Used to offer
// downstream input-field suggestions and to flag duplicate output names.
export function collectOutputFields(nodes = []) {
  const out = [];
  for (const node of nodes) {
    const name = node?.data?.config?.output_field;
    if (typeof name === 'string' && name.trim()) {
      out.push({ nodeId: node.id, name: name.trim() });
    }
  }
  return out;
}

// A backend context variable name must be a bare identifier so it can be
// referenced as {{name}} / {{name.field}} downstream.
export const OUTPUT_FIELD_PATTERN = /^[a-z_][a-z0-9_]*$/;

// Condition fields can evaluate against the trigger item's parent — the
// backend resolver honors a "parent." prefix (GH #267: "transition the parent
// when all its children are done").
export const CONDITION_SCOPE_PREFIX = 'parent.';

export function conditionFieldScope(fieldName) {
  return fieldName?.startsWith(CONDITION_SCOPE_PREFIX) ? 'parent' : 'item';
}

export function stripConditionScope(fieldName) {
  return fieldName?.startsWith(CONDITION_SCOPE_PREFIX)
    ? fieldName.slice(CONDITION_SCOPE_PREFIX.length)
    : fieldName || '';
}

// Applies the chosen scope while preserving the selected backend field key.
export function scopeConditionFieldName(fieldName, scope) {
  const bare = stripConditionScope(fieldName);
  return scope === 'parent' ? `${CONDITION_SCOPE_PREFIX}${bare}` : bare;
}

export function isValidOutputFieldName(name) {
  return typeof name === 'string' && OUTPUT_FIELD_PATTERN.test(name.trim());
}
