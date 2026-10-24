<script>
  import Bot from '@lucide/svelte/icons/bot';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Bot} title={t('actions.nodes.aiAgent')} accentColor="magenta" showCapabilityId={true} showToolsCount={true} capabilityLabelKey="actions.config.model" />