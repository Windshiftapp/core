export const BOOLEAN_CUSTOM_FIELD_TYPE = 'boolean';
export const CHECKBOX_CUSTOM_FIELD_ALIAS = 'checkbox';

export function isBooleanCustomFieldType(fieldType) {
  return fieldType === BOOLEAN_CUSTOM_FIELD_TYPE || fieldType === CHECKBOX_CUSTOM_FIELD_ALIAS;
}

export function canonicalCustomFieldType(fieldType) {
  return isBooleanCustomFieldType(fieldType) ? BOOLEAN_CUSTOM_FIELD_TYPE : fieldType;
}

// Field descriptions are short admin notes.
export const CUSTOM_FIELD_DESCRIPTION_MAX_LENGTH = 250;

// Free-text types that can show a per-screen hint as their input placeholder.
// Mirrors models.CustomFieldTypeSupportsHint.
const SCREEN_HINT_FIELD_TYPES = new Set(['text', 'textarea', 'richtext']);

export function supportsScreenHint(fieldType) {
  return SCREEN_HINT_FIELD_TYPES.has(fieldType);
}

// Hints render as placeholders, so they stay short (models.ScreenFieldHintMaxLength).
export const SCREEN_FIELD_HINT_MAX_LENGTH = 250;

// Placeholder for a free-text custom field: the screen's hint, else the
// caller's default. Other field types always keep the default.
export function customFieldPlaceholder(field, hint, fallback) {
  if (!supportsScreenHint(field?.field_type)) return fallback;
  return hint?.trim() || fallback;
}

// Linking fields attach to an existing item through item_links, so a create
// form has no item id to link against and cannot satisfy them. They stay valid
// on edit/view screens; create surfaces must exclude them.
export function isCreateRenderableCustomFieldType(fieldType) {
  return fieldType !== 'linking';
}

// Array-valued types start as arrays, booleans as false, everything else as an
// empty string. Sending "" for an untouched multiselect makes the API reject the
// item; keeping the value undefined breaks bound custom-field renderers.
export function defaultCustomFieldValue(fieldType) {
  if (isBooleanCustomFieldType(fieldType)) return false;
  if (fieldType === 'multiselect' || fieldType === 'multi_user' || fieldType === 'multi_milestone')
    return [];
  return '';
}

// Rich text and plain multi-line text both store Markdown-shaped strings, but
// only rich text is rendered and edited as Markdown.
export function isRichTextCustomFieldType(fieldType) {
  return fieldType === 'richtext';
}

// Free-text types a screen may place in the item detail main column, under the
// description. Mirrors models.CustomFieldTypeSupportsMainPlacement.
export function supportsMainColumnPlacement(fieldType) {
  return fieldType === 'text' || fieldType === 'textarea' || fieldType === 'richtext';
}

// Money values are objects ({ amount, currency }) rather than bare numbers, so
// the currency travels with the value on items, assets, and customer records.
export function isMoneyCustomFieldType(fieldType) {
  return fieldType === 'money';
}

// Format a stored money value, falling back to the given currency when the
// value omits one (pinned fields or legacy data). Returns '' for an empty value.
export function moneyFieldAmount(value) {
  if (value && typeof value === 'object') {
    const raw = value.amount;
    if (raw === null || raw === undefined || (typeof raw === 'string' && raw.trim() === '')) {
      return '';
    }
    const amount = Number(raw);
    return Number.isFinite(amount) ? amount : '';
  }
  return '';
}

// Read compatibility for historical asset/import values. Write paths still
// emit only actual booleans from the shared Checkbox component.
export function booleanCustomFieldChecked(raw) {
  if (typeof raw === 'boolean') return raw;
  if (typeof raw === 'number') return raw !== 0;
  if (typeof raw === 'string') {
    const normalized = raw.trim().toLowerCase();
    if (['false', '0', 'no', 'off'].includes(normalized)) return false;
    if (['true', '1', 'yes', 'on'].includes(normalized)) return true;
  }
  return Boolean(raw);
}
