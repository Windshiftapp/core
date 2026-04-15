<script>
  import GenericActionLogsViewer from '../actions/shared/GenericActionLogsViewer.svelte';
  import { assetActions } from '../../api/assetActions.js';

  let { assetSetId, action, onBack } = $props();

  const columns = [
    { key: 'status', label: 'Status', width: '80px' },
    { key: 'trigger_event', label: 'Trigger', width: '150px' },
    { key: 'asset_title', label: 'Asset', width: '180px' },
    { key: 'started_at', label: 'Started', width: '180px' },
    { key: 'duration', label: 'Duration', width: '80px' },
    { key: 'error_message', label: 'Error', width: 'auto' },
    { key: 'actions', label: '', width: '60px' },
  ];
</script>

<GenericActionLogsViewer
  containerId={assetSetId}
  {action}
  {onBack}
  apiGetLogs={assetActions.getLogs}
  {columns}
/>
