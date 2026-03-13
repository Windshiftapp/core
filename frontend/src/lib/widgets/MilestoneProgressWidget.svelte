<script>
  import { Flag } from 'lucide-svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateShort } from '../utils/dateFormatter.js';

  export let milestones = [];

  const radius = 48;
  const circumference = 2 * Math.PI * radius;
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
        ? segment.category_name.trim()
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
    if (!totalItems || totalItems <= 0) return [];
    let offset = 0;
    return breakdown
      .filter(segment => segment.count > 0)
      .map(segment => {
        const fraction = segment.count / totalItems;
        const arcLength = Math.max(fraction * circumference, 0);
        const dasharray = `${arcLength} ${circumference}`;
        const segmentData = {
          ...segment,
          dasharray,
          offset
        };
        offset -= arcLength;
        return segmentData;
      });
  }
</script>

<div class="milestones-container">
  {#if milestones && milestones.length > 0}
    <div class="milestone-grid">
      {#each milestones as milestone (milestone.milestone_id)}
        {@const breakdown = normalizeBreakdown(milestone.status_breakdown)}
        {@const segments = buildSegments(breakdown, milestone.total_items)}

        <div class="milestone-card">
          <div class="card-header">
            <div class="title-group">
              <div
                class="icon-pill"
                style={`background-color: color-mix(in srgb, ${milestone.category_color || '#2563eb'} 12%, transparent);`}
              >
                <Flag class="icon" style={`color: ${milestone.category_color || '#2563eb'};`} />
              </div>
              <div class="title-text">
                <p class="milestone-name">{milestone.milestone_name}</p>
                {#if milestone.target_date}
                  <p class="milestone-date">{t('widgets.milestoneProgress.due')} {formatDate(milestone.target_date)}</p>
                {/if}
              </div>
            </div>
            <div class="percent-chip">
              {formatPercent(milestone.percent_complete)}%
            </div>
          </div>

          <div class="card-body">
            <div class="pie-wrapper">
              {#if milestone.total_items > 0}
                <svg viewBox="0 0 140 140" role="img" aria-label="Milestone status breakdown">
                  <circle
                    cx="70"
                    cy="70"
                    r={radius}
                    fill="transparent"
                    stroke="var(--ds-border)"
                    stroke-width="16"
                  />
                  {#each segments as segment (segment.key)}
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
                  <text class="pie-total" x="70" y="68">{milestone.total_items || 0}</text>
                  <text class="pie-label" x="70" y="84">{t('widgets.milestoneProgress.items')}</text>
                </svg>
              {:else}
                <div class="pie-empty">
                  <p>{t('widgets.milestoneProgress.noItems')}</p>
                </div>
              {/if}
            </div>

            <div class="summary">
              <p class="summary-value">
                {milestone.completed_items || 0}/{milestone.total_items || 0} {t('widgets.milestoneProgress.done')}
              </p>
              <p class="summary-subtle">
                {milestone.status ? milestone.status.replace(/_/g, ' ') : t('widgets.milestoneProgress.activeMilestone')}
              </p>
            </div>

            <ul class="legend">
              {#if breakdown.length > 0}
                {#each breakdown as segment (segment.key)}
                  <li>
                    <span class="legend-dot" style={`background-color:${segment.color};`}></span>
                    <div>
                      <p class="legend-label">{segment.label}</p>
                      <p class="legend-value">{segment.count} {segment.count === 1 ? t('widgets.milestoneProgress.item') : t('widgets.milestoneProgress.items')}</p>
                    </div>
                  </li>
                {/each}
              {:else}
                <li class="legend-empty">{t('widgets.milestoneProgress.noCategorizedWork')}</li>
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

<style>
  .milestones-container {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    overflow: visible;
  }

  .milestone-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    align-items: flex-start;
  }

  .milestone-card {
    border: 1px solid var(--ds-border);
    border-radius: 1rem;
    padding: 1rem;
    background: var(--ds-surface-raised);
    flex: 0 1 320px;
    min-width: 260px;
    max-width: 360px;
    width: min(100%, 320px);
    box-sizing: border-box;
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    margin-bottom: 0.75rem;
  }

  .title-group {
    display: flex;
    gap: 0.75rem;
    align-items: center;
  }

  .icon-pill {
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 999px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .icon {
    width: 1.1rem;
    height: 1.1rem;
  }

  .title-text {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .milestone-name {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--ds-text);
  }

  .milestone-date {
    font-size: 0.78rem;
    color: var(--ds-text-subtle);
  }

  .percent-chip {
    background: rgba(16, 185, 129, 0.12);
    color: #047857;
    font-weight: 600;
    padding: 0.3rem 0.65rem;
    border-radius: 999px;
    font-size: 0.85rem;
  }

  .card-body {
    display: grid;
    grid-template-columns: minmax(140px, 160px) 1fr;
    gap: 1rem;
    align-items: center;
  }

  .pie-wrapper {
    display: flex;
    justify-content: center;
    align-items: center;
  }

  svg {
    width: 140px;
    height: 140px;
  }

  .pie-total {
    font-size: 1.5rem;
    font-weight: 600;
    text-anchor: middle;
    fill: var(--ds-text);
  }

  .pie-label {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    text-anchor: middle;
    fill: var(--ds-text-subtle);
  }

  .pie-empty {
    width: 140px;
    height: 140px;
    border-radius: 50%;
    border: 1px dashed var(--ds-border);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--ds-text-subtlest);
    font-size: 0.85rem;
  }

  .summary {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .summary-value {
    font-weight: 600;
    color: var(--ds-text);
  }

  .summary-subtle {
    font-size: 0.8rem;
    color: var(--ds-text-subtle);
    text-transform: capitalize;
  }

  .legend {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 0.75rem;
    list-style: none;
    padding: 0;
    margin: 0.75rem 0 0;
  }

  .legend li {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .legend-dot {
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .legend-label {
    font-size: 0.82rem;
    color: var(--ds-text);
    font-weight: 500;
  }

  .legend-value {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }

  .legend-empty {
    font-size: 0.8rem;
    color: var(--ds-text-subtle);
  }

  @media (max-width: 768px) {
    .milestone-grid {
      flex-direction: column;
    }

    .card-body {
      grid-template-columns: 1fr;
    }

    .legend {
      grid-template-columns: 1fr;
    }
  }
</style>
