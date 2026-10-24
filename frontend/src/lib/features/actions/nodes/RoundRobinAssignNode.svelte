<script>
  import UsersRound from '@lucide/svelte/icons/users-round';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={UsersRound} title={t('actions.nodes.roundRobinAssign')} accentColor="magenta" showAssignmentInfo={true} />