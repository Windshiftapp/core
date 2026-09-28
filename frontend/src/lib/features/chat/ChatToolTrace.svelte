<script>
  /**
   * Renders the per-message tool-call trace + iteration footer so users
   * can see what the agent actually did. Surfaces three failure modes
   * that were previously invisible:
   *   1. Tools that returned soft errors (`{"error": "..."}` in result).
   *   2. The agent hitting the iteration ceiling (stopReason === 'max_iterations').
   *   3. Empty tool_calls when the agent went straight to a text reply.
   */
  import { IconChevronRight, IconAlertTriangle, IconCircleCheck, IconCircleX, IconReceipt } from '@tabler/icons-svelte-runes';
  import { formatCostUSD, hasMeteredUsage } from '../../utils/llmUsage.js';

  /**
   * @type {{ toolCalls?: any[], iterations?: number, maxIterations?: number, stopReason?: string, needsReview?: boolean, reviewReasons?: string[], model?: string, usage?: { prompt_tokens: number, completion_tokens: number, total_tokens: number, cost_usd: number|null, calls: number }|null }}
   */
  let {
    toolCalls = [],
    iterations = 0,
    maxIterations = 0,
    stopReason = '',
    needsReview = false,
    reviewReasons = [],
    model = '',
    usage = null,
  } = $props();

  let expanded = $state(false);
  let expandedIdx = $state(/** @type {number | null} */ (null));
  // The token/cost reading is behind a click rather than inline: it is
  // reference detail most turns never need, and keeping it collapsed stops the
  // footer from competing with the answer for attention.
  let showMeter = $state(false);

  const hitLimit = $derived(stopReason === 'max_iterations');
  // Nothing to report when metering never ran for the turn, or when the
  // fallback client served it and there is no model to name.
  const hasMeter = $derived(hasMeteredUsage(usage));
  const meterLabel = $derived.by(() => {
    if (!hasMeter) return '';
    const parts = [];
    if (model) parts.push(model);
    parts.push(`${usage.total_tokens.toLocaleString()} tokens`);
    parts.push(
      `(${usage.prompt_tokens.toLocaleString()} in · ${usage.completion_tokens.toLocaleString()} out)`
    );
    const cost = formatCostUSD(usage.cost_usd);
    parts.push(cost || 'cost unknown');
    return parts.join(' · ');
  });

  function toolStatus(tc) {
    // Soft-error convention: tool returned a JSON body with an "error"
    // field. Distinct from a Go-level execution error, which the agent
    // wraps as `{"error": "..."}` too — both look the same to the user,
    // which is fine.
    if (!tc?.result) return 'ok';
    try {
      const parsed = JSON.parse(tc.result);
      if (parsed && typeof parsed === 'object' && parsed.error) return 'error';
    } catch {
      // Non-JSON result counts as success — the tool returned a string.
    }
    return 'ok';
  }

  function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, n) + '…' : s;
  }

  function prettyJSON(s) {
    if (!s) return '';
    try {
      return JSON.stringify(JSON.parse(s), null, 2);
    } catch {
      return s;
    }
  }
</script>

