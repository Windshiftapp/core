<script>
  import Box from '@lucide/svelte/icons/box';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Box} title={t('actions.nodes.containerRun')} accentColor="blue" showCapabilityId={true} placeholderKey="actions.config.selectCapability" />