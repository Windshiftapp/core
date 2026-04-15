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

  function configSummaryFn(nodeData) {
    const config = nodeData.config;
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
