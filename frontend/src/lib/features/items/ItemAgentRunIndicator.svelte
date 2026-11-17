<script>
  // Live "agent working" indicator for the comments view. It reads the same
  // run list as the Agent log tab: a run is in flight until it reaches a
  // terminal status. Stale runners are reaped server-side, so a lingering
  // "running" status self-heals instead of pinning the indicator forever.
  import { onMount, onDestroy } from 'svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import { agentRuns } from '../../api/agentRuns.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { itemId, onOpenLog = undefined } = $props();

  const TERMINAL = ['succeeded', 'failed', 'canceled', 'killed'];
  const POLL_MS = 10_000;

  let activeRun = $state(null);
  let disposed = false;
  let timer = null;

  async function load() {
    try {
      const runs = await agentRuns.listForItem(itemId, { limit: 10 });
      if (disposed) return;
      activeRun = (runs || []).find((run) => !TERMINAL.includes(run.status)) || null;
    } catch {
      // The indicator is passive; a failed probe just hides it.
      if (!disposed) activeRun = null;
    }
  }

  onMount(() => {
    load();
    timer = setInterval(load, POLL_MS);
  });

  onDestroy(() => {
    disposed = true;
    if (timer) clearInterval(timer);
  });
</script>

{#if activeRun}
  {@const running = activeRun.status === 'running'}
  <button
    type="button"
    class="mb-4 inline-flex items-center gap-1.5 rounded text-xs text-ds-text-subtle transition-colors hover:text-ds-text"
    onclick={() => onOpenLog?.()}
    title={t('items.agentLog')}
    data-testid="item-agent-run-indicator"
  >
    <ColorDot
      color={running ? 'var(--ds-status-success-solid)' : 'var(--ds-accent-gray)'}
      size="sm"
    />
    <span>{running ? t('items.agentRunning') : t('items.agentQueued')}</span>
  </button>
{/if}
