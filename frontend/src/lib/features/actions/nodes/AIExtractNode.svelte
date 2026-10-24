<script>
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Sparkles} title={t('actions.nodes.aiExtract')} accentColor="purple" showInputOutput={true} />