<script>
  // Agent log tab (WI-260): the runs an agent executed for this work item,
  // with a live-tailing transcript of the selected run. Pure add-on — reads
  // only the agent_runs/agent_run_events surface, never item state.
  import { onMount, onDestroy } from 'svelte';
  import { Bot, RefreshCw, TriangleAlert, Ban } from '@lucide/svelte';
  import { agentRuns } from '../../api/agentRuns.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { formatAuthenticatedDateTime as formatDateTimeLocale } from '../../utils/authenticatedDateFormatter.js';
  import { formatCostUSD, hasMeteredUsage } from '../../utils/llmUsage.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacePermissions } from '../../stores';
  import { confirm } from '../../composables/useConfirm.js';

  let { itemId, workspaceId } = $props();

  // Re-run enqueues a real agent run — gate the control on the same item.edit
  // permission the backend enforces, so view-only users don't see a button
  // that would 404 on click.
  const canRerun = $derived(workspaceId ? workspacePermissions.canEdit(workspaceId) : false);
  // Cancel is workspace-admin only, matching the backend gate.
  const canCancel = $derived(workspaceId ? workspacePermissions.canAdminWorkspace(workspaceId) : false);

  const RUNS_POLL_MS = 10_000;
  const EVENTS_POLL_MS = 1_500;
  const TERMINAL = ['succeeded', 'failed', 'canceled', 'killed'];

  let runs = $state([]);
  let loading = $state(true);
  let selectedRunId = $state(null);
  let lines = $state([]);
  let liveToken = null; // invalidates the tail loop on switch/unmount
  let runsTimer = null;

  // Recovery-aware "needs human review" flag for the selected run, parsed from
  // its review_flagged event (emitted by the runner on unrecovered, high-signal
  // tool misuse). Null when the selected run is clean.
  let reviewFlag = $state(/** @type {null | { reasons: string[] }} */ (null));

  // Metered LLM token/cost totals for the selected run (WI-494), fetched
  // alongside the transcript. Null until loaded; cost_usd is null when rates
  // are unknown (non-OpenRouter), in which case only tokens are shown.
  let usage = $state(/** @type {null | { prompt_tokens: number, completion_tokens: number, total_tokens: number, cost_usd: number|null, calls: number }} */ (null));

  // Re-run button state. A run is enqueued, not started synchronously, so the
  // button stays disabled while any run is in flight (hasActiveRun) AND while a
  // trigger request is outstanding (rerunning) — together they prevent stacking.
  let rerunning = $state(false);
  let rerunError = $state('');

  // Cancel state. cancelRequested flips after a cooperative cancel so the
  // control offers Force cancel when the run stays running (a phantom whose
  // runner never observed the flag).
  let canceling = $state(false);
  let cancelRequested = $state(false);
  let cancelError = $state('');

  const selectedRun = $derived(runs.find((r) => r.id === selectedRunId) || null);
  const hasActiveRun = $derived(runs.some((r) => !TERMINAL.includes(r.status)));
  const activeRun = $derived(runs.find((r) => !TERMINAL.includes(r.status)) || null);

  // Once the active run clears (canceled, or a later run begins), reset the
  // cancel control back to its cooperative-first state.
  $effect(() => {
    if (!activeRun) cancelRequested = false;
  });

  async function doCancel(force) {
    if (!activeRun || canceling) return;
    const ok = await confirm({
      title: force ? 'Force-cancel run' : 'Cancel run',
      message: force
        ? `Force-cancel run #${activeRun.id}? This marks it canceled immediately, regardless of the runner. Use this only for a stuck run whose worker is gone — if the runner is still working, its result will be discarded.`
        : `Cancel run #${activeRun.id}? The runner is asked to stop at its next heartbeat.`,
      confirmText: force ? 'Force cancel' : 'Cancel run',
      variant: 'danger',
    });
    if (!ok) return;
    canceling = true;
    cancelError = '';
    try {
      await agentRuns.cancel(activeRun.id, { force });
      cancelRequested = true;
      await loadRuns();
    } catch (e) {
      cancelError = e?.message || 'Failed to cancel run';
    } finally {
      canceling = false;
    }
  }

  function statusAppearance(status) {
    switch (status) {
      case 'succeeded': return 'success';
      case 'failed': case 'killed': return 'error';
      case 'running': return 'inprogress';
      case 'canceled': return 'default';
      default: return 'info'; // queued
    }
  }

  const TOOL_FAILURE_PATTERN = /(^|\n)\((?:exit:|timeout after |cancelled\)|empty (?:command|path|pattern|old_string)\)|unknown tool:|tool arguments were not valid JSON:|path error:|read error:|write error:|mkdir error:|not found:|ambiguous:|offset \d+ is past the end)/;

  function failedToolOutput(output) {
    if (typeof output !== 'string' || !output.trim()) return null;
    const trimmed = output.trim();
    if (TOOL_FAILURE_PATTERN.test(trimmed)) return trimmed;
    if (!trimmed.startsWith('{')) return null;
    try {
      const body = JSON.parse(trimmed);
      return typeof body?.error === 'string' && body.error.trim() ? trimmed : null;
    } catch {
      return null;
    }
  }

  // Inspection-grade transcript: unlike the bindings test panel this KEEPS
  // lifecycle and warning events (queued/claimed/stall warnings are exactly
  // what you need when a run goes nowhere), and still drops streaming
  // `content` deltas that duplicate the final message.
  function eventText(ev) {
    let payload;
    try {
      payload = JSON.parse(ev.payload_json);
    } catch {
      return ev.payload_json; // non-JSON line, show as-is
    }
    if (ev.type === 'lifecycle') {
      const pool = payload.target_pool_id ? ` (pool ${payload.target_pool_id})` : '';
      const runner = payload.runner_name || payload.runner_id;
      return `· ${payload.phase || 'lifecycle'}${pool}${runner ? ` by ${runner}` : ''}`;
    }
    if (ev.type === 'warning') {
      return payload.message ? `⚠ ${payload.message}` : null;
    }
    if (ev.type === 'review_flagged') {
      const reasons = Array.isArray(payload.reasons) ? payload.reasons.join('; ') : '';
      return `⚠ Needs human review — ${reasons || 'unrecovered tool misuse'}`;
    }
    switch (payload.type) {
      case 'message':
        return typeof payload.text === 'string' ? payload.text : null;
      case 'content':
        return null; // streaming duplicate of the final message
      case 'tool_start': {
        if (payload.args?.cmd) return `$ ${payload.args.cmd}`;
        const path = payload.tool === 'read_file' && typeof payload.args?.path === 'string'
          ? payload.args.path.trim()
          : '';
        return `→ ${payload.tool || 'tool'}${path ? ` ${path}` : ''}`;
      }
      case 'tool_done': {
        const failure = failedToolOutput(payload.output);
        return failure ? `⚠ ${payload.tool || 'tool'} failed\n${failure}` : null;
      }
      case 'comment_failed': {
        const error = typeof payload.error === 'string' ? payload.error.trim() : '';
        return `⚠ Work-item comment failed${error ? `\n${error}` : ''}`;
      }
      case 'error':
        return payload.message ? `⚠ ${payload.message}` : null;
      case 'starting':
      case 'session_idle':
        return null;
      default:
        return typeof (payload.text ?? payload.message ?? payload.line) === 'string'
          ? (payload.text ?? payload.message ?? payload.line)
          : null;
    }
  }

  async function loadRuns() {
    try {
      const fetched = await agentRuns.listForItem(itemId, { limit: 50 });
      runs = fetched || [];
      if (!selectedRunId && runs.length) selectRun(runs[0].id);
    } finally {
      loading = false;
    }
  }

  // Pull the review flag out of the raw events of the selected run, if any.
  function scanReviewFlag(events) {
    for (const ev of events) {
      if (ev.type !== 'review_flagged') continue;
      try {
        const p = JSON.parse(ev.payload_json);
        reviewFlag = { reasons: Array.isArray(p.reasons) ? p.reasons : [] };
      } catch {
        reviewFlag = { reasons: [] };
      }
    }
  }

  async function loadUsage(runId, token) {
    const u = await agentRuns.usage(runId).catch(() => null);
    if (liveToken === token) usage = u;
  }

  async function selectRun(runId) {
    selectedRunId = runId;
    lines = [];
    reviewFlag = null;
    usage = null;
    const token = Symbol('agent-log');
    liveToken = token;
    let afterId = 0;
    loadUsage(runId, token); // best-effort initial totals
    while (liveToken === token) {
      let run;
      try {
        const events = await agentRuns.listEventsAfter(runId, afterId, 200);
        if (liveToken !== token) return;
        if (events?.length) {
          afterId = events[events.length - 1].id;
          scanReviewFlag(events);
          const fresh = events.map(eventText).filter(Boolean);
          if (fresh.length) lines = [...lines, ...fresh];
        }
        run = await agentRuns.get(runId);
        if (liveToken !== token) return;
        runs = runs.map((r) => (r.id === run.id ? { ...r, ...run } : r));
      } catch {
        return; // run vanished or request failed; stop tailing quietly
      }
      if (TERMINAL.includes(run.status)) {
        const tail = await agentRuns.listEventsAfter(runId, afterId, 200).catch(() => []);
        if (liveToken === token && tail?.length) {
          scanReviewFlag(tail);
          lines = [...lines, ...tail.map(eventText).filter(Boolean)];
        }
        loadUsage(runId, token); // final totals once the run is done
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, EVENTS_POLL_MS));
    }
  }

  async function doRerun() {
    if (rerunning || hasActiveRun) return;
    rerunning = true;
    rerunError = '';
    try {
      await agentRuns.rerun(itemId);
      // The new run is queued, not started; refresh and jump to it so the
      // transcript tails the fresh run. hasActiveRun then keeps the button
      // disabled until it reaches a terminal state.
      await loadRuns();
      if (runs.length) selectRun(runs[0].id);
    } catch (e) {
      rerunError = e?.message || t('items.agentRerunFailed');
    } finally {
      rerunning = false;
    }
  }

  onMount(() => {
    loadRuns();
    runsTimer = setInterval(loadRuns, RUNS_POLL_MS);
  });
  onDestroy(() => {
    liveToken = null;
    if (runsTimer) clearInterval(runsTimer);
  });