{#if needsReview}
  <div class="review" role="status">
    <IconAlertTriangle size={13} stroke={1.5} class="icon-err" />
    <div class="review-body">
      <strong>Needs human review</strong>
      <span>The model misused a tool and didn't recover — the answer may not be grounded.</span>
      {#if reviewReasons.length > 0}
        <ul>
          {#each reviewReasons as reason}
            <li>{reason}</li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}

{#if toolCalls.length > 0 || hitLimit || hasMeter}
  <div class="trace">
    {#if toolCalls.length > 0 || hitLimit}
      <button
        class="summary"
        class:warn={hitLimit}
        onclick={() => (expanded = !expanded)}
        aria-expanded={expanded}
        type="button"
      >
        <IconChevronRight
          size={12}
          stroke={1.5}
          class={expanded ? 'chev open' : 'chev'}
        />
        {#if hitLimit}
          <IconAlertTriangle size={12} stroke={1.5} class="icon-warn" />
          <span>Ran out of steps after {iterations}/{maxIterations} — {toolCalls.length} tool call{toolCalls.length === 1 ? '' : 's'}</span>
        {:else}
          <span>{toolCalls.length} tool call{toolCalls.length === 1 ? '' : 's'} · {iterations}/{maxIterations} steps</span>
        {/if}
      </button>
    {/if}

    {#if hasMeter}
      <!-- No aria-label: the accessible name comes from the content, so the
           expanded reading is announced rather than overridden by a label
           that says only "usage". The collapsed icon gets its name from the
           visually-hidden span instead. -->
      <button
        class="meter"
        class:on={showMeter}
        onclick={() => (showMeter = !showMeter)}
        aria-expanded={showMeter}
        title="Model and token usage"
        data-testid="chat-usage-toggle"
        type="button"
      >
        <IconReceipt size={12} stroke={1.5} />
        {#if showMeter}
          <span class="meter-detail" data-testid="chat-usage-detail">{meterLabel}</span>
        {:else}
          <span class="sr-only">Model and token usage</span>
        {/if}
      </button>
    {/if}

    {#if expanded}
      <ol class="calls">
        {#each toolCalls as tc, i}
          {@const status = toolStatus(tc)}
          <li>
            <button
              class="call-head"
              class:err={status === 'error'}
              onclick={() => (expandedIdx = expandedIdx === i ? null : i)}
              aria-expanded={expandedIdx === i}
              type="button"
            >
              {#if status === 'error'}
                <IconCircleX size={11} stroke={2} class="icon-err" />
              {:else}
                <IconCircleCheck size={11} stroke={2} class="icon-ok" />
              {/if}
              <code class="name">{tc.name}</code>
              <span class="argpreview">{truncate(tc.arguments || '', 80)}</span>
            </button>
            {#if expandedIdx === i}
              <div class="detail">
                <div class="label">arguments</div>
                <pre>{prettyJSON(tc.arguments)}</pre>
                <div class="label">result</div>
                <pre class:err={status === 'error'}>{prettyJSON(tc.result)}</pre>
              </div>
            {/if}
          </li>
        {/each}
      </ol>
    {/if}
  </div>
{/if}

<style>
  .review {
    display: flex;
    gap: 6px;
    margin-top: 6px;
    padding: 6px 8px;
    font-size: 11px;
    border-radius: 4px;
    background: var(--ds-background-danger-subtle, #fef2f2);
    border: 1px solid var(--ds-border-danger, #f87171);
    color: var(--ds-text-danger, #b91c1c);
  }
  .review-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .review-body strong {
    font-weight: 600;
  }
  .review-body ul {
    margin: 2px 0 0;
    padding-left: 16px;
  }
  .trace {
    margin-top: 6px;
    font-size: 11px;
    color: var(--ds-text-subtle);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
  }
  .meter {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    color: var(--ds-text-subtle);
    cursor: pointer;
    font: inherit;
  }
  .meter:hover {
    background: var(--ds-background-neutral-hovered);
  }
  .meter.on {
    /* Expanded: the reading is the button's content, so it reads as one
       control rather than a label paired with loose text. */
    align-items: baseline;
  }
  .meter-detail {
    color: var(--ds-text);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  .summary {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    color: var(--ds-text-subtle);
    cursor: pointer;
    font: inherit;
  }
  .summary:hover {
    background: var(--ds-background-neutral-hovered);
  }
  .summary.warn {
    background: var(--ds-background-warning-subtle, #fef3c7);
    border-color: var(--ds-border-warning, #f59e0b);
    color: var(--ds-text-warning, #92400e);
  }
  :global(.chev) {
    transition: transform 120ms ease;
  }
  :global(.chev.open) {
    transform: rotate(90deg);
  }
  :global(.icon-warn) {
    color: var(--ds-text-warning, #b45309);
  }
  :global(.icon-ok) {
    color: var(--ds-text-success, #15803d);
  }
  :global(.icon-err) {
    color: var(--ds-text-danger, #b91c1c);
  }
  .calls {
    margin: 6px 0 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .call-head {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 3px 6px;
    text-align: left;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-radius: 4px;
    cursor: pointer;
    font: inherit;
    color: var(--ds-text);
  }
  .call-head:hover {
    background: var(--ds-background-neutral-hovered);
  }
  .call-head.err {
    border-color: var(--ds-border-danger, #f87171);
  }
  .name {
    font-family: var(--font-mono, ui-monospace, monospace);
    color: var(--ds-text);
  }
  .argpreview {
    color: var(--ds-text-subtlest);
    font-family: var(--font-mono, ui-monospace, monospace);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .detail {
    margin: 4px 0 0 14px;
    padding: 6px 8px;
    background: var(--ds-surface-sunken);
    border-radius: 4px;
  }
  .label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ds-text-subtlest);
    margin: 4px 0 2px;
  }
  pre {
    margin: 0;
    padding: 4px 6px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-radius: 3px;
    font-size: 10.5px;
    font-family: var(--font-mono, ui-monospace, monospace);
    color: var(--ds-text);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 240px;
    overflow: auto;
  }
  pre.err {
    color: var(--ds-text-danger, #b91c1c);
    background: var(--ds-background-danger-subtle, #fef2f2);
    border-color: var(--ds-border-danger, #f87171);
  }
</style>
