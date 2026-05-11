<script>
  import { onDestroy, onMount } from 'svelte';
  import { IconRefresh, IconAlertCircle } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { getActionLogs } from '../../api/diagnostics.js';

  /** @type {{loading: boolean, error: string|null, failed: any[], slowest: any[]}} */
  let view = $state({ loading: true, error: null, failed: [], slowest: [] });
  let lastRefreshed = $state(null);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const [failed, slowest] = await Promise.all([
        getActionLogs({ mode: 'failed', since: '24h', limit: 25 }),
        getActionLogs({ mode: 'slowest', since: '24h', limit: 10 }),
      ]);
      view = { loading: false, error: null, failed: failed ?? [], slowest: slowest ?? [] };
      lastRefreshed = new Date();
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  let interval;
  onMount(() => {
    load();
    interval = setInterval(load, 30_000);
  });
  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  function formatTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toISOString().replace('T', ' ').replace(/\..*Z$/, ' UTC');
  }

  function formatDuration(ms) {
    if (ms == null) return '—';
    if (ms < 1000) return `${ms} ms`;
    const sec = ms / 1000;
    if (sec < 60) return `${sec.toFixed(sec < 10 ? 2 : 1)}s`;
    const min = Math.floor(sec / 60);
    const remSec = Math.round(sec - min * 60);
    return `${min}m ${remSec}s`;
  }

  function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? `${s.slice(0, n - 1)}…` : s;
  }

  const failedColumns = [
    { key: 'started_at', label: 'When', render: (log) => formatTime(log.started_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'action_name', label: 'Action', render: (log) => log.action_name || `#${log.action_id}` },
    { key: 'workspace_name', label: 'Workspace', render: (log) => log.workspace_name || '—', textColor: 'var(--ds-text-subtle)' },
    { key: 'item_title', label: 'Item', render: (log) => log.item_title || (log.item_id ? `#${log.item_id}` : '—'), textColor: 'var(--ds-text-subtle)' },
    { key: 'error_message', label: 'Error', slot: 'error' },
    { key: 'duration_ms', label: 'Duration', render: (log) => formatDuration(log.duration_ms), align: 'text-right', textColor: 'var(--ds-text-subtle)' },
  ];

  const slowestColumns = [
    { key: 'started_at', label: 'When', render: (log) => formatTime(log.started_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'action_name', label: 'Action', render: (log) => log.action_name || `#${log.action_id}` },
    { key: 'workspace_name', label: 'Workspace', render: (log) => log.workspace_name || '—', textColor: 'var(--ds-text-subtle)' },
    { key: 'status', label: 'Status', textColor: 'var(--ds-text-subtle)' },
    { key: 'duration_ms', label: 'Duration', render: (log) => formatDuration(log.duration_ms), align: 'text-right' },
  ];
</script>

<section class="space-y-6" data-testid="diagnostics-action-logs">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text);">Action executions (last 24h)</h3>
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        Recent failures and slowest completed runs across all workspaces. Auto-refreshes every 30s.
      </p>
    </div>
    <button
      onclick={load}
      disabled={view.loading}
      class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
      aria-label="Refresh"
    >
      <IconRefresh size={14} stroke={1.75} />
      {view.loading ? 'Refreshing…' : 'Refresh'}
    </button>
  </div>

  {#if view.error}
    <Card variant="outlined">
      <div class="flex items-start gap-3" style="color: var(--ds-text-danger);">
        <IconAlertCircle size={18} stroke={1.75} style="flex-shrink: 0; margin-top: 2px;" />
        <div class="text-sm">
          <p class="font-medium">Failed to load action logs</p>
          <p style="color: var(--ds-text-subtle);">{view.error}</p>
        </div>
      </div>
    </Card>
  {/if}

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Recent failures</h4>
    <DataTable
      columns={failedColumns}
      data={view.failed}
      keyField="id"
      emptyMessage="No failed action executions in the last 24h."
    >
      {#snippet error(log)}
        <span style="color: var(--ds-text);" title={log.error_message}>
          {truncate(log.error_message, 80) || '—'}
        </span>
      {/snippet}
    </DataTable>
  </div>

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Slowest completed runs</h4>
    <DataTable
      columns={slowestColumns}
      data={view.slowest}
      keyField="id"
      emptyMessage="No completed action executions in the last 24h."
    />
  </div>

  {#if lastRefreshed}
    <p class="text-xs" style="color: var(--ds-text-subtle);">
      Last refreshed {lastRefreshed.toLocaleTimeString()}
    </p>
  {/if}
</section>
