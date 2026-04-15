/**
 * Creates a field object for form/request type field configuration.
 * @param {Object} options
 * @param {string} options.fieldData - The field data from field picker
 * @param {number} options.parentId - ID of the parent form/screen
 * @param {string} options.parentType - 'request_type' or 'screen'
 * @param {number} options.displayOrder - Position in the field list
 * @returns {Object} Field object ready to be added to fields array
 */
export function createFieldObject({ fieldData, parentId, parentType, displayOrder }) {
  let identifier = fieldData.identifier;
  if (fieldData.type === 'virtual') {
    identifier = `vf_${fieldData.fieldType}_${Date.now()}`;
  }

  const base = {
    [parentType === 'request_type' ? 'request_type_id' : 'screen_id']: parentId,
    field_identifier: identifier,
    field_type: fieldData.type === 'default' ? 'default' : fieldData.type,
    display_order: displayOrder,
    is_required: false,
    display_name: fieldData.name,
    description: null,
    step_number: 1,
    field_name: fieldData.name,
    field_label: fieldData.name,
  };

  if (fieldData.type === 'virtual') {
    base.virtual_field_type = fieldData.fieldType;
    base.virtual_field_options = null;
  }

  return base;
}

/**
 * Checks if a field already exists in the field list.
 * @param {Object[]} fields - Array of existing fields
 * @param {Object} fieldData - Field data to check
 * @param {string} identifier - Field identifier
 * @returns {boolean} True if field already exists
 */
export function fieldExists(fields, fieldData, identifier) {
  if (fieldData.type === 'virtual') return false;
  return fields.some((f) => f.field_type === fieldData.type && f.field_identifier === identifier);
}
