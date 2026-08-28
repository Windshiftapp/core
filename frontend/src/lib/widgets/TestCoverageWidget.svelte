<script>
  import { onMount } from 'svelte';
  import { ShieldX } from '@lucide/svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PieChartSegments from '../components/PieChartSegments.svelte';
  import { api } from '../api.js';
  import { buildCoveragePieSegments } from '../utils/pieChart.js';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId, collectionId = null } = $props();

  let loading = $state(true);
  let error = $state(null);
  let coverageData = $state(null);

  // Pie chart configuration
  const radius = 48;
  const coveredColor = 'var(--ds-status-success-solid, #10b981)';
  const notCoveredColor = 'var(--ds-status-danger-solid, #ef4444)';;

  onMount(() => {
    loadCoverageData();
  });

  async function loadCoverageData() {
    try {
      loading = true;
      error = null;
      const id = collectionId || 'default';
      coverageData = await api.tests.coverage.getSummary(id, workspaceId);
    } catch (err) {
      console.error('Failed to load test coverage:', err);
      error = t('widgets.testCoverage.loadError');
    } finally {
      loading = false;
    }
  }

  function buildPieSegments(covered, notCovered, total) {
    return buildCoveragePieSegments(covered, notCovered, total, coveredColor, notCoveredColor, radius);
  }

  const segments = $derived(coverageData ? buildPieSegments(coverageData.covered, coverageData.not_covered, coverageData.total) : []);
  const coverageRate = $derived(coverageData?.coverage_rate ?? 0);
</script>

<div class="coverage-widget">
  {#if loading}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <p>{t('widgets.testCoverage.loading')}</p>
    </div>
  {:else if error}
    <div class="error-state">
      <p>{error}</p>
      <button class="retry-btn" onclick={loadCoverageData}>{t('common.retry')}</button>
    </div>
  {:else if !coverageData || coverageData.total === 0}
    <EmptyState
      icon={ShieldX}
      title={t('widgets.testCoverage.emptyTitle')}
      description={t('widgets.testCoverage.emptyDescription')}
    />
  {:else}
    <div class="coverage-content">
      <div class="pie-wrapper">
        <svg viewBox="0 0 140 140" role="img" aria-label={t('widgets.testCoverage.chartAria')}>
          <PieChartSegments {segments} {radius} />
          <text class="pie-percent" x="70" y="68">{Math.round(coverageRate)}%</text>
          <text class="pie-label" x="70" y="84">{t('widgets.testCoverage.covered')}</text>
        </svg>
      </div>

      <div class="summary">
        <p class="summary-value">
          {t('widgets.testCoverage.requirementCount', { covered: coverageData.covered, total: coverageData.total })}
        </p>
        <p class="summary-subtle">
          {t('widgets.testCoverage.linkedCases')}
        </p>
      </div>

      <ul class="legend">
        <li>
          <span class="legend-dot covered"></span>
          <div>
            <p class="legend-label">{t('widgets.testCoverage.covered')}</p>
            <p class="legend-value">{t('widgets.testCoverage.requirements', { count: coverageData.covered })}</p>
          </div>
        </li>
        <li>
          <span class="legend-dot not-covered"></span>
          <div>
            <p class="legend-label">{t('widgets.testCoverage.notCovered')}</p>
            <p class="legend-value">{t('widgets.testCoverage.requirements', { count: coverageData.not_covered })}</p>
          </div>
        </li>
      </ul>
    </div>
  {/if}
</div>

<style>
  .coverage-widget {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 280px;
  }

  .loading-state,
  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    flex: 1;
    text-align: center;
    gap: 0.5rem;
    color: var(--ds-text-subtle);
    padding: 1rem;
  }

  .loading-spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--ds-border);
    border-top-color: var(--ds-accent);
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .retry-btn {
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
    background: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    cursor: pointer;
  }

  .retry-btn:hover {
    background: var(--ds-surface-sunken);
  }

  .coverage-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
    padding: 0.5rem;
  }

  .pie-wrapper {
    width: 140px;
    height: 140px;
  }

  .pie-wrapper svg {
    width: 100%;
    height: 100%;
  }

  .pie-wrapper :global(.pie-percent) {
    font-size: 1.5rem;
    font-weight: 700;
    fill: var(--ds-text);
    text-anchor: middle;
    dominant-baseline: central;
  }

  .pie-wrapper :global(.pie-label) {
    font-size: 0.75rem;
    fill: var(--ds-text-subtle);
    text-anchor: middle;
    dominant-baseline: central;
  }

  .summary {
    text-align: center;
  }

  .summary-value {
    font-size: 1rem;
    font-weight: 600;
    color: var(--ds-text);
  }

  .summary-subtle {
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
  }

  .legend {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    width: 100%;
  }

  .legend li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .legend-dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .legend-dot.covered {
    background-color: var(--ds-status-success-solid, #10b981);
  }

  .legend-dot.not-covered {
    background-color: var(--ds-status-danger-solid, #ef4444);
  }

  .legend-label {
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--ds-text);
  }

  .legend-value {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }
</style>
