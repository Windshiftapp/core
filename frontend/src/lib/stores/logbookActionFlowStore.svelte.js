import { createActionFlowStore } from './createActionFlowStore.js';

export const logbookActionFlowStore = createActionFlowStore({
  defaultTrigger: 'document_classified',
  nodeConfigDefaults: {
    create_item: {
      workspace_id: 0,
      item_type_id: 0,
      title: '{{doc.title}}',
      description: 'Source: {{doc.link}}',
    },
    create_asset: {
      asset_set_id: 0,
      asset_type_id: 0,
      title: '{{doc.title}}',
      description: '',
      asset_tag: '',
      category_id: null,
      status_id: null,
      field_mappings: [],
    },
    associate_customer: { customer_organisation_id: null, portal_customer_id: null },
    condition: { field_name: 'content_type', operator: 'eq', value: '' },
  },
});