</script>

{#if loading}
  <div class="p-4 text-center" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
{:else if runs.length === 0}
  <EmptyState icon={Bot} title={t('items.agentLogEmpty')} />
{:else}
  <div class="space-y-3" data-testid="item-agent-log">
    <!-- Header: run count + manual re-run -->
    <div class="flex items-center justify-between gap-2">
      <span class="text-xs" style="color: var(--ds-text-subtle);">
        {runs.length} {runs.length === 1 ? t('items.agentRunSingular') : t('items.agentRunPlural')}
      </span>
      <div class="flex items-center gap-2">
        {#if canCancel && activeRun}
          <button
            type="button"
            class="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            style="border: 1px solid var(--ds-border-danger, #f87171); color: var(--ds-text-danger, #b91c1c); background-color: transparent;"
            onclick={() => doCancel(cancelRequested)}
            disabled={canceling}
            title={cancelRequested
              ? 'The run is still active — force it to canceled regardless of the runner'
              : 'Ask the runner to stop this run'}
            data-testid="agent-cancel-button"
          >
            <Ban class="w-3 h-3" />
            {canceling ? 'Canceling…' : cancelRequested ? 'Force cancel' : 'Cancel run'}
          </button>
        {/if}
        {#if canRerun}
          <button
            type="button"
            class="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            style="border: 1px solid var(--ds-border); color: var(--ds-text); background-color: transparent;"
            onclick={doRerun}
            disabled={rerunning || hasActiveRun}
            title={hasActiveRun ? t('items.agentRerunBusy') : t('items.agentRerunTitle')}
            data-testid="agent-rerun-button"
          >
            <RefreshCw class={`w-3 h-3 ${rerunning ? 'animate-spin' : ''}`} />
            {rerunning ? t('items.agentRerunStarting') : hasActiveRun ? t('items.agentRerunBusy') : t('items.agentRerunLabel')}
          </button>
        {/if}
      </div>
    </div>

    {#if cancelError}
      <div class="text-xs px-3 py-2 rounded" style="color: var(--ds-text-danger); border: 1px solid var(--ds-border-danger, #f87171); background-color: var(--ds-background-danger-subtle, #fef2f2);" data-testid="agent-cancel-error">
        {cancelError}
      </div>
    {/if}

    {#if rerunError}
      <div class="text-xs px-3 py-2 rounded" style="color: var(--ds-text-danger); border: 1px solid var(--ds-border-danger, #f87171); background-color: var(--ds-background-danger-subtle, #fef2f2);" data-testid="agent-rerun-error">
        {rerunError}
      </div>
    {/if}

    {#if reviewFlag}
      <div
        class="flex gap-2 px-3 py-2 rounded text-xs"
        style="color: var(--ds-text-danger, #b91c1c); border: 1px solid var(--ds-border-danger, #f87171); background-color: var(--ds-background-danger-subtle, #fef2f2);"
        role="status"
        data-testid="agent-review-flag"
      >
        <TriangleAlert class="w-4 h-4 shrink-0" />
        <div class="flex flex-col gap-0.5">
          <strong>{t('items.agentReviewFlagTitle')}</strong>
          <span>{t('items.agentReviewFlagBody')}</span>
          {#if reviewFlag.reasons.length}
            <ul class="list-disc pl-4 mt-0.5">
              {#each reviewFlag.reasons as reason}
                <li>{reason}</li>
              {/each}
            </ul>
          {/if}
        </div>
      </div>
    {/if}

    <!-- Run selector -->
    <div class="flex flex-wrap gap-2">
      {#each runs as run (run.id)}
        <button
          type="button"
          class="flex items-center gap-2 px-2.5 py-1.5 rounded text-xs transition-colors"
          style="
            border: 1px solid {selectedRunId === run.id ? 'var(--ds-border-focused)' : 'var(--ds-border)'};
            background-color: {selectedRunId === run.id ? 'var(--ds-background-selected)' : 'transparent'};
            color: var(--ds-text);
          "
          data-testid={`agent-log-run-${run.id}`}
          onclick={() => selectRun(run.id)}
        >
          <span>#{run.id}</span>
          <Lozenge appearance={statusAppearance(run.status)} size="sm">{run.status}</Lozenge>
          <span style="color: var(--ds-text-subtle);">{formatDateTimeLocale(run.queued_at)}</span>
        </button>
      {/each}
    </div>

    {#if selectedRun}
      {#if selectedRun.error}
        <div class="text-xs px-3 py-2 rounded" style="color: var(--ds-text-danger); border: 1px solid var(--ds-border);">
          {selectedRun.error}
        </div>
      {/if}
      {#if hasMeteredUsage(usage)}
        <div data-testid="agent-run-usage" class="flex items-center flex-wrap gap-x-3 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
          <span>{usage.total_tokens.toLocaleString()} tokens</span>
          <span>({usage.prompt_tokens.toLocaleString()} in · {usage.completion_tokens.toLocaleString()} out)</span>
          {#if formatCostUSD(usage.cost_usd)}
            <span style="color: var(--ds-text);">{formatCostUSD(usage.cost_usd)}</span>
          {:else}
            <span>cost unknown</span>
          {/if}
        </div>
      {/if}
      <pre
        class="text-xs p-3 rounded overflow-auto whitespace-pre-wrap"
        style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text); max-height: 420px;"
        data-testid="agent-log-transcript"
      >{lines.length ? lines.join('\n') : t('items.agentLogWaiting')}</pre>
      {#if !TERMINAL.includes(selectedRun.status)}
        <div class="flex items-center gap-1.5 text-xs" style="color: var(--ds-text-subtle);">
          <RefreshCw class="w-3 h-3 animate-spin" />
          {selectedRun.status}…
        </div>
      {/if}
    {/if}
  </div>
{/if}
