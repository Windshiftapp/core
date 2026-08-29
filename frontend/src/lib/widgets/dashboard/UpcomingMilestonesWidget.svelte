<script>
  import { Target, CheckCircle, AlertCircle } from '@lucide/svelte';
  import { homepageStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';

  let milestones = $derived(homepageStore.upcomingMilestones);
  let loading = $derived(homepageStore.loading);

  function barColor(percent, overdue) {
    if (overdue && percent < 100) return 'var(--ds-status-danger-bg)';
    if (percent >= 100) return 'var(--ds-status-success-bg)';
    return 'var(--ds-status-info-bg)';
  }
</script>

{#if loading && milestones.length === 0}
  <div class="space-y-3 animate-pulse">
    {#each Array(2) as _}
      <div class="h-14 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if milestones.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Target class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">{t('dashboard.states.noUpcomingMilestones')}</p>
  </div>
{:else}
  <ul class="flex flex-col gap-3">
    {#each milestones as m (m.milestone_id)}
      {@const daysUntil = homepageStore.calculateDaysUntil(m.target_date)}
      {@const overdue = daysUntil !== null && daysUntil < 0 && m.percent_complete < 100}
      {@const complete = m.percent_complete >= 100}
      <li
        class="p-3 rounded border"
        style={`border-color: var(--ds-border); background-color: var(--ds-surface);`}
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5">
              {#if complete}
                <CheckCircle class="w-4 h-4" style="color: var(--ds-icon-success);" />
              {:else if overdue}
                <AlertCircle class="w-4 h-4" style="color: var(--ds-icon-danger);" />
              {:else}
                <Target class="w-4 h-4" style="color: var(--ds-icon-accent);" />
              {/if}
              <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                {m.milestone_name}
              </span>
            </div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {t('dashboard.states.milestoneProgress', { done: m.done_items, total: m.total_items })}
              {#if m.target_date}
                ·
                {#if overdue}
                  <span style="color: var(--ds-text-danger);">{t('dashboard.states.daysOverdue', { days: Math.abs(daysUntil) })}</span>
                {:else if daysUntil === 0}
                  {t('dashboard.dueToday')}
                {:else if daysUntil !== null}
                  {t('dashboard.states.daysLeft', { days: daysUntil })}
                {/if}
              {/if}
            </p>
          </div>
          <span class="text-sm font-semibold" style="color: var(--ds-text);">
            {Math.round(m.percent_complete)}%
          </span>
        </div>
        <div
          class="mt-2 h-1.5 rounded-full overflow-hidden"
          style="background-color: var(--ds-background-neutral);"
        >
          <div
            class="h-full rounded-full transition-all"
            style={`width: ${Math.min(100, Math.max(0, m.percent_complete))}%; background-color: ${barColor(m.percent_complete, overdue)};`}
          ></div>
        </div>
      </li>
    {/each}
  </ul>
{/if}
