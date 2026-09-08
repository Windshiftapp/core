<script>
  import { Flag } from '@lucide/svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PieChartSegments from '../components/PieChartSegments.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateShort } from '../utils/dateFormatter.js';
  import { buildPieSegments } from '../utils/pieChart.js';
  import { objectDisplayName } from '../utils/systemLabels.js';

  let { milestones = [] } = $props();

  const radius = 48;
  const fallbackColors = ['#2563eb', '#0ea5e9', '#10b981', '#f97316', '#ec4899', '#8b5cf6', '#facc15', '#14b8a6'];

  const formatPercent = (value) => {
    if (typeof value === 'number' && Number.isFinite(value)) {
      return Math.min(100, Math.max(0, Math.round(value)));
    }
    return 0;
  };

  function formatDate(value) {
    if (!value) return null;
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return value;
    return formatDateShort(parsed);
  }

  function normalizeBreakdown(breakdown = []) {
    if (!Array.isArray(breakdown)) return [];
    return breakdown.map((segment, index) => {
      const label = typeof segment?.category_name === 'string' && segment.category_name.trim().length > 0
        ? objectDisplayName({
            name: segment.category_name.trim(),
            builtin_key: segment.category_builtin_key,
          }, 'status_category')
        : t('widgets.milestoneProgress.noStatus');
      const color = segment?.category_color || fallbackColors[index % fallbackColors.length];
      const count = typeof segment?.item_count === 'number' && Number.isFinite(segment.item_count)
        ? segment.item_count
        : 0;
      return {
        key: `${label}-${index}`,
        label,
        color,
        count,
        isCompleted: Boolean(segment?.is_completed)
      };
    });
  }

  function buildSegments(breakdown, totalItems) {
    return buildPieSegments(breakdown, totalItems, radius);
  }
</script>

<div>
  {#if milestones && milestones.length > 0}
    <div class="grid grid-cols-[repeat(auto-fill,minmax(min(100%,17rem),1fr))] gap-3">
      {#each milestones as milestone (milestone.milestone_id)}
        {@const breakdown = normalizeBreakdown(milestone.status_breakdown)}
        {@const segments = buildSegments(breakdown, milestone.total_items)}

        <div class="flex min-w-0 flex-col rounded-lg border border-ds-border bg-ds-surface p-4">
          <div class="mb-3 flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2">
              <div
                class="flex size-7 shrink-0 items-center justify-center rounded-md"
                style={`background-color: color-mix(in srgb, ${milestone.category_color || 'var(--ds-icon-accent)'} 12%, transparent);`}
              >
                <Flag size={16} style={`color: ${milestone.category_color || 'var(--ds-icon-accent)'};`} />
              </div>
              <div class="flex min-w-0 flex-col gap-1">
                <p class="text-sm leading-snug font-semibold wrap-anywhere text-ds-text">{milestone.milestone_name}</p>
                {#if milestone.target_date}
                  <p class="text-xs text-ds-text-subtle">{t('widgets.milestoneProgress.due')} {formatDate(milestone.target_date)}</p>
                {/if}
              </div>
            </div>
            <div class="shrink-0 rounded bg-(--ds-background-neutral) px-2 py-1 text-xs font-semibold tabular-nums text-ds-text">
              {formatPercent(milestone.percent_complete)}%
            </div>
          </div>

          <div class="mt-auto grid grid-cols-[6rem_minmax(0,1fr)] items-center gap-3">
            <div class="flex items-center justify-center">
              {#if milestone.total_items > 0}
                <svg class="size-24 overflow-visible" viewBox="0 0 140 140" role="img" aria-label={t('widgets.milestoneProgress.chartAria')}>
                  <PieChartSegments {segments} {radius} strokeWidth={10} />
                  <text text-anchor="middle" class="fill-ds-text text-[1.75rem] font-semibold tabular-nums" x="70" y="68">{milestone.total_items || 0}</text>
                  <text text-anchor="middle" class="fill-ds-text-subtle text-sm" x="70" y="84">{t('widgets.milestoneProgress.items')}</text>
                </svg>
              {:else}
                <div class="flex size-20 items-center justify-center rounded-full border border-dashed border-ds-border text-center text-xs text-ds-text-subtle">
                  <p>{t('widgets.milestoneProgress.noItems')}</p>
                </div>
              {/if}
            </div>

            <div class="flex flex-col gap-1 wrap-anywhere">
              <p class="text-sm font-medium tabular-nums text-ds-text">
                {milestone.completed_items || 0}/{milestone.total_items || 0} {t('widgets.milestoneProgress.done')}
              </p>
              <p class="text-xs text-ds-text-subtle capitalize">
                {milestone.status ? milestone.status.replace(/_/g, ' ') : t('widgets.milestoneProgress.activeMilestone')}
              </p>
            </div>

            <ul class="col-span-full m-0 flex list-none flex-wrap gap-x-4 gap-y-2 border-t border-ds-border px-0 pt-3 text-xs leading-normal wrap-anywhere text-ds-text-subtle">
              {#if breakdown.length > 0}
                {#each breakdown as segment (segment.key)}
                  <li class="flex min-w-0 items-baseline gap-1.5">
                    <span class="size-2 shrink-0 rounded-full" style={`background-color:${segment.color};`}></span>
                    <div class="flex min-w-0 flex-wrap items-baseline gap-1">
                      <p>{segment.label}</p>
                      <p class="tabular-nums">{segment.count} {segment.count === 1 ? t('widgets.milestoneProgress.item') : t('widgets.milestoneProgress.items')}</p>
                    </div>
                  </li>
                {/each}
              {:else}
                <li>{t('widgets.milestoneProgress.noCategorizedWork')}</li>
              {/if}
            </ul>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <EmptyState
      icon={Flag}
      title={t('widgets.milestoneProgress.emptyTitle')}
      description={t('widgets.milestoneProgress.emptySubtitle')}
    />
  {/if}
</div>
