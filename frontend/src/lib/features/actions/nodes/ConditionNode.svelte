<script>
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericConditionNode from '../shared/GenericConditionNode.svelte';

  let { data = {}, selected = false } = $props();

  const operatorSymbols = {
    'equals': '=',
    'not_equals': '≠',
    'contains': '∋',
    'not_contains': '∌',
    'greater_than': '>',
    'less_than': '<',
    'is_empty': '∅',
    'is_not_empty': '≠∅'
  };
</script>

<GenericConditionNode
  {data}
  {selected}
  flowStore={actionFlowStore}
  {operatorSymbols}
  title={t('actions.nodes.condition')}
  placeholderText={t('actions.config.setCondition')}
  trueBranchLabel={t('actions.condition.true')}
  falseBranchLabel={t('actions.condition.false')}
/>
