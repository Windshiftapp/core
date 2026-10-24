<script>
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={RefreshCw} title={t('actions.nodes.setStatus')} accentColor="teal" showStatusInfo={true} />