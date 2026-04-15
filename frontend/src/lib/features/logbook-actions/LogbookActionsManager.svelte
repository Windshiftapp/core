<script>
  import GenericActionListManager from '../actions/shared/GenericActionListManager.svelte';

  let {
    actions = [],
    loading = false,
    oncreate,
    onedit,
    ontoggle,
    ondelete,
    onviewlogs,
    onexecute,
  } = $props();

  const triggerLabels = {
    'document_classified': 'Document Classified',
    'content_keyword': 'Content Keyword',
    'mime_type': 'MIME Type',
    'manual': 'Manual',
  };
</script>

<GenericActionListManager
  {actions}
  {loading}
  {triggerLabels}
  headerTitle="Actions"
  headerSubtitle="Automate workflows triggered by document events"
  emptyStateDescription="Create an action to automate workflows when documents are ingested."
  {oncreate}
  {onedit}
  {ontoggle}
  {ondelete}
  {onviewlogs}
  {onexecute}
/>
