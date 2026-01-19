<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import { ArrowLeft, CheckCircle, XCircle, Clock, SkipForward } from 'lucide-svelte';

  export let workspaceId;
  export let action;
  export let onBack;

  function getTriggerTypeLabel(triggerType) {
    const labels = {
      'status_transition': t('actions.trigger.statusTransition'),
      'item_created': t('actions.trigger.itemCreated'),
      'item_updated': t('actions.trigger.itemUpdated'),
      'item_linked': t('actions.trigger.itemLinked')
    };
    return labels[triggerType] || triggerType;
  }

  let logs = [];
  let loading = true;

  // DataTable columns
  const columns = [
    { key: 'status', label: t('actions.logs.status'), slot: 'status', width: '120px' },
    { key: 'item_title', label: t('items.item'), slot: 'item' },
    { key: 'trigger_event', label: t('common.type'), render: (item) => getTriggerTypeLabel(item.trigger_event) },
    { key: 'started_at', label: t('actions.logs.startedAt'), slot: 'date', width: '180px' },
    { key: 'duration', label: t('time.duration'), render: (item) => formatDuration(item), width: '100px' },
    { key: 'error_message', label: t('actions.logs.error'), slot: 'details' }
  ];

  onMount(loadLogs);

  async function loadLogs() {
    loading = true;
    try {
      logs = await api.get(`/workspaces/${workspaceId}/actions/${action.id}/logs?limit=50`) || [];
    } catch (err) {
      console.error('Failed to load logs:', err);
      logs = [];
    } finally {
      loading = false;
    }
  }

  function formatDuration(log) {
    if (!log.started_at || !log.completed_at) return '-';
    const ms = new Date(log.completed_at) - new Date(log.started_at);
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  function formatDate(dateStr) {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  }
</script>

<div class="action-logs h-full flex flex-col">
  <!-- Header with back button -->
  <div class="flex items-center gap-4 p-4 border-b header">
    <Button variant="ghost" icon={ArrowLeft} onclick={onBack} />
    <div>
      <h2 class="text-lg font-semibold title">{t('actions.logs.title')}</h2>
      <p class="text-sm subtitle">{action.name}</p>
    </div>
  </div>

  <!-- DataTable -->
  <div class="flex-1 p-4 overflow-auto">
    {#if loading}
      <div class="flex justify-center py-12">
        <div class="animate-spin h-8 w-8 border-b-2 border-blue-600 rounded-full"></div>
      </div>
    {:else}
      <DataTable
        {columns}
        data={logs}
        keyField="id"
        emptyMessage={t('actions.logs.noLogs')}
        emptyIcon={Clock}
        pagination={true}
        pageSize={25}
      >
        <!-- Status slot with icon -->
        <svelte:fragment slot="status" let:item>
          <div class="flex items-center gap-2">
            {#if item.status === 'completed'}
              <CheckCircle class="w-4 h-4 text-green-500" />
              <span class="text-green-700">{t('actions.logs.completed')}</span>
            {:else if item.status === 'failed'}
              <XCircle class="w-4 h-4 text-red-500" />
              <span class="text-red-700">{t('actions.logs.failed')}</span>
            {:else if item.status === 'running'}
              <Clock class="w-4 h-4 text-blue-500 animate-pulse" />
              <span class="text-blue-700">{t('actions.logs.running')}</span>
            {:else if item.status === 'skipped'}
              <SkipForward class="w-4 h-4 text-gray-400" />
              <span class="text-gray-500">{t('actions.logs.skipped')}</span>
            {:else}
              <span class="capitalize status-text">{item.status}</span>
            {/if}
          </div>
        </svelte:fragment>

        <!-- Item slot with clickable link -->
        <svelte:fragment slot="item" let:item>
          {#if item.item_id}
            <a
              href={`/workspaces/${workspaceId}/items/${item.item_id}`}
              class="item-link hover:underline"
            >
              {item.item_title || '-'}
            </a>
          {:else}
            <span class="text-subtle">{item.item_title || '-'}</span>
          {/if}
        </svelte:fragment>

        <!-- Date slot -->
        <svelte:fragment slot="date" let:item>
          <span class="date-text">{formatDate(item.started_at)}</span>
        </svelte:fragment>

        <!-- Error details slot -->
        <svelte:fragment slot="details" let:item>
          {#if item.error_message}
            <span class="text-red-500 text-xs truncate block max-w-xs" title={item.error_message}>
              {item.error_message}
            </span>
          {:else}
            <span class="text-gray-400">—</span>
          {/if}
        </svelte:fragment>
      </DataTable>
    {/if}
  </div>
</div>

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

  .item-link {
    color: var(--ds-link);
  }

  .item-link:hover {
    color: var(--ds-link-hovered);
  }

  .text-subtle {
    color: var(--ds-text-subtle);
  }
</style>
