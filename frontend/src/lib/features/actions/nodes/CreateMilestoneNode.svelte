<script>
  import Milestone from '@lucide/svelte/icons/milestone';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Milestone} title={t('actions.nodes.createMilestone')} accentColor="green" showMilestoneInfo={true} />