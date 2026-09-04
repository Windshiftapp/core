<script>
  import { buildProgressSegments, calculatePercentComplete, PROGRESS_CHART_RADIUS } from '../utils/progressChart.js';
  import { objectDisplayName } from '../utils/systemLabels.js';

  let {
    progress,
    ariaLabel,
    completeLabel,
    noItemsLabel,
    summaryLabel,
    totalLabel,
    completedLabel,
    remainingLabel,
    statusLabel,
    noStatusDataLabel,
  } = $props();

  const radius = PROGRESS_CHART_RADIUS;
  const segments = $derived(buildProgressSegments(progress.status_breakdown, progress.total_items));
  const percentComplete = $derived(
    calculatePercentComplete(progress.completed_items, progress.total_items, progress.percent_complete),
  );
</script>

<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-6">
  <div class="rounded-xl border p-6 flex flex-col items-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="relative">
      {#if progress.total_items > 0}
        <svg viewBox="0 0 140 140" class="w-36 h-36" role="img" aria-label={ariaLabel}>
          <circle cx="70" cy="70" r={radius} fill="transparent" stroke="var(--ds-border)" stroke-width="16" />
          {#each segments as segment (segment.category_name)}
            <circle
              cx="70"
              cy="70"
              r={radius}
              fill="transparent"
              stroke={segment.color}
              stroke-width="16"
              stroke-linecap="butt"
              stroke-dasharray={segment.dasharray}
              stroke-dashoffset={segment.offset}
              transform="rotate(-90 70 70)"
            />
          {/each}
          <text class="text-2xl font-bold" x="70" y="68" text-anchor="middle" fill="var(--ds-text)">
            {percentComplete}%
          </text>
          <text class="text-xs uppercase" x="70" y="86" text-anchor="middle" fill="var(--ds-text-subtle)">
            {completeLabel}
          </text>
        </svg>
      {:else}
        <div class="w-36 h-36 rounded-full border-2 border-dashed flex items-center justify-center" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtlest);">{noItemsLabel}</span>
        </div>
      {/if}
    </div>
  </div>

  <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">{summaryLabel}</h3>
    <div class="space-y-3">
      <div class="flex justify-between items-center">
        <span style="color: var(--ds-text-subtle);">{totalLabel}</span>
        <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items}</span>
      </div>
      <div class="flex justify-between items-center">
        <span style="color: var(--ds-text-subtle);">{completedLabel}</span>
        <span class="font-semibold" style="color: var(--ds-text-success);">{progress.completed_items}</span>
      </div>
      <div class="flex justify-between items-center">
        <span style="color: var(--ds-text-subtle);">{remainingLabel}</span>
        <span class="font-semibold" style="color: var(--ds-text);">{progress.total_items - progress.completed_items}</span>
      </div>
    </div>
  </div>

  <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text-subtle);">{statusLabel}</h3>
    <div class="space-y-2">
      {#if progress.status_breakdown?.length > 0}
        {#each progress.status_breakdown as breakdown}
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <div class="w-3 h-3 rounded-full" style="background-color: {breakdown.category_color || '#9ca3af'};"></div>
              <span class="text-sm" style="color: var(--ds-text);">
                {objectDisplayName({
                  name: breakdown.category_name,
                  builtin_key: breakdown.category_builtin_key,
                }, 'status_category')}
              </span>
            </div>
            <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{breakdown.item_count}</span>
          </div>
        {/each}
      {:else}
        <p class="text-sm" style="color: var(--ds-text-subtlest);">{noStatusDataLabel}</p>
      {/if}
    </div>
  </div>
</div>
