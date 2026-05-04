<script>
  import { onDestroy, onMount } from 'svelte';
  import { IconRefresh, IconAlertCircle } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import { getActionLogs } from '../../api/diagnostics.js';

  /** @type {{loading: boolean, error: string|null, failed: any[], slowest: any[]}} */
  let state = $state({ loading: true, error: null, failed: [], slowest: [] });
  let lastRefreshed = $state(null);

  async function load() {
    state = { ...state, loading: true, error: null };
    try {
      const [failed, slowest] = await Promise.all([
        getActionLogs({ mode: 'failed', since: '24h', limit: 25 }),
        getActionLogs({ mode: 'slowest', since: '24h', limit: 10 }),
      ]);
      state = { loading: false, error: null, failed: failed ?? [], slowest: slowest ?? [] };
      lastRefreshed = new Date();
    } catch (err) {
      state = { ...state, loading: false, error: err?.message ?? String(err) };
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
      disabled={state.loading}
      class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
      aria-label="Refresh"
    >
      <IconRefresh size={14} stroke={1.75} />
      {state.loading ? 'Refreshing…' : 'Refresh'}
    </button>
  </div>

  {#if state.error}
    <Card variant="outlined">
      <div class="flex items-start gap-3" style="color: var(--ds-text-danger);">
        <IconAlertCircle size={18} stroke={1.75} style="flex-shrink: 0; margin-top: 2px;" />
        <div class="text-sm">
          <p class="font-medium">Failed to load action logs</p>
          <p style="color: var(--ds-text-subtle);">{state.error}</p>
        </div>
      </div>
    </Card>
  {/if}

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Recent failures</h4>
    <Card padding="none">
      {#if state.failed.length === 0 && !state.loading}
        <div class="px-4 py-8 text-center text-sm" style="color: var(--ds-text-subtle);">
          No failed action executions in the last 24h.
        </div>
      {:else}
        <table class="w-full text-sm" data-testid="action-failures-table">
          <thead>
            <tr style="background-color: var(--ds-surface);">
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">When</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Action</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Workspace</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Item</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Error</th>
              <th class="text-right font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Duration</th>
            </tr>
          </thead>
          <tbody>
            {#each state.failed as log (log.id)}
              <tr style="border-top: 1px solid var(--ds-border);">
                <td class="px-4 py-2 font-mono text-xs whitespace-nowrap" style="color: var(--ds-text-subtle);">
                  {formatTime(log.started_at)}
                </td>
                <td class="px-4 py-2" style="color: var(--ds-text);">{log.action_name || `#${log.action_id}`}</td>
                <td class="px-4 py-2" style="color: var(--ds-text-subtle);">{log.workspace_name || '—'}</td>
                <td class="px-4 py-2" style="color: var(--ds-text-subtle);">{log.item_title || (log.item_id ? `#${log.item_id}` : '—')}</td>
                <td class="px-4 py-2" style="color: var(--ds-text);" title={log.error_message}>
                  {truncate(log.error_message, 80) || '—'}
                </td>
                <td class="px-4 py-2 text-right font-mono" style="color: var(--ds-text-subtle);">
                  {formatDuration(log.duration_ms)}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </Card>
  </div>

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Slowest completed runs</h4>
    <Card padding="none">
      {#if state.slowest.length === 0 && !state.loading}
        <div class="px-4 py-8 text-center text-sm" style="color: var(--ds-text-subtle);">
          No completed action executions in the last 24h.
        </div>
      {:else}
        <table class="w-full text-sm" data-testid="action-slowest-table">
          <thead>
            <tr style="background-color: var(--ds-surface);">
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">When</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Action</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Workspace</th>
              <th class="text-left font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Status</th>
              <th class="text-right font-medium px-4 py-2" style="color: var(--ds-text-subtle);">Duration</th>
            </tr>
          </thead>
          <tbody>
            {#each state.slowest as log (log.id)}
              <tr style="border-top: 1px solid var(--ds-border);">
                <td class="px-4 py-2 font-mono text-xs whitespace-nowrap" style="color: var(--ds-text-subtle);">
                  {formatTime(log.started_at)}
                </td>
                <td class="px-4 py-2" style="color: var(--ds-text);">{log.action_name || `#${log.action_id}`}</td>
                <td class="px-4 py-2" style="color: var(--ds-text-subtle);">{log.workspace_name || '—'}</td>
                <td class="px-4 py-2" style="color: var(--ds-text-subtle);">{log.status}</td>
                <td class="px-4 py-2 text-right font-mono" style="color: var(--ds-text);">
                  {formatDuration(log.duration_ms)}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </Card>
  </div>

  {#if lastRefreshed}
    <p class="text-xs" style="color: var(--ds-text-subtle);">
      Last refreshed {lastRefreshed.toLocaleTimeString()}
    </p>
  {/if}
</section>
