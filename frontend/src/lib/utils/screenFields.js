import { resolveScreenId } from './screenResolution.js';

// System fields that are shown in the desktop create form and can be submitted
// through the existing work-item create API. Fields omitted here are either
// server-managed (key, created_at, time_in_status), relationship-context fields
// (parent), or currently managed outside the create payload (status).
export const CREATE_RENDERABLE_SYSTEM_FIELDS = new Set([
  'title',
  'description',
  'priority',
  'assignee',
  'team',
  'milestone',
  'iteration',
  'due_date',
  'start_date',
  'end_date',
  'project',
  'labels',
  'story_points',
  'estimate',
  'estimate_minutes',
]);

export const CREATE_AUTO_MANAGED_SYSTEM_FIELDS = new Set([
  'key',
  'status',
  'created_at',
  'parent',
  'time_in_status',
]);

// These are rendered in fixed item UI locations regardless of screen layout;
// keep them present and locked in screen configuration.
export const ALWAYS_VISIBLE_SYSTEM_FIELDS = ['title', 'description', 'status'];

export const SYSTEM_FIELD_ALIASES = {
  estimate: ['estimate', 'estimate_minutes'],
  estimate_minutes: ['estimate', 'estimate_minutes'],
};

export function systemFieldIdentifiers(fieldName) {
  return SYSTEM_FIELD_ALIASES[fieldName] || [fieldName];
}

export function normalizeSystemFieldIdentifier(fieldName) {
  if (fieldName === 'estimate_minutes') return 'estimate';
  return fieldName;
}

export function isSystemFieldConfigured(configuredFields, fieldName) {
  if (!Array.isArray(configuredFields)) return false;
  return systemFieldIdentifiers(fieldName).some((identifier) =>
    configuredFields.includes(identifier)
  );
}

export function isCreateSystemFieldRenderable(fieldName) {
  return systemFieldIdentifiers(fieldName).some((identifier) =>
    CREATE_RENDERABLE_SYSTEM_FIELDS.has(identifier)
  );
}

export function isCreateSystemFieldAutoManaged(fieldName) {
  return systemFieldIdentifiers(fieldName).some((identifier) =>
    CREATE_AUTO_MANAGED_SYSTEM_FIELDS.has(identifier)
  );
}

export function isAlwaysVisibleSystemField(fieldName) {
  return systemFieldIdentifiers(fieldName).some((identifier) =>
    ALWAYS_VISIBLE_SYSTEM_FIELDS.includes(identifier)
  );
}

export function canSystemFieldBeRequiredOnCreate(fieldName) {
  return isCreateSystemFieldRenderable(fieldName) && !isCreateSystemFieldAutoManaged(fieldName);
}

export function splitScreenFields(fields = []) {
  const customFields = [];
  const systemFields = [];

  for (const field of fields || []) {
    if (field?.field_type === 'custom') {
      customFields.push(field);
    } else if (field?.field_type === 'system') {
      systemFields.push(field);
    }
  }

  return {
    customFields,
    systemFields,
    systemFieldIdentifiers: systemFields.map((field) => field.field_identifier),
  };
}

export function dedupeScreenFields(fields = [], keyForField = defaultScreenFieldKey) {
  const seen = new Set();
  const out = [];
  for (const field of fields || []) {
    const key = keyForField(field);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push(field);
  }
  return out;
}

function defaultScreenFieldKey(field) {
  if (!field) return '';
  if (field.field_type === 'system') {
    return `system:${normalizeSystemFieldIdentifier(field.field_identifier)}`;
  }
  return `${field.field_type}:${field.field_identifier}`;
}

export function customFieldDefinitionIdFromScreenField(field) {
  const id = parseInt(field?.field_identifier, 10);
  return Number.isFinite(id) ? id : null;
}

export function withAlwaysVisibleSystemFields(systemFields = []) {
  const out = [...ALWAYS_VISIBLE_SYSTEM_FIELDS];
  for (const field of systemFields || []) {
    const normalized = normalizeSystemFieldIdentifier(field);
    if (!out.includes(normalized)) out.push(field);
  }
  return out;
}

export function systemFieldsFromScreen(screen) {
  const fields = screen?.fields || [];
  const identifiers = fields
    .filter((field) => field.field_type === 'system')
    .map((field) => field.field_identifier);
  return withAlwaysVisibleSystemFields(
    identifiers.length > 0 ? identifiers : screen?.system_fields || []
  );
}

export function customFieldsFromScreen(screen) {
  return (screen?.fields || []).filter((field) => field.field_type === 'custom');
}

/**
 * Checks whether an item can use a system field on its effective edit screen.
 * A missing workspace mapping means the screen configuration is unresolved.
 */
export function isSystemFieldAvailableForItem(
  item,
  fieldName,
  configSetsByWorkspaceId,
  screensById,
  fallbackScreenId = 1
) {
  const workspaceId = item?.workspace_id;
  const workspaceKey = workspaceId == null ? null : String(workspaceId);
  if (!workspaceKey || !configSetsByWorkspaceId?.has(workspaceKey)) return false;

  const configSet = configSetsByWorkspaceId.get(workspaceKey);
  if (configSet === undefined) return false;
  const screenId = resolveEffectiveScreenIds(configSet, item?.item_type_id, fallbackScreenId).edit;
  const screen = screensById?.get(screenId) || screensById?.get(String(screenId));
  return isSystemFieldConfigured(systemFieldsFromScreen(screen), fieldName);
}

/** Resolve inline-detail visibility as edit ∪ view; view-only fields remain
 * read-only, while edit fields use effective identifiers. */
export function buildDetailScreenFieldConfig(editScreen, viewScreen = null) {
  const sameScreen = !viewScreen || viewScreen.id === editScreen?.id;
  const editCustom = customFieldsFromScreen(editScreen);
  const editSystem = systemFieldsFromScreen(editScreen);

  if (sameScreen) {
    return {
      visibleCustomFields: editCustom,
      visibleSystemFields: editSystem,
      editableCustomFieldIds: null,
      editableSystemFields: null,
    };
  }

  const viewCustom = customFieldsFromScreen(viewScreen);
  const viewSystem = systemFieldsFromScreen(viewScreen);

  return {
    visibleCustomFields: dedupeScreenFields([...editCustom, ...viewCustom]),
    visibleSystemFields: Array.from(new Set([...editSystem, ...viewSystem])),
    editableCustomFieldIds: new Set(
      editCustom.map(customFieldDefinitionIdFromScreenField).filter((id) => id !== null)
    ),
    editableSystemFields: new Set(withAlwaysVisibleSystemFields(editSystem)),
  };
}

export function resolveEffectiveScreenIds(configSet, itemTypeId, fallbackScreenId = 1) {
  return {
    create: resolveScreenId(configSet, itemTypeId, 'create') ?? fallbackScreenId,
    edit: resolveScreenId(configSet, itemTypeId, 'edit') ?? fallbackScreenId,
    view: resolveScreenId(configSet, itemTypeId, 'view') ?? fallbackScreenId,
  };
}
