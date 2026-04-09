<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Text from '../../components/Text.svelte';

  let { data = null } = $props();

  const forecasts = $derived(data?.forecasts || []);
  const remaining = $derived(data?.remaining_items || 0);
  const remainingPoints = $derived(data?.remaining_points || 0);
  const throughput = $derived(data?.throughput_samples || []);
  const method = $derived(data?.method || 'linear');

  const throughputStats = $derived.by(() => {
    if (!throughput.length) return { avg: 0, min: 0, max: 0 };
    const sum = throughput.reduce((a, b) => a + b, 0);
    return {
      avg: (sum / throughput.length).toFixed(1),
      min: Math.min(...throughput),
      max: Math.max(...throughput),
    };
  });

  function formatDate(dateStr) {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }
</script>

{#if data}
  <div class="forecast-panel">
    <!-- Low data warning -->
    {#if data.data_quality && !data.data_quality.sufficient}
      <div class="mb-4 p-3 rounded-lg flex items-start gap-2" style="background: color-mix(in srgb, var(--ds-warning, #f59e0b) 10%, transparent); border: 1px solid color-mix(in srgb, var(--ds-warning, #f59e0b) 30%, transparent);">
        <span style="color: var(--ds-warning, #f59e0b); font-size: 14px;">⚠</span>
        <Text variant="subtle" size="sm">
          {#if data.data_quality.reason && t('analytics.insufficientData.' + data.data_quality.reason) !== 'analytics.insufficientData.' + data.data_quality.reason}
            {t('analytics.insufficientData.' + data.data_quality.reason)}
          {:else}
            {t('analytics.forecast.lowDataWarning')}
          {/if}
        </Text>
      </div>
    {/if}

    <!-- Remaining work -->
    <div class="grid grid-cols-2 gap-3 mb-4">
      <div class="p-3 rounded-lg" style="background: var(--ds-surface-sunken);">
        <Text variant="subtle" size="xs">{t('analytics.forecast.remainingItems')}</Text>
        <div class="text-2xl font-bold mt-1" style="color: var(--ds-text);">{remaining}</div>
      </div>
      <div class="p-3 rounded-lg" style="background: var(--ds-surface-sunken);">
        <Text variant="subtle" size="xs">{t('analytics.forecast.remainingPoints')}</Text>
        <div class="text-2xl font-bold mt-1" style="color: var(--ds-text);">{remainingPoints}</div>
      </div>
    </div>

    <!-- Throughput stats -->
    {#if throughput.length > 0}
      <div class="mb-4">
        <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider mb-2">{t('analytics.forecast.throughput')}</Text>
        <div class="flex gap-4 text-sm" style="color: var(--ds-text-subtle);">
          <span>{t('analytics.forecast.avg')}: <strong style="color: var(--ds-text);">{throughputStats.avg}</strong></span>
          <span>{t('analytics.forecast.min')}: <strong style="color: var(--ds-text);">{throughputStats.min}</strong></span>
          <span>{t('analytics.forecast.max')}: <strong style="color: var(--ds-text);">{throughputStats.max}</strong></span>
        </div>
      </div>
    {/if}

    <!-- Forecast table -->
    {#if forecasts.length > 0}
      <div class="mb-3">
        <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider mb-2">{t('analytics.forecast.predictions')}</Text>
        <div class="rounded-lg overflow-hidden border" style="border-color: var(--ds-border);">
          <table class="w-full text-sm">
            <thead>
              <tr style="background: var(--ds-surface-sunken);">
                <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text-subtle);">{t('analytics.forecast.confidence')}</th>
                <th class="text-right px-3 py-2 font-medium" style="color: var(--ds-text-subtle);">{t('analytics.forecast.iterations')}</th>
                <th class="text-right px-3 py-2 font-medium" style="color: var(--ds-text-subtle);">{t('analytics.forecast.estimatedDate')}</th>
              </tr>
            </thead>
            <tbody>
              {#each forecasts as forecast}
                <tr class="border-t" style="border-color: var(--ds-border);">
                  <td class="px-3 py-2" style="color: var(--ds-text);">{forecast.confidence}%</td>
                  <td class="px-3 py-2 text-right" style="color: var(--ds-text);">{forecast.iterations_remaining}</td>
                  <td class="px-3 py-2 text-right" style="color: var(--ds-text-subtle);">{formatDate(forecast.estimated_date)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    <!-- Method indicator -->
    <div class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtlest);">
      <span>{t('analytics.forecast.method')}:</span>
      <span class="px-1.5 py-0.5 rounded" style="background: var(--ds-surface-sunken);">
        {method === 'monte_carlo' ? 'Monte Carlo' : 'Linear'}
      </span>
    </div>
  </div>
{:else}
  <div class="flex flex-col items-center justify-center py-10 gap-2" style="color: var(--ds-text-subtle);">
    <Text variant="subtle" size="sm">{t('analytics.insufficientData.no_iterations')}</Text>
  </div>
{/if}
