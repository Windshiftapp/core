<script>
  import Database from '@lucide/svelte/icons/database';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Database} title={t('actions.nodes.updateAsset')} accentColor="teal" showFieldInfo={true} />