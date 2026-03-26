<script>
  import { onMount } from 'svelte';
  import { CheckCircle, XCircle, Clock, ArrowLeft, Eye } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import ExecutionTraceModal from '../actions/ExecutionTraceModal.svelte';
  import { logbookActions } from '../../api/logbookActions.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { formatDate } from '../../utils/dateFormatter.js';

  let { bucketId, action, onBack } = $props();

  let logs = $state([]);
  let loading = $state(true);
  let selectedLog = $state(null);

  onMount(async () => {
    await loadLogs();
  });

  async function loadLogs() {
    loading = true;
    try {
      logs = await logbookActions.getLogs(bucketId, action.id);
    } catch (err) {
      errorToast('Failed to load execution logs');
      console.error(err);
    } finally {
      loading = false;
    }
  }

  function getStatusIcon(status) {
    switch (status) {
      case 'completed': return CheckCircle;
      case 'failed': return XCircle;
      case 'running': return Clock;
      default: return Clock;
    }
  }

  function getStatusColor(status) {
    switch (status) {
      case 'completed': return 'var(--ds-success)';
      case 'failed': return 'var(--ds-danger)';
      case 'running': return 'var(--ds-accent-blue)';
      default: return 'var(--ds-text-subtlest)';
    }
  }

  function formatDuration(log) {
    if (!log.started_at || !log.completed_at) return '-';
    const start = new Date(log.started_at);
    const end = new Date(log.completed_at);
    const ms = end - start;
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }

  const columns = [
    { key: 'status', label: 'Status', width: '80px' },
    { key: 'trigger_event', label: 'Trigger', width: '150px' },
    { key: 'started_at', label: 'Started', width: '180px' },
    { key: 'duration', label: 'Duration', width: '80px' },
    { key: 'error_message', label: 'Error', width: 'auto' },
    { key: 'actions', label: '', width: '60px' }
  ];
</script>

<div class="action-logs">
  <div class="header">
    <Button variant="ghost" size="small" onclick={onBack}>
      <ArrowLeft size={14} />
      Back
    </Button>
    <h3>Logs: {action.name}</h3>
  </div>

  <DataTable {columns} data={logs} {loading} pageSize={25} emptyMessage="No execution logs yet">
    {#snippet cell(column, row)}
      {#if column.key === 'status'}
        <span class="status-cell" style="color: {getStatusColor(row.status)}">
          <svelte:component this={getStatusIcon(row.status)} size={14} />
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
            {row.error_message.length > 60 ? row.error_message.slice(0, 60) + '...' : row.error_message}
          </span>
        {/if}
      {:else if column.key === 'actions'}
        {#if row.execution_trace}
          <Button variant="ghost" size="small" onclick={() => selectedLog = row}>
            <Eye size={14} />
          </Button>
        {/if}
      {/if}
    {/snippet}
  </DataTable>
</div>

{#if selectedLog}
  <ExecutionTraceModal
    log={selectedLog}
    onclose={() => selectedLog = null}
  />
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
