<script>
  import { onMount } from 'svelte';
  import { ShieldCheck, ShieldX, Settings } from 'lucide-svelte';
  import { api } from '../api.js';

  let { workspaceId, collectionId = null } = $props();

  let loading = $state(true);
  let error = $state(null);
  let coverageData = $state(null);

  // Pie chart configuration
  const radius = 48;
  const circumference = 2 * Math.PI * radius;
  const coveredColor = 'var(--ds-status-success-solid, #10b981)';
  const notCoveredColor = 'var(--ds-status-danger-solid, #ef4444)';

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
      error = err.message || 'Failed to load coverage data';
    } finally {
      loading = false;
    }
  }

  function buildPieSegments(covered, notCovered, total) {
    if (!total || total <= 0) return [];

    const segments = [];
    let offset = 0;

    if (covered > 0) {
      const fraction = covered / total;
      const arcLength = fraction * circumference;
      segments.push({
        key: 'covered',
        color: coveredColor,
        dasharray: `${arcLength} ${circumference}`,
        offset: offset,
        label: 'Covered',
        count: covered
      });
      offset -= arcLength;
    }

    if (notCovered > 0) {
      const fraction = notCovered / total;
      const arcLength = fraction * circumference;
      segments.push({
        key: 'not-covered',
        color: notCoveredColor,
        dasharray: `${arcLength} ${circumference}`,
        offset: offset,
        label: 'Not Covered',
        count: notCovered
      });
    }

    return segments;
  }

  const segments = $derived(coverageData ? buildPieSegments(coverageData.covered, coverageData.not_covered, coverageData.total) : []);
  const coverageRate = $derived(coverageData?.coverage_rate ?? 0);
</script>

<div class="coverage-widget">
  {#if loading}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <p>Loading coverage data...</p>
    </div>
  {:else if error}
    <div class="error-state">
      <p>{error}</p>
      <button class="retry-btn" onclick={loadCoverageData}>Retry</button>
    </div>
  {:else if !coverageData || coverageData.total === 0}
    <div class="empty-state">
      <ShieldX class="empty-icon" />
      <p class="empty-title">No requirements configured</p>
      <p class="empty-copy">
        Configure requirement types in the Test Reports page to see coverage data.
      </p>
    </div>
  {:else}
    <div class="coverage-content">
      <div class="pie-wrapper">
        <svg viewBox="0 0 140 140" role="img" aria-label="Test coverage breakdown">
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
          <text class="pie-percent" x="70" y="68">{Math.round(coverageRate)}%</text>
          <text class="pie-label" x="70" y="84">covered</text>
        </svg>
      </div>

      <div class="summary">
        <p class="summary-value">
          {coverageData.covered}/{coverageData.total} requirements
        </p>
        <p class="summary-subtle">
          have linked test cases
        </p>
      </div>

      <ul class="legend">
        <li>
          <span class="legend-dot covered"></span>
          <div>
            <p class="legend-label">Covered</p>
            <p class="legend-value">{coverageData.covered} requirements</p>
          </div>
        </li>
        <li>
          <span class="legend-dot not-covered"></span>
          <div>
            <p class="legend-label">Not Covered</p>
            <p class="legend-value">{coverageData.not_covered} requirements</p>
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
  .error-state,
  .empty-state {
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

  .empty-state :global(.empty-icon) {
    width: 48px;
    height: 48px;
    color: var(--ds-text-subtle);
    opacity: 0.5;
  }

  .empty-title {
    font-weight: 600;
    color: var(--ds-text);
    margin-top: 0.5rem;
  }

  .empty-copy {
    font-size: 0.875rem;
    max-width: 280px;
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
