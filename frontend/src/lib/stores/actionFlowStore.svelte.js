import { createActionFlowStore } from './createActionFlowStore.svelte.js';

export const actionFlowStore = createActionFlowStore({
  defaultTrigger: 'status_transition',
  nodeConfigDefaults: {
    set_field: { field_name: '', value: '' },
    set_status: { status_id: null },
    add_comment: { content: '', is_private: false },
    notify_user: {
      recipient_type: 'assignee',
      recipients: ['assignee'],
      message: '',
      include_link: true,
    },
    condition: { field_name: '', operator: 'eq', value: '' },
    related_items: { relation: 'descendants', cross_workspace: false },
    transition_item: { target: { mode: 'matching_terminal' }, skip_if_already_matching: true },
    round_robin_assign: { team_id: 0, skip_on_leave_members: true, use_leave_substitutes: true },
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
    create_milestone: {
      upsert_key_template: '{{ref.short}}',
      name_template: 'Release {{ref.short}}',
      status_on_branch: 'planning',
      status_on_tag: 'in-progress',
      attach_release_on_tag: true,
      attach_commit_issues: true,
    },
    notify_customer: { subject: '', message: '' },
    insert_canned_response: { canned_response_id: 0 },
    adjust_labels: { add_label_ids: [], remove_label_ids: [] },
  },
});
