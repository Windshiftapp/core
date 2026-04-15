<script>
  import GenericActionLogsViewer from '../actions/shared/GenericActionLogsViewer.svelte';
  import { logbookActions } from '../../api/logbookActions.js';

  let { bucketId, action, onBack } = $props();
</script>

<GenericActionLogsViewer
  containerId={bucketId}
  {action}
  {onBack}
  apiGetLogs={logbookActions.getLogs}
/>
