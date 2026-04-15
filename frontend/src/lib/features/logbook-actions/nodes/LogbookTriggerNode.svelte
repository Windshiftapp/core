<script>
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import GenericTriggerNode from '../../actions/shared/GenericTriggerNode.svelte';

  let { data = {}, selected = false } = $props();

  const triggerLabels = {
    'document_classified': 'Document Classified',
    'content_keyword': 'Content Keyword',
    'mime_type': 'MIME Type',
    'manual': 'Manual'
  };

  function configSummaryFn(nodeData) {
    const config = nodeData.config;
    if (!config) return '';
    if (config.content_types?.length) return config.content_types.join(', ');
    if (config.keywords?.length) return config.keywords.join(', ');
    if (config.mime_types?.length) return config.mime_types.join(', ');
    return '';
  }
</script>

<GenericTriggerNode
  {data}
  {selected}
  flowStore={logbookActionFlowStore}
  {triggerLabels}
  {configSummaryFn}
/>
