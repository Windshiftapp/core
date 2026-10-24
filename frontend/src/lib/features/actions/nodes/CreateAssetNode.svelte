<script>
  import PlusSquare from '@lucide/svelte/icons/plus-square';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={PlusSquare} title={t('actions.nodes.createAsset')} accentColor="green" showAssetInfo={true} />