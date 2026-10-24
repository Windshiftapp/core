import { customFields, screens } from '../api/configuration.js';
import { canonicalCustomFieldType } from '../utils/customFieldTypes.js';

const defaultIndexCounts = {
  items: { current: 0, max: 20 },
  assets: { current: 0, max: 20 },
};

export function customFieldFormData(field = null) {
  return {
    field_name: field?.name || '',
    field_type: canonicalCustomFieldType(field?.field_type || 'text'),
    field_config: { max_length: '' },
    description: field?.description || '',
    required: field?.required || false,
    applies_to_portal_customers: field?.applies_to_portal_customers || false,
    applies_to_customer_organisations: field?.applies_to_customer_organisations || false,
  };
}

/** Load custom fields and every screen assignment with two bounded requests. */
export async function loadCustomFieldsOverview() {
  const [fieldsOutcome, screensOutcome] = await Promise.allSettled([
    customFields.getOverview(),
    screens.getAllWithFields(),
  ]);
  if (fieldsOutcome.status === 'rejected') {
    throw fieldsOutcome.reason;
  }
  const fieldsResult = fieldsOutcome.value;
  const screensResult = screensOutcome.status === 'fulfilled' ? screensOutcome.value : [];
  return {
    customFields: fieldsResult?.customFields ?? [],
    indexCounts: fieldsResult?.indexCounts ?? defaultIndexCounts,
    screens: Array.isArray(screensResult)
      ? screensResult.map((screen) => ({
          ...screen,
          fields: Array.isArray(screen?.fields) ? screen.fields : [],
        }))
      : [],
  };
}

/**
 * Build the options payload for saving a linking field. A mirror field is
 * configured through its primary field, so its stored options pass through
 * unchanged — renaming a mirror must never require a link type or drop the
 * mirror linkage. A primary field's stored mirror_field_id is likewise
 * carried over so editing the primary never orphans its mirror.
 */
export function linkingFieldOptions({
  editingOptions = null,
  linkTypeId = null,
  allowedItemTypeIds = [],
  allowedEntityTypes = ['item'],
  multi = true,
  mirrorName = '',
  mirrorAllowedItemTypeIds = [],
}) {
  if (editingOptions?.mirror_of_field_id) {
    return editingOptions;
  }
  const options = {
    link_type_id: parseInt(linkTypeId, 10),
    allowed_entity_types: allowedEntityTypes,
    multi,
  };
  // Preserve the mirror linkage when editing a primary field that already
  // has a mirror; the mirror field itself is managed via delete cascades.
  if (editingOptions?.mirror_field_id) {
    options.mirror_field_id = editingOptions.mirror_field_id;
  }
  if (allowedItemTypeIds.length > 0) {
    options.allowed_item_type_ids = allowedItemTypeIds.map(Number);
  }
  if (mirrorName.trim()) {
    options.mirror_name = mirrorName.trim();
    if (mirrorAllowedItemTypeIds.length > 0) {
      options.mirror_allowed_item_type_ids = mirrorAllowedItemTypeIds.map(Number);
    }
  }
  return options;
}

/**
 * Build the base update payload for the custom-field editor. The update
 * endpoint stores display_order as sent and the edit form offers no
 * reordering, so the stored order is carried through (WI-1169).
 */
export function customFieldUpdatePayload(formData, editingField = null) {
  const payload = {
    name: formData.field_name,
    field_type: formData.field_type,
    description: formData.description || '',
    required: formData.required || false,
    applies_to_portal_customers: formData.applies_to_portal_customers || false,
    applies_to_customer_organisations: formData.applies_to_customer_organisations || false,
  };
  if (editingField) {
    payload.display_order = editingField.display_order || 0;
  }
  return payload;
}
