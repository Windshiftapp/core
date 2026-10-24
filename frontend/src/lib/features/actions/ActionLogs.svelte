<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import { actions } from '../../api/actions.js';
  import { t } from '../../stores/i18n.svelte.js';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import ExecutionTraceModal from './ExecutionTraceModal.svelte';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import XCircle from '@lucide/svelte/icons/x-circle';
  import Clock from '@lucide/svelte/icons/clock';
  import SkipForward from '@lucide/svelte/icons/skip-forward';
  import Eye from '@lucide/svelte/icons/eye';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';

  let { workspaceId, action, onBack } = $props();

  function getTriggerTypeLabel(triggerType) {
    const labels = {
      'status_transition': t('actions.trigger.statusTransition'),
      'item_created': t('actions.trigger.itemCreated'),
      'sla_breached': t('actions.trigger.slaBreached'),
      'sla_warning': t('actions.trigger.slaWarning'),
      'item_updated': t('actions.trigger.itemUpdated'),
      'item_linked': t('actions.trigger.itemLinked')
    };
    return labels[triggerType] || triggerType;
  }

  let logs = $state([]);
  let loading = $state(true);
  let selectedLog = $state(null);

  function showDetails(log) {
    selectedLog = log;
  }

  function closeDetails() {
    selectedLog = null;
  }

  // DataTable columns
  const columns = [
    { key: 'status', label: t('actions.logs.status'), slot: 'status', width: '120px' },
    { key: 'item_title', label: t('items.item'), slot: 'item' },
    { key: 'trigger_event', label: t('common.type'), render: (item) => getTriggerTypeLabel(item.trigger_event) },
    { key: 'started_at', label: t('actions.logs.startedAt'), slot: 'date', width: '180px' },
    { key: 'duration', label: t('time.duration'), render: (item) => formatDuration(item), width: '100px' },
    { key: 'error_message', label: t('actions.logs.error'), slot: 'error' },
    { key: 'details', label: t('actions.logs.details'), slot: 'details', width: '100px' }
  ];

  onMount(loadLogs);

  async function loadLogs() {
    loading = true;
    try {
      logs = await actions.getLogs(workspaceId, action.id) || [];
    } catch (err) {
      console.error('Failed to load logs:', err);
      logs = [];
    } finally {
      loading = false;
    }
  }

  function formatDuration(log) {
    if (!log.started_at || !log.completed_at) return '-';
    const ms = new Date(log.completed_at).getTime() - new Date(log.started_at).getTime();
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  function formatDate(dateStr) {
    if (!dateStr) return '-';
    return formatAuthenticatedDateTime(dateStr);
  }
</script>

<div class="action-logs h-full flex flex-col" data-testid="action-logs">
  <!-- Header with back button -->
  <div class="flex items-center gap-4 p-4 border-b header">
    <BackButton onclick={onBack} tooltip={t('common.back')} class="p-1" />
    <div class="flex-1">
      <h1 class="text-xl font-medium title">{t('actions.logs.title')}</h1>
      <p class="text-sm subtitle">{action.name}</p>
    </div>
  </div>

  <!-- DataTable -->
  <div class="flex-1 p-4 overflow-auto">
    {#if loading}
      <StateDisplay type="loading" />
    {:else}
      <DataTable
        {columns}
        data={logs}
        keyField="id"
        emptyMessage={t('actions.logs.noLogs')}
        emptyIcon={Clock}
        pagination={true}
        pageSize={25}
        rowAttrs={(item) => ({ 'data-testid': `action-log-row-${item.id}` })}
      >
        <!-- Status slot with icon -->
        {#snippet status(item)}
          <div class="flex items-center gap-2">
            {#if item.status === 'completed'}
              <CheckCircle class="w-4 h-4" style="color: var(--ds-icon-success);" />
              <span style="color: var(--ds-text-success);">{t('actions.logs.completed')}</span>
            {:else if item.status === 'failed'}
              <XCircle class="w-4 h-4" style="color: var(--ds-icon-danger);" />
              <span style="color: var(--ds-text-danger);">{t('actions.logs.failed')}</span>
            {:else if item.status === 'running'}
              <Clock class="w-4 h-4 animate-pulse" style="color: var(--ds-text-info);" />
              <span style="color: var(--ds-text-info);">{t('actions.logs.running')}</span>
            {:else if item.status === 'skipped'}
              <SkipForward class="w-4 h-4 text-ds-icon-subtle" />
              <span class="text-ds-text-subtle">{t('actions.logs.skipped')}</span>
            {:else}
              <span class="capitalize status-text">{item.status}</span>
            {/if}
          </div>
        {/snippet}

        <!-- Item slot with clickable link -->
        {#snippet item(item)}
          {#if item.item_id}
            <a
              href={`/workspaces/${workspaceId}/items/${item.item_id}`}
              class="link"
            >
              {item.item_title || '-'}
            </a>
          {:else}
            <span class="text-subtle">{item.item_title || '-'}</span>
          {/if}
        {/snippet}

        <!-- Date slot -->
        {#snippet date(item)}
          <span class="date-text whitespace-nowrap">{formatDate(item.started_at)}</span>
        {/snippet}

        <!-- Error slot -->
        {#snippet error(item)}
          {#if item.error_message}
            <span class="text-ds-text-danger text-xs truncate block max-w-xs" title={item.error_message}>
              {item.error_message}
            </span>
          {:else}
            <span class="text-ds-text-subtlest">—</span>
          {/if}
        {/snippet}

        <!-- Details button slot -->
        {#snippet details(item)}
          <Button
            variant="ghost"
            size="sm"
            onclick={() => showDetails(item)}
            title={t('actions.logs.viewDetails')}
            dataTestid="action-log-details"
          >
            <Eye class="w-4 h-4" />
          </Button>
        {/snippet}
      </DataTable>
    {/if}
  </div>
</div>

<!-- Execution Trace Modal -->
{#if selectedLog}
  <ExecutionTraceModal log={selectedLog} onclose={closeDetails} />
{/if}

<style>
  .action-logs {
    background-color: var(--ds-surface);
  }

  .header {
    border-color: var(--ds-border);
    background-color: var(--ds-surface-raised);
  }

  .title {
    color: var(--ds-text);
  }

  .subtitle {
    color: var(--ds-text-subtle);
  }

  .status-text {
    color: var(--ds-text-subtle);
  }

  .date-text {
    color: var(--ds-text);
  }

  .text-subtle {
    color: var(--ds-text-subtle);
  }
</style>
