<script>
  import Chart from '../../widgets/Chart.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  let { data } = $props();
  let metric = $state('items');
</script>

{#if data && data.data_points?.length > 1}
  {@const pts = data.data_points.map(point => metric === 'points' ? {
    ...point, remaining: point.remaining_points ?? 0, completed: point.completed_points ?? 0, ideal: point.ideal_points ?? 0
  } : point)}
  {@const fmtD = (s) => { const d = new Date(s); return `${String(d.getMonth()+1).padStart(2,'0')}/${String(d.getDate()).padStart(2,'0')}`; }}
  <div class="rounded-xl border p-6 mb-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="flex items-center justify-between gap-4 mb-4">
      <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('iterations.burndownChart')}</h3>
      <select data-testid="iteration-burndown-metric" aria-label={t('iterations.burndownChart')} bind:value={metric}
        class="rounded-md border px-3 py-2 text-sm" style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);">
        <option value="items">{t('iterations.totalItems')}</option>
        <option value="points">{t('items.storyPoints')}</option>
      </select>
    </div>
    <Chart
      type="line"
      series={[
        { key: 'remaining', label: t('iterations.remaining'), color: '#3b82f6', values: pts.map(d => d.remaining), smooth: false, showArea: true, showPoints: true, strokeWidth: 2.5 },
        { key: 'ideal', label: t('iterations.idealProgress'), color: '#9ca3af', values: pts.map(d => d.ideal), smooth: false, showArea: false, showPoints: true, pointRadius: 3, strokeWidth: 2, dashed: true }
      ]}
      categories={pts.map(d => fmtD(d.date))}
      maxValue={Math.max(0, ...pts.map(point => Math.max(point.remaining + point.completed, point.ideal)))}
      emptyMessage={t('iterations.noBurndownData')}
    >
      {#snippet tooltipContent({ index, category, seriesValues })}
        <div style="font-weight:600;margin-bottom:0.25rem;border-bottom:1px solid var(--ds-border);padding-bottom:0.25rem;">{category}</div>
        <div style="display:flex;justify-content:space-between;gap:0.5rem;margin-top:0.25rem;">
          <span style="color:var(--ds-text-subtle);">{t('iterations.remaining')}:</span>
          <span style="font-weight:500;color:#3b82f6;">{seriesValues[0].value}</span>
        </div>
        <div style="display:flex;justify-content:space-between;gap:0.5rem;margin-top:0.25rem;">
          <span style="color:var(--ds-text-subtle);">{t('iterations.completed')}:</span>
          <span style="font-weight:500;color:#22c55e;">{pts[index].completed}</span>
        </div>
        <div style="display:flex;justify-content:space-between;gap:0.5rem;margin-top:0.25rem;color:var(--ds-text-subtle);font-size:0.7rem;">
          <span>{t('iterations.ideal')}:</span>
          <span>{seriesValues[1].value}</span>
        </div>
      {/snippet}
    </Chart>
  </div>
{/if}
