<script>
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={MessageSquare} title={t('actions.nodes.addComment')} accentColor="orange" showCommentInfo={true} />