import { createActionFlowStore } from './createActionFlowStore.svelte.js';

export const actionFlowStore = createActionFlowStore({
  defaultTrigger: 'status_transition',
  includeStatuses: true,
  nodeConfigDefaults: {
    set_field: { field_name: '', value: '' },
    set_status: { status_id: null },
    add_comment: { content: '', is_private: false },
    notify_user: { recipient_type: 'assignee', recipients: [], message: '', include_link: true },
    condition: { field_name: '', operator: 'eq', value: '' },
    update_asset: { source_field_id: '', asset_set_id: 0, asset_type_id: 0, field_mappings: [] },
    create_asset: {
      asset_set_id: 0,
      asset_type_id: 0,
      title: '',
      description: '',
      asset_tag: '',
      category_id: null,
      status_id: null,
      field_mappings: [],
    },
  },
});
