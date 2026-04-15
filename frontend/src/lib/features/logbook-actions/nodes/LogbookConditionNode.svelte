<script>
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import GenericConditionNode from '../../actions/shared/GenericConditionNode.svelte';

  let { data = {}, selected = false } = $props();

  const operatorSymbols = {
    'eq': '=', 'ne': '\u2260', 'contains': '\u220B', 'not_contains': '\u220C',
    'starts_with': '\u2282', 'ends_with': '\u2283', 'matches': '~'
  };
</script>

<GenericConditionNode
  {data}
  {selected}
  flowStore={logbookActionFlowStore}
  {operatorSymbols}
/>
