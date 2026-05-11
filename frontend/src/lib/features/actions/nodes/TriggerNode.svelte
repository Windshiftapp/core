<script>
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericTriggerNode from '../shared/GenericTriggerNode.svelte';

  let { data = {}, selected = false } = $props();

  const triggerLabels = {
    'status_transition': t('actions.trigger.statusTransition'),
    'item_created': t('actions.trigger.itemCreated'),
    'item_updated': t('actions.trigger.itemUpdated'),
    'item_linked': t('actions.trigger.itemLinked')
  };

  // Reverse mapping from backend name to display label
  const backendNameToLabel = {
    title: 'title', description: 'description', status_id: 'status',
    priority_id: 'priority', assignee_id: 'assignee', creator_id: 'reporter',
    milestone_id: 'milestone', iteration_id: 'iteration', due_date: 'dueDate',
    start_date: 'startDate', story_points: 'storyPoints', parent_id: 'parent',
    project_id: 'project', item_type_id: 'itemType'
  };

  function getFieldLabel(fieldName) {
    if (!fieldName) return '';
    if (fieldName.startsWith('cf_')) return fieldName.slice(3);
    const fieldKey = backendNameToLabel[fieldName];
    if (fieldKey) {
      const translated = /** @type {any} */ (t(`pickers.fields.${fieldKey}`));
      if (typeof translated === 'object' && translated !== null) return translated.name || fieldKey;
      return translated || fieldKey;
    }
    return fieldName;
  }

  function configSummaryFn(nodeData) {
    const config = nodeData.config;
    const triggerType = nodeData.triggerType;

    // Handle item_updated with field_name
    if (triggerType === 'item_updated' && config?.field_name) {
      return `${t('actions.config.triggerField')}: ${getFieldLabel(config.field_name)}`;
    }

    // Handle status_transition with from/to status
    if (!config?.from_status_id && !config?.to_status_id) return '';
    const parts = [];
    if (config.from_status_id) {
      const status = nodeData.statuses?.find(s => s.id === config.from_status_id);
      parts.push(`${t('actions.config.from')}: ${status?.name || config.from_status_id}`);
    }
    if (config.to_status_id) {
      const status = nodeData.statuses?.find(s => s.id === config.to_status_id);
      parts.push(`${t('actions.config.to')}: ${status?.name || config.to_status_id}`);
    }
    return parts.join(' ');
  }
</script>

<GenericTriggerNode
  {data}
  {selected}
  flowStore={actionFlowStore}
  {triggerLabels}
  title={t('actions.nodes.trigger')}
  {configSummaryFn}
/>
