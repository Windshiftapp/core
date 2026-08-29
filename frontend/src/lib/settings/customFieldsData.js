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
export async function loadCustomFieldsOverview(apiClient) {
  const [fieldsOutcome, screensOutcome] = await Promise.allSettled([
    apiClient.customFields.getAll(),
    apiClient.screens.getAllWithFields(),
  ]);
  if (fieldsOutcome.status === 'rejected') {
    throw fieldsOutcome.reason;
  }
  const fieldsResult = fieldsOutcome.value;
  const screensResult = screensOutcome.status === 'fulfilled' ? screensOutcome.value : [];
  return {
    customFields: Array.isArray(fieldsResult?.data)
      ? fieldsResult.data
      : Array.isArray(fieldsResult)
        ? fieldsResult
        : [],
    indexCounts: fieldsResult?.index_counts ?? defaultIndexCounts,
    screens: Array.isArray(screensResult)
      ? screensResult.map((screen) => ({
          ...screen,
          fields: Array.isArray(screen?.fields) ? screen.fields : [],
        }))
      : [],
  };
}
