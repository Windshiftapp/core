<script>
  import { onMount } from 'svelte';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import XCircle from '@lucide/svelte/icons/x-circle';
  import Clock from '@lucide/svelte/icons/clock';
  import Eye from '@lucide/svelte/icons/eye';
  import Button from '../../../components/Button.svelte';
  import BackButton from '../../../components/BackButton.svelte';
  import DataTable from '../../../components/DataTable.svelte';
  import ExecutionTraceModal from '../ExecutionTraceModal.svelte';
  import { errorToast } from '../../../stores/toasts.svelte.js';
  import { formatDate } from '../../../utils/dateFormatter.js';
  import { actionLogElapsedMs, formatElapsedMs } from '../../../utils/timeUtils.js';
  import { getActionLogStatusColor } from '../../../utils/statusMaps.js';

  let {
    containerId,
    action,
    onBack,
    apiGetLogs,
    columns = [
      { key: 'status', label: 'Status', width: '80px' },
      { key: 'trigger_event', label: 'Trigger', width: '150px' },
      { key: 'started_at', label: 'Started', width: '180px' },
      { key: 'duration', label: 'Duration', width: '80px' },
      { key: 'error_message', label: 'Error', width: 'auto' },
      { key: 'actions', label: '', width: '60px' },
    ],
  } = $props();

  let logs = $state([]);
  let loading = $state(true);
  let selectedLog = $state(null);

  onMount(async () => {
    await loadLogs();
  });

  async function loadLogs() {
    loading = true;
    try {
      logs = await apiGetLogs(containerId, action.id);
    } catch (err) {
      errorToast('Failed to load execution logs');
      console.error(err);
    } finally {
      loading = false;
    }
  }

  function getStatusIcon(status) {
    switch (status) {
      case 'completed':
        return CheckCircle;
      case 'failed':
        return XCircle;
      case 'running':
        return Clock;
      default:
        return Clock;
    }
  }

  function formatDuration(log) {
    const ms = actionLogElapsedMs(log);
    return ms == null ? '-' : formatElapsedMs(ms, 1);
  }
</script>

<div class="action-logs">
  <div class="header">
    <BackButton onclick={onBack} label="Back" />
    <h3>Logs: {action.name}</h3>
  </div>

  <DataTable
    {columns}
    data={logs}
    {loading}
    pageSize={25}
    emptyMessage="No execution logs yet"
    rowAttrs={() => ({ 'data-testid': 'action-log-row' })}
  >
    {#snippet cell(column, row)}
      {#if column.key === 'status'}
        {@const StatusIcon = getStatusIcon(row.status)}
        <span class="status-cell" style="color: {getActionLogStatusColor(row.status)}">
          <StatusIcon size={14} />
          {row.status}
        </span>
      {:else if column.key === 'trigger_event'}
        {row.trigger_event}
      {:else if column.key === 'started_at'}
        {formatDate(row.started_at)}
      {:else if column.key === 'duration'}
        {formatDuration(row)}
      {:else if column.key === 'error_message'}
        {#if row.error_message}
          <span class="error-text" title={row.error_message}>
            {row.error_message.length > 60
              ? row.error_message.slice(0, 60) + '...'
              : row.error_message}
          </span>
        {/if}
      {:else if column.key === 'actions'}
        {#if row.execution_trace}
          <Button variant="ghost" size="small" onclick={() => (selectedLog = row)}>
            <Eye size={14} />
          </Button>
        {/if}
      {:else}
        {row[column.key] || '-'}
      {/if}
    {/snippet}
  </DataTable>
</div>

{#if selectedLog}
  <ExecutionTraceModal log={selectedLog} onclose={() => (selectedLog = null)} />
{/if}

<style>
  .action-logs {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .header {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .header h3 {
    font-size: 16px;
    font-weight: 600;
    color: var(--ds-text);
    margin: 0;
  }

  .status-cell {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    font-weight: 500;
    text-transform: capitalize;
  }

  .error-text {
    font-size: 12px;
    color: var(--ds-danger);
    font-family: monospace;
  }
</style>
