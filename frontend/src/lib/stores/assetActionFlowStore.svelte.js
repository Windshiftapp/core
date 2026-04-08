import { createActionFlowStore } from './createActionFlowStore.js';

export const assetActionFlowStore = createActionFlowStore({
  defaultTrigger: 'asset_created',
  nodeConfigDefaults: {
    create_item: {
      workspace_id: 0,
      item_type_id: 0,
      title: '{{asset.title}}',
      description: 'Asset: {{asset.tag}}',
    },
    set_field: { field_id: 0, value: '' },
    set_status: { status_id: 0 },
    condition: { field_name: 'title', operator: 'eq', value: '' },
    notify_user: { user_id: 0, message: 'Asset {{asset.title}} was updated' },
  },
});
